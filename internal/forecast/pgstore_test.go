//go:build integration

package forecast_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ios9000/PGPulse_01/internal/forecast"
	"github.com/ios9000/PGPulse_01/internal/storage"

	"log/slog"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("PGPULSE_TEST_DSN")
	if dsn == "" {
		t.Skip("PGPULSE_TEST_DSN not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	logger := slog.Default()
	if err := storage.Migrate(ctx, pool, logger, storage.MigrateOptions{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// Clean tables before each test.
	_, _ = pool.Exec(ctx, "DELETE FROM maintenance_operations")
	_, _ = pool.Exec(ctx, "DELETE FROM maintenance_forecasts")

	return pool
}

func TestPGStore_WriteAndListOperations(t *testing.T) {
	pool := testPool(t)
	store := forecast.NewPGForecastStore(pool, slog.Default())
	ctx := context.Background()

	op := &forecast.MaintenanceOperation{
		InstanceID:     "inst-1",
		Operation:      "vacuum",
		Outcome:        "completed",
		Database:       "mydb",
		Table:          "orders",
		TableSizeBytes: 1024000,
		StartedAt:      time.Now().Add(-5 * time.Minute),
		CompletedAt:    time.Now(),
		DurationSec:    300,
		FinalPct:       100,
		AvgRatePerSec:  340,
		Metadata:       map[string]any{"phase": "scanning heap"},
	}

	if err := store.WriteOperation(ctx, op); err != nil {
		t.Fatalf("WriteOperation: %v", err)
	}
	if op.ID == 0 {
		t.Error("expected non-zero ID after insert")
	}

	ops, total, err := store.ListOperations(ctx, forecast.OperationFilter{
		InstanceID: "inst-1",
		Limit:      10,
	})
	if err != nil {
		t.Fatalf("ListOperations: %v", err)
	}
	if total != 1 {
		t.Errorf("total = %d, want 1", total)
	}
	if len(ops) != 1 {
		t.Fatalf("len(ops) = %d, want 1", len(ops))
	}
	if ops[0].Operation != "vacuum" {
		t.Errorf("operation = %q, want %q", ops[0].Operation, "vacuum")
	}
	if ops[0].Outcome != "completed" {
		t.Errorf("outcome = %q, want %q", ops[0].Outcome, "completed")
	}
}

func TestPGStore_ListOperations_FilterByOperation(t *testing.T) {
	pool := testPool(t)
	store := forecast.NewPGForecastStore(pool, slog.Default())
	ctx := context.Background()

	for _, opType := range []string{"vacuum", "analyze", "vacuum"} {
		err := store.WriteOperation(ctx, &forecast.MaintenanceOperation{
			InstanceID:  "inst-1",
			Operation:   opType,
			Outcome:     "completed",
			StartedAt:   time.Now().Add(-1 * time.Minute),
			CompletedAt: time.Now(),
			DurationSec: 60,
			Metadata:    map[string]any{},
		})
		if err != nil {
			t.Fatalf("WriteOperation: %v", err)
		}
	}

	ops, total, err := store.ListOperations(ctx, forecast.OperationFilter{
		InstanceID: "inst-1",
		Operation:  "vacuum",
		Limit:      10,
	})
	if err != nil {
		t.Fatalf("ListOperations: %v", err)
	}
	if total != 2 {
		t.Errorf("total = %d, want 2 (vacuum only)", total)
	}
	if len(ops) != 2 {
		t.Errorf("len(ops) = %d, want 2", len(ops))
	}
}

func TestPGStore_ListOperations_FilterByDatabase(t *testing.T) {
	pool := testPool(t)
	store := forecast.NewPGForecastStore(pool, slog.Default())
	ctx := context.Background()

	for _, db := range []string{"db1", "db2", "db1"} {
		err := store.WriteOperation(ctx, &forecast.MaintenanceOperation{
			InstanceID:  "inst-1",
			Operation:   "vacuum",
			Outcome:     "completed",
			Database:    db,
			StartedAt:   time.Now().Add(-1 * time.Minute),
			CompletedAt: time.Now(),
			DurationSec: 60,
			Metadata:    map[string]any{},
		})
		if err != nil {
			t.Fatalf("WriteOperation: %v", err)
		}
	}

	ops, total, err := store.ListOperations(ctx, forecast.OperationFilter{
		InstanceID: "inst-1",
		Database:   "db1",
		Limit:      10,
	})
	if err != nil {
		t.Fatalf("ListOperations: %v", err)
	}
	if total != 2 {
		t.Errorf("total = %d, want 2 (db1 only)", total)
	}
	if len(ops) != 2 {
		t.Errorf("len(ops) = %d, want 2", len(ops))
	}
}

func TestPGStore_ListOperations_Pagination(t *testing.T) {
	pool := testPool(t)
	store := forecast.NewPGForecastStore(pool, slog.Default())
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		err := store.WriteOperation(ctx, &forecast.MaintenanceOperation{
			InstanceID:  "inst-1",
			Operation:   "analyze",
			Outcome:     "completed",
			StartedAt:   time.Now().Add(time.Duration(-5+i) * time.Minute),
			CompletedAt: time.Now().Add(time.Duration(-4+i) * time.Minute),
			DurationSec: 60,
			Metadata:    map[string]any{},
		})
		if err != nil {
			t.Fatalf("WriteOperation: %v", err)
		}
	}

	// Page 1: first 2.
	ops, total, err := store.ListOperations(ctx, forecast.OperationFilter{
		InstanceID: "inst-1",
		Limit:      2,
		Offset:     0,
	})
	if err != nil {
		t.Fatalf("ListOperations page 1: %v", err)
	}
	if total != 5 {
		t.Errorf("total = %d, want 5", total)
	}
	if len(ops) != 2 {
		t.Errorf("page 1 len = %d, want 2", len(ops))
	}

	// Page 2: next 2.
	ops2, _, err := store.ListOperations(ctx, forecast.OperationFilter{
		InstanceID: "inst-1",
		Limit:      2,
		Offset:     2,
	})
	if err != nil {
		t.Fatalf("ListOperations page 2: %v", err)
	}
	if len(ops2) != 2 {
		t.Errorf("page 2 len = %d, want 2", len(ops2))
	}

	// Page 3: last 1.
	ops3, _, err := store.ListOperations(ctx, forecast.OperationFilter{
		InstanceID: "inst-1",
		Limit:      2,
		Offset:     4,
	})
	if err != nil {
		t.Fatalf("ListOperations page 3: %v", err)
	}
	if len(ops3) != 1 {
		t.Errorf("page 3 len = %d, want 1", len(ops3))
	}
}

func TestPGStore_CleanOldOperations(t *testing.T) {
	pool := testPool(t)
	store := forecast.NewPGForecastStore(pool, slog.Default())
	ctx := context.Background()

	// Old operation (90 days ago).
	err := store.WriteOperation(ctx, &forecast.MaintenanceOperation{
		InstanceID:  "inst-1",
		Operation:   "vacuum",
		Outcome:     "completed",
		StartedAt:   time.Now().Add(-91 * 24 * time.Hour),
		CompletedAt: time.Now().Add(-90 * 24 * time.Hour),
		DurationSec: 60,
		Metadata:    map[string]any{},
	})
	if err != nil {
		t.Fatalf("WriteOperation old: %v", err)
	}

	// Recent operation.
	err = store.WriteOperation(ctx, &forecast.MaintenanceOperation{
		InstanceID:  "inst-1",
		Operation:   "vacuum",
		Outcome:     "completed",
		StartedAt:   time.Now().Add(-1 * time.Hour),
		CompletedAt: time.Now(),
		DurationSec: 60,
		Metadata:    map[string]any{},
	})
	if err != nil {
		t.Fatalf("WriteOperation recent: %v", err)
	}

	cutoff := time.Now().Add(-30 * 24 * time.Hour)
	deleted, err := store.CleanOldOperations(ctx, cutoff)
	if err != nil {
		t.Fatalf("CleanOldOperations: %v", err)
	}
	if deleted != 1 {
		t.Errorf("deleted = %d, want 1", deleted)
	}

	// Verify only recent remains.
	_, total, err := store.ListOperations(ctx, forecast.OperationFilter{
		InstanceID: "inst-1",
		Limit:      10,
	})
	if err != nil {
		t.Fatalf("ListOperations: %v", err)
	}
	if total != 1 {
		t.Errorf("remaining total = %d, want 1", total)
	}
}

func TestPGStore_UpsertForecast_InsertAndOverwrite(t *testing.T) {
	pool := testPool(t)
	store := forecast.NewPGForecastStore(pool, slog.Default())
	ctx := context.Background()
	now := time.Now()

	f := &forecast.MaintenanceForecast{
		InstanceID:   "inst-1",
		Database:     "mydb",
		Table:        "orders",
		Operation:    "vacuum",
		Status:       "predicted",
		TimeUntilSec: 7200,
		CurrentValue: 100,
		ThresholdValue: 500,
		AccumulationRate: 0.5,
		Method:       "threshold_projection",
		EvaluatedAt:  now,
	}

	// Insert.
	if err := store.UpsertForecast(ctx, f); err != nil {
		t.Fatalf("UpsertForecast insert: %v", err)
	}

	forecasts, err := store.ListForecasts(ctx, forecast.ForecastFilter{
		InstanceID: "inst-1",
	})
	if err != nil {
		t.Fatalf("ListForecasts: %v", err)
	}
	if len(forecasts) != 1 {
		t.Fatalf("len = %d, want 1", len(forecasts))
	}
	if forecasts[0].Status != "predicted" {
		t.Errorf("status = %q, want %q", forecasts[0].Status, "predicted")
	}

	// Upsert (overwrite).
	f.Status = "imminent"
	f.TimeUntilSec = 1800
	if err := store.UpsertForecast(ctx, f); err != nil {
		t.Fatalf("UpsertForecast overwrite: %v", err)
	}

	forecasts, err = store.ListForecasts(ctx, forecast.ForecastFilter{
		InstanceID: "inst-1",
	})
	if err != nil {
		t.Fatalf("ListForecasts after upsert: %v", err)
	}
	if len(forecasts) != 1 {
		t.Fatalf("len = %d, want 1 (upsert should overwrite)", len(forecasts))
	}
	if forecasts[0].Status != "imminent" {
		t.Errorf("status = %q, want %q after overwrite", forecasts[0].Status, "imminent")
	}
}

func TestPGStore_ListForecasts_FilterByStatus(t *testing.T) {
	pool := testPool(t)
	store := forecast.NewPGForecastStore(pool, slog.Default())
	ctx := context.Background()
	now := time.Now()

	statuses := []string{"overdue", "imminent", "predicted", "not_needed"}
	for i, s := range statuses {
		err := store.UpsertForecast(ctx, &forecast.MaintenanceForecast{
			InstanceID:  "inst-1",
			Database:    "mydb",
			Table:       "table_" + s,
			Operation:   "vacuum",
			Status:      s,
			CurrentValue: float64(i * 100),
			ThresholdValue: 500,
			Method:      "threshold_projection",
			EvaluatedAt: now,
		})
		if err != nil {
			t.Fatalf("UpsertForecast %s: %v", s, err)
		}
	}

	// Filter for actionable only.
	forecasts, err := store.ListForecasts(ctx, forecast.ForecastFilter{
		InstanceID: "inst-1",
		Statuses:   []string{"imminent", "overdue"},
	})
	if err != nil {
		t.Fatalf("ListForecasts filtered: %v", err)
	}
	if len(forecasts) != 2 {
		t.Errorf("len = %d, want 2 (imminent + overdue)", len(forecasts))
	}
}

func TestPGStore_ListForecasts_Ordering(t *testing.T) {
	pool := testPool(t)
	store := forecast.NewPGForecastStore(pool, slog.Default())
	ctx := context.Background()
	now := time.Now()

	// Insert in reverse priority order.
	entries := []struct {
		table  string
		status string
		timeUntil float64
	}{
		{"t_predicted", "predicted", 7200},
		{"t_overdue", "overdue", 0},
		{"t_imminent", "imminent", 1800},
		{"t_not_needed", "not_needed", 0},
	}
	for _, e := range entries {
		err := store.UpsertForecast(ctx, &forecast.MaintenanceForecast{
			InstanceID:   "inst-1",
			Database:     "mydb",
			Table:        e.table,
			Operation:    "vacuum",
			Status:       e.status,
			TimeUntilSec: e.timeUntil,
			Method:       "threshold_projection",
			EvaluatedAt:  now,
		})
		if err != nil {
			t.Fatalf("UpsertForecast: %v", err)
		}
	}

	forecasts, err := store.ListForecasts(ctx, forecast.ForecastFilter{
		InstanceID: "inst-1",
	})
	if err != nil {
		t.Fatalf("ListForecasts: %v", err)
	}
	if len(forecasts) != 4 {
		t.Fatalf("len = %d, want 4", len(forecasts))
	}

	// Expected order: overdue, imminent, predicted, not_needed.
	expectedStatuses := []string{"overdue", "imminent", "predicted", "not_needed"}
	for i, f := range forecasts {
		if f.Status != expectedStatuses[i] {
			t.Errorf("forecasts[%d].Status = %q, want %q", i, f.Status, expectedStatuses[i])
		}
	}
}

func TestPGStore_UpsertForecasts_Batch(t *testing.T) {
	pool := testPool(t)
	store := forecast.NewPGForecastStore(pool, slog.Default())
	ctx := context.Background()
	now := time.Now()

	batch := []forecast.MaintenanceForecast{
		{InstanceID: "inst-1", Database: "db1", Table: "t1", Operation: "vacuum", Status: "predicted", Method: "threshold_projection", EvaluatedAt: now},
		{InstanceID: "inst-1", Database: "db1", Table: "t2", Operation: "vacuum", Status: "imminent", Method: "threshold_projection", EvaluatedAt: now},
		{InstanceID: "inst-1", Database: "db1", Table: "t1", Operation: "analyze", Status: "not_needed", Method: "threshold_projection", EvaluatedAt: now},
	}

	if err := store.UpsertForecasts(ctx, batch); err != nil {
		t.Fatalf("UpsertForecasts: %v", err)
	}

	forecasts, err := store.ListForecasts(ctx, forecast.ForecastFilter{
		InstanceID: "inst-1",
	})
	if err != nil {
		t.Fatalf("ListForecasts: %v", err)
	}
	if len(forecasts) != 3 {
		t.Errorf("len = %d, want 3", len(forecasts))
	}
}

func TestPGStore_DeleteForecasts(t *testing.T) {
	pool := testPool(t)
	store := forecast.NewPGForecastStore(pool, slog.Default())
	ctx := context.Background()
	now := time.Now()

	err := store.UpsertForecast(ctx, &forecast.MaintenanceForecast{
		InstanceID: "inst-1", Database: "db1", Table: "t1", Operation: "vacuum",
		Status: "predicted", Method: "threshold_projection", EvaluatedAt: now,
	})
	if err != nil {
		t.Fatalf("UpsertForecast: %v", err)
	}

	if err := store.DeleteForecasts(ctx, "inst-1"); err != nil {
		t.Fatalf("DeleteForecasts: %v", err)
	}

	forecasts, err := store.ListForecasts(ctx, forecast.ForecastFilter{InstanceID: "inst-1"})
	if err != nil {
		t.Fatalf("ListForecasts: %v", err)
	}
	if len(forecasts) != 0 {
		t.Errorf("len = %d, want 0 after delete", len(forecasts))
	}
}
