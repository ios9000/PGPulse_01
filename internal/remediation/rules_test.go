package remediation

import (
	"context"
	"testing"
)

func TestRuleIDs_Unique(t *testing.T) {
	t.Parallel()
	pg := pgRules()
	os := osRules()
	all := append(pg, os...)
	seen := make(map[string]bool)
	for _, r := range all {
		if seen[r.ID] {
			t.Errorf("duplicate rule ID: %s", r.ID)
		}
		seen[r.ID] = true
	}
}

func TestAllRules_HaveRequiredFields(t *testing.T) {
	t.Parallel()
	pg := pgRules()
	os := osRules()
	all := append(pg, os...)
	for _, r := range all {
		if r.ID == "" {
			t.Error("rule has empty ID")
		}
		if r.Priority == "" {
			t.Errorf("rule %s has empty Priority", r.ID)
		}
		if r.Category == "" {
			t.Errorf("rule %s has empty Category", r.ID)
		}
		if r.Evaluate == nil {
			t.Errorf("rule %s has nil Evaluate function", r.ID)
		}
	}
}

func TestPGRules(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		ruleID    string
		snapshot  MetricSnapshot
		metricKey string
		value     float64
		wantMatch bool
	}{
		// rem_conn_high — utilization_pct is already a percentage
		{"conn_high positive", "rem_conn_high", MetricSnapshot{
			"pg.connections.utilization_pct": 85,
		}, "pg.connections.utilization_pct", 85, true},
		{"conn_high negative", "rem_conn_high", MetricSnapshot{
			"pg.connections.utilization_pct": 50,
		}, "pg.connections.utilization_pct", 50, false},
		{"conn_high boundary 80", "rem_conn_high", MetricSnapshot{
			"pg.connections.utilization_pct": 80,
		}, "pg.connections.utilization_pct", 80, false}, // > 80, not >=
		{"conn_high diagnose mode", "rem_conn_high", MetricSnapshot{
			"pg.connections.utilization_pct": 85,
		}, "", 0, true},

		// rem_conn_exhausted
		{"conn_exhausted positive", "rem_conn_exhausted", MetricSnapshot{
			"pg.connections.utilization_pct": 99,
		}, "pg.connections.utilization_pct", 99, true},
		{"conn_exhausted negative", "rem_conn_exhausted", MetricSnapshot{
			"pg.connections.utilization_pct": 90,
		}, "pg.connections.utilization_pct", 90, false},

		// rem_cache_low
		{"cache_low positive", "rem_cache_low", MetricSnapshot{
			"pg.cache.hit_ratio": 85,
		}, "pg.cache.hit_ratio", 85, true},
		{"cache_low negative", "rem_cache_low", MetricSnapshot{
			"pg.cache.hit_ratio": 95,
		}, "pg.cache.hit_ratio", 95, false},
		{"cache_low boundary", "rem_cache_low", MetricSnapshot{
			"pg.cache.hit_ratio": 90,
		}, "pg.cache.hit_ratio", 90, false}, // < 90

		// rem_commit_ratio_low
		{"commit_ratio positive", "rem_commit_ratio_low", MetricSnapshot{
			"pg.transactions.commit_ratio_pct": 80,
		}, "pg.transactions.commit_ratio_pct", 80, true},
		{"commit_ratio negative", "rem_commit_ratio_low", MetricSnapshot{
			"pg.transactions.commit_ratio_pct": 95,
		}, "pg.transactions.commit_ratio_pct", 95, false},

		// rem_repl_lag_bytes (10-100 MB)
		{"repl_lag positive", "rem_repl_lag_bytes", MetricSnapshot{
			"pg.replication.lag.replay_bytes": 20 * 1024 * 1024,
		}, "pg.replication.lag.replay_bytes", 20 * 1024 * 1024, true},
		{"repl_lag negative small", "rem_repl_lag_bytes", MetricSnapshot{
			"pg.replication.lag.replay_bytes": 5 * 1024 * 1024,
		}, "pg.replication.lag.replay_bytes", 5 * 1024 * 1024, false},
		{"repl_lag negative large", "rem_repl_lag_bytes", MetricSnapshot{
			"pg.replication.lag.replay_bytes": 200 * 1024 * 1024,
		}, "pg.replication.lag.replay_bytes", 200 * 1024 * 1024, false}, // >100 MB triggers critical instead

		// rem_repl_lag_critical (>100 MB)
		{"repl_lag_crit positive", "rem_repl_lag_critical", MetricSnapshot{
			"pg.replication.lag.replay_bytes": 200 * 1024 * 1024,
		}, "pg.replication.lag.replay_bytes", 200 * 1024 * 1024, true},
		{"repl_lag_crit negative", "rem_repl_lag_critical", MetricSnapshot{
			"pg.replication.lag.replay_bytes": 50 * 1024 * 1024,
		}, "pg.replication.lag.replay_bytes", 50 * 1024 * 1024, false},

		// rem_repl_slot_inactive — collector emits slot.active: 0=inactive, 1=active
		{"slot_inactive positive", "rem_repl_slot_inactive", MetricSnapshot{
			"pg.replication.slot.active": 0,
		}, "pg.replication.slot.active", 0, true},
		{"slot_inactive negative", "rem_repl_slot_inactive", MetricSnapshot{
			"pg.replication.slot.active": 1,
		}, "pg.replication.slot.active", 1, false},

		// rem_long_txn_warn (60-300s)
		{"long_txn_warn positive", "rem_long_txn_warn", MetricSnapshot{
			"pg.long_transactions.oldest_seconds": 120,
		}, "pg.long_transactions.oldest_seconds", 120, true},
		{"long_txn_warn negative", "rem_long_txn_warn", MetricSnapshot{
			"pg.long_transactions.oldest_seconds": 30,
		}, "pg.long_transactions.oldest_seconds", 30, false},
		{"long_txn_warn too high", "rem_long_txn_warn", MetricSnapshot{
			"pg.long_transactions.oldest_seconds": 400,
		}, "pg.long_transactions.oldest_seconds", 400, false},

		// rem_long_txn_crit (>300s)
		{"long_txn_crit positive", "rem_long_txn_crit", MetricSnapshot{
			"pg.long_transactions.oldest_seconds": 400,
		}, "pg.long_transactions.oldest_seconds", 400, true},
		{"long_txn_crit negative", "rem_long_txn_crit", MetricSnapshot{
			"pg.long_transactions.oldest_seconds": 200,
		}, "pg.long_transactions.oldest_seconds", 200, false},

		// rem_locks_blocking
		{"locks positive", "rem_locks_blocking", MetricSnapshot{
			"pg.locks.blocked_count": 3,
		}, "pg.locks.blocked_count", 3, true},
		{"locks negative", "rem_locks_blocking", MetricSnapshot{
			"pg.locks.blocked_count": 0,
		}, "pg.locks.blocked_count", 0, false},

		// rem_pgss_fill
		{"pgss_fill positive", "rem_pgss_fill", MetricSnapshot{
			"pg.extensions.pgss_fill_pct": 97,
		}, "pg.extensions.pgss_fill_pct", 97, true},
		{"pgss_fill negative", "rem_pgss_fill", MetricSnapshot{
			"pg.extensions.pgss_fill_pct": 80,
		}, "pg.extensions.pgss_fill_pct", 80, false},

		// rem_wraparound_warn (20-50%)
		{"wraparound_warn positive", "rem_wraparound_warn", MetricSnapshot{
			"pg.server.wraparound_pct": 30,
		}, "pg.server.wraparound_pct", 30, true},
		{"wraparound_warn negative", "rem_wraparound_warn", MetricSnapshot{
			"pg.server.wraparound_pct": 10,
		}, "pg.server.wraparound_pct", 10, false},
		{"wraparound_warn too high", "rem_wraparound_warn", MetricSnapshot{
			"pg.server.wraparound_pct": 60,
		}, "pg.server.wraparound_pct", 60, false},

		// rem_wraparound_crit (>50%)
		{"wraparound_crit positive", "rem_wraparound_crit", MetricSnapshot{
			"pg.server.wraparound_pct": 60,
		}, "pg.server.wraparound_pct", 60, true},
		{"wraparound_crit negative", "rem_wraparound_crit", MetricSnapshot{
			"pg.server.wraparound_pct": 40,
		}, "pg.server.wraparound_pct", 40, false},

		// rem_track_io
		{"track_io positive", "rem_track_io", MetricSnapshot{
			"pg.settings.track_io_timing": 0,
		}, "pg.settings.track_io_timing", 0, true},
		{"track_io negative", "rem_track_io", MetricSnapshot{
			"pg.settings.track_io_timing": 1,
		}, "pg.settings.track_io_timing", 1, false},

		// rem_deadlocks
		{"deadlocks positive", "rem_deadlocks", MetricSnapshot{
			"pg.transactions.deadlocks": 5,
		}, "pg.transactions.deadlocks", 5, true},
		{"deadlocks negative", "rem_deadlocks", MetricSnapshot{
			"pg.transactions.deadlocks": 0,
		}, "pg.transactions.deadlocks", 0, false},

		// rem_bloat_high (2-50x)
		{"bloat_high positive", "rem_bloat_high", MetricSnapshot{
			"pg.db.bloat.table_ratio": 5,
		}, "pg.db.bloat.table_ratio", 5, true},
		{"bloat_high negative", "rem_bloat_high", MetricSnapshot{
			"pg.db.bloat.table_ratio": 1.5,
		}, "pg.db.bloat.table_ratio", 1.5, false},
		{"bloat_high too high", "rem_bloat_high", MetricSnapshot{
			"pg.db.bloat.table_ratio": 60,
		}, "pg.db.bloat.table_ratio", 60, false},

		// rem_bloat_extreme (>50x)
		{"bloat_extreme positive", "rem_bloat_extreme", MetricSnapshot{
			"pg.db.bloat.table_ratio": 60,
		}, "pg.db.bloat.table_ratio", 60, true},
		{"bloat_extreme negative", "rem_bloat_extreme", MetricSnapshot{
			"pg.db.bloat.table_ratio": 30,
		}, "pg.db.bloat.table_ratio", 30, false},
	}

	rules := pgRules()
	ruleMap := make(map[string]Rule)
	for _, r := range rules {
		ruleMap[r.ID] = r
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rule, ok := ruleMap[tc.ruleID]
			if !ok {
				t.Fatalf("rule %s not found", tc.ruleID)
			}
			ctx := EvalContext{
				InstanceID: "test",
				MetricKey:  tc.metricKey,
				Value:      tc.value,
				Snapshot:   tc.snapshot,
			}
			result := rule.Evaluate(ctx)
			if tc.wantMatch && result == nil {
				t.Errorf("expected rule %s to fire, got nil", tc.ruleID)
			}
			if !tc.wantMatch && result != nil {
				t.Errorf("expected rule %s NOT to fire, got %q", tc.ruleID, result.Title)
			}
			if result != nil {
				if result.Title == "" {
					t.Error("result has empty Title")
				}
				if result.Description == "" {
					t.Error("result has empty Description")
				}
			}
		})
	}
}

