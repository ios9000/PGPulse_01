//go:build integration

package alert_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/ios9000/PGPulse_01/internal/alert"
	"github.com/ios9000/PGPulse_01/internal/storage"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

func setupAlertDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()

	pgc, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("pgpulse_test"),
		postgres.WithUsername("pgpulse"),
		postgres.WithPassword("secret"),
	)
	if err != nil {
		t.Fatalf("start postgres container: %v", err)
	}
	t.Cleanup(func() { _ = pgc.Terminate(ctx) })

	dsn, err := pgc.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("get connection string: %v", err)
	}

	pool, err := storage.NewPool(ctx, dsn)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	t.Cleanup(pool.Close)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := storage.Migrate(ctx, pool, logger, storage.MigrateOptions{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	return pool
}

func makeTestRule(id string) *alert.Rule {
	return &alert.Rule{
		ID:               id,
		Name:             "Test Rule " + id,
		Description:      "Test description",
		Metric:           "pg.test.metric",
		Operator:         alert.OpGreater,
		Threshold:        80,
		Severity:         alert.SeverityWarning,
		ConsecutiveCount: 3,
		CooldownMinutes:  15,
		Source:           alert.SourceBuiltin,
		Enabled:          true,
	}
}

func TestPGAlertRuleStore_CreateAndGet(t *testing.T) {
	pool := setupAlertDB(t)
	store := alert.NewPGAlertRuleStore(pool)
	ctx := context.Background()

	rule := makeTestRule("test_create")
	if err := store.Create(ctx, rule); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := store.Get(ctx, "test_create")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != "test_create" {
		t.Errorf("ID = %q, want %q", got.ID, "test_create")
	}
	if got.Name != "Test Rule test_create" {
		t.Errorf("Name = %q, want %q", got.Name, "Test Rule test_create")
	}
	if got.Threshold != 80 {
		t.Errorf("Threshold = %v, want 80", got.Threshold)
	}
	if got.Operator != alert.OpGreater {
		t.Errorf("Operator = %q, want %q", got.Operator, alert.OpGreater)
	}
}

func TestPGAlertRuleStore_Get_NotFound(t *testing.T) {
	pool := setupAlertDB(t)
	store := alert.NewPGAlertRuleStore(pool)

	_, err := store.Get(context.Background(), "nonexistent")
	if !errors.Is(err, alert.ErrRuleNotFound) {
		t.Errorf("error = %v, want ErrRuleNotFound", err)
	}
}

func TestPGAlertRuleStore_List(t *testing.T) {
	pool := setupAlertDB(t)
	store := alert.NewPGAlertRuleStore(pool)
	ctx := context.Background()

	if err := store.Create(ctx, makeTestRule("rule_a")); err != nil {
		t.Fatalf("Create rule_a: %v", err)
	}
	if err := store.Create(ctx, makeTestRule("rule_b")); err != nil {
		t.Fatalf("Create rule_b: %v", err)
	}

	rules, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rules) != 2 {
		t.Errorf("List count = %d, want 2", len(rules))
	}
}

func TestPGAlertRuleStore_Update(t *testing.T) {
	pool := setupAlertDB(t)
	store := alert.NewPGAlertRuleStore(pool)
	ctx := context.Background()

	rule := makeTestRule("test_update")
	if err := store.Create(ctx, rule); err != nil {
		t.Fatalf("Create: %v", err)
	}

	rule.Threshold = 95
	rule.Name = "Updated Rule"
	if err := store.Update(ctx, rule); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := store.Get(ctx, "test_update")
	if err != nil {
		t.Fatalf("Get after update: %v", err)
	}
	if got.Threshold != 95 {
		t.Errorf("Threshold = %v, want 95", got.Threshold)
	}
	if got.Name != "Updated Rule" {
		t.Errorf("Name = %q, want %q", got.Name, "Updated Rule")
	}
}

