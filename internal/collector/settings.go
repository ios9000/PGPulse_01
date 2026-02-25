package collector

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/ios9000/PGPulse_01/internal/version"
)

const sqlSettings = `
SELECT name, setting
FROM pg_settings
WHERE name IN (
    'track_io_timing',
    'shared_buffers',
    'max_locks_per_transaction',
    'max_prepared_transactions'
)`

// settingDef maps a pg_settings name to its metric name and conversion type.
type settingDef struct {
	metric   string
	boolType bool // true = "on"/"off" → 1.0/0.0; false = parse as float64
}

// settingsMap defines how each pg_settings row is converted to a MetricPoint.
// Adding a new setting requires only one entry here.
var settingsMap = map[string]settingDef{
	"track_io_timing":           {metric: "settings.track_io_timing", boolType: true},
	"shared_buffers":            {metric: "settings.shared_buffers_8kb", boolType: false},
	"max_locks_per_transaction": {metric: "settings.max_locks_per_tx", boolType: false},
	"max_prepared_transactions": {metric: "settings.max_prepared_tx", boolType: false},
}

// SettingsCollector collects key PostgreSQL runtime settings as metric values.
// It covers PGAM query Q17, extended with additional settings.
type SettingsCollector struct {
	Base
}

// NewSettingsCollector creates a new SettingsCollector for the given PostgreSQL instance.
func NewSettingsCollector(instanceID string, v version.PGVersion) *SettingsCollector {
	return &SettingsCollector{
		Base: newBase(instanceID, v, 300*time.Second),
	}
}

// Name returns the collector's identifier.
func (c *SettingsCollector) Name() string { return "settings" }

// Collect executes the settings query and returns metric points.
// Emits: settings.track_io_timing, settings.shared_buffers_8kb,
// settings.max_locks_per_tx, settings.max_prepared_tx.
func (c *SettingsCollector) Collect(ctx context.Context, conn *pgx.Conn) ([]MetricPoint, error) {
	tctx, cancel := queryContext(ctx)
	rows, err := conn.Query(tctx, sqlSettings)
	cancel()
	if err != nil {
		return nil, fmt.Errorf("settings collect: %w", err)
	}
	defer rows.Close()

	var points []MetricPoint
	for rows.Next() {
		var name, setting string
		if err := rows.Scan(&name, &setting); err != nil {
			return nil, fmt.Errorf("settings scan row: %w", err)
		}
		def, ok := settingsMap[name]
		if !ok {
			continue // unknown setting, skip
		}
		var value float64
		if def.boolType {
			if setting == "on" {
				value = 1.0
			}
			// "off" → 0.0 (zero value)
		} else {
			value, err = strconv.ParseFloat(setting, 64)
			if err != nil {
				return nil, fmt.Errorf("settings parse %q value %q: %w", name, setting, err)
			}
		}
		points = append(points, c.point(def.metric, value, nil))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("settings iterate rows: %w", err)
	}

	return points, nil
}
