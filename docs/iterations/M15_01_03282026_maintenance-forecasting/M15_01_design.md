# M15_01 — Design: Maintenance Operation Forecasting (Foundation + ETA + Need Forecasting)

**Iteration:** M15_01
**Date:** 2026-03-27
**Companion:** M15_01_requirements.md
**Locked Decisions:** D700–D711

---

## DO NOT RE-DISCUSS

All D400–D711 decisions are locked. Additionally:

- Core 4 operations only — not CLUSTER, CREATE INDEX, COPY
- WMA for ETA, not simple linear, not phase-aware
- Threshold projection primary + ML confidence bands — not ML-only
- OperationTracker in `internal/forecast/` — not in collectors
- NeedEvaluator as dedicated 5-min goroutine — not coupled to DatabaseCollector
- `internal/forecast/` for domain logic, `internal/ml/` for math — not monolithic
- 2 agents: Backend-heavy + Frontend-light
- REINDEX ETA via create_index progress only (REINDEX CONCURRENTLY)
- `maintenance_forecasts` uses UPSERT (one row per table/operation) — not append-only
- `maintenance_operations.outcome` field is mandatory — distinguishes completed/canceled/failed/disappeared/unknown
- Tracker debounce: `MissedPolls >= 2` before finalizing — single scrape gap does NOT trigger completion
- REINDEX CONCURRENTLY identification via `pg_stat_activity.query` substring match — NOT relname heuristics
- ETA minimum samples gate: `forecast.eta_min_samples` (default 4) — below this, return "estimating", never "high" confidence
- ETA is computed from in-memory tracker state — intentional exception to "all state in PostgreSQL" rule; does NOT survive restart
- Threshold querier uses transaction-scoped `BEGIN + SET LOCAL + ROLLBACK` — NOT session-level SET on pooled connections

---

## 1. Architecture Overview

M15_01 introduces `internal/forecast/` and extends `internal/ml/`, `internal/api/`, `internal/config/`, and the frontend:

```
NEW:  internal/forecast/          — domain logic: tracker, evaluator, ETA, store, types
NEW:  internal/ml/linear.go       — linear regression helper
NEW:  internal/ml/wma.go          — weighted moving average helper
NEW:  internal/api/forecast_maint.go — API handlers for forecast endpoints
NEW:  migrations/019_maintenance_forecasting.sql
MOD:  internal/config/config.go   — ForecastConfig struct
MOD:  cmd/pgpulse-server/main.go  — wire forecast subsystem
NEW:  web/src/hooks/useForecast.ts
NEW:  web/src/components/forecast/ — ETABadge, NeedForecastCard, NeedForecastTable, etc.
MOD:  web/src/pages/ProgressPage.tsx — ETA column
MOD:  web/src/pages/InstanceDashboard.tsx — NeedForecastCard
```

Data flow:

```
                      Progress Collectors (existing)
                      emit pg.progress.* metrics
                              │
          ┌───────────────────┼───────────────────┐
          │                   │                   │
          ▼                   ▼                   ▼
    MetricStore         MetricStore         MetricStore
    (vacuum)            (analyze)           (basebackup)
          │                   │                   │
          └───────────┬───────┘───────────────────┘
                      │
         ┌────────────┴────────────┐
         │                         │
         ▼                         ▼
  OperationTracker          ETACalculator
  (background goroutine)    (stateless, per-request)
         │                         │
         │ detects start/end       │ computes WMA rate
         │ writes completion       │ returns OperationETA
         ▼                         │
  maintenance_operations           │
  table (history)                  │
                                   ▼
                            API: GET /forecast/eta
                            ────────────────────

  DatabaseCollector (existing)
  emits pg.db.vacuum.* metrics
         │
         ▼
    MetricStore
    (dead_tuples, mod_since_analyze, bloat)
         │
         ▼
  NeedEvaluator
  (background goroutine, 5-min cycle)
         │
         ├─ reads pg_settings + reloptions (direct DB query, cached)
         ├─ computes accumulation rate (linear regression)
         ├─ projects threshold crossing time
         ├─ optionally enhances with ML confidence bands
         │
         ▼
  maintenance_forecasts
  table (UPSERT per table/operation)
         │
         ▼
  API: GET /forecast/needs
```

---

## 2. Go Types (`internal/forecast/types.go`)

```go
package forecast

import "time"

// --- Operation tracking ---

// TrackedOperation is the in-memory state of an active maintenance operation.
// Keyed by "{instance}:{pid}:{operation}" in OperationTracker.activeOps.
type TrackedOperation struct {
    InstanceID   string
    PID          int
    Operation    string    // "vacuum", "analyze", "reindex_concurrent", "basebackup"
    Database     string
    Table        string
    TableSizeBytes int64
    StartedAt    time.Time
    LastSeenAt   time.Time
    MissedPolls  int              // consecutive polls where this op was absent from progress metrics
    Samples      []ProgressSample // ring buffer for WMA, capped at ETAWindowSize
}

// ProgressSample is a single progress observation for WMA calculation.
type ProgressSample struct {
    Timestamp  time.Time
    WorkDone   float64   // blocks scanned, bytes processed, etc.
    WorkTotal  float64   // total blocks/bytes (may update between samples)
    PctDone    float64   // 0.0–100.0
}

// MaintenanceOperation is a completed operation record persisted to the database.
type MaintenanceOperation struct {
    ID              int64              `json:"id"`
    InstanceID      string             `json:"instance_id"`
    Operation       string             `json:"operation"`
    Outcome         string             `json:"outcome"`  // "completed", "canceled", "failed", "disappeared", "unknown"
    Database        string             `json:"database"`
    Table           string             `json:"table_name"`
    TableSizeBytes  int64              `json:"table_size_bytes"`
    StartedAt       time.Time          `json:"started_at"`
    CompletedAt     time.Time          `json:"completed_at"`
    DurationSec     float64            `json:"duration_sec"`
    FinalPct        float64            `json:"final_pct"`
    AvgRatePerSec   float64            `json:"avg_rate_per_sec"`
    Metadata        map[string]any     `json:"metadata"`
    CreatedAt       time.Time          `json:"created_at"`
}

// --- ETA ---

// OperationETA is the real-time ETA for a single in-progress operation.
type OperationETA struct {
    InstanceID    string    `json:"instance_id"`
    PID           int       `json:"pid"`
    Operation     string    `json:"operation"`
    Database      string    `json:"database"`
    Table         string    `json:"table"`
    Phase         string    `json:"phase"`
    PercentDone   float64   `json:"percent_done"`
    StartedAt     time.Time `json:"started_at"`
    ElapsedSec    float64   `json:"elapsed_sec"`
    ETASec        float64   `json:"eta_sec"`       // -1 if stalled
    ETAAt         time.Time `json:"eta_at"`        // zero if stalled
    RateCurrent   float64   `json:"rate_current"`  // work units per second (WMA)
    Confidence    string    `json:"confidence"`     // "high", "medium", "low"
    SampleCount   int       `json:"sample_count"`
}

// --- Need forecasting ---

// MaintenanceForecast is a predicted maintenance need for a specific table/operation.
type MaintenanceForecast struct {
    ID                int64      `json:"id"`
    InstanceID        string     `json:"instance_id"`
    Database          string     `json:"database"`
    Table             string     `json:"table_name"`
    Operation         string     `json:"operation"`
    Status            string     `json:"status"`  // "predicted", "imminent", "overdue", "not_needed", "insufficient_data"
    PredictedAt       *time.Time `json:"predicted_at"`
    TimeUntilSec      float64    `json:"time_until_sec"`
    ConfidenceLower   *time.Time `json:"confidence_lower"`
    ConfidenceUpper   *time.Time `json:"confidence_upper"`
    CurrentValue      float64    `json:"current_value"`
    ThresholdValue    float64    `json:"threshold_value"`
    AccumulationRate  float64    `json:"accumulation_rate"`
    Method            string     `json:"method"`  // "threshold_projection", "threshold_projection+ml"
    EvaluatedAt       time.Time  `json:"evaluated_at"`
}

// ForecastSummary is the aggregated summary returned by the needs endpoint.
type ForecastSummary struct {
    ImminentCount      int `json:"imminent_count"`
    OverdueCount       int `json:"overdue_count"`
    PredictedCount     int `json:"predicted_count"`
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
    VacuumThreshold       int     // effective (per-table override or global)
    VacuumScaleFactor     float64
    AnalyzeThreshold      int
    AnalyzeScaleFactor    float64
    EffectiveVacuumLimit  float64 // threshold + scale_factor * reltuples
    EffectiveAnalyzeLimit float64
}
```

