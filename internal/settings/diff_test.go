package settings

import "testing"

func TestDiffSnapshots_Changed(t *testing.T) {
	older := map[string]SettingValue{
		"max_connections": {Setting: "100", Unit: "", Source: "configuration file"},
	}
	newer := map[string]SettingValue{
		"max_connections": {Setting: "200", Unit: "", Source: "configuration file"},
	}
	diff := DiffSnapshots(older, newer)
	if len(diff.Changed) != 1 {
		t.Fatalf("expected 1 changed, got %d", len(diff.Changed))
	}
	if diff.Changed[0].OldValue != "100" || diff.Changed[0].NewValue != "200" {
		t.Errorf("unexpected change values: %+v", diff.Changed[0])
	}
	if diff.Changed[0].Name != "max_connections" {
		t.Errorf("unexpected name: %s", diff.Changed[0].Name)
	}
}

func TestDiffSnapshots_Added(t *testing.T) {
	older := map[string]SettingValue{}
	newer := map[string]SettingValue{
		"new_setting": {Setting: "42", Unit: "kB", Source: "default"},
	}
	diff := DiffSnapshots(older, newer)
	if len(diff.Added) != 1 {
		t.Fatalf("expected 1 added, got %d", len(diff.Added))
	}
	if diff.Added[0].Name != "new_setting" {
		t.Errorf("unexpected added name: %s", diff.Added[0].Name)
	}
	if diff.Added[0].NewValue != "42" {
		t.Errorf("unexpected added value: %s", diff.Added[0].NewValue)
	}
	if diff.Added[0].Unit != "kB" {
		t.Errorf("unexpected added unit: %s", diff.Added[0].Unit)
	}
}

func TestDiffSnapshots_Removed(t *testing.T) {
	older := map[string]SettingValue{
		"old_setting": {Setting: "on"},
	}
	newer := map[string]SettingValue{}
	diff := DiffSnapshots(older, newer)
	if len(diff.Removed) != 1 {
		t.Fatalf("expected 1 removed, got %d", len(diff.Removed))
	}
	if diff.Removed[0].Name != "old_setting" {
		t.Errorf("unexpected removed name: %s", diff.Removed[0].Name)
	}
	if diff.Removed[0].OldValue != "on" {
		t.Errorf("unexpected removed value: %s", diff.Removed[0].OldValue)
	}
}

func TestDiffSnapshots_PendingRestart(t *testing.T) {
	older := map[string]SettingValue{
		"shared_buffers": {Setting: "4096", PendingRestart: false},
	}
	newer := map[string]SettingValue{
		"shared_buffers": {Setting: "8192", PendingRestart: true},
	}
	diff := DiffSnapshots(older, newer)
	if len(diff.PendingRestart) != 1 || diff.PendingRestart[0] != "shared_buffers" {
		t.Errorf("expected shared_buffers in pending_restart, got %v", diff.PendingRestart)
	}
}

func TestDiffSnapshots_PendingRestart_Unchanged(t *testing.T) {
	// PendingRestart should be reported even if the value hasn't changed
	settings := map[string]SettingValue{
		"shared_buffers": {Setting: "4096", PendingRestart: true},
	}
	diff := DiffSnapshots(settings, settings)
	if len(diff.PendingRestart) != 1 || diff.PendingRestart[0] != "shared_buffers" {
		t.Errorf("expected shared_buffers in pending_restart even when unchanged, got %v", diff.PendingRestart)
	}
}

func TestDiffSnapshots_NoChange(t *testing.T) {
	settings := map[string]SettingValue{
		"max_connections": {Setting: "100"},
		"shared_buffers":  {Setting: "4096"},
	}
	diff := DiffSnapshots(settings, settings)
	if len(diff.Changed) != 0 || len(diff.Added) != 0 || len(diff.Removed) != 0 {
		t.Errorf("expected empty diff, got changed=%d added=%d removed=%d",
			len(diff.Changed), len(diff.Added), len(diff.Removed))
	}
}

func TestDiffSnapshots_NonNilSlices(t *testing.T) {
	diff := DiffSnapshots(map[string]SettingValue{}, map[string]SettingValue{})
	if diff.Changed == nil {
		t.Error("Changed slice should not be nil")
	}
	if diff.Added == nil {
		t.Error("Added slice should not be nil")
	}
	if diff.Removed == nil {
		t.Error("Removed slice should not be nil")
	}
	if diff.PendingRestart == nil {
		t.Error("PendingRestart slice should not be nil")
	}
}

func TestDiffSnapshots_MultipleChanges_Sorted(t *testing.T) {
	older := map[string]SettingValue{
		"work_mem":        {Setting: "4MB"},
		"max_connections": {Setting: "100"},
		"shared_buffers":  {Setting: "128MB"},
	}
	newer := map[string]SettingValue{
		"work_mem":        {Setting: "8MB"},
		"max_connections": {Setting: "200"},
		"shared_buffers":  {Setting: "256MB"},
	}
	diff := DiffSnapshots(older, newer)
	if len(diff.Changed) != 3 {
		t.Fatalf("expected 3 changed, got %d", len(diff.Changed))
	}
	// Verify sorted order
	if diff.Changed[0].Name != "max_connections" ||
		diff.Changed[1].Name != "shared_buffers" ||
		diff.Changed[2].Name != "work_mem" {
		t.Errorf("changed not sorted: %s, %s, %s",
			diff.Changed[0].Name, diff.Changed[1].Name, diff.Changed[2].Name)
	}
}

func TestDiffSnapshots_Mixed(t *testing.T) {
	older := map[string]SettingValue{
		"max_connections": {Setting: "100"},
		"old_param":       {Setting: "yes"},
	}
	newer := map[string]SettingValue{
		"max_connections": {Setting: "200"},
		"new_param":       {Setting: "on"},
	}
	diff := DiffSnapshots(older, newer)
	if len(diff.Changed) != 1 {
		t.Errorf("expected 1 changed, got %d", len(diff.Changed))
	}
	if len(diff.Added) != 1 {
		t.Errorf("expected 1 added, got %d", len(diff.Added))
	}
	if len(diff.Removed) != 1 {
		t.Errorf("expected 1 removed, got %d", len(diff.Removed))
	}
}
