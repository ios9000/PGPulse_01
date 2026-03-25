package playbook

func walArchiveFailurePlaybook() Playbook {
	return Playbook{
		Slug:        "wal-archive-failure",
		Name:        "WAL Archive Failure",
		Description: "Diagnoses and helps resolve WAL archiving failures that can lead to disk exhaustion and database shutdown.",
		Version:     1,
		Status:      "stable",
		Category:    "storage",
		TriggerBindings: TriggerBindings{
			Hooks:      []string{"remediation.wal_archive"},
			RootCauses: []string{"root_cause.wal_accumulation"},
			Metrics:    []string{"pg.server.archive_fail_count"},
		},
		EstimatedDurationMin: intPtr(10),
		RequiresPermission:   "view_all",
		Author:               "pgpulse",
		IsBuiltin:            true,
		Steps: []Step{
			{
				StepOrder:      1,
				Name:           "Check archive status",
				Description:    "Query pg_stat_archiver to check for archiving failures.",
				SQLTemplate:    `SELECT archived_count, failed_count, last_archived_wal, last_failed_wal, last_archived_time, last_failed_time FROM pg_stat_archiver`,
				SafetyTier:     TierDiagnostic,
				TimeoutSeconds: 5,
				ResultInterpretation: InterpretationSpec{
					Rules: []InterpretationRule{
						{Column: "failed_count", Operator: ">", Value: 0, Verdict: "red",
							Message: "{{failed_count}} archive failures detected. Last failure: {{last_failed_time}}"},
						{Column: "failed_count", Operator: "==", Value: 0, Verdict: "green",
							Message: "No archive failures — archiving is healthy"},
					},
					DefaultVerdict: "yellow",
					DefaultMessage: "Unable to determine archive status",
				},
				BranchRules: []BranchRule{
					{Condition: BranchCondition{Column: "failed_count", Operator: ">", Value: 0},
						GotoStep: 2, Reason: "Failures detected — check WAL accumulation"},
					{Condition: BranchCondition{Verdict: "green"},
						GotoStep: 5, Reason: "Archiving healthy — verify no residual issues"},
				},
				NextStepDefault: intPtr(2),
			},
			{
				StepOrder:      2,
				Name:           "Check WAL file accumulation",
				Description:    "Count WAL files in pg_wal directory. Normal is 10-50 files. Above 100 indicates a backlog.",
				SQLTemplate:    `SELECT count(*) AS wal_file_count, pg_size_pretty(sum(size)) AS total_wal_size FROM pg_ls_waldir()`,
				SafetyTier:     TierDiagnostic,
				TimeoutSeconds: 10,
				ResultInterpretation: InterpretationSpec{
					Rules: []InterpretationRule{
						{Column: "wal_file_count", Operator: ">", Value: 500, Verdict: "red",
							Message: "CRITICAL: {{wal_file_count}} WAL files accumulated ({{total_wal_size}}). Disk exhaustion imminent!"},
						{Column: "wal_file_count", Operator: ">", Value: 100, Verdict: "yellow",
							Message: "WARNING: {{wal_file_count}} WAL files ({{total_wal_size}}). Archiving is falling behind."},
						{Column: "wal_file_count", Operator: "<=", Value: 100, Verdict: "green",
							Message: "WAL file count normal: {{wal_file_count}} files ({{total_wal_size}})"},
					},
					DefaultVerdict: "yellow",
				},
				BranchRules: []BranchRule{
					{Condition: BranchCondition{Column: "wal_file_count", Operator: ">", Value: 500},
						GotoStep: 4, Reason: "Emergency — escalate immediately"},
				},
				NextStepDefault: intPtr(3),
			},
			{
				StepOrder:      3,
				Name:           "Check archive_command configuration",
				Description:    "Verify the archive_command setting to identify the archive destination.",
				SQLTemplate:    `SELECT count(*) AS setting_count, max(CASE WHEN name='archive_mode' THEN setting END) AS archive_mode, max(CASE WHEN name='archive_command' THEN setting END) AS archive_command FROM pg_settings WHERE name IN ('archive_mode', 'archive_command', 'archive_library', 'archive_timeout')`,
				SafetyTier:     TierDiagnostic,
				TimeoutSeconds: 5,
				ResultInterpretation: InterpretationSpec{
					DefaultVerdict: "yellow",
					DefaultMessage: "Review the archive_command output. The destination path/host may be unreachable or full.",
				},
				NextStepDefault: intPtr(4),
			},
			{
				StepOrder:          4,
				Name:               "Emergency: Contact infrastructure team",
				Description:        "WAL accumulation indicates the archive target is unavailable. This is typically a storage/network issue outside PostgreSQL.",
				SafetyTier:         TierExternal,
				ManualInstructions: "1. Check the archive target server/storage for disk space.\n2. Verify network connectivity to the archive destination.\n3. If using pg_basebackup or barman, check its logs.\n4. If disk is full on the archive target, free space immediately.\n5. After fixing, verify: SELECT * FROM pg_stat_archiver; -- failed_count should stop increasing.",
				EscalationContact:  "Infrastructure team / Storage admin",
				NextStepDefault:    intPtr(5),
			},
			{
				StepOrder:      5,
				Name:           "Verify recovery",
				Description:    "After the archive target is restored, verify archiving has resumed.",
				SQLTemplate:    `SELECT archived_count, failed_count, last_archived_wal, last_archived_time, NOW() - last_archived_time AS time_since_last_archive FROM pg_stat_archiver`,
				SafetyTier:     TierDiagnostic,
				TimeoutSeconds: 5,
				ResultInterpretation: InterpretationSpec{
					Rules: []InterpretationRule{
						{Column: "failed_count", Operator: "==", Value: 0, Verdict: "green",
							Message: "Archiving recovered — no more failures"},
					},
					DefaultVerdict: "yellow",
					DefaultMessage: "Archiving may still be catching up. Re-run this step in a few minutes.",
				},
			},
		},
	}
}
