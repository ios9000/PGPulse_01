package alert

// BuiltinRules returns the default set of alert rules shipped with PGPulse.
func BuiltinRules() []Rule {
	return []Rule{
		// --- Ported from PGAM (14 rules: 7 warning/critical pairs) ---
		{
			ID: "wraparound_warning", Name: "Transaction ID Wraparound Warning",
			Description: "Transaction ID wraparound approaching dangerous levels. Consider running VACUUM FREEZE on affected databases.",
			Metric: "pg.databases.wraparound_pct", Operator: OpGreater, Threshold: 20,
			Severity: SeverityWarning, Source: SourceBuiltin, Enabled: true,
			ConsecutiveCount: 3, CooldownMinutes: 15,
		},
		{
			ID: "wraparound_critical", Name: "Transaction ID Wraparound Critical",
			Description: "Transaction ID wraparound at critical level. Immediate VACUUM FREEZE required.",
			Metric: "pg.databases.wraparound_pct", Operator: OpGreater, Threshold: 50,
			Severity: SeverityCritical, Source: SourceBuiltin, Enabled: true,
			ConsecutiveCount: 1, CooldownMinutes: 5,
		},
		{
			ID: "multixact_warning", Name: "MultiXact ID Wraparound Warning",
			Description: "MultiXact ID usage approaching dangerous levels.",
			Metric: "pg.databases.multixact_pct", Operator: OpGreater, Threshold: 20,
			Severity: SeverityWarning, Source: SourceBuiltin, Enabled: true,
			ConsecutiveCount: 3, CooldownMinutes: 15,
		},
		{
			ID: "multixact_critical", Name: "MultiXact ID Wraparound Critical",
			Description: "MultiXact ID usage at critical level. Immediate action required.",
			Metric: "pg.databases.multixact_pct", Operator: OpGreater, Threshold: 50,
			Severity: SeverityCritical, Source: SourceBuiltin, Enabled: true,
			ConsecutiveCount: 1, CooldownMinutes: 5,
		},
		{
			ID: "connections_warning", Name: "Connection Utilization Warning",
			Description: "Connection usage exceeds 80% of max_connections.",
			Metric: "pg.connections.utilization_pct", Operator: OpGreater, Threshold: 80,
			Severity: SeverityWarning, Source: SourceBuiltin, Enabled: true,
			ConsecutiveCount: 3, CooldownMinutes: 15,
		},
		{
			ID: "connections_critical", Name: "Connection Utilization Critical",
			Description: "Connection usage at or above 99% of max_connections. New connections may be refused.",
			Metric: "pg.connections.utilization_pct", Operator: OpGreaterEqual, Threshold: 99,
			Severity: SeverityCritical, Source: SourceBuiltin, Enabled: true,
			ConsecutiveCount: 1, CooldownMinutes: 5,
		},
		{
			ID: "cache_hit_warning", Name: "Cache Hit Ratio Low",
			Description: "Buffer cache hit ratio below 90%. Consider increasing shared_buffers.",
			Metric: "pg.cache.hit_ratio", Operator: OpLess, Threshold: 0.90,
			Severity: SeverityWarning, Source: SourceBuiltin, Enabled: true,
			ConsecutiveCount: 3, CooldownMinutes: 15,
		},
		{
			ID: "commit_ratio_warning", Name: "Commit Ratio Low",
			Description: "Transaction commit ratio below 95%. High rollback rate may indicate application issues.",
			Metric: "pg.transactions.commit_ratio", Operator: OpLess, Threshold: 0.95,
			Severity: SeverityWarning, Source: SourceBuiltin, Enabled: true,
			ConsecutiveCount: 3, CooldownMinutes: 15,
		},
		{
			ID: "replication_slot_inactive", Name: "Inactive Replication Slot",
			Description: "Replication slot is inactive. Inactive slots prevent WAL cleanup and cause disk growth.",
			Metric: "pg.replication.slot_active", Operator: OpEqual, Threshold: 0,
			Severity: SeverityWarning, Source: SourceBuiltin, Enabled: true,
			ConsecutiveCount: 3, CooldownMinutes: 30,
		},
		{
			ID: "long_transaction_warning", Name: "Long Running Transaction",
			Description: "Transaction running longer than 30 minutes detected.",
			Metric: "pg.transactions.longest_seconds", Operator: OpGreater, Threshold: 1800,
			Severity: SeverityWarning, Source: SourceBuiltin, Enabled: true,
			ConsecutiveCount: 3, CooldownMinutes: 30,
		},
		{
			ID: "long_transaction_critical", Name: "Very Long Running Transaction",
			Description: "Transaction running longer than 2 hours. May cause table bloat and lock contention.",
			Metric: "pg.transactions.longest_seconds", Operator: OpGreater, Threshold: 7200,
			Severity: SeverityCritical, Source: SourceBuiltin, Enabled: true,
			ConsecutiveCount: 1, CooldownMinutes: 15,
		},
		{
			ID: "table_bloat_warning", Name: "Table Bloat Warning",
			Description: "Estimated table bloat exceeds 50%. Consider running VACUUM FULL or pg_repack.",
			Metric: "pg.tables.bloat_pct", Operator: OpGreater, Threshold: 50,
			Severity: SeverityWarning, Source: SourceBuiltin, Enabled: true,
			ConsecutiveCount: 3, CooldownMinutes: 60,
		},
		{
			ID: "table_bloat_critical", Name: "Table Bloat Critical",
			Description: "Estimated table bloat exceeds 80%. Severely impacting storage and query performance.",
			Metric: "pg.tables.bloat_pct", Operator: OpGreater, Threshold: 80,
			Severity: SeverityCritical, Source: SourceBuiltin, Enabled: true,
			ConsecutiveCount: 1, CooldownMinutes: 15,
		},
		{
			ID: "pgss_dealloc_warning", Name: "pg_stat_statements Near Capacity",
			Description: "pg_stat_statements deallocation count is increasing. Consider increasing pg_stat_statements.max.",
			Metric: "pg.statements.dealloc_count", Operator: OpGreater, Threshold: 0,
			Severity: SeverityWarning, Source: SourceBuiltin, Enabled: true,
			ConsecutiveCount: 3, CooldownMinutes: 60,
		},

		// --- New rules (2 rules) ---
		{
			ID: "replication_lag_warning", Name: "Replication Lag Warning",
			Description: "Replication lag exceeds 1 MB.",
			Metric: "pg.replication.lag_bytes", Operator: OpGreater, Threshold: 1048576,
			Severity: SeverityWarning, Source: SourceBuiltin, Enabled: true,
			ConsecutiveCount: 3, CooldownMinutes: 15,
		},
		{
			ID: "replication_lag_critical", Name: "Replication Lag Critical",
			Description: "Replication lag exceeds 100 MB. Replica may be significantly behind.",
			Metric: "pg.replication.lag_bytes", Operator: OpGreater, Threshold: 104857600,
			Severity: SeverityCritical, Source: SourceBuiltin, Enabled: true,
			ConsecutiveCount: 1, CooldownMinutes: 5,
		},

		{
			ID: "logical_repl_pending_sync", Name: "Logical Replication Pending Sync",
			Description: "Logical replication tables not fully synchronized.",
			Metric: "pg.db.logical_replication.pending_sync_tables", Operator: OpGreater, Threshold: 0,
			Severity: SeverityWarning, Source: SourceBuiltin, Enabled: false,
			ConsecutiveCount: 3, CooldownMinutes: 10,
		},

		// --- Deferred rules (defined but disabled, need ML in M8) ---
		{
			ID: "wal_spike_warning", Name: "WAL Generation Spike",
			Description: "WAL generation rate exceeds 3x baseline. Requires ML baseline (M8).",
			Metric: "pg.wal.spike_ratio", Operator: OpGreater, Threshold: 3,
			Severity: SeverityWarning, Source: SourceBuiltin, Enabled: false,
			ConsecutiveCount: 3, CooldownMinutes: 30,
		},
		{
			ID: "query_regression_warning", Name: "Query Performance Regression",
			Description: "Query execution time exceeds 2x historical baseline. Requires ML baseline (M8).",
			Metric: "pg.statements.regression_ratio", Operator: OpGreater, Threshold: 2,
			Severity: SeverityWarning, Source: SourceBuiltin, Enabled: false,
			ConsecutiveCount: 3, CooldownMinutes: 30,
		},
		{
			ID: "disk_forecast_critical", Name: "Disk Space Forecast Critical",
			Description: "Disk projected to run out within 7 days based on growth trend. Requires ML forecast (M8).",
			Metric: "pg.disk.days_remaining", Operator: OpLess, Threshold: 7,
			Severity: SeverityCritical, Source: SourceBuiltin, Enabled: false,
			ConsecutiveCount: 3, CooldownMinutes: 60,
		},

		// --- ML Anomaly Detection (M8_02) ---
		{
			ID: "ml_anomaly_warning", Name: "ML Anomaly Warning",
			Description: "ML anomaly detected: Z-score exceeds warning threshold.",
			Metric: "anomaly.", Operator: OpGreater, Threshold: 3.0,
			Severity: SeverityWarning, Source: SourceBuiltin, Enabled: true,
			ConsecutiveCount: 1, CooldownMinutes: 15,
		},
		{
			ID: "ml_anomaly_critical", Name: "ML Anomaly Critical",
			Description: "Critical ML anomaly: Z-score exceeds critical threshold.",
			Metric: "anomaly.", Operator: OpGreater, Threshold: 5.0,
			Severity: SeverityCritical, Source: SourceBuiltin, Enabled: true,
			ConsecutiveCount: 1, CooldownMinutes: 5,
		},
	}
}
