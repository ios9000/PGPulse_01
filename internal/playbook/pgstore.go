package playbook

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PGStore is the PostgreSQL implementation of PlaybookStore.
type PGStore struct {
	pool *pgxpool.Pool
}

// NewPGStore creates a PGStore backed by the given connection pool.
func NewPGStore(pool *pgxpool.Pool) *PGStore {
	return &PGStore{pool: pool}
}

// Create inserts a new playbook with its steps.
func (s *PGStore) Create(ctx context.Context, pb *Playbook) (int64, error) {
	bindings, err := json.Marshal(pb.TriggerBindings)
	if err != nil {
		return 0, fmt.Errorf("marshal trigger_bindings: %w", err)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var id int64
	err = tx.QueryRow(ctx, `
		INSERT INTO playbooks (slug, name, description, version, status, category,
			trigger_bindings, estimated_duration_min, requires_permission, author, is_builtin)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id`,
		pb.Slug, pb.Name, pb.Description, pb.Version, pb.Status, pb.Category,
		bindings, pb.EstimatedDurationMin, pb.RequiresPermission, pb.Author, pb.IsBuiltin,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("insert playbook: %w", err)
	}

	for _, step := range pb.Steps {
		if err := insertStep(ctx, tx, id, &step); err != nil {
			return 0, fmt.Errorf("insert step %d: %w", step.StepOrder, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}
	return id, nil
}

func insertStep(ctx context.Context, tx pgx.Tx, playbookID int64, step *Step) error {
	interp, err := json.Marshal(step.ResultInterpretation)
	if err != nil {
		return fmt.Errorf("marshal result_interpretation: %w", err)
	}
	branches, err := json.Marshal(step.BranchRules)
	if err != nil {
		return fmt.Errorf("marshal branch_rules: %w", err)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO playbook_steps (playbook_id, step_order, name, description,
			sql_template, safety_tier, timeout_seconds, result_interpretation,
			branch_rules, next_step_default, manual_instructions, escalation_contact)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
		playbookID, step.StepOrder, step.Name, step.Description,
		nilIfEmpty(step.SQLTemplate), step.SafetyTier, step.TimeoutSeconds, interp,
		branches, step.NextStepDefault, nilIfEmpty(step.ManualInstructions), nilIfEmpty(step.EscalationContact),
	)
	return err
}

// Get returns a playbook by ID with all its steps.
func (s *PGStore) Get(ctx context.Context, id int64) (*Playbook, error) {
	pb, err := s.scanPlaybook(ctx, `SELECT * FROM playbooks WHERE id = $1`, id)
	if err != nil {
		return nil, err
	}
	if pb == nil {
		return nil, nil
	}
	steps, err := s.getSteps(ctx, pb.ID)
	if err != nil {
		return nil, err
	}
	pb.Steps = steps
	return pb, nil
}

// GetBySlug returns a playbook by slug with all its steps.
func (s *PGStore) GetBySlug(ctx context.Context, slug string) (*Playbook, error) {
	pb, err := s.scanPlaybook(ctx, `SELECT * FROM playbooks WHERE slug = $1`, slug)
	if err != nil {
		return nil, err
	}
	if pb == nil {
		return nil, nil
	}
	steps, err := s.getSteps(ctx, pb.ID)
	if err != nil {
		return nil, err
	}
	pb.Steps = steps
	return pb, nil
}

// Update replaces a playbook's metadata and steps (version bump + draft reset).
func (s *PGStore) Update(ctx context.Context, pb *Playbook) error {
	bindings, err := json.Marshal(pb.TriggerBindings)
	if err != nil {
		return fmt.Errorf("marshal trigger_bindings: %w", err)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	_, err = tx.Exec(ctx, `
		UPDATE playbooks SET name=$1, description=$2, version=version+1, status='draft',
			category=$3, trigger_bindings=$4, estimated_duration_min=$5,
			requires_permission=$6, author=$7, updated_at=NOW()
		WHERE id=$8`,
		pb.Name, pb.Description, pb.Category, bindings,
		pb.EstimatedDurationMin, pb.RequiresPermission, pb.Author, pb.ID,
	)
	if err != nil {
		return fmt.Errorf("update playbook: %w", err)
	}

	// Replace steps: delete old, insert new.
	_, err = tx.Exec(ctx, `DELETE FROM playbook_steps WHERE playbook_id = $1`, pb.ID)
	if err != nil {
		return fmt.Errorf("delete old steps: %w", err)
	}
	for _, step := range pb.Steps {
		if err := insertStep(ctx, tx, pb.ID, &step); err != nil {
			return fmt.Errorf("insert step %d: %w", step.StepOrder, err)
		}
	}

	return tx.Commit(ctx)
}

// Delete removes a playbook by ID (blocked for builtins at API level).
func (s *PGStore) Delete(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM playbooks WHERE id = $1`, id)
	return err
}

// List returns playbooks matching the given filters.
func (s *PGStore) List(ctx context.Context, opts PlaybookListOpts) ([]Playbook, int, error) {
	where := "WHERE 1=1"
	args := []any{}
	argN := 0

	if opts.Status != "" {
		argN++
		where += fmt.Sprintf(" AND status = $%d", argN)
		args = append(args, opts.Status)
	}
	if opts.Category != "" {
		argN++
		where += fmt.Sprintf(" AND category = $%d", argN)
		args = append(args, opts.Category)
	}
	if opts.Search != "" {
		argN++
		where += fmt.Sprintf(" AND (name ILIKE '%%' || $%d || '%%' OR slug ILIKE '%%' || $%d || '%%')", argN, argN)
		args = append(args, opts.Search)
	}

	var total int
	countArgs := make([]any, len(args))
	copy(countArgs, args)
	err := s.pool.QueryRow(ctx, "SELECT count(*) FROM playbooks "+where, countArgs...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count playbooks: %w", err)
	}

	limit := opts.Limit
	if limit <= 0 {
		limit = 50
	}
	offset := opts.Offset

	argN++
	limitArg := argN
	argN++
	offsetArg := argN
	args = append(args, limit, offset)

	query := fmt.Sprintf("SELECT * FROM playbooks %s ORDER BY name ASC LIMIT $%d OFFSET $%d", where, limitArg, offsetArg)
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list playbooks: %w", err)
	}
	defer rows.Close()

	var playbooks []Playbook
	for rows.Next() {
		pb, err := scanPlaybookRow(rows)
		if err != nil {
			return nil, 0, err
		}
		playbooks = append(playbooks, *pb)
	}
	return playbooks, total, nil
}

// Promote changes a playbook's status to stable.
func (s *PGStore) Promote(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `UPDATE playbooks SET status='stable', updated_at=NOW() WHERE id=$1`, id)
	return err
}

// Deprecate marks a playbook as deprecated.
func (s *PGStore) Deprecate(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `UPDATE playbooks SET status='deprecated', updated_at=NOW() WHERE id=$1`, id)
	return err
}

// FindByTriggerBinding finds the best stable playbook matching a trigger binding.
// Uses JSONB containment with the GIN index.
func (s *PGStore) FindByTriggerBinding(ctx context.Context, bindingType, bindingValue string) (*Playbook, error) {
	// Build containment JSON: {"hooks": ["value"]}
	containment := map[string][]string{bindingType: {bindingValue}}
	containmentJSON, err := json.Marshal(containment)
	if err != nil {
		return nil, fmt.Errorf("marshal containment: %w", err)
	}

	pb, err := s.scanPlaybook(ctx, `
		SELECT * FROM playbooks
		WHERE status = 'stable' AND trigger_bindings @> $1::jsonb
		ORDER BY version DESC
		LIMIT 1`, string(containmentJSON))
	if err != nil {
		return nil, err
	}
	if pb == nil {
		return nil, nil
	}

	steps, err := s.getSteps(ctx, pb.ID)
	if err != nil {
		return nil, err
	}
	pb.Steps = steps
	return pb, nil
}

// CreateRun inserts a new playbook run.
func (s *PGStore) CreateRun(ctx context.Context, run *Run) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `
		INSERT INTO playbook_runs (playbook_id, playbook_version, instance_id, started_by,
			status, current_step_order, trigger_source, trigger_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id`,
		run.PlaybookID, run.PlaybookVersion, run.InstanceID, run.StartedBy,
		run.Status, run.CurrentStepOrder,
		nilIfEmpty(run.TriggerSource), nilIfEmpty(run.TriggerID),
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("create run: %w", err)
	}
	return id, nil
}

// GetRun returns a run by ID with all its step records.
func (s *PGStore) GetRun(ctx context.Context, id int64) (*Run, error) {
	run := &Run{}
	var triggerSource, triggerID, feedbackNotes *string
	err := s.pool.QueryRow(ctx, `
		SELECT r.id, r.playbook_id, r.playbook_version, r.instance_id, r.started_by,
			r.status, r.current_step_order, r.trigger_source, r.trigger_id,
			r.started_at, r.updated_at, r.completed_at,
			r.feedback_useful, r.feedback_resolved, r.feedback_notes,
			p.name
		FROM playbook_runs r
		JOIN playbooks p ON p.id = r.playbook_id
		WHERE r.id = $1`, id).Scan(
		&run.ID, &run.PlaybookID, &run.PlaybookVersion, &run.InstanceID, &run.StartedBy,
		&run.Status, &run.CurrentStepOrder, &triggerSource, &triggerID,
		&run.StartedAt, &run.UpdatedAt, &run.CompletedAt,
		&run.FeedbackUseful, &run.FeedbackResolved, &feedbackNotes,
		&run.PlaybookName,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get run: %w", err)
	}
	if triggerSource != nil {
		run.TriggerSource = *triggerSource
	}
	if triggerID != nil {
		run.TriggerID = *triggerID
	}
	if feedbackNotes != nil {
		run.FeedbackNotes = *feedbackNotes
	}

	steps, err := s.GetRunSteps(ctx, id)
	if err != nil {
		return nil, err
	}
	run.Steps = steps
	return run, nil
}

// UpdateRun updates a run's mutable fields.
func (s *PGStore) UpdateRun(ctx context.Context, run *Run) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE playbook_runs SET status=$1, current_step_order=$2, completed_at=$3, updated_at=NOW()
		WHERE id=$4`,
		run.Status, run.CurrentStepOrder, run.CompletedAt, run.ID,
	)
	return err
}

// ListRuns returns all runs matching filters.
func (s *PGStore) ListRuns(ctx context.Context, opts RunListOpts) ([]Run, int, error) {
	return s.listRunsWhere(ctx, "", nil, opts)
}

// ListRunsByInstance returns runs for a specific instance.
func (s *PGStore) ListRunsByInstance(ctx context.Context, instanceID string, opts RunListOpts) ([]Run, int, error) {
	return s.listRunsWhere(ctx, "r.instance_id = $1", []any{instanceID}, opts)
}

func (s *PGStore) listRunsWhere(ctx context.Context, extraWhere string, extraArgs []any, opts RunListOpts) ([]Run, int, error) {
	where := "WHERE 1=1"
	args := make([]any, 0, len(extraArgs)+3)
	argN := 0

	if extraWhere != "" {
		// Re-number the placeholder in extraWhere.
		argN++
		where += " AND r.instance_id = $1"
		args = append(args, extraArgs[0])
	}
	if opts.Status != "" {
		argN++
		where += fmt.Sprintf(" AND r.status = $%d", argN)
		args = append(args, opts.Status)
	}

	var total int
	countArgs := make([]any, len(args))
	copy(countArgs, args)
	err := s.pool.QueryRow(ctx, "SELECT count(*) FROM playbook_runs r "+where, countArgs...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count runs: %w", err)
	}

	limit := opts.Limit
	if limit <= 0 {
		limit = 50
	}
	argN++
	limitN := argN
	argN++
	offsetN := argN
	args = append(args, limit, opts.Offset)

	query := fmt.Sprintf(`
		SELECT r.id, r.playbook_id, r.playbook_version, r.instance_id, r.started_by,
			r.status, r.current_step_order, r.trigger_source, r.trigger_id,
			r.started_at, r.updated_at, r.completed_at,
			r.feedback_useful, r.feedback_resolved, r.feedback_notes,
			p.name
		FROM playbook_runs r
		JOIN playbooks p ON p.id = r.playbook_id
		%s
		ORDER BY r.started_at DESC
		LIMIT $%d OFFSET $%d`, where, limitN, offsetN)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list runs: %w", err)
	}
	defer rows.Close()

	var runs []Run
	for rows.Next() {
		run := Run{}
		var triggerSource, triggerID, feedbackNotes *string
		err := rows.Scan(
			&run.ID, &run.PlaybookID, &run.PlaybookVersion, &run.InstanceID, &run.StartedBy,
			&run.Status, &run.CurrentStepOrder, &triggerSource, &triggerID,
			&run.StartedAt, &run.UpdatedAt, &run.CompletedAt,
			&run.FeedbackUseful, &run.FeedbackResolved, &feedbackNotes,
			&run.PlaybookName,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("scan run: %w", err)
		}
		if triggerSource != nil {
			run.TriggerSource = *triggerSource
		}
		if triggerID != nil {
			run.TriggerID = *triggerID
		}
		if feedbackNotes != nil {
			run.FeedbackNotes = *feedbackNotes
		}
		runs = append(runs, run)
	}
	return runs, total, nil
}

// CreateRunStep inserts a run step record.
func (s *PGStore) CreateRunStep(ctx context.Context, step *RunStep) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `
		INSERT INTO playbook_run_steps (run_id, step_order, status, sql_executed,
			result_json, result_verdict, result_message, error, executed_at, duration_ms, confirmed_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (run_id, step_order) DO UPDATE SET
			status=EXCLUDED.status, sql_executed=EXCLUDED.sql_executed,
			result_json=EXCLUDED.result_json, result_verdict=EXCLUDED.result_verdict,
			result_message=EXCLUDED.result_message, error=EXCLUDED.error,
			executed_at=EXCLUDED.executed_at, duration_ms=EXCLUDED.duration_ms,
			confirmed_by=EXCLUDED.confirmed_by
		RETURNING id`,
		step.RunID, step.StepOrder, step.Status,
		nilIfEmpty(step.SQLExecuted), step.ResultJSON,
		nilIfEmpty(step.ResultVerdict), nilIfEmpty(step.ResultMessage),
		nilIfEmpty(step.Error), step.ExecutedAt, step.DurationMs,
		nilIfEmpty(step.ConfirmedBy),
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("create run step: %w", err)
	}
	return id, nil
}

// UpdateRunStep updates an existing run step.
func (s *PGStore) UpdateRunStep(ctx context.Context, step *RunStep) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE playbook_run_steps SET status=$1, sql_executed=$2, result_json=$3,
			result_verdict=$4, result_message=$5, error=$6, executed_at=$7,
			duration_ms=$8, confirmed_by=$9
		WHERE run_id=$10 AND step_order=$11`,
		step.Status, nilIfEmpty(step.SQLExecuted), step.ResultJSON,
		nilIfEmpty(step.ResultVerdict), nilIfEmpty(step.ResultMessage),
		nilIfEmpty(step.Error), step.ExecutedAt, step.DurationMs,
		nilIfEmpty(step.ConfirmedBy), step.RunID, step.StepOrder,
	)
	return err
}

// GetRunSteps returns all step records for a run.
func (s *PGStore) GetRunSteps(ctx context.Context, runID int64) ([]RunStep, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, run_id, step_order, status, sql_executed, result_json,
			result_verdict, result_message, error, executed_at, duration_ms, confirmed_by
		FROM playbook_run_steps
		WHERE run_id = $1
		ORDER BY step_order`, runID)
	if err != nil {
		return nil, fmt.Errorf("get run steps: %w", err)
	}
	defer rows.Close()

	var steps []RunStep
	for rows.Next() {
		var rs RunStep
		var sqlExec, verdict, msg, errMsg, confirmedBy *string
		err := rows.Scan(&rs.ID, &rs.RunID, &rs.StepOrder, &rs.Status,
			&sqlExec, &rs.ResultJSON, &verdict, &msg, &errMsg,
			&rs.ExecutedAt, &rs.DurationMs, &confirmedBy)
		if err != nil {
			return nil, fmt.Errorf("scan run step: %w", err)
		}
		if sqlExec != nil {
			rs.SQLExecuted = *sqlExec
		}
		if verdict != nil {
			rs.ResultVerdict = *verdict
		}
		if msg != nil {
			rs.ResultMessage = *msg
		}
		if errMsg != nil {
			rs.Error = *errMsg
		}
		if confirmedBy != nil {
			rs.ConfirmedBy = *confirmedBy
		}
		steps = append(steps, rs)
	}
	return steps, nil
}

