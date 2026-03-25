package playbook

func wraparoundRiskPlaybook() Playbook {
	return Playbook{
		Slug:        "wraparound-risk",
		Name:        "Transaction Wraparound Risk",
		Description: "Diagnoses transaction ID wraparound risk by checking datfrozenxid age and per-table freeze status.",
		Version:     1,
		Status:      "stable",
		Category:    "vacuum",
		TriggerBindings: TriggerBindings{
			Hooks:        []string{"remediation.wraparound_vacuum"},
			RootCauses:   []string{"root_cause.wraparound_approaching"},
			Metrics:      []string{"pg.server.oldest_xid_age"},
			AdviserRules: []string{"rem_wraparound_crit"},
		},
		EstimatedDurationMin: intPtr(10),
		RequiresPermission:   "view_all",
		Author:               "pgpulse",
		IsBuiltin:            true,
		Steps: []Step{
			{
				StepOrder:      1,
				Name:           "Check database transaction age",
				Description:    "Check the age of the oldest unfrozen transaction ID across all databases.",
				SQLTemplate:    `SELECT max(age(datfrozenxid)) AS max_xid_age, max(age(datfrozenxid))::float / 2147483647 * 100 AS pct_to_wraparound, count(*) FILTER (WHERE age(datfrozenxid) > 1000000000) AS critical_dbs FROM pg_database WHERE NOT datistemplate`,
				SafetyTier:     TierDiagnostic,
				TimeoutSeconds: 5,
				ResultInterpretation: InterpretationSpec{
					Rules: []InterpretationRule{
						{Column: "pct_to_wraparound", Operator: ">", Value: 75, Verdict: "red",
							Message: "CRITICAL: {{pct_to_wraparound}}% to wraparound (xid age: {{max_xid_age}}). Immediate action required!"},
						{Column: "pct_to_wraparound", Operator: ">", Value: 50, Verdict: "yellow",
							Message: "WARNING: {{pct_to_wraparound}}% to wraparound (xid age: {{max_xid_age}})"},
					},
					DefaultVerdict: "green",
					DefaultMessage: "Transaction age is healthy ({{pct_to_wraparound}}% to wraparound)",
				},
				BranchRules: []BranchRule{
					{Condition: BranchCondition{Verdict: "green"},
						GotoStep: 4, Reason: "No wraparound risk — skip to verification"},
				},
				NextStepDefault: intPtr(2),
			},
			{
				StepOrder:      2,
				Name:           "Check aggressive vacuum status",
				Description:    "See if PostgreSQL has triggered aggressive (anti-wraparound) vacuum.",
				SQLTemplate:    `SELECT count(*) AS aggressive_vacuum_count FROM pg_stat_activity WHERE backend_type = 'autovacuum worker' AND query LIKE '%wraparound%'`,
				SafetyTier:     TierDiagnostic,
				TimeoutSeconds: 5,
				ResultInterpretation: InterpretationSpec{
					Rules: []InterpretationRule{
						{Column: "aggressive_vacuum_count", Operator: ">", Value: 0, Verdict: "yellow",
							Message: "{{aggressive_vacuum_count}} aggressive (anti-wraparound) vacuum workers running — system is self-healing but under pressure"},
					},
					DefaultVerdict: "yellow",
					DefaultMessage: "No aggressive vacuum workers running — manual intervention may be needed",
				},
				NextStepDefault: intPtr(3),
			},
			{
				StepOrder:      3,
				Name:           "Check per-table wraparound status",
				Description:    "Identify the oldest unfrozen tables that are driving the wraparound risk.",
				SQLTemplate:    `SELECT count(*) AS old_table_count, max(age(relfrozenxid)) AS max_table_xid_age FROM pg_class WHERE relkind = 'r' AND age(relfrozenxid) > 500000000`,
				SafetyTier:     TierDiagnostic,
				TimeoutSeconds: 10,
				ResultInterpretation: InterpretationSpec{
					Rules: []InterpretationRule{
						{Column: "old_table_count", Operator: ">", Value: 0, Verdict: "red",
							Message: "{{old_table_count}} tables with xid age >500M (max: {{max_table_xid_age}}) — these drive the wraparound risk"},
					},
					DefaultVerdict: "green",
					DefaultMessage: "No tables with critically old transaction IDs",
				},
				NextStepDefault: intPtr(4),
			},
			{
				StepOrder:          4,
				Name:               "Remediation: Wraparound prevention",
				Description:        "Transaction wraparound requires urgent vacuum and possible configuration changes.",
				SafetyTier:         TierExternal,
				ManualInstructions: "1. Run VACUUM FREEZE on the most critical tables (those with highest relfrozenxid age).\n2. If vacuum is being blocked by long transactions, terminate them first.\n3. Increase autovacuum_freeze_max_age if it is set too low (default 200M is usually fine).\n4. Ensure autovacuum is running and not being blocked.\n5. Monitor progress: SELECT relname, age(relfrozenxid) FROM pg_class WHERE relkind='r' ORDER BY age(relfrozenxid) DESC LIMIT 10;\n6. For emergency: run vacuumdb --freeze --all from the command line.",
				EscalationContact:  "DBA",
			},
		},
	}
}
