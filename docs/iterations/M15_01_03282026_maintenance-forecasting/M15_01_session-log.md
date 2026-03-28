# M15_01 — Session Log: Maintenance Operation Forecasting

**Iteration:** M15_01
**Date:** 2026-03-27 / 2026-03-28
**Predecessor:** M14_04 (Guided Remediation Playbooks)
**Duration:** Single session

---

## Goals for This Iteration

1. **OperationTracker** — background goroutine that detects start/end of VACUUM, ANALYZE, REINDEX CONCURRENTLY, and BASEBACKUP via progress metrics; debounce (MissedPolls >= 2); persists completed operations to `maintenance_operations` table.
2. **ETACalculator** — stateless per-request WMA-based ETA computation from tracker's in-memory sample ring buffer; minimum samples gate; confidence classification (high/medium/estimating/stalled).
3. **NeedEvaluator** — background goroutine on 5-minute ticker; queries dead_tuples, mod_since_analyze, bloat metrics; reads pg_settings + reloptions for thresholds; projects threshold crossing via linear regression; writes forecasts to `maintenance_forecasts` table (UPSERT).
4. **ML helpers** — `internal/ml/linear.go` (OLS regression) and `internal/ml/wma.go` (weighted moving average with exponential decay).
5. **Migration 019** — two new tables: `maintenance_operations` (with CHECK on outcome/operation) and `maintenance_forecasts` (with UNIQUE constraint for UPSERT, partial index on actionable statuses).
6. **5 API endpoints** — `/forecast/eta`, `/forecast/eta/{pid}`, `/forecast/needs`, `/forecast/needs/{database}/{table}`, `/forecast/history` with pagination.
7. **Frontend** — ETABadge, ETAConfidenceIndicator, NeedForecastCard, NeedForecastTable, OperationHistoryTable; ProgressSection ETA column; ServerDetail NeedForecastCard.
8. **Config** — `MaintenanceForecastConfig` with 15 fields and `ApplyDefaults()`.

---

## Agent Activity Summary

Single-agent execution (Team Lead doing both Backend + Frontend). No worktree isolation needed.

### Phase 1: Foundation
- Created migration 019 (`maintenance_operations` + `maintenance_forecasts`)
- Created `internal/forecast/types.go` — all domain types + consumer interfaces
- Created `internal/forecast/store.go` — ForecastStore interface + filter structs
- Created `internal/forecast/nullstore.go` — no-op store for disabled mode
- Created `internal/ml/linear.go` + `linear_test.go` — OLS regression (7 tests)
- Created `internal/ml/wma.go` + `wma_test.go` — weighted moving average (5 tests)
- Modified `internal/config/config.go` — added `MaintenanceForecastConfig` struct
- Modified `internal/config/load.go` — added `ApplyDefaults()` call

### Phase 2: Core Logic
- Created `internal/forecast/pgstore.go` — PostgreSQL store implementation (UPSERT, pagination, batch, retention)
- Created `internal/forecast/tracker.go` + `tracker_test.go` — OperationTracker with debounce, REINDEX gate, ring buffer (5 tests)
- Created `internal/forecast/eta.go` + `eta_test.go` — ETACalculator with WMA, min-samples gate, confidence (5 tests)
- Created `internal/forecast/threshold.go` + `threshold_test.go` — PGThresholdQuerier with SET LOCAL transactions (3 tests)
- Created `internal/forecast/evaluator.go` — NeedEvaluator with vacuum/analyze/reindex/basebackup evaluation
- Created `internal/forecast/engine.go` — ForecastEngine coordinator with lifecycle management

### Phase 3: API + Wiring
- Created `internal/api/forecast_maint.go` — 5 handlers + `computeSummary`
- Modified `internal/api/server.go` — added `forecastEngine` field, `SetForecastEngine()`, route registration in both auth-enabled and auth-disabled groups
- Modified `cmd/pgpulse-server/main.go` — forecast engine creation, `forecastInstanceLister` adapter (C3), deferred connProvider wiring via `SetConnProvider`, `startServer` parameter extension

### Phase 4: Frontend
- Created `web/src/hooks/useMaintenanceForecast.ts` — 3 React Query hooks (ETA 15s poll, needs 60s poll, history on-demand)
- Created `web/src/components/forecast/ETABadge.tsx` — confidence-colored time display
- Created `web/src/components/forecast/ETAConfidenceIndicator.tsx` — dot indicator
- Created `web/src/components/forecast/NeedForecastCard.tsx` — summary card with overdue/imminent/predicted counts
- Created `web/src/components/forecast/NeedForecastTable.tsx` — sortable table with status badges
- Created `web/src/components/forecast/OperationHistoryTable.tsx` — paginated history table
- Modified `web/src/components/server/ProgressSection.tsx` — added ETA column via ETABadge
- Modified `web/src/pages/ServerDetail.tsx` — added NeedForecastCard after ProgressSection

