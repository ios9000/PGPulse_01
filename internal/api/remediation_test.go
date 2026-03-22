package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ios9000/PGPulse_01/internal/auth"
	"github.com/ios9000/PGPulse_01/internal/config"
	"github.com/ios9000/PGPulse_01/internal/remediation"
)

// --- Mock implementations for remediation tests ---

type mockRemediationStore struct {
	recs        []remediation.Recommendation
	total       int
	listErr     error
	ackErr      error
	ackCalled   bool
	lastOpts    remediation.ListOpts
}

func (m *mockRemediationStore) Write(_ context.Context, recs []remediation.Recommendation) ([]remediation.Recommendation, error) {
	return recs, nil
}

func (m *mockRemediationStore) ListByInstance(_ context.Context, _ string, opts remediation.ListOpts) ([]remediation.Recommendation, int, error) {
	m.lastOpts = opts
	if m.listErr != nil {
		return nil, 0, m.listErr
	}
	return m.recs, m.total, nil
}

func (m *mockRemediationStore) ListAll(_ context.Context, opts remediation.ListOpts) ([]remediation.Recommendation, int, error) {
	m.lastOpts = opts
	if m.listErr != nil {
		return nil, 0, m.listErr
	}
	return m.recs, m.total, nil
}

func (m *mockRemediationStore) ListByAlertEvent(_ context.Context, _ int64) ([]remediation.Recommendation, error) {
	return m.recs, nil
}

func (m *mockRemediationStore) Acknowledge(_ context.Context, _ int64, _ string) error {
	m.ackCalled = true
	return m.ackErr
}

func (m *mockRemediationStore) CleanOld(_ context.Context, _ time.Duration) error {
	return nil
}

func (m *mockRemediationStore) ResolveStale(_ context.Context, _ string, _ []string) error {
	return nil
}

func (m *mockRemediationStore) Upsert(_ context.Context, _ remediation.Recommendation) error {
	return nil
}

func (m *mockRemediationStore) ListByIncident(_ context.Context, _ int64) ([]remediation.Recommendation, error) {
	return nil, nil
}

type mockMetricSource struct {
	snapshot remediation.MetricSnapshot
	err      error
}

func (m *mockMetricSource) CurrentSnapshot(_ context.Context, _ string) (remediation.MetricSnapshot, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.snapshot, nil
}

