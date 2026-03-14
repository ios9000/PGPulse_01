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
	// 14 PGAM-ported + 2 new + 3 deferred + 2 ML anomaly + 1 logical repl = 22
	if len(rules) != 22 {
		t.Errorf("expected 22 builtin rules, got %d", len(rules))
	}
}

func TestBuiltinRulesMetricKeys(t *testing.T) {
	rules := BuiltinRules()
	ruleMap := make(map[string]Rule)
	for _, r := range rules {
		ruleMap[r.ID] = r
	}

	expected := map[string]string{
		"wraparound_warning":        "pg.server.wraparound_pct",
		"connections_warning":       "pg.connections.utilization_pct",
		"cache_hit_warning":         "pg.cache.hit_ratio",
		"commit_ratio_warning":      "pg.transactions.commit_ratio_pct",
		"replication_slot_inactive":  "pg.replication.slot.active",
		"long_transaction_warning":   "pg.long_transactions.oldest_seconds",
		"table_bloat_warning":        "pg.db.bloat.table_ratio",
		"pgss_dealloc_warning":       "pg.extensions.pgss_fill_pct",
		"replication_lag_warning":    "pg.replication.lag.total_bytes",
		"logical_repl_pending_sync":  "pg.db.logical_replication.pending_sync_tables",
	}

	for ruleID, wantMetric := range expected {
		t.Run(ruleID, func(t *testing.T) {
			r, ok := ruleMap[ruleID]
			if !ok {
				t.Fatalf("rule %q not found", ruleID)
			}
			if r.Metric != wantMetric {
				t.Errorf("rule %q: Metric = %q, want %q", ruleID, r.Metric, wantMetric)
			}
		})
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
