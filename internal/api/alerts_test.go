package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ios9000/PGPulse_01/internal/alert"
	"github.com/ios9000/PGPulse_01/internal/config"
)

// --- Mock implementations for alert tests ---

type mockAlertRuleStore struct {
	rules     []alert.Rule
	getErr    error
	createErr error
}

func (m *mockAlertRuleStore) List(_ context.Context) ([]alert.Rule, error) {
	return m.rules, nil
}

func (m *mockAlertRuleStore) ListEnabled(_ context.Context) ([]alert.Rule, error) {
	var enabled []alert.Rule
	for _, r := range m.rules {
		if r.Enabled {
			enabled = append(enabled, r)
		}
	}
	return enabled, nil
}

func (m *mockAlertRuleStore) Get(_ context.Context, id string) (*alert.Rule, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	for i := range m.rules {
		if m.rules[i].ID == id {
			return &m.rules[i], nil
		}
	}
	return nil, alert.ErrRuleNotFound
}

func (m *mockAlertRuleStore) Create(_ context.Context, rule *alert.Rule) error {
	if m.createErr != nil {
		return m.createErr
	}
	m.rules = append(m.rules, *rule)
	return nil
}

func (m *mockAlertRuleStore) Update(_ context.Context, rule *alert.Rule) error {
	for i := range m.rules {
		if m.rules[i].ID == rule.ID {
			m.rules[i] = *rule
			return nil
		}
	}
	return alert.ErrRuleNotFound
}

func (m *mockAlertRuleStore) Delete(_ context.Context, id string) error {
	for i := range m.rules {
		if m.rules[i].ID == id {
			m.rules = append(m.rules[:i], m.rules[i+1:]...)
			return nil
		}
	}
	return nil
}

func (m *mockAlertRuleStore) UpsertBuiltin(_ context.Context, _ *alert.Rule) error {
	return nil
}

type mockAlertHistoryStore struct {
	events    []alert.AlertEvent
	queryErr  error
	cleanupN  int64
	lastQuery alert.AlertHistoryQuery
}

func (m *mockAlertHistoryStore) Record(_ context.Context, event *alert.AlertEvent) error {
	m.events = append(m.events, *event)
	return nil
}

func (m *mockAlertHistoryStore) Resolve(_ context.Context, _, _ string, _ time.Time) error {
	return nil
}

func (m *mockAlertHistoryStore) ListUnresolved(_ context.Context) ([]alert.AlertEvent, error) {
	var unresolved []alert.AlertEvent
	for _, ev := range m.events {
		if ev.ResolvedAt == nil {
			unresolved = append(unresolved, ev)
		}
	}
	if unresolved == nil {
		unresolved = []alert.AlertEvent{}
	}
	return unresolved, nil
}

func (m *mockAlertHistoryStore) Query(_ context.Context, q alert.AlertHistoryQuery) ([]alert.AlertEvent, error) {
	m.lastQuery = q
	if m.queryErr != nil {
		return nil, m.queryErr
	}
	result := m.events
	if result == nil {
		result = []alert.AlertEvent{}
	}
	return result, nil
}

func (m *mockAlertHistoryStore) Cleanup(_ context.Context, _ time.Duration) (int64, error) {
	return m.cleanupN, nil
}

type mockNotifierForAPI struct {
	name    string
	sendErr error
	called  int
}

func (m *mockNotifierForAPI) Name() string { return m.name }
func (m *mockNotifierForAPI) Send(_ context.Context, _ alert.AlertEvent) error {
	m.called++
	return m.sendErr
}

