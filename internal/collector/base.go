package collector

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/ios9000/PGPulse_01/internal/version"
)

const (
	defaultTimeout = 5 * time.Second
	metricPrefix   = "pgpulse"
)

// Base provides common fields and helpers shared by all collectors.
type Base struct {
	instanceID string
	pgVersion  version.PGVersion
	interval   time.Duration
}

// newBase creates a Base with the given instance ID, PG version, and collection interval.
func newBase(instanceID string, v version.PGVersion, interval time.Duration) Base {
	return Base{instanceID: instanceID, pgVersion: v, interval: interval}
}

// point creates a MetricPoint with the metric name prefixed by "pgpulse.",
// InstanceID set to b.instanceID, and Timestamp set to the current time.
func (b *Base) point(metric string, value float64, labels map[string]string) MetricPoint {
	return MetricPoint{
		InstanceID: b.instanceID,
		Metric:     metricPrefix + "." + metric,
		Value:      value,
		Labels:     labels,
		Timestamp:  time.Now(),
	}
}

// Interval returns the collection interval for this collector.
func (b *Base) Interval() time.Duration {
	return b.interval
}

// queryContext returns a child context with the default 5-second statement timeout.
// The caller must call the returned CancelFunc to release resources.
func queryContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, defaultTimeout)
}

// pgssAvailable checks whether the pg_stat_statements extension is installed.
func pgssAvailable(ctx context.Context, conn *pgx.Conn) (bool, error) {
	qCtx, cancel := queryContext(ctx)
	defer cancel()
	var exists bool
	err := conn.QueryRow(qCtx,
		`SELECT EXISTS(SELECT 1 FROM pg_extension WHERE extname = 'pg_stat_statements')`,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check pgss availability: %w", err)
	}
	return exists, nil
}
