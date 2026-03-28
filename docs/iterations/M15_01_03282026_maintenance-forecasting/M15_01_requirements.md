# M15_01 — Requirements: Maintenance Operation Forecasting (Foundation + ETA + Need Forecasting)

**Iteration:** M15_01
**Date:** 2026-03-27
**Predecessor:** M14_04 (Guided Remediation Playbooks — complete)
**Successor:** M15_02 (Window Feasibility + Full UI + Integrations)
**Locked Decisions:** D700–D711

---

## DO NOT RE-DISCUSS

All D400–D609 decisions remain locked (M14 complete). Additionally:

- Core 4 operations only: VACUUM, ANALYZE, REINDEX, BASEBACKUP — not CLUSTER, CREATE INDEX, COPY
- All three pillars in M15 (ETA + Need Forecasting + Window Feasibility) — split across two sub-iterations
- Hybrid package: `internal/forecast/` for domain logic, `internal/ml/` for algorithms — not monolithic
- Weighted moving average for ETA — not simple linear, not phase-aware
- Threshold projection primary + ML confidence bands — not ML-only, not threshold-only
- REINDEX ETA only for REINDEX CONCURRENTLY — no ETA for regular REINDEX (no `pg_stat_progress_reindex`)
- OperationTracker in `internal/forecast/` — not in collectors, not in a separate `internal/maintenance/`
- 2 agents for M15_01 — not 3
- WMA window configurable, default 10 — not hardcoded
- Need-forecast evaluation: dedicated 5-min goroutine — not coupled to DatabaseCollector
- Operation history retention: configurable, default 90 days

---

## 1. Objective

Add a **Maintenance Operation Forecasting** subsystem to PGPulse that answers two questions every DBA asks daily:

1. **"How much longer will this vacuum/backup take?"** — real-time ETA for in-progress operations using weighted moving average over progress metric samples.
2. **"When will this table need vacuuming?"** — predictive forecasting based on dead tuple accumulation rates projected against PostgreSQL's autovacuum thresholds, enhanced with ML confidence bands from existing STL baselines.

M15_01 delivers the backend engine, data persistence layer, API endpoints, and minimal UI extensions (ETA column on Progress page, Maintenance Forecasts card on instance dashboard). M15_02 will add the Window Feasibility planner, full Forecast Dashboard, and integration with the Adviser/Playbook subsystems.

**Target users:** DBAs planning maintenance windows, on-call operators monitoring overnight operations, team leads reviewing fleet maintenance health.

---

## 2. Decision Summary

| ID | Decision |
|----|----------|
| D700 | Core 4 operations: VACUUM, ANALYZE, REINDEX, BASEBACKUP |
| D701 | All three pillars: ETA + Need Forecasting + Window Feasibility (M15_01 + M15_02) |
| D702 | Hybrid package: `internal/forecast/` domain logic, `internal/ml/` algorithms |
| D703 | Two sub-iterations: M15_01 (Foundation + ETA + Need) → M15_02 (Window + UI) |
| D704 | Weighted moving average for ETA estimation |
| D705 | Dual approach: threshold projection primary + ML confidence bands |
| D706 | REINDEX ETA only for REINDEX CONCURRENTLY (via create_index progress) |
| D707 | OperationTracker as goroutine in `internal/forecast/`, polls MetricStore |
| D708 | 2 agents: Backend-heavy (engine + API) + Frontend-light |
| D709 | WMA window size configurable, default 10 (`forecast.eta_window_size`) |
| D710 | Need-forecast evaluation: dedicated background goroutine, default 5 min |
| D711 | Operation history retention: configurable, default 90 days |

---

## 3. Scope — What M15_01 Builds

### 3.1 Pillar 1: ETA Estimation (In-Progress Operations)

Real-time estimated time of arrival for currently running maintenance operations.

**Supported operations and data sources:**

