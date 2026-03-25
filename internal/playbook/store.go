package playbook

import (
	"context"
	"time"
)

// PlaybookStore defines persistence operations for playbooks and runs.
type PlaybookStore interface {
	// CRUD
	Create(ctx context.Context, pb *Playbook) (int64, error)
	Get(ctx context.Context, id int64) (*Playbook, error)
	GetBySlug(ctx context.Context, slug string) (*Playbook, error)
	Update(ctx context.Context, pb *Playbook) error
	Delete(ctx context.Context, id int64) error
	List(ctx context.Context, opts PlaybookListOpts) ([]Playbook, int, error)

	// Status lifecycle
	Promote(ctx context.Context, id int64) error
	Deprecate(ctx context.Context, id int64) error

	// Resolver query
	FindByTriggerBinding(ctx context.Context, bindingType, bindingValue string) (*Playbook, error)

	// Runs
	CreateRun(ctx context.Context, run *Run) (int64, error)
	GetRun(ctx context.Context, id int64) (*Run, error)
	UpdateRun(ctx context.Context, run *Run) error
	ListRuns(ctx context.Context, opts RunListOpts) ([]Run, int, error)
	ListRunsByInstance(ctx context.Context, instanceID string, opts RunListOpts) ([]Run, int, error)

	// Run steps
	CreateRunStep(ctx context.Context, step *RunStep) (int64, error)
	UpdateRunStep(ctx context.Context, step *RunStep) error
	GetRunSteps(ctx context.Context, runID int64) ([]RunStep, error)

	// C5: Concurrency guard — atomically lock a step for execution.
	LockStepForExecution(ctx context.Context, runID int64, stepOrder int) (bool, error)

	// C6: Retry failed steps — reset step status to pending.
	ResetStepForRetry(ctx context.Context, runID int64, stepOrder int) error

	// C7: Request approval — set step status to pending_approval.
	RequestStepApproval(ctx context.Context, runID int64, stepOrder int) error

	// Seed
	SeedBuiltins(ctx context.Context, playbooks []Playbook) error

	// Feedback
	UpdateFeedback(ctx context.Context, runID int64, useful, resolved *bool, notes string) error
	ListCompletedRunsWithoutFeedback(ctx context.Context, since time.Time) ([]Run, error)

	// Cleanup
	CleanOldRuns(ctx context.Context, olderThan time.Duration) (int64, error)
}
