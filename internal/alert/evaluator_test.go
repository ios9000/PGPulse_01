package alert

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/ios9000/PGPulse_01/internal/collector"
)

// --- Mock implementations ---

type mockRuleStore struct {
	rules []Rule
}

func (m *mockRuleStore) List(_ context.Context) ([]Rule, error) {
	return m.rules, nil
}

func (m *mockRuleStore) ListEnabled(_ context.Context) ([]Rule, error) {
	var enabled []Rule
	for _, r := range m.rules {
		if r.Enabled {
			enabled = append(enabled, r)
		}
	}
	return enabled, nil
}

func (m *mockRuleStore) Get(_ context.Context, id string) (*Rule, error) {
	for i := range m.rules {
		if m.rules[i].ID == id {
			return &m.rules[i], nil
		}
	}
	return nil, ErrRuleNotFound
}

func (m *mockRuleStore) Create(_ context.Context, rule *Rule) error {
	m.rules = append(m.rules, *rule)
	return nil
}

func (m *mockRuleStore) Update(_ context.Context, rule *Rule) error {
	for i := range m.rules {
		if m.rules[i].ID == rule.ID {
			m.rules[i] = *rule
			return nil
		}
	}
	return ErrRuleNotFound
}

func (m *mockRuleStore) Delete(_ context.Context, id string) error {
	for i := range m.rules {
		if m.rules[i].ID == id {
			m.rules = append(m.rules[:i], m.rules[i+1:]...)
			return nil
		}
	}
	return nil
}

func (m *mockRuleStore) UpsertBuiltin(_ context.Context, rule *Rule) error {
	for i := range m.rules {
		if m.rules[i].ID == rule.ID {
			// Preserve user-modifiable fields, update metadata only
			m.rules[i].Name = rule.Name
			m.rules[i].Description = rule.Description
			m.rules[i].Metric = rule.Metric
			m.rules[i].Operator = rule.Operator
			m.rules[i].Severity = rule.Severity
			m.rules[i].Labels = rule.Labels
			return nil
		}
	}
	m.rules = append(m.rules, *rule)
	return nil
}

type resolveRecord struct {
	ruleID     string
	instanceID string
	at         time.Time
}

type mockHistoryStore struct {
	events   []AlertEvent
	resolved []resolveRecord
}

func (m *mockHistoryStore) Record(_ context.Context, event *AlertEvent) error {
	m.events = append(m.events, *event)
	return nil
}

func (m *mockHistoryStore) Resolve(_ context.Context, ruleID, instanceID string, resolvedAt time.Time) error {
	m.resolved = append(m.resolved, resolveRecord{ruleID, instanceID, resolvedAt})
	return nil
}

func (m *mockHistoryStore) ListUnresolved(_ context.Context) ([]AlertEvent, error) {
	var unresolved []AlertEvent
	for _, ev := range m.events {
		if ev.ResolvedAt == nil {
			unresolved = append(unresolved, ev)
		}
	}
	return unresolved, nil
}

func (m *mockHistoryStore) Query(_ context.Context, _ AlertHistoryQuery) ([]AlertEvent, error) {
	return m.events, nil
}

func (m *mockHistoryStore) Cleanup(_ context.Context, _ time.Duration) (int64, error) {
	return 0, nil
}

// --- Helpers ---

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func makeRule(id, metric string, op Operator, threshold float64, severity Severity, consecutiveCount int) Rule {
	return Rule{
		ID:               id,
		Name:             id,
		Metric:           metric,
		Operator:         op,
		Threshold:        threshold,
		Severity:         severity,
		ConsecutiveCount: consecutiveCount,
		CooldownMinutes:  15,
		Source:           SourceBuiltin,
		Enabled:          true,
	}
}

func makePoint(instanceID, metric string, value float64, labels map[string]string) collector.MetricPoint {
	return collector.MetricPoint{
		InstanceID: instanceID,
		Metric:     metric,
		Value:      value,
		Labels:     labels,
		Timestamp:  time.Now(),
	}
}

// --- Tests ---

