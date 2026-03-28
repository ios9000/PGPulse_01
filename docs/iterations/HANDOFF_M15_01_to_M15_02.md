# PGPulse — Iteration Handoff: M15_01 → M15_02

**Date:** 2026-03-28
**From:** M15_01 (Maintenance Forecasting Foundation — ETA + Need Forecasting)
**To:** M15_02 (Window Feasibility + Full UI + Integrations)

---

## DO NOT RE-DISCUSS

All D400–D711 decisions remain locked. M15_01 is complete. Additionally:

- Core 4 operations only: VACUUM, ANALYZE, REINDEX CONCURRENTLY, BASEBACKUP
- WMA for ETA, threshold projection for need forecasting — not ML-only
- OperationTracker debounce MissedPolls >= 2 — non-negotiable
- REINDEX CONCURRENTLY identification via pg_stat_activity.query — not relname heuristics
- ETA in-memory only — does NOT survive restart, do NOT persist to DB
- SET LOCAL transactions for threshold queries — never session-level SET
- MaintenanceForecastConfig (not ForecastConfig) — avoids M8 collision
- BaselineProvider = nil in M15_01 — ML enhancement deferred to M15_02

---

## What Was Just Completed (M15_01)

### Backend — `internal/forecast/` (14 files, 2,911 lines)

| File | Lines | Purpose |
|------|-------|---------|
| types.go | 144 | Domain types: TrackedOperation, MaintenanceOperation, OperationETA, MaintenanceForecast, ForecastSummary, TableThresholds + consumer interfaces (InstanceConnProvider, BaselineProvider, InstanceLister, ThresholdQuerier) |
| store.go | 43 | ForecastStore interface + OperationFilter, ForecastFilter |
| pgstore.go | 289 | PostgreSQL implementation: operations CRUD, forecast UPSERT, batch, cleanup |
| nullstore.go | 41 | No-op store for disabled mode |
| engine.go | 87 | ForecastEngine coordinator: lifecycle, SetConnProvider(), retention cleanup |
| tracker.go | 347 | OperationTracker goroutine: progress metric polling, start/end detection, debounce (MissedPolls >= 2), REINDEX gate via pg_stat_activity, ring buffer samples |
| eta.go | 145 | ETACalculator: WMA rate computation, min-samples gate (default 4), confidence classification (high/medium/estimating/stalled), stall detection |
| evaluator.go | 330 | NeedEvaluator goroutine (5-min ticker): vacuum/analyze threshold projection via linear regression, reindex via bloat ratio, basebackup returns insufficient_data (C4) |
| threshold.go | 240 | PGThresholdQuerier: per-database pg_settings + reloptions with SET LOCAL transactions |

### Backend — `internal/ml/` (4 new files)

| File | Lines | Purpose |
|------|-------|---------|
| linear.go | 70 | LinearRegression — OLS slope/intercept/R² |
| wma.go | 91 | WeightedMovingAverage — configurable window, exponential decay |

### Backend — API + Config + Wiring

| File | Lines | Purpose |
|------|-------|---------|
| api/forecast_maint.go | 205 | 5 handlers: handleForecastETA, handleForecastETAByPID, handleForecastNeeds, handleForecastNeedsForTable, handleForecastHistory |
| config/config.go | +64 | MaintenanceForecastConfig struct (15 fields) with ApplyDefaults() |
| api/server.go | +29 | forecastEngine field, SetForecastEngine(), route registration |
| cmd/pgpulse-server/main.go | +55 | ForecastEngine wiring, forecastInstanceLister adapter (C3) |

### Migration 019

```sql
CREATE TABLE maintenance_operations (
    id BIGSERIAL PRIMARY KEY,
    instance_id TEXT NOT NULL,
    operation TEXT NOT NULL CHECK (operation IN ('vacuum','analyze','reindex_concurrent','basebackup')),
    outcome TEXT NOT NULL DEFAULT 'unknown' CHECK (outcome IN ('completed','canceled','failed','disappeared','unknown')),
    -- ... started_at, completed_at, duration_sec, final_pct, avg_rate_per_sec, metadata JSONB
);

CREATE TABLE maintenance_forecasts (
    id BIGSERIAL PRIMARY KEY,
    instance_id TEXT NOT NULL,
    database TEXT, table_name TEXT, operation TEXT,
    status TEXT CHECK (status IN ('predicted','imminent','overdue','not_needed','insufficient_data')),
    -- ... predicted_at, time_until_sec, confidence bands, current_value, threshold_value, rate, method
    UNIQUE(instance_id, database, table_name, operation)  -- enables UPSERT
);
-- Partial index: WHERE status IN ('imminent', 'overdue')
```

