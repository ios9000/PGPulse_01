package api

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/ios9000/PGPulse_01/internal/auth"
	"github.com/ios9000/PGPulse_01/internal/collector"
	"github.com/ios9000/PGPulse_01/internal/config"
	"github.com/ios9000/PGPulse_01/internal/storage"
)

// mockStore is a collector.MetricStore that returns preset results.
type mockStore struct {
	points    []collector.MetricPoint
	err       error
	lastQuery collector.MetricQuery
}

func (m *mockStore) Write(_ context.Context, _ []collector.MetricPoint) error {
	return m.err
}

func (m *mockStore) Query(_ context.Context, q collector.MetricQuery) ([]collector.MetricPoint, error) {
	m.lastQuery = q
	return m.points, m.err
}

func (m *mockStore) Close() error { return nil }

// mockMetricsStore extends mockStore with MetricsQuerier methods.
type mockMetricsStore struct {
	mockStore
	currentResult *storage.CurrentMetricsResult
	historyResult *storage.HistoryResult
	metricValues  map[string]float64
	currentErr    error
	historyErr    error
	valuesErr     error
}

func (m *mockMetricsStore) CurrentMetrics(_ context.Context, _ string) (*storage.CurrentMetricsResult, error) {
	return m.currentResult, m.currentErr
}

func (m *mockMetricsStore) HistoryMetrics(_ context.Context, _ storage.HistoryRequest) (*storage.HistoryResult, error) {
	return m.historyResult, m.historyErr
}

func (m *mockMetricsStore) CurrentMetricValues(_ context.Context, _ string) (map[string]float64, error) {
	return m.metricValues, m.valuesErr
}

// mockPinger is an api.Pinger that returns a preset error.
type mockPinger struct {
	err error
}

func (m *mockPinger) Ping(_ context.Context) error { return m.err }

// newTestServer creates an APIServer with auth disabled for handler unit tests.
func newTestServer(
	_ *testing.T,
	store collector.MetricStore,
	pool Pinger,
	instances []config.InstanceConfig,
) *APIServer {
	cfg := config.Config{
		Server:    config.ServerConfig{CORSEnabled: false},
		Auth:      config.AuthConfig{Enabled: false},
		Instances: instances,
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return New(cfg, store, pool, nil, nil, logger, nil, nil, nil, nil, nil, false, 0, auth.AuthDisabled)
}