func TestOSRules(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		ruleID    string
		snapshot  MetricSnapshot
		metricKey string
		value     float64
		wantMatch bool
	}{
		// rem_cpu_high
		{"cpu_high positive", "rem_cpu_high", MetricSnapshot{
			"os.cpu.user_pct": 60, "os.cpu.system_pct": 25,
		}, "", 0, true},
		{"cpu_high negative", "rem_cpu_high", MetricSnapshot{
			"os.cpu.user_pct": 30, "os.cpu.system_pct": 20,
		}, "", 0, false},
		{"cpu_high missing key", "rem_cpu_high", MetricSnapshot{
			"os.cpu.user_pct": 60,
		}, "", 0, false},

		// rem_cpu_iowait
		{"iowait positive", "rem_cpu_iowait", MetricSnapshot{
			"os.cpu.iowait_pct": 30,
		}, "os.cpu.iowait_pct", 30, true},
		{"iowait negative", "rem_cpu_iowait", MetricSnapshot{
			"os.cpu.iowait_pct": 10,
		}, "os.cpu.iowait_pct", 10, false},

		// rem_mem_pressure
		{"mem_pressure positive", "rem_mem_pressure", MetricSnapshot{
			"os.memory.available_kb": 500000, "os.memory.total_kb": 8000000,
		}, "", 0, true},
		{"mem_pressure negative", "rem_mem_pressure", MetricSnapshot{
			"os.memory.available_kb": 2000000, "os.memory.total_kb": 8000000,
		}, "", 0, false},

		// rem_mem_overcommit
		{"overcommit positive", "rem_mem_overcommit", MetricSnapshot{
			"os.memory.committed_as_kb": 10000000, "os.memory.commit_limit_kb": 8000000,
		}, "", 0, true},
		{"overcommit negative", "rem_mem_overcommit", MetricSnapshot{
			"os.memory.committed_as_kb": 5000000, "os.memory.commit_limit_kb": 8000000,
		}, "", 0, false},

		// rem_load_high
		{"load positive", "rem_load_high", MetricSnapshot{
			"os.load.1m": 6.5,
		}, "os.load.1m", 6.5, true},
		{"load negative", "rem_load_high", MetricSnapshot{
			"os.load.1m": 2.0,
		}, "os.load.1m", 2.0, false},

		// rem_disk_util
		{"disk_util positive", "rem_disk_util", MetricSnapshot{
			"os.disk.util_pct": 90,
		}, "os.disk.util_pct", 90, true},
		{"disk_util negative", "rem_disk_util", MetricSnapshot{
			"os.disk.util_pct": 50,
		}, "os.disk.util_pct", 50, false},

		// rem_disk_read_latency
		{"read_latency positive", "rem_disk_read_latency", MetricSnapshot{
			"os.disk.read_await_ms": 30,
		}, "os.disk.read_await_ms", 30, true},
		{"read_latency negative", "rem_disk_read_latency", MetricSnapshot{
			"os.disk.read_await_ms": 5,
		}, "os.disk.read_await_ms", 5, false},

		// rem_disk_write_latency
		{"write_latency positive", "rem_disk_write_latency", MetricSnapshot{
			"os.disk.write_await_ms": 30,
		}, "os.disk.write_await_ms", 30, true},
		{"write_latency negative", "rem_disk_write_latency", MetricSnapshot{
			"os.disk.write_await_ms": 5,
		}, "os.disk.write_await_ms", 5, false},
	}

	rules := osRules()
	ruleMap := make(map[string]Rule)
	for _, r := range rules {
		ruleMap[r.ID] = r
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rule, ok := ruleMap[tc.ruleID]
			if !ok {
				t.Fatalf("rule %s not found", tc.ruleID)
			}
			ctx := EvalContext{
				InstanceID: "test",
				MetricKey:  tc.metricKey,
				Value:      tc.value,
				Snapshot:   tc.snapshot,
			}
			result := rule.Evaluate(ctx)
			if tc.wantMatch && result == nil {
				t.Errorf("expected rule %s to fire, got nil", tc.ruleID)
			}
			if !tc.wantMatch && result != nil {
				t.Errorf("expected rule %s NOT to fire, got %q", tc.ruleID, result.Title)
			}
			if result != nil {
				if result.Title == "" {
					t.Error("result has empty Title")
				}
				if result.Description == "" {
					t.Error("result has empty Description")
				}
			}
		})
	}
}

