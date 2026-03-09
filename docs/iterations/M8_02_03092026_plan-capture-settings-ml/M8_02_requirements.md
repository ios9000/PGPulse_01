# M8_02 Requirements
## Auto-Capture Plans + Temporal Settings Diff + ML Anomaly Detection

**Iteration:** M8_02
**Date:** 2026-03-09
**Milestone:** M8 — ML Phase 1
**Status:** Ready for implementation

---

## Context

M8_01 established the ML foundation (gonum integration, MetricStore query pipeline,
baseline data structures). M8_02 ships three production features that depend on that
foundation:

1. **Auto-capture query plans** — capture `EXPLAIN` output triggered by duration,
   schedule, hash diff, or manual API call
2. **Temporal settings diff** — full `pg_settings` snapshots with event-driven
   capture and diff API
3. **ML anomaly detection** — STL-decomposed baseline per metric with Z-score/IQR
   flagging wired into the existing alert pipeline

---

## Feature 1: Auto-Capture Query Plans

### Goals
- Capture `EXPLAIN (FORMAT JSON)` for queries of interest without DBA intervention
- Store plans efficiently (deduplicate by plan hash, cap per-plan size)
- Surface plan regressions when shape changes between captures

### Trigger Priority

| Priority | Trigger | Description |
|----------|---------|-------------|
| Primary | Duration threshold | Active query in `pg_stat_activity` with `now() - query_start` exceeding configurable threshold (default: 1s). Run `EXPLAIN (FORMAT JSON)` on the query text immediately. |
| Mandatory | Manual API | `POST /api/v1/instances/:id/plans/capture` with `{query: "...", label: "..."}`. Always supported regardless of other triggers. |
| Secondary | Scheduled top-N | Each collection cycle, capture plans for top-N queries by `total_exec_time` from `pg_stat_statements` (default N=10, interval=1h). Runs `EXPLAIN (FORMAT JSON)` on normalized query text. |
| Secondary | Plan hash diff | After any capture, compare plan JSON hash against most-recent stored plan for same query fingerprint. Emit a plan regression event if hash changes. |

### Constraints
- `EXPLAIN` only — no `ANALYZE` on active queries (would affect the running query)
- For `pg_stat_statements`-triggered captures, run `EXPLAIN` on the normalized query text against the database where `dbid` matches; skip if query text is incomplete (`<too long>`)
- Duration-threshold captures: deduplicate within a 60s window per `(instance_id, query_fingerprint)` to prevent storm on a slow repeated query
- Hard cap: 64KB per plan JSON (truncate with a `truncated: true` flag in metadata if exceeded)
- Retention: 30 days (configurable via config YAML)

### Storage Schema

```sql
CREATE TABLE query_plans (
    id              BIGSERIAL PRIMARY KEY,
    instance_id     TEXT        NOT NULL,
    database_name   TEXT        NOT NULL,
    query_fingerprint TEXT      NOT NULL,  -- md5 of normalized query text
    plan_hash       TEXT        NOT NULL,  -- sha256 of plan JSON (before truncation)
    plan_json       JSONB,                  -- EXPLAIN output; NULL if truncated beyond recovery
    plan_text       TEXT,                   -- raw JSON string, capped at 64KB
    trigger_type    TEXT        NOT NULL,  -- 'duration_threshold' | 'manual' | 'scheduled_topn' | 'hash_diff_signal'
    duration_ms     BIGINT,                -- query duration at capture time (NULL for scheduled)
    query_text      TEXT,                   -- first 4096 chars of query
    truncated       BOOLEAN     NOT NULL DEFAULT FALSE,
    metadata        JSONB,                  -- trigger-specific extras (e.g. top_n_rank, threshold_used)
    captured_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_query_plans_instance_fingerprint ON query_plans(instance_id, query_fingerprint, captured_at DESC);
CREATE INDEX idx_query_plans_instance_captured ON query_plans(instance_id, captured_at DESC);
```

Deduplication: before INSERT, check if `plan_hash` already exists for the same
`(instance_id, query_fingerprint)`. If yes, skip INSERT but update `captured_at`
on the existing row (upsert on `plan_hash` per fingerprint).

### API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/v1/instances/:id/plans/capture` | Trigger manual capture (body: `{query, database, label}`) |
| `GET`  | `/api/v1/instances/:id/plans` | List captured plans (filters: `?fingerprint=`, `?since=`, `?trigger=`) |
| `GET`  | `/api/v1/instances/:id/plans/:plan_id` | Full plan JSON for one capture |
| `GET`  | `/api/v1/instances/:id/plans/regressions` | List plan hash changes (shape regressions) |

### Configuration

```yaml
plan_capture:
  enabled: true
  duration_threshold_ms: 1000        # trigger on queries > 1s
  dedup_window_seconds: 60           # suppress re-capture within window
  scheduled_topn_count: 10           # top-N queries per scheduled capture
  scheduled_topn_interval: "1h"      # how often to run scheduled capture
  max_plan_bytes: 65536              # 64KB cap
  retention_days: 30
```

---

## Feature 2: Temporal Settings Diff