// newRemediationTestServer builds an APIServer with remediation wired, auth disabled.
func newRemediationTestServer(
	t *testing.T,
	store remediation.RecommendationStore,
	source remediation.MetricSource,
) *APIServer {
	t.Helper()
	cfg := config.Config{
		Server: config.ServerConfig{CORSEnabled: false},
		Auth:   config.AuthConfig{Enabled: false},
		Instances: []config.InstanceConfig{
			{ID: "test-inst", DSN: "postgres://localhost/test"},
		},
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := New(cfg, &mockStore{}, nil, nil, nil, logger, nil, nil, nil, nil, nil, false, 0, auth.AuthDisabled)
	engine := remediation.NewEngine()
	srv.SetRemediation(engine, store, source)
	return srv
}

// --- GET /api/v1/instances/{id}/recommendations ---

func TestHandleListRecommendations(t *testing.T) {
	t.Parallel()

	t.Run("returns recommendations", func(t *testing.T) {
		t.Parallel()
		store := &mockRemediationStore{
			recs: []remediation.Recommendation{
				{ID: 1, RuleID: "cache_ratio_low", InstanceID: "test-inst", Priority: "suggestion", Category: "performance", Title: "Low cache hit ratio"},
				{ID: 2, RuleID: "idle_in_tx", InstanceID: "test-inst", Priority: "action_required", Category: "performance", Title: "Idle in transaction"},
			},
			total: 2,
		}
		srv := newRemediationTestServer(t, store, nil)
		ts := httptest.NewServer(srv.Routes())
		defer ts.Close()

		resp, err := http.Get(ts.URL + "/api/v1/instances/test-inst/recommendations")
		if err != nil {
			t.Fatalf("GET: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
		}

		var env Envelope
		if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
			t.Fatalf("decode: %v", err)
		}
		meta := env.Meta.(map[string]interface{})
		if int(meta["count"].(float64)) != 2 {
			t.Errorf("count = %v, want 2", meta["count"])
		}
		if int(meta["total"].(float64)) != 2 {
			t.Errorf("total = %v, want 2", meta["total"])
		}
	})

	t.Run("empty result returns empty array", func(t *testing.T) {
		t.Parallel()
		store := &mockRemediationStore{recs: nil, total: 0}
		srv := newRemediationTestServer(t, store, nil)
		ts := httptest.NewServer(srv.Routes())
		defer ts.Close()

		resp, err := http.Get(ts.URL + "/api/v1/instances/test-inst/recommendations")
		if err != nil {
			t.Fatalf("GET: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
		}

		body, _ := io.ReadAll(resp.Body)
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(body, &raw); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if string(raw["data"]) != "[]" {
			t.Errorf("data = %s, want []", string(raw["data"]))
		}
	})

	t.Run("unknown instance returns 404", func(t *testing.T) {
		t.Parallel()
		store := &mockRemediationStore{}
		srv := newRemediationTestServer(t, store, nil)
		ts := httptest.NewServer(srv.Routes())
		defer ts.Close()

		resp, err := http.Get(ts.URL + "/api/v1/instances/unknown/recommendations")
		if err != nil {
			t.Fatalf("GET: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
		}
	})

	t.Run("pagination params forwarded", func(t *testing.T) {
		t.Parallel()
		store := &mockRemediationStore{recs: nil, total: 0}
		srv := newRemediationTestServer(t, store, nil)
		ts := httptest.NewServer(srv.Routes())
		defer ts.Close()

		resp, err := http.Get(ts.URL + "/api/v1/instances/test-inst/recommendations?limit=10&offset=20&priority=suggestion&category=performance")
		if err != nil {
			t.Fatalf("GET: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
		}

		if store.lastOpts.Limit != 10 {
			t.Errorf("limit = %d, want 10", store.lastOpts.Limit)
		}
		if store.lastOpts.Offset != 20 {
			t.Errorf("offset = %d, want 20", store.lastOpts.Offset)
		}
		if store.lastOpts.Priority != "suggestion" {
			t.Errorf("priority = %q, want %q", store.lastOpts.Priority, "suggestion")
		}
		if store.lastOpts.Category != "performance" {
			t.Errorf("category = %q, want %q", store.lastOpts.Category, "performance")
		}
	})

	t.Run("limit capped at 500", func(t *testing.T) {
		t.Parallel()
		store := &mockRemediationStore{recs: nil, total: 0}
		srv := newRemediationTestServer(t, store, nil)
		ts := httptest.NewServer(srv.Routes())
		defer ts.Close()

		resp, err := http.Get(ts.URL + "/api/v1/instances/test-inst/recommendations?limit=9999")
		if err != nil {
			t.Fatalf("GET: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
		}
		if store.lastOpts.Limit != 500 {
			t.Errorf("limit = %d, want 500 (capped)", store.lastOpts.Limit)
		}
	})
}

// --- POST /api/v1/instances/{id}/diagnose ---

func TestHandleDiagnose(t *testing.T) {
	t.Parallel()

	t.Run("returns diagnose results", func(t *testing.T) {
		t.Parallel()
		source := &mockMetricSource{
			snapshot: remediation.MetricSnapshot{
				"pgpulse.cache.hit_ratio": 0.5,
			},
		}
		store := &mockRemediationStore{}
		srv := newRemediationTestServer(t, store, source)
		ts := httptest.NewServer(srv.Routes())
		defer ts.Close()

		req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/instances/test-inst/diagnose", nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("POST: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("status = %d, want %d; body: %s", resp.StatusCode, http.StatusOK, b)
		}

		var env Envelope
		if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
			t.Fatalf("decode: %v", err)
		}
		data, ok := env.Data.(map[string]interface{})
		if !ok {
			t.Fatal("data is not a map")
		}
		if _, ok := data["recommendations"]; !ok {
			t.Error("missing 'recommendations' in response")
		}
		if _, ok := data["metrics_evaluated"]; !ok {
			t.Error("missing 'metrics_evaluated' in response")
		}
		if _, ok := data["rules_evaluated"]; !ok {
			t.Error("missing 'rules_evaluated' in response")
		}
	})

	t.Run("unknown instance returns 404", func(t *testing.T) {
		t.Parallel()
		source := &mockMetricSource{snapshot: remediation.MetricSnapshot{}}
		srv := newRemediationTestServer(t, &mockRemediationStore{}, source)
		ts := httptest.NewServer(srv.Routes())
		defer ts.Close()

		req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/instances/unknown/diagnose", nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("POST: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
		}
	})

	t.Run("no metric source returns 503", func(t *testing.T) {
		t.Parallel()
		srv := newRemediationTestServer(t, &mockRemediationStore{}, nil)
		// Override: set metricSource to nil
		srv.metricSource = nil
		ts := httptest.NewServer(srv.Routes())
		defer ts.Close()

		req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/instances/test-inst/diagnose", nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("POST: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusServiceUnavailable)
		}
	})

	t.Run("metric source error returns 500", func(t *testing.T) {
		t.Parallel()
		source := &mockMetricSource{err: fmt.Errorf("db down")}
		srv := newRemediationTestServer(t, &mockRemediationStore{}, source)
		ts := httptest.NewServer(srv.Routes())
		defer ts.Close()

		req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/instances/test-inst/diagnose", nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("POST: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusInternalServerError {
			t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusInternalServerError)
		}
	})
}

