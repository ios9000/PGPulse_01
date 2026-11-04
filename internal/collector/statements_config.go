package collector

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/ios9000/PGPulse_01/internal/version"
)

const sqlStatementsSettings = `
SELECT name, setting
FROM pg_settings
WHERE name IN ('pg_stat_statements.max', 'pg_stat_statements.track', 'track_io_timing')`

const sqlStatementsCount = `SELECT count(*)::float8 FROM pg_stat_statements`

const sqlStatementsResetAge = `
SELECT EXTRACT(EPOCH FROM now() - stats_reset)
FROM pg_stat_statements_info`

// StatementsConfigCollector reports whether pg_stat_statements is configured
// to produce trustworthy query statistics: track mode, I/O timing, entry
// count against pg_stat_statements.max, and time since the last reset.
//
//	Tier:       postgres
//	Question:   Can the top-query statistics be trusted — is tracking on, is the entry buffer overflowing, and how long has it been accumulating?
//	Reads:      pg_settings, pg_stat_statements, pg_stat_statements_info, pg_extension
//	Collection: 60s
//	Versions:   PG 14+ (pg_stat_statements_info exists from PG 14; returns nil when the extension is not installed)
//
// stats_reset is NULL when the statistics have never been reset, in which case
// the reset-age metric is omitted rather than reported as 0.
type StatementsConfigCollector struct {
	Base
}

// NewStatementsConfigCollector creates a new StatementsConfigCollector for the given instance.
func NewStatementsConfigCollector(instanceID string, v version.PGVersion) *StatementsConfigCollector {
	return &StatementsConfigCollector{
		Base: newBase(instanceID, v, 60*time.Second),
	}
}

// Name returns the collector's identifier.
func (c *StatementsConfigCollector) Name() string { return "statements_config" }

// Collect queries pg_stat_statements settings, entry count, and reset age.
// Returns nil, nil when pg_stat_statements is not installed.
func (c *StatementsConfigCollector) Collect(ctx context.Context, conn *pgx.Conn, _ InstanceContext) ([]MetricPoint, error) {
	ok, err := pgssAvailable(ctx, conn)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}

	// Query the relevant GUC settings.
	qCtx, cancel := queryContext(ctx)
	settingsRows, err := conn.Query(qCtx, sqlStatementsSettings)
	cancel()
	if err != nil {
		return nil, fmt.Errorf("statements_config: settings: %w", err)
	}
	settings := make(map[string]string, 3)
	for settingsRows.Next() {
		var name, setting string
		if scanErr := settingsRows.Scan(&name, &setting); scanErr != nil {
			settingsRows.Close()
			return nil, fmt.Errorf("statements_config: settings scan: %w", scanErr)
		}
		settings[name] = setting
	}
	if rowsErr := settingsRows.Err(); rowsErr != nil {
		settingsRows.Close()
		return nil, fmt.Errorf("statements_config: settings rows: %w", rowsErr)
	}
	settingsRows.Close()

	// Query the statement entry count.
	qCtx2, cancel2 := queryContext(ctx)
	var count float64
	err = conn.QueryRow(qCtx2, sqlStatementsCount).Scan(&count)
	cancel2()
	if err != nil {
		return nil, fmt.Errorf("statements_config: count: %w", err)
	}

	// Query reset age from pg_stat_statements_info.
	// Available since PG 14 (our minimum). stats_reset is NULL when never reset.
	qCtx3, cancel3 := queryContext(ctx)
	var resetAge *float64
	err = conn.QueryRow(qCtx3, sqlStatementsResetAge).Scan(&resetAge)
	cancel3()
	if err != nil {
		return nil, fmt.Errorf("statements_config: reset_age: %w", err)
	}

	return c.buildMetrics(settings, count, resetAge), nil
}

// buildMetrics converts raw settings and measurements into MetricPoints.
// Extracted as a method so it can be tested without a database connection.
func (c *StatementsConfigCollector) buildMetrics(settings map[string]string, count float64, resetAge *float64) []MetricPoint {
	var points []MetricPoint

	// pgpulse.statements.max + pgpulse.statements.fill_pct
	if maxStr, ok := settings["pg_stat_statements.max"]; ok {
		maxVal, err := strconv.ParseFloat(maxStr, 64)
		if err == nil {
			points = append(points, c.point("statements.max", maxVal, nil))
			// Skip fill_pct when max is 0 to avoid division by zero.
			if maxVal > 0 {
				points = append(points, c.point("statements.fill_pct", count/maxVal*100, nil))
			}
		}
	}

	// pgpulse.statements.track — value=1 with the track mode as a label.
	if track, ok := settings["pg_stat_statements.track"]; ok {
		points = append(points, c.point("statements.track", 1, map[string]string{"value": track}))
	}

	// pgpulse.statements.track_io_timing — 1 if on, 0 otherwise.
	if timing, ok := settings["track_io_timing"]; ok {
		timingVal := 0.0
		if timing == "on" {
			timingVal = 1.0
		}
		points = append(points, c.point("statements.track_io_timing", timingVal, nil))
	}

	// pgpulse.statements.count
	points = append(points, c.point("statements.count", count, nil))

	// pgpulse.statements.stats_reset_age_seconds — omitted when stats_reset is NULL.
	if resetAge != nil {
		points = append(points, c.point("statements.stats_reset_age_seconds", *resetAge, nil))
	}

	return points
}
