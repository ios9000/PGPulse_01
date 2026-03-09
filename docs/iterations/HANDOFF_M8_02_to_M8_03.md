# PGPulse — Iteration Handoff: M8_02 → M8_03

---

## DO NOT RE-DISCUSS

These decisions are final:

- Plan capture uses `EXPLAIN` only — never `EXPLAIN ANALYZE` on active queries
- Settings history is full snapshots per event (not delta), Go-side diff
- STL baseline is simplified (EWMA trend + period-folded seasonal mean) — not full Loess
- ML model state is recomputed from TimescaleDB on startup — no persistence yet
- Forecast horizon (predict next N points) is out of scope until M8_03+
- `MetricAlertAdapter` is the canonical bridge between `collector.AlertEvaluator` and `alert.Evaluator`
- `InstanceLister` is a separate interface from `MetricStore` — do not merge them

---

## What Was Just Completed (M8_02)

Three features shipped and wired end-to-end:

### 1. Auto-Capture Query Plans
- **Triggers:** duration threshold (primary), manual API (mandatory), scheduled top-N CPU (secondary), plan hash diff (signal)
- **Dedup:** upsert on `(instance_id, query_fingerprint, plan_hash)` — identical plans update `captured_at` only
- **Cap:** 64KB per plan, `truncated=true` flag if exceeded
- **Retention:** hourly cleanup goroutine, default 30 days
- **Routes:** `POST /api/v1/instances/{id}/plans/capture`, `GET /plans`, `GET /plans/{plan_id}`, `GET /plans/regressions`

### 2. Temporal Settings Diff
- **Capture:** startup (once per instance per server lifetime), scheduled (24h), manual API
- **Storage:** full `pg_settings` snapshot as JSONB — ~300 rows per capture
- **Diff:** Go-side comparison returns changed/added/removed/pending_restart
- **Routes:** `POST /settings/snapshot`, `GET /settings/history`, `GET /settings/diff?from=&to=`, `GET /settings/latest`, `GET /settings/pending-restart`

### 3. ML Anomaly Detection
- **Method:** STL decomposition (EWMA trend + period-folded seasonal mean) → residuals → Z-score + IQR flagging
- **Bootstrap:** loads last `max(3*period, 1000)` points from TimescaleDB on startup, 30s timeout
- **Alert integration:** anomalies dispatch through `MetricAlertAdapter` → real `alert.Evaluator` pipeline
- **Default rules seeded:** Z>3 → warning, Z>5 → critical on `anomaly.*` metric prefix
- **Configured metrics:** `connections.utilization_pct`, `cache.hit_ratio`, `transactions.commit_rate`, `replication.lag_bytes`, `locks.blocking_count` (all period=1440)

---

## Key Interfaces (actual Go signatures from committed code)

```go
// internal/collector/collector.go (unchanged from M8_01)

type AlertEvaluator interface {
    Evaluate(ctx context.Context, metric string, value float64, labels map[string]string) error
}

type MetricStore interface {
    Write(ctx context.Context, points []MetricPoint) error
    Query(ctx context.Context, query MetricQuery) ([]MetricPoint, error)
    Close() error
}
```

```go
// internal/ml/detector.go

type InstanceLister interface {
    ListInstances(ctx context.Context) ([]string, error)
}

type Detector struct { /* unexported */ }

func NewDetector(cfg DetectorConfig, store collector.MetricStore,
    lister InstanceLister, evaluator collector.AlertEvaluator) *Detector

func (d *Detector) Bootstrap(ctx context.Context) error
func (d *Detector) Evaluate(ctx context.Context, points []collector.MetricPoint) ([]AnomalyResult, error)
func (d *Detector) SetAlertEvaluator(e collector.AlertEvaluator)
```

```go
// internal/ml/baseline.go

type STLBaseline struct { /* unexported */ }

func NewSTLBaseline(key string, period int) *STLBaseline
func (b *STLBaseline) Update(value float64)
func (b *STLBaseline) Score(value float64) (zScore float64, isIQR bool)
func (b *STLBaseline) ResidualStddev() float64
func (b *STLBaseline) Ready() bool  // true after Period*2 observations
```

```go
// internal/alert/adapter.go

type MetricAlertAdapter struct { /* unexported */ }

func NewMetricAlertAdapter(e *Evaluator) *MetricAlertAdapter
// satisfies collector.AlertEvaluator
func (a *MetricAlertAdapter) Evaluate(ctx context.Context,
    metric string, value float64, labels map[string]string) error
```

