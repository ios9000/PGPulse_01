package alert

import "testing"

func TestBuiltinRulesValid(t *testing.T) {
	validOperators := map[Operator]bool{
		OpGreater: true, OpGreaterEqual: true,
		OpLess: true, OpLessEqual: true,
		OpEqual: true, OpNotEqual: true,
	}
	validSeverities := map[Severity]bool{
		SeverityInfo: true, SeverityWarning: true, SeverityCritical: true,
	}

	rules := BuiltinRules()
	for _, r := range rules {
		t.Run(r.ID, func(t *testing.T) {
			if r.ID == "" {
				t.Error("ID is empty")
			}
			if r.Name == "" {
				t.Error("Name is empty")
			}
			if r.Metric == "" {
				t.Error("Metric is empty")
			}
			if !validOperators[r.Operator] {
				t.Errorf("invalid Operator: %q", r.Operator)
			}
			if !validSeverities[r.Severity] {
				t.Errorf("invalid Severity: %q", r.Severity)
			}
			if r.ConsecutiveCount <= 0 {
				t.Errorf("ConsecutiveCount = %d, want > 0", r.ConsecutiveCount)
			}
			if r.CooldownMinutes <= 0 {
				t.Errorf("CooldownMinutes = %d, want > 0", r.CooldownMinutes)
			}
			if r.Source != SourceBuiltin {
				t.Errorf("Source = %q, want %q", r.Source, SourceBuiltin)
			}
		})
	}
}

func TestBuiltinRulesNoDuplicateIDs(t *testing.T) {
	rules := BuiltinRules()
	seen := make(map[string]bool)
	for _, r := range rules {
		if seen[r.ID] {
			t.Errorf("duplicate rule ID: %s", r.ID)
		}
		seen[r.ID] = true
	}
}

func TestBuiltinRulesCount(t *testing.T) {
	rules := BuiltinRules()
	// 14 PGAM-ported + 2 new + 3 deferred = 19
	if len(rules) != 19 {
		t.Errorf("expected 19 builtin rules, got %d", len(rules))
	}
}

func TestDeferredRulesDisabled(t *testing.T) {
	deferredIDs := map[string]bool{
		"wal_spike_warning":        true,
		"query_regression_warning": true,
		"disk_forecast_critical":   true,
	}

	rules := BuiltinRules()
	found := 0
	for _, r := range rules {
		if deferredIDs[r.ID] {
			found++
			if r.Enabled {
				t.Errorf("deferred rule %q should be disabled (Enabled=false)", r.ID)
			}
		}
	}
	if found != len(deferredIDs) {
		t.Errorf("found %d deferred rules, expected %d", found, len(deferredIDs))
	}
}