func TestGetOS_BothPrefixes(t *testing.T) {
	t.Parallel()

	// Agent prefix (os.*)
	snap1 := MetricSnapshot{"os.cpu.user_pct": 42}
	v, ok := getOS(snap1, "cpu.user_pct")
	if !ok || v != 42 {
		t.Errorf("expected 42/true from os.* prefix, got %v/%v", v, ok)
	}

	// SQL collector prefix (pg.os.*)
	snap2 := MetricSnapshot{"pg.os.cpu.user_pct": 55}
	v, ok = getOS(snap2, "cpu.user_pct")
	if !ok || v != 55 {
		t.Errorf("expected 55/true from pg.os.* prefix, got %v/%v", v, ok)
	}

	// os.* takes priority when both present
	snap3 := MetricSnapshot{"os.cpu.user_pct": 10, "pg.os.cpu.user_pct": 20}
	v, ok = getOS(snap3, "cpu.user_pct")
	if !ok || v != 10 {
		t.Errorf("expected os.* prefix to take priority, got %v/%v", v, ok)
	}

	// Missing key
	snap4 := MetricSnapshot{"os.memory.total_kb": 100}
	_, ok = getOS(snap4, "cpu.user_pct")
	if ok {
		t.Error("expected false for missing key")
	}
}

