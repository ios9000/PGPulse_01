package rca

import (
	"context"
	"time"
)

// MetricStatsProvider computes batch statistics for metrics.
// Used by ThresholdAnomalySource to establish baseline values.
type MetricStatsProvider interface {
	GetMetricStats(ctx context.Context, instanceID string, keys []string, from, to time.Time) (map[string]MetricStats, error)
}

// MetricStats holds aggregated statistics for a single metric key.
type MetricStats struct {
	Mean   float64
	StdDev float64
	Min    float64
	Max    float64
	Count  int
}
