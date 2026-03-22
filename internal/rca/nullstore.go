package rca

import (
	"context"
	"time"
)

// NullIncidentStore is a no-op IncidentStore for live mode (no persistent storage).
type NullIncidentStore struct{}

// NewNullIncidentStore returns a no-op IncidentStore.
func NewNullIncidentStore() *NullIncidentStore {
	return &NullIncidentStore{}
}

func (s *NullIncidentStore) Create(_ context.Context, _ *Incident) (int64, error) {
	return 0, nil
}

func (s *NullIncidentStore) Get(_ context.Context, _ int64) (*Incident, error) {
	return nil, nil
}

func (s *NullIncidentStore) ListByInstance(_ context.Context, _ string, _, _ int) ([]Incident, int, error) {
	return nil, 0, nil
}

func (s *NullIncidentStore) ListAll(_ context.Context, _, _ int) ([]Incident, int, error) {
	return nil, 0, nil
}

func (s *NullIncidentStore) UpdateReview(_ context.Context, _ int64, _, _ string) error {
	return nil
}

func (s *NullIncidentStore) Cleanup(_ context.Context, _ time.Duration) (int64, error) {
	return 0, nil
}
