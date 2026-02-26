package orchestrator

import (
	"context"
	"log/slog"

	"github.com/ios9000/PGPulse_01/internal/collector"
)

// LogStore is a MetricStore that logs writes instead of persisting them.
// Used during development before the real TimescaleDB storage is implemented (M2).
type LogStore struct {
	logger *slog.Logger
}

// NewLogStore creates a LogStore backed by the given logger.
func NewLogStore(logger *slog.Logger) *LogStore {
	return &LogStore{logger: logger}
}

// Write logs the batch size and a sample metric name at debug level.
func (s *LogStore) Write(_ context.Context, points []collector.MetricPoint) error {
	attrs := []any{"count", len(points)}
	if len(points) > 0 {
		attrs = append(attrs, "sample", points[0].Metric)
	}
	s.logger.Debug("stored metric points", attrs...)
	return nil
}

// Query is a no-op stub; returns nil, nil until real storage is implemented.
func (s *LogStore) Query(_ context.Context, _ collector.MetricQuery) ([]collector.MetricPoint, error) {
	return nil, nil
}

// Close is a no-op; LogStore holds no resources.
func (s *LogStore) Close() error {
	return nil
}