// LockStepForExecution atomically sets a step to running if it is in an executable state.
// Returns false if the step is already running or completed.
func (s *PGStore) LockStepForExecution(ctx context.Context, runID int64, stepOrder int) (bool, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `
		UPDATE playbook_run_steps SET status = 'running'
		WHERE run_id = $1 AND step_order = $2
			AND status IN ('pending', 'awaiting_confirmation', 'pending_approval')
		RETURNING id`, runID, stepOrder).Scan(&id)
	if err != nil {
		if err == pgx.ErrNoRows {
			return false, nil
		}
		return false, fmt.Errorf("lock step: %w", err)
	}
	return true, nil
}

// ResetStepForRetry resets a failed step back to pending.
func (s *PGStore) ResetStepForRetry(ctx context.Context, runID int64, stepOrder int) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE playbook_run_steps SET status = 'pending', error = NULL
		WHERE run_id = $1 AND step_order = $2 AND status = 'failed'`, runID, stepOrder)
	return err
}

// RequestStepApproval sets a step's status to pending_approval.
func (s *PGStore) RequestStepApproval(ctx context.Context, runID int64, stepOrder int) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE playbook_run_steps SET status = 'pending_approval'
		WHERE run_id = $1 AND step_order = $2 AND status IN ('pending', 'awaiting_confirmation')`, runID, stepOrder)
	return err
}

