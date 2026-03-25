package playbook

func connectionSaturationPlaybook() Playbook {
	return Playbook{
		Slug:        "connection-saturation",
		Name:        "Connection Saturation",
		Description: "Diagnoses connection pool exhaustion, identifies idle-in-transaction sessions, and guides connection management.",
		Version:     1,
		Status:      "stable",
		Category:    "connections",
		TriggerBindings: TriggerBindings{
			Hooks:        []string{"remediation.connection_pooling"},
			RootCauses:   []string{"root_cause.connection_spike"},
			Metrics:      []string{"pg.connections.used_pct"},
			AdviserRules: []string{"rem_conn_high"},
		},
		EstimatedDurationMin: intPtr(10),
		RequiresPermission:   "view_all",
		Author:               "pgpulse",
		IsBuiltin:            true,
		Steps: []Step{
			{
				StepOrder:      1,
				Name:           "Check connection utilization",
				Description:    "Compare current connections against max_connections setting.",
				SQLTemplate:    `SELECT count(*) AS total_connections, (SELECT setting::int FROM pg_settings WHERE name='max_connections') AS max_connections, round(100.0 * count(*) / (SELECT setting::int FROM pg_settings WHERE name='max_connections'), 1) AS used_pct FROM pg_stat_activity WHERE pid != pg_backend_pid()`,
				SafetyTier:     TierDiagnostic,
				TimeoutSeconds: 5,
				ResultInterpretation: InterpretationSpec{
					Rules: []InterpretationRule{
						{Column: "used_pct", Operator: ">", Value: 90, Verdict: "red",
							Message: "CRITICAL: {{used_pct}}% connections used ({{total_connections}}/{{max_connections}})"},
						{Column: "used_pct", Operator: ">", Value: 70, Verdict: "yellow",
							Message: "WARNING: {{used_pct}}% connections used ({{total_connections}}/{{max_connections}})"},
					},
					DefaultVerdict: "green",
					DefaultMessage: "Connection utilization is within normal bounds",
				},
				NextStepDefault: intPtr(2),
			},
			{
				StepOrder:      2,
				Name:           "Check connections by state",
				Description:    "Break down connections by state to identify idle-in-transaction bloat.",
				SQLTemplate:    `SELECT count(*) AS total, count(*) FILTER (WHERE state='active') AS active, count(*) FILTER (WHERE state='idle') AS idle, count(*) FILTER (WHERE state='idle in transaction') AS idle_in_transaction, max(extract(epoch FROM now()-state_change)) FILTER (WHERE state='idle in transaction') AS max_idle_tx_sec FROM pg_stat_activity WHERE pid != pg_backend_pid()`,
				SafetyTier:     TierDiagnostic,
				TimeoutSeconds: 5,
				ResultInterpretation: InterpretationSpec{
					Rules: []InterpretationRule{
						{Column: "idle_in_transaction", Operator: ">", Value: 10, Verdict: "red",
							Message: "{{idle_in_transaction}} idle-in-transaction sessions found (max age: {{max_idle_tx_sec}}s) — these hold locks and block vacuum"},
						{Column: "idle_in_transaction", Operator: ">", Value: 3, Verdict: "yellow",
							Message: "{{idle_in_transaction}} idle-in-transaction sessions — consider idle_in_transaction_session_timeout"},
					},
					DefaultVerdict: "green",
					DefaultMessage: "Connection state distribution looks healthy",
				},
				BranchRules: []BranchRule{
					{Condition: BranchCondition{Column: "idle_in_transaction", Operator: ">", Value: 10},
						GotoStep: 3, Reason: "Excessive idle-in-transaction — check application patterns"},
				},
				NextStepDefault: intPtr(3),
			},
			{
				StepOrder:      3,
				Name:           "Check connections by application",
				Description:    "Identify which applications consume the most connections.",
				SQLTemplate:    `SELECT COALESCE(application_name, 'unknown') AS app_name, count(*) AS conn_count, count(*) FILTER (WHERE state='idle in transaction') AS idle_in_tx FROM pg_stat_activity WHERE pid != pg_backend_pid() GROUP BY application_name ORDER BY conn_count DESC LIMIT 1`,
				SafetyTier:     TierDiagnostic,
				TimeoutSeconds: 5,
				ResultInterpretation: InterpretationSpec{
					DefaultVerdict: "yellow",
					DefaultMessage: "Review the top connection consumers and their idle-in-transaction counts",
				},
				NextStepDefault: intPtr(4),
			},
			{
				StepOrder:      4,
				Name:           "Check connection pool settings",
				Description:    "Verify connection-related PostgreSQL settings.",
				SQLTemplate:    `SELECT max(CASE WHEN name='max_connections' THEN setting END) AS max_connections, max(CASE WHEN name='superuser_reserved_connections' THEN setting END) AS reserved, max(CASE WHEN name='idle_in_transaction_session_timeout' THEN setting END) AS idle_tx_timeout FROM pg_settings WHERE name IN ('max_connections', 'superuser_reserved_connections', 'idle_in_transaction_session_timeout')`,
				SafetyTier:     TierDiagnostic,
				TimeoutSeconds: 5,
				ResultInterpretation: InterpretationSpec{
					DefaultVerdict: "yellow",
					DefaultMessage: "Review settings. Consider setting idle_in_transaction_session_timeout if it is 0.",
				},
				NextStepDefault: intPtr(5),
			},
			{
				StepOrder:          5,
				Name:               "Remediation: Connection management",
				Description:        "Connection saturation typically requires application-level changes or a connection pooler.",
				SafetyTier:         TierExternal,
				ManualInstructions: "1. If idle_in_transaction_session_timeout is 0, set it to 300000 (5 min) to auto-kill stale transactions.\n2. Deploy or tune a connection pooler (PgBouncer recommended).\n3. Review application connection pool settings — ensure connections are returned promptly.\n4. Consider increasing max_connections only as a last resort (higher memory per connection).\n5. Terminate idle-in-transaction sessions if urgent: SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE state = 'idle in transaction' AND state_change < now() - interval '10 minutes';",
				EscalationContact:  "DBA / Application team",
			},
		},
	}
}