func TestEvaluate_OKToFiring(t *testing.T) {
	rule := makeRule("test_rule", "pg.test.metric", OpGreater, 80, SeverityWarning, 3)
	rs := &mockRuleStore{rules: []Rule{rule}}
	hs := &mockHistoryStore{}
	ev := NewEvaluator(rs, hs, discardLogger())

	if err := ev.LoadRules(context.Background()); err != nil {
		t.Fatalf("LoadRules: %v", err)
	}

	pt := makePoint("inst-1", "pg.test.metric", 90, nil)

	// Call 1: breach, state=Pending, count=1 — no event
	events, err := ev.Evaluate(context.Background(), []collector.MetricPoint{pt})
	if err != nil {
		t.Fatalf("Evaluate call 1: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("call 1: expected 0 events, got %d", len(events))
	}

	// Call 2: breach, count=2 — no event
	events, err = ev.Evaluate(context.Background(), []collector.MetricPoint{pt})
	if err != nil {
		t.Fatalf("Evaluate call 2: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("call 2: expected 0 events, got %d", len(events))
	}

	// Call 3: breach, count=3 — FIRE
	events, err = ev.Evaluate(context.Background(), []collector.MetricPoint{pt})
	if err != nil {
		t.Fatalf("Evaluate call 3: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("call 3: expected 1 event, got %d", len(events))
	}

	e := events[0]
	if e.RuleID != "test_rule" {
		t.Errorf("RuleID = %q, want %q", e.RuleID, "test_rule")
	}
	if e.Severity != SeverityWarning {
		t.Errorf("Severity = %q, want %q", e.Severity, SeverityWarning)
	}
	if e.Value != 90 {
		t.Errorf("Value = %v, want 90", e.Value)
	}
	if e.Threshold != 80 {
		t.Errorf("Threshold = %v, want 80", e.Threshold)
	}
	if e.IsResolution {
		t.Error("IsResolution should be false for fire event")
	}
}

func TestEvaluate_PendingResetOnOK(t *testing.T) {
	rule := makeRule("test_rule", "pg.test.metric", OpGreater, 80, SeverityWarning, 3)
	rs := &mockRuleStore{rules: []Rule{rule}}
	hs := &mockHistoryStore{}
	ev := NewEvaluator(rs, hs, discardLogger())

	if err := ev.LoadRules(context.Background()); err != nil {
		t.Fatalf("LoadRules: %v", err)
	}

	breach := makePoint("inst-1", "pg.test.metric", 90, nil)
	ok := makePoint("inst-1", "pg.test.metric", 50, nil)

	// Call 1: breach (count=1)
	events, _ := ev.Evaluate(context.Background(), []collector.MetricPoint{breach})
	if len(events) != 0 {
		t.Errorf("call 1: expected 0 events, got %d", len(events))
	}

	// Call 2: breach (count=2)
	events, _ = ev.Evaluate(context.Background(), []collector.MetricPoint{breach})
	if len(events) != 0 {
		t.Errorf("call 2: expected 0 events, got %d", len(events))
	}

	// Call 3: OK value — resets count
	events, _ = ev.Evaluate(context.Background(), []collector.MetricPoint{ok})
	if len(events) != 0 {
		t.Errorf("call 3: expected 0 events, got %d", len(events))
	}

	// Call 4: breach again — back to count=1, no fire
	events, _ = ev.Evaluate(context.Background(), []collector.MetricPoint{breach})
	if len(events) != 0 {
		t.Errorf("call 4: expected 0 events, got %d", len(events))
	}
}

func TestEvaluate_FiringToOK(t *testing.T) {
	rule := makeRule("test_rule", "pg.test.metric", OpGreater, 80, SeverityWarning, 1)
	rs := &mockRuleStore{rules: []Rule{rule}}
	hs := &mockHistoryStore{}
	ev := NewEvaluator(rs, hs, discardLogger())

	if err := ev.LoadRules(context.Background()); err != nil {
		t.Fatalf("LoadRules: %v", err)
	}

	breach := makePoint("inst-1", "pg.test.metric", 90, nil)
	ok := makePoint("inst-1", "pg.test.metric", 50, nil)

	// Fire the alert (consecutive_count=1)
	events, err := ev.Evaluate(context.Background(), []collector.MetricPoint{breach})
	if err != nil {
		t.Fatalf("Evaluate fire: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 fire event, got %d", len(events))
	}
	if events[0].IsResolution {
		t.Error("first event should not be a resolution")
	}

	// Resolve with OK value
	events, err = ev.Evaluate(context.Background(), []collector.MetricPoint{ok})
	if err != nil {
		t.Fatalf("Evaluate resolve: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 resolution event, got %d", len(events))
	}

	e := events[0]
	if !e.IsResolution {
		t.Error("expected IsResolution=true")
	}
	if e.ResolvedAt == nil {
		t.Error("ResolvedAt should not be nil")
	}
}

func TestEvaluate_Hysteresis_ExactThreshold(t *testing.T) {
	rule := makeRule("test_rule", "pg.test.metric", OpGreater, 80, SeverityWarning, 3)
	rs := &mockRuleStore{rules: []Rule{rule}}
	hs := &mockHistoryStore{}
	ev := NewEvaluator(rs, hs, discardLogger())

	if err := ev.LoadRules(context.Background()); err != nil {
		t.Fatalf("LoadRules: %v", err)
	}

	pt := makePoint("inst-1", "pg.test.metric", 90, nil)

	// Calls 1 and 2 should NOT fire
	for i := 1; i <= 2; i++ {
		events, _ := ev.Evaluate(context.Background(), []collector.MetricPoint{pt})
		if len(events) != 0 {
			t.Errorf("call %d: expected 0 events, got %d", i, len(events))
		}
	}

	// Call 3 should fire (exactly at consecutive_count=3)
	events, _ := ev.Evaluate(context.Background(), []collector.MetricPoint{pt})
	if len(events) != 1 {
		t.Fatalf("call 3: expected 1 event, got %d", len(events))
	}
}

func TestEvaluate_ConsecutiveCountOne(t *testing.T) {
	rule := makeRule("test_rule", "pg.test.metric", OpGreater, 80, SeverityWarning, 1)
	rs := &mockRuleStore{rules: []Rule{rule}}
	hs := &mockHistoryStore{}
	ev := NewEvaluator(rs, hs, discardLogger())

	if err := ev.LoadRules(context.Background()); err != nil {
		t.Fatalf("LoadRules: %v", err)
	}

	pt := makePoint("inst-1", "pg.test.metric", 90, nil)

	// First breach — immediate fire
	events, err := ev.Evaluate(context.Background(), []collector.MetricPoint{pt})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].IsResolution {
		t.Error("expected fire event, not resolution")
	}
}

func TestEvaluate_NoMatchingMetrics(t *testing.T) {
	rule := makeRule("test_rule", "pg.cache.hit_ratio", OpLess, 0.9, SeverityWarning, 1)
	rs := &mockRuleStore{rules: []Rule{rule}}
	hs := &mockHistoryStore{}
	ev := NewEvaluator(rs, hs, discardLogger())

	if err := ev.LoadRules(context.Background()); err != nil {
		t.Fatalf("LoadRules: %v", err)
	}

	// Different metric name — should not match
	pt := makePoint("inst-1", "pg.connections.total", 100, nil)

	events, err := ev.Evaluate(context.Background(), []collector.MetricPoint{pt})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("expected 0 events for non-matching metric, got %d", len(events))
	}
}

func TestEvaluate_LabelFiltering(t *testing.T) {
	rule := makeRule("slot_rule", "pg.replication.slot_active", OpEqual, 0, SeverityWarning, 1)
	rule.Labels = map[string]string{"slot_name": "my_slot"}

	rs := &mockRuleStore{rules: []Rule{rule}}
	hs := &mockHistoryStore{}
	ev := NewEvaluator(rs, hs, discardLogger())

	if err := ev.LoadRules(context.Background()); err != nil {
		t.Fatalf("LoadRules: %v", err)
	}

	matchingPt := makePoint("inst-1", "pg.replication.slot_active", 0,
		map[string]string{"slot_name": "my_slot", "extra": "val"})
	nonMatchingPt := makePoint("inst-1", "pg.replication.slot_active", 0,
		map[string]string{"slot_name": "other_slot"})

	events, err := ev.Evaluate(context.Background(), []collector.MetricPoint{matchingPt, nonMatchingPt})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	// Only the matching point should trigger
	if len(events) != 1 {
		t.Fatalf("expected 1 event (only matching labels), got %d", len(events))
	}
	if events[0].InstanceID != "inst-1" {
		t.Errorf("InstanceID = %q, want %q", events[0].InstanceID, "inst-1")
	}
}

func TestEvaluate_MultipleRules(t *testing.T) {
	warning := makeRule("conn_warn", "pg.connections.utilization_pct", OpGreater, 80, SeverityWarning, 1)
	critical := makeRule("conn_crit", "pg.connections.utilization_pct", OpGreaterEqual, 99, SeverityCritical, 1)

	rs := &mockRuleStore{rules: []Rule{warning, critical}}
	hs := &mockHistoryStore{}
	ev := NewEvaluator(rs, hs, discardLogger())

	if err := ev.LoadRules(context.Background()); err != nil {
		t.Fatalf("LoadRules: %v", err)
	}

	pt := makePoint("inst-1", "pg.connections.utilization_pct", 100, nil)

	events, err := ev.Evaluate(context.Background(), []collector.MetricPoint{pt})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events (warning + critical), got %d", len(events))
	}

	severities := map[Severity]bool{}
	for _, e := range events {
		severities[e.Severity] = true
	}
	if !severities[SeverityWarning] {
		t.Error("expected a warning event")
	}
	if !severities[SeverityCritical] {
		t.Error("expected a critical event")
	}
}

