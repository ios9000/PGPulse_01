package rca

import (
	"context"
	"time"

	"github.com/ios9000/PGPulse_01/internal/settings"
)

// SnapshotSettingsProvider implements SettingsProvider by diffing adjacent
// settings snapshots from the PGSnapshotStore.
type SnapshotSettingsProvider struct {
	store *settings.PGSnapshotStore
}

// NewSnapshotSettingsProvider creates a SettingsProvider backed by settings snapshots.
func NewSnapshotSettingsProvider(store *settings.PGSnapshotStore) *SnapshotSettingsProvider {
	return &SnapshotSettingsProvider{store: store}
}

// GetChanges returns settings that changed between from and to by examining
// snapshots within that window. It loads snapshots around the window boundaries,
// diffs them, and returns the changes.
func (p *SnapshotSettingsProvider) GetChanges(ctx context.Context, instanceID string, from, to time.Time) ([]SettingChange, error) {
	// List snapshots for this instance (get enough to cover the window).
	metas, err := p.store.ListSnapshots(ctx, instanceID, 50)
	if err != nil {
		return nil, err
	}

	if len(metas) < 2 {
		return nil, nil // need at least two snapshots to detect changes
	}

	// Find snapshots within or near the window.
	// We want pairs of adjacent snapshots where the later one falls within [from, to].
	var changes []SettingChange

	for i := 0; i < len(metas)-1; i++ {
		newer := metas[i]   // ListSnapshots returns DESC order
		older := metas[i+1]

		// Check if the newer snapshot falls within the window.
		if newer.CapturedAt.Before(from) || newer.CapturedAt.After(to) {
			continue
		}

		// Load full snapshots.
		newerSnap, err := p.store.GetSnapshot(ctx, newer.ID)
		if err != nil {
			continue
		}
		olderSnap, err := p.store.GetSnapshot(ctx, older.ID)
		if err != nil {
			continue
		}

		// Diff the two snapshots.
		diff := settings.DiffSnapshots(olderSnap.Settings, newerSnap.Settings)

		for _, ch := range diff.Changed {
			changes = append(changes, SettingChange{
				Name:      ch.Name,
				OldValue:  ch.OldValue,
				NewValue:  ch.NewValue,
				ChangedAt: newer.CapturedAt,
			})
		}
	}

	return changes, nil
}