| Operation | Progress Source | Key Metrics |
|-----------|---------------|-------------|
| VACUUM | `pg_stat_progress_vacuum` | `heap_blks_total`, `heap_blks_scanned`, `heap_blks_vacuumed`, `completion_pct` |
| ANALYZE | `pg_stat_progress_analyze` | Blocks sampled vs total (PG ≥ 13) |
| REINDEX CONCURRENTLY | `pg_stat_progress_create_index` | `blocks_done`, `blocks_total`, `tuples_done`, `tuples_total` |
| BASEBACKUP | `pg_stat_progress_basebackup` | Bytes processed vs total |

**Note:** Regular REINDEX has no progress view in PostgreSQL. Only REINDEX CONCURRENTLY is trackable (via `pg_stat_progress_create_index`). This is documented in the UI, not hidden.

**Algorithm: Weighted Moving Average (WMA)**

Given N progress samples (default N=10, configurable via `forecast.eta_window_size`):

1. Each collection cycle, record `(timestamp, work_done)` from progress metrics.
2. Compute per-interval rate: `rate_i = (work_done_i - work_done_{i-1}) / (time_i - time_{i-1})`.
3. Apply exponentially decaying weights: `weight_i = decay^(N - i)` where `decay = 0.85` (configurable via `forecast.eta_decay_factor`). Most recent samples weigh most.
4. Weighted average rate: `wma_rate = Σ(weight_i × rate_i) / Σ(weight_i)`.
5. ETA = `remaining_work / wma_rate`.
6. If `wma_rate ≤ 0` (stalled operation), report "Stalled" instead of a nonsensical ETA.

**Output per operation:**

```go
type OperationETA struct {
    InstanceID    string
    PID           int
    Operation     string        // "vacuum", "analyze", "reindex", "basebackup"
    Database      string
    Table         string        // empty for basebackup
    Phase         string        // current phase from progress view
    PercentDone   float64
    StartedAt     time.Time
    ElapsedSec    float64
    ETASec        float64       // estimated seconds remaining (-1 if stalled)
    ETAAt         time.Time     // predicted completion time
    RateCurrent   float64       // current WMA rate (blocks/sec or bytes/sec)
    Confidence    string        // "high" (≥8 samples), "medium" (4-7), "low" (1-3)
    SampleCount   int           // how many rate samples in the WMA window
}
```

**Confidence levels:**

- **High:** ≥ 8 rate samples in window — ETA is stable.
- **Medium:** 4–7 samples — ETA is converging, show with "~" prefix.
- **Low:** 1–3 samples — ETA is volatile, show with "estimating..." qualifier.

### 3.2 Pillar 2: Need Forecasting (When Will Maintenance Be Needed?)

Predictive analysis answering "table X will need vacuum in ~N hours."

**Forecast types:**

#### 3.2.1 Vacuum Need Forecast

**Threshold model:** Replicates PostgreSQL's autovacuum trigger formula:

```
autovacuum fires when:
  dead_tuples > autovacuum_vacuum_threshold + (autovacuum_vacuum_scale_factor × reltuples)

Default values:
  autovacuum_vacuum_threshold = 50
  autovacuum_vacuum_scale_factor = 0.2
```

Per-table overrides from `reloptions` are respected. The forecast engine must query both `pg_settings` (global defaults) and `pg_class.reloptions` (per-table overrides) to compute the effective threshold for each table.

**Algorithm:**

1. Query the MetricStore for `pg.db.vacuum.dead_tuples` time-series for target table (last 1h minimum, up to 24h).
2. Compute dead tuple accumulation rate using linear regression over recent samples.
3. Compute effective autovacuum threshold: `threshold = vacuum_threshold + scale_factor × live_tuples`.
4. Project: `time_to_threshold = (threshold - current_dead_tuples) / accumulation_rate`.
5. If accumulation rate ≤ 0 (dead tuples stable or decreasing), report "Not predicted — stable or decreasing."

**ML enhancement (confidence bands):**

If ML baselines exist for `pg.db.vacuum.dead_tuples` for this table:
1. Use the STL-decomposed trend component for rate estimation instead of raw linear regression.
2. Compute 95% confidence interval using the existing `ml.forecast.confidence_z` config value.
3. Report: predicted vacuum time ± confidence band (e.g., "vacuum needed in 6–8 hours").

