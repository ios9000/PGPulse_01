// Package forecast implements maintenance operation tracking, ETA estimation,
// and predictive need forecasting for PostgreSQL maintenance operations.
package forecast

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

// --- Operation tracking ---

// TrackedOperation is the in-memory state of an active maintenance operation.
// Keyed by "{instance}:{pid}:{operation}" in OperationTracker.activeOps.
type TrackedOperation struct {
	InstanceID     string
	PID            int
	Operation      string // "vacuum", "analyze", "reindex_concurrent", "basebackup"
	Database       string
	Table          string
	TableSizeBytes int64
	StartedAt      time.Time
	LastSeenAt     time.Time
	MissedPolls    int              // consecutive polls where this op was absent from progress metrics
	Samples        []ProgressSample // ring buffer for WMA, capped at ETAWindowSize
}

// ProgressSample is a single progress observation for WMA calculation.
type ProgressSample struct {
	Timestamp time.Time
	WorkDone  float64 // blocks scanned, bytes processed, etc.
	WorkTotal float64 // total blocks/bytes (may update between samples)
	PctDone   float64 // 0.0–100.0
}

// MaintenanceOperation is a completed operation record persisted to the database.
type MaintenanceOperation struct {
	ID             int64          `json:"id"`
	InstanceID     string         `json:"instance_id"`
	Operation      string         `json:"operation"`
	Outcome        string         `json:"outcome"` // "completed", "canceled", "failed", "disappeared", "unknown"
	Database       string         `json:"database"`
	Table          string         `json:"table_name"`
	TableSizeBytes int64          `json:"table_size_bytes"`
	StartedAt      time.Time      `json:"started_at"`
	CompletedAt    time.Time      `json:"completed_at"`
	DurationSec    float64        `json:"duration_sec"`
	FinalPct       float64        `json:"final_pct"`
	AvgRatePerSec  float64        `json:"avg_rate_per_sec"`
	Metadata       map[string]any `json:"metadata"`
	CreatedAt      time.Time      `json:"created_at"`
}

// --- ETA ---

// OperationETA is the real-time ETA for a single in-progress operation.
type OperationETA struct {
	InstanceID  string    `json:"instance_id"`
	PID         int       `json:"pid"`
	Operation   string    `json:"operation"`
	Database    string    `json:"database"`
	Table       string    `json:"table"`
	Phase       string    `json:"phase"`
	PercentDone float64   `json:"percent_done"`
	StartedAt   time.Time `json:"started_at"`
	ElapsedSec  float64   `json:"elapsed_sec"`
	ETASec      float64   `json:"eta_sec"`      // -1 if stalled or estimating
	ETAAt       time.Time `json:"eta_at"`        // zero if stalled or estimating
	RateCurrent float64   `json:"rate_current"`  // work units per second (WMA)
	Confidence  string    `json:"confidence"`    // "high", "medium", "estimating", "stalled"
	SampleCount int       `json:"sample_count"`
}

// --- Need forecasting ---

// MaintenanceForecast is a predicted maintenance need for a specific table/operation.
type MaintenanceForecast struct {
	ID               int64      `json:"id"`
	InstanceID       string     `json:"instance_id"`
	Database         string     `json:"database"`
	Table            string     `json:"table_name"`
	Operation        string     `json:"operation"`
	Status           string     `json:"status"` // "predicted", "imminent", "overdue", "not_needed", "insufficient_data"
	PredictedAt      *time.Time `json:"predicted_at"`
	TimeUntilSec     float64    `json:"time_until_sec"`
	ConfidenceLower  *time.Time `json:"confidence_lower"`
	ConfidenceUpper  *time.Time `json:"confidence_upper"`
	CurrentValue     float64    `json:"current_value"`
	ThresholdValue   float64    `json:"threshold_value"`
	AccumulationRate float64    `json:"accumulation_rate"`
	Method           string     `json:"method"` // "threshold_projection", "threshold_projection+ml"
	EvaluatedAt      time.Time  `json:"evaluated_at"`
}

// ForecastSummary is the aggregated summary returned by the needs endpoint.
type ForecastSummary struct {
	ImminentCount        int `json:"imminent_count"`
	OverdueCount         int `json:"overdue_count"`
	PredictedCount       int `json:"predicted_count"`
	TotalTablesEvaluated int `json:"total_tables_evaluated"`
}

// --- Threshold calculation ---

// TableThresholds holds the effective autovacuum/autoanalyze thresholds for a table.
type TableThresholds struct {
	Database              string
	Schema                string
	Table                 string
	RelTuples             int64
	AutovacuumEnabled     bool
	VacuumThreshold       int
	VacuumScaleFactor     float64
	AnalyzeThreshold      int
	AnalyzeScaleFactor    float64
	EffectiveVacuumLimit  float64 // threshold + scale_factor * reltuples
	EffectiveAnalyzeLimit float64
}

// --- Interfaces consumed by forecast package ---

// InstanceConnProvider provides database connections to monitored instances.
// Mirrors internal/api.InstanceConnProvider without creating a circular import.
type InstanceConnProvider interface {
	ConnFor(ctx context.Context, instanceID string) (*pgx.Conn, error)
	ConnForDB(ctx context.Context, instanceID, dbName string) (*pgx.Conn, error)
}

// BaselineProvider is an optional interface for ML-enhanced forecasting.
// Nil if ML is disabled. C2/Option B: deferred to M15_02.
type BaselineProvider interface {
	GetBaselineStats(instanceID, metricKey string) (trend []float64, residualStdDev float64, ok bool)
}

// InstanceLister returns the list of active instance IDs.
type InstanceLister interface {
	ActiveInstanceIDs(ctx context.Context) ([]string, error)
}

// ThresholdQuerier executes pg_settings + reloptions queries on target instances.
type ThresholdQuerier interface {
	GetTableThresholds(ctx context.Context, instanceID string) ([]TableThresholds, error)
}
