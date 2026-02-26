package collector

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/ios9000/PGPulse_01/internal/version"
)

// completionPct computes percentage safely. Returns 0 when total is 0.
// Used by all progress collectors.
func completionPct(done, total float64) float64 {
	if total <= 0 {
		return 0
	}
	return (done / total) * 100
}

// progressVacuumSQL queries active vacuum operations with progress stats.
// PGAM source: analiz2.php Q42.
const progressVacuumSQL = `
SELECT
    a.pid::text,
    COALESCE(a.datname, '') AS datname,
    COALESCE(p.relid::regclass::text, 'oid:' || p.relid::text) AS table_name,
    p.phase,
    p.heap_blks_total,
    p.heap_blks_scanned,
    p.heap_blks_vacuumed,
    p.index_vacuum_count,
    p.max_dead_tuples,
    p.num_dead_tuples
FROM pg_stat_progress_vacuum p
JOIN pg_stat_activity a ON a.pid = p.pid`

// VacuumProgressCollector collects vacuum operation progress from pg_stat_progress_vacuum.
// PGAM source: analiz2.php Q42.
type VacuumProgressCollector struct {
	Base
}

// NewVacuumProgressCollector creates a new VacuumProgressCollector for the given instance.
func NewVacuumProgressCollector(instanceID string, v version.PGVersion) *VacuumProgressCollector {
	return &VacuumProgressCollector{
		Base: newBase(instanceID, v, 10*time.Second),
	}
}

// Name returns the collector's identifier.
func (c *VacuumProgressCollector) Name() string { return "progress_vacuum" }

// Collect queries vacuum progress and returns metric points.
// Works on both primary and replica instances.
// Returns empty slice when no vacuums are running.
func (c *VacuumProgressCollector) Collect(ctx context.Context, conn *pgx.Conn, _ InstanceContext) ([]MetricPoint, error) {
	qCtx, cancel := queryContext(ctx)
	defer cancel()

	rows, err := conn.Query(qCtx, progressVacuumSQL)
	if err != nil {
		return nil, fmt.Errorf("progress_vacuum: %w", err)
	}
	defer rows.Close()

	var points []MetricPoint

	for rows.Next() {
		var (
			pid, datname, tableName, phase                        string
			blksTotal, blksScanned, blksVacuumed                  float64
			indexVacuumCount, maxDeadTuples, numDeadTuples float64
		)
		if err := rows.Scan(
			&pid, &datname, &tableName, &phase,
			&blksTotal, &blksScanned, &blksVacuumed,
			&indexVacuumCount, &maxDeadTuples, &numDeadTuples,
		); err != nil {
			return nil, fmt.Errorf("progress_vacuum scan: %w", err)
		}

		labels := map[string]string{
			"pid":        pid,
			"datname":    datname,
			"table_name": tableName,
			"phase":      phase,
		}

		pct := completionPct(blksScanned, blksTotal)

		points = append(points,
			c.point("progress.vacuum.heap_blks_total", blksTotal, labels),
			c.point("progress.vacuum.heap_blks_scanned", blksScanned, labels),
			c.point("progress.vacuum.heap_blks_vacuumed", blksVacuumed, labels),
			c.point("progress.vacuum.index_vacuum_count", indexVacuumCount, labels),
			c.point("progress.vacuum.max_dead_tuples", maxDeadTuples, labels),
			c.point("progress.vacuum.num_dead_tuples", numDeadTuples, labels),
			c.point("progress.vacuum.completion_pct", pct, labels),
		)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("progress_vacuum rows: %w", err)
	}

	return points, nil
}