**Output:**

```go
type MaintenanceForecast struct {
    ID                  int64
    InstanceID          string
    Database            string
    Table               string        // empty for instance-level forecasts (basebackup)
    Operation           string        // "vacuum", "analyze", "reindex", "basebackup"
    Status              string        // "predicted", "imminent", "overdue", "not_needed", "insufficient_data"
    PredictedAt         time.Time     // when the operation will be needed
    TimeUntilSec        float64       // seconds until predicted need
    ConfidenceLower     *time.Time    // ML lower bound (nil if ML unavailable)
    ConfidenceUpper     *time.Time    // ML upper bound (nil if ML unavailable)
    CurrentValue        float64       // e.g., current dead_tuples count
    ThresholdValue      float64       // e.g., effective autovacuum threshold
    AccumulationRate    float64       // units per second
    Method              string        // "threshold_projection", "threshold_projection+ml"
    EvaluatedAt         time.Time
}
```

**Status values:**

- **predicted:** Threshold crossing expected in > 1 hour.
- **imminent:** Threshold crossing expected in ≤ 1 hour.
- **overdue:** Already past threshold but autovacuum hasn't fired (possible autovacuum backlog or disabled).
- **not_needed:** Accumulation rate ≤ 0 or current value far below threshold.
- **insufficient_data:** < 3 data points — cannot compute rate.

#### 3.2.2 Analyze Need Forecast

Similar to vacuum but using PostgreSQL's autoanalyze formula:

```
autoanalyze fires when:
  mod_since_analyze > autovacuum_analyze_threshold + (autovacuum_analyze_scale_factor × reltuples)

Default values:
  autovacuum_analyze_threshold = 50
  autovacuum_analyze_scale_factor = 0.1
```

Uses `pg.db.vacuum.mod_since_analyze` metric. Same algorithm, same output struct.

#### 3.2.3 Reindex Need Forecast

Based on bloat ratio growth:

1. Query `pg.db.bloat.table_ratio` and `pg.db.bloat.index_ratio` time-series.
2. Compute bloat growth rate via linear regression.
3. Project when bloat exceeds configurable threshold (default: 40% for tables, 30% for indexes — `forecast.reindex_bloat_threshold_table`, `forecast.reindex_bloat_threshold_index`).
4. Report predicted time to threshold crossing.

**Note:** Bloat estimation is statistical and inherently imprecise. The forecast includes a caveat flag: `bloat_estimation_method: "statistical"`. M15_02 may refine this with pgstattuple if available.

#### 3.2.4 Basebackup Need Forecast

Based on WAL generation rate and backup interval policy:

1. Query `pg.wal.bytes_rate` (WAL generation rate) over the configured baseline window.
2. Given configured backup interval (e.g., 24h — `forecast.basebackup_interval`), compute whether the next scheduled backup window is sufficient.
3. If no interval is configured, skip basebackup forecasting for this instance.

This is the simplest forecast — it's policy-based, not threshold-based. Primary value is in M15_02's Window Feasibility ("will the backup finish before 6am?").

### 3.3 OperationTracker (Completion Detector)

A background goroutine that detects when maintenance operations start and finish by observing progress metric transitions.

**Detection logic:**

1. Each cycle (piggybacks on progress collector interval — typically 15s), query the MetricStore for all `pg.progress.*` metrics.
2. Maintain an in-memory map: `activeOps map[string]*TrackedOperation` keyed by `"{instance}:{pid}:{operation}"`.
3. **Start detection:** New PID+operation appears in progress metrics → create `TrackedOperation`, record start time, table, database.
4. **End detection:** Previously tracked PID+operation absent from progress metrics → operation completed. Record end time, compute duration, write to `maintenance_operations` table.
5. **Crash detection:** If PGPulse restarts, any `TrackedOperation` in memory is lost. This is acceptable for M15_01 — orphaned operations will not have completion records. M15_02 can add recovery logic.

**Completion record:**

