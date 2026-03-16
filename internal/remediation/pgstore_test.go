//go:build integration

package remediation_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ios9000/PGPulse_01/internal/remediation"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("PGPULSE_TEST_DSN")
	if dsn == "" {
		t.Skip("PGPULSE_TEST_DSN not set")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	// Ensure table exists.
	_, err = pool.Exec(context.Background(), `
		CREATE TABLE IF NOT EXISTS remediation_recommendations (
			id BIGSERIAL PRIMARY KEY,
			rule_id TEXT NOT NULL,
			instance_id TEXT NOT NULL,
			alert_event_id BIGINT,
			metric_key TEXT NOT NULL DEFAULT '',
			metric_value DOUBLE PRECISION NOT NULL DEFAULT 0,
			priority TEXT NOT NULL DEFAULT 'info',
			category TEXT NOT NULL DEFAULT 'performance',
			title TEXT NOT NULL DEFAULT '',
			description TEXT NOT NULL DEFAULT '',
			doc_url TEXT NOT NULL DEFAULT '',
			status VARCHAR(20) NOT NULL DEFAULT 'active',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			evaluated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			resolved_at TIMESTAMPTZ,
			acknowledged_at TIMESTAMPTZ,
			acknowledged_by TEXT NOT NULL DEFAULT ''
		)`)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}

	// Clean up before each test.
	_, _ = pool.Exec(context.Background(), "DELETE FROM remediation_recommendations")

	return pool
}

func TestPGStore_WriteAndList(t *testing.T) {
	pool := testPool(t)
	store := remediation.NewPGStore(pool)
	ctx := context.Background()

	recs := []remediation.Recommendation{
		{RuleID: "cache_ratio_low", InstanceID: "inst-1", MetricKey: "pgpulse.cache.hit_ratio", MetricValue: 0.85, Priority: "suggestion", Category: "performance", Title: "Low cache", Description: "Increase shared_buffers"},
		{RuleID: "idle_in_tx", InstanceID: "inst-1", MetricKey: "pgpulse.connections.idle_in_transaction", MetricValue: 5, Priority: "action_required", Category: "performance", Title: "Idle in tx", Description: "Check app connection pool"},
	}

	saved, err := store.Write(ctx, recs)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if len(saved) != 2 {
		t.Fatalf("saved = %d, want 2", len(saved))
	}
	if saved[0].ID == 0 {
		t.Error("expected non-zero ID after insert")
	}

	listed, total, err := store.ListByInstance(ctx, "inst-1", remediation.ListOpts{Limit: 10})
	if err != nil {
		t.Fatalf("ListByInstance: %v", err)
	}
	if total != 2 {
		t.Errorf("total = %d, want 2", total)
	}
	if len(listed) != 2 {
		t.Errorf("listed = %d, want 2", len(listed))
	}
}

func TestPGStore_Filters(t *testing.T) {
	pool := testPool(t)
	store := remediation.NewPGStore(pool)
	ctx := context.Background()

	recs := []remediation.Recommendation{
		{RuleID: "r1", InstanceID: "inst-1", Priority: "suggestion", Category: "performance", Title: "T1"},
		{RuleID: "r2", InstanceID: "inst-1", Priority: "action_required", Category: "capacity", Title: "T2"},
		{RuleID: "r3", InstanceID: "inst-2", Priority: "info", Category: "maintenance", Title: "T3"},
	}
	if _, err := store.Write(ctx, recs); err != nil {
		t.Fatalf("Write: %v", err)
	}

	// Filter by priority.
	listed, total, err := store.ListByInstance(ctx, "inst-1", remediation.ListOpts{Limit: 10, Priority: "suggestion"})
	if err != nil {
		t.Fatalf("ListByInstance: %v", err)
	}
	if total != 1 {
		t.Errorf("total = %d, want 1", total)
	}
	if len(listed) != 1 || listed[0].RuleID != "r1" {
		t.Errorf("filter by priority failed")
	}

	// Filter by category fleet-wide.
	listed, total, err = store.ListAll(ctx, remediation.ListOpts{Limit: 10, Category: "maintenance"})
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	if total != 1 {
		t.Errorf("total = %d, want 1", total)
	}
	if len(listed) != 1 || listed[0].InstanceID != "inst-2" {
		t.Errorf("filter by category failed")
	}
}

