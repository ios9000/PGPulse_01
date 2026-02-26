package collector

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/ios9000/PGPulse_01/internal/version"
)

// sqlWaitEvents counts active client backends grouped by wait event.
// NULL wait_event_type means the backend is running on CPU (not waiting);
// COALESCE maps this to 'CPU' / 'Running' so it appears in the output
// alongside real wait events rather than being silently dropped.
// PGAM source: analiz2.php Q53/Q54.
const sqlWaitEvents = `
SELECT
    COALESCE(wait_event_type, 'CPU') AS wait_event_type,
    COALESCE(wait_event, 'Running')  AS wait_event,
    count(*)                         AS cnt
FROM pg_stat_activity
WHERE pid != pg_backend_pid()
  AND backend_type = 'client backend'
GROUP BY 1, 2
ORDER BY 3 DESC`

// waitEventRow holds one scanned row from the wait events query.
type waitEventRow struct {
	eventType string
	event     string
	count     float64
}

// WaitEventsCollector reports per-wait-event backend counts and a total.
// PGAM source: analiz2.php Q53/Q54.
type WaitEventsCollector struct {
	Base
}

// NewWaitEventsCollector creates a new WaitEventsCollector for the given instance.
func NewWaitEventsCollector(instanceID string, v version.PGVersion) *WaitEventsCollector {
	return &WaitEventsCollector{
		Base: newBase(instanceID, v, 10*time.Second),
	}
}

// Name returns the collector's identifier.
func (c *WaitEventsCollector) Name() string { return "wait_events" }

// Collect queries pg_stat_activity for current wait events among client backends.
func (c *WaitEventsCollector) Collect(ctx context.Context, conn *pgx.Conn, _ InstanceContext) ([]MetricPoint, error) {
	qCtx, cancel := queryContext(ctx)
	defer cancel()

	rows, err := conn.Query(qCtx, sqlWaitEvents)
	if err != nil {
		return nil, fmt.Errorf("wait_events: %w", err)
	}
	defer rows.Close()

	var scanned []waitEventRow
	for rows.Next() {
		var row waitEventRow
		if err := rows.Scan(&row.eventType, &row.event, &row.count); err != nil {
			return nil, fmt.Errorf("wait_events scan: %w", err)
		}
		scanned = append(scanned, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("wait_events rows: %w", err)
	}

	return c.buildMetrics(scanned), nil
}

// buildMetrics converts scanned rows into MetricPoints.
// Extracted as a method so it can be tested without a database connection.
func (c *WaitEventsCollector) buildMetrics(rows []waitEventRow) []MetricPoint {
	var points []MetricPoint
	var total float64

	for _, row := range rows {
		labels := map[string]string{
			"wait_event_type": row.eventType,
			"wait_event":      row.event,
		}
		points = append(points, c.point("wait_events.count", row.count, labels))
		total += row.count
	}

	// Always emit total, even when 0 rows (total_backends = 0 is meaningful).
	points = append(points, c.point("wait_events.total_backends", total, nil))
	return points
}
