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

// progressVacuumGate selects the pg_stat_progress_vacuum SQL by PG version.
//
// Both variants return the same 12 columns in the same order. Columns that do
// not exist on a version are returned as -1 sentinels and are not emitted:
//   - PG 14–16: dead_tuple_bytes / max_dead_tuple_bytes = -1 (not tracked)
//   - PG 17+:   max_dead_tuples = -1 (replaced by max_dead_tuple_bytes)
//
// On PG 17+ num_dead_item_ids is selected AS num_dead_tuples so the existing
// progress.vacuum.num_dead_tuples metric keeps reporting the dead-item count.
var progressVacuumGate = version.Gate{
	Name: "progress_vacuum",
	Variants: []version.SQLVariant{
		{
			// PG 14–16: dead tuples tracked as a tuple count with a tuple-count limit.
			Range: version.VersionRange{MinMajor: 14, MinMinor: 0, MaxMajor: 16, MaxMinor: 99},
			SQL: `
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
    p.num_dead_tuples,
    -1 AS dead_tuple_bytes,
    -1 AS max_dead_tuple_bytes
FROM pg_stat_progress_vacuum p
JOIN pg_stat_activity a ON a.pid = p.pid`,
		},
		{
			// PG 17+: dead-item storage is a TID store measured in bytes.
			Range: version.VersionRange{MinMajor: 17, MinMinor: 0, MaxMajor: 99, MaxMinor: 99},
			SQL: `
SELECT
    a.pid::text,
    COALESCE(a.datname, '') AS datname,
    COALESCE(p.relid::regclass::text, 'oid:' || p.relid::text) AS table_name,
    p.phase,
    p.heap_blks_total,
    p.heap_blks_scanned,
    p.heap_blks_vacuumed,
    p.index_vacuum_count,
    -1 AS max_dead_tuples,
    p.num_dead_item_ids AS num_dead_tuples,
    p.dead_tuple_bytes,
    p.max_dead_tuple_bytes
FROM pg_stat_progress_vacuum p
JOIN pg_stat_activity a ON a.pid = p.pid`,
		},
	},
}

// VacuumProgressCollector collects vacuum operation progress from pg_stat_progress_vacuum.
//
// PG 17 changed how VACUUM tracks dead items (TID store instead of a fixed
// tuple array) and pg_stat_progress_vacuum changed with it:
//   - max_dead_tuples (tuple count) → max_dead_tuple_bytes (bytes)
//   - num_dead_tuples (tuple count) → num_dead_item_ids (count) plus dead_tuple_bytes (bytes)
//
// Emitted metrics by version:
//
//   - progress.vacuum.num_dead_tuples: num_dead_tuples (14–16), num_dead_item_ids (17+)
//
//   - progress.vacuum.max_dead_tuples: 14–16 only
//
//   - progress.vacuum.dead_tuple_bytes, progress.vacuum.max_dead_tuple_bytes: 17+ only
//
//   - Tier: session
//
//   - Question: Which VACUUMs are running now, in which phase, and how far along are they?
//
//   - Views: pg_stat_progress_vacuum, pg_stat_activity
//
//   - Collection: 60s (medium group)
//
//   - PG versions: 14–16 / 17+ via progressVacuumGate
type VacuumProgressCollector struct {
	Base
	sqlGate version.Gate
}

// NewVacuumProgressCollector creates a new VacuumProgressCollector for the given instance.
func NewVacuumProgressCollector(instanceID string, v version.PGVersion) *VacuumProgressCollector {
	return &VacuumProgressCollector{
		Base:    newBase(instanceID, v, 10*time.Second),
		sqlGate: progressVacuumGate,
	}
}

// Name returns the collector's identifier.
func (c *VacuumProgressCollector) Name() string { return "progress_vacuum" }

// vacuumProgressRow holds one scanned row. Fields set to -1 are unavailable on
// the current PG version.
type vacuumProgressRow struct {
	pid, datname, tableName, phase       string
	blksTotal, blksScanned, blksVacuumed float64
	indexVacuumCount                     float64
	maxDeadTuples                        float64 // -1 on PG 17+
	numDeadTuples                        float64 // num_dead_item_ids on PG 17+
	deadTupleBytes                       float64 // -1 on PG <= 16
	maxDeadTupleBytes                    float64 // -1 on PG <= 16
}

// Collect queries vacuum progress and returns metric points.
// Works on both primary and replica instances.
// Returns empty slice when no vacuums are running.
func (c *VacuumProgressCollector) Collect(ctx context.Context, conn *pgx.Conn, _ InstanceContext) ([]MetricPoint, error) {
	sql, ok := c.sqlGate.Select(c.pgVersion)
	if !ok {
		return nil, fmt.Errorf("progress_vacuum: no SQL variant for PG %s", c.pgVersion.Full)
	}

	qCtx, cancel := queryContext(ctx)
	defer cancel()

	rows, err := conn.Query(qCtx, sql)
	if err != nil {
		return nil, fmt.Errorf("progress_vacuum: %w", err)
	}
	defer rows.Close()

	var points []MetricPoint

	for rows.Next() {
		var r vacuumProgressRow
		if err := rows.Scan(
			&r.pid, &r.datname, &r.tableName, &r.phase,
			&r.blksTotal, &r.blksScanned, &r.blksVacuumed,
			&r.indexVacuumCount, &r.maxDeadTuples, &r.numDeadTuples,
			&r.deadTupleBytes, &r.maxDeadTupleBytes,
		); err != nil {
			return nil, fmt.Errorf("progress_vacuum scan: %w", err)
		}
		points = append(points, c.rowPoints(r)...)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("progress_vacuum rows: %w", err)
	}

	return points, nil
}

// rowPoints converts one scanned row into metric points, skipping -1 sentinels.
func (c *VacuumProgressCollector) rowPoints(r vacuumProgressRow) []MetricPoint {
	labels := map[string]string{
		"pid":        r.pid,
		"datname":    r.datname,
		"table_name": r.tableName,
		"phase":      r.phase,
	}

	points := []MetricPoint{
		c.point("progress.vacuum.heap_blks_total", r.blksTotal, labels),
		c.point("progress.vacuum.heap_blks_scanned", r.blksScanned, labels),
		c.point("progress.vacuum.heap_blks_vacuumed", r.blksVacuumed, labels),
		c.point("progress.vacuum.index_vacuum_count", r.indexVacuumCount, labels),
	}
	if r.maxDeadTuples >= 0 {
		points = append(points, c.point("progress.vacuum.max_dead_tuples", r.maxDeadTuples, labels))
	}
	points = append(points, c.point("progress.vacuum.num_dead_tuples", r.numDeadTuples, labels))
	if r.deadTupleBytes >= 0 {
		points = append(points, c.point("progress.vacuum.dead_tuple_bytes", r.deadTupleBytes, labels))
	}
	if r.maxDeadTupleBytes >= 0 {
		points = append(points, c.point("progress.vacuum.max_dead_tuple_bytes", r.maxDeadTupleBytes, labels))
	}
	points = append(points, c.point("progress.vacuum.completion_pct", completionPct(r.blksScanned, r.blksTotal), labels))
	return points
}
