package collector

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/ios9000/PGPulse_01/internal/version"
)

// longTxnThreshold is the minimum transaction age to be considered "long".
// Matches PGAM's hardcoded 5-second threshold. Will become configurable in M2.
const longTxnThreshold = "5 seconds"

// sqlLongTransactions counts active client backends with long-running transactions,
// split into "active" (running) and "waiting" (blocked on a lock or event).
// Merges PGAM Q56 (active) and Q57 (waiting) into a single parameterized query.
// $1 is the minimum transaction age interval (e.g. "5 seconds").
const sqlLongTransactions = `
SELECT
    CASE WHEN wait_event IS NULL THEN 'active' ELSE 'waiting' END AS txn_type,
    count(*)                                                       AS cnt,
    COALESCE(extract(epoch FROM max(now() - xact_start)), 0)      AS oldest_seconds
FROM pg_stat_activity
WHERE xact_start < now() - $1::interval
  AND state = 'active'
  AND pid != pg_backend_pid()
  AND backend_type = 'client backend'
GROUP BY 1`

// longTxnRow holds one scanned row from the long transactions query.
type longTxnRow struct {
	txnType       string
	count         float64
	oldestSeconds float64
}

// LongTransactionsCollector reports counts and maximum age of long-running
// active and waiting transactions. Always emits exactly 4 metric points
// (2 types × 2 metrics), using 0 for types with no matching transactions.
// PGAM source: analiz2.php Q56/Q57.
type LongTransactionsCollector struct {
	Base
}

// NewLongTransactionsCollector creates a new LongTransactionsCollector for the given instance.
func NewLongTransactionsCollector(instanceID string, v version.PGVersion) *LongTransactionsCollector {
	return &LongTransactionsCollector{
		Base: newBase(instanceID, v, 10*time.Second),
	}
}

// Name returns the collector's identifier.
func (c *LongTransactionsCollector) Name() string { return "long_transactions" }

// Collect queries pg_stat_activity for long-running transactions.
// Always emits 4 points: count and oldest_seconds for each of "active" and "waiting".
// Missing categories (no rows for that type) are emitted as 0.
func (c *LongTransactionsCollector) Collect(ctx context.Context, conn *pgx.Conn, _ InstanceContext) ([]MetricPoint, error) {
	qCtx, cancel := queryContext(ctx)
	defer cancel()

	rows, err := conn.Query(qCtx, sqlLongTransactions, longTxnThreshold)
	if err != nil {
		return nil, fmt.Errorf("long_transactions: %w", err)
	}
	defer rows.Close()

	var scanned []longTxnRow
	for rows.Next() {
		var row longTxnRow
		if err := rows.Scan(&row.txnType, &row.count, &row.oldestSeconds); err != nil {
			return nil, fmt.Errorf("long_transactions scan: %w", err)
		}
		scanned = append(scanned, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("long_transactions rows: %w", err)
	}

	return c.buildMetrics(scanned), nil
}

// buildMetrics converts scanned rows into MetricPoints.
// Always returns exactly 4 points — zero-fills missing transaction types.
// Extracted as a method so it can be tested without a database connection.
func (c *LongTransactionsCollector) buildMetrics(rows []longTxnRow) []MetricPoint {
	seen := make(map[string]bool)
	var points []MetricPoint

	for _, row := range rows {
		labels := map[string]string{"type": row.txnType}
		points = append(points,
			c.point("long_transactions.count", row.count, labels),
			c.point("long_transactions.oldest_seconds", row.oldestSeconds, labels),
		)
		seen[row.txnType] = true
	}

	// Emit zero-value points for any transaction type not present in the result.
	for _, t := range []string{"active", "waiting"} {
		if !seen[t] {
			labels := map[string]string{"type": t}
			points = append(points,
				c.point("long_transactions.count", 0, labels),
				c.point("long_transactions.oldest_seconds", 0, labels),
			)
		}
	}

	return points
}