---

## 3. Interfaces (`internal/forecast/store.go`)

```go
package forecast

import (
    "context"
    "time"
)

// ForecastStore persists completed operation history and cached forecasts.
type ForecastStore interface {
    // --- Operation history ---
    WriteOperation(ctx context.Context, op *MaintenanceOperation) error
    ListOperations(ctx context.Context, filter OperationFilter) ([]MaintenanceOperation, int, error)
    CleanOldOperations(ctx context.Context, olderThan time.Time) (int64, error)

    // --- Forecasts ---
    UpsertForecast(ctx context.Context, f *MaintenanceForecast) error
    UpsertForecasts(ctx context.Context, forecasts []MaintenanceForecast) error
    ListForecasts(ctx context.Context, filter ForecastFilter) ([]MaintenanceForecast, error)
    DeleteForecasts(ctx context.Context, instanceID string) error

    // --- Cleanup ---
    Close() error
}

// OperationFilter controls history queries.
type OperationFilter struct {
    InstanceID string
    Operation  string // empty = all
    Database   string // empty = all
    Table      string // empty = all
    Since      time.Time
    Limit      int
    Offset     int
}

// ForecastFilter controls forecast queries.
type ForecastFilter struct {
    InstanceID string
    Operation  string   // empty = all
    Database   string
    Table      string
    Statuses   []string // empty = all; e.g. ["imminent", "overdue"]
}
```

---

## 4. PostgreSQL Store (`internal/forecast/pgstore.go`)

```go
package forecast

// PGForecastStore implements ForecastStore using pgx.

func NewPGForecastStore(pool *pgxpool.Pool, logger *slog.Logger) *PGForecastStore

// WriteOperation — single INSERT into maintenance_operations.
//
// SQL:
//   INSERT INTO maintenance_operations
//     (instance_id, operation, outcome, database, table_name, table_size_bytes,
//      started_at, completed_at, duration_sec, final_pct, avg_rate_per_sec, metadata)
//   VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
//   RETURNING id

// ListOperations — filtered SELECT with pagination.
//
// SQL (dynamic WHERE clauses built via parameterized placeholders):
//   SELECT id, instance_id, operation, outcome, database, table_name, table_size_bytes,
//          started_at, completed_at, duration_sec, final_pct, avg_rate_per_sec,
//          metadata, created_at
//   FROM maintenance_operations
//   WHERE instance_id = $1
//     AND ($2 = '' OR operation = $2)
//     AND ($3 = '' OR database = $3)
//     AND ($4 = '' OR table_name = $4)
//     AND completed_at >= $5
//   ORDER BY completed_at DESC
//   LIMIT $6 OFFSET $7

// Also: SELECT COUNT(*) with same WHERE for total count.

// UpsertForecast — INSERT ON CONFLICT UPDATE keyed on (instance_id, database, table_name, operation).
//
// SQL:
//   INSERT INTO maintenance_forecasts
//     (instance_id, database, table_name, operation, status, predicted_at,
//      time_until_sec, confidence_lower, confidence_upper, current_value,
//      threshold_value, accumulation_rate, method, evaluated_at)
//   VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
//   ON CONFLICT (instance_id, database, table_name, operation) DO UPDATE SET
//     status = EXCLUDED.status,
//     predicted_at = EXCLUDED.predicted_at,
//     time_until_sec = EXCLUDED.time_until_sec,
//     confidence_lower = EXCLUDED.confidence_lower,
//     confidence_upper = EXCLUDED.confidence_upper,
//     current_value = EXCLUDED.current_value,
//     threshold_value = EXCLUDED.threshold_value,
//     accumulation_rate = EXCLUDED.accumulation_rate,
//     method = EXCLUDED.method,
//     evaluated_at = EXCLUDED.evaluated_at

// UpsertForecasts — wraps UpsertForecast in a transaction for batch writes.
// Uses a single transaction with prepared statement for efficiency.

// CleanOldOperations — DELETE FROM maintenance_operations WHERE completed_at < $1

// ListForecasts — filtered SELECT.
// SQL:
//   SELECT id, instance_id, database, table_name, operation, status,
//          predicted_at, time_until_sec, confidence_lower, confidence_upper,
//          current_value, threshold_value, accumulation_rate, method, evaluated_at
//   FROM maintenance_forecasts
//   WHERE instance_id = $1
//     AND ($2 = '' OR operation = $2)
//     AND ($3 = '' OR database = $3)
//     AND ($4 = '' OR table_name = $4)
//     AND (array_length($5::text[], 1) IS NULL OR status = ANY($5))
//   ORDER BY
//     CASE status
//       WHEN 'overdue' THEN 1
//       WHEN 'imminent' THEN 2
//       WHEN 'predicted' THEN 3
//       WHEN 'insufficient_data' THEN 4
//       WHEN 'not_needed' THEN 5
//     END,
//     time_until_sec ASC NULLS LAST
```

---

## 5. ML Helpers (`internal/ml/`)

### 5.1 Linear Regression (`internal/ml/linear.go`)

