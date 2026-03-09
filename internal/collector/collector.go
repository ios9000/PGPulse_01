package collector

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

// MetricPoint represents a single metric data point collected from PostgreSQL.
type MetricPoint struct {
	InstanceID string
	Metric     string
	Value      float64
	Labels     map[string]string
	Timestamp  time.Time
}

// InstanceContext holds per-scrape-cycle metadata about the PostgreSQL instance.
// It is queried once by the orchestrator at the start of each collection cycle
// and passed to all collectors, avoiding redundant per-collector queries.
type InstanceContext struct {
	// IsRecovery is true when the instance is a standby (replica).
	// Derived from pg_is_in_recovery() queried once per cycle by the orchestrator.
	IsRecovery bool
}

// Collector is the interface that all metric collectors must implement.
type Collector interface {
	// Name returns the collector's identifier (e.g., "instance", "replication").
	Name() string
	// Collect executes queries and returns metric points.
	// ic provides per-cycle instance state (e.g., recovery role) without additional queries.
	Collect(ctx context.Context, conn *pgx.Conn, ic InstanceContext) ([]MetricPoint, error)
	// Interval returns how often this collector should run.
	Interval() time.Duration
}

// MetricQuery defines parameters for querying stored metrics.
type MetricQuery struct {
	InstanceID string
	Metric     string            // optional: filter by metric name prefix
	Labels     map[string]string // optional: filter by label values
	Start      time.Time         // time range start
	End        time.Time         // time range end
	Limit      int               // max results (0 = no limit)
}

// MetricStore is the interface for time-series metric storage.
type MetricStore interface {
	// Write persists a batch of metric points.
	Write(ctx context.Context, points []MetricPoint) error
	// Query retrieves metric points matching the query parameters.
	Query(ctx context.Context, query MetricQuery) ([]MetricPoint, error)
	// Close releases storage resources.
	Close() error
}

// AlertEvaluator processes metric values against alert rules.
type AlertEvaluator interface {
	// Evaluate checks a metric value against configured thresholds.
	Evaluate(ctx context.Context, metric string, value float64, labels map[string]string) error
}

// Queryer defines the minimal SQL execution interface.
// Both *pgx.Conn and *pgxpool.Pool satisfy this interface natively.
// Using Queryer instead of *pgx.Conn or *pgxpool.Pool enables mock injection
// in unit tests without spinning up a real PostgreSQL instance.
type Queryer interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// DBCollector collects metrics for a single database.
// Dispatched once per discovered database per collection cycle by the orchestrator.
// Contrast with Collector, which operates at the instance level.
type DBCollector interface {
	Name() string
	Interval() time.Duration
	CollectDB(ctx context.Context, q Queryer, dbName string, ic InstanceContext) ([]MetricPoint, error)
}