func TestOSRules_PGOSPrefix(t *testing.T) {
	t.Parallel()

	rules := osRules()
	ruleMap := make(map[string]Rule)
	for _, r := range rules {
		ruleMap[r.ID] = r
	}

	// CPU high should fire with pg.os.* prefix
	rule := ruleMap["rem_cpu_high"]
	ctx := EvalContext{
		InstanceID: "test",
		Snapshot: MetricSnapshot{
			"pg.os.cpu.user_pct":   60,
			"pg.os.cpu.system_pct": 25,
		},
	}
	result := rule.Evaluate(ctx)
	if result == nil {
		t.Error("rem_cpu_high should fire with pg.os.* prefix")
	}

	// iowait should fire with pg.os.* prefix via alert trigger
	rule = ruleMap["rem_cpu_iowait"]
	ctx = EvalContext{
		InstanceID: "test",
		MetricKey:  "pg.os.cpu.iowait_pct",
		Value:      30,
		Snapshot:   MetricSnapshot{"pg.os.cpu.iowait_pct": 30},
	}
	result = rule.Evaluate(ctx)
	if result == nil {
		t.Error("rem_cpu_iowait should fire with pg.os.* prefix")
	}

	// mem_pressure should fire with pg.os.* prefix
	rule = ruleMap["rem_mem_pressure"]
	ctx = EvalContext{
		InstanceID: "test",
		Snapshot: MetricSnapshot{
			"pg.os.memory.available_kb": 500000,
			"pg.os.memory.total_kb":     8000000,
		},
	}
	result = rule.Evaluate(ctx)
	if result == nil {
		t.Error("rem_mem_pressure should fire with pg.os.* prefix")
	}

	// disk_util should fire with pg.os.* prefix via alert trigger
	rule = ruleMap["rem_disk_util"]
	ctx = EvalContext{
		InstanceID: "test",
		MetricKey:  "pg.os.disk.util_pct",
		Value:      90,
		Snapshot:   MetricSnapshot{"pg.os.disk.util_pct": 90},
	}
	result = rule.Evaluate(ctx)
	if result == nil {
		t.Error("rem_disk_util should fire with pg.os.* prefix")
	}
}

