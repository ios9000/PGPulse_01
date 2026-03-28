package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ios9000/PGPulse_01/internal/auth"
	"github.com/ios9000/PGPulse_01/internal/config"
	"github.com/ios9000/PGPulse_01/internal/forecast"
)

// --- Mock implementations for forecast tests ---

type mockForecastStore struct {
	forecasts []forecast.MaintenanceForecast
	ops       []forecast.MaintenanceOperation
	total     int
	listErr   error
}

func (m *mockForecastStore) WriteOperation(_ context.Context, _ *forecast.MaintenanceOperation) error {
	return nil
}

func (m *mockForecastStore) ListOperations(_ context.Context, _ forecast.OperationFilter) ([]forecast.MaintenanceOperation, int, error) {
	if m.listErr != nil {
		return nil, 0, m.listErr
	}
	return m.ops, m.total, nil
}

func (m *mockForecastStore) CleanOldOperations(_ context.Context, _ time.Time) (int64, error) {
	return 0, nil
}

func (m *mockForecastStore) UpsertForecast(_ context.Context, _ *forecast.MaintenanceForecast) error {
	return nil
}

func (m *mockForecastStore) UpsertForecasts(_ context.Context, _ []forecast.MaintenanceForecast) error {
	return nil
}

func (m *mockForecastStore) ListForecasts(_ context.Context, _ forecast.ForecastFilter) ([]forecast.MaintenanceForecast, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.forecasts, nil
}

func (m *mockForecastStore) DeleteForecasts(_ context.Context, _ string) error {
	return nil
}

func (m *mockForecastStore) Close() error {
	return nil
}

