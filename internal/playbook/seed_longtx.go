package playbook

func longTransactionsPlaybook() Playbook {
	return Playbook{
		Slug:        "long-transactions",
		Name:        "Long Transaction Investigation",
		Description: "Investigates long-running transactions that cause bloat accumulation and vacuum delays.",
		Version:     1,
		Status:      "stable",
		Category:    "locks",
		TriggerBindings: TriggerBindings{
			Hooks:        []string{"remediation.kill_long_transaction"},
			RootCauses:   []string{"root_cause.long_transaction"},
			Metrics:      []string{"pg.activity.longest_tx_sec"},
			AdviserRules: []string{"rem_long_txn_warn"},
		},
		EstimatedDurationMin: intPtr(8),
		RequiresPermission:   "view_all",
		Author:               "pgpulse",
		IsBuiltin:            true,
		Steps: []Step{
			{
				StepOrder:      1,
				Name:           "Check longest running transactions",
				Description:    "Find the longest active transactions and their impact.",
				SQLTemplate:    `SELECT count(*) AS long_tx_count, max(extract(epoch FROM now() - xact_start)) AS max_age_sec, count(*) FILTER (WHERE state = 'idle in transaction') AS idle_in_tx_count FROM pg_stat_activity WHERE xact_start IS NOT NULL AND xact_start < now() - interval '5 minutes' AND pid != pg_backend_pid()`,
				SafetyTier:     TierDiagnostic,
				TimeoutSeconds: 5,
				ResultInterpretation: InterpretationSpec{
					Rules: []InterpretationRule{
						{Column: "max_age_sec", Operator: ">", Value: 3600, Verdict: "red",
							Message: "CRITICAL: Longest transaction running for {{max_age_sec}}s ({{long_tx_count}} long transactions, {{idle_in_tx_count}} idle-in-tx)"},
						{Column: "max_age_sec", Operator: ">", Value: 300, Verdict: "yellow",
							Message: "{{long_tx_count}} transactions running >5 min (max: {{max_age_sec}}s)"},
					},
					DefaultVerdict: "green",
					DefaultMessage: "No long-running transactions found",
				},
				BranchRules: []BranchRule{
					{Condition: BranchCondition{Verdict: "green"},
						GotoStep: 4, Reason: "No long transactions — skip to verification"},
				},
				NextStepDefault: intPtr(2),
			},
			{
				StepOrder:      2,
				Name:           "Check vacuum impact",
				Description:    "Long transactions prevent vacuum from reclaiming dead tuples.",
				SQLTemplate:    `SELECT sum(n_dead_tup) AS total_dead_tuples, count(*) FILTER (WHERE n_dead_tup > 10000) AS tables_with_dead_tuples, max(n_dead_tup) AS max_dead_tuples FROM pg_stat_user_tables`,
				SafetyTier:     TierDiagnostic,
				TimeoutSeconds: 10,
				ResultInterpretation: InterpretationSpec{
					Rules: []InterpretationRule{
						{Column: "total_dead_tuples", Operator: ">", Value: 10000000, Verdict: "red",
							Message: "{{total_dead_tuples}} dead tuples across all tables — severe bloat from long transactions"},
						{Column: "total_dead_tuples", Operator: ">", Value: 1000000, Verdict: "yellow",
							Message: "{{total_dead_tuples}} dead tuples accumulating — vacuum is being blocked"},
					},
					DefaultVerdict: "green",
					DefaultMessage: "Dead tuple count is manageable",
				},
				NextStepDefault: intPtr(3),
			},
			{
				StepOrder:      3,
				Name:           "Check bloat accumulation",
				Description:    "Estimate table bloat from dead tuples that vacuum cannot reclaim.",
				SQLTemplate:    `SELECT count(*) AS bloated_table_count, max(round(100.0 * n_dead_tup / NULLIF(n_live_tup + n_dead_tup, 0), 1)) AS max_dead_pct FROM pg_stat_user_tables WHERE n_dead_tup > 1000`,
				SafetyTier:     TierDiagnostic,
				TimeoutSeconds: 10,
				ResultInterpretation: InterpretationSpec{
					Rules: []InterpretationRule{
						{Column: "max_dead_pct", Operator: ">", Value: 50, Verdict: "red",
							Message: "{{bloated_table_count}} tables with significant bloat (max {{max_dead_pct}}% dead)"},
					},
					DefaultVerdict: "yellow",
					DefaultMessage: "Review bloat levels and consider VACUUM after resolving long transactions",
				},
				NextStepDefault: intPtr(4),
			},
			{
				StepOrder:          4,
				Name:               "Remediation: Transaction management",
				Description:        "Long transactions must be addressed at the application level.",
				SafetyTier:         TierExternal,
				ManualInstructions: "1. Identify the application causing long transactions from pg_stat_activity.\n2. Set idle_in_transaction_session_timeout to auto-kill idle transactions.\n3. Review application code for missing COMMIT/ROLLBACK.\n4. Consider breaking large batch operations into smaller transactions.\n5. After resolving, run VACUUM on affected tables to reclaim space.\n6. For urgent cases: pg_terminate_backend(pid) on the specific long-running PID.",
				EscalationContact:  "DBA / Application development team",
			},
		},
	}
}