// newAlertTestServer builds an APIServer with alert components wired, auth disabled.
func newAlertTestServer(t *testing.T, ruleStore alert.AlertRuleStore, historyStore alert.AlertHistoryStore, registry *alert.NotifierRegistry) *APIServer {
	t.Helper()
	cfg := config.Config{
		Server:   config.ServerConfig{CORSEnabled: false},
		Auth:     config.AuthConfig{Enabled: false},
		Alerting: config.AlertingConfig{DefaultConsecutiveCount: 3, DefaultCooldownMinutes: 15},
		Instances: []config.InstanceConfig{
			{ID: "test", DSN: "postgres://localhost/test"},
		},
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return New(cfg, &mockStore{}, nil, nil, nil, logger, ruleStore, historyStore, nil, registry)
}

// --- GET /api/v1/alerts ---

func TestGetActiveAlerts(t *testing.T) {
	now := time.Now()
	hs := &mockAlertHistoryStore{
		events: []alert.AlertEvent{
			{RuleID: "rule-1", InstanceID: "inst-1", Severity: alert.SeverityWarning, FiredAt: now},
			{RuleID: "rule-2", InstanceID: "inst-1", Severity: alert.SeverityCritical, FiredAt: now},
		},
	}
	srv := newAlertTestServer(t, &mockAlertRuleStore{}, hs, nil)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/alerts")
	if err != nil {
		t.Fatalf("GET /alerts: %v", err)
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
	count, ok := meta["count"].(float64)
	if !ok {
		t.Fatal("count is not a number")
	}
	if int(count) != 2 {
		t.Errorf("count = %v, want 2", count)
	}
}

func TestGetActiveAlerts_Empty(t *testing.T) {
	hs := &mockAlertHistoryStore{}
	srv := newAlertTestServer(t, &mockAlertRuleStore{}, hs, nil)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/alerts")
	if err != nil {
		t.Fatalf("GET /alerts: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	body, _ := io.ReadAll(resp.Body)
	// Verify data is [] not null
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if string(raw["data"]) != "[]" {
		t.Errorf("data = %s, want []", string(raw["data"]))
	}
}

// --- GET /api/v1/alerts/history ---

func TestGetAlertHistory(t *testing.T) {
	now := time.Now()
	hs := &mockAlertHistoryStore{
		events: []alert.AlertEvent{
			{RuleID: "rule-1", InstanceID: "inst-1", Severity: alert.SeverityWarning, FiredAt: now},
		},
	}
	srv := newAlertTestServer(t, &mockAlertRuleStore{}, hs, nil)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/alerts/history")
	if err != nil {
		t.Fatalf("GET /alerts/history: %v", err)
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
	count := meta["count"].(float64)
	if int(count) != 1 {
		t.Errorf("count = %v, want 1", count)
	}
}

func TestGetAlertHistory_InvalidStartTime(t *testing.T) {
	hs := &mockAlertHistoryStore{}
	srv := newAlertTestServer(t, &mockAlertRuleStore{}, hs, nil)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/alerts/history?start=bad")
	if err != nil {
		t.Fatalf("GET /alerts/history: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}

	var errResp ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if errResp.Error.Code != "INVALID_TIME" {
		t.Errorf("code = %q, want %q", errResp.Error.Code, "INVALID_TIME")
	}
}

func TestGetAlertHistory_LimitCapped(t *testing.T) {
	hs := &mockAlertHistoryStore{}
	srv := newAlertTestServer(t, &mockAlertRuleStore{}, hs, nil)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/alerts/history?limit=5000")
	if err != nil {
		t.Fatalf("GET /alerts/history: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	if hs.lastQuery.Limit != 1000 {
		t.Errorf("limit = %d, want 1000 (capped)", hs.lastQuery.Limit)
	}
}

// --- GET /api/v1/alerts/rules ---

func TestGetAlertRules(t *testing.T) {
	rs := &mockAlertRuleStore{
		rules: []alert.Rule{
			{ID: "rule-1", Name: "Rule 1", Metric: "pgpulse.test", Operator: alert.OpGreater, Threshold: 80, Severity: alert.SeverityWarning, Source: alert.SourceBuiltin, Enabled: true},
			{ID: "rule-2", Name: "Rule 2", Metric: "pgpulse.test2", Operator: alert.OpLess, Threshold: 0.9, Severity: alert.SeverityCritical, Source: alert.SourceCustom, Enabled: true},
		},
	}
	srv := newAlertTestServer(t, rs, &mockAlertHistoryStore{}, nil)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/alerts/rules")
	if err != nil {
		t.Fatalf("GET /alerts/rules: %v", err)
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
	count := meta["count"].(float64)
	if int(count) != 2 {
		t.Errorf("count = %v, want 2", count)
	}
}

// --- POST /api/v1/alerts/rules ---

func TestCreateAlertRule_Valid(t *testing.T) {
	rs := &mockAlertRuleStore{}
	srv := newAlertTestServer(t, rs, &mockAlertHistoryStore{}, nil)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	body := `{
		"id": "my-rule",
		"name": "My Rule",
		"metric": "pgpulse.test.metric",
		"operator": ">",
		"threshold": 80,
		"severity": "warning",
		"consecutive_count": 5,
		"cooldown_minutes": 30
	}`

	resp, err := http.Post(ts.URL+"/api/v1/alerts/rules", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("POST /alerts/rules: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want %d; body: %s", resp.StatusCode, http.StatusCreated, b)
	}

	var env Envelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode: %v", err)
	}

	data, ok := env.Data.(map[string]interface{})
	if !ok {
		t.Fatal("data is not a map")
	}
	if data["source"] != "custom" {
		t.Errorf("source = %v, want %q", data["source"], "custom")
	}
	if data["id"] != "my-rule" {
		t.Errorf("id = %v, want %q", data["id"], "my-rule")
	}
}

func TestCreateAlertRule_Defaults(t *testing.T) {
	rs := &mockAlertRuleStore{}
	srv := newAlertTestServer(t, rs, &mockAlertHistoryStore{}, nil)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	body := `{
		"id": "my-rule",
		"name": "My Rule",
		"metric": "pgpulse.test.metric",
		"operator": ">",
		"threshold": 80,
		"severity": "warning"
	}`

	resp, err := http.Post(ts.URL+"/api/v1/alerts/rules", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("POST /alerts/rules: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want %d; body: %s", resp.StatusCode, http.StatusCreated, b)
	}

	var env Envelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode: %v", err)
	}

	data, ok := env.Data.(map[string]interface{})
	if !ok {
		t.Fatal("data is not a map")
	}

	// Defaults come from alertingCfg: ConsecutiveCount=3, CooldownMinutes=15
	cc := data["consecutive_count"].(float64)
	if int(cc) != 3 {
		t.Errorf("consecutive_count = %v, want 3 (default)", cc)
	}
	cd := data["cooldown_minutes"].(float64)
	if int(cd) != 15 {
		t.Errorf("cooldown_minutes = %v, want 15 (default)", cd)
	}
}

func TestCreateAlertRule_InvalidOperator(t *testing.T) {
	rs := &mockAlertRuleStore{}
	srv := newAlertTestServer(t, rs, &mockAlertHistoryStore{}, nil)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	body := `{
		"id": "my-rule",
		"name": "My Rule",
		"metric": "pgpulse.test.metric",
		"operator": ">>>",
		"threshold": 80,
		"severity": "warning"
	}`

	resp, err := http.Post(ts.URL+"/api/v1/alerts/rules", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("POST /alerts/rules: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}

	var errResp ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if errResp.Error.Code != "VALIDATION_ERROR" {
		t.Errorf("code = %q, want %q", errResp.Error.Code, "VALIDATION_ERROR")
	}
}

func TestCreateAlertRule_MissingID(t *testing.T) {
	rs := &mockAlertRuleStore{}
	srv := newAlertTestServer(t, rs, &mockAlertHistoryStore{}, nil)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	body := `{
		"name": "My Rule",
		"metric": "pgpulse.test.metric",
		"operator": ">",
		"threshold": 80,
		"severity": "warning"
	}`

	resp, err := http.Post(ts.URL+"/api/v1/alerts/rules", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("POST /alerts/rules: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}

	var errResp ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if errResp.Error.Code != "VALIDATION_ERROR" {
		t.Errorf("code = %q, want %q", errResp.Error.Code, "VALIDATION_ERROR")
	}
}

func TestCreateAlertRule_MissingMetric(t *testing.T) {
	rs := &mockAlertRuleStore{}
	srv := newAlertTestServer(t, rs, &mockAlertHistoryStore{}, nil)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	body := `{
		"id": "my-rule",
		"name": "My Rule",
		"operator": ">",
		"threshold": 80,
		"severity": "warning"
	}`

	resp, err := http.Post(ts.URL+"/api/v1/alerts/rules", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("POST /alerts/rules: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}

	var errResp ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if errResp.Error.Code != "VALIDATION_ERROR" {
		t.Errorf("code = %q, want %q", errResp.Error.Code, "VALIDATION_ERROR")
	}
}

func TestCreateAlertRule_DuplicateID(t *testing.T) {
	rs := &mockAlertRuleStore{createErr: errors.New("duplicate")}
	srv := newAlertTestServer(t, rs, &mockAlertHistoryStore{}, nil)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	body := `{
		"id": "my-rule",
		"name": "My Rule",
		"metric": "pgpulse.test.metric",
		"operator": ">",
		"threshold": 80,
		"severity": "warning"
	}`

	resp, err := http.Post(ts.URL+"/api/v1/alerts/rules", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("POST /alerts/rules: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusConflict)
	}

	var errResp ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if errResp.Error.Code != "DUPLICATE_ID" {
		t.Errorf("code = %q, want %q", errResp.Error.Code, "DUPLICATE_ID")
	}
}

// --- PUT /api/v1/alerts/rules/{id} ---

func TestUpdateAlertRule_Success(t *testing.T) {
	rs := &mockAlertRuleStore{
		rules: []alert.Rule{
			{ID: "my-rule", Name: "Old Name", Metric: "pgpulse.test", Operator: alert.OpGreater, Threshold: 80, Severity: alert.SeverityWarning, Source: alert.SourceCustom, Enabled: true},
		},
	}
	srv := newAlertTestServer(t, rs, &mockAlertHistoryStore{}, nil)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	body := `{"threshold": 95}`
	req, err := http.NewRequest(http.MethodPut, ts.URL+"/api/v1/alerts/rules/my-rule", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT /alerts/rules/my-rule: %v", err)
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
	if data["threshold"].(float64) != 95 {
		t.Errorf("threshold = %v, want 95", data["threshold"])
	}
}

func TestUpdateAlertRule_NotFound(t *testing.T) {
	rs := &mockAlertRuleStore{}
	srv := newAlertTestServer(t, rs, &mockAlertHistoryStore{}, nil)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	body := `{"threshold": 95}`
	req, err := http.NewRequest(http.MethodPut, ts.URL+"/api/v1/alerts/rules/unknown", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT /alerts/rules/unknown: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}

	var errResp ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if errResp.Error.Code != "NOT_FOUND" {
		t.Errorf("code = %q, want %q", errResp.Error.Code, "NOT_FOUND")
	}
}

func TestUpdateAlertRule_BuiltinLimited(t *testing.T) {
	rs := &mockAlertRuleStore{
		rules: []alert.Rule{
			{
				ID: "builtin-rule", Name: "Builtin Rule", Metric: "pgpulse.test",
				Operator: alert.OpGreater, Threshold: 80, Severity: alert.SeverityWarning,
				Source: alert.SourceBuiltin, Enabled: true,
			},
		},
	}
	srv := newAlertTestServer(t, rs, &mockAlertHistoryStore{}, nil)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	// Try to change metric and threshold — only threshold should change for builtin
	body := `{"metric": "pgpulse.hacked", "threshold": 99}`
	req, err := http.NewRequest(http.MethodPut, ts.URL+"/api/v1/alerts/rules/builtin-rule", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
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
	// Threshold should be updated
	if data["threshold"].(float64) != 99 {
		t.Errorf("threshold = %v, want 99", data["threshold"])
	}
	// Metric should NOT be changed for builtin rules
	if data["metric"] != "pgpulse.test" {
		t.Errorf("metric = %v, want %q (builtin metric should not change)", data["metric"], "pgpulse.test")
	}
}

// --- DELETE /api/v1/alerts/rules/{id} ---

func TestDeleteAlertRule_Custom(t *testing.T) {
	rs := &mockAlertRuleStore{
		rules: []alert.Rule{
			{ID: "custom-rule", Name: "Custom", Source: alert.SourceCustom, Enabled: true},
		},
	}
	srv := newAlertTestServer(t, rs, &mockAlertHistoryStore{}, nil)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	req, err := http.NewRequest(http.MethodDelete, ts.URL+"/api/v1/alerts/rules/custom-rule", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNoContent {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want %d; body: %s", resp.StatusCode, http.StatusNoContent, b)
	}
}

func TestDeleteAlertRule_Builtin(t *testing.T) {
	rs := &mockAlertRuleStore{
		rules: []alert.Rule{
			{ID: "builtin-rule", Name: "Builtin", Source: alert.SourceBuiltin, Enabled: true},
		},
	}
	srv := newAlertTestServer(t, rs, &mockAlertHistoryStore{}, nil)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	req, err := http.NewRequest(http.MethodDelete, ts.URL+"/api/v1/alerts/rules/builtin-rule", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusConflict)
	}

	var errResp ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if errResp.Error.Code != "BUILTIN_RULE" {
		t.Errorf("code = %q, want %q", errResp.Error.Code, "BUILTIN_RULE")
	}
}

func TestDeleteAlertRule_NotFound(t *testing.T) {
	rs := &mockAlertRuleStore{}
	srv := newAlertTestServer(t, rs, &mockAlertHistoryStore{}, nil)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	req, err := http.NewRequest(http.MethodDelete, ts.URL+"/api/v1/alerts/rules/unknown", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}

	var errResp ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if errResp.Error.Code != "NOT_FOUND" {
		t.Errorf("code = %q, want %q", errResp.Error.Code, "NOT_FOUND")
	}
}

// --- POST /api/v1/alerts/test ---

func TestTestNotification_Success(t *testing.T) {
	notifier := &mockNotifierForAPI{name: "telegram"}
	registry := alert.NewNotifierRegistry()
	registry.Register(notifier)

	srv := newAlertTestServer(t, &mockAlertRuleStore{}, &mockAlertHistoryStore{}, registry)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	body := `{"channel": "telegram", "message": "test"}`
	resp, err := http.Post(ts.URL+"/api/v1/alerts/test", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("POST /alerts/test: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want %d; body: %s", resp.StatusCode, http.StatusOK, b)
	}

	if notifier.called != 1 {
		t.Errorf("notifier.called = %d, want 1", notifier.called)
	}
}

func TestTestNotification_UnknownChannel(t *testing.T) {
	registry := alert.NewNotifierRegistry()
	srv := newAlertTestServer(t, &mockAlertRuleStore{}, &mockAlertHistoryStore{}, registry)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	body := `{"channel": "sms"}`
	resp, err := http.Post(ts.URL+"/api/v1/alerts/test", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("POST /alerts/test: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}

	var errResp ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if errResp.Error.Code != "UNKNOWN_CHANNEL" {
		t.Errorf("code = %q, want %q", errResp.Error.Code, "UNKNOWN_CHANNEL")
	}
}

func TestTestNotification_MissingChannel(t *testing.T) {
	registry := alert.NewNotifierRegistry()
	srv := newAlertTestServer(t, &mockAlertRuleStore{}, &mockAlertHistoryStore{}, registry)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	body := `{"channel": ""}`
	resp, err := http.Post(ts.URL+"/api/v1/alerts/test", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("POST /alerts/test: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}

	var errResp ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if errResp.Error.Code != "VALIDATION_ERROR" {
		t.Errorf("code = %q, want %q", errResp.Error.Code, "VALIDATION_ERROR")
	}
}