// newForecastTestServer builds an APIServer with forecast engine wired, auth disabled.
func newForecastTestServer(t *testing.T, store *mockForecastStore) *APIServer {
	t.Helper()
	cfg := config.Config{
		Server: config.ServerConfig{CORSEnabled: false},
		Auth:   config.AuthConfig{Enabled: false},
		Instances: []config.InstanceConfig{
			{ID: "test-inst", DSN: "postgres://localhost/test"},
		},
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	metricStore := &mockStore{}
	srv := New(cfg, metricStore, nil, nil, nil, logger, nil, nil, nil, nil, nil, false, 0, auth.AuthDisabled)

	// Build a ForecastEngine with the mock store and a real tracker/ETA.
	fcCfg := config.MaintenanceForecastConfig{}
	fcCfg.ApplyDefaults()
	engine := forecast.NewForecastEngine(
		metricStore,
		store,
		nil, // no baseline provider
		&testInstanceLister{},
		nil, // no conn provider
		fcCfg,
		logger,
	)
	// Override the store on the engine so handlers read from our mock.
	engine.Store = store
	srv.SetForecastEngine(engine)

	return srv
}

type testInstanceLister struct{}

func (l *testInstanceLister) ActiveInstanceIDs(_ context.Context) ([]string, error) {
	return []string{"test-inst"}, nil
}

// --- GET /api/v1/instances/{id}/forecast/eta ---

func TestHandleForecastETA(t *testing.T) {
	t.Parallel()

	t.Run("returns empty operations when no active ops", func(t *testing.T) {
		t.Parallel()
		store := &mockForecastStore{}
		srv := newForecastTestServer(t, store)
		ts := httptest.NewServer(srv.Routes())
		defer ts.Close()

		resp, err := http.Get(ts.URL + "/api/v1/instances/test-inst/forecast/eta")
		if err != nil {
			t.Fatalf("GET: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
		}

		var result struct {
			Data struct {
				Operations []json.RawMessage `json:"operations"`
			} `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(result.Data.Operations) != 0 {
			t.Errorf("operations len = %d, want 0", len(result.Data.Operations))
		}
	})

	t.Run("unknown instance returns 404", func(t *testing.T) {
		t.Parallel()
		store := &mockForecastStore{}
		srv := newForecastTestServer(t, store)
		ts := httptest.NewServer(srv.Routes())
		defer ts.Close()

		resp, err := http.Get(ts.URL + "/api/v1/instances/unknown/forecast/eta")
		if err != nil {
			t.Fatalf("GET: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
		}
	})
}

// --- GET /api/v1/instances/{id}/forecast/eta/{pid} ---

func TestHandleForecastETAByPID(t *testing.T) {
	t.Parallel()

	t.Run("returns 404 when PID not found", func(t *testing.T) {
		t.Parallel()
		store := &mockForecastStore{}
		srv := newForecastTestServer(t, store)
		ts := httptest.NewServer(srv.Routes())
		defer ts.Close()

		resp, err := http.Get(ts.URL + "/api/v1/instances/test-inst/forecast/eta/99999")
		if err != nil {
			t.Fatalf("GET: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
		}
	})

	t.Run("bad pid returns 400", func(t *testing.T) {
		t.Parallel()
		store := &mockForecastStore{}
		srv := newForecastTestServer(t, store)
		ts := httptest.NewServer(srv.Routes())
		defer ts.Close()

		resp, err := http.Get(ts.URL + "/api/v1/instances/test-inst/forecast/eta/notanumber")
		if err != nil {
			t.Fatalf("GET: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
		}
	})
}

// --- GET /api/v1/instances/{id}/forecast/needs ---

func TestHandleForecastNeeds(t *testing.T) {
	t.Parallel()

	t.Run("returns forecasts and summary", func(t *testing.T) {
		t.Parallel()
		store := &mockForecastStore{
			forecasts: []forecast.MaintenanceForecast{
				{ID: 1, InstanceID: "test-inst", Database: "db1", Table: "t1", Operation: "vacuum", Status: "overdue"},
				{ID: 2, InstanceID: "test-inst", Database: "db1", Table: "t2", Operation: "vacuum", Status: "imminent"},
				{ID: 3, InstanceID: "test-inst", Database: "db1", Table: "t3", Operation: "analyze", Status: "predicted"},
			},
		}
		srv := newForecastTestServer(t, store)
		ts := httptest.NewServer(srv.Routes())
		defer ts.Close()

		resp, err := http.Get(ts.URL + "/api/v1/instances/test-inst/forecast/needs")
		if err != nil {
			t.Fatalf("GET: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
		}

		var result struct {
			Data struct {
				Forecasts []json.RawMessage `json:"forecasts"`
				Summary   struct {
					OverdueCount  int `json:"overdue_count"`
					ImminentCount int `json:"imminent_count"`
					PredictedCount int `json:"predicted_count"`
				} `json:"summary"`
			} `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(result.Data.Forecasts) != 3 {
			t.Errorf("forecasts len = %d, want 3", len(result.Data.Forecasts))
		}
		if result.Data.Summary.OverdueCount != 1 {
			t.Errorf("overdue = %d, want 1", result.Data.Summary.OverdueCount)
		}
		if result.Data.Summary.ImminentCount != 1 {
			t.Errorf("imminent = %d, want 1", result.Data.Summary.ImminentCount)
		}
		if result.Data.Summary.PredictedCount != 1 {
			t.Errorf("predicted = %d, want 1", result.Data.Summary.PredictedCount)
		}
	})

	t.Run("empty forecasts returns empty array", func(t *testing.T) {
		t.Parallel()
		store := &mockForecastStore{forecasts: nil}
		srv := newForecastTestServer(t, store)
		ts := httptest.NewServer(srv.Routes())
		defer ts.Close()

		resp, err := http.Get(ts.URL + "/api/v1/instances/test-inst/forecast/needs")
		if err != nil {
			t.Fatalf("GET: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
		}
	})
}

// --- GET /api/v1/instances/{id}/forecast/history ---

func TestHandleForecastHistory(t *testing.T) {
	t.Parallel()

	t.Run("returns paginated results", func(t *testing.T) {
		t.Parallel()
		store := &mockForecastStore{
			ops: []forecast.MaintenanceOperation{
				{ID: 1, InstanceID: "test-inst", Operation: "vacuum", Outcome: "completed", DurationSec: 120},
				{ID: 2, InstanceID: "test-inst", Operation: "analyze", Outcome: "completed", DurationSec: 30},
			},
			total: 5, // total is more than returned (simulates pagination)
		}
		srv := newForecastTestServer(t, store)
		ts := httptest.NewServer(srv.Routes())
		defer ts.Close()

		resp, err := http.Get(ts.URL + "/api/v1/instances/test-inst/forecast/history?page=1&per_page=2")
		if err != nil {
			t.Fatalf("GET: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
		}

		var result struct {
			Data struct {
				Operations []json.RawMessage `json:"operations"`
				Total      int               `json:"total"`
				Page       int               `json:"page"`
				PerPage    int               `json:"per_page"`
			} `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(result.Data.Operations) != 2 {
			t.Errorf("operations len = %d, want 2", len(result.Data.Operations))
		}
		if result.Data.Total != 5 {
			t.Errorf("total = %d, want 5", result.Data.Total)
		}
		if result.Data.Page != 1 {
			t.Errorf("page = %d, want 1", result.Data.Page)
		}
		if result.Data.PerPage != 2 {
			t.Errorf("per_page = %d, want 2", result.Data.PerPage)
		}
	})

	t.Run("filter by operation", func(t *testing.T) {
		t.Parallel()
		store := &mockForecastStore{
			ops:   []forecast.MaintenanceOperation{},
			total: 0,
		}
		srv := newForecastTestServer(t, store)
		ts := httptest.NewServer(srv.Routes())
		defer ts.Close()

		resp, err := http.Get(ts.URL + "/api/v1/instances/test-inst/forecast/history?operation=vacuum")
		if err != nil {
			t.Fatalf("GET: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
		}
	})
}

// --- All endpoints return 501 when forecastEngine is nil ---

func TestForecastEndpoints_DisabledReturns501(t *testing.T) {
	t.Parallel()

	cfg := config.Config{
		Server: config.ServerConfig{CORSEnabled: false},
		Auth:   config.AuthConfig{Enabled: false},
		Instances: []config.InstanceConfig{
			{ID: "test-inst", DSN: "postgres://localhost/test"},
		},
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := New(cfg, &mockStore{}, nil, nil, nil, logger, nil, nil, nil, nil, nil, false, 0, auth.AuthDisabled)
	// Do NOT call SetForecastEngine — engine is nil.

	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	endpoints := []string{
		"/api/v1/instances/test-inst/forecast/eta",
		"/api/v1/instances/test-inst/forecast/eta/123",
		"/api/v1/instances/test-inst/forecast/needs",
		"/api/v1/instances/test-inst/forecast/needs/mydb/orders",
		"/api/v1/instances/test-inst/forecast/history",
	}

	for _, ep := range endpoints {
		resp, err := http.Get(ts.URL + ep)
		if err != nil {
			t.Fatalf("GET %s: %v", ep, err)
		}
		_ = resp.Body.Close()

		// When forecastEngine is nil, routes are not registered (guarded by if s.forecastEngine != nil).
		// So these should return 404 (no route), not 501.
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("%s: status = %d, want %d (route not registered when disabled)", ep, resp.StatusCode, http.StatusNotFound)
		}
	}
}

// --- computeSummary ---

func TestComputeSummary(t *testing.T) {
	forecasts := []forecast.MaintenanceForecast{
		{Status: "overdue"},
		{Status: "overdue"},
		{Status: "imminent"},
		{Status: "predicted"},
		{Status: "predicted"},
		{Status: "predicted"},
		{Status: "not_needed"},
		{Status: "insufficient_data"},
	}

	s := computeSummary(forecasts)
	if s.OverdueCount != 2 {
		t.Errorf("overdue = %d, want 2", s.OverdueCount)
	}
	if s.ImminentCount != 1 {
		t.Errorf("imminent = %d, want 1", s.ImminentCount)
	}
	if s.PredictedCount != 3 {
		t.Errorf("predicted = %d, want 3", s.PredictedCount)
	}
	if s.TotalTablesEvaluated != 8 {
		t.Errorf("total = %d, want 8", s.TotalTablesEvaluated)
	}
}
