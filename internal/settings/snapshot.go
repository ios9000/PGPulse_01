package settings

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// SettingValue represents a single pg_settings row.
type SettingValue struct {
	Setting        string `json:"setting"`
	Unit           string `json:"unit"`
	Source         string `json:"source"`
	PendingRestart bool   `json:"pending_restart"`
}

// Snapshot is a complete pg_settings snapshot for one instance.
type Snapshot struct {
	ID          int64                  `json:"id,omitempty"`
	InstanceID  string                 `json:"instance_id"`
	CapturedAt  time.Time              `json:"captured_at"`
	TriggerType string                 `json:"trigger_type"` // "startup" | "scheduled" | "manual"
	PGVersion   string                 `json:"pg_version"`
	Settings    map[string]SettingValue `json:"settings"`
}

// SnapshotStore persists settings snapshots.
type SnapshotStore interface {
	SaveSnapshot(ctx context.Context, s Snapshot) error
	GetSnapshot(ctx context.Context, id int64) (*Snapshot, error)
	ListSnapshots(ctx context.Context, instanceID string, limit int) ([]SnapshotMeta, error)
	LatestSnapshot(ctx context.Context, instanceID string) (*Snapshot, error)
}

// SnapshotMeta is a summary of a snapshot (without the full settings map).
type SnapshotMeta struct {
	ID          int64     `json:"id"`
	InstanceID  string    `json:"instance_id"`
	CapturedAt  time.Time `json:"captured_at"`
	TriggerType string    `json:"trigger_type"`
	PGVersion   string    `json:"pg_version"`
}

// SnapshotConfig configures the settings snapshot collector.
type SnapshotConfig struct {
	Enabled           bool
	ScheduledInterval time.Duration
	CaptureOnStartup  bool
	RetentionDays     int
}

// SnapshotCollector captures pg_settings snapshots on schedule or on demand.
type SnapshotCollector struct {
	config          SnapshotConfig
	store           SnapshotStore
	mu              sync.Mutex
	capturedOnStart map[string]bool
	lastScheduled   map[string]time.Time
}

// NewSnapshotCollector creates a settings snapshot collector.
func NewSnapshotCollector(cfg SnapshotConfig, store SnapshotStore) *SnapshotCollector {
	return &SnapshotCollector{
		config:          cfg,
		store:           store,
		capturedOnStart: make(map[string]bool),
		lastScheduled:   make(map[string]time.Time),
	}
}

// Collect captures settings snapshots based on startup and scheduled triggers.
func (c *SnapshotCollector) Collect(ctx context.Context, pool *pgxpool.Pool, instanceID string) error {
	if !c.config.Enabled {
		return nil
	}

	c.mu.Lock()
	needsStartup := c.config.CaptureOnStartup && !c.capturedOnStart[instanceID]
	needsScheduled := time.Since(c.lastScheduled[instanceID]) >= c.config.ScheduledInterval
	c.mu.Unlock()

	if needsStartup {
		if err := c.capture(ctx, pool, instanceID, "startup"); err != nil {
			return fmt.Errorf("startup settings snapshot: %w", err)
		}
		c.mu.Lock()
		c.capturedOnStart[instanceID] = true
		c.mu.Unlock()
	} else if needsScheduled {
		if err := c.capture(ctx, pool, instanceID, "scheduled"); err != nil {
			return fmt.Errorf("scheduled settings snapshot: %w", err)
		}
		c.mu.Lock()
		c.lastScheduled[instanceID] = time.Now()
		c.mu.Unlock()
	}

	return nil
}

// CaptureManual triggers a manual settings snapshot.
func (c *SnapshotCollector) CaptureManual(ctx context.Context, pool *pgxpool.Pool, instanceID string) error {
	return c.capture(ctx, pool, instanceID, "manual")
}

func (c *SnapshotCollector) capture(ctx context.Context, pool *pgxpool.Pool, instanceID, trigger string) error {
	rows, err := pool.Query(ctx, `
		SELECT name, setting, COALESCE(unit, ''), source, pending_restart
		FROM pg_catalog.pg_settings
		ORDER BY name
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	settingsMap := make(map[string]SettingValue)
	for rows.Next() {
		var name, setting, unit, source string
		var pendingRestart bool
		if err := rows.Scan(&name, &setting, &unit, &source, &pendingRestart); err != nil {
			continue
		}
		settingsMap[name] = SettingValue{
			Setting:        setting,
			Unit:           unit,
			Source:         source,
			PendingRestart: pendingRestart,
		}
	}

	var version string
	_ = pool.QueryRow(ctx, "SELECT version()").Scan(&version)

	return c.store.SaveSnapshot(ctx, Snapshot{
		InstanceID:  instanceID,
		CapturedAt:  time.Now(),
		TriggerType: trigger,
		PGVersion:   version,
		Settings:    settingsMap,
	})
}