func TestPGAlertRuleStore_Delete(t *testing.T) {
	pool := setupAlertDB(t)
	store := alert.NewPGAlertRuleStore(pool)
	ctx := context.Background()

	if err := store.Create(ctx, makeTestRule("test_delete")); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := store.Delete(ctx, "test_delete"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err := store.Get(ctx, "test_delete")
	if !errors.Is(err, alert.ErrRuleNotFound) {
		t.Errorf("Get after delete: error = %v, want ErrRuleNotFound", err)
	}
}

func TestPGAlertRuleStore_UpsertBuiltin_NewRule(t *testing.T) {
	pool := setupAlertDB(t)
	store := alert.NewPGAlertRuleStore(pool)
	ctx := context.Background()

	rule := makeTestRule("test_upsert_new")
	if err := store.UpsertBuiltin(ctx, rule); err != nil {
		t.Fatalf("UpsertBuiltin: %v", err)
	}

	got, err := store.Get(ctx, "test_upsert_new")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Threshold != 80 {
		t.Errorf("Threshold = %v, want 80", got.Threshold)
	}
}

func TestPGAlertRuleStore_UpsertBuiltin_PreservesUserThreshold(t *testing.T) {
	pool := setupAlertDB(t)
	store := alert.NewPGAlertRuleStore(pool)
	ctx := context.Background()

	// Create initial rule
	rule := makeTestRule("test_upsert_preserve")
	if err := store.Create(ctx, rule); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// User modifies threshold via Update
	rule.Threshold = 95
	if err := store.Update(ctx, rule); err != nil {
		t.Fatalf("Update: %v", err)
	}

	// UpsertBuiltin with original threshold (80) — should preserve user's 95
	original := makeTestRule("test_upsert_preserve")
	original.Name = "Updated Name" // metadata changes
	if err := store.UpsertBuiltin(ctx, original); err != nil {
		t.Fatalf("UpsertBuiltin: %v", err)
	}

	got, err := store.Get(ctx, "test_upsert_preserve")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	// Threshold should be preserved at user's value
	if got.Threshold != 95 {
		t.Errorf("Threshold = %v, want 95 (user-modified, should be preserved)", got.Threshold)
	}
	// Name should be updated (metadata field)
	if got.Name != "Updated Name" {
		t.Errorf("Name = %q, want %q (should be updated by upsert)", got.Name, "Updated Name")
	}
}

func TestPGAlertRuleStore_ListEnabled(t *testing.T) {
	pool := setupAlertDB(t)
	store := alert.NewPGAlertRuleStore(pool)
	ctx := context.Background()

	enabled := makeTestRule("rule_enabled")
	enabled.Enabled = true
	if err := store.Create(ctx, enabled); err != nil {
		t.Fatalf("Create enabled: %v", err)
	}

	disabled := makeTestRule("rule_disabled")
	disabled.Enabled = false
	if err := store.Create(ctx, disabled); err != nil {
		t.Fatalf("Create disabled: %v", err)
	}

	rules, err := store.ListEnabled(ctx)
	if err != nil {
		t.Fatalf("ListEnabled: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("ListEnabled count = %d, want 1", len(rules))
	}
	if rules[0].ID != "rule_enabled" {
		t.Errorf("ListEnabled[0].ID = %q, want %q", rules[0].ID, "rule_enabled")
	}
}

func TestPGAlertHistoryStore_RecordAndResolve(t *testing.T) {
	pool := setupAlertDB(t)
	ruleStore := alert.NewPGAlertRuleStore(pool)
	histStore := alert.NewPGAlertHistoryStore(pool)
	ctx := context.Background()

	// Must create rule first (FK constraint)
	if err := ruleStore.Create(ctx, makeTestRule("hist_rule")); err != nil {
		t.Fatalf("Create rule: %v", err)
	}

	firedAt := time.Now().UTC().Truncate(time.Microsecond)
	event := &alert.AlertEvent{
		RuleID:     "hist_rule",
		InstanceID: "inst-1",
		Severity:   alert.SeverityWarning,
		Metric:     "pg.test.metric",
		Value:      90,
		Threshold:  80,
		Operator:   alert.OpGreater,
		FiredAt:    firedAt,
	}

	if err := histStore.Record(ctx, event); err != nil {
		t.Fatalf("Record: %v", err)
	}

	// Verify unresolved
	unresolved, err := histStore.ListUnresolved(ctx)
	if err != nil {
		t.Fatalf("ListUnresolved: %v", err)
	}
	if len(unresolved) != 1 {
		t.Fatalf("unresolved count = %d, want 1", len(unresolved))
	}

	// Resolve
	resolvedAt := time.Now().UTC().Truncate(time.Microsecond)
	if err := histStore.Resolve(ctx, "hist_rule", "inst-1", resolvedAt); err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	// Verify no longer unresolved
	unresolved, err = histStore.ListUnresolved(ctx)
	if err != nil {
		t.Fatalf("ListUnresolved after resolve: %v", err)
	}
	if len(unresolved) != 0 {
		t.Errorf("unresolved count after resolve = %d, want 0", len(unresolved))
	}
}

func TestPGAlertHistoryStore_ListUnresolved(t *testing.T) {
	pool := setupAlertDB(t)
	ruleStore := alert.NewPGAlertRuleStore(pool)
	histStore := alert.NewPGAlertHistoryStore(pool)
	ctx := context.Background()

	if err := ruleStore.Create(ctx, makeTestRule("unresolved_rule")); err != nil {
		t.Fatalf("Create rule: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Microsecond)
	// Record two events
	for i, instID := range []string{"inst-1", "inst-2"} {
		ev := &alert.AlertEvent{
			RuleID:     "unresolved_rule",
			InstanceID: instID,
			Severity:   alert.SeverityWarning,
			Metric:     "pg.test.metric",
			Value:      90,
			Threshold:  80,
			Operator:   alert.OpGreater,
			FiredAt:    now.Add(time.Duration(i) * time.Second),
		}
		if err := histStore.Record(ctx, ev); err != nil {
			t.Fatalf("Record %s: %v", instID, err)
		}
	}

	// Resolve one
	if err := histStore.Resolve(ctx, "unresolved_rule", "inst-1", now); err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	unresolved, err := histStore.ListUnresolved(ctx)
	if err != nil {
		t.Fatalf("ListUnresolved: %v", err)
	}
	if len(unresolved) != 1 {
		t.Fatalf("unresolved count = %d, want 1", len(unresolved))
	}
	if unresolved[0].InstanceID != "inst-2" {
		t.Errorf("unresolved instance = %q, want %q", unresolved[0].InstanceID, "inst-2")
	}
}

func TestPGAlertHistoryStore_Cleanup(t *testing.T) {
	pool := setupAlertDB(t)
	ruleStore := alert.NewPGAlertRuleStore(pool)
	histStore := alert.NewPGAlertHistoryStore(pool)
	ctx := context.Background()

	if err := ruleStore.Create(ctx, makeTestRule("cleanup_rule")); err != nil {
		t.Fatalf("Create rule: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Microsecond)
	ev := &alert.AlertEvent{
		RuleID:     "cleanup_rule",
		InstanceID: "inst-1",
		Severity:   alert.SeverityWarning,
		Metric:     "pg.test.metric",
		Value:      90,
		Threshold:  80,
		Operator:   alert.OpGreater,
		FiredAt:    now,
	}

	if err := histStore.Record(ctx, ev); err != nil {
		t.Fatalf("Record: %v", err)
	}

	// Resolve it (cleanup only deletes resolved events)
	if err := histStore.Resolve(ctx, "cleanup_rule", "inst-1", now); err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	// Cleanup with a 0 duration — should delete everything resolved
	deleted, err := histStore.Cleanup(ctx, 0)
	if err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if deleted != 1 {
		t.Errorf("Cleanup deleted = %d, want 1", deleted)
	}
}

func TestPGAlertHistoryStore_Query(t *testing.T) {
	pool := setupAlertDB(t)
	ruleStore := alert.NewPGAlertRuleStore(pool)
	histStore := alert.NewPGAlertHistoryStore(pool)
	ctx := context.Background()

	if err := ruleStore.Create(ctx, makeTestRule("query_rule")); err != nil {
		t.Fatalf("Create rule: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Microsecond)
	ev := &alert.AlertEvent{
		RuleID:     "query_rule",
		InstanceID: "inst-1",
		Severity:   alert.SeverityWarning,
		Metric:     "pg.test.metric",
		Value:      90,
		Threshold:  80,
		Operator:   alert.OpGreater,
		FiredAt:    now,
	}

	if err := histStore.Record(ctx, ev); err != nil {
		t.Fatalf("Record: %v", err)
	}

	// Query by instance
	events, err := histStore.Query(ctx, alert.AlertHistoryQuery{InstanceID: "inst-1"})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(events) != 1 {
		t.Errorf("Query count = %d, want 1", len(events))
	}

	// Query for non-matching instance
	events, err = histStore.Query(ctx, alert.AlertHistoryQuery{InstanceID: "no-such"})
	if err != nil {
		t.Fatalf("Query no-match: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("Query no-match count = %d, want 0", len(events))
	}
}