```go
// internal/plans/capture.go

type TriggerType string
const (
    TriggerDuration  TriggerType = "duration_threshold"
    TriggerManual    TriggerType = "manual"
    TriggerScheduled TriggerType = "scheduled_topn"
    TriggerHashDiff  TriggerType = "hash_diff_signal"
)

type Collector struct { /* unexported */ }

func NewCollector(cfg CaptureConfig, store CaptureStore) *Collector
func (c *Collector) Collect(ctx context.Context, pool *pgxpool.Pool, instanceID string, ic collector.InstanceContext) error
func (c *Collector) CaptureManual(ctx context.Context, pool *pgxpool.Pool, instanceID, dbname, query string) (PlanCapture, error)
```

```go
// internal/settings/snapshot.go

type SnapshotCollector struct { /* unexported */ }

func NewSnapshotCollector(cfg SnapshotConfig, store SnapshotStore) *SnapshotCollector
func (c *SnapshotCollector) Collect(ctx context.Context, pool *pgxpool.Pool, instanceID string) error
func (c *SnapshotCollector) CaptureManual(ctx context.Context, pool *pgxpool.Pool, instanceID string) error
```

---

## Files Added/Modified in M8_02

```
internal/plans/
  capture.go          ← plan capture collector (all triggers + dedup)
  store.go            ← PGPlanStore (upsert, list, get, regressions)
  retention.go        ← hourly cleanup goroutine

internal/settings/
  snapshot.go         ← settings snapshot collector
  store.go            ← PGSnapshotStore
  diff.go             ← DiffSnapshots (Go-side, no SQL)

internal/ml/
  config.go           ← DetectorConfig, MetricConfig, DefaultConfig()
  baseline.go         ← STLBaseline (gonum-backed)
  detector.go         ← Detector with Bootstrap + Evaluate + SetAlertEvaluator

internal/alert/
  adapter.go          ← MetricAlertAdapter (NEW — bridges interface mismatch)
  rules.go            ← 2 ML anomaly rules added (Z=3 warn, Z=5 crit)

internal/api/
  plan_handlers.go    ← ListPlans, GetPlan, ListRegressions, ManualCapture
  settings_handlers.go← SettingsHistory, SettingsDiff, SettingsLatest, PendingRestart, ManualSnapshot

internal/config/
  config.go           ← PlanCaptureConfig, SettingsSnapshotConfig, MLConfig added

migrations/
  008_plan_capture.sql
  009_settings_snapshots.sql

cmd/pgpulse-server/
  main.go             ← all M8_02 components wired

configs/
  pgpulse.example.yml ← plan_capture, settings_snapshot, ml sections

go.mod / go.sum       ← gonum v0.17.0

DELETED (M8_01 orphans, no routes registered):
  internal/api/plans.go            (was from M8_01, had unregistered handlers)
  internal/api/sessions.go         (was from M8_01, had unregistered handlers)
  internal/api/settings_diff.go    (was from M8_01, had unregistered handlers)
```

---

## Known Issues

| Issue | Status |
|-------|--------|
| Session kill/terminate handlers deleted | Were in sessions.go with no routes — deleted to fix lint. Reintroduce with route wiring in a future iteration (M8_03 or M9) |
| Settings diff and plan capture have no UI | API complete. Frontend work deferred |
| Integration tests for plan capture skipped in CI | Tagged `//go:build integration` — require testcontainers + `pg_sleep`. Run manually with `-tags integration` |
| ML baseline needs `Period*2` points before it scores | On fresh install, anomaly detection is silent until enough history accumulates. Expected behavior, documented in logs |
| `configInstanceLister` only sees instances from config | Instances added manually via API after startup are not visible to ML Bootstrap until restart. Acceptable for M8_02 — fix in M8_03 by querying the instances table instead |

---

## Next Task: M8_03

M8_03 scope is not yet defined. Candidates to prioritize at the start of the session:

1. **Fix `configInstanceLister`** — query the `instances` DB table instead of config so dynamically added instances get ML coverage without restart. Small, high-value, low-risk.

2. **Session kill/terminate API** — reintroduce `handleCancelSession` / `handleTerminateSession` with proper route registration. Was deleted from M8_01 orphans; clean implementation is straightforward.

3. **ML model persistence** — serialize fitted `STLBaseline` state to storage DB so Bootstrap is instant on restart instead of recomputing from raw history. Reduces startup time on large deployments.

4. **Forecast horizon** — extend `STLBaseline` to predict next N points using the fitted trend + seasonal components. Was explicitly deferred from M8_02.

5. **Settings diff + plan capture UI** — frontend components for the two new APIs. Depends on Frontend Agent.

Recommended start: items 1 + 2 together (both small, clear scope), then decide on 3 vs 4 based on deployment feedback.

---

## Workflow Reminder

```bash
# Start new session
cd C:\Users\Archer\Projects\PGPulse_01
claude --model claude-opus-4-6

# Build verification sequence
cd web && npm run build && npm run typecheck && npm run lint
cd .. && go build ./cmd/pgpulse-server && go test ./cmd/... ./internal/...
golangci-lint run

# Commit
git add . && git commit -m "..." && git push
```
