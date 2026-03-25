package playbook

func replicationLagPlaybook() Playbook {
	return Playbook{
		Slug:        "replication-lag",
		Name:        "Replication Lag Investigation",
		Description: "Investigates replication lag causes including WAL sender issues, slot retention, and network problems.",
		Version:     1,
		Status:      "stable",
		Category:    "replication",
		TriggerBindings: TriggerBindings{
			Hooks:      []string{"remediation.replication_lag"},
			RootCauses: []string{"root_cause.network_issue", "root_cause.inactive_replication_slot"},
			Metrics:    []string{"pg.replication.lag_bytes"},
		},
		EstimatedDurationMin: intPtr(8),
		RequiresPermission:   "view_all",
		Author:               "pgpulse",
		IsBuiltin:            true,
		Steps: []Step{
			{
				StepOrder:      1,
				Name:           "Check replication status",
				Description:    "Query pg_stat_replication to see lag for each replica.",
				SQLTemplate:    `SELECT count(*) AS replica_count, max(pg_wal_lsn_diff(pg_current_wal_insert_lsn(), flush_lsn)) AS max_lag_bytes, max(extract(epoch FROM replay_lag)) AS max_replay_lag_sec FROM pg_stat_replication`,
				SafetyTier:     TierDiagnostic,
				TimeoutSeconds: 5,
				ResultInterpretation: InterpretationSpec{
					Rules: []InterpretationRule{
						{Column: "max_lag_bytes", Operator: ">", Value: 1073741824, Verdict: "red",
							Message: "CRITICAL: Max replication lag is {{max_lag_bytes}} bytes (>1GB)"},
						{Column: "max_lag_bytes", Operator: ">", Value: 104857600, Verdict: "yellow",
							Message: "WARNING: Max replication lag is {{max_lag_bytes}} bytes (>100MB)"},
						{Column: "replica_count", Operator: "==", Value: 0, Verdict: "yellow",
							Message: "No replicas connected to this primary"},
					},
					DefaultVerdict: "green",
					DefaultMessage: "Replication lag is within normal bounds",
				},
				BranchRules: []BranchRule{
					{Condition: BranchCondition{Verdict: "red"},
						GotoStep: 2, Reason: "High lag — investigate replication slots"},
				},
				NextStepDefault: intPtr(2),
			},
			{
				StepOrder:      2,
				Name:           "Check replication slots",
				Description:    "Identify inactive or lagging replication slots that may be retaining WAL.",
				SQLTemplate:    `SELECT count(*) AS total_slots, count(*) FILTER (WHERE NOT active) AS inactive_slots, max(CASE WHEN NOT active THEN pg_wal_lsn_diff(pg_current_wal_insert_lsn(), restart_lsn) END) AS max_inactive_lag FROM pg_replication_slots`,
				SafetyTier:     TierDiagnostic,
				TimeoutSeconds: 5,
				ResultInterpretation: InterpretationSpec{
					Rules: []InterpretationRule{
						{Column: "inactive_slots", Operator: ">", Value: 0, Verdict: "red",
							Message: "{{inactive_slots}} inactive replication slot(s) found — these retain WAL and cause disk growth"},
					},
					DefaultVerdict: "green",
					DefaultMessage: "All replication slots are active",
				},
				NextStepDefault: intPtr(3),
			},
			{
				StepOrder:      3,
				Name:           "Check WAL accumulation from lag",
				Description:    "Measure WAL directory size to assess disk pressure from replication lag.",
				SQLTemplate:    `SELECT count(*) AS wal_file_count, pg_size_pretty(sum(size)) AS total_wal_size FROM pg_ls_waldir()`,
				SafetyTier:     TierDiagnostic,
				TimeoutSeconds: 10,
				ResultInterpretation: InterpretationSpec{
					Rules: []InterpretationRule{
						{Column: "wal_file_count", Operator: ">", Value: 200, Verdict: "red",
							Message: "{{wal_file_count}} WAL files retained ({{total_wal_size}}) — disk exhaustion risk"},
						{Column: "wal_file_count", Operator: ">", Value: 50, Verdict: "yellow",
							Message: "{{wal_file_count}} WAL files — above normal but manageable"},
					},
					DefaultVerdict: "green",
					DefaultMessage: "WAL file count is normal",
				},
				NextStepDefault: intPtr(4),
			},
			{
				StepOrder:          4,
				Name:               "Escalation: Network / replica investigation",
				Description:        "Replication lag may be caused by network issues, replica load, or storage problems on the replica.",
				SafetyTier:         TierExternal,
				ManualInstructions: "1. Check network latency between primary and replica hosts.\n2. Verify replica is running and accepting connections.\n3. Check replica's I/O subsystem for saturation.\n4. If inactive slots exist, consider dropping them: SELECT pg_drop_replication_slot('slot_name');\n5. Monitor pg_stat_replication for improvement.",
				EscalationContact:  "DBA / Infrastructure team",
			},
		},
	}
}