// SeedBuiltins inserts built-in playbooks using INSERT ON CONFLICT DO NOTHING.
func (s *PGStore) SeedBuiltins(ctx context.Context, playbooks []Playbook) error {
	for _, pb := range playbooks {
		bindings, err := json.Marshal(pb.TriggerBindings)
		if err != nil {
			return fmt.Errorf("marshal trigger_bindings for %s: %w", pb.Slug, err)
		}

		tx, err := s.pool.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin tx for %s: %w", pb.Slug, err)
		}

		var id int64
		err = tx.QueryRow(ctx, `
			INSERT INTO playbooks (slug, name, description, version, status, category,
				trigger_bindings, estimated_duration_min, requires_permission, author, is_builtin)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, true)
			ON CONFLICT (slug) DO NOTHING
			RETURNING id`,
			pb.Slug, pb.Name, pb.Description, pb.Version, pb.Status, pb.Category,
			bindings, pb.EstimatedDurationMin, pb.RequiresPermission, pb.Author,
		).Scan(&id)

		if err != nil {
			// ON CONFLICT DO NOTHING — no row returned, playbook already exists.
			_ = tx.Rollback(ctx)
			continue
		}

		for _, step := range pb.Steps {
			if err := insertStep(ctx, tx, id, &step); err != nil {
				_ = tx.Rollback(ctx)
				return fmt.Errorf("seed step %d for %s: %w", step.StepOrder, pb.Slug, err)
			}
		}

		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit seed for %s: %w", pb.Slug, err)
		}
	}
	return nil
}

