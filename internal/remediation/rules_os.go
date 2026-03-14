package remediation

import "fmt"

// getOS looks up an OS metric in the snapshot, checking both the agent prefix
// (os.*) and the SQL collector prefix (pg.os.*).
func getOS(snap MetricSnapshot, suffix string) (float64, bool) {
	if v, ok := snap.Get("os." + suffix); ok {
		return v, true
	}
	return snap.Get("pg.os." + suffix)
}

// isOSMetric checks if a metric key matches an OS metric with either prefix.
func isOSMetric(key, suffix string) bool {
	return key == "os."+suffix || key == "pg.os."+suffix
}

func osRules() []Rule {
	return []Rule{
		{
			ID:       "rem_cpu_high",
			Priority: PrioritySuggestion,
			Category: CategoryPerformance,
			Evaluate: func(ctx EvalContext) *RuleResult {
				user, ok1 := getOS(ctx.Snapshot, "cpu.user_pct")
				sys, ok2 := getOS(ctx.Snapshot, "cpu.system_pct")
				if !ok1 || !ok2 {
					return nil
				}
				total := user + sys
				if total > 80 {
					return &RuleResult{
						Title: "High CPU utilization",
						Description: fmt.Sprintf(
							"CPU usage at %.0f%% (user: %.0f%%, system: %.0f%%). "+
								"Investigate CPU-intensive queries with pg_stat_statements. "+
								"Check for missing indexes causing sequential scans.",
							total, user, sys),
						MetricKey:   "os.cpu.user_pct",
						MetricValue: total,
					}
				}
				return nil
			},
		},
		{
			ID:       "rem_cpu_iowait",
			Priority: PriorityActionRequired,
			Category: CategoryPerformance,
			Evaluate: func(ctx EvalContext) *RuleResult {
				var val float64
				var ok bool
				if isOSMetric(ctx.MetricKey, "cpu.iowait_pct") {
					val = ctx.Value
					ok = true
				} else {
					val, ok = getOS(ctx.Snapshot, "cpu.iowait_pct")
				}
				if !ok {
					return nil
				}
				if val > 20 {
					return &RuleResult{
						Title: "I/O wait bottleneck detected",
						Description: fmt.Sprintf(
							"I/O wait at %.0f%%, indicating storage is the bottleneck. "+
								"Check for queries doing large sequential scans or sorts spilling to disk. "+
								"Consider faster storage (NVMe), increasing work_mem, or adding indexes.",
							val),
						MetricKey:   "os.cpu.iowait_pct",
						MetricValue: val,
					}
				}
				return nil
			},
		},
		{
			ID:       "rem_mem_pressure",
			Priority: PriorityActionRequired,
			Category: CategoryCapacity,
			Evaluate: func(ctx EvalContext) *RuleResult {
				avail, ok1 := getOS(ctx.Snapshot, "memory.available_kb")
				total, ok2 := getOS(ctx.Snapshot, "memory.total_kb")
				if !ok1 || !ok2 || total == 0 {
					return nil
				}
				pct := (avail / total) * 100
				if pct < 10 {
					return &RuleResult{
						Title: "Memory pressure detected",
						Description: fmt.Sprintf(
							"Available memory is only %.1f%% of total (%.0f KB available / %.0f KB total). "+
								"The system may start swapping, severely impacting PostgreSQL performance. "+
								"Reduce shared_buffers, work_mem, or add more RAM.",
							pct, avail, total),
						MetricKey:   "os.memory.available_kb",
						MetricValue: avail,
					}
				}
				return nil
			},
		},
		{
			ID:       "rem_mem_overcommit",
			Priority: PrioritySuggestion,
			Category: CategoryCapacity,
			Evaluate: func(ctx EvalContext) *RuleResult {
				committed, ok1 := getOS(ctx.Snapshot, "memory.committed_as_kb")
				limit, ok2 := getOS(ctx.Snapshot, "memory.commit_limit_kb")
				if !ok1 || !ok2 || limit == 0 {
					return nil
				}
				if committed > limit {
					return &RuleResult{
						Title: "Memory overcommit detected",
						Description: fmt.Sprintf(
							"Committed memory (%.0f KB) exceeds commit limit (%.0f KB). "+
								"The OOM killer may terminate PostgreSQL processes under pressure. "+
								"Review vm.overcommit_memory sysctl and reduce total memory reservations.",
							committed, limit),
						MetricKey:   "os.memory.committed_as_kb",
						MetricValue: committed,
					}
				}
				return nil
			},
		},
		{
			ID:       "rem_load_high",
			Priority: PrioritySuggestion,
			Category: CategoryPerformance,
			Evaluate: func(ctx EvalContext) *RuleResult {
				var val float64
				var ok bool
				if isOSMetric(ctx.MetricKey, "load.1m") {
					val = ctx.Value
					ok = true
				} else {
					val, ok = getOS(ctx.Snapshot, "load.1m")
				}
				if !ok {
					return nil
				}
				if val > 4.0 {
					return &RuleResult{
						Title: "System load elevated",
						Description: fmt.Sprintf(
							"1-minute load average is %.2f. "+
								"This indicates more processes are competing for CPU than available cores. "+
								"Investigate with top/htop and check pg_stat_activity for runaway queries.",
							val),
						MetricKey:   "os.load.1m",
						MetricValue: val,
					}
				}
				return nil
			},
		},
		{
			ID:       "rem_disk_util",
			Priority: PriorityActionRequired,
			Category: CategoryCapacity,
			Evaluate: func(ctx EvalContext) *RuleResult {
				var val float64
				var ok bool
				if isOSMetric(ctx.MetricKey, "disk.util_pct") {
					val = ctx.Value
					ok = true
				} else {
					val, ok = getOS(ctx.Snapshot, "disk.util_pct")
				}
				if !ok {
					return nil
				}
				if val > 80 {
					return &RuleResult{
						Title: "Disk saturation approaching",
						Description: fmt.Sprintf(
							"Disk utilization at %.0f%%. The device is spending most of its time servicing I/O. "+
								"This leads to increased query latencies and checkpoint stalls. "+
								"Consider moving to faster storage or distributing I/O across multiple disks.",
							val),
						MetricKey:   "os.disk.util_pct",
						MetricValue: val,
					}
				}
				return nil
			},
		},
		{
			ID:       "rem_disk_read_latency",
			Priority: PrioritySuggestion,
			Category: CategoryPerformance,
			Evaluate: func(ctx EvalContext) *RuleResult {
				var val float64
				var ok bool
				if isOSMetric(ctx.MetricKey, "disk.read_await_ms") {
					val = ctx.Value
					ok = true
				} else {
					val, ok = getOS(ctx.Snapshot, "disk.read_await_ms")
				}
				if !ok {
					return nil
				}
				if val > 20 {
					return &RuleResult{
						Title: "Storage read latency elevated",
						Description: fmt.Sprintf(
							"Average read latency is %.1f ms (threshold: 20 ms). "+
								"High read latency impacts sequential scans and index lookups. "+
								"Check for I/O contention from other processes or consider storage upgrades.",
							val),
						MetricKey:   "os.disk.read_await_ms",
						MetricValue: val,
					}
				}
				return nil
			},
		},
		{
			ID:       "rem_disk_write_latency",
			Priority: PrioritySuggestion,
			Category: CategoryPerformance,
			Evaluate: func(ctx EvalContext) *RuleResult {
				var val float64
				var ok bool
				if isOSMetric(ctx.MetricKey, "disk.write_await_ms") {
					val = ctx.Value
					ok = true
				} else {
					val, ok = getOS(ctx.Snapshot, "disk.write_await_ms")
				}
				if !ok {
					return nil
				}
				if val > 20 {
					return &RuleResult{
						Title: "Storage write latency elevated",
						Description: fmt.Sprintf(
							"Average write latency is %.1f ms (threshold: 20 ms). "+
								"High write latency impacts WAL writes and checkpoints. "+
								"Consider enabling WAL compression, tuning checkpoint_completion_target, or upgrading storage.",
							val),
						MetricKey:   "os.disk.write_await_ms",
						MetricValue: val,
					}
				}
				return nil
			},
		},
	}
}
