package collector

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/ios9000/PGPulse_01/internal/version"
)

const (
	// sqlConnectionsByState counts client backends by state, excluding the monitoring connection.
	// PGAM bug fix: WHERE pid != pg_backend_pid() prevents counting PGPulse's own connection.
	sqlConnectionsByState = `
SELECT COALESCE(state, 'unknown') AS state, count(*) AS cnt
FROM pg_stat_activity
WHERE pid != pg_backend_pid()
  AND backend_type = 'client backend'
GROUP BY 1`

	// sqlConnectionsMaxReserved retrieves the connection ceiling and reserved slots.
	sqlConnectionsMaxReserved = `
SELECT
    current_setting('max_connections')::bigint AS max_conn,
    current_setting('superuser_reserved_connections')::bigint AS reserved`
)

// ConnectionsCollector collects connection count and utilization metrics.
// It covers PGAM queries Q11, Q12, and Q13, enhanced with per-state breakdown.
type ConnectionsCollector struct {
	Base
}

// NewConnectionsCollector creates a new ConnectionsCollector for the given PostgreSQL instance.
func NewConnectionsCollector(instanceID string, v version.PGVersion) *ConnectionsCollector {
	return &ConnectionsCollector{
		Base: newBase(instanceID, v, 10*time.Second),
	}
}

// Name returns the collector's identifier.
func (c *ConnectionsCollector) Name() string { return "connections" }

// Collect executes connection queries and returns metric points.
// Emits: connections.by_state (per state label), connections.total,
// connections.max, connections.superuser_reserved, connections.utilization_pct.
func (c *ConnectionsCollector) Collect(ctx context.Context, conn *pgx.Conn, _ InstanceContext) ([]MetricPoint, error) {
	var points []MetricPoint
	var total float64

	// Q11: connection counts by state, excluding this monitoring connection
	tctx, cancel := queryContext(ctx)
	rows, err := conn.Query(tctx, sqlConnectionsByState)
	cancel()
	if err != nil {
		return nil, fmt.Errorf("connections collect states: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var state string
		var cnt int64
		if err := rows.Scan(&state, &cnt); err != nil {
			return nil, fmt.Errorf("connections scan state row: %w", err)
		}
		points = append(points, c.point("connections.by_state", float64(cnt), map[string]string{"state": state}))
		total += float64(cnt)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("connections iterate state rows: %w", err)
	}

	points = append(points, c.point("connections.total", total, nil))

	// Q12+Q13: max_connections and superuser_reserved_connections
	tctx2, cancel2 := queryContext(ctx)
	var maxConn, reserved int64
	err = conn.QueryRow(tctx2, sqlConnectionsMaxReserved).Scan(&maxConn, &reserved)
	cancel2()
	if err != nil {
		return nil, fmt.Errorf("connections collect max/reserved: %w", err)
	}
	points = append(points, c.point("connections.max", float64(maxConn), nil))
	points = append(points, c.point("connections.superuser_reserved", float64(reserved), nil))

	// Utilization = active connections / available slots * 100
	available := maxConn - reserved
	var utilization float64
	if available > 0 {
		utilization = total / float64(available) * 100
	} else {
		utilization = 100.0 // edge case: no available slots
	}
	points = append(points, c.point("connections.utilization_pct", utilization, nil))

	return points, nil
}
