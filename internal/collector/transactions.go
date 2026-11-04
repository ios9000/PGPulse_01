package collector

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/ios9000/PGPulse_01/internal/version"
)

const sqlTransactions = `
SELECT
    d.datname,
    COALESCE(
        s.xact_commit * 100.0 / NULLIF(s.xact_commit + s.xact_rollback, 0),
        0
    ) AS commit_ratio,
    s.deadlocks
FROM pg_stat_database s
JOIN pg_database d ON d.oid = s.datid
WHERE d.datistemplate = false
  AND s.xact_commit + s.xact_rollback > 0`

// TransactionsCollector reports the commit/rollback ratio and cumulative
// deadlock count for each database.
//
//	Tier:       database
//	Question:   Which databases are rolling back an unusual share of transactions or hitting deadlocks?
//	Reads:      pg_stat_database, pg_database
//	Collection: 300s
//	Versions:   PG 14+
//
// Databases with no transactions yet are skipped so the ratio is only emitted
// where it is meaningful; NULLIF/COALESCE guard the division for the same
// reason. deadlocks is a cumulative counter since the last stats reset.
type TransactionsCollector struct {
	Base
}

// NewTransactionsCollector creates a new TransactionsCollector for the given PostgreSQL instance.
func NewTransactionsCollector(instanceID string, v version.PGVersion) *TransactionsCollector {
	return &TransactionsCollector{
		Base: newBase(instanceID, v, 60*time.Second),
	}
}

// Name returns the collector's identifier.
func (c *TransactionsCollector) Name() string { return "transactions" }

// Collect executes transaction statistics queries and returns metric points.
// Emits: transactions.commit_ratio_pct and transactions.deadlocks, both labeled by database.
func (c *TransactionsCollector) Collect(ctx context.Context, conn *pgx.Conn, _ InstanceContext) ([]MetricPoint, error) {
	tctx, cancel := queryContext(ctx)
	rows, err := conn.Query(tctx, sqlTransactions)
	cancel()
	if err != nil {
		return nil, fmt.Errorf("transactions collect: %w", err)
	}
	defer rows.Close()

	var points []MetricPoint
	for rows.Next() {
		var dbName string
		var commitRatio float64
		var deadlocks int64
		if err := rows.Scan(&dbName, &commitRatio, &deadlocks); err != nil {
			return nil, fmt.Errorf("transactions scan row: %w", err)
		}
		labels := map[string]string{"database": dbName}
		points = append(points, c.point("transactions.commit_ratio_pct", commitRatio, labels))
		points = append(points, c.point("transactions.deadlocks", float64(deadlocks), labels))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("transactions iterate rows: %w", err)
	}

	return points, nil
}
