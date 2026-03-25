package playbook

func lockContentionPlaybook() Playbook {
	return Playbook{
		Slug:        "lock-contention",
		Name:        "Lock Contention Analysis",
		Description: "Identifies blocking locks, long-held locks, and potential deadlock candidates to resolve lock contention.",
		Version:     1,
		Status:      "stable",
		Category:    "locks",
		TriggerBindings: TriggerBindings{
			Hooks:        []string{"remediation.lock_investigation"},
			RootCauses:   []string{"root_cause.long_tx_blocking_vacuum"},
			Metrics:      []string{"pg.locks.blocked_count"},
			AdviserRules: []string{"rem_locks_blocking"},
		},
		EstimatedDurationMin: intPtr(10),
		RequiresPermission:   "view_all",
		Author:               "pgpulse",
		IsBuiltin:            true,
		Steps: []Step{
			{
				StepOrder:      1,
				Name:           "Check blocking lock tree",
				Description:    "Identify sessions that are blocking other sessions.",
				SQLTemplate:    `SELECT count(*) AS blocked_count, count(DISTINCT blocking.pid) AS blocker_count, max(extract(epoch FROM now() - blocked_activity.xact_start)) AS max_blocked_age_sec FROM pg_locks blocked JOIN pg_locks blocking ON blocking.locktype = blocked.locktype AND blocking.database IS NOT DISTINCT FROM blocked.database AND blocking.relation IS NOT DISTINCT FROM blocked.relation AND blocking.page IS NOT DISTINCT FROM blocked.page AND blocking.tuple IS NOT DISTINCT FROM blocked.tuple AND blocking.transactionid IS NOT DISTINCT FROM blocked.transactionid AND blocking.classid IS NOT DISTINCT FROM blocked.classid AND blocking.objid IS NOT DISTINCT FROM blocked.objid AND blocking.objsubid IS NOT DISTINCT FROM blocked.objsubid AND blocking.pid != blocked.pid JOIN pg_stat_activity blocked_activity ON blocked_activity.pid = blocked.pid JOIN pg_stat_activity blocking_activity ON blocking_activity.pid = blocking.pid WHERE NOT blocked.granted AND blocking.granted`,
				SafetyTier:     TierDiagnostic,
				TimeoutSeconds: 5,
				ResultInterpretation: InterpretationSpec{
					Rules: []InterpretationRule{
						{Column: "blocked_count", Operator: ">", Value: 10, Verdict: "red",
							Message: "CRITICAL: {{blocked_count}} blocked sessions by {{blocker_count}} blockers (max age: {{max_blocked_age_sec}}s)"},
						{Column: "blocked_count", Operator: ">", Value: 0, Verdict: "yellow",
							Message: "{{blocked_count}} blocked sessions found — investigating"},
						{Column: "blocked_count", Operator: "==", Value: 0, Verdict: "green",
							Message: "No blocking locks detected"},
					},
					DefaultVerdict: "green",
					DefaultMessage: "No lock contention found",
				},
				BranchRules: []BranchRule{
					{Condition: BranchCondition{Verdict: "green"},
						GotoStep: 5, Reason: "No contention — skip to verification"},
				},
				NextStepDefault: intPtr(2),
			},
			{
				StepOrder:      2,
				Name:           "Check long-held locks",
				Description:    "Find sessions holding locks for an extended time.",
				SQLTemplate:    `SELECT count(*) AS long_lock_count, max(extract(epoch FROM now() - xact_start)) AS max_tx_age_sec FROM pg_stat_activity WHERE state != 'idle' AND xact_start < now() - interval '5 minutes' AND pid != pg_backend_pid()`,
				SafetyTier:     TierDiagnostic,
				TimeoutSeconds: 5,
				ResultInterpretation: InterpretationSpec{
					Rules: []InterpretationRule{
						{Column: "long_lock_count", Operator: ">", Value: 0, Verdict: "yellow",
							Message: "{{long_lock_count}} long-running transactions (max age: {{max_tx_age_sec}}s) — these may be holding locks"},
					},
					DefaultVerdict: "green",
					DefaultMessage: "No long-running transactions holding locks",
				},
				NextStepDefault: intPtr(3),
			},
			{
				StepOrder:      3,
				Name:           "Check lock types distribution",
				Description:    "Analyze the types of locks currently held.",
				SQLTemplate:    `SELECT count(*) AS total_locks, count(*) FILTER (WHERE NOT granted) AS waiting_locks, count(*) FILTER (WHERE locktype = 'relation') AS relation_locks, count(*) FILTER (WHERE locktype = 'transactionid') AS txid_locks FROM pg_locks WHERE pid != pg_backend_pid()`,
				SafetyTier:     TierDiagnostic,
				TimeoutSeconds: 5,
				ResultInterpretation: InterpretationSpec{
					Rules: []InterpretationRule{
						{Column: "waiting_locks", Operator: ">", Value: 20, Verdict: "red",
							Message: "{{waiting_locks}} locks in waiting state — severe contention"},
					},
					DefaultVerdict: "yellow",
					DefaultMessage: "Review lock distribution: {{total_locks}} total, {{waiting_locks}} waiting",
				},
				NextStepDefault: intPtr(4),
			},
			{
				StepOrder:      4,
				Name:           "Terminate blocking sessions",
				Description:    "If blocking sessions are identified and causing cascading issues, terminate the blocker.",
				SQLTemplate:    `SELECT count(*) AS candidate_count FROM pg_stat_activity WHERE state = 'idle in transaction' AND xact_start < now() - interval '10 minutes' AND pid != pg_backend_pid()`,
				SafetyTier:     TierDangerous,
				TimeoutSeconds: 5,
				ResultInterpretation: InterpretationSpec{
					DefaultVerdict: "yellow",
					DefaultMessage: "Review candidate sessions before termination. Use pg_terminate_backend(pid) on the specific blocker PID.",
				},
				NextStepDefault: intPtr(5),
			},
			{
				StepOrder:          5,
				Name:               "Escalation: Application lock review",
				Description:        "Persistent lock contention requires application-level investigation.",
				SafetyTier:         TierExternal,
				ManualInstructions: "1. Review application code for long-held transactions.\n2. Check for missing COMMIT/ROLLBACK in application error paths.\n3. Consider advisory locks instead of table locks where appropriate.\n4. Set lock_timeout at the application level to prevent indefinite waits.\n5. Add indexes to reduce lock duration on UPDATE/DELETE operations.",
				EscalationContact:  "DBA / Application development team",
			},
		},
	}
}
