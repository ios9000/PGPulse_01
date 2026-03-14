package remediation

import "fmt"

func pgRules() []Rule {
	return []Rule{
		{
			ID:       "rem_conn_high",
			Priority: PrioritySuggestion,
			Category: CategoryCapacity,
			Evaluate: func(ctx EvalContext) *RuleResult {
				if ctx.MetricKey == "pg.connections.active" {
					maxConn, ok := ctx.Snapshot.Get("pg.connections.max_connections")
					if !ok || maxConn == 0 {
						return nil
					}
					pct := (ctx.Value / maxConn) * 100
					if pct > 80 && pct < 99 {
						return &RuleResult{
							Title: "Consider connection pooling",
							Description: fmt.Sprintf(
								"Connection utilization at %.0f%% (%v/%v). "+
									"Consider adding PgBouncer or increasing max_connections. "+
									"Review application connection pool settings for idle connections.",
								pct, ctx.Value, maxConn),
							DocURL: "https://www.pgbouncer.org/",
						}
					}
					return nil
				}
				active, ok1 := ctx.Snapshot.Get("pg.connections.active")
				maxConn, ok2 := ctx.Snapshot.Get("pg.connections.max_connections")
				if !ok1 || !ok2 || maxConn == 0 {
					return nil
				}
				pct := (active / maxConn) * 100
				if pct > 80 && pct < 99 {
					return &RuleResult{
						Title: "Consider connection pooling",
						Description: fmt.Sprintf(
							"Connection utilization at %.0f%% (%v/%v). "+
								"Consider adding PgBouncer or increasing max_connections. "+
								"Review application connection pool settings for idle connections.",
							pct, active, maxConn),
						DocURL: "https://www.pgbouncer.org/",
					}
				}
				return nil
			},
		},
		{
			ID:       "rem_conn_exhausted",
			Priority: PriorityActionRequired,
			Category: CategoryCapacity,
			Evaluate: func(ctx EvalContext) *RuleResult {
				if ctx.MetricKey == "pg.connections.active" {
					maxConn, ok := ctx.Snapshot.Get("pg.connections.max_connections")
					if !ok || maxConn == 0 {
						return nil
					}
					pct := (ctx.Value / maxConn) * 100
					if pct >= 99 {
						return &RuleResult{
							Title: "Connections near limit",
							Description: fmt.Sprintf(
								"Connection utilization at %.0f%% (%v/%v). "+
									"New connections will be refused. Immediately terminate idle sessions "+
									"and deploy a connection pooler like PgBouncer.",
								pct, ctx.Value, maxConn),
							DocURL: "https://www.postgresql.org/docs/current/runtime-config-connection.html",
						}
					}
					return nil
				}
				active, ok1 := ctx.Snapshot.Get("pg.connections.active")
				maxConn, ok2 := ctx.Snapshot.Get("pg.connections.max_connections")
				if !ok1 || !ok2 || maxConn == 0 {
					return nil
				}
				pct := (active / maxConn) * 100
				if pct >= 99 {
					return &RuleResult{
						Title: "Connections near limit",
						Description: fmt.Sprintf(
							"Connection utilization at %.0f%% (%v/%v). "+
								"New connections will be refused. Immediately terminate idle sessions "+
								"and deploy a connection pooler like PgBouncer.",
							pct, active, maxConn),
						DocURL: "https://www.postgresql.org/docs/current/runtime-config-connection.html",
					}
				}
				return nil
			},
		},
		{
			ID:       "rem_cache_low",
			Priority: PrioritySuggestion,
			Category: CategoryPerformance,
			Evaluate: func(ctx EvalContext) *RuleResult {
				var val float64
				var ok bool
				if ctx.MetricKey == "pg.cache.hit_ratio" {
					val = ctx.Value
					ok = true
				} else {
					val, ok = ctx.Snapshot.Get("pg.cache.hit_ratio")
				}
				if !ok {
					return nil
				}
				if val < 90 {
					return &RuleResult{
						Title: "Review shared_buffers sizing",
						Description: fmt.Sprintf(
							"Buffer cache hit ratio is %.1f%%, below the recommended 90%% threshold. "+
								"Consider increasing shared_buffers or investigating queries that perform excessive sequential scans. "+
								"Run EXPLAIN ANALYZE on slow queries to identify missing indexes.",
							val),
						DocURL: "https://www.postgresql.org/docs/current/runtime-config-resource.html#GUC-SHARED-BUFFERS",
					}
				}
				return nil
			},
		},
		{
			ID:       "rem_commit_ratio_low",
			Priority: PrioritySuggestion,
			Category: CategoryPerformance,
			Evaluate: func(ctx EvalContext) *RuleResult {
				var val float64
				var ok bool
				if ctx.MetricKey == "pg.transactions.commit_ratio" {
					val = ctx.Value
					ok = true
				} else {
					val, ok = ctx.Snapshot.Get("pg.transactions.commit_ratio")
				}
				if !ok {
					return nil
				}
				if val < 90 {
					return &RuleResult{
						Title: "High rollback rate detected",
						Description: fmt.Sprintf(
							"Transaction commit ratio is %.1f%%, indicating a high rollback rate. "+
								"Investigate application error handling and retry logic. "+
								"Check pg_stat_database for rollback trends per database.",
							val),
						DocURL: "https://www.postgresql.org/docs/current/monitoring-stats.html#MONITORING-PG-STAT-DATABASE-VIEW",
					}
				}
				return nil
			},
		},
		{
			ID:       "rem_repl_lag_bytes",
			Priority: PrioritySuggestion,
			Category: CategoryReplication,
			Evaluate: func(ctx EvalContext) *RuleResult {
				var val float64
				var ok bool
				if ctx.MetricKey == "pg.replication.replay_lag_bytes" {
					val = ctx.Value
					ok = true
				} else {
					val, ok = ctx.Snapshot.Get("pg.replication.replay_lag_bytes")
				}
				if !ok {
					return nil
				}
				mb := val / (1024 * 1024)
				if mb > 10 && mb <= 100 {
					return &RuleResult{
						Title: "Check replica load and network",
						Description: fmt.Sprintf(
							"Replication replay lag is %.1f MB. "+
								"Verify replica is not under heavy read load and network latency is acceptable. "+
								"Check wal_receiver_status_interval and recovery settings on the replica.",
							mb),
						DocURL: "https://www.postgresql.org/docs/current/warm-standby.html#STREAMING-REPLICATION-MONITORING",
					}
				}
				return nil
			},
		},
		{
			ID:       "rem_repl_lag_critical",
			Priority: PriorityActionRequired,
			Category: CategoryReplication,
			Evaluate: func(ctx EvalContext) *RuleResult {
				var val float64
				var ok bool
				if ctx.MetricKey == "pg.replication.replay_lag_bytes" {
					val = ctx.Value
					ok = true
				} else {
					val, ok = ctx.Snapshot.Get("pg.replication.replay_lag_bytes")
				}
				if !ok {
					return nil
				}
				mb := val / (1024 * 1024)
				if mb > 100 {
					return &RuleResult{
						Title: "Replica severely lagging",
						Description: fmt.Sprintf(
							"Replication replay lag is %.1f MB, severely behind primary. "+
								"Risk of slot-induced WAL retention bloat. Consider pausing non-essential replica workloads "+
								"and verifying network connectivity. If unrecoverable, rebuild the replica.",
							mb),
						DocURL: "https://www.postgresql.org/docs/current/warm-standby.html#STREAMING-REPLICATION-MONITORING",
					}
				}
				return nil
			},
		},
		{
			ID:       "rem_repl_slot_inactive",
			Priority: PriorityActionRequired,
			Category: CategoryReplication,
			Evaluate: func(ctx EvalContext) *RuleResult {
				var val float64
				var ok bool
				if ctx.MetricKey == "pg.replication.slot_inactive" {
					val = ctx.Value
					ok = true
				} else {
					val, ok = ctx.Snapshot.Get("pg.replication.slot_inactive")
				}
				if !ok {
					return nil
				}
				if val > 0 {
					return &RuleResult{
						Title: "Inactive replication slots detected",
						Description: fmt.Sprintf(
							"%.0f inactive replication slot(s) found. "+
								"Inactive slots prevent WAL cleanup and can fill the disk. "+
								"Drop unused slots with pg_drop_replication_slot() or reconnect the subscriber.",
							val),
						DocURL: "https://www.postgresql.org/docs/current/view-pg-replication-slots.html",
					}
				}
				return nil
			},
		},
		{
			ID:       "rem_long_txn_warn",
			Priority: PrioritySuggestion,
			Category: CategoryPerformance,
			Evaluate: func(ctx EvalContext) *RuleResult {
				var val float64
				var ok bool
				if ctx.MetricKey == "pg.transactions.oldest_active_sec" {
					val = ctx.Value
					ok = true
				} else {
					val, ok = ctx.Snapshot.Get("pg.transactions.oldest_active_sec")
				}
				if !ok {
					return nil
				}
				if val > 60 && val <= 300 {
					return &RuleResult{
						Title: "Long-running transactions detected",
						Description: fmt.Sprintf(
							"Oldest active transaction has been running for %.0f seconds. "+
								"Long transactions hold locks, prevent autovacuum, and bloat tables. "+
								"Identify the session in pg_stat_activity and consider terminating it.",
							val),
						DocURL: "https://www.postgresql.org/docs/current/monitoring-stats.html#MONITORING-PG-STAT-ACTIVITY-VIEW",
					}
				}
				return nil
			},
		},
		{
			ID:       "rem_long_txn_crit",
			Priority: PriorityActionRequired,
			Category: CategoryPerformance,
			Evaluate: func(ctx EvalContext) *RuleResult {
				var val float64
				var ok bool
				if ctx.MetricKey == "pg.transactions.oldest_active_sec" {
					val = ctx.Value
					ok = true
				} else {
					val, ok = ctx.Snapshot.Get("pg.transactions.oldest_active_sec")
				}
				if !ok {
					return nil
				}
				if val > 300 {
					return &RuleResult{
						Title: "Stale transactions require intervention",
						Description: fmt.Sprintf(
							"Oldest active transaction has been running for %.0f seconds (>5 minutes). "+
								"This blocks autovacuum and causes table bloat. "+
								"Use pg_terminate_backend() to kill the offending session immediately.",
							val),
						DocURL: "https://www.postgresql.org/docs/current/functions-admin.html#FUNCTIONS-ADMIN-SIGNAL",
					}
				}
				return nil
			},
		},
		{
			ID:       "rem_locks_blocking",
			Priority: PrioritySuggestion,
			Category: CategoryPerformance,
			Evaluate: func(ctx EvalContext) *RuleResult {
				var val float64
				var ok bool
				if ctx.MetricKey == "pg.locks.blocked_count" {
					val = ctx.Value
					ok = true
				} else {
					val, ok = ctx.Snapshot.Get("pg.locks.blocked_count")
				}
				if !ok {
					return nil
				}
				if val > 0 {
					return &RuleResult{
						Title: "Blocking lock chains detected",
						Description: fmt.Sprintf(
							"%.0f session(s) are currently blocked by lock contention. "+
								"Review the lock tree to identify the blocker and consider canceling the blocking query. "+
								"Frequent blocking may indicate schema-level contention or missing indexes.",
							val),
						DocURL: "https://wiki.postgresql.org/wiki/Lock_Monitoring",
					}
				}
				return nil
			},
		},
		{
			ID:       "rem_pgss_fill",
			Priority: PrioritySuggestion,
			Category: CategoryMaintenance,
			Evaluate: func(ctx EvalContext) *RuleResult {
				var val float64
				var ok bool
				if ctx.MetricKey == "pg.statements.fill_pct" {
					val = ctx.Value
					ok = true
				} else {
					val, ok = ctx.Snapshot.Get("pg.statements.fill_pct")
				}
				if !ok {
					return nil
				}
				if val >= 95 {
					return &RuleResult{
						Title: "pg_stat_statements nearing capacity",
						Description: fmt.Sprintf(
							"pg_stat_statements is %.0f%% full. "+
								"New query fingerprints will evict older entries, causing loss of historical data. "+
								"Increase pg_stat_statements.max or call pg_stat_statements_reset() to reclaim slots.",
							val),
						DocURL: "https://www.postgresql.org/docs/current/pgstatstatements.html",
					}
				}
				return nil
			},
		},
		{
			ID:       "rem_wraparound_warn",
			Priority: PrioritySuggestion,
			Category: CategoryMaintenance,
			Evaluate: func(ctx EvalContext) *RuleResult {
				var val float64
				var ok bool
				if ctx.MetricKey == "pg.server.wraparound_pct" {
					val = ctx.Value
					ok = true
				} else {
					val, ok = ctx.Snapshot.Get("pg.server.wraparound_pct")
				}
				if !ok {
					return nil
				}
				if val > 20 && val <= 50 {
					return &RuleResult{
						Title: "Transaction wraparound approaching",
						Description: fmt.Sprintf(
							"Transaction ID wraparound is at %.1f%%. "+
								"Ensure autovacuum is running and not blocked by long transactions. "+
								"Monitor pg_stat_user_tables.n_dead_tup for tables with high dead tuple counts.",
							val),
						DocURL: "https://www.postgresql.org/docs/current/routine-vacuuming.html#VACUUM-FOR-WRAPAROUND",
					}
				}
				return nil
			},
		},
		{
			ID:       "rem_wraparound_crit",
			Priority: PriorityActionRequired,
			Category: CategoryMaintenance,
			Evaluate: func(ctx EvalContext) *RuleResult {
				var val float64
				var ok bool
				if ctx.MetricKey == "pg.server.wraparound_pct" {
					val = ctx.Value
					ok = true
				} else {
					val, ok = ctx.Snapshot.Get("pg.server.wraparound_pct")
				}
				if !ok {
					return nil
				}
				if val > 50 {
					return &RuleResult{
						Title: "Wraparound imminent — vacuum urgently",
						Description: fmt.Sprintf(
							"Transaction ID wraparound is at %.1f%%, critically high. "+
								"PostgreSQL will shut down to prevent data corruption if this reaches 100%%. "+
								"Run VACUUM FREEZE on the most affected databases immediately.",
							val),
						DocURL: "https://www.postgresql.org/docs/current/routine-vacuuming.html#VACUUM-FOR-WRAPAROUND",
					}
				}
				return nil
			},
		},
		{
			ID:       "rem_track_io",
			Priority: PriorityInfo,
			Category: CategoryConfiguration,
			Evaluate: func(ctx EvalContext) *RuleResult {
				var val float64
				var ok bool
				if ctx.MetricKey == "pg.settings.track_io_timing" {
					val = ctx.Value
					ok = true
				} else {
					val, ok = ctx.Snapshot.Get("pg.settings.track_io_timing")
				}
				if !ok {
					return nil
				}
				if val == 0 {
					return &RuleResult{
						Title: "Enable track_io_timing for I/O analysis",
						Description: "track_io_timing is disabled. Without it, EXPLAIN ANALYZE cannot show I/O timing " +
							"and pg_stat_statements lacks I/O cost data. Enable it with ALTER SYSTEM SET track_io_timing = on; " +
							"SELECT pg_reload_conf(); — overhead is minimal on modern systems.",
						DocURL: "https://www.postgresql.org/docs/current/runtime-config-statistics.html#GUC-TRACK-IO-TIMING",
					}
				}
				return nil
			},
		},
		{
			ID:       "rem_deadlocks",
			Priority: PrioritySuggestion,
			Category: CategoryPerformance,
			Evaluate: func(ctx EvalContext) *RuleResult {
				var val float64
				var ok bool
				if ctx.MetricKey == "pg.transactions.deadlocks" {
					val = ctx.Value
					ok = true
				} else {
					val, ok = ctx.Snapshot.Get("pg.transactions.deadlocks")
				}
				if !ok {
					return nil
				}
				if val > 0 {
					return &RuleResult{
						Title: "Deadlocks occurring",
						Description: fmt.Sprintf(
							"%.0f deadlock(s) detected. Deadlocks indicate competing lock acquisition orders "+
								"in concurrent transactions. Review application logic to ensure consistent lock ordering. "+
								"Check pg_stat_database.deadlocks for per-database breakdown.",
							val),
						DocURL: "https://www.postgresql.org/docs/current/explicit-locking.html#LOCKING-DEADLOCKS",
					}
				}
				return nil
			},
		},
		{
			ID:       "rem_bloat_high",
			Priority: PrioritySuggestion,
			Category: CategoryMaintenance,
			Evaluate: func(ctx EvalContext) *RuleResult {
				var val float64
				var ok bool
				if ctx.MetricKey == "pg.db.bloat.ratio" {
					val = ctx.Value
					ok = true
				} else {
					val, ok = ctx.Snapshot.Get("pg.db.bloat.ratio")
				}
				if !ok {
					return nil
				}
				if val > 2 && val <= 50 {
					return &RuleResult{
						Title: "Table bloat exceeding threshold",
						Description: fmt.Sprintf(
							"Table bloat ratio is %.1fx. Dead tuples are accumulating faster than autovacuum can clean them. "+
								"Check autovacuum settings (autovacuum_vacuum_scale_factor, autovacuum_naptime). "+
								"Consider running VACUUM FULL or pg_repack on heavily bloated tables.",
							val),
						DocURL: "https://www.postgresql.org/docs/current/routine-vacuuming.html",
					}
				}
				return nil
			},
		},
		{
			ID:       "rem_bloat_extreme",
			Priority: PriorityActionRequired,
			Category: CategoryMaintenance,
			Evaluate: func(ctx EvalContext) *RuleResult {
				var val float64
				var ok bool
				if ctx.MetricKey == "pg.db.bloat.ratio" {
					val = ctx.Value
					ok = true
				} else {
					val, ok = ctx.Snapshot.Get("pg.db.bloat.ratio")
				}
				if !ok {
					return nil
				}
				if val > 50 {
					return &RuleResult{
						Title: "Severe bloat — schedule pg_repack",
						Description: fmt.Sprintf(
							"Table bloat ratio is %.1fx, severely impacting storage and query performance. "+
								"VACUUM FULL requires an exclusive lock; prefer pg_repack for online compaction. "+
								"Schedule maintenance during a low-traffic window.",
							val),
						DocURL: "https://reorg.github.io/pg_repack/",
					}
				}
				return nil
			},
		},
	}
}