func TestEvaluate_ResolutionAlwaysEmits(t *testing.T) {
	rule := makeRule("test_rule", "pg.test.metric", OpGreater, 80, SeverityWarning, 1)
	rs := &mockRuleStore{rules: []Rule{rule}}
	hs := &mockHistoryStore{}
	ev := NewEvaluator(rs, hs, discardLogger())

	if err := ev.LoadRules(context.Background()); err != nil {
		t.Fatalf("LoadRules: %v", err)
	}

	breach := makePoint("inst-1", "pg.test.metric", 90, nil)
	ok := makePoint("inst-1", "pg.test.metric", 50, nil)

	// Fire
	events, _ := ev.Evaluate(context.Background(), []collector.MetricPoint{breach})
	if len(events) != 1 {
		t.Fatalf("expected fire event, got %d events", len(events))
	}

	// Resolve
	events, _ = ev.Evaluate(context.Background(), []collector.MetricPoint{ok})
	if len(events) != 1 {
		t.Fatalf("expected resolution event, got %d events", len(events))
	}
	if !events[0].IsResolution {
		t.Error("expected IsResolution=true")
	}

	// Verify history store recorded the resolve
	if len(hs.resolved) != 1 {
		t.Fatalf("expected 1 resolve record, got %d", len(hs.resolved))
	}
	if hs.resolved[0].ruleID != "test_rule" {
		t.Errorf("resolved ruleID = %q, want %q", hs.resolved[0].ruleID, "test_rule")
	}
}

