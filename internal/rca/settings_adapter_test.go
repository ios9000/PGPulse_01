package rca

import (
	"context"
	"testing"
	"time"
)

// mockSettingsProvider implements SettingsProvider for testing.
type mockSettingsProvider struct {
	changes []SettingChange
	err     error
}

func (m *mockSettingsProvider) GetChanges(_ context.Context, _ string, _, _ time.Time) ([]SettingChange, error) {
	return m.changes, m.err
}

func TestSettingsProvider_Interface(t *testing.T) {
	t.Parallel()

	// Verify the interface is implementable.
	var p SettingsProvider = &mockSettingsProvider{}
	changes, err := p.GetChanges(context.Background(), "inst-1", time.Now().Add(-time.Hour), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if changes != nil {
		t.Errorf("expected nil changes from empty mock, got %v", changes)
	}
}

func TestSettingsProvider_WithChanges(t *testing.T) {
	t.Parallel()

	now := time.Now()
	p := &mockSettingsProvider{
		changes: []SettingChange{
			{Name: "shared_buffers", OldValue: "128MB", NewValue: "256MB", ChangedAt: now.Add(-10 * time.Minute)},
			{Name: "work_mem", OldValue: "4MB", NewValue: "8MB", ChangedAt: now.Add(-5 * time.Minute)},
		},
	}

	changes, err := p.GetChanges(context.Background(), "inst-1", now.Add(-time.Hour), now)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 2 {
		t.Fatalf("expected 2 changes, got %d", len(changes))
	}
	if changes[0].Name != "shared_buffers" {
		t.Errorf("expected shared_buffers, got %s", changes[0].Name)
	}
}
