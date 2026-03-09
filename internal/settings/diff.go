package settings

import "sort"

// SettingChange represents a changed, added, or removed setting.
type SettingChange struct {
	Name     string `json:"name"`
	OldValue string `json:"old_value,omitempty"`
	NewValue string `json:"new_value,omitempty"`
	Unit     string `json:"unit,omitempty"`
	Source   string `json:"source,omitempty"`
}

// SettingsDiff is the result of comparing two settings snapshots.
type SettingsDiff struct {
	Changed        []SettingChange `json:"changed"`
	Added          []SettingChange `json:"added"`
	Removed        []SettingChange `json:"removed"`
	PendingRestart []string        `json:"pending_restart"`
}

// DiffSnapshots compares two settings maps and returns the differences.
func DiffSnapshots(older, newer map[string]SettingValue) SettingsDiff {
	var diff SettingsDiff

	for name, newVal := range newer {
		if oldVal, ok := older[name]; !ok {
			diff.Added = append(diff.Added, SettingChange{
				Name:     name,
				NewValue: newVal.Setting,
				Unit:     newVal.Unit,
				Source:   newVal.Source,
			})
		} else if oldVal.Setting != newVal.Setting {
			diff.Changed = append(diff.Changed, SettingChange{
				Name:     name,
				OldValue: oldVal.Setting,
				NewValue: newVal.Setting,
				Unit:     newVal.Unit,
				Source:   newVal.Source,
			})
		}
		if newVal.PendingRestart {
			diff.PendingRestart = append(diff.PendingRestart, name)
		}
	}

	for name := range older {
		if _, ok := newer[name]; !ok {
			diff.Removed = append(diff.Removed, SettingChange{
				Name:     name,
				OldValue: older[name].Setting,
			})
		}
	}

	sort.Slice(diff.Changed, func(i, j int) bool { return diff.Changed[i].Name < diff.Changed[j].Name })
	sort.Slice(diff.Added, func(i, j int) bool { return diff.Added[i].Name < diff.Added[j].Name })
	sort.Slice(diff.Removed, func(i, j int) bool { return diff.Removed[i].Name < diff.Removed[j].Name })
	sort.Strings(diff.PendingRestart)

	// Ensure non-nil slices for JSON.
	if diff.Changed == nil {
		diff.Changed = []SettingChange{}
	}
	if diff.Added == nil {
		diff.Added = []SettingChange{}
	}
	if diff.Removed == nil {
		diff.Removed = []SettingChange{}
	}
	if diff.PendingRestart == nil {
		diff.PendingRestart = []string{}
	}

	return diff
}
