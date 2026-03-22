package rca

import (
	"context"
	"time"
)

// SettingChange represents a PostgreSQL configuration parameter that changed
// within the analysis window.
type SettingChange struct {
	Name     string    // parameter name, e.g., "shared_buffers"
	OldValue string    // value before the change
	NewValue string    // value after the change
	ChangedAt time.Time // when the change was detected (snapshot timestamp)
}

// SettingsProvider retrieves configuration changes for an instance
// within a given time window.
type SettingsProvider interface {
	// GetChanges returns all settings that changed between from and to
	// for the given instance.
	GetChanges(ctx context.Context, instanceID string, from, to time.Time) ([]SettingChange, error)
}