### Goals
- Maintain a complete history of `pg_settings` for each monitored instance
- Detect and surface configuration changes (e.g. after `pg_reload_conf()`, restart)
- Provide a diff API between any two snapshots

### Capture Triggers

| Trigger | Description |
|---------|-------------|
| Startup detection | On first successful connection to an instance (and after reconnect following downtime), always capture a snapshot |
| Scheduled | Once per day per instance (configurable) |
| Manual | `POST /api/v1/instances/:id/settings/snapshot` |

Note: full snapshot per capture (not delta). Storage cost is low — `pg_settings`
is ~300 rows of text. A full snapshot per day per instance is ~50KB/instance/day.

### Storage Schema

```sql
CREATE TABLE settings_snapshots (
    id           BIGSERIAL    PRIMARY KEY,
    instance_id  TEXT         NOT NULL,
    captured_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),
    trigger_type TEXT         NOT NULL,  -- 'startup' | 'scheduled' | 'manual'
    pg_version   TEXT         NOT NULL,  -- version string at capture time
    settings     JSONB        NOT NULL   -- {name: {setting, unit, source, pending_restart}}
);

CREATE INDEX idx_settings_snapshots_instance ON settings_snapshots(instance_id, captured_at DESC);
```

Settings JSONB shape:
```json
{
  "max_connections": {"setting": "200", "unit": null, "source": "configuration file", "pending_restart": false},
  "shared_buffers":  {"setting": "4096", "unit": "8kB", "source": "configuration file", "pending_restart": false}
}
```

### Diff Logic

Implemented server-side in Go (not in SQL). Given two snapshot IDs:

```
changed: rows where name exists in both but setting differs
added:   rows in newer snapshot missing from older (extension added a GUC)
removed: rows in older snapshot missing from newer (GUC dropped)
pending_restart: rows where pending_restart=true in newer snapshot
```

### API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/v1/instances/:id/settings/snapshot` | Trigger manual snapshot |
| `GET`  | `/api/v1/instances/:id/settings/history` | List snapshots (id, captured_at, trigger_type, pg_version) |
| `GET`  | `/api/v1/instances/:id/settings/diff?from=:id&to=:id` | Diff between two snapshot IDs |
| `GET`  | `/api/v1/instances/:id/settings/latest` | Most recent full settings snapshot |
| `GET`  | `/api/v1/instances/:id/settings/pending-restart` | Parameters with `pending_restart=true` from latest snapshot |

### Configuration

```yaml
settings_snapshot:
  enabled: true
  scheduled_interval: "24h"
  capture_on_startup: true
  retention_days: 90
```

---

## Feature 3: ML Anomaly Detection

### Goals
- Compute a STL-decomposed baseline per metric per instance
- Flag anomalies using Z-score on STL residuals (primary) and IQR (secondary)
- Wire anomaly events into the existing `AlertEvaluator` interface
- Model state computed on startup from TimescaleDB history (no persistence of fitted state)

### In Scope (M8_02)
- STL decomposition (simplified: trend via EWMA, seasonal via period folding, residuals = actual - trend - seasonal)
- Z-score anomaly: `|residual| / stddev(residuals) > threshold`
- IQR anomaly: `value < Q1 - 1.5*IQR` or `value > Q3 + 1.5*IQR`
- Both methods run; flag if **either** triggers (OR logic, configurable to AND)
- Alert integration: anomaly emits through `AlertEvaluator` as `metric = "anomaly." + original_metric`
- Configurable per-metric: enable/disable, thresholds, seasonal period

### Out of Scope (M8_02)
- Forecast horizon / predict next N points (deferred to M8_03)
- Model state serialization / persistence across restarts
- Multivariate anomaly detection
- Automatic seasonal period detection (period is configured, not inferred)

### Metrics with ML Baseline (Default Set)

| Metric | Default Period | Notes |
|--------|---------------|-------|
| `connections.utilization_pct` | 1440 (daily at 1min interval) | Connection count has strong daily pattern |
| `cache.hit_ratio` | 1440 | |
| `transactions.commit_rate` | 1440 | |
| `replication.lag_bytes` | 1440 | |
| `locks.blocking_count` | 1440 | |
| `statements.total_exec_time_ms` | 1440 | |
| `bgwriter.checkpoint_rate` | 10080 (weekly) | Checkpoint patterns can be weekly |

Configurable: any metric key can be added to the ML set via YAML.

### STL Implementation (gonum-based)

Simplified STL suitable for M8_02 foundation:

```
1. Trend:    EWMA over window W (W = 2 * period + 1)
2. Seasonal: period-folded median
             seasonal[i] = median(values[i mod period]) - overall_mean
3. Residual: actual[t] - trend[t] - seasonal[t mod period]
4. Baseline: online-updated using ring buffer of last max(3*period, 1000) points
5. Z-score:  (current_residual - mean(residuals)) / stddev(residuals)
6. IQR:      compute Q1, Q3 over residuals; flag if outside [Q1-1.5*IQR, Q3+1.5*IQR]
```

### Key Interfaces