func TestWraparound_Fires(t *testing.T) {
	t.Parallel()

	rules := pgRules()
	ruleMap := make(map[string]Rule)
	for _, r := range rules {
		ruleMap[r.ID] = r
	}

	// Warning fires at 30%
	rule := ruleMap["rem_wraparound_warn"]
	ctx := EvalContext{
		InstanceID: "test",
		Snapshot:   MetricSnapshot{"pg.server.wraparound_pct": 30},
	}
	result := rule.Evaluate(ctx)
	if result == nil {
		t.Error("rem_wraparound_warn should fire at 30%")
	}

	// Critical fires at 60%
	rule = ruleMap["rem_wraparound_crit"]
	ctx = EvalContext{
		InstanceID: "test",
		Snapshot:   MetricSnapshot{"pg.server.wraparound_pct": 60},
	}
	result = rule.Evaluate(ctx)
	if result == nil {
		t.Error("rem_wraparound_crit should fire at 60%")
	}

	// Warning does NOT fire at 10%
	rule = ruleMap["rem_wraparound_warn"]
	ctx = EvalContext{
		InstanceID: "test",
		Snapshot:   MetricSnapshot{"pg.server.wraparound_pct": 10},
	}
	result = rule.Evaluate(ctx)
	if result != nil {
		t.Error("rem_wraparound_warn should NOT fire at 10%")
	}
}

