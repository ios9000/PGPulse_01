package statements

import (
	"context"
	"time"
)

// SnapshotStore persists and retrieves pg_stat_statements snapshots.
type SnapshotStore interface {
	WriteSnapshot(ctx context.Context, snap Snapshot, entries []SnapshotEntry) (int64, error)
	GetSnapshot(ctx context.Context, id int64) (*Snapshot, error)
	GetSnapshotEntries(ctx context.Context, snapshotID int64, limit, offset int) ([]SnapshotEntry, int, error)
	ListSnapshots(ctx context.Context, instanceID string, opts SnapshotListOptions) ([]Snapshot, int, error)
	GetLatestSnapshots(ctx context.Context, instanceID string, n int) ([]Snapshot, error)
	GetEntriesForQuery(ctx context.Context, instanceID string, queryID int64, from, to time.Time) ([]SnapshotEntry, []Snapshot, error)
	CleanOld(ctx context.Context, olderThan time.Time) error
}
