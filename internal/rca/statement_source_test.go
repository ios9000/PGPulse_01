package rca

import (
	"context"
	"testing"
	"time"

	"github.com/ios9000/PGPulse_01/internal/statements"
)

// mockSnapshotStore implements statements.SnapshotStore for testing.
type mockSnapshotStore struct {
	snapshots []statements.Snapshot
	entries   map[int64][]statements.SnapshotEntry
}

func (m *mockSnapshotStore) WriteSnapshot(_ context.Context, _ statements.Snapshot, _ []statements.SnapshotEntry) (int64, error) {
	return 0, nil
}

func (m *mockSnapshotStore) GetSnapshot(_ context.Context, id int64) (*statements.Snapshot, error) {
	for _, s := range m.snapshots {
		if s.ID == id {
			return &s, nil
		}
	}
	return nil, nil
}

func (m *mockSnapshotStore) GetSnapshotEntries(_ context.Context, snapshotID int64, _, _ int) ([]statements.SnapshotEntry, int, error) {
	entries := m.entries[snapshotID]
	return entries, len(entries), nil
}

func (m *mockSnapshotStore) ListSnapshots(_ context.Context, _ string, _ statements.SnapshotListOptions) ([]statements.Snapshot, int, error) {
	return m.snapshots, len(m.snapshots), nil
}

func (m *mockSnapshotStore) GetLatestSnapshots(_ context.Context, _ string, n int) ([]statements.Snapshot, error) {
	if n > len(m.snapshots) {
		n = len(m.snapshots)
	}
	return m.snapshots[:n], nil
}

func (m *mockSnapshotStore) GetEntriesForQuery(_ context.Context, _ string, _ int64, _, _ time.Time) ([]statements.SnapshotEntry, []statements.Snapshot, error) {
	return nil, nil, nil
}

func (m *mockSnapshotStore) CleanOld(_ context.Context, _ time.Time) error {
	return nil
}

func TestStatementDiffSource_Regression(t *testing.T) {
	t.Parallel()

	now := time.Now()
	store := &mockSnapshotStore{
		snapshots: []statements.Snapshot{
			{ID: 2, InstanceID: "inst-1", CapturedAt: now.Add(-5 * time.Minute)},
			{ID: 1, InstanceID: "inst-1", CapturedAt: now.Add(-35 * time.Minute)},
		},
		entries: map[int64][]statements.SnapshotEntry{
			1: {
				{QueryID: 100, Calls: 1000, TotalExecTime: 5000, Query: "SELECT 1"},
			},
			2: {
				{QueryID: 100, Calls: 2000, TotalExecTime: 30000, Query: "SELECT 1"}, // 3x slower avg
			},
		},
	}

	src := NewStatementDiffSource(store)
	anomalies, err := src.GetAnomalies(context.Background(), "inst-1",
		now.Add(-30*time.Minute), now)
	if err != nil {
		t.Fatalf("GetAnomalies error: %v", err)
	}

	regressions := anomalies["pg.statements.regression"]
	if len(regressions) == 0 {
		t.Fatal("expected at least one regression anomaly")
	}
	if regressions[0].Source != "statement_diff" {
		t.Errorf("expected source 'statement_diff', got %s", regressions[0].Source)
	}
}

func TestStatementDiffSource_NewQuery(t *testing.T) {
	t.Parallel()

	now := time.Now()
	store := &mockSnapshotStore{
		snapshots: []statements.Snapshot{
			{ID: 2, InstanceID: "inst-1", CapturedAt: now.Add(-5 * time.Minute)},
			{ID: 1, InstanceID: "inst-1", CapturedAt: now.Add(-35 * time.Minute)},
		},
		entries: map[int64][]statements.SnapshotEntry{
			1: {}, // empty
			2: {
				{QueryID: 200, Calls: 50, TotalExecTime: 5000, Query: "SELECT new_thing()"},
			},
		},
	}

	src := NewStatementDiffSource(store)
	anomalies, err := src.GetAnomalies(context.Background(), "inst-1",
		now.Add(-30*time.Minute), now)
	if err != nil {
		t.Fatalf("GetAnomalies error: %v", err)
	}

	newQueries := anomalies["pg.statements.new_query"]
	if len(newQueries) == 0 {
		t.Fatal("expected at least one new query anomaly")
	}
}

func TestStatementDiffSource_InsufficientSnapshots(t *testing.T) {
	t.Parallel()

	now := time.Now()
	store := &mockSnapshotStore{
		snapshots: []statements.Snapshot{
			{ID: 1, InstanceID: "inst-1", CapturedAt: now.Add(-5 * time.Minute)},
		},
	}

	src := NewStatementDiffSource(store)
	anomalies, err := src.GetAnomalies(context.Background(), "inst-1",
		now.Add(-30*time.Minute), now)
	if err != nil {
		t.Fatalf("GetAnomalies error: %v", err)
	}

	if len(anomalies) != 0 {
		t.Errorf("expected empty anomaly map with single snapshot, got %d keys", len(anomalies))
	}
}