// TestDiagnoseMode_MatchesAlertMode verifies that Diagnose-mode produces the same
// result as alert-triggered mode for the same snapshot values.
func TestDiagnoseMode_MatchesAlertMode(t *testing.T) {
	t.Parallel()
	engine := NewEngine()

	snapshot := MetricSnapshot{
		"pg.cache.hit_ratio": 80,
	}

	// Alert-triggered mode
	alertRecs := engine.EvaluateMetric(context.Background(), "inst1", "pg.cache.hit_ratio", 80, nil, "warning", snapshot)
	// Diagnose mode
	diagnoseRecs := engine.Diagnose(context.Background(), "inst1", snapshot)

	// Find rem_cache_low in both
	var alertResult, diagnoseResult *Recommendation
	for i := range alertRecs {
		if alertRecs[i].RuleID == "rem_cache_low" {
			alertResult = &alertRecs[i]
		}
	}
	for i := range diagnoseRecs {
		if diagnoseRecs[i].RuleID == "rem_cache_low" {
			diagnoseResult = &diagnoseRecs[i]
		}
	}

	if alertResult == nil {
		t.Fatal("rem_cache_low not found in alert mode")
	}
	if diagnoseResult == nil {
		t.Fatal("rem_cache_low not found in diagnose mode")
	}
	if alertResult.Title != diagnoseResult.Title {
		t.Errorf("title mismatch: alert=%q diagnose=%q", alertResult.Title, diagnoseResult.Title)
	}
}

func TestNullStore_ImplementsInterface(t *testing.T) {
	t.Parallel()
	var store RecommendationStore = NewNullStore()
	ctx := context.Background()

	recs, err := store.Write(ctx, []Recommendation{{RuleID: "test"}})
	if err != nil {
		t.Errorf("Write: %v", err)
	}
	if len(recs) != 0 {
		t.Errorf("Write: expected nil, got %d recs", len(recs))
	}

	list, total, err := store.ListByInstance(ctx, "inst1", ListOpts{})
	if err != nil || total != 0 || len(list) != 0 {
		t.Errorf("ListByInstance: unexpected result")
	}

	list, total, err = store.ListAll(ctx, ListOpts{})
	if err != nil || total != 0 || len(list) != 0 {
		t.Errorf("ListAll: unexpected result")
	}

	list, err = store.ListByAlertEvent(ctx, 1)
	if err != nil || len(list) != 0 {
		t.Errorf("ListByAlertEvent: unexpected result")
	}

	if err := store.Acknowledge(ctx, 1, "user"); err != nil {
		t.Errorf("Acknowledge: %v", err)
	}

	if err := store.CleanOld(ctx, 0); err != nil {
		t.Errorf("CleanOld: %v", err)
	}
}