```go
package ml

// LinearRegressionResult holds the output of a simple OLS regression.
type LinearRegressionResult struct {
    Slope     float64 // dy/dx (rate of change)
    Intercept float64
    RSquared  float64 // goodness of fit (0.0–1.0)
    N         int     // number of data points
}

// LinearRegression computes simple OLS regression over (x, y) pairs.
// Returns error if len(xs) < 2 or len(xs) != len(ys).
//
// Uses gonum/stat if available, otherwise manual computation:
//   slope = (N * Σ(xy) - Σx * Σy) / (N * Σ(x²) - (Σx)²)
//   intercept = (Σy - slope * Σx) / N
//   R² = 1 - SS_res / SS_tot
func LinearRegression(xs, ys []float64) (LinearRegressionResult, error)
```

### 5.2 Weighted Moving Average (`internal/ml/wma.go`)

```go
package ml

// WMAConfig controls the weighted moving average behavior.
type WMAConfig struct {
    WindowSize  int     // max samples to consider (default 10)
    DecayFactor float64 // exponential decay for older samples (default 0.85)
}

// WMAResult holds the output of a weighted moving average calculation.
type WMAResult struct {
    WeightedRate float64 // weighted average rate (units per second)
    SampleCount  int     // actual samples used
    StdDev       float64 // weighted standard deviation of rates
}

// WeightedMovingAverage computes the WMA rate from a slice of (timestamp, value) pairs.
// Pairs must be sorted chronologically (oldest first).
// Returns error if fewer than 2 data points.
//
// Algorithm:
//   1. Compute per-interval rates: rate_i = (value_i - value_{i-1}) / (time_i - time_{i-1})
//   2. Apply exponential decay weights: weight_i = decay^(N - i - 1) where i=0 is oldest
//   3. weighted_rate = Σ(weight_i * rate_i) / Σ(weight_i)
//   4. weighted_stddev = sqrt(Σ(weight_i * (rate_i - weighted_rate)²) / Σ(weight_i))
func WeightedMovingAverage(cfg WMAConfig, timestamps []time.Time, values []float64) (WMAResult, error)
```

---

## 6. ETA Calculator (`internal/forecast/eta.go`)

```go
package forecast

import (
    "context"
    "time"

    "github.com/ios9000/PGPulse_01/internal/collector"
    "github.com/ios9000/PGPulse_01/internal/ml"
)

// ETACalculator computes real-time ETAs for in-progress maintenance operations.
// It is stateless — called per API request. The OperationTracker maintains the
// sample ring buffer; ETACalculator reads it.
//
// ARCHITECTURAL NOTE: ETA depends on the tracker's in-memory sample buffer.
// This is an intentional exception to the "all state in PostgreSQL" rule.
// ETA does NOT survive PGPulse restart — the tracker rebuilds its sample buffer
// from scratch after restart. This is acceptable because ETA is a real-time
// transient computation, not a persisted record. Do NOT "fix" this by persisting
// samples to the database — the overhead is not justified for a volatile metric.
type ETACalculator struct {
    tracker    *OperationTracker
    wmaCfg     ml.WMAConfig
    minSamples int  // minimum samples before emitting ETA (default 4, from forecast.eta_min_samples)
    logger     *slog.Logger
}

func NewETACalculator(tracker *OperationTracker, cfg ForecastConfig, logger *slog.Logger) *ETACalculator

// ComputeAll returns ETAs for all active operations on a given instance.
func (c *ETACalculator) ComputeAll(ctx context.Context, instanceID string) ([]OperationETA, error)

// ComputeByPID returns the ETA for a specific operation identified by PID.
func (c *ETACalculator) ComputeByPID(ctx context.Context, instanceID string, pid int) (*OperationETA, error)

// computeOne builds an OperationETA from a TrackedOperation's sample buffer.
//
// Logic:
//   1. If len(op.Samples) < c.minSamples → return ETA with ETASec=-1, Confidence="estimating"
//      (the UI renders "estimating..." — never show a numeric ETA with < minSamples)
//   2. Extract timestamps and workDone values from op.Samples
//   3. Call ml.WeightedMovingAverage(wmaCfg, timestamps, workDone)
//   4. remaining = workTotal - workDone (from latest sample)
//   5. If wmaResult.WeightedRate <= 0 → return ETA with ETASec=-1, Confidence="stalled"
//   6. etaSec = remaining / wmaResult.WeightedRate
//   7. confidence = classifyConfidence(wmaResult.SampleCount)
//   8. Assemble OperationETA
func (c *ETACalculator) computeOne(op *TrackedOperation, now time.Time) OperationETA

// classifyConfidence maps sample count to confidence level.
// Note: callers must check minSamples BEFORE calling this — if below minSamples,
// the ETA should not be emitted at all (see computeOne step 1).
//   ≥ 8 → "high"
//   4–7 → "medium"
//   (below minSamples is handled before this is called)
func classifyConfidence(sampleCount int) string
```

---

## 7. Operation Tracker (`internal/forecast/tracker.go`)

