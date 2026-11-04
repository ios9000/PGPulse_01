package collector

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/ios9000/PGPulse_01/internal/version"
)

const (
	// sqlPGSSInstalled checks whether pg_stat_statements is installed.
	sqlPGSSInstalled = `
SELECT count(*) FROM pg_extension WHERE extname = 'pg_stat_statements'`

	// sqlPGSSFillPct computes the fill percentage of the pg_stat_statements ring buffer.
	sqlPGSSFillPct = `
SELECT count(*) * 100.0 / current_setting('pg_stat_statements.max')::float AS fill_pct
FROM pg_stat_statements`

	// sqlPGSSStatsReset retrieves the last stats reset time from pg_stat_statements_info (PG ≥ 14).
	sqlPGSSStatsReset = `
SELECT COALESCE(extract(epoch FROM stats_reset)::bigint, 0) AS reset_epoch
FROM pg_stat_statements_info`
)

// ExtensionsCollector reports whether pg_stat_statements is installed and,
// if so, how full its entry buffer is and when its statistics were last reset.
//
//	Tier:       environment
//	Question:   Is pg_stat_statements installed, and is its buffer or reset age about to make query statistics unreliable?
//	Reads:      pg_extension, pg_stat_statements, pg_stat_statements_info, current_setting('pg_stat_statements.max')
//	Collection: 300s
//	Versions:   PG 14+ (pg_stat_statements_info exists from PG 14)
//
// A buffer near 100% means the least-used entries are being evicted and
// top-query rankings drift; a very recent reset means the rankings have not
// had time to become representative. Fill and reset failures are logged and
// skipped so the installed/not-installed signal is always emitted.
type ExtensionsCollector struct {
	Base
}

// NewExtensionsCollector creates a new ExtensionsCollector for the given PostgreSQL instance.
func NewExtensionsCollector(instanceID string, v version.PGVersion) *ExtensionsCollector {
	return &ExtensionsCollector{
		Base: newBase(instanceID, v, 300*time.Second),
	}
}

// Name returns the collector's identifier.
func (c *ExtensionsCollector) Name() string { return "extensions" }

// Collect checks extension state and returns metric points.
// Emits: extensions.pgss_installed.
// If pg_stat_statements is installed, also emits:
// extensions.pgss_fill_pct, extensions.pgss_stats_reset_unix.
func (c *ExtensionsCollector) Collect(ctx context.Context, conn *pgx.Conn, _ InstanceContext) ([]MetricPoint, error) {
	var points []MetricPoint

	// Is pg_stat_statements installed?
	tctx, cancel := queryContext(ctx)
	var pgssCount int64
	err := conn.QueryRow(tctx, sqlPGSSInstalled).Scan(&pgssCount)
	cancel()
	if err != nil {
		return nil, fmt.Errorf("extensions collect pgss_installed: %w", err)
	}

	installed := pgssCount > 0
	pgssVal := 0.0
	if installed {
		pgssVal = 1.0
	}
	points = append(points, c.point("extensions.pgss_installed", pgssVal, nil))

	if !installed {
		return points, nil
	}

	// pg_stat_statements buffer fill percentage.
	tctx2, cancel2 := queryContext(ctx)
	var fillPct float64
	err = conn.QueryRow(tctx2, sqlPGSSFillPct).Scan(&fillPct)
	cancel2()
	if err != nil {
		slog.Warn("extensions: failed to collect pgss fill_pct, skipping",
			"error", err)
	} else {
		points = append(points, c.point("extensions.pgss_fill_pct", fillPct, nil))
	}

	// Stats reset time — pg_stat_statements_info exists on PG ≥ 14 (our minimum).
	if c.pgVersion.AtLeast(14, 0) {
		tctx3, cancel3 := queryContext(ctx)
		var resetEpoch int64
		err = conn.QueryRow(tctx3, sqlPGSSStatsReset).Scan(&resetEpoch)
		cancel3()
		if err != nil {
			slog.Warn("extensions: failed to collect pgss stats_reset, skipping",
				"error", err)
		} else {
			points = append(points, c.point("extensions.pgss_stats_reset_unix", float64(resetEpoch), nil))
		}
	}

	return points, nil
}
