package playbook

func heavyQueryPlaybook() Playbook {
	return Playbook{
		Slug:        "heavy-query",
		Name:        "Heavy Query Diagnostics",
		Description: "Identifies resource-intensive queries using pg_stat_statements and provides optimization guidance.",
		Version:     1,
		Status:      "stable",
		Category:    "performance",
		TriggerBindings: TriggerBindings{
			Hooks:      []string{"remediation.query_optimization"},
			RootCauses: []string{"root_cause.query_regression", "root_cause.missing_index"},
			Metrics:    []string{"pg.statements.total_exec_time_ms"},
		},
		EstimatedDurationMin: intPtr(10),
		RequiresPermission:   "view_all",
		Author:               "pgpulse",
		IsBuiltin:            true,
		Steps: []Step{
			{
				StepOrder:      1,
				Name:           "Check top queries by total time",
				Description:    "Identify the heaviest queries from pg_stat_statements.",
				SQLTemplate:    `SELECT count(*) AS tracked_queries, max(total_exec_time) AS max_total_exec_time_ms, sum(calls) AS total_calls FROM pg_stat_statements WHERE dbid = (SELECT oid FROM pg_database WHERE datname = current_database())`,
				SafetyTier:     TierDiagnostic,
				TimeoutSeconds: 10,
				ResultInterpretation: InterpretationSpec{
					Rules: []InterpretationRule{
						{Column: "max_total_exec_time_ms", Operator: ">", Value: 3600000, Verdict: "red",
							Message: "Heaviest query consumed {{max_total_exec_time_ms}}ms total exec time across {{total_calls}} total calls"},
					},
					DefaultVerdict: "yellow",
					DefaultMessage: "{{tracked_queries}} queries tracked. Review the top consumers.",
				},
				NextStepDefault: intPtr(2),
			},
			{
				StepOrder:      2,
				Name:           "Check queries with high mean time",
				Description:    "Find queries with the highest average execution time.",
				SQLTemplate:    `SELECT count(*) AS slow_query_count, max(mean_exec_time) AS max_mean_ms FROM pg_stat_statements WHERE mean_exec_time > 1000 AND dbid = (SELECT oid FROM pg_database WHERE datname = current_database())`,
				SafetyTier:     TierDiagnostic,
				TimeoutSeconds: 10,
				ResultInterpretation: InterpretationSpec{
					Rules: []InterpretationRule{
						{Column: "slow_query_count", Operator: ">", Value: 10, Verdict: "red",
							Message: "{{slow_query_count}} queries averaging >1s execution (max: {{max_mean_ms}}ms)"},
						{Column: "slow_query_count", Operator: ">", Value: 0, Verdict: "yellow",
							Message: "{{slow_query_count}} queries averaging >1s — candidates for optimization"},
					},
					DefaultVerdict: "green",
					DefaultMessage: "No queries with high average execution time",
				},
				NextStepDefault: intPtr(3),
			},
			{
				StepOrder:      3,
				Name:           "Check sequential scan activity",
				Description:    "Identify tables with excessive sequential scans that may benefit from indexing.",
				SQLTemplate:    `SELECT count(*) AS seq_scan_tables, max(seq_scan) AS max_seq_scans, max(seq_scan - COALESCE(idx_scan, 0)) AS max_seq_excess FROM pg_stat_user_tables WHERE seq_scan > 1000 AND COALESCE(idx_scan, 0) < seq_scan`,
				SafetyTier:     TierDiagnostic,
				TimeoutSeconds: 10,
				ResultInterpretation: InterpretationSpec{
					Rules: []InterpretationRule{
						{Column: "seq_scan_tables", Operator: ">", Value: 5, Verdict: "yellow",
							Message: "{{seq_scan_tables}} tables with more sequential scans than index scans — potential missing indexes"},
					},
					DefaultVerdict: "green",
					DefaultMessage: "Index usage looks healthy",
				},
				NextStepDefault: intPtr(4),
			},
			{
				StepOrder:          4,
				Name:               "Remediation: Query optimization",
				Description:        "Heavy queries require EXPLAIN analysis and possible index creation.",
				SafetyTier:         TierExternal,
				ManualInstructions: "1. Run EXPLAIN (ANALYZE, BUFFERS) on the top slow queries to identify bottlenecks.\n2. Look for sequential scans on large tables — add appropriate indexes.\n3. Check for missing WHERE clauses or non-selective predicates.\n4. Consider increasing work_mem for sort/hash-heavy queries.\n5. Review query plans for nested loop joins on large tables — may need index or join order hints.\n6. For write-heavy queries: check if triggers or cascading FKs are adding overhead.\n7. Reset pg_stat_statements after optimization to measure improvement: SELECT pg_stat_statements_reset();",
				EscalationContact:  "DBA / Application development team",
			},
		},
	}
}
