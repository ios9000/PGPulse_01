package playbook

func autovacuumFailingPlaybook() Playbook {
	return Playbook{
		Slug:        "autovacuum-failing",
		Name:        "Autovacuum Health Check",
		Description: "Diagnoses autovacuum effectiveness including worker status, dead tuple accumulation, and configuration issues.",
		Version:     1,
		Status:      "stable",
		Category:    "vacuum",
		TriggerBindings: TriggerBindings{
			Hooks:        []string{"remediation.vacuum_cost_settings"},
			RootCauses:   []string{"root_cause.dead_tuple_accumulation"},
			Metrics:      []string{"pg.vacuum.dead_tuple_ratio"},
			AdviserRules: []string{"rem_bloat_high"},
		},
		EstimatedDurationMin: intPtr(10),
		RequiresPermission:   "view_all",
		Author:               "pgpulse",
		IsBuiltin:            true,
		Steps: []Step{
			{
				StepOrder:      1,
				Name:           "Check autovacuum worker status",
				Description:    "See how many autovacuum workers are currently running.",
				SQLTemplate:    `SELECT count(*) AS active_workers, (SELECT setting::int FROM pg_settings WHERE name='autovacuum_max_workers') AS max_workers, count(*) FILTER (WHERE query LIKE 'autovacuum:%wraparound%') AS wraparound_workers FROM pg_stat_activity WHERE backend_type = 'autovacuum worker'`,
				SafetyTier:     TierDiagnostic,
				TimeoutSeconds: 5,
				ResultInterpretation: InterpretationSpec{
					Rules: []InterpretationRule{
						{Column: "wraparound_workers", Operator: ">", Value: 0, Verdict: "red",
							Message: "{{wraparound_workers}} wraparound vacuum workers active — urgent wraparound prevention"},
						{Column: "active_workers", Operator: "==", Value: 0, Verdict: "yellow",
							Message: "No autovacuum workers currently active — check if autovacuum is enabled"},
					},
					DefaultVerdict: "green",
					DefaultMessage: "{{active_workers}} of {{max_workers}} autovacuum workers active",
				},
				NextStepDefault: intPtr(2),
			},
			{
				StepOrder:      2,
				Name:           "Check dead tuple accumulation",
				Description:    "Identify tables with excessive dead tuples that vacuum is not cleaning.",
				SQLTemplate:    `SELECT count(*) AS problem_tables, sum(n_dead_tup) AS total_dead, max(n_dead_tup) AS max_dead, max(round(100.0 * n_dead_tup / NULLIF(n_live_tup + n_dead_tup, 0), 1)) AS max_dead_pct FROM pg_stat_user_tables WHERE n_dead_tup > 10000`,
				SafetyTier:     TierDiagnostic,
				TimeoutSeconds: 10,
				ResultInterpretation: InterpretationSpec{
					Rules: []InterpretationRule{
						{Column: "max_dead_pct", Operator: ">", Value: 30, Verdict: "red",
							Message: "{{problem_tables}} tables with high dead tuple ratio (max {{max_dead_pct}}%). Total dead: {{total_dead}}"},
						{Column: "total_dead", Operator: ">", Value: 1000000, Verdict: "yellow",
							Message: "{{total_dead}} total dead tuples across {{problem_tables}} tables"},
					},
					DefaultVerdict: "green",
					DefaultMessage: "Dead tuple levels are manageable",
				},
				NextStepDefault: intPtr(3),
			},
			{
				StepOrder:      3,
				Name:           "Check last vacuum times",
				Description:    "Find tables that haven't been vacuumed recently.",
				SQLTemplate:    `SELECT count(*) AS never_vacuumed, count(*) FILTER (WHERE last_autovacuum < now() - interval '1 day' OR last_autovacuum IS NULL) AS stale_vacuum FROM pg_stat_user_tables WHERE n_dead_tup > 1000`,
				SafetyTier:     TierDiagnostic,
				TimeoutSeconds: 10,
				ResultInterpretation: InterpretationSpec{
					Rules: []InterpretationRule{
						{Column: "stale_vacuum", Operator: ">", Value: 5, Verdict: "yellow",
							Message: "{{stale_vacuum}} tables with dead tuples haven't been vacuumed in >1 day"},
					},
					DefaultVerdict: "green",
					DefaultMessage: "Vacuum timing looks healthy",
				},
				NextStepDefault: intPtr(4),
			},
			{
				StepOrder:      4,
				Name:           "Check autovacuum configuration",
				Description:    "Verify autovacuum settings to ensure they are appropriately tuned.",
				SQLTemplate:    `SELECT max(CASE WHEN name='autovacuum' THEN setting END) AS autovacuum, max(CASE WHEN name='autovacuum_max_workers' THEN setting END) AS max_workers, max(CASE WHEN name='autovacuum_vacuum_cost_delay' THEN setting END) AS cost_delay, max(CASE WHEN name='autovacuum_vacuum_cost_limit' THEN setting END) AS cost_limit, max(CASE WHEN name='autovacuum_vacuum_threshold' THEN setting END) AS threshold, max(CASE WHEN name='autovacuum_vacuum_scale_factor' THEN setting END) AS scale_factor FROM pg_settings WHERE name LIKE 'autovacuum%'`,
				SafetyTier:     TierDiagnostic,
				TimeoutSeconds: 5,
				ResultInterpretation: InterpretationSpec{
					DefaultVerdict: "yellow",
					DefaultMessage: "Review autovacuum settings. Consider lowering vacuum_cost_delay and increasing cost_limit for faster cleanup.",
				},
				NextStepDefault: intPtr(5),
			},
			{
				StepOrder:          5,
				Name:               "Remediation: Vacuum tuning",
				Description:        "Autovacuum issues require configuration tuning and potentially manual VACUUM.",
				SafetyTier:         TierExternal,
				ManualInstructions: "1. If autovacuum is OFF, enable it immediately: ALTER SYSTEM SET autovacuum = on;\n2. Reduce autovacuum_vacuum_cost_delay (e.g., 2ms) for faster cleanup.\n3. Increase autovacuum_vacuum_cost_limit (e.g., 2000) to allow more work per cycle.\n4. For large tables, set per-table autovacuum_vacuum_scale_factor lower (e.g., 0.01).\n5. Run manual VACUUM on the most bloated tables.\n6. For emergency bloat: VACUUM FULL (requires exclusive lock — schedule during maintenance window).\n7. Apply changes: SELECT pg_reload_conf();",
				EscalationContact:  "DBA",
			},
		},
	}
}