```go
package forecast

import (
    "context"
    "log/slog"
    "sync"
    "time"

    "github.com/ios9000/PGPulse_01/internal/collector"
)

// OperationTracker is a background goroutine that monitors progress metrics
// to detect when maintenance operations start and complete.
//
// Lifecycle:
//   engine := NewForecastEngine(...)
//   go engine.tracker.Run(ctx) // started by ForecastEngine.Start()
//   // runs until ctx is cancelled
//   // on shutdown, flushes any incomplete operations (marks as interrupted)
type OperationTracker struct {
    metricStore  collector.MetricStore
    forecastStore ForecastStore
    connProv     InstanceConnProvider // for REINDEX identification via pg_stat_activity
    logger       *slog.Logger
    pollInterval time.Duration  // matches progress collector interval (default 15s)
    windowSize   int            // WMA sample buffer capacity

    mu        sync.RWMutex
    activeOps map[string]*TrackedOperation // key: "{instance}:{pid}:{operation}"
}

func NewOperationTracker(
    metricStore collector.MetricStore,
    forecastStore ForecastStore,
    connProv InstanceConnProvider,
    pollInterval time.Duration,
    windowSize int,
    logger *slog.Logger,
) *OperationTracker

// Run is the main loop. Polls MetricStore every pollInterval.
//
// Pseudocode:
//   ticker := time.NewTicker(t.pollInterval)
//   for {
//     select {
//     case <-ctx.Done():
//       t.flushAll(ctx, "unknown")  // mark all active ops with outcome="unknown" (restart/shutdown)
//       return
//     case <-ticker.C:
//       t.tick(ctx)
//     }
//   }
func (t *OperationTracker) Run(ctx context.Context)

// tick executes one observation cycle:
//
//   1. Query MetricStore for all pg.progress.vacuum.completion_pct,
//      pg.progress.analyze.completion_pct, pg.progress.create_index.completion_pct,
//      pg.progress.basebackup.completion_pct metrics.
//
//   2. Build currentOps map from metric labels: instance_id, pid, datname, relname, phase
//
//   3. For each op in currentOps:
//      a. If not in t.activeOps → startOp()
//      b. If in t.activeOps → updateOp() — append sample, reset MissedPolls to 0
//
//   4. For each op in t.activeOps NOT in currentOps:
//      a. Increment op.MissedPolls
//      b. If op.MissedPolls < 2 → skip (debounce: tolerate single scrape gap)
//      c. If op.MissedPolls >= 2 → finalizeOp() with outcome classification:
//         - If last FinalPct >= 99.0 → outcome = "completed"
//         - If last FinalPct < 99.0 and elapsed > 2s → outcome = "disappeared"
//         - Else → outcome = "unknown"
//         Write MaintenanceOperation to ForecastStore, remove from map.
//
// IMPORTANT: The debounce prevents a single MetricStore scrape timeout or
// network hiccup from closing all active operations. Two consecutive absences
// (≥ 30s at default 15s poll) provide high confidence the operation truly ended.
func (t *OperationTracker) tick(ctx context.Context)

// GetActiveOps returns a snapshot of active operations for a given instance.
// Called by ETACalculator (read-only, under RLock).
func (t *OperationTracker) GetActiveOps(instanceID string) []TrackedOperation

// startOp creates a new TrackedOperation with initial sample and MissedPolls=0.
func (t *OperationTracker) startOp(key string, instanceID string, pid int, op string, db, table string, sample ProgressSample)

// updateOp appends a sample to the ring buffer, resets MissedPolls to 0.
// If buffer is full, oldest sample is evicted.
func (t *OperationTracker) updateOp(key string, sample ProgressSample)

// finalizeOp computes the MaintenanceOperation record and writes to store.
//
// Duration = lastSeen - startedAt
// AvgRate = total work done / duration
// FinalPct = last sample's PctDone
// Outcome = classified by caller (see tick step 4c)
// TableSizeBytes: looked up from latest pg.db.table.total_bytes metric
func (t *OperationTracker) finalizeOp(ctx context.Context, key string, outcome string)

// flushAll writes all active operations to store on shutdown with the given outcome.
// Called from Run() on context cancellation.
func (t *OperationTracker) flushAll(ctx context.Context, outcome string)

// REINDEX CONCURRENTLY detection:
//
// The tracker sees pg.progress.create_index.* metrics. To distinguish a regular
// CREATE INDEX from a REINDEX CONCURRENTLY, the tracker queries pg_stat_activity
// for the specific PID and checks if the query text contains "REINDEX".
//
// Hard gate: If the pg_stat_activity query fails, times out, or returns a query
// string that does NOT contain the substring "REINDEX" (case-insensitive), the
// operation is classified as "create_index" and is NOT tracked (not in Core 4 scope).
// Only confirmed REINDEX CONCURRENTLY operations are classified as "reindex_concurrent".
//
// This eliminates false positives from regular CREATE INDEX CONCURRENTLY operations
// which also use temporary naming patterns during the swap phase.
//
// Connection: uses the same InstanceConnProvider as the ThresholdQuerier.
// Query:
//   SELECT query FROM pg_stat_activity WHERE pid = $1
// Timeout: 2s (fast, single-row lookup).
// Frequency: only on first detection (startOp), not every tick.
```

### 7.1 Ring Buffer for Samples

```go
// appendSample adds a sample to op.Samples, evicting the oldest if at capacity.
func appendSample(op *TrackedOperation, sample ProgressSample, maxSize int) {
    if len(op.Samples) >= maxSize {
        op.Samples = op.Samples[1:] // evict oldest
    }
    op.Samples = append(op.Samples, sample)
}
```

---

## 8. Need Evaluator (`internal/forecast/evaluator.go`)

```go
package forecast

import (
    "context"
    "log/slog"
    "time"

    "github.com/ios9000/PGPulse_01/internal/collector"
    "github.com/ios9000/PGPulse_01/internal/config"
    "github.com/ios9000/PGPulse_01/internal/ml"
)

// BaselineProvider is an optional interface for ML-enhanced forecasting.
// Satisfied by ml.Detector. Nil if ML is disabled.
type BaselineProvider interface {
    GetBaselineStats(instanceID, metricKey string) (trend []float64, residualStdDev float64, ok bool)
}

// InstanceLister returns the list of active instance IDs.
// Satisfied by orchestrator or storage layer.
type InstanceLister interface {
    ActiveInstanceIDs(ctx context.Context) ([]string, error)
}

// ThresholdQuerier executes pg_settings + reloptions queries on target instances.
// Satisfied by the ConnProvider-based implementation.
type ThresholdQuerier interface {
    GetTableThresholds(ctx context.Context, instanceID string) ([]TableThresholds, error)
}

// NeedEvaluator is a background goroutine that computes maintenance need forecasts.
type NeedEvaluator struct {
    metricStore     collector.MetricStore
    forecastStore   ForecastStore
    baselineProvider BaselineProvider  // nil if ML disabled
    instanceLister  InstanceLister
    thresholdQuerier ThresholdQuerier
    cfg             config.ForecastConfig
    logger          *slog.Logger
}

func NewNeedEvaluator(
    metricStore collector.MetricStore,
    forecastStore ForecastStore,
    baselineProvider BaselineProvider,
    instanceLister InstanceLister,
    thresholdQuerier ThresholdQuerier,
    cfg config.ForecastConfig,
    logger *slog.Logger,
) *NeedEvaluator

// Run is the main loop.
//
// Pseudocode:
//   ticker := time.NewTicker(e.cfg.EvaluationInterval)
//   e.evaluate(ctx) // run immediately on startup
//   for {
//     select {
//     case <-ctx.Done(): return
//     case <-ticker.C: e.evaluate(ctx)
//     }
//   }
func (e *NeedEvaluator) Run(ctx context.Context)

// evaluate runs one full evaluation cycle across all active instances.
//
// For each instance:
//   1. Get table thresholds (cached per cycle via ThresholdQuerier)
//   2. Evaluate vacuum needs for all tables with thresholds
//   3. Evaluate analyze needs for all tables with thresholds
//   4. Evaluate reindex needs for tables with bloat metrics
//   5. Evaluate basebackup need (instance-level, if interval configured)
//   6. Batch UPSERT all forecasts
func (e *NeedEvaluator) evaluate(ctx context.Context)

// evaluateVacuum computes vacuum forecast for a single table.
//
// Algorithm:
//   1. Query MetricStore: pg.db.vacuum.dead_tuples for (instance, database, table)
//      over lookback_window (default 24h)
//   2. If < min_data_points → return status="insufficient_data"
//   3. Compute accumulation rate via ml.LinearRegression(timestamps, deadTupleValues)
//   4. If rate <= 0 → return status="not_needed"
//   5. currentDeadTuples = latest value
//   6. threshold = tableThresholds.EffectiveVacuumLimit
//   7. If currentDeadTuples >= threshold → return status="overdue"
//   8. timeToThreshold = (threshold - currentDeadTuples) / rate
//   9. If timeToThreshold <= 3600 → status="imminent", else "predicted"
//  10. ML enhancement: if baselineProvider != nil:
//      a. Get trend + residualStdDev for the metric
//      b. Use trend slope as rate (may be smoother than raw regression)
//      c. Compute confidence band: lower = time at (threshold) using (rate + z*stddev)
//                                  upper = time at (threshold) using (rate - z*stddev)
//      d. Set method = "threshold_projection+ml"
//  11. Return MaintenanceForecast
func (e *NeedEvaluator) evaluateVacuum(
    ctx context.Context, instanceID string, t TableThresholds, now time.Time,
) MaintenanceForecast

// evaluateAnalyze — same pattern as evaluateVacuum, using:
//   metric: pg.db.vacuum.mod_since_analyze
//   threshold: AnalyzeThreshold + AnalyzeScaleFactor * reltuples
func (e *NeedEvaluator) evaluateAnalyze(
    ctx context.Context, instanceID string, t TableThresholds, now time.Time,
) MaintenanceForecast

// evaluateReindex — uses bloat metrics:
//   metric: pg.db.bloat.table_ratio + pg.db.bloat.index_ratio
//   threshold: configurable (default 40% table, 30% index)
func (e *NeedEvaluator) evaluateReindex(
    ctx context.Context, instanceID, database, table string, now time.Time,
) MaintenanceForecast

// evaluateBasebackup — instance-level:
//   metric: pg.wal.bytes_rate
//   threshold: policy-based (forecast.basebackup_interval)
func (e *NeedEvaluator) evaluateBasebackup(
    ctx context.Context, instanceID string, now time.Time,
) MaintenanceForecast
```

