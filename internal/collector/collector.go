// Package collector gathers the signals PGPulse uses for fleet incident
// intelligence: each collector answers one operational question about a
// monitored PostgreSQL instance and emits the metric points that back
// anomaly detection, root-cause analysis and alerting.
//
// Every collector type carries a structured doc comment with five fields,
// which docs/collectors.md is generated from:
//
//	Tier:       postgres | database | session | replication | patroni-etcd | pooler | os | environment
//	Question:   the one-sentence question a DBA gets answered
//	Reads:      the PostgreSQL views, functions or external sources consulted
//	Collection: 10s | 60s | 300s | per-database — the orchestrator group it runs in
//	Versions:   the PostgreSQL version gate, or "PG 14+" when the collector runs everywhere
//
// Tiers group collectors by the layer of the stack they observe:
//
//   - postgres: instance-wide server state (cache, checkpoints, settings, I/O, wraparound)
//   - database: per-database and per-relation analysis (sizes, bloat, vacuum need, indexes)
//   - session: what backends are doing right now (connections, waits, locks, long transactions, progress)
//   - replication: physical and logical replication health
//   - patroni-etcd: HA cluster membership and quorum
//   - pooler: connection pooler state (reserved; no collector yet)
//   - os: host resources of the monitored server
//   - environment: identity and configuration that changes rarely (server info, extensions)
//
// Collection cadence is decided by the orchestrator, which runs collectors in
// three shared ticker groups (high 10s, medium 60s, low 300s by default) plus
// a per-database runner that dispatches DBCollectors once per discovered
// database every 5 minutes. The Interval() a collector reports is advisory and
// is not consulted by the orchestrator; the Collection field documents the
// group a collector is actually registered in.
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