```go
type MaintenanceOperation struct {
    ID              int64
    InstanceID      string
    Operation       string        // "vacuum", "analyze", "reindex_concurrent", "basebackup"
    Database        string
    Table           string
    TableSizeBytes  int64         // table size at start (from pg.db.table.total_bytes)
    StartedAt       time.Time
    CompletedAt     time.Time
    DurationSec     float64
    FinalPct        float64       // last known completion_pct before disappearance
    AvgRatePerSec   float64       // average processing rate over entire operation
    Metadata        map[string]any // operation-specific: index_vacuum_count, phases traversed, etc.
}
```

**Purpose:** The `maintenance_operations` table feeds M15_02's duration prediction model. In M15_01, it also serves as an operation history log — useful on its own for operational review ("last 10 vacuums on this table, how long did each take?").

---

## 4. Database Schema

### 4.1 Migration 019

```sql
-- M15_01: Maintenance Operation Forecasting

-- Completed maintenance operations (history for duration prediction)
CREATE TABLE IF NOT EXISTS maintenance_operations (
    id                BIGSERIAL PRIMARY KEY,
    instance_id       TEXT NOT NULL,
    operation         TEXT NOT NULL,        -- 'vacuum', 'analyze', 'reindex_concurrent', 'basebackup'
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

-- Cached maintenance forecasts (written by background evaluator)
CREATE TABLE IF NOT EXISTS maintenance_forecasts (
    id                  BIGSERIAL PRIMARY KEY,
    instance_id         TEXT NOT NULL,
    database            TEXT NOT NULL DEFAULT '',
    table_name          TEXT NOT NULL DEFAULT '',
    operation           TEXT NOT NULL,
    status              TEXT NOT NULL,     -- 'predicted', 'imminent', 'overdue', 'not_needed', 'insufficient_data'
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

The `maintenance_forecasts` table uses `UNIQUE(instance_id, database, table_name, operation)` so the background evaluator can `INSERT ... ON CONFLICT DO UPDATE` — always keeping the latest forecast per table/operation. Old forecasts are simply overwritten, not accumulated.

---

## 5. Configuration

New config section under `forecast:` in `pgpulse.yml`:

```yaml
forecast:
  enabled: true
  
  # ETA estimation
  eta_window_size: 10              # WMA sample window (number of progress samples)
  eta_decay_factor: 0.85           # Exponential decay weight for older samples
  
  # Need forecasting
  evaluation_interval: 5m          # How often the background evaluator runs
  min_data_points: 3               # Minimum time-series points for rate calculation
  lookback_window: 24h             # How far back to look for rate calculation
  
  # Vacuum/analyze threshold overrides (global defaults; per-table from reloptions)
  # These are PostgreSQL defaults — only override if the user has non-standard global settings
  # The engine always reads pg_settings and reloptions for actual values; these are fallbacks
  # if the settings query fails.
  vacuum_threshold_fallback: 50
  vacuum_scale_factor_fallback: 0.2
  analyze_threshold_fallback: 50
  analyze_scale_factor_fallback: 0.1
  
  # Reindex thresholds
  reindex_bloat_threshold_table: 0.40   # 40% bloat triggers reindex forecast
  reindex_bloat_threshold_index: 0.30   # 30% bloat triggers reindex forecast
  
  # Basebackup
  basebackup_interval: 0s          # 0 = disabled. Set to e.g., "24h" to enable basebackup forecasting
  
  # Operation history
  retention_days: 90               # How long to keep completed operation records