---

## 9. Threshold Querier (`internal/forecast/threshold.go`)

```go
package forecast

import (
    "context"
    "fmt"
    "log/slog"
    "strconv"
    "strings"
)

// PGThresholdQuerier queries PostgreSQL for autovacuum/autoanalyze settings.
// Uses the instance connection provider to connect to each target instance.
type PGThresholdQuerier struct {
    connProv InstanceConnProvider
    cfg      ForecastConfig
    logger   *slog.Logger
}

// InstanceConnProvider provides database connections to monitored instances.
// The existing interface in internal/api/connprovider.go only exposes:
//   ConnFor(ctx context.Context, instanceID string) (*pgxpool.Conn, error)
//
// This connects to the instance's default database (from DSN). For threshold
// queries that must run per-database, M15_01 extends the interface:
//
//   type InstanceConnProvider interface {
//       ConnFor(ctx context.Context, instanceID string) (*pgxpool.Conn, error)
//       ConnForDB(ctx context.Context, instanceID, database string) (*pgxpool.Conn, error)
//   }
//
// ConnForDB creates a one-off connection to the specified database on the instance,
// cloning the instance's DSN but replacing the dbname parameter. The connection is
// NOT pooled (pgx.Connect, not pool.Acquire) and must be closed by the caller.
//
// If ConnForDB is too invasive for M15_01, the fallback approach is:
//   1. ConnFor() to the default database
//   2. Query pg_database for the list of databases
//   3. For each database, use dblink or pg_class is per-database anyway
//
// DECISION: Implement ConnForDB. The threshold querier and the OperationTracker's
// REINDEX identification both need per-database queries. The interface extension
// is minimal (one method) and the implementation is straightforward (clone DSN,
// swap dbname). The playbook executor will also benefit from this in future iterations.

func NewPGThresholdQuerier(connProv InstanceConnProvider, cfg ForecastConfig, logger *slog.Logger) *PGThresholdQuerier

// GetTableThresholds queries pg_settings and pg_class.reloptions for all user tables.
//
// Step 0: Discover databases
//
//   Connect to default database via ConnFor(instanceID).
//   Query: SELECT datname FROM pg_database
//          WHERE datallowconn AND NOT datistemplate AND datname != 'template0'
//
// Step 1: Global defaults (once, from default connection)
//
//   SELECT name, setting FROM pg_settings
//   WHERE name IN (
//     'autovacuum_vacuum_threshold',
//     'autovacuum_vacuum_scale_factor',
//     'autovacuum_analyze_threshold',
//     'autovacuum_analyze_scale_factor',
//     'autovacuum'
//   )
//
// Step 2: Per-database table data (via ConnForDB for each database)
//
//   For each database:
//     conn := ConnForDB(ctx, instanceID, dbname)
//     Use transaction-scoped session settings (defense-in-depth):
//
//     BEGIN;
//     SET LOCAL statement_timeout = '5s';
//     SET LOCAL lock_timeout = '2s';
//     SET LOCAL application_name = 'pgpulse_forecast';
//
//     SELECT
//       n.nspname AS schema,
//       c.relname AS table_name,
//       current_database() AS database,
//       c.reltuples::bigint AS reltuples,
//       c.reloptions
//     FROM pg_class c
//     JOIN pg_namespace n ON n.oid = c.relnamespace
//     WHERE c.relkind IN ('r', 'm')
//       AND n.nspname NOT IN ('pg_catalog', 'information_schema', 'pg_toast')
//
//     ROLLBACK;  -- read-only, no mutations; ROLLBACK releases locks and resets SET LOCAL
//     conn.Close()
//
// Step 3: Parse reloptions for per-table overrides
//
//   reloptions is text[] like: {autovacuum_vacuum_threshold=100,autovacuum_vacuum_scale_factor=0.05}
//   Parse each element: split on '=' → key, value
//   Apply override if key matches autovacuum_* setting
//
// Step 4: Compute effective limits
//
//   EffectiveVacuumLimit = vacuumThreshold + vacuumScaleFactor * reltuples
//   EffectiveAnalyzeLimit = analyzeThreshold + analyzeScaleFactor * reltuples
//
// Connection hygiene:
//   - All session settings via SET LOCAL inside a transaction — never raw SET on pooled connections
//   - ROLLBACK (not COMMIT) to release cleanly — no mutations occurred
//   - This matches the M14_04 C1 correction pattern (BEGIN + SET LOCAL + ROLLBACK)
func (q *PGThresholdQuerier) GetTableThresholds(ctx context.Context, instanceID string) ([]TableThresholds, error)

// parseReloptions extracts autovacuum settings from the reloptions text array.
//
//   Input: []string{"autovacuum_vacuum_threshold=100", "fillfactor=80"}
//   Output: map[string]string{"autovacuum_vacuum_threshold": "100"}
func parseReloptions(opts []string) map[string]string
```

---

## 10. Forecast Engine (`internal/forecast/engine.go`)