```go
// internal/ml/baseline.go

type STLBaseline struct {
    MetricKey      string
    Period         int
    windowSize     int
    values         []float64  // ring buffer
    trend          []float64
    seasonal       []float64
    residuals      []float64
    LastUpdated    time.Time
}

func (b *STLBaseline) Update(value float64, ts time.Time)
func (b *STLBaseline) Score(value float64) (zScore float64, isIQROutlier bool)

// internal/ml/detector.go

type AnomalyResult struct {
    InstanceID  string
    Metric      string
    Value       float64
    ZScore      float64
    IsIQR       bool
    IsAnomaly   bool
    Timestamp   time.Time
}

type Detector struct {
    baselines map[string]*STLBaseline  // key: "instance_id:metric"
    config    DetectorConfig
    store     MetricStore
    evaluator AlertEvaluator
}

func (d *Detector) Bootstrap(ctx context.Context) error  // load history, fit baselines
func (d *Detector) Evaluate(ctx context.Context, points []MetricPoint) ([]AnomalyResult, error)
```

### Alert Integration

Anomaly results are routed through `AlertEvaluator.Evaluate()`:
- `metric`: `"anomaly." + original_metric` (e.g. `"anomaly.connections.utilization_pct"`)
- `value`: Z-score (float64)
- `labels`: `{"instance_id": "...", "method": "zscore|iqr", "original_value": "..."}`

The existing alert rule engine handles threshold comparison (`z_score > 3.0` → WARNING,
`z_score > 5.0` → CRITICAL) using standard rule config. Default ML alert rules are
seeded into the DB on startup using the existing rules seeding pattern.

### Bootstrap Behavior

On startup:
1. Query TimescaleDB for last `max(3 * period, 1000)` points per configured metric per instance
2. Feed all historical points into `STLBaseline.Update()` in chronological order
3. Log baseline fit quality (stddev of residuals) per metric at INFO level
4. If fewer than `period` points exist for a metric: skip that metric, log WARNING, retry on next startup

Startup bootstrap is synchronous before the collector loop begins. Target: complete
within 10 seconds for default metric set on a typical deployment.

### Configuration

```yaml
ml:
  enabled: true
  zscore_threshold_warning: 3.0
  zscore_threshold_critical: 5.0
  anomaly_logic: "or"               # "or" = flag if either Z-score OR IQR; "and" = both required
  metrics:
    - key: "connections.utilization_pct"
      period: 1440
      enabled: true
    - key: "cache.hit_ratio"
      period: 1440
      enabled: true
    - key: "replication.lag_bytes"
      period: 1440
      enabled: true
```

---

## Files to Create / Modify

### New Files

| File | Owner | Description |
|------|-------|-------------|
| `internal/plans/capture.go` | Collector Agent | Plan capture logic (all 4 triggers) |
| `internal/plans/store.go` | API Agent | Plan storage + dedup logic |
| `internal/api/plans.go` | API Agent | Plan API endpoints |
| `internal/settings/snapshot.go` | Collector Agent | Settings snapshot capture |
| `internal/settings/diff.go` | API Agent | Diff computation between two snapshots |
| `internal/api/settings.go` | API Agent | Settings API endpoints |
| `internal/ml/baseline.go` | Collector Agent | STL baseline implementation (gonum) |
| `internal/ml/detector.go` | Collector Agent | AnomalyDetector + Bootstrap |
| `internal/ml/detector_test.go` | QA Agent | Unit + integration tests |
| `migrations/007_plan_capture.sql` | API Agent | `query_plans` table |
| `migrations/008_settings_snapshots.sql` | API Agent | `settings_snapshots` table |

### Modified Files

| File | Change |
|------|--------|
| `internal/collector/instance.go` | Wire settings snapshot trigger on startup/reconnect |
| `internal/alert/rules.go` | Seed default ML anomaly alert rules |
| `cmd/pgpulse-server/main.go` | Bootstrap `Detector` before collector loop starts |
| `configs/pgpulse.example.yml` | Add `plan_capture`, `settings_snapshot`, `ml` sections |
| `.claude/CLAUDE.md` | Update current iteration |

---

## Testing Requirements

- Unit tests for STL baseline: verify trend/seasonal/residual decomposition on synthetic data with known pattern
- Unit tests for anomaly scoring: inject known outlier, assert Z-score exceeds threshold
- Unit tests for settings diff: two manually constructed snapshots, verify changed/added/removed output
- Unit tests for plan dedup: two captures with same plan hash → assert second is upserted, not inserted
- Integration test (testcontainers PG 16): duration-threshold trigger fires for a `SELECT pg_sleep(2)` query
- Integration test: scheduled top-N capture returns plans for at least 1 query when `pg_stat_statements` has data
- Integration test: manual API capture returns 200 and plan is stored
- golangci-lint: 0 issues
- No `fmt.Sprintf` in any SQL string

---

## Non-Goals for M8_02

- `EXPLAIN ANALYZE` (would affect running queries — never in this trigger path)
- Plan regression notifications via UI (UI work deferred)
- ML forecast / predict next N points
- Automatic seasonal period detection
- Persisting fitted ML model state across restarts
