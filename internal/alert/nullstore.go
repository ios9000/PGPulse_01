package alert

import (
	"context"
	"time"
)

// NullAlertHistoryStore discards all writes and returns empty results.
// Used in live mode where no persistent storage is available.
type NullAlertHistoryStore struct{}

// NewNullAlertHistoryStore creates a NullAlertHistoryStore.
func NewNullAlertHistoryStore() *NullAlertHistoryStore {
	return &NullAlertHistoryStore{}
}

func (n *NullAlertHistoryStore) Record(_ context.Context, _ *AlertEvent) error {
	return nil
}

func (n *NullAlertHistoryStore) Resolve(_ context.Context, _, _ string, _ time.Time) error {
	return nil
}

func (n *NullAlertHistoryStore) ListUnresolved(_ context.Context) ([]AlertEvent, error) {
	return nil, nil
}

func (n *NullAlertHistoryStore) Query(_ context.Context, _ AlertHistoryQuery) ([]AlertEvent, error) {
	return nil, nil
}

func (n *NullAlertHistoryStore) Cleanup(_ context.Context, _ time.Duration) (int64, error) {
	return 0, nil
}