### Frontend (6 new files, 506 lines)

| File | Lines | Purpose |
|------|-------|---------|
| hooks/useMaintenanceForecast.ts | 122 | useETAForInstance (15s poll), useMaintenanceForecasts (60s poll), useOperationHistory (on-demand) |
| components/forecast/ETABadge.tsx | 64 | Confidence-colored ETA display |
| components/forecast/ETAConfidenceIndicator.tsx | 19 | Dot indicator |
| components/forecast/NeedForecastCard.tsx | 82 | Summary card: overdue/imminent/predicted counts |
| components/forecast/NeedForecastTable.tsx | 109 | Sortable forecast table with status badges |
| components/forecast/OperationHistoryTable.tsx | 110 | Paginated completed operations log |

### Modified Pages

- **ProgressSection.tsx** — added ETA column via ETABadge, matched by PID
- **ServerDetail.tsx** — added NeedForecastCard after ProgressSection

### Tests

57 new tests, all passing:
- 12 ML tests (linear regression + WMA)
- 26 forecast tests (tracker, eta, evaluator, threshold)
- 10 API handler tests (mock + httptest)
- 9 pgstore integration tests (//go:build integration)

### Corrections Applied

| # | Issue | Resolution |
|---|-------|------------|
| C1 | ConnForDB already exists in connprovider.go | Did not modify — used as-is |
| C2 | ML Detector has no GetBaselineStats | Option B: baselineProvider=nil, threshold only |
| C3 | No ActiveInstanceIDs on orchestrator | forecastInstanceLister adapter wraps InstanceStore.List() |
| C4 | No pg.wal.bytes_rate metric | basebackup_interval=0 (disabled), returns insufficient_data |
| C5 | Basebackup has no datname/relname | Set Database="" and Table="" |
| C6 | ForecastConfig name collision with M8 | Renamed to MaintenanceForecastConfig |
| C7 | Config defaults are zero-value based | Added ApplyDefaults() method |
| C8 | Per-operation work unit mapping | vacuum=heap_blks_vacuumed, analyze=sample_blks_scanned, create_index=blocks_done, basebackup=backup_streamed |
| C9 | ConnFor/ConnForDB return *pgx.Conn | Close with conn.Close(ctx) not conn.Release() |

---

## Known Issues

| # | Issue | Severity | Notes |
|---|-------|----------|-------|
| 1 | Basebackup forecasting always returns insufficient_data | Low | C4: no pg.wal.bytes_rate metric exists. Requires new WAL rate collector or agent integration. |
| 2 | ML confidence bands not available | Low | C2/Option B: baselineProvider=nil. evaluateVacuum/evaluateAnalyze never set confidence_lower/confidence_upper. Wired in M15_02 via GetBaselineTrend. |
| 3 | No sidebar navigation for forecasts | Expected | Deferred to M15_02. Forecasts accessible via ServerDetail page only. |
| 4 | NeedEvaluator reads pg_settings per cycle | Medium | Each evaluation cycle opens ConnForDB per database. For instances with many databases (>20), this could create connection pressure. Consider caching thresholds across cycles. |
| 5 | maintenance_operations table starts empty | Expected | Operations accumulate as VACUUM/ANALYZE complete. First data appears after autovacuum runs (typically within minutes). |
| 6 | Demo VM not yet deployed with M15_01 | Pending | Requires git pull + rebuild + config update + restart |

---

## Integration Points Built But Not Wired

These hooks exist in the codebase but are NOT connected to the forecast subsystem. M15_02 wires them.

### 1. EvaluateHook() on remediation.Engine

```go
// internal/remediation/engine.go — exists since M14_03
func (e *Engine) EvaluateHook(ctx context.Context, hookID string, snapshot MetricSnapshot) error
```

M15_02 adds: when NeedEvaluator detects status="overdue", call `remEngine.EvaluateHook()` with `source: "forecast"`. This generates remediation recommendations from forecast findings.

### 2. Playbook Resolver

```go
// internal/playbook/resolver.go — exists since M14_04
func (r *Resolver) Resolve(ctx context.Context, bindingType, bindingValue string) (*Playbook, error)
```

M15_02 adds: forecast-triggered playbook resolution. When a vacuum forecast hits "overdue", resolve against `bindingType: "metric"`, `bindingValue: "pg.db.vacuum.dead_tuples"` to find the vacuum backlog playbook.

### 3. Forecast Alert Rules

```go
// internal/alert/evaluator.go — SetForecastProvider() exists since M8_05
func (e *Evaluator) SetForecastProvider(fp ForecastProvider, minConsecutive int)
```

M15_02 adds: forecast-based alert rules (e.g., "alert when any table is 'imminent' for vacuum for > 30 minutes"). Requires new rule type and evaluator logic.

### 4. ConnForDB

```go
// internal/api/connprovider.go — exists since M7 (used by playbook executor, threshold querier)
ConnForDB(ctx context.Context, instanceID, dbName string) (*pgx.Conn, error)
```

Available for M15_02's per-database queries. Already used by PGThresholdQuerier in M15_01. Orchestrator implementation clones DSN and swaps dbname.

---

## Codebase Scale (post-M15_01)

| Metric | Value |
|--------|-------|
| Go files | 301 |
| Go lines | ~50,100 |
| TypeScript files | 175 |
| TypeScript lines | ~13,650 |
| Metric keys | 148 |
| API endpoints | 80 |
| Alert rules | 23 |
| Remediation rules | 25 |
| Causal chains | 20 |
| Seed playbooks | 10 |
| SQL migrations | 19 |
| Collectors | 25 + 1 per-database |

---

## M15_02 Scope Reminder

**Window Feasibility + Full UI + Integrations**

From M15_01_requirements.md, these items are explicitly deferred to M15_02:

1. **Window Feasibility engine** — duration prediction model using maintenance_operations history
2. **Maintenance Calendar / Planner UI** — drag-and-drop scheduling of maintenance windows
3. **Fleet-wide Forecast Dashboard page** — /forecasts route with cross-instance view
4. **Forecast → Adviser integration** — EvaluateHook() with `source: "forecast"` for overdue/imminent
5. **Forecast → Playbook integration** — maintenance playbooks via Resolver
6. **Forecast-based alert rules** — new rule type for forecast status thresholds
7. **Sidebar navigation for forecasts** — top-level nav item
8. **New router pages** — /forecasts fleet dashboard, /servers/:id/forecasts detail
9. **Operation history charts/graphs** — ECharts visualizations of operation duration trends
10. **Duration prediction from maintenance_operations history** — "last 10 vacuums on this table took X on average"
11. **ML confidence bands** — wire GetBaselineTrend into NeedEvaluator for confidence_lower/upper
12. **Basebackup WAL rate tracking** — requires new collector or agent metric

---

## Build & Deploy Commands

### Local Build Verification

```bash
cd web && npm run build && npm run lint && npm run typecheck && cd ..
go build ./cmd/pgpulse-server
go test ./cmd/... ./internal/... -count=1
golangci-lint run ./cmd/... ./internal/...
```

### Demo Server Deployment

```bash
ssh pgpulse@demo.example.com
cd /opt/pgpulse
git pull origin master
go build -o pgpulse-server ./cmd/pgpulse-server

# Add to pgpulse.yml:
#   maintenance_forecast:
#     enabled: true

sudo systemctl restart pgpulse
journalctl -u pgpulse -f  # watch for "forecast engine started"
```

### Verify Forecast Subsystem

```bash
# Check ETA endpoint (empty when no active maintenance)
curl -s http://localhost:8989/api/v1/instances/<id>/forecast/eta | jq .

# Check needs endpoint (populated after first 5-min evaluation cycle)
curl -s http://localhost:8989/api/v1/instances/<id>/forecast/needs | jq .data.summary

# Check operation history (accumulates as operations complete)
curl -s http://localhost:8989/api/v1/instances/<id>/forecast/history | jq .data.total
```

---

## Workflow Reminder

Claude Code agents run go build/test/lint/commit directly. No manual steps needed.
NEVER use `go test ./...` — scans web/node_modules/ and fails.
CGO_ENABLED=0 blocks -race on Windows MSYS2.
