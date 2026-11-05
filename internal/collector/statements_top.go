package collector

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/ios9000/PGPulse_01/internal/version"
)

// statementsTopSQL builds the top-N query over source, which must expose
// queryid, dbid, userid, calls, rows, total_exec_time, blk_read_time and
// blk_write_time with pg_stat_statements ≤ 1.10 semantics.
//
// The query selects the top-N queries by total execution time along with
// aggregate totals for all queries. The totals row is cross-joined so that every
// result row carries the same totals, enabling "other" bucket computation.
// Ranking once on total_exec_time and deriving both IO time and CPU time from that
// single scan avoids separate IO-sorted and CPU-sorted passes over pg_stat_statements.
func statementsTopSQL(source string) string {
	return `
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
    FROM ` + source + `
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
}

// pgss111TimingSource adapts pg_stat_statements 1.11 (PG 17+) to the pre-1.11
// column names. Before 1.11, blk_read_time / blk_write_time covered both shared
// and local block I/O; 1.11 split them into shared_blk_*_time and
// local_blk_*_time, so summing the two preserves the old meaning.
const pgss111TimingSource = `(
        SELECT
            queryid, dbid, userid, calls, rows, total_exec_time,
            shared_blk_read_time + local_blk_read_time   AS blk_read_time,
            shared_blk_write_time + local_blk_write_time AS blk_write_time
        FROM pg_stat_statements
    ) s`

// statementsTopGate selects the pg_stat_statements top-N SQL by PG version.
//
// PG 14–16 (pg_stat_statements ≤ 1.10): reads blk_read_time / blk_write_time directly.
// PG 17+ (pg_stat_statements 1.11): those columns were renamed to
// shared_blk_read_time / shared_blk_write_time and local_blk_read_time /
// local_blk_write_time were added; the variant reads them through
// pgss111TimingSource so emitted metric keys and semantics are unchanged.
var statementsTopGate = version.Gate{
	Name: "statements_top",
	Variants: []version.SQLVariant{
		{
			Range: version.VersionRange{MinMajor: 14, MinMinor: 0, MaxMajor: 16, MaxMinor: 99},
			SQL:   statementsTopSQL("pg_stat_statements"),
		},
		{
			Range: version.VersionRange{MinMajor: 17, MinMinor: 0, MaxMajor: 99, MaxMinor: 99},
			SQL:   statementsTopSQL(pgss111TimingSource),
		},
	},
}

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
//
//   - Tier: postgres
//   - Question: Which queries consume the most execution time, and how much of it is I/O versus CPU?
//   - Views: pg_stat_statements
//   - Collection: 60s (medium group); no-op when pg_stat_statements is not installed
//   - PG versions: 14–16 / 17+ via statementsTopGate (PG 17 renamed blk_*_time to shared_blk_*_time and added local_blk_*_time)
type StatementsTopCollector struct {
	Base
	sqlGate version.Gate
	limit   int
}

// NewStatementsTopCollector creates a new StatementsTopCollector.
// limit controls how many top queries to return; values ≤ 0 default to 20.
func NewStatementsTopCollector(instanceID string, v version.PGVersion, limit int) *StatementsTopCollector {
	if limit <= 0 {
		limit = 20
	}
	return &StatementsTopCollector{
		Base:    newBase(instanceID, v, 60*time.Second),
		sqlGate: statementsTopGate,
		limit:   limit,
	}
}

// Name returns the collector's identifier.
func (c *StatementsTopCollector) Name() string { return "statements_top" }

// Collect queries pg_stat_statements for the top-N queries by total execution time.
// Returns nil, nil when pg_stat_statements is not installed.
// Returns an empty slice when pg_stat_statements is installed but has no entries.
func (c *StatementsTopCollector) Collect(ctx context.Context, conn *pgx.Conn, _ InstanceContext) ([]MetricPoint, error) {
	sql, ok := c.sqlGate.Select(c.pgVersion)
	if !ok {
		return nil, fmt.Errorf("statements_top: no SQL variant for PG %s", c.pgVersion.Full)
	}

	ok, err := pgssAvailable(ctx, conn)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}

	qCtx, cancel := queryContext(ctx)
	defer cancel()

	rows, err := conn.Query(qCtx, sql, c.limit)
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
