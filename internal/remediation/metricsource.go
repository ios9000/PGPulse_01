package remediation

import (
	"context"
	"time"

	"github.com/ios9000/PGPulse_01/internal/collector"
)

// StoreMetricSource implements MetricSource using a collector.MetricStore.
type StoreMetricSource struct {
	store collector.MetricStore
}

// NewStoreMetricSource creates a StoreMetricSource.
func NewStoreMetricSource(store collector.MetricStore) *StoreMetricSource {
	return &StoreMetricSource{store: store}
}

// CurrentSnapshot queries the store for the last 2 minutes of metrics and returns
// the latest value per metric key.
func (s *StoreMetricSource) CurrentSnapshot(ctx context.Context, instanceID string) (MetricSnapshot, error) {
	snap := make(MetricSnapshot)
	now := time.Now()
	points, err := s.store.Query(ctx, collector.MetricQuery{
		InstanceID: instanceID,
		Start:      now.Add(-2 * time.Minute),
		End:        now,
	})
	if err != nil {
		return snap, err
	}
	for _, p := range points {
		snap[p.Metric] = p.Value
	}
	return snap, nil
}
