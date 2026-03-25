package playbook

func checkpointStormPlaybook() Playbook {
	return Playbook{
		Slug:        "checkpoint-storm",
		Name:        "Checkpoint Storm Diagnosis",
		Description: "Diagnoses checkpoint storms causing I/O saturation and elevated WAL generation rates.",
		Version:     1,
		Status:      "stable",
		Category:    "performance",
		TriggerBindings: TriggerBindings{
			Hooks:        []string{"remediation.checkpoint_completion_target"},
			RootCauses:   []string{"root_cause.bulk_workload"},
			Metrics:      []string{"pg.checkpoint.requested_count"},
			AdviserRules: []string{"rem_wraparound_warn"},
		},
		EstimatedDurationMin: intPtr(8),
		RequiresPermission:   "view_all",
		Author:               "pgpulse",
		IsBuiltin:            true,
		Steps: []Step{
			{
				StepOrder:      1,
				Name:           "Check checkpoint activity",
				Description:    "Review checkpoint frequency and timing from pg_stat_bgwriter/checkpointer.",
				SQLTemplate:    `SELECT checkpoints_timed, checkpoints_req, buffers_checkpoint, buffers_backend, maxwritten_clean FROM pg_stat_bgwriter`,
				SafetyTier:     TierDiagnostic,
				TimeoutSeconds: 5,
				ResultInterpretation: InterpretationSpec{
					Rules: []InterpretationRule{
						{Column: "checkpoints_req", Operator: ">", Value: 100, Verdict: "red",
							Message: "{{checkpoints_req}} requested checkpoints — high checkpoint pressure"},
						{Column: "buffers_backend", Operator: ">", Value: 10000, Verdict: "yellow",
							Message: "{{buffers_backend}} backend-written buffers — bgwriter/checkpointer falling behind"},
					},
					DefaultVerdict: "green",
					DefaultMessage: "Checkpoint activity appears normal",
				},
				NextStepDefault: intPtr(2),
			},
			{
				StepOrder:      2,
				Name:           "Check WAL generation rate",
				Description:    "Measure current WAL generation to identify burst workloads.",
				SQLTemplate:    `SELECT count(*) AS wal_file_count, pg_size_pretty(sum(size)) AS total_wal_size FROM pg_ls_waldir()`,
				SafetyTier:     TierDiagnostic,
				TimeoutSeconds: 10,
				ResultInterpretation: InterpretationSpec{
					Rules: []InterpretationRule{
						{Column: "wal_file_count", Operator: ">", Value: 100, Verdict: "yellow",
							Message: "{{wal_file_count}} WAL files present ({{total_wal_size}}) — elevated WAL generation"},
					},
					DefaultVerdict: "green",
					DefaultMessage: "WAL file count is normal",
				},
				NextStepDefault: intPtr(3),
			},
			{
				StepOrder:      3,
				Name:           "Check checkpoint configuration",
				Description:    "Verify checkpoint-related settings.",
				SQLTemplate:    `SELECT max(CASE WHEN name='checkpoint_timeout' THEN setting END) AS checkpoint_timeout, max(CASE WHEN name='checkpoint_completion_target' THEN setting END) AS checkpoint_completion_target, max(CASE WHEN name='max_wal_size' THEN setting END) AS max_wal_size, max(CASE WHEN name='min_wal_size' THEN setting END) AS min_wal_size FROM pg_settings WHERE name IN ('checkpoint_timeout', 'checkpoint_completion_target', 'max_wal_size', 'min_wal_size')`,
				SafetyTier:     TierDiagnostic,
				TimeoutSeconds: 5,
				ResultInterpretation: InterpretationSpec{
					DefaultVerdict: "yellow",
					DefaultMessage: "Review checkpoint_timeout and max_wal_size. Increasing max_wal_size reduces checkpoint frequency.",
				},
				NextStepDefault: intPtr(4),
			},
			{
				StepOrder:          4,
				Name:               "Remediation: Checkpoint tuning",
				Description:        "Checkpoint storms require configuration adjustments and workload review.",
				SafetyTier:         TierExternal,
				ManualInstructions: "1. Increase max_wal_size (e.g., 2GB-4GB) to reduce checkpoint frequency.\n2. Ensure checkpoint_completion_target is 0.9 (spread I/O over the interval).\n3. Increase shared_buffers if backend writes are high.\n4. Review bulk operations (COPY, large UPDATEs) that generate excessive WAL.\n5. Consider wal_compression = on to reduce WAL volume.\n6. After changes, run pg_reload_conf() and monitor checkpoint frequency.",
				EscalationContact:  "DBA",
			},
		},
	}
}