func TestEvaluate_EmptyPoints(t *testing.T) {
	rule := makeRule("test_rule", "pg.test.metric", OpGreater, 80, SeverityWarning, 1)
	rs := &mockRuleStore{rules: []Rule{rule}}
	hs := &mockHistoryStore{}
	ev := NewEvaluator(rs, hs, discardLogger())

	if err := ev.LoadRules(context.Background()); err != nil {
		t.Fatalf("LoadRules: %v", err)
	}

	events, err := ev.Evaluate(context.Background(), nil)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if events != nil {
		t.Errorf("expected nil events for empty points, got %d", len(events))
	}
}

func TestEvaluate_NoRulesLoaded(t *testing.T) {
	rs := &mockRuleStore{}
	hs := &mockHistoryStore{}
	ev := NewEvaluator(rs, hs, discardLogger())

	// Don't call LoadRules — no rules
	pt := makePoint("inst-1", "pg.test.metric", 90, nil)

	events, err := ev.Evaluate(context.Background(), []collector.MetricPoint{pt})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if events != nil {
		t.Errorf("expected nil events with no rules, got %d", len(events))
	}
}

func TestEvaluate_FiringStaysWhileBreached(t *testing.T) {
	rule := makeRule("test_rule", "pg.test.metric", OpGreater, 80, SeverityWarning, 1)
	rs := &mockRuleStore{rules: []Rule{rule}}
	hs := &mockHistoryStore{}
	ev := NewEvaluator(rs, hs, discardLogger())

	if err := ev.LoadRules(context.Background()); err != nil {
		t.Fatalf("LoadRules: %v", err)
	}

	breach := makePoint("inst-1", "pg.test.metric", 90, nil)

	// Fire
	events, _ := ev.Evaluate(context.Background(), []collector.MetricPoint{breach})
	if len(events) != 1 {
		t.Fatalf("expected 1 fire event, got %d", len(events))
	}

	// Still breached — should produce no events (stays in Firing)
	events, _ = ev.Evaluate(context.Background(), []collector.MetricPoint{breach})
	if len(events) != 0 {
		t.Errorf("expected 0 events while still firing, got %d", len(events))
	}
}