// UpdateFeedback sets the feedback fields on a run.
func (s *PGStore) UpdateFeedback(ctx context.Context, runID int64, useful, resolved *bool, notes string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE playbook_runs SET feedback_useful=$1, feedback_resolved=$2, feedback_notes=$3, updated_at=NOW()
		WHERE id=$4`,
		useful, resolved, nilIfEmpty(notes), runID,
	)
	return err
}

// ListCompletedRunsWithoutFeedback returns completed runs with null feedback_resolved
// that completed after the given time.
func (s *PGStore) ListCompletedRunsWithoutFeedback(ctx context.Context, since time.Time) ([]Run, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT r.id, r.playbook_id, r.playbook_version, r.instance_id, r.started_by,
			r.status, r.current_step_order, r.trigger_source, r.trigger_id,
			r.started_at, r.updated_at, r.completed_at,
			r.feedback_useful, r.feedback_resolved, r.feedback_notes
		FROM playbook_runs r
		WHERE r.status = 'completed'
			AND r.feedback_resolved IS NULL
			AND r.completed_at >= $1
		ORDER BY r.completed_at`, since)
	if err != nil {
		return nil, fmt.Errorf("list feedback runs: %w", err)
	}
	defer rows.Close()

	var runs []Run
	for rows.Next() {
		run := Run{}
		var triggerSource, triggerID, feedbackNotes *string
		err := rows.Scan(
			&run.ID, &run.PlaybookID, &run.PlaybookVersion, &run.InstanceID, &run.StartedBy,
			&run.Status, &run.CurrentStepOrder, &triggerSource, &triggerID,
			&run.StartedAt, &run.UpdatedAt, &run.CompletedAt,
			&run.FeedbackUseful, &run.FeedbackResolved, &feedbackNotes,
		)
		if err != nil {
			return nil, fmt.Errorf("scan feedback run: %w", err)
		}
		if triggerSource != nil {
			run.TriggerSource = *triggerSource
		}
		if triggerID != nil {
			run.TriggerID = *triggerID
		}
		if feedbackNotes != nil {
			run.FeedbackNotes = *feedbackNotes
		}
		runs = append(runs, run)
	}
	return runs, nil
}

