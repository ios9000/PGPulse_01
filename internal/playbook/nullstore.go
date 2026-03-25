package playbook

import (
	"context"
	"time"
)

// NullPlaybookStore is a no-op PlaybookStore for live mode (no persistent storage).
type NullPlaybookStore struct{}

// NewNullPlaybookStore returns a no-op PlaybookStore.
func NewNullPlaybookStore() *NullPlaybookStore {
	return &NullPlaybookStore{}
}

func (n *NullPlaybookStore) Create(_ context.Context, _ *Playbook) (int64, error) { return 0, nil }
func (n *NullPlaybookStore) Get(_ context.Context, _ int64) (*Playbook, error)    { return nil, nil }
func (n *NullPlaybookStore) GetBySlug(_ context.Context, _ string) (*Playbook, error) {
	return nil, nil
}
func (n *NullPlaybookStore) Update(_ context.Context, _ *Playbook) error { return nil }
func (n *NullPlaybookStore) Delete(_ context.Context, _ int64) error     { return nil }
func (n *NullPlaybookStore) List(_ context.Context, _ PlaybookListOpts) ([]Playbook, int, error) {
	return nil, 0, nil
}
func (n *NullPlaybookStore) Promote(_ context.Context, _ int64) error  { return nil }
func (n *NullPlaybookStore) Deprecate(_ context.Context, _ int64) error { return nil }
func (n *NullPlaybookStore) FindByTriggerBinding(_ context.Context, _, _ string) (*Playbook, error) {
	return nil, nil
}
func (n *NullPlaybookStore) CreateRun(_ context.Context, _ *Run) (int64, error)   { return 0, nil }
func (n *NullPlaybookStore) GetRun(_ context.Context, _ int64) (*Run, error)      { return nil, nil }
func (n *NullPlaybookStore) UpdateRun(_ context.Context, _ *Run) error            { return nil }
func (n *NullPlaybookStore) ListRuns(_ context.Context, _ RunListOpts) ([]Run, int, error) {
	return nil, 0, nil
}
func (n *NullPlaybookStore) ListRunsByInstance(_ context.Context, _ string, _ RunListOpts) ([]Run, int, error) {
	return nil, 0, nil
}
func (n *NullPlaybookStore) CreateRunStep(_ context.Context, _ *RunStep) (int64, error) {
	return 0, nil
}
func (n *NullPlaybookStore) UpdateRunStep(_ context.Context, _ *RunStep) error { return nil }
func (n *NullPlaybookStore) GetRunSteps(_ context.Context, _ int64) ([]RunStep, error) {
	return nil, nil
}
func (n *NullPlaybookStore) LockStepForExecution(_ context.Context, _ int64, _ int) (bool, error) {
	return true, nil
}
func (n *NullPlaybookStore) ResetStepForRetry(_ context.Context, _ int64, _ int) error { return nil }
func (n *NullPlaybookStore) RequestStepApproval(_ context.Context, _ int64, _ int) error {
	return nil
}
func (n *NullPlaybookStore) SeedBuiltins(_ context.Context, _ []Playbook) error { return nil }
func (n *NullPlaybookStore) UpdateFeedback(_ context.Context, _ int64, _, _ *bool, _ string) error {
	return nil
}
func (n *NullPlaybookStore) ListCompletedRunsWithoutFeedback(_ context.Context, _ time.Time) ([]Run, error) {
	return nil, nil
}
func (n *NullPlaybookStore) CleanOldRuns(_ context.Context, _ time.Duration) (int64, error) {
	return 0, nil
}