func TestLabelsMatch(t *testing.T) {
	tests := []struct {
		name     string
		required map[string]string
		actual   map[string]string
		want     bool
	}{
		{"empty_required_matches_everything", nil, map[string]string{"a": "1"}, true},
		{"empty_required_matches_empty", nil, nil, true},
		{"subset_match", map[string]string{"a": "1"}, map[string]string{"a": "1", "b": "2"}, true},
		{"exact_match", map[string]string{"a": "1"}, map[string]string{"a": "1"}, true},
		{"value_mismatch", map[string]string{"a": "1"}, map[string]string{"a": "2"}, false},
		{"missing_key", map[string]string{"a": "1"}, map[string]string{}, false},
		{"missing_key_nil", map[string]string{"a": "1"}, nil, false},
		{"multi_required_match", map[string]string{"a": "1", "b": "2"}, map[string]string{"a": "1", "b": "2", "c": "3"}, true},
		{"multi_required_partial_mismatch", map[string]string{"a": "1", "b": "2"}, map[string]string{"a": "1", "b": "99"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := labelsMatch(tt.required, tt.actual)
			if got != tt.want {
				t.Errorf("labelsMatch(%v, %v) = %v, want %v",
					tt.required, tt.actual, got, tt.want)
			}
		})
	}
}

func TestRestoreState(t *testing.T) {
	rule := makeRule("test_rule", "pg.test.metric", OpGreater, 80, SeverityWarning, 1)
	rs := &mockRuleStore{rules: []Rule{rule}}

	// Pre-populate the history store with an unresolved event
	firedAt := time.Now().Add(-10 * time.Minute)
	hs := &mockHistoryStore{
		events: []AlertEvent{
			{
				RuleID:     "test_rule",
				InstanceID: "inst-1",
				Severity:   SeverityWarning,
				Metric:     "pg.test.metric",
				Value:      90,
				Threshold:  80,
				Operator:   OpGreater,
				FiredAt:    firedAt,
				ResolvedAt: nil,
			},
		},
	}

	ev := NewEvaluator(rs, hs, discardLogger())

	ctx := context.Background()
	if err := ev.LoadRules(ctx); err != nil {
		t.Fatalf("LoadRules: %v", err)
	}
	if err := ev.RestoreState(ctx); err != nil {
		t.Fatalf("RestoreState: %v", err)
	}

	// Now feed an OK metric — should generate a resolution event
	// because RestoreState set state to Firing
	ok := makePoint("inst-1", "pg.test.metric", 50, nil)
	events, err := ev.Evaluate(ctx, []collector.MetricPoint{ok})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 resolution event after RestoreState, got %d", len(events))
	}
	if !events[0].IsResolution {
		t.Error("expected IsResolution=true")
	}
}

func TestStateKeyWithLabels(t *testing.T) {
	// No labels
	key1 := stateKeyWithLabels("rule1", "inst1", nil)
	if key1 != "rule1:inst1" {
		t.Errorf("no labels: got %q, want %q", key1, "rule1:inst1")
	}

	// With labels — should be sorted
	key2 := stateKeyWithLabels("rule1", "inst1", map[string]string{"b": "2", "a": "1"})
	expected := "rule1:inst1:a=1:b=2"
	if key2 != expected {
		t.Errorf("with labels: got %q, want %q", key2, expected)
	}

	// Same labels different order should produce same key
	key3 := stateKeyWithLabels("rule1", "inst1", map[string]string{"a": "1", "b": "2"})
	if key2 != key3 {
		t.Errorf("label order should not matter: %q != %q", key2, key3)
	}
}

// --- Cleanup tests ---

type cleanupRecordingStore struct {
	mockHistoryStore
	cleanupCalled    bool
	cleanupRetention time.Duration
}

func (s *cleanupRecordingStore) Cleanup(_ context.Context, olderThan time.Duration) (int64, error) {
	s.cleanupCalled = true
	s.cleanupRetention = olderThan
	return 5, nil
}

func TestEvaluator_RunCleanup(t *testing.T) {
	rs := &mockRuleStore{}
	hs := &cleanupRecordingStore{}
	ev := NewEvaluator(rs, hs, discardLogger())

	retention := 30 * 24 * time.Hour
	ev.runCleanup(context.Background(), retention)

	if !hs.cleanupCalled {
		t.Error("Cleanup was not called")
	}
	if hs.cleanupRetention != retention {
		t.Errorf("Cleanup retention = %v, want %v", hs.cleanupRetention, retention)
	}
}
