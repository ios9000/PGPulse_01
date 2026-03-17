package alert

// BuiltinRules returns the default set of alert rules shipped with PGPulse.
func BuiltinRules() []Rule {
	return []Rule{
		// --- Ported from PGAM (14 rules: 7 warning/critical pairs) ---
		{
			ID: "wraparound_warning", Name: "Transaction ID Wraparound Warning",
			Description: "Measures how close the oldest unfrozen transaction ID is to the 2-billion wraparound limit. " +
				"At >20% usage, autovacuum may not keep up. Run VACUUM FREEZE on the databases with the oldest XID age.",
			Metric: "pg.server.wraparound_pct", Operator: OpGreater, Threshold: 20,
			Severity: SeverityWarning, Source: SourceBuiltin, Enabled: true,
			ConsecutiveCount: 3, CooldownMinutes: 15,
		},
		{
			ID: "wraparound_critical", Name: "Transaction ID Wraparound Critical",
			Description: "Transaction ID wraparound is imminent. PostgreSQL will stop accepting writes at 100% to prevent data loss. " +
				"Run VACUUM FREEZE immediately on all affected databases and check that autovacuum workers are not blocked.",
			Metric: "pg.server.wraparound_pct", Operator: OpGreater, Threshold: 50,
			Severity: SeverityCritical, Source: SourceBuiltin, Enabled: true,
			ConsecutiveCount: 1, CooldownMinutes: 5,
		},
		{
			ID: "multixact_warning", Name: "MultiXact ID Wraparound Warning",
			Description: "Measures MultiXact ID consumption relative to the wraparound limit. " +
				"MultiXacts are used for row-level shared locks (SELECT FOR SHARE). " +
				"If usage exceeds 20%, investigate long-held shared locks and run VACUUM FREEZE.",
			Metric: "pg.server.multixact_pct", Operator: OpGreater, Threshold: 20,
			Severity: SeverityWarning, Source: SourceBuiltin, Enabled: true,
			ConsecutiveCount: 3, CooldownMinutes: 15,
		},
		{
			ID: "multixact_critical", Name: "MultiXact ID Wraparound Critical",
			Description: "MultiXact ID consumption at critical level. PostgreSQL will refuse writes if wraparound occurs. " +
				"Immediately run VACUUM FREEZE and check for long-running transactions holding shared locks.",
			Metric: "pg.server.multixact_pct", Operator: OpGreater, Threshold: 50,
			Severity: SeverityCritical, Source: SourceBuiltin, Enabled: true,
			ConsecutiveCount: 1, CooldownMinutes: 5,
		},
		{
			ID: "connections_warning", Name: "Connection Utilization Warning",
			Description: "Percentage of max_connections currently in use. At >80%, connection exhaustion risk increases. " +
				"Consider deploying PgBouncer, tuning application pool sizes, or increasing max_connections.",
			Metric: "pg.connections.utilization_pct", Operator: OpGreater, Threshold: 80,
			Severity: SeverityWarning, Source: SourceBuiltin, Enabled: true,
			ConsecutiveCount: 3, CooldownMinutes: 15,
		},
		{
			ID: "connections_critical", Name: "Connection Utilization Critical",
			Description: "Connection usage at or above 99% of max_connections. New client connections will be refused. " +
				"Immediately terminate idle sessions and investigate connection leaks in the application layer.",
			Metric: "pg.connections.utilization_pct", Operator: OpGreaterEqual, Threshold: 99,
			Severity: SeverityCritical, Source: SourceBuiltin, Enabled: true,
			ConsecutiveCount: 1, CooldownMinutes: 5,
		},
		{
			ID: "cache_hit_warning", Name: "Cache Hit Ratio Low",
			Description: "Ratio of pages served from shared_buffers vs. disk reads. Below 90% indicates excessive disk I/O. " +
				"Consider increasing shared_buffers, adding missing indexes, or investigating sequential-scan-heavy queries.",
			Metric: "pg.cache.hit_ratio", Operator: OpLess, Threshold: 0.90,
			Severity: SeverityWarning, Source: SourceBuiltin, Enabled: true,
			ConsecutiveCount: 3, CooldownMinutes: 15,
		},
		{
			ID: "commit_ratio_warning", Name: "Commit Ratio Low",
			Description: "Ratio of committed transactions to total (committed + rolled back). Below 95% signals a high rollback rate, " +
				"often caused by application errors, deadlocks, or serialization failures. Review application logs for error patterns.",
			Metric: "pg.transactions.commit_ratio_pct", Operator: OpLess, Threshold: 0.95,
			Severity: SeverityWarning, Source: SourceBuiltin, Enabled: true,
			ConsecutiveCount: 3, CooldownMinutes: 15,
		},
		{
			ID: "replication_slot_inactive", Name: "Inactive Replication Slot",
			Description: "A replication slot exists but has no active consumer. Inactive slots prevent WAL segment cleanup, " +
				"causing unbounded disk growth. Drop the slot if the subscriber is permanently removed, or restart the subscriber.",
			Metric: "pg.replication.slot.active", Operator: OpEqual, Threshold: 0,
			Severity: SeverityWarning, Source: SourceBuiltin, Enabled: true,
			ConsecutiveCount: 3, CooldownMinutes: 30,
		},
		{
			ID: "long_transaction_warning", Name: "Long Running Transaction",
			Description: "Oldest open transaction has been running for over 30 minutes. Long transactions hold back VACUUM, " +
				"cause table bloat, and may block DDL operations. Identify the session with pg_stat_activity and consider terminating it.",
			Metric: "pg.long_transactions.oldest_seconds", Operator: OpGreater, Threshold: 1800,
			Severity: SeverityWarning, Source: SourceBuiltin, Enabled: true,
			ConsecutiveCount: 3, CooldownMinutes: 30,
		},
		{
			ID: "long_transaction_critical", Name: "Very Long Running Transaction",
			Description: "A transaction has been open for over 2 hours. This causes significant table bloat, lock contention, " +
				"and replication lag on replicas. Terminate the session immediately and investigate the application for missing commits.",
			Metric: "pg.long_transactions.oldest_seconds", Operator: OpGreater, Threshold: 7200,
			Severity: SeverityCritical, Source: SourceBuiltin, Enabled: true,
			ConsecutiveCount: 1, CooldownMinutes: 15,
		},
		{
			ID: "table_bloat_warning", Name: "Table Bloat Warning",
			Description: "Estimated ratio of dead tuples to live tuples in a table. Above 50%, queries scan significantly more pages " +
				"than necessary. Run VACUUM FULL or use pg_repack for online compaction.",
			Metric: "pg.db.bloat.table_ratio", Operator: OpGreater, Threshold: 50,
			Severity: SeverityWarning, Source: SourceBuiltin, Enabled: true,
			ConsecutiveCount: 3, CooldownMinutes: 60,
		},
		{
			ID: "table_bloat_critical", Name: "Table Bloat Critical",
			Description: "Table bloat exceeds 80%, severely degrading query performance and wasting storage. " +
				"Immediate compaction required via VACUUM FULL or pg_repack. Investigate why autovacuum is not keeping up.",
			Metric: "pg.db.bloat.table_ratio", Operator: OpGreater, Threshold: 80,
			Severity: SeverityCritical, Source: SourceBuiltin, Enabled: true,
			ConsecutiveCount: 1, CooldownMinutes: 15,
		},
		{
			ID: "pgss_dealloc_warning", Name: "pg_stat_statements Near Capacity",
			Description: "Tracks how full the pg_stat_statements hash table is. Above 95%, least-used entries are evicted " +
				"and query statistics are lost. Increase pg_stat_statements.max in postgresql.conf and reload.",
			Metric: "pg.extensions.pgss_fill_pct", Operator: OpGreater, Threshold: 95,
			Severity: SeverityWarning, Source: SourceBuiltin, Enabled: true,
			ConsecutiveCount: 3, CooldownMinutes: 60,
		},

		// --- Replication lag rules ---
		{
			ID: "replication_lag_warning", Name: "Replication Lag Warning",
			Description: "Total WAL bytes not yet replayed on the replica. Above 1 MB, the replica is falling behind the primary. " +
				"Check replica I/O throughput, network latency, and whether long queries on the replica are blocking recovery.",
			Metric: "pg.replication.lag.total_bytes", Operator: OpGreater, Threshold: 1048576,
			Severity: SeverityWarning, Source: SourceBuiltin, Enabled: true,
			ConsecutiveCount: 3, CooldownMinutes: 15,
		},
		{
			ID: "replication_lag_critical", Name: "Replication Lag Critical",
			Description: "Replication lag exceeds 100 MB. The replica is significantly behind and may serve stale reads. " +
				"Investigate replica resource constraints, network issues, or long-running recovery conflicts.",
			Metric: "pg.replication.lag.total_bytes", Operator: OpGreater, Threshold: 104857600,
			Severity: SeverityCritical, Source: SourceBuiltin, Enabled: true,
			ConsecutiveCount: 1, CooldownMinutes: 5,
		},

		{
			ID: "logical_repl_pending_sync", Name: "Logical Replication Pending Sync",
			Description: "Number of tables in a logical replication subscription that have not completed initial synchronization. " +
				"Tables in pending state will not receive replicated changes until sync completes. Check pg_subscription_rel for details.",
			Metric: "pg.db.logical_replication.pending_sync_tables", Operator: OpGreater, Threshold: 0,
			Severity: SeverityWarning, Source: SourceBuiltin, Enabled: false,
			ConsecutiveCount: 3, CooldownMinutes: 10,
		},

		// --- Deferred rules (defined but disabled, need ML baseline) ---
		{
			ID: "wal_spike_warning", Name: "WAL Generation Spike",
			Description: "WAL generation rate compared to the ML-computed baseline. A 3x spike indicates an unusual write burst " +
				"(bulk load, REINDEX, or unexpected DML). Investigate active queries and check for unplanned batch jobs.",
			Metric: "pg.wal.spike_ratio", Operator: OpGreater, Threshold: 3,
			Severity: SeverityWarning, Source: SourceBuiltin, Enabled: false,
			ConsecutiveCount: 3, CooldownMinutes: 30,
		},
		{
			ID: "query_regression_warning", Name: "Query Performance Regression",
			Description: "A query's execution time exceeds 2x its ML-computed historical baseline. Often caused by plan changes, " +
				"table bloat, or statistics drift. Run EXPLAIN ANALYZE and compare with cached plans.",
			Metric: "pg.statements.regression_ratio", Operator: OpGreater, Threshold: 2,
			Severity: SeverityWarning, Source: SourceBuiltin, Enabled: false,
			ConsecutiveCount: 3, CooldownMinutes: 30,
		},
		{
			ID: "disk_forecast_critical", Name: "Disk Space Forecast Critical",
			Description: "ML-projected days until disk exhaustion based on observed growth trend. Below 7 days, " +
				"take immediate action: add storage, archive old data, or run VACUUM FULL on bloated tables.",
			Metric: "pg.disk.days_remaining", Operator: OpLess, Threshold: 7,
			Severity: SeverityCritical, Source: SourceBuiltin, Enabled: false,
			ConsecutiveCount: 3, CooldownMinutes: 60,
		},

		// --- ML Anomaly Detection (M8_02) ---
		{
			ID: "ml_anomaly_warning", Name: "ML Anomaly Warning",
			Description: "A metric's Z-score exceeds the warning threshold (3.0), indicating a statistically significant deviation " +
				"from learned behavior. Review the anomalous metric in context of recent changes or workload shifts.",
			Metric: "anomaly.", Operator: OpGreater, Threshold: 3.0,
			Severity: SeverityWarning, Source: SourceBuiltin, Enabled: true,
			ConsecutiveCount: 1, CooldownMinutes: 15,
		},
		{
			ID: "ml_anomaly_critical", Name: "ML Anomaly Critical",
			Description: "A metric's Z-score exceeds the critical threshold (5.0), indicating an extreme deviation from baseline. " +
				"This typically signals a serious issue requiring immediate investigation.",
			Metric: "anomaly.", Operator: OpGreater, Threshold: 5.0,
			Severity: SeverityCritical, Source: SourceBuiltin, Enabled: true,
			ConsecutiveCount: 1, CooldownMinutes: 5,
		},
	}
}
