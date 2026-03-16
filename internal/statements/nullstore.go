package statements

import (
	"context"
	"time"
)

// NullSnapshotStore is a no-op implementation of SnapshotStore.
// Used when PGSS snapshot storage is not configured.
type NullSnapshotStore struct{}

func (n *NullSnapshotStore) WriteSnapshot(_ context.Context, _ Snapshot, _ []SnapshotEntry) (int64, error) {
	return 0, nil
}

func (n *NullSnapshotStore) GetSnapshot(_ context.Context, _ int64) (*Snapshot, error) {
	return nil, nil
}

func (n *NullSnapshotStore) GetSnapshotEntries(_ context.Context, _ int64, _, _ int) ([]SnapshotEntry, int, error) {
	return nil, 0, nil
}

func (n *NullSnapshotStore) ListSnapshots(_ context.Context, _ string, _ SnapshotListOptions) ([]Snapshot, int, error) {
	return nil, 0, nil
}

func (n *NullSnapshotStore) GetLatestSnapshots(_ context.Context, _ string, _ int) ([]Snapshot, error) {
	return nil, nil
}

func (n *NullSnapshotStore) GetEntriesForQuery(_ context.Context, _ string, _ int64, _, _ time.Time) ([]SnapshotEntry, []Snapshot, error) {
	return nil, nil, nil
}

func (n *NullSnapshotStore) CleanOld(_ context.Context, _ time.Time) error {
	return nil
}
