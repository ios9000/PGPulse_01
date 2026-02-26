package collector

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/ios9000/PGPulse_01/internal/version"
)

// progressClusterSQL queries active CLUSTER / VACUUM FULL operations.
// PGAM source: analiz2.php Q43.
const progressClusterSQL = `
SELECT
    a.pid::text,
    COALESCE(a.datname, '') AS datname,
    COALESCE(p.relid::regclass::text, 'oid:' || p.relid::text) AS table_name,
    p.command, p.phase,
    p.heap_tuples_scanned, p.heap_tuples_written,
    p.heap_blks_total, p.heap_blks_scanned,
    p.index_rebuild_count
FROM pg_stat_progress_cluster p
JOIN pg_stat_activity a ON a.pid = p.pid`

// ClusterProgressCollector collects CLUSTER/VACUUM FULL progress from pg_stat_progress_cluster.
// PGAM source: analiz2.php Q43.
type ClusterProgressCollector struct {
	Base
}

// NewClusterProgressCollector creates a new ClusterProgressCollector for the given instance.
func NewClusterProgressCollector(instanceID string, v version.PGVersion) *ClusterProgressCollector {
	return &ClusterProgressCollector{
		Base: newBase(instanceID, v, 10*time.Second),
	}
}

// Name returns the collector's identifier.
func (c *ClusterProgressCollector) Name() string { return "progress_cluster" }

// Collect queries cluster/vacuum full progress and returns metric points.
// Works on both primary and replica instances.
func (c *ClusterProgressCollector) Collect(ctx context.Context, conn *pgx.Conn, _ InstanceContext) ([]MetricPoint, error) {
	qCtx, cancel := queryContext(ctx)
	defer cancel()

	rows, err := conn.Query(qCtx, progressClusterSQL)
	if err != nil {
		return nil, fmt.Errorf("progress_cluster: %w", err)
	}
	defer rows.Close()

	var points []MetricPoint

	for rows.Next() {
		var (
			pid, datname, tableName, command, phase string
			tuplesScanned, tuplesWritten            float64
			blksTotal, blksScanned                  float64
			indexRebuildCount                       float64
		)
		if err := rows.Scan(
			&pid, &datname, &tableName, &command, &phase,
			&tuplesScanned, &tuplesWritten,
			&blksTotal, &blksScanned,
			&indexRebuildCount,
		); err != nil {
			return nil, fmt.Errorf("progress_cluster scan: %w", err)
		}

		labels := map[string]string{
			"pid":        pid,
			"datname":    datname,
			"table_name": tableName,
			"command":    command,
			"phase":      phase,
		}

		pct := completionPct(blksScanned, blksTotal)

		points = append(points,
			c.point("progress.cluster.heap_tuples_scanned", tuplesScanned, labels),
			c.point("progress.cluster.heap_tuples_written", tuplesWritten, labels),
			c.point("progress.cluster.heap_blks_total", blksTotal, labels),
			c.point("progress.cluster.heap_blks_scanned", blksScanned, labels),
			c.point("progress.cluster.index_rebuild_count", indexRebuildCount, labels),
			c.point("progress.cluster.completion_pct", pct, labels),
		)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("progress_cluster rows: %w", err)
	}

	return points, nil
}

// progressAnalyzeSQL queries active ANALYZE operations.
// PGAM source: analiz2.php Q45.
const progressAnalyzeSQL = `
SELECT
    a.pid::text,
    COALESCE(a.datname, '') AS datname,
    COALESCE(p.relid::regclass::text, 'oid:' || p.relid::text) AS table_name,
    p.phase,
    p.sample_blks_total, p.sample_blks_scanned,
    p.ext_stats_total, p.ext_stats_computed,
    p.child_tables_total, p.child_tables_done,
    COALESCE(p.current_child_table_relid::regclass::text, '') AS current_child
FROM pg_stat_progress_analyze p
JOIN pg_stat_activity a ON a.pid = p.pid`

// AnalyzeProgressCollector collects ANALYZE progress from pg_stat_progress_analyze.
// PGAM source: analiz2.php Q45.
type AnalyzeProgressCollector struct {
	Base
}

// NewAnalyzeProgressCollector creates a new AnalyzeProgressCollector for the given instance.
func NewAnalyzeProgressCollector(instanceID string, v version.PGVersion) *AnalyzeProgressCollector {
	return &AnalyzeProgressCollector{
		Base: newBase(instanceID, v, 10*time.Second),
	}
}

// Name returns the collector's identifier.
func (c *AnalyzeProgressCollector) Name() string { return "progress_analyze" }

// Collect queries analyze progress and returns metric points.
// Works on both primary and replica instances.
func (c *AnalyzeProgressCollector) Collect(ctx context.Context, conn *pgx.Conn, _ InstanceContext) ([]MetricPoint, error) {
	qCtx, cancel := queryContext(ctx)
	defer cancel()

	rows, err := conn.Query(qCtx, progressAnalyzeSQL)
	if err != nil {
		return nil, fmt.Errorf("progress_analyze: %w", err)
	}
	defer rows.Close()

	var points []MetricPoint

	for rows.Next() {
		var (
			pid, datname, tableName, phase, currentChild string
			sampleBlksTotal, sampleBlksScanned           float64
			extStatsTotal, extStatsComputed               float64
			childTablesTotal, childTablesDone             float64
		)
		if err := rows.Scan(
			&pid, &datname, &tableName, &phase,
			&sampleBlksTotal, &sampleBlksScanned,
			&extStatsTotal, &extStatsComputed,
			&childTablesTotal, &childTablesDone,
			&currentChild,
		); err != nil {
			return nil, fmt.Errorf("progress_analyze scan: %w", err)
		}

		labels := map[string]string{
			"pid":           pid,
			"datname":       datname,
			"table_name":    tableName,
			"phase":         phase,
			"current_child": currentChild,
		}

		pct := completionPct(sampleBlksScanned, sampleBlksTotal)

		points = append(points,
			c.point("progress.analyze.sample_blks_total", sampleBlksTotal, labels),
			c.point("progress.analyze.sample_blks_scanned", sampleBlksScanned, labels),
			c.point("progress.analyze.ext_stats_total", extStatsTotal, labels),
			c.point("progress.analyze.ext_stats_computed", extStatsComputed, labels),
			c.point("progress.analyze.child_tables_total", childTablesTotal, labels),
			c.point("progress.analyze.child_tables_done", childTablesDone, labels),
			c.point("progress.analyze.completion_pct", pct, labels),
		)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("progress_analyze rows: %w", err)
	}

	return points, nil
}