```go
package forecast

import (
    "context"
    "log/slog"
    "time"

    "github.com/ios9000/PGPulse_01/internal/collector"
    "github.com/ios9000/PGPulse_01/internal/config"
)

// ForecastEngine is the top-level coordinator. It owns the OperationTracker,
// NeedEvaluator, and ETACalculator, manages their lifecycle, and provides
// the API surface for handlers.
type ForecastEngine struct {
    Tracker       *OperationTracker
    Evaluator     *NeedEvaluator
    ETA           *ETACalculator
    Store         ForecastStore
    cfg           config.ForecastConfig
    logger        *slog.Logger
    cleanupTicker *time.Ticker
}

func NewForecastEngine(
    metricStore collector.MetricStore,
    forecastStore ForecastStore,
    baselineProvider BaselineProvider,
    instanceLister InstanceLister,
    connProv InstanceConnProvider,
    cfg config.ForecastConfig,
    logger *slog.Logger,
) *ForecastEngine {
    tracker := NewOperationTracker(metricStore, forecastStore, connProv, 15*time.Second, cfg.ETAWindowSize, logger)
    thresholdQuerier := NewPGThresholdQuerier(connProv, cfg, logger)
    evaluator := NewNeedEvaluator(metricStore, forecastStore, baselineProvider, instanceLister, thresholdQuerier, cfg, logger)
    eta := NewETACalculator(tracker, cfg, logger)

    return &ForecastEngine{
        Tracker:   tracker,
        Evaluator: evaluator,
        ETA:       eta,
        Store:     forecastStore,
        cfg:       cfg,
        logger:    logger,
    }
}

// Start launches background goroutines. Called from main.go.
//
//   go e.Tracker.Run(ctx)
//   go e.Evaluator.Run(ctx)
//   go e.retentionCleanup(ctx)
func (e *ForecastEngine) Start(ctx context.Context)

// retentionCleanup runs daily, deletes operations older than retention_days.
//
//   ticker := time.NewTicker(24 * time.Hour)
//   for {
//     select {
//     case <-ctx.Done(): return
//     case <-ticker.C:
//       cutoff := time.Now().AddDate(0, 0, -e.cfg.RetentionDays)
//       deleted, err := e.Store.CleanOldOperations(ctx, cutoff)
//       e.logger.Info("forecast retention cleanup", "deleted", deleted)
//     }
//   }
func (e *ForecastEngine) retentionCleanup(ctx context.Context)
```

---

## 11. Wire-Up in `cmd/pgpulse-server/main.go`

Following the existing pattern used by RCA engine, playbook engine, and remediation engine:

```go
// In the main() function, after existing subsystem initialization:

// --- Forecast Engine ---
var forecastEngine *forecast.ForecastEngine
if cfg.Forecast.Enabled {
    forecastStore := forecast.NewPGForecastStore(metadataPool, logger)

    // BaselineProvider: use ML detector if ML is enabled, nil otherwise
    var baselineProvider forecast.BaselineProvider
    if cfg.ML.Enabled && mlDetector != nil {
        baselineProvider = mlDetector
    }

    // InstanceLister: use orchestrator (has ActiveInstanceIDs method)
    instanceLister := orchestrator

    forecastEngine = forecast.NewForecastEngine(
        metricStore,
        forecastStore,
        baselineProvider,
        instanceLister,
        connProvider,  // same InstanceConnProvider used by playbook executor
        cfg.Forecast,
        logger.With("component", "forecast"),
    )
    forecastEngine.Start(ctx)
    logger.Info("forecast engine started")
}

// Wire to API server
if forecastEngine != nil {
    apiServer.SetForecastEngine(forecastEngine)
}

// Shutdown: context cancellation drains all forecast goroutines automatically
```

---

## 12. API Handlers (`internal/api/forecast_maint.go`)

```go
package api

import (
    "net/http"
    "strconv"

    "github.com/go-chi/chi/v5"
    "github.com/ios9000/PGPulse_01/internal/forecast"
)

// Route registration in server.go Routes() method:
//
// r.Route("/api/v1/instances/{id}/forecast", func(r chi.Router) {
//     r.Use(requireAuth)
//     r.Get("/eta", s.handleForecastETA)           // viewer+
//     r.Get("/eta/{pid}", s.handleForecastETAByPID) // viewer+
//     r.Get("/needs", s.handleForecastNeeds)        // viewer+
//     r.Get("/needs/{database}/{table}", s.handleForecastNeedsForTable) // viewer+
//     r.Get("/history", s.handleForecastHistory)    // viewer+
// })

// SetForecastEngine wires the forecast engine into the API server.
// Follows same pattern as SetRemediationEngine, SetPlaybookStore.
func (s *APIServer) SetForecastEngine(engine *forecast.ForecastEngine)

// handleForecastETA returns ETAs for all active operations on the instance.
//
// Response: { "operations": [OperationETA...], "evaluated_at": "..." }
// Empty operations list is valid (no active maintenance).
func (s *APIServer) handleForecastETA(w http.ResponseWriter, r *http.Request) {
    instanceID := chi.URLParam(r, "id")
    etas, err := s.forecastEngine.ETA.ComputeAll(r.Context(), instanceID)
    // ... writeJSON
}

// handleForecastETAByPID returns ETA for a specific PID.
//
// 404 if PID not found in active operations.
func (s *APIServer) handleForecastETAByPID(w http.ResponseWriter, r *http.Request) {
    instanceID := chi.URLParam(r, "id")
    pid, _ := strconv.Atoi(chi.URLParam(r, "pid"))
    eta, err := s.forecastEngine.ETA.ComputeByPID(r.Context(), instanceID, pid)
    // ... writeJSON or 404
}

// handleForecastNeeds returns cached maintenance forecasts.
//
// Query params: status (comma-separated), operation
// Response: { "forecasts": [MaintenanceForecast...], "summary": ForecastSummary }
func (s *APIServer) handleForecastNeeds(w http.ResponseWriter, r *http.Request) {
    instanceID := chi.URLParam(r, "id")
    filter := forecast.ForecastFilter{
        InstanceID: instanceID,
        Operation:  r.URL.Query().Get("operation"),
    }
    if statuses := r.URL.Query().Get("status"); statuses != "" {
        filter.Statuses = strings.Split(statuses, ",")
    }
    forecasts, err := s.forecastEngine.Store.ListForecasts(r.Context(), filter)
    summary := computeSummary(forecasts)
    // ... writeJSON
}

// handleForecastNeedsForTable returns forecasts for a specific table.
func (s *APIServer) handleForecastNeedsForTable(w http.ResponseWriter, r *http.Request)

// handleForecastHistory returns completed operation history with pagination.
//
// Query params: operation, table, page, per_page (default 50)
// Response: { "operations": [...], "total": N, "page": P, "per_page": PP }
func (s *APIServer) handleForecastHistory(w http.ResponseWriter, r *http.Request)
```

