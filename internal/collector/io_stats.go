package collector

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/ios9000/PGPulse_01/internal/version"
)

// sqlIOStats queries I/O statistics from pg_stat_io.
// Available since PostgreSQL 16. COALESCE translates NULL to -1 for columns
// that are not applicable for a given (backend_type, object, context) combination.
const sqlIOStats = `
SELECT
    backend_type,
    object,
    context,
    COALESCE(reads, -1)       AS reads,
    COALESCE(read_time, -1)   AS read_time,
    COALESCE(writes, -1)      AS writes,
    COALESCE(write_time, -1)  AS write_time,
    COALESCE(extends, -1)     AS extends,
    COALESCE(extend_time, -1) AS extend_time,
    COALESCE(hits, -1)        AS hits,
    COALESCE(evictions, -1)   AS evictions,
    COALESCE(reuses, -1)      AS reuses,
    COALESCE(fsyncs, -1)      AS fsyncs,
    COALESCE(fsync_time, -1)  AS fsync_time
FROM pg_stat_io`

// ioStatsRow holds one scanned row from pg_stat_io.
// Any field that was NULL in the view is represented as -1.
type ioStatsRow struct {
	backendType string
	object      string
	ioContext   string // named ioContext to avoid shadowing the "context" import
	reads       float64
	readTime    float64
	writes      float64
	writeTime   float64
	extends     float64
	extendTime  float64
	hits        float64
	evictions   float64
	reuses      float64
	fsyncs      float64
	fsyncTime   float64
}

// IOStatsCollector collects I/O statistics from pg_stat_io (PostgreSQL 16+).
// Returns nil, nil for PostgreSQL < 16 where the view does not exist.
type IOStatsCollector struct {
	Base
}

// NewIOStatsCollector creates a new IOStatsCollector for the given instance.
func NewIOStatsCollector(instanceID string, v version.PGVersion) *IOStatsCollector {
	return &IOStatsCollector{
		Base: newBase(instanceID, v, 60*time.Second),
	}
}

// Name returns the collector's identifier.
func (c *IOStatsCollector) Name() string { return "io_stats" }

// Collect queries pg_stat_io and returns metric points.
// Returns nil, nil on PostgreSQL < 16 where pg_stat_io does not exist.
// Returns an empty slice when the view exists but has no rows.
// Metrics with a -1 sentinel value (column not tracked for this combination) are not emitted.
func (c *IOStatsCollector) Collect(ctx context.Context, conn *pgx.Conn, _ InstanceContext) ([]MetricPoint, error) {
	if c.pgVersion.Major < 16 {
		return nil, nil
	}

	qCtx, cancel := queryContext(ctx)
	defer cancel()

	rows, err := conn.Query(qCtx, sqlIOStats)
	if err != nil {
		return nil, fmt.Errorf("io_stats: %w", err)
	}
	defer rows.Close()

	var points []MetricPoint

	for rows.Next() {
		var row ioStatsRow
		if err := rows.Scan(
			&row.backendType, &row.object, &row.ioContext,
			&row.reads, &row.readTime,
			&row.writes, &row.writeTime,
			&row.extends, &row.extendTime,
			&row.hits, &row.evictions, &row.reuses,
			&row.fsyncs, &row.fsyncTime,
		); err != nil {
			return nil, fmt.Errorf("io_stats scan: %w", err)
		}
		points = append(points, c.rowPoints(row)...)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("io_stats rows: %w", err)
	}

	return points, nil
}

// rowPoints converts a scanned ioStatsRow into MetricPoints.
// Fields with value -1 (NULL sentinel) are not emitted.
func (c *IOStatsCollector) rowPoints(row ioStatsRow) []MetricPoint {
	labels := map[string]string{
		"backend_type": row.backendType,
		"object":       row.object,
		"context":      row.ioContext,
	}

	type mv struct {
		name string
		val  float64
	}
	candidates := []mv{
		{"io.reads", row.reads},
		{"io.read_time", row.readTime},
		{"io.writes", row.writes},
		{"io.write_time", row.writeTime},
		{"io.extends", row.extends},
		{"io.extend_time", row.extendTime},
		{"io.hits", row.hits},
		{"io.evictions", row.evictions},
		{"io.reuses", row.reuses},
		{"io.fsyncs", row.fsyncs},
		{"io.fsync_time", row.fsyncTime},
	}

	var points []MetricPoint
	for _, m := range candidates {
		if m.val >= 0 {
			points = append(points, c.point(m.name, m.val, labels))
		}
	}
	return points
}