func TestPGStore_Acknowledge(t *testing.T) {
	pool := testPool(t)
	store := remediation.NewPGStore(pool)
	ctx := context.Background()

	recs := []remediation.Recommendation{
		{RuleID: "r1", InstanceID: "inst-1", Priority: "suggestion", Category: "performance", Title: "T1"},
	}
	saved, err := store.Write(ctx, recs)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}

	if err := store.Acknowledge(ctx, saved[0].ID, "admin"); err != nil {
		t.Fatalf("Acknowledge: %v", err)
	}

	// Verify acknowledged filter.
	acked := true
	listed, total, err := store.ListByInstance(ctx, "inst-1", remediation.ListOpts{Limit: 10, Acknowledged: &acked})
	if err != nil {
		t.Fatalf("ListByInstance: %v", err)
	}
	if total != 1 || len(listed) != 1 {
		t.Errorf("acknowledged filter: total=%d, len=%d", total, len(listed))
	}
	if listed[0].AcknowledgedBy != "admin" {
		t.Errorf("acknowledged_by = %q, want %q", listed[0].AcknowledgedBy, "admin")
	}

	// Non-existent ID.
	if err := store.Acknowledge(ctx, 999999, "admin"); err == nil {
		t.Error("expected error for non-existent recommendation")
	}
}

func TestPGStore_CleanOld(t *testing.T) {
	pool := testPool(t)
	store := remediation.NewPGStore(pool)
	ctx := context.Background()

	recs := []remediation.Recommendation{
		{RuleID: "r1", InstanceID: "inst-1", Priority: "info", Category: "maintenance", Title: "Old"},
	}
	saved, err := store.Write(ctx, recs)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}

	// Acknowledge and backdate.
	if err := store.Acknowledge(ctx, saved[0].ID, "admin"); err != nil {
		t.Fatalf("Acknowledge: %v", err)
	}
	_, err = pool.Exec(ctx,
		"UPDATE remediation_recommendations SET created_at = NOW() - interval '31 days' WHERE id = $1",
		saved[0].ID)
	if err != nil {
		t.Fatalf("backdate: %v", err)
	}

	if err := store.CleanOld(ctx, 30*24*time.Hour); err != nil {
		t.Fatalf("CleanOld: %v", err)
	}

	_, total, err := store.ListAll(ctx, remediation.ListOpts{Limit: 10})
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	if total != 0 {
		t.Errorf("total = %d, want 0 after cleanup", total)
	}
}

func TestPGStore_AlertEventLink(t *testing.T) {
	pool := testPool(t)
	store := remediation.NewPGStore(pool)
	ctx := context.Background()

	eventID := int64(42)
	recs := []remediation.Recommendation{
		{RuleID: "r1", InstanceID: "inst-1", AlertEventID: &eventID, Priority: "suggestion", Category: "performance", Title: "Alert-linked"},
	}
	if _, err := store.Write(ctx, recs); err != nil {
		t.Fatalf("Write: %v", err)
	}

	listed, err := store.ListByAlertEvent(ctx, eventID)
	if err != nil {
		t.Fatalf("ListByAlertEvent: %v", err)
	}
	if len(listed) != 1 {
		t.Errorf("len = %d, want 1", len(listed))
	}
	if listed[0].AlertEventID == nil || *listed[0].AlertEventID != eventID {
		t.Errorf("alert_event_id = %v, want %d", listed[0].AlertEventID, eventID)
	}
}