---

## 13. Frontend Components

### 13.1 React Query Hooks (`web/src/hooks/useForecast.ts`)

```typescript
// useETAForInstance — polls GET /forecast/eta every 15s
// Returns: { data: { operations: OperationETA[] }, isLoading, error }
export function useETAForInstance(instanceId: string)

// useMaintenanceForecasts — polls GET /forecast/needs every 60s
// Accepts optional filters: { status?: string[], operation?: string }
// Returns: { data: { forecasts: MaintenanceForecast[], summary: ForecastSummary }, isLoading, error }
export function useMaintenanceForecasts(instanceId: string, filters?: ForecastFilters)

// useOperationHistory — on-demand GET /forecast/history
// Returns: { data: { operations: MaintenanceOperation[], total: number }, isLoading, error, fetchNextPage }
export function useOperationHistory(instanceId: string, filters?: HistoryFilters)
```

### 13.2 ETABadge (`web/src/components/forecast/ETABadge.tsx`)

Inline badge rendering human-readable ETA:

| Condition | Display |
|-----------|---------|
| `eta_sec > 0`, confidence "high" | `"~2h 15m remaining"` (green text) |
| `eta_sec > 0`, confidence "medium" | `"~2h 15m remaining"` (yellow text, "~" prefix) |
| `eta_sec > 0`, confidence "low" | `"estimating... ~2h 15m"` (gray text) |
| `eta_sec == -1` (stalled) | `"Stalled"` (red text, warning icon) |
| No data | `"—"` |

Time formatting: `< 60s` → "< 1 min", `< 3600s` → "Xm", `< 86400s` → "Xh Ym", `>= 86400s` → "Xd Yh".

### 13.3 NeedForecastCard (`web/src/components/forecast/NeedForecastCard.tsx`)

Summary card for instance dashboard:

```
┌─────────────────────────────────┐
│ Maintenance Forecast            │
│                                 │
│  🔴 2 Overdue   🟡 3 Imminent  │
│  🔵 14 Predicted                │
│                                 │
│  Next: vacuum on orders (~2h)   │
│  [View All Forecasts →]         │
└─────────────────────────────────┘
```

Clicking "View All Forecasts" scrolls to NeedForecastTable (or links to a dedicated section on the instance page).

### 13.4 NeedForecastTable (`web/src/components/forecast/NeedForecastTable.tsx`)

Sortable table with columns: Status (badge), Database, Table, Operation, Time Until, Current / Threshold, Rate, Method, Last Evaluated.

Status badges: overdue (red), imminent (yellow), predicted (blue), not_needed (gray), insufficient_data (gray dashed).

Default sort: overdue first, then imminent, then predicted by time_until ascending.

### 13.5 OperationHistoryTable (`web/src/components/forecast/OperationHistoryTable.tsx`)

Paginated table: Operation, Database, Table, Size, Started, Duration, Avg Rate. No chart in M15_01 — charts deferred to M15_02 Forecast Dashboard.

### 13.6 Page Modifications

**ProgressPage.tsx:** Add "ETA" column to the right side of the existing progress table. Uses `useETAForInstance` hook. Renders `ETABadge` per row, matching by PID.

**InstanceDashboard.tsx:** Add `NeedForecastCard` to the dashboard grid. Position: after the alerts summary card, before the replication status card (or at the end of the first row if layout allows). The card is only rendered if `forecast.enabled` is true (check via a health/config endpoint or feature flag in the API response).

---

## 14. Migration 019 — Full SQL

```sql
-- Migration 019: Maintenance Operation Forecasting
-- Iteration: M15_01

-- Completed maintenance operations history
CREATE TABLE IF NOT EXISTS maintenance_operations (
    id                BIGSERIAL PRIMARY KEY,
    instance_id       TEXT NOT NULL,
    operation         TEXT NOT NULL CHECK (operation IN ('vacuum', 'analyze', 'reindex_concurrent', 'basebackup')),
    outcome           TEXT NOT NULL DEFAULT 'unknown' CHECK (outcome IN ('completed', 'canceled', 'failed', 'disappeared', 'unknown')),
    database          TEXT NOT NULL DEFAULT '',
    table_name        TEXT NOT NULL DEFAULT '',
    table_size_bytes  BIGINT,
    started_at        TIMESTAMPTZ NOT NULL,
    completed_at      TIMESTAMPTZ NOT NULL,
    duration_sec      DOUBLE PRECISION NOT NULL,
    final_pct         DOUBLE PRECISION,
    avg_rate_per_sec  DOUBLE PRECISION,
    metadata          JSONB NOT NULL DEFAULT '{}',
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_maint_ops_instance_op
    ON maintenance_operations(instance_id, operation);
CREATE INDEX IF NOT EXISTS idx_maint_ops_instance_table
    ON maintenance_operations(instance_id, database, table_name);
CREATE INDEX IF NOT EXISTS idx_maint_ops_completed
    ON maintenance_operations(completed_at DESC);

-- Cached maintenance forecasts (UPSERT by unique constraint)
CREATE TABLE IF NOT EXISTS maintenance_forecasts (
    id                  BIGSERIAL PRIMARY KEY,
    instance_id         TEXT NOT NULL,
    database            TEXT NOT NULL DEFAULT '',
    table_name          TEXT NOT NULL DEFAULT '',
    operation           TEXT NOT NULL CHECK (operation IN ('vacuum', 'analyze', 'reindex', 'basebackup')),
    status              TEXT NOT NULL CHECK (status IN ('predicted', 'imminent', 'overdue', 'not_needed', 'insufficient_data')),
    predicted_at        TIMESTAMPTZ,
    time_until_sec      DOUBLE PRECISION,
    confidence_lower    TIMESTAMPTZ,
    confidence_upper    TIMESTAMPTZ,
    current_value       DOUBLE PRECISION,
    threshold_value     DOUBLE PRECISION,
    accumulation_rate   DOUBLE PRECISION,
    method              TEXT NOT NULL DEFAULT 'threshold_projection',
    evaluated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(instance_id, database, table_name, operation)
);

CREATE INDEX IF NOT EXISTS idx_maint_forecasts_instance
    ON maintenance_forecasts(instance_id);
CREATE INDEX IF NOT EXISTS idx_maint_forecasts_status
    ON maintenance_forecasts(status) WHERE status IN ('imminent', 'overdue');
```

---

## 15. Null Store (`internal/forecast/nullstore.go`)

