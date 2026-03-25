package playbook

func diskFullPlaybook() Playbook {
	return Playbook{
		Slug:        "disk-full",
		Name:        "Disk Space Emergency",
		Description: "Emergency playbook for disk space exhaustion — identifies WAL, bloat, temp files, and tablespace usage.",
		Version:     1,
		Status:      "stable",
		Category:    "storage",
		TriggerBindings: TriggerBindings{
			Hooks:      []string{"remediation.disk_capacity"},
			RootCauses: []string{"root_cause.wal_accumulation"},
			Metrics:    []string{"pg.server.db_size_bytes"},
		},
		EstimatedDurationMin: intPtr(15),
		RequiresPermission:   "view_all",
		Author:               "pgpulse",
		IsBuiltin:            true,
		Steps: []Step{
			{
				StepOrder:      1,
				Name:           "Check database sizes",
				Description:    "Identify which databases are consuming the most disk space.",
				SQLTemplate:    `SELECT count(*) AS db_count, pg_size_pretty(sum(pg_database_size(datname))) AS total_size, max(pg_database_size(datname)) AS max_db_bytes FROM pg_database WHERE NOT datistemplate`,
				SafetyTier:     TierDiagnostic,
				TimeoutSeconds: 15,
				ResultInterpretation: InterpretationSpec{
					DefaultVerdict: "yellow",
					DefaultMessage: "Total database size: {{total_size}} across {{db_count}} databases",
				},
				NextStepDefault: intPtr(2),
			},
			{
				StepOrder:      2,
				Name:           "Check table bloat",
				Description:    "Identify tables with significant dead tuple bloat.",
				SQLTemplate:    `SELECT count(*) AS bloated_tables, sum(n_dead_tup) AS total_dead_tuples, max(round(100.0 * n_dead_tup / NULLIF(n_live_tup + n_dead_tup, 0), 1)) AS max_dead_pct FROM pg_stat_user_tables WHERE n_dead_tup > 10000`,
				SafetyTier:     TierDiagnostic,
				TimeoutSeconds: 10,
				ResultInterpretation: InterpretationSpec{
					Rules: []InterpretationRule{
						{Column: "total_dead_tuples", Operator: ">", Value: 10000000, Verdict: "red",
							Message: "{{bloated_tables}} bloated tables with {{total_dead_tuples}} dead tuples (max {{max_dead_pct}}% dead)"},
					},
					DefaultVerdict: "yellow",
					DefaultMessage: "Bloat levels: {{bloated_tables}} tables with >10K dead tuples",
				},
				NextStepDefault: intPtr(3),
			},
			{
				StepOrder:      3,
				Name:           "Check WAL accumulation",
				Description:    "WAL files can consume significant disk space during archiving delays or replication issues.",
				SQLTemplate:    `SELECT count(*) AS wal_file_count, pg_size_pretty(sum(size)) AS total_wal_size FROM pg_ls_waldir()`,
				SafetyTier:     TierDiagnostic,
				TimeoutSeconds: 10,
				ResultInterpretation: InterpretationSpec{
					Rules: []InterpretationRule{
						{Column: "wal_file_count", Operator: ">", Value: 200, Verdict: "red",
							Message: "{{wal_file_count}} WAL files ({{total_wal_size}}) — major disk consumer"},
						{Column: "wal_file_count", Operator: ">", Value: 50, Verdict: "yellow",
							Message: "{{wal_file_count}} WAL files ({{total_wal_size}}) — above normal"},
					},
					DefaultVerdict: "green",
					DefaultMessage: "WAL file count is normal",
				},
				NextStepDefault: intPtr(4),
			},
			{
				StepOrder:      4,
				Name:           "Check temp file usage",
				Description:    "Large temp files from sorts/hashes can consume significant disk space.",
				SQLTemplate:    `SELECT sum(temp_files) AS total_temp_files, pg_size_pretty(sum(temp_bytes)) AS total_temp_size FROM pg_stat_database`,
				SafetyTier:     TierDiagnostic,
				TimeoutSeconds: 5,
				ResultInterpretation: InterpretationSpec{
					DefaultVerdict: "yellow",
					DefaultMessage: "Temp file usage: {{total_temp_files}} files ({{total_temp_size}} total since stats reset)",
				},
				NextStepDefault: intPtr(5),
			},
			{
				StepOrder:          5,
				Name:               "Emergency: Free disk space",
				Description:        "Disk space exhaustion requires immediate action to prevent PostgreSQL shutdown.",
				SafetyTier:         TierExternal,
				ManualInstructions: "IMMEDIATE ACTIONS:\n1. Remove old WAL files if archive lag: pg_archivecleanup /path/to/wal last_safe_wal\n2. Drop inactive replication slots: SELECT pg_drop_replication_slot('slot_name');\n3. VACUUM FULL on most bloated tables (requires exclusive lock!).\n4. Truncate/archive old log files.\n5. Clear temp files in $PGDATA/base/pgsql_tmp/.\n\nLONGER TERM:\n6. Increase disk capacity.\n7. Configure temp_file_limit to prevent temp file explosion.\n8. Schedule regular VACUUM.\n9. Enable wal_compression.",
				EscalationContact:  "DBA / Infrastructure team / Storage admin",
			},
		},
	}
}