// CleanOldRuns deletes runs older than the given duration.
func (s *PGStore) CleanOldRuns(ctx context.Context, olderThan time.Duration) (int64, error) {
	cutoff := time.Now().Add(-olderThan)
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM playbook_runs WHERE completed_at IS NOT NULL AND completed_at < $1`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("clean old runs: %w", err)
	}
	return tag.RowsAffected(), nil
}

// scanPlaybook executes a query returning a single playbook row.
func (s *PGStore) scanPlaybook(ctx context.Context, query string, args ...any) (*Playbook, error) {
	row := s.pool.QueryRow(ctx, query, args...)
	return scanPlaybookFromRow(row)
}

func scanPlaybookFromRow(row pgx.Row) (*Playbook, error) {
	pb := &Playbook{}
	var bindings []byte
	err := row.Scan(
		&pb.ID, &pb.Slug, &pb.Name, &pb.Description, &pb.Version, &pb.Status,
		&pb.Category, &bindings, &pb.EstimatedDurationMin, &pb.RequiresPermission,
		&pb.Author, &pb.IsBuiltin, &pb.CreatedAt, &pb.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("scan playbook: %w", err)
	}
	if err := json.Unmarshal(bindings, &pb.TriggerBindings); err != nil {
		return nil, fmt.Errorf("unmarshal trigger_bindings: %w", err)
	}
	return pb, nil
}

func scanPlaybookRow(rows pgx.Rows) (*Playbook, error) {
	pb := &Playbook{}
	var bindings []byte
	err := rows.Scan(
		&pb.ID, &pb.Slug, &pb.Name, &pb.Description, &pb.Version, &pb.Status,
		&pb.Category, &bindings, &pb.EstimatedDurationMin, &pb.RequiresPermission,
		&pb.Author, &pb.IsBuiltin, &pb.CreatedAt, &pb.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("scan playbook row: %w", err)
	}
	if err := json.Unmarshal(bindings, &pb.TriggerBindings); err != nil {
		return nil, fmt.Errorf("unmarshal trigger_bindings: %w", err)
	}
	return pb, nil
}

func (s *PGStore) getSteps(ctx context.Context, playbookID int64) ([]Step, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, playbook_id, step_order, name, description, sql_template,
			safety_tier, timeout_seconds, result_interpretation, branch_rules,
			next_step_default, manual_instructions, escalation_contact, created_at
		FROM playbook_steps
		WHERE playbook_id = $1
		ORDER BY step_order`, playbookID)
	if err != nil {
		return nil, fmt.Errorf("get steps: %w", err)
	}
	defer rows.Close()

	var steps []Step
	for rows.Next() {
		var step Step
		var sqlTemplate, manualInstr, escalation *string
		var interpJSON, branchJSON []byte
		err := rows.Scan(&step.ID, &step.PlaybookID, &step.StepOrder, &step.Name, &step.Description,
			&sqlTemplate, &step.SafetyTier, &step.TimeoutSeconds, &interpJSON, &branchJSON,
			&step.NextStepDefault, &manualInstr, &escalation, &step.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("scan step: %w", err)
		}
		if sqlTemplate != nil {
			step.SQLTemplate = *sqlTemplate
		}
		if manualInstr != nil {
			step.ManualInstructions = *manualInstr
		}
		if escalation != nil {
			step.EscalationContact = *escalation
		}
		if err := json.Unmarshal(interpJSON, &step.ResultInterpretation); err != nil {
			return nil, fmt.Errorf("unmarshal interpretation: %w", err)
		}
		if err := json.Unmarshal(branchJSON, &step.BranchRules); err != nil {
			return nil, fmt.Errorf("unmarshal branch_rules: %w", err)
		}
		steps = append(steps, step)
	}
	return steps, nil
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