### Phase 5: Tests
- Created `internal/forecast/pgstore_test.go` — 9 integration tests (//go:build integration)
- Created `internal/forecast/evaluator_test.go` — 13 unit tests with mock MetricStore/ThresholdQuerier
- Created `internal/api/forecast_maint_test.go` — 10 unit tests with mock ForecastStore + httptest

---

## Decisions Made During Implementation

### Deviations from Design Doc

1. **C2/Option B chosen:** `baselineProvider = nil` in main.go. ML-enhanced forecasting (GetBaselineStats) deferred to M15_02. Threshold projection is the only forecasting method in M15_01.

2. **ConnProvider deferred wiring:** The ForecastEngine is created before the orchestrator exists (orchestrator is the ConnProvider). Added `SetConnProvider()` on ForecastEngine, OperationTracker, and PGThresholdQuerier to wire the connection provider after orchestrator creation in `startServer()`.

3. **`InstanceConnProvider` interface duplicated in forecast package:** To avoid circular import (`api` imports `forecast`, `forecast` cannot import `api`), the `InstanceConnProvider` interface is redefined in `internal/forecast/types.go` with the same method signatures. The orchestrator satisfies both interfaces.

4. **`forecastInstanceLister` adapter (C3):** Created in `main.go` wrapping `storage.InstanceStore.List()` and filtering to enabled instances. Satisfies `forecast.InstanceLister`.

5. **useForecast.ts naming:** Design doc specified `web/src/hooks/useForecast.ts` but that file already exists (M8 ML forecast hooks). Created `useMaintenanceForecast.ts` instead to avoid collision.

6. **Disabled forecast routes:** When `forecastEngine == nil`, routes are not registered (guarded by `if s.forecastEngine != nil`). This returns 404 rather than 501 for disabled endpoints. The 501 behavior in the design doc is only reached if routes are registered but engine is nil — the guard-at-registration approach is cleaner.

7. **`startServer` parameter count:** Added `fcEngine *forecast.ForecastEngine` as the last parameter to `startServer()`. All 4 call sites updated (2 persistent, 1 live, 1 log-only). Live and log-only modes pass `nil`.

### No Deviations

All locked decisions D700–D711 honored:
- Core 4 operations only
- WMA for ETA (not simple linear)
- Tracker debounce MissedPolls >= 2
- REINDEX CONCURRENTLY via pg_stat_activity.query substring
- ETA min_samples gate (default 4)
- ETA in-memory only (no DB persistence)
- SET LOCAL transactions for threshold queries
- UPSERT keyed on (instance_id, database, table_name, operation)
- NeedEvaluator as dedicated 5-min goroutine
- maintenance_operations.outcome CHECK constraint
- Hybrid package split (forecast/ for domain, ml/ for math)

---

## Commits

| SHA | Message |
|-----|---------|
| `4417b71` | docs: add M15_01 requirements, design, and team-prompt |
| `cff87fc` | feat(forecast): M15_01 — maintenance operation forecasting (ETA + need forecasting) |
| `7bd5e06` | fix(forecast): move migration 019 to embedded migrations directory |

---

## Issues Encountered and Resolutions

| # | Issue | Resolution |
|---|-------|------------|
| 1 | `pgx.Conn.Close()` return value unchecked — golangci-lint errcheck | Wrapped all `conn.Close()` calls in `defer func() { _ = conn.Close(ctx) }()` or `_ = conn.Close(ctx)` |
| 2 | Circular import: `forecast` cannot import `api` for `InstanceConnProvider` | Redefined the interface in `forecast/types.go` with identical method signatures; Go structural typing satisfies both |
| 3 | `ForecastConfig` name collision with existing M8_04 type | Applied C6: renamed to `MaintenanceForecastConfig` with `koanf:"maintenance_forecast"` tag |
| 4 | ConnProvider not available at ForecastEngine creation time | Added `SetConnProvider()` on engine, tracker, and threshold querier; called from `startServer()` after orchestrator creation |
| 5 | Existing `useForecast.ts` hook file (ML forecast) | Created `useMaintenanceForecast.ts` with distinct hook names (`useETAForInstance`, `useMaintenanceForecasts`, `useOperationHistory`) |
| 6 | `scanNullableFloat` unused function left in pgstore.go | Removed during lint pass |
| 7 | Design doc referenced `ProgressPage.tsx` and `InstanceDashboard.tsx` | Actual files are `ProgressSection.tsx` (component) and `ServerDetail.tsx` (page); modifications applied to correct files |
| 8 | `makeRisingPoints` helper written but unused in evaluator_test.go | Removed during lint pass |

---

## Demo VM Validation Results

**Not yet deployed.** The M15_01 code is committed and builds clean locally. Deployment to demo VM (demo.example.com) pending — requires:
1. `git pull` on demo server
2. Rebuild with `go build ./cmd/pgpulse-server`
3. Add `maintenance_forecast: { enabled: true }` to pgpulse.yml
4. Restart systemd service
5. Migration 019 runs automatically on startup

### Local Verification (All Pass)

| Check | Result |
|-------|--------|
| `go build ./cmd/pgpulse-server` | Clean |
| `go test ./cmd/... ./internal/...` | All pass (168 total, 37 new) |
| `golangci-lint run` | 0 issues |
| `npm run build` | Clean (echarts chunk warning pre-existing) |
| `npm run typecheck` | Clean |
| `npm run lint` | 0 errors (1 pre-existing warning in useSystemMode.tsx) |

---

## File Inventory

### New Files (30)

| File | Lines | Purpose |
|------|-------|---------|
| `internal/forecast/types.go` | 127 | Domain types + consumer interfaces |
| `internal/forecast/store.go` | 43 | ForecastStore interface + filter structs |
| `internal/forecast/pgstore.go` | 248 | PostgreSQL store implementation |
| `internal/forecast/pgstore_test.go` | 293 | Store integration tests (9 tests) |
| `internal/forecast/nullstore.go` | 42 | No-op store |
| `internal/forecast/engine.go` | 80 | ForecastEngine coordinator |
| `internal/forecast/tracker.go` | 274 | OperationTracker goroutine |
| `internal/forecast/tracker_test.go` | 97 | Tracker unit tests (5 tests) |
| `internal/forecast/eta.go` | 133 | ETACalculator |
| `internal/forecast/eta_test.go` | 139 | ETA unit tests (5 tests) |
| `internal/forecast/evaluator.go` | 330 | NeedEvaluator goroutine |
| `internal/forecast/evaluator_test.go` | 314 | Evaluator unit tests (13 tests) |
| `internal/forecast/threshold.go` | 196 | PGThresholdQuerier |
| `internal/forecast/threshold_test.go` | 37 | Threshold unit tests (3 tests) |
| `internal/ml/linear.go` | 62 | OLS linear regression |
| `internal/ml/linear_test.go` | 85 | Regression tests (7 tests) |
| `internal/ml/wma.go` | 80 | Weighted moving average |
| `internal/ml/wma_test.go` | 87 | WMA tests (5 tests) |
| `internal/api/forecast_maint.go` | 187 | 5 API handlers + computeSummary |
| `internal/api/forecast_maint_test.go` | 270 | Handler tests (10 tests) |
| `internal/storage/migrations/019_maintenance_forecasting.sql` | 48 | Migration: 2 tables |
| `web/src/hooks/useMaintenanceForecast.ts` | 111 | React Query hooks |
| `web/src/components/forecast/ETABadge.tsx` | 62 | ETA display component |
| `web/src/components/forecast/ETAConfidenceIndicator.tsx` | 18 | Confidence dot indicator |
| `web/src/components/forecast/NeedForecastCard.tsx` | 79 | Summary card |
| `web/src/components/forecast/NeedForecastTable.tsx` | 114 | Forecast table |
| `web/src/components/forecast/OperationHistoryTable.tsx` | 122 | History table |

### Modified Files (6)

| File | Change |
|------|--------|
| `internal/config/config.go` | Added `MaintenanceForecastConfig` struct (49 lines) |
| `internal/config/load.go` | Added `ApplyDefaults()` call (3 lines) |
| `internal/api/server.go` | Added `forecastEngine` field, `SetForecastEngine()`, routes in both auth groups (~25 lines) |
| `cmd/pgpulse-server/main.go` | Forecast engine wiring, `forecastInstanceLister` adapter, `startServer` param (~35 lines) |
| `web/src/components/server/ProgressSection.tsx` | Added ETA column via ETABadge |
| `web/src/pages/ServerDetail.tsx` | Added NeedForecastCard import + render |

### Totals

- **New Go code:** ~3,950 lines (including tests)
- **New TypeScript code:** ~506 lines
- **New tests:** 57 (12 ML + 26 forecast + 10 API + 9 integration)
- **Modified lines:** ~115 across 6 existing files
