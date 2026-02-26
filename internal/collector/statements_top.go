package collector

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/ios9000/PGPulse_01/internal/version"
)

// sqlStatementsTop selects the top-N queries by total execution time along with
// aggregate totals for all queries. The totals row is cross-joined so that every
// result row carries the same totals, enabling "other" bucket computation.
// PGAM source: analiz2.php Q50 (IO-sorted) and Q51 (CPU-sorted) — unified here
// by ranking on total_exec_time and deriving both IO and CPU from that single scan.
const sqlStatementsTop = `
WITH ranked AS (
    SELECT
        queryid::text                                        AS queryid,
        dbid::text                                           AS dbid,
        userid::text                                         AS userid,
        calls,
        rows,
        total_exec_time                                      AS total_time_ms,
        blk_read_time + blk_write_time                       AS io_time_ms,
        total_exec_time - blk_read_time - blk_write_time     AS cpu_time_ms,
        total_exec_time / calls                              AS avg_time_ms,
        ROW_NUMBER() OVER (ORDER BY total_exec_time DESC)    AS rn
    FROM pg_stat_statements
    WHERE calls > 0
),
totals AS (
    SELECT
        sum(calls)::float8          AS total_calls,
        sum(rows)::float8           AS total_rows,
        sum(total_time_ms)::float8  AS total_time,
        sum(io_time_ms)::float8     AS total_io,
        sum(cpu_time_ms)::float8    AS total_cpu
    FROM ranked
)
SELECT
    r.queryid,
    r.dbid,
    r.userid,
    r.calls::float8,
    r.rows::float8,
    r.total_time_ms,
    r.io_time_ms,
    r.cpu_time_ms,
    r.avg_time_ms,
    t.total_calls,
    t.total_rows,
    t.total_time,
    t.total_io,
    t.total_cpu
FROM ranked r
CROSS JOIN totals t
WHERE r.rn <= $1
ORDER BY r.rn`

// topQueryRow holds one scanned row from the statements top query.
// The ttl* fields come from the CROSS JOIN totals CTE and are identical in every row.
type topQueryRow struct {
	queryID   string
	dbID      string
	userID    string
	calls     float64
	rowCount  float64
	totalTime float64
	ioTime    float64
	cpuTime   float64
	avgTime   float64
	// totals — same value in every row
	ttlCalls float64
	ttlRows  float64
	ttlTime  float64
	ttlIO    float64
	ttlCPU   float64
}

// StatementsTopCollector collects top-N pg_stat_statements entries by total
// execution time and emits an "other" aggregate for the remaining queries.
// PGAM source: analiz2.php Q50 and Q51.
type StatementsTopCollector struct {
	Base
	limit int
}

// NewStatementsTopCollector creates a new StatementsTopCollector.
// limit controls how many top queries to return; values ≤ 0 default to 20.
func NewStatementsTopCollector(instanceID string, v version.PGVersion, limit int) *StatementsTopCollector {
	if limit <= 0 {
		limit = 20
	}
	return &StatementsTopCollector{
		Base:  newBase(instanceID, v, 60*time.Second),
		limit: limit,
	}
}

// Name returns the collector's identifier.
func (c *StatementsTopCollector) Name() string { return "statements_top" }

// Collect queries pg_stat_statements for the top-N queries by total execution time.
// Returns nil, nil when pg_stat_statements is not installed.
// Returns an empty slice when pg_stat_statements is installed but has no entries.
func (c *StatementsTopCollector) Collect(ctx context.Context, conn *pgx.Conn, _ InstanceContext) ([]MetricPoint, error) {
	ok, err := pgssAvailable(ctx, conn)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}

	qCtx, cancel := queryContext(ctx)
	defer cancel()

	rows, err := conn.Query(qCtx, sqlStatementsTop, c.limit)
	if err != nil {
		return nil, fmt.Errorf("statements_top: query: %w", err)
	}
	defer rows.Close()

	var scanned []topQueryRow
	for rows.Next() {
		var row topQueryRow
		if err := rows.Scan(
			&row.queryID, &row.dbID, &row.userID,
			&row.calls, &row.rowCount,
			&row.totalTime, &row.ioTime, &row.cpuTime, &row.avgTime,
			&row.ttlCalls, &row.ttlRows, &row.ttlTime, &row.ttlIO, &row.ttlCPU,
		); err != nil {
			return nil, fmt.Errorf("statements_top: scan: %w", err)
		}
		scanned = append(scanned, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("statements_top: rows: %w", err)
	}

	return c.buildTopMetrics(scanned), nil
}

// buildTopMetrics converts scanned rows into MetricPoints.
// For each top-N row it emits 6 metrics. When the top-N rows account for fewer
// calls than the total, it also emits 6 "other" bucket metrics.
// Extracted as a method so it can be tested without a database connection.
func (c *StatementsTopCollector) buildTopMetrics(rows []topQueryRow) []MetricPoint {
	if len(rows) == 0 {
		return []MetricPoint{}
	}

	// Totals are the same in every row (CROSS JOIN with single-row CTE).
	ttlCalls := rows[0].ttlCalls
	ttlRows := rows[0].ttlRows
	ttlTime := rows[0].ttlTime
	ttlIO := rows[0].ttlIO
	ttlCPU := rows[0].ttlCPU

	var points []MetricPoint
	// Track unclamped sums for accurate "other" bucket arithmetic.
	var sumCalls, sumRows, sumTime, sumIO, sumCPU float64

	for _, row := range rows {
		// Clamp cpu_time to 0 — can go negative when IO timing > exec time due to
		// measurement imprecision at very short query durations.
		cpuTime := row.cpuTime
		if cpuTime < 0 {
			cpuTime = 0
		}

		labels := map[string]string{
			"queryid": row.queryID,
			"dbid":    row.dbID,
			"userid":  row.userID,
		}
		points = append(points,
			c.point("statements.top.total_time_ms", row.totalTime, labels),
			c.point("statements.top.io_time_ms", row.ioTime, labels),
			c.point("statements.top.cpu_time_ms", cpuTime, labels),
			c.point("statements.top.calls", row.calls, labels),
			c.point("statements.top.rows", row.rowCount, labels),
			c.point("statements.top.avg_time_ms", row.avgTime, labels),
		)

		// Accumulate unclamped values so "other" = totals - actual top-N sums.
		sumCalls += row.calls
		sumRows += row.rowCount
		sumTime += row.totalTime
		sumIO += row.ioTime
		sumCPU += row.cpuTime
	}

	// Emit "other" bucket only when there are queries beyond the top-N.
	otherCalls := ttlCalls - sumCalls
	if otherCalls > 0 {
		otherRows := ttlRows - sumRows
		otherTime := ttlTime - sumTime
		otherIO := ttlIO - sumIO
		otherCPU := ttlCPU - sumCPU
		if otherCPU < 0 {
			otherCPU = 0
		}
		otherAvg := otherTime / otherCalls // safe: otherCalls > 0

		otherLabels := map[string]string{
			"queryid": "other",
			"dbid":    "all",
			"userid":  "all",
		}
		points = append(points,
			c.point("statements.top.total_time_ms", otherTime, otherLabels),
			c.point("statements.top.io_time_ms", otherIO, otherLabels),
			c.point("statements.top.cpu_time_ms", otherCPU, otherLabels),
			c.point("statements.top.calls", otherCalls, otherLabels),
			c.point("statements.top.rows", otherRows, otherLabels),
			c.point("statements.top.avg_time_ms", otherAvg, otherLabels),
		)
	}

	return points
}
