package collector

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/ios9000/PGPulse_01/internal/version"
)

const sqlDatabaseSizes = `
SELECT
    datname,
    pg_database_size(datname) AS size_bytes
FROM pg_database
WHERE datistemplate = false
ORDER BY size_bytes DESC`

// DatabaseSizesCollector reports the on-disk size of every non-template database.
//
//	Tier:       database
//	Question:   How big is each database, and which ones are growing?
//	Reads:      pg_database, pg_database_size()
//	Collection: 300s
//	Versions:   PG 14+
//
// Template databases are excluded because they never change size in normal
// operation and would only add noise to growth forecasting.
type DatabaseSizesCollector struct {
	Base
}

// NewDatabaseSizesCollector creates a new DatabaseSizesCollector for the given PostgreSQL instance.
func NewDatabaseSizesCollector(instanceID string, v version.PGVersion) *DatabaseSizesCollector {
	return &DatabaseSizesCollector{
		Base: newBase(instanceID, v, 300*time.Second),
	}
}

// Name returns the collector's identifier.
func (c *DatabaseSizesCollector) Name() string { return "database_sizes" }

// Collect executes database size queries and returns metric points.
// Emits: database.size_bytes labeled by database name.
func (c *DatabaseSizesCollector) Collect(ctx context.Context, conn *pgx.Conn, _ InstanceContext) ([]MetricPoint, error) {
	tctx, cancel := queryContext(ctx)
	rows, err := conn.Query(tctx, sqlDatabaseSizes)
	cancel()
	if err != nil {
		return nil, fmt.Errorf("database_sizes collect: %w", err)
	}
	defer rows.Close()

	var points []MetricPoint
	for rows.Next() {
		var dbName string
		var sizeBytes int64
		if err := rows.Scan(&dbName, &sizeBytes); err != nil {
			return nil, fmt.Errorf("database_sizes scan row: %w", err)
		}
		points = append(points, c.point("database.size_bytes", float64(sizeBytes), map[string]string{"database": dbName}))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("database_sizes iterate rows: %w", err)
	}

	return points, nil
}