```go
package forecast

// NullForecastStore is a no-op implementation for when forecast is disabled.
// Follows the pattern of alert/nullstore.go and playbook/nullstore.go.

type NullForecastStore struct{}

func (n *NullForecastStore) WriteOperation(ctx context.Context, op *MaintenanceOperation) error { return nil }
func (n *NullForecastStore) ListOperations(ctx context.Context, filter OperationFilter) ([]MaintenanceOperation, int, error) {
    return nil, 0, nil
}
func (n *NullForecastStore) CleanOldOperations(ctx context.Context, olderThan time.Time) (int64, error) { return 0, nil }
func (n *NullForecastStore) UpsertForecast(ctx context.Context, f *MaintenanceForecast) error { return nil }
func (n *NullForecastStore) UpsertForecasts(ctx context.Context, forecasts []MaintenanceForecast) error { return nil }
func (n *NullForecastStore) ListForecasts(ctx context.Context, filter ForecastFilter) ([]MaintenanceForecast, error) {
    return nil, nil
}
func (n *NullForecastStore) DeleteForecasts(ctx context.Context, instanceID string) error { return nil }
func (n *NullForecastStore) Close() error { return nil }
```

---

## 16. Config Additions (`internal/config/config.go`)

```go
// Add to Config struct:
type Config struct {
    // ... existing fields ...
    Forecast ForecastConfig `koanf:"forecast"`
}

type ForecastConfig struct {
    Enabled                    bool          `koanf:"enabled"`
    ETAWindowSize              int           `koanf:"eta_window_size"`
    ETADecayFactor             float64       `koanf:"eta_decay_factor"`
    ETAMinSamples              int           `koanf:"eta_min_samples"`
    EvaluationInterval         time.Duration `koanf:"evaluation_interval"`
    MinDataPoints              int           `koanf:"min_data_points"`
    LookbackWindow             time.Duration `koanf:"lookback_window"`
    VacuumThresholdFallback    int           `koanf:"vacuum_threshold_fallback"`
    VacuumScaleFactorFallback  float64       `koanf:"vacuum_scale_factor_fallback"`
    AnalyzeThresholdFallback   int           `koanf:"analyze_threshold_fallback"`
    AnalyzeScaleFactorFallback float64       `koanf:"analyze_scale_factor_fallback"`
    ReindexBloatThresholdTable float64       `koanf:"reindex_bloat_threshold_table"`
    ReindexBloatThresholdIndex float64       `koanf:"reindex_bloat_threshold_index"`
    BasebackupInterval         time.Duration `koanf:"basebackup_interval"`
    RetentionDays              int           `koanf:"retention_days"`
}

// Defaults (applied in load.go):
//   forecast.enabled = false
//   forecast.eta_window_size = 10
//   forecast.eta_decay_factor = 0.85
//   forecast.eta_min_samples = 4
//   forecast.evaluation_interval = 5m
//   forecast.min_data_points = 3
//   forecast.lookback_window = 24h
//   forecast.vacuum_threshold_fallback = 50
//   forecast.vacuum_scale_factor_fallback = 0.2
//   forecast.analyze_threshold_fallback = 50
//   forecast.analyze_scale_factor_fallback = 0.1
//   forecast.reindex_bloat_threshold_table = 0.40
//   forecast.reindex_bloat_threshold_index = 0.30
//   forecast.basebackup_interval = 0s (disabled)
//   forecast.retention_days = 90
```

---

## 17. File Inventory

### New Files (22 total)

| File | Est. Lines | Owner |
|------|-----------|-------|
| `internal/forecast/types.go` | ~120 | Backend |
| `internal/forecast/store.go` | ~50 | Backend |
| `internal/forecast/pgstore.go` | ~300 | Backend |
| `internal/forecast/pgstore_test.go` | ~200 | Backend |
| `internal/forecast/nullstore.go` | ~30 | Backend |
| `internal/forecast/engine.go` | ~80 | Backend |
| `internal/forecast/tracker.go` | ~250 | Backend |
| `internal/forecast/tracker_test.go` | ~200 | Backend |
| `internal/forecast/eta.go` | ~120 | Backend |
| `internal/forecast/eta_test.go` | ~150 | Backend |
| `internal/forecast/evaluator.go` | ~350 | Backend |
| `internal/forecast/evaluator_test.go` | ~300 | Backend |
| `internal/forecast/threshold.go` | ~150 | Backend |
| `internal/forecast/threshold_test.go` | ~120 | Backend |
| `internal/ml/linear.go` | ~60 | Backend |
| `internal/ml/linear_test.go` | ~80 | Backend |
| `internal/ml/wma.go` | ~60 | Backend |
| `internal/ml/wma_test.go` | ~80 | Backend |
| `internal/api/forecast_maint.go` | ~250 | Backend |
| `internal/api/forecast_maint_test.go` | ~200 | Backend |
| `migrations/019_maintenance_forecasting.sql` | ~45 | Backend |
| `web/src/hooks/useForecast.ts` | ~60 | Frontend |

### New Frontend Components (5 files)

| File | Est. Lines | Owner |
|------|-----------|-------|
| `web/src/components/forecast/ETABadge.tsx` | ~80 | Frontend |
| `web/src/components/forecast/ETAConfidenceIndicator.tsx` | ~30 | Frontend |
| `web/src/components/forecast/NeedForecastCard.tsx` | ~100 | Frontend |
| `web/src/components/forecast/NeedForecastTable.tsx` | ~150 | Frontend |
| `web/src/components/forecast/OperationHistoryTable.tsx` | ~120 | Frontend |

### Modified Files (7 files)

| File | Change | Owner |
|------|--------|-------|
| `internal/config/config.go` | Add ForecastConfig struct + field | Backend |
| `internal/config/load.go` | Add forecast defaults | Backend |
| `internal/api/connprovider.go` | Add ConnForDB method to InstanceConnProvider interface | Backend |
| `internal/api/server.go` | Add SetForecastEngine + routes | Backend |
| `cmd/pgpulse-server/main.go` | Wire ForecastEngine | Backend |
| `web/src/pages/ProgressPage.tsx` | Add ETA column | Frontend |
| `web/src/pages/InstanceDashboard.tsx` | Add NeedForecastCard | Frontend |

**Total estimated new Go code:** ~2,800 lines (including tests)
**Total estimated new TypeScript code:** ~540 lines
**Total estimated modifications:** ~150 lines across existing files

---

## 18. Dependency Graph

```
internal/forecast/
    ├── imports internal/ml          (LinearRegression, WMA)
    ├── imports internal/collector   (MetricStore, MetricQuery, MetricPoint)
    ├── imports internal/config      (ForecastConfig)
    └── imports pgx/v5/pgxpool       (PGForecastStore)

internal/api/
    └── imports internal/forecast    (ForecastEngine, types)

cmd/pgpulse-server/
    └── imports internal/forecast    (NewForecastEngine, NewPGForecastStore)

NO imports from internal/forecast to:
    - internal/remediation   (deferred to M15_02)
    - internal/playbook      (deferred to M15_02)
    - internal/alert         (deferred to M15_02)
    - internal/rca           (no dependency)
```

This ensures `internal/forecast/` is a clean, self-contained package with minimal coupling. M15_02 will add the cross-subsystem integrations.