```

**Config struct addition to `internal/config/config.go`:**

```go
type ForecastConfig struct {
    Enabled                    bool          `koanf:"enabled"`
    ETAWindowSize              int           `koanf:"eta_window_size"`
    ETADecayFactor             float64       `koanf:"eta_decay_factor"`
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
```

---

## 6. API Endpoints

### 6.1 ETA Endpoints

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/api/v1/instances/{id}/forecast/eta` | viewer+ | All active operation ETAs for instance |
| GET | `/api/v1/instances/{id}/forecast/eta/{pid}` | viewer+ | ETA for specific operation by PID |

**GET `/api/v1/instances/{id}/forecast/eta` response:**

```json
{
  "operations": [
    {
      "pid": 12345,
      "operation": "vacuum",
      "database": "mydb",
      "table": "orders",
      "phase": "scanning heap",
      "percent_done": 42.7,
      "started_at": "2026-03-27T02:15:00Z",
      "elapsed_sec": 1847.3,
      "eta_sec": 2480.1,
      "eta_at": "2026-03-27T02:56:20Z",
      "rate_current": 1285.4,
      "confidence": "high",
      "sample_count": 10
    }
  ],
  "evaluated_at": "2026-03-27T02:45:47Z"
}
```

### 6.2 Need Forecast Endpoints

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/api/v1/instances/{id}/forecast/needs` | viewer+ | All maintenance forecasts for instance |
| GET | `/api/v1/instances/{id}/forecast/needs?status=imminent,overdue` | viewer+ | Filtered by status |
| GET | `/api/v1/instances/{id}/forecast/needs?operation=vacuum` | viewer+ | Filtered by operation type |
| GET | `/api/v1/instances/{id}/forecast/needs/{database}/{table}` | viewer+ | Forecasts for specific table |

**GET `/api/v1/instances/{id}/forecast/needs` response:**

```json
{
  "forecasts": [
    {
      "id": 1,
      "database": "mydb",
      "table": "orders",
      "operation": "vacuum",
      "status": "predicted",
      "predicted_at": "2026-03-27T08:30:00Z",
      "time_until_sec": 21600,
      "confidence_lower": "2026-03-27T07:15:00Z",
      "confidence_upper": "2026-03-27T09:45:00Z",
      "current_value": 28500,
      "threshold_value": 50200,
      "accumulation_rate": 1.01,
      "method": "threshold_projection+ml",
      "evaluated_at": "2026-03-27T02:30:00Z"
    }
  ],
  "summary": {
    "imminent_count": 2,
    "overdue_count": 0,
    "predicted_count": 14,
    "total_tables_evaluated": 45
  }
}
```

### 6.3 Operation History Endpoints

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/api/v1/instances/{id}/forecast/history` | viewer+ | Completed operations history |
| GET | `/api/v1/instances/{id}/forecast/history?operation=vacuum&table=orders` | viewer+ | Filtered history |

**GET `/api/v1/instances/{id}/forecast/history` response:**

```json
{
  "operations": [
    {
      "id": 42,
      "operation": "vacuum",
      "database": "mydb",
      "table": "orders",
      "table_size_bytes": 5368709120,
      "started_at": "2026-03-26T03:00:12Z",
      "completed_at": "2026-03-26T03:42:18Z",
      "duration_sec": 2526.0,
      "avg_rate_per_sec": 2124.8,
      "metadata": {
        "index_vacuum_count": 3,
        "phases": ["scanning heap", "vacuuming indexes", "vacuuming heap", "cleaning up indexes"]
      }
    }
  ],
  "total": 156,
  "page": 1,
  "per_page": 50
}
```

---

## 7. Package Structure

### 7.1 New: `internal/forecast/`

```
internal/forecast/
├── config.go           — ForecastConfig helpers, defaults
├── engine.go           — ForecastEngine: wires tracker, evaluator, ETA calculator
├── eta.go              — ETACalculator: WMA algorithm, per-operation ETA computation
├── eta_test.go         — Unit tests for WMA, edge cases (stalled, insufficient data)
├── evaluator.go        — NeedEvaluator: background goroutine, threshold projection
├── evaluator_test.go   — Unit tests for threshold calculation, rate projection
├── tracker.go          — OperationTracker: goroutine, start/end detection, writes history
├── tracker_test.go     — Unit tests for state transitions, completion detection
├── store.go            — ForecastStore interface
├── pgstore.go          — PostgreSQL implementation of ForecastStore
├── pgstore_test.go     — Store tests
├── threshold.go        — Autovacuum/autoanalyze threshold calculator (reads pg_settings + reloptions)
├── threshold_test.go   — Threshold calculation tests
├── types.go            — OperationETA, MaintenanceForecast, MaintenanceOperation, TrackedOperation
```

### 7.2 Modified: `internal/ml/`

```
internal/ml/
├── forecast.go         — EXISTING: ForecastPoint, ForecastResult types
├── linear.go           — NEW: LinearRegression helper (slope, intercept, R²)
├── linear_test.go      — NEW: Regression tests
├── wma.go              — NEW: WeightedMovingAverage calculator
├── wma_test.go         — NEW: WMA tests
```

The ML math (linear regression, WMA) lives in `internal/ml/`. The forecast domain logic (what to forecast, threshold semantics, PostgreSQL autovacuum formula) lives in `internal/forecast/`.

### 7.3 Modified: `internal/api/`

```
internal/api/
├── forecast_maint.go       — NEW: handlers for /forecast/eta, /forecast/needs, /forecast/history
├── forecast_maint_test.go  — NEW: handler tests
├── server.go               — MOD: wire ForecastEngine, add routes
```

### 7.4 Modified: `internal/config/`

```
internal/config/
├── config.go           — MOD: add ForecastConfig struct
```

### 7.5 Modified: frontend

```
web/src/
├── hooks/useForecast.ts           — NEW: React Query hooks for forecast endpoints
├── components/forecast/
│   ├── ETABadge.tsx               — NEW: inline ETA display (e.g., "~41 min remaining")
│   ├── ETAConfidenceIndicator.tsx — NEW: high/medium/low confidence icon
│   ├── NeedForecastCard.tsx       — NEW: summary card (imminent/predicted counts)
│   ├── NeedForecastTable.tsx      — NEW: table listing per-table forecasts
│   └── OperationHistoryTable.tsx  — NEW: completed operations log
├── pages/
│   ├── InstanceDashboard.tsx      — MOD: add NeedForecastCard
│   └── ProgressPage.tsx           — MOD: add ETA column with ETABadge
```

---

## 8. Integration Points

### 8.1 With Existing MetricStore

The OperationTracker reads from MetricStore (not from PostgreSQL directly). It queries:
- `pg.progress.vacuum.*` metrics
- `pg.progress.analyze.*` metrics
- `pg.progress.create_index.*` metrics (for REINDEX CONCURRENTLY detection)
- `pg.progress.basebackup.*` metrics

The NeedEvaluator reads:
- `pg.db.vacuum.dead_tuples` (per table)
- `pg.db.vacuum.live_tuples` (per table, for threshold calculation)
- `pg.db.vacuum.mod_since_analyze` (per table)
- `pg.db.bloat.table_ratio` (per table)
- `pg.db.bloat.index_ratio` (per table/index)
- `pg.wal.bytes_rate` (instance-level, for basebackup)

### 8.2 With Existing ML Subsystem

The NeedEvaluator calls `ml.Detector.GetBaseline(metricKey)` to check if STL baselines exist for a given metric. If they do, it uses the decomposed trend component for rate estimation and the residual standard deviation for confidence band computation. If ML is disabled or no baseline exists, it falls back to raw linear regression (threshold projection only).

### 8.3 Deferred to M15_02

- **EvaluateHook() integration:** Push "imminent" forecasts into Adviser feed via `remediation.Engine.EvaluateHook()` with `source: "forecast"`.
- **Playbook Resolver integration:** Create maintenance-specific playbooks triggered by forecast hooks.
- **Forecast-based alert rules:** "vacuum predicted to exceed maintenance window."
- **Window Feasibility engine:** Duration prediction model using `maintenance_operations` history.
- **Full Forecast Dashboard page:** Timeline visualization of predicted maintenance needs across the fleet.
- **Maintenance Calendar UI:** Visual planner with drag-and-drop window assignment.

---

## 9. Threshold Calculation Detail

The threshold calculator must handle PostgreSQL's layered override system:

**Priority order (highest to lowest):**

1. Per-table `reloptions` (e.g., `ALTER TABLE orders SET (autovacuum_vacuum_threshold = 100)`)
2. Global `pg_settings` values
3. Fallback values from PGPulse config (only if `pg_settings` query fails)

**Query for effective thresholds:**

```sql
-- Global autovacuum settings
SELECT name, setting
FROM pg_settings
WHERE name IN (
    'autovacuum_vacuum_threshold',
    'autovacuum_vacuum_scale_factor',
    'autovacuum_analyze_threshold',
    'autovacuum_analyze_scale_factor',
    'autovacuum'
);

-- Per-table overrides
SELECT
    n.nspname AS schema,
    c.relname AS table_name,
    c.reloptions,
    c.reltuples::bigint AS reltuples
FROM pg_class c
JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE c.relkind IN ('r', 'm')
  AND n.nspname NOT IN ('pg_catalog', 'information_schema', 'pg_toast')
  AND c.reloptions IS NOT NULL
  AND array_to_string(c.reloptions, ',') LIKE '%autovacuum%';
```

The threshold calculator parses `reloptions` array entries like `autovacuum_vacuum_threshold=100` and `autovacuum_vacuum_scale_factor=0.05` to compute the effective trigger point per table.

**Caching:** Threshold values are cached per evaluation cycle (5 min). The `pg_settings` and `reloptions` queries run once per cycle, not per table.

---

## 10. Agent Assignment

### Agent 1: Backend (Forecast Engine + API)

**Owns:**
- `internal/forecast/` — all files
- `internal/ml/linear.go`, `internal/ml/wma.go` + tests
- `internal/api/forecast_maint.go` + tests
- `migrations/019_maintenance_forecasting.sql`
- Config additions to `internal/config/config.go`
- Wiring in `cmd/pgpulse-server/main.go`

**Key responsibilities:**
- OperationTracker goroutine with clean shutdown
- NeedEvaluator goroutine with clean shutdown
- ETACalculator (stateless, called per-request)
- ForecastStore (PostgreSQL CRUD)
- Threshold calculator with pg_settings + reloptions query
- ML baseline integration (call existing detector, don't duplicate)
- All API handlers
- Retention cleanup goroutine (daily, deletes operations older than `retention_days`)

**Critical rules:**
- All SQL via pgx parameterized queries
- OperationTracker polls MetricStore, NOT the database directly
- NeedEvaluator queries MetricStore for time-series, queries DB only for thresholds
- Set `application_name = 'pgpulse_forecast'` on any direct DB connections
- `statement_timeout = '5s'` on threshold queries
- Graceful shutdown: drain goroutines via context cancellation
- Do not import from `internal/remediation/` or `internal/playbook/` — those integrations are M15_02

### Agent 2: Frontend

**Owns:**
- `web/src/hooks/useForecast.ts`
- `web/src/components/forecast/` — all 5 components
- Modifications to `ProgressPage.tsx` and `InstanceDashboard.tsx`

**Key responsibilities:**
- React Query hooks for all 3 endpoint groups (ETA, needs, history)
- ETABadge: renders human-readable ETA ("~41 min", "~2h 15m", "Stalled") with confidence indicator
- NeedForecastCard: summary widget showing imminent/overdue/predicted counts with color coding
- NeedForecastTable: sortable table of per-table forecasts with status badges
- OperationHistoryTable: paginated list of completed operations with duration and rate
- Extend ProgressPage: add ETA column to existing operation progress table
- Extend InstanceDashboard: add NeedForecastCard to the existing dashboard layout
- Auto-refresh: ETA polls every 15s (matches progress collector), forecasts poll every 60s

**Critical rules:**
- Use existing ECharts setup for any mini-charts
- Follow existing Tailwind CSS patterns
- Use existing `useApiQuery` / React Query infrastructure
- No new router pages — only extend existing pages and add components
- Responsive: all new components work on mobile widths

---

## 11. Testing Requirements

### 11.1 Backend Unit Tests

| Test Area | Key Cases |
|-----------|-----------|
| WMA algorithm | 10 even samples, 3 samples (low confidence), stalled (rate=0), single sample, decaying weights correctness |
| Linear regression | Positive slope, zero slope, negative slope, insufficient data, perfect fit R²=1 |
| ETA calculation | Normal operation, phase change mid-operation, stalled detection, operation disappears |
| OperationTracker | Start detection, completion detection, concurrent operations, PID reuse |
| NeedEvaluator | Vacuum threshold (default), vacuum threshold (per-table override), analyze threshold, bloat projection, insufficient data |
| Threshold calculator | Global defaults, per-table reloptions override, autovacuum disabled tables, fallback values |
| ForecastStore | CRUD, upsert on conflict, retention cleanup, filtered queries |
| API handlers | Valid requests, empty results, pagination, filtering |

### 11.2 Frontend

- ETABadge renders all confidence levels correctly
- NeedForecastCard shows correct counts and colors
- ETA column appears on Progress page
- Auto-refresh intervals are correct
- Stalled operation shows "Stalled" not "Infinity"
- Graceful empty state ("No active operations" / "No forecasts yet — data accumulating")

---

## 12. Known Limitations (M15_01)

| Limitation | Severity | Notes |
|------------|----------|-------|
| No REINDEX ETA for regular REINDEX | 🟡 Medium | PostgreSQL lacks `pg_stat_progress_reindex`. Only REINDEX CONCURRENTLY tracked. Documented in UI. |
| OperationTracker state lost on restart | 🟢 Low | In-flight operations at restart time won't have completion records. M15_02 may add recovery. |
| Bloat estimation is statistical | 🟢 Info | Uses `pg_stats`-based formula, not `pgstattuple`. Documented as "estimated." |
| No fleet-wide forecast dashboard | 🟢 Info | M15_01 shows per-instance forecasts only. Fleet view deferred to M15_02. |
| No Adviser/Playbook integration | 🟢 Info | Deferred to M15_02 per D703. |
| Basebackup forecast requires manual interval config | 🟢 Info | No auto-detection of backup schedule. User sets `forecast.basebackup_interval`. |
| Threshold query requires DB access | 🟢 Info | NeedEvaluator makes direct DB queries for `pg_settings`/`reloptions`, not purely MetricStore-based. This is unavoidable — thresholds are config values, not metrics. |

---

## 13. Build Verification

```bash
# Full verification (same as M14_04)
cd web && npm run build && npm run lint && npm run typecheck && cd ..
go build ./cmd/pgpulse-server
go test ./cmd/... ./internal/... -count=1
golangci-lint run ./cmd/... ./internal/...
```

---

## 14. Demo Validation Plan

After deployment to demo VM:

1. **Trigger a long vacuum:** Create a table with 1M rows, delete 500K, run `VACUUM VERBOSE` manually.
2. **Verify ETA:** While vacuum runs, hit `/api/v1/instances/{id}/forecast/eta` — should show ETA with "medium" confidence (building samples), converging to "high."
3. **Verify completion tracking:** After vacuum finishes, hit `/api/v1/instances/{id}/forecast/history` — should show the completed operation with duration.
4. **Verify need forecasting:** With active workload generating dead tuples, hit `/api/v1/instances/{id}/forecast/needs` — should show predictions for tables approaching autovacuum threshold.
5. **Verify UI:** Progress page shows ETA column. Instance dashboard shows NeedForecastCard.
6. **Edge case:** Start a vacuum, then kill the backend with `Ctrl+C` — verify clean shutdown (goroutines drain, no panic).

---

## 15. Migration to M15_02

M15_01 produces:

- `maintenance_operations` table with accumulating history → M15_02 uses for duration prediction model
- `maintenance_forecasts` table → M15_02 extends with window feasibility columns
- `internal/forecast/` package → M15_02 adds `planner.go`, `duration_model.go`
- `internal/ml/linear.go` → M15_02 uses for duration ~ table_size regression
- API endpoints → M15_02 extends with `/forecast/plan` and `/forecast/window`
- Frontend components → M15_02 builds full Forecast Dashboard and Maintenance Calendar

The handoff from M15_01 to M15_02 will include the operation history data collected between iterations — the longer M15_01 runs on the demo VM before M15_02, the better the duration model will be.
