package remediation

import (
	"context"
	"time"
)

// NullStore discards all writes and returns empty results.
// Used in live mode where no persistent storage is available.
type NullStore struct{}

// NewNullStore creates a NullStore.
func NewNullStore() *NullStore {
	return &NullStore{}
}

func (n *NullStore) Write(_ context.Context, _ []Recommendation) ([]Recommendation, error) {
	return nil, nil
}

func (n *NullStore) ListByInstance(_ context.Context, _ string, _ ListOpts) ([]Recommendation, int, error) {
	return nil, 0, nil
}

func (n *NullStore) ListAll(_ context.Context, _ ListOpts) ([]Recommendation, int, error) {
	return nil, 0, nil
}

func (n *NullStore) ListByAlertEvent(_ context.Context, _ int64) ([]Recommendation, error) {
	return nil, nil
}

func (n *NullStore) Acknowledge(_ context.Context, _ int64, _ string) error {
	return nil
}

func (n *NullStore) CleanOld(_ context.Context, _ time.Duration) error {
	return nil
}
