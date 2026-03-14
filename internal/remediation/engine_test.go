package remediation

import (
	"context"
	"testing"
)

func TestEvaluateMetric_MatchingRule(t *testing.T) {
	t.Parallel()
	engine := NewEngine()
	snapshot := MetricSnapshot{
		"pg.connections.active":          85,
		"pg.connections.max_connections": 100,
	}
	recs := engine.EvaluateMetric(context.Background(), "inst1", "pg.connections.active", 85, nil, "warning", snapshot)
	if len(recs) == 0 {
		t.Fatal("expected at least one recommendation for 85% connection usage")
	}
	found := false
	for _, r := range recs {
		if r.RuleID == "rem_conn_high" {
			found = true
			if r.InstanceID != "inst1" {
				t.Errorf("expected instance inst1, got %s", r.InstanceID)
			}
			if r.Priority != PrioritySuggestion {
				t.Errorf("expected suggestion priority, got %s", r.Priority)
			}
		}
	}
	if !found {
		t.Error("expected rem_conn_high rule to fire")
	}
}

func TestEvaluateMetric_NoMatch(t *testing.T) {
	t.Parallel()
	engine := NewEngine()
	snapshot := MetricSnapshot{
		"pg.connections.active":          10,
		"pg.connections.max_connections": 100,
	}
	recs := engine.EvaluateMetric(context.Background(), "inst1", "pg.connections.active", 10, nil, "warning", snapshot)
	for _, r := range recs {
		if r.RuleID == "rem_conn_high" || r.RuleID == "rem_conn_exhausted" {
			t.Errorf("unexpected connection rule fired: %s", r.RuleID)
		}
	}
}

func TestEvaluateMetric_MultipleRules(t *testing.T) {
	t.Parallel()
	engine := NewEngine()
	// 99% connections should fire both rem_conn_exhausted (>=99%) but not rem_conn_high (80-99 exclusive)
	snapshot := MetricSnapshot{
		"pg.connections.active":          99,
		"pg.connections.max_connections": 100,
	}
	recs := engine.EvaluateMetric(context.Background(), "inst1", "pg.connections.active", 99, nil, "critical", snapshot)
	foundExhausted := false
	for _, r := range recs {
		if r.RuleID == "rem_conn_exhausted" {
			foundExhausted = true
		}
	}
	if !foundExhausted {
		t.Error("expected rem_conn_exhausted to fire at 99%")
	}
}

func TestDiagnose_AllRules(t *testing.T) {
	t.Parallel()
	engine := NewEngine()
	// Snapshot with multiple issues
	snapshot := MetricSnapshot{
		"pg.connections.active":          85,
		"pg.connections.max_connections": 100,
		"pg.cache.hit_ratio":            80,
		"os.cpu.user_pct":               70,
		"os.cpu.system_pct":             20,
	}
	recs := engine.Diagnose(context.Background(), "inst1", snapshot)
	if len(recs) < 3 {
		t.Errorf("expected at least 3 recommendations, got %d", len(recs))
	}

	ids := make(map[string]bool)
	for _, r := range recs {
		ids[r.RuleID] = true
		if r.InstanceID != "inst1" {
			t.Errorf("expected instance inst1, got %s", r.InstanceID)
		}
	}
	for _, expected := range []string{"rem_conn_high", "rem_cache_low", "rem_cpu_high"} {
		if !ids[expected] {
			t.Errorf("expected rule %s to fire in Diagnose", expected)
		}
	}
}

func TestDiagnose_EmptySnapshot(t *testing.T) {
	t.Parallel()
	engine := NewEngine()
	recs := engine.Diagnose(context.Background(), "inst1", MetricSnapshot{})
	if len(recs) != 0 {
		t.Errorf("expected 0 recommendations for empty snapshot, got %d", len(recs))
	}
}

func TestDiagnose_PartialSnapshot(t *testing.T) {
	t.Parallel()
	engine := NewEngine()
	// Only cache metric, missing connections etc — should not panic
	snapshot := MetricSnapshot{
		"pg.cache.hit_ratio": 50,
	}
	recs := engine.Diagnose(context.Background(), "inst1", snapshot)
	found := false
	for _, r := range recs {
		if r.RuleID == "rem_cache_low" {
			found = true
		}
	}
	if !found {
		t.Error("expected rem_cache_low to fire for hit ratio of 50%")
	}
}

func TestRules_ReturnsAll(t *testing.T) {
	t.Parallel()
	engine := NewEngine()
	rules := engine.Rules()
	if len(rules) != 25 {
		t.Errorf("expected 25 rules, got %d", len(rules))
	}
}