// --- GET /api/v1/recommendations ---

func TestHandleListAllRecommendations(t *testing.T) {
	t.Parallel()

	t.Run("returns fleet-wide recommendations", func(t *testing.T) {
		t.Parallel()
		store := &mockRemediationStore{
			recs: []remediation.Recommendation{
				{ID: 1, RuleID: "r1", InstanceID: "inst-a", Title: "Rec A"},
				{ID: 2, RuleID: "r2", InstanceID: "inst-b", Title: "Rec B"},
			},
			total: 2,
		}
		srv := newRemediationTestServer(t, store, nil)
		ts := httptest.NewServer(srv.Routes())
		defer ts.Close()

		resp, err := http.Get(ts.URL + "/api/v1/recommendations")
		if err != nil {
			t.Fatalf("GET: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
		}

		var env Envelope
		if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
			t.Fatalf("decode: %v", err)
		}
		meta := env.Meta.(map[string]interface{})
		if int(meta["count"].(float64)) != 2 {
			t.Errorf("count = %v, want 2", meta["count"])
		}
	})

	t.Run("filters forwarded", func(t *testing.T) {
		t.Parallel()
		store := &mockRemediationStore{recs: nil, total: 0}
		srv := newRemediationTestServer(t, store, nil)
		ts := httptest.NewServer(srv.Routes())
		defer ts.Close()

		resp, err := http.Get(ts.URL + "/api/v1/recommendations?priority=action_required&acknowledged=false")
		if err != nil {
			t.Fatalf("GET: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
		}
		if store.lastOpts.Priority != "action_required" {
			t.Errorf("priority = %q, want %q", store.lastOpts.Priority, "action_required")
		}
		if store.lastOpts.Acknowledged == nil || *store.lastOpts.Acknowledged != false {
			t.Errorf("acknowledged filter not applied correctly")
		}
	})
}

// --- PUT /api/v1/recommendations/{id}/acknowledge ---

func TestHandleAcknowledgeRecommendation(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		store := &mockRemediationStore{}
		srv := newRemediationTestServer(t, store, nil)
		ts := httptest.NewServer(srv.Routes())
		defer ts.Close()

		req, _ := http.NewRequest(http.MethodPut, ts.URL+"/api/v1/recommendations/42/acknowledge", nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("PUT: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("status = %d, want %d; body: %s", resp.StatusCode, http.StatusOK, b)
		}

		if !store.ackCalled {
			t.Error("Acknowledge was not called on the store")
		}

		var env Envelope
		if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
			t.Fatalf("decode: %v", err)
		}
		data := env.Data.(map[string]interface{})
		if data["acknowledged"] != true {
			t.Errorf("acknowledged = %v, want true", data["acknowledged"])
		}
	})

	t.Run("not found returns 404", func(t *testing.T) {
		t.Parallel()
		store := &mockRemediationStore{ackErr: fmt.Errorf("recommendation 999 not found")}
		srv := newRemediationTestServer(t, store, nil)
		ts := httptest.NewServer(srv.Routes())
		defer ts.Close()

		req, _ := http.NewRequest(http.MethodPut, ts.URL+"/api/v1/recommendations/999/acknowledge", nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("PUT: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
		}
	})

	t.Run("invalid id returns 400", func(t *testing.T) {
		t.Parallel()
		store := &mockRemediationStore{}
		srv := newRemediationTestServer(t, store, nil)
		ts := httptest.NewServer(srv.Routes())
		defer ts.Close()

		req, _ := http.NewRequest(http.MethodPut, ts.URL+"/api/v1/recommendations/abc/acknowledge", nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("PUT: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
		}

		var errResp ErrorResponse
		if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if errResp.Error.Code != "INVALID_ID" {
			t.Errorf("code = %q, want %q", errResp.Error.Code, "INVALID_ID")
		}
	})
}

// --- GET /api/v1/recommendations/rules ---

func TestHandleListRemediationRules(t *testing.T) {
	t.Parallel()

	store := &mockRemediationStore{}
	srv := newRemediationTestServer(t, store, nil)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/recommendations/rules")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var env Envelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode: %v", err)
	}

	meta, ok := env.Meta.(map[string]interface{})
	if !ok {
		t.Fatal("meta is not a map")
	}
	count := int(meta["count"].(float64))
	if count == 0 {
		t.Error("expected at least 1 rule, got 0")
	}

	// Verify structure of first rule.
	data, ok := env.Data.([]interface{})
	if !ok {
		t.Fatal("data is not an array")
	}
	rule := data[0].(map[string]interface{})
	if _, ok := rule["id"]; !ok {
		t.Error("rule missing 'id' field")
	}
	if _, ok := rule["priority"]; !ok {
		t.Error("rule missing 'priority' field")
	}
	if _, ok := rule["category"]; !ok {
		t.Error("rule missing 'category' field")
	}
}
