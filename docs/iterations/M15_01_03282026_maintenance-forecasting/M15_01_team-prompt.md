# M15_01 — Team Prompt: Maintenance Operation Forecasting (Foundation + ETA + Need Forecasting)

**Iteration:** M15_01
**Date:** 2026-03-27
**Input docs:** `docs/iterations/M15_01_MMDDYYYY_maintenance-forecasting/M15_01_requirements.md`, `M15_01_design.md`
**Locked Decisions:** D700–D711
**Agents:** 2 (Backend + Frontend)

---

## DO NOT RE-DISCUSS — MANDATORY CONSTRAINTS

These decisions are locked. Do not re-litigate, simplify, defer, or modify any of these. If you encounter a design tension, resolve it within these constraints — never by relaxing them.

1. Core 4 operations only: VACUUM, ANALYZE, REINDEX (CONCURRENTLY only), BASEBACKUP — not CLUSTER, CREATE INDEX, COPY.
2. Hybrid package split: `internal/forecast/` owns domain logic, `internal/ml/` owns math (LinearRegression, WMA). Do not merge them.
3. Weighted moving average for ETA — not simple linear extrapolation, not phase-aware.
4. Threshold projection as primary need-forecasting method + ML confidence bands as enhancement. Not ML-only.
5. `maintenance_operations.outcome` field is mandatory — every persisted operation MUST have one of: `completed`, `canceled`, `failed`, `disappeared`, `unknown`. The migration MUST include the CHECK constraint.
6. Tracker debounce: `MissedPolls >= 2` before finalizing an operation. A single scrape gap does NOT trigger completion. This is non-negotiable.
7. REINDEX CONCURRENTLY identification: query `pg_stat_activity.query` for the PID and substring-match "REINDEX" (case-insensitive). If match fails, ambiguous, or query times out → classify as `create_index` and DO NOT track. Never use relname pattern heuristics.
8. ETA minimum samples gate: `forecast.eta_min_samples` (default 4). Below this threshold, return `confidence: "estimating"` with `eta_sec: -1`. Never return a numeric ETA with fewer than `min_samples` rate observations.
9. ETA is computed from in-memory tracker state. This is an intentional exception to "all state in PostgreSQL." ETA does NOT survive restart. Do NOT "fix" this by persisting samples to the database.
10. Threshold querier uses transaction-scoped `BEGIN; SET LOCAL ...; ROLLBACK;` — NEVER session-level `SET` on pooled connections. Use `pgx.Tx` for the transaction boundary.
11. `InstanceConnProvider` interface must be extended with `ConnForDB(ctx, instanceID, database string)` for per-database threshold queries.
12. `maintenance_forecasts` uses `INSERT ... ON CONFLICT DO UPDATE` keyed on `(instance_id, database, table_name, operation)` — one live row per table/operation, not append-only.
13. NeedEvaluator runs as a dedicated background goroutine on a 5-minute ticker. Not coupled to DatabaseCollector.
14. WMA window size configurable via `forecast.eta_window_size` (default 10). Decay factor via `forecast.eta_decay_factor` (default 0.85).
15. Operation history retention configurable via `forecast.retention_days` (default 90).
16. Two agents only. Backend owns all Go + migration + API. Frontend owns all TypeScript/React.

---

## Architecture Overview

Read `M15_01_design.md` Sections 1–2 for full data flow diagrams and type definitions.

```
Progress Collectors (existing, untouched)
    │
    ▼
MetricStore ──────────────────────────┐
    │                                  │
    ├─→ OperationTracker (goroutine)   ├─→ ETACalculator (stateless, per-request)
    │     detects start/end            │     reads tracker's in-memory samples
    │     writes to maintenance_operations   returns OperationETA
    │                                  │
    ├─→ NeedEvaluator (goroutine, 5min)│
    │     reads dead_tuples, bloat     │
    │     queries pg_settings/reloptions│
    │     writes to maintenance_forecasts
    │                                  │
    └──────────────────────────────────┘
                    │
                    ▼
            API handlers → Frontend
```

---

## Agent 1: Backend

### Owns (creates)

| File | Purpose | Est. Lines |
|------|---------|-----------|
| `internal/forecast/types.go` | All domain types: TrackedOperation, ProgressSample, MaintenanceOperation, OperationETA, MaintenanceForecast, ForecastSummary, TableThresholds | ~130 |
| `internal/forecast/store.go` | ForecastStore interface + filter structs | ~55 |
| `internal/forecast/pgstore.go` | PostgreSQL implementation of ForecastStore | ~300 |
| `internal/forecast/pgstore_test.go` | Store integration tests | ~200 |
| `internal/forecast/nullstore.go` | No-op store for disabled/live mode | ~35 |
| `internal/forecast/engine.go` | ForecastEngine coordinator: wires tracker + evaluator + ETA, lifecycle | ~90 |
| `internal/forecast/tracker.go` | OperationTracker: background goroutine, start/end detection, debounce, REINDEX gate | ~280 |
| `internal/forecast/tracker_test.go` | Tracker unit tests: debounce, completion, REINDEX classification | ~220 |
| `internal/forecast/eta.go` | ETACalculator: WMA-based ETA computation, min_samples gate, confidence classification | ~130 |
| `internal/forecast/eta_test.go` | ETA unit tests: WMA math, stall detection, estimating state | ~160 |
| `internal/forecast/evaluator.go` | NeedEvaluator: background goroutine, threshold projection, ML enhancement | ~370 |
| `internal/forecast/evaluator_test.go` | Evaluator unit tests: vacuum/analyze/reindex/basebackup forecasting | ~320 |
| `internal/forecast/threshold.go` | PGThresholdQuerier: pg_settings + reloptions, per-database iteration, SET LOCAL | ~170 |
| `internal/forecast/threshold_test.go` | Threshold tests: global defaults, per-table overrides, reloptions parsing | ~130 |
| `internal/ml/linear.go` | LinearRegression: OLS slope/intercept/R² | ~65 |
| `internal/ml/linear_test.go` | Regression tests: positive/zero/negative slope, insufficient data | ~85 |
| `internal/ml/wma.go` | WeightedMovingAverage: configurable window, exponential decay, stddev | ~65 |
| `internal/ml/wma_test.go` | WMA tests: even samples, decay correctness, edge cases | ~90 |
| `internal/api/forecast_maint.go` | 5 API handlers: ETA all, ETA by PID, needs, needs for table, history | ~260 |
| `internal/api/forecast_maint_test.go` | Handler tests | ~210 |
| `migrations/019_maintenance_forecasting.sql` | maintenance_operations + maintenance_forecasts tables | ~50 |

### Owns (modifies)

| File | Change |
|------|--------|
| `internal/config/config.go` | Add `ForecastConfig` struct with all 15 fields |
| `internal/config/load.go` | Add forecast defaults (see design doc Section 16) |
| `internal/api/connprovider.go` | Add `ConnForDB(ctx, instanceID, database string)` to `InstanceConnProvider` interface |
| `internal/api/server.go` | Add `SetForecastEngine(*forecast.ForecastEngine)` method + route registration |
| `cmd/pgpulse-server/main.go` | Wire ForecastEngine: create store, create engine, start goroutines, set on API server |

### Critical Implementation Rules

#### Database Access

1. **All SQL uses parameterized queries via pgx.** No `fmt.Sprintf` in any SQL string. No string concatenation for query building. Use `$1`, `$2`, etc.

2. **ThresholdQuerier MUST use `pgx.Tx` for transaction-scoped session settings:**
```go
// CORRECT — transaction-scoped, safe for pooled connections
tx, err := conn.Begin(ctx)
if err != nil { return nil, err }
defer tx.Rollback(ctx) // always rollback — we're read-only

tx.Exec(ctx, "SET LOCAL statement_timeout = '5s'")
tx.Exec(ctx, "SET LOCAL lock_timeout = '2s'")
tx.Exec(ctx, "SET LOCAL application_name = 'pgpulse_forecast'")
rows, err := tx.Query(ctx, "SELECT n.nspname, c.relname, ...")

// WRONG — pollutes the pooled connection for subsequent borrowers
conn.Exec(ctx, "SET statement_timeout = '5s'")
```

3. **ConnForDB implementation:** Clone the instance's DSN, replace the `dbname` parameter, and use `pgx.Connect()` (not pool). The caller MUST close the connection. Example approach:
```go
func (p *connProvider) ConnForDB(ctx context.Context, instanceID, database string) (*pgx.Conn, error) {
    cfg := p.getInstanceConfig(instanceID)
    connCfg, err := pgx.ParseConfig(cfg.DSN)
    if err != nil { return nil, err }
    connCfg.Database = database
    return pgx.ConnectConfig(ctx, connCfg)
}
```

4. **Set `application_name = 'pgpulse_forecast'`** on all direct database connections (threshold queries, REINDEX identification).

#### OperationTracker

5. **Debounce is non-negotiable.** The tick loop MUST:
   - For ops present in current metrics: reset `MissedPolls = 0`, append sample.
   - For ops absent from current metrics: increment `MissedPolls`. If `MissedPolls < 2`, do nothing. If `MissedPolls >= 2`, call `finalizeOp()`.
   - On context cancellation (shutdown): call `flushAll(ctx, "unknown")`.

6. **Outcome classification at finalization:**
   - `FinalPct >= 99.0` → `"completed"`
   - `FinalPct < 99.0` AND `elapsed > 2s` → `"disappeared"`
   - Everything else → `"unknown"`
   - On shutdown flush: always `"unknown"`

7. **REINDEX CONCURRENTLY identification** runs ONCE at `startOp()`, not every tick:
```go
// When a new create_index progress metric appears:
query := "SELECT query FROM pg_stat_activity WHERE pid = $1"
// Use ConnFor with 2s context timeout
// If query text contains "REINDEX" (case-insensitive) → track as "reindex_concurrent"
// If query text does NOT contain "REINDEX" → skip, do not track
// If query fails or times out → skip, do not track
```

8. **Ring buffer for samples:** Simple slice with eviction:
```go
if len(op.Samples) >= maxSize {
    op.Samples = op.Samples[1:]
}
op.Samples = append(op.Samples, sample)
```

9. **Thread safety:** `sync.RWMutex` on `OperationTracker`. Write operations (`startOp`, `updateOp`, `finalizeOp`, `flushAll`) take `Lock()`. Read operations (`GetActiveOps`) take `RLock()`. The ETACalculator calls `GetActiveOps` from API handler goroutines — it must never block the tracker's tick loop for long.

#### ETACalculator

10. **Minimum samples gate is enforced before WMA computation:**
```go
if len(op.Samples) < c.minSamples {
    return OperationETA{
        // ... fill from op fields ...
        ETASec:     -1,
        Confidence: "estimating",
        SampleCount: len(op.Samples),
    }
}
```

11. **Confidence classification** (only reached if `>= minSamples`):
    - `>= 8` samples → `"high"`
    - `4–7` samples → `"medium"`
    - (below `minSamples` is already handled as `"estimating"`)

12. **Stall detection:** If `wmaResult.WeightedRate <= 0`, return `ETASec: -1`, `Confidence: "stalled"`.

#### NeedEvaluator

13. **Runs immediately on startup**, then on 5-minute ticker. Don't wait 5 minutes for first evaluation.

14. **Per-cycle threshold caching:** Call `ThresholdQuerier.GetTableThresholds()` ONCE per instance per evaluation cycle. Do not query pg_settings per-table.

15. **ML enhancement is optional.** If `baselineProvider == nil` (ML disabled), use raw `ml.LinearRegression` for rate estimation. If `baselineProvider != nil`, attempt to get STL trend — if unavailable for this metric, fall back to linear regression. Never fail the entire evaluation because ML is unavailable.

16. **Status classification:**
    - `current >= threshold` → `"overdue"`
    - `time_to_threshold <= 3600s` → `"imminent"`
    - `time_to_threshold > 3600s` → `"predicted"`
    - `rate <= 0` → `"not_needed"`
    - `< min_data_points` samples → `"insufficient_data"`

#### Goroutine Lifecycle

17. **All goroutines accept `context.Context` and exit on cancellation.** Pattern:
```go
func (t *OperationTracker) Run(ctx context.Context) {
    ticker := time.NewTicker(t.pollInterval)
    defer ticker.Stop()
    for {
        select {
        case <-ctx.Done():
            t.flushAll(ctx, "unknown")
            return
        case <-ticker.C:
            t.tick(ctx)
        }
    }
}
```

18. **ForecastEngine.Start() launches goroutines with `go`:**
```go
func (e *ForecastEngine) Start(ctx context.Context) {
    go e.Tracker.Run(ctx)
    go e.Evaluator.Run(ctx)
    go e.retentionCleanup(ctx)
}
```
Do NOT use errgroup or waitgroup — context cancellation is sufficient for shutdown. The main function already handles graceful shutdown via signal trapping.

#### API Handlers

19. **Route registration** in `server.go` `Routes()` method:
```go
r.Route("/api/v1/instances/{id}/forecast", func(r chi.Router) {
    r.Use(requireAuth)
    r.Get("/eta", s.handleForecastETA)
    r.Get("/eta/{pid}", s.handleForecastETAByPID)
    r.Get("/needs", s.handleForecastNeeds)
    r.Get("/needs/{database}/{table}", s.handleForecastNeedsForTable)
    r.Get("/history", s.handleForecastHistory)
})
```

20. **Null-safe:** If `forecastEngine == nil` (forecast disabled), all handlers return `501 Not Implemented` with `{"error": "forecast subsystem is disabled"}`. Check once in a middleware or at the top of each handler.

21. **Pagination for history:** Use `page` + `per_page` query params (default page=1, per_page=50, max per_page=200). Return `total` count in response.

#### Migration 019

22. **The migration MUST include CHECK constraints on `outcome`, `operation`, and `status` columns** as specified in the design doc. Do not use bare TEXT without constraints.

23. **The UNIQUE constraint on `maintenance_forecasts(instance_id, database, table_name, operation)`** enables the UPSERT pattern. Do not omit it.

24. **Index `idx_maint_forecasts_status` uses a partial index** (`WHERE status IN ('imminent', 'overdue')`) for fast filtering of actionable forecasts.

#### Code Quality

25. **Every exported function and type in `internal/forecast/` MUST have a doc comment.** Follow existing codebase conventions.

26. **Error wrapping:** Use `fmt.Errorf("forecast: context: %w", err)` pattern. Never swallow errors silently in goroutines — log at minimum.

27. **Commit messages:** Use scope prefix: `feat(forecast): ...`, `feat(ml): ...`, `feat(api): ...`.

28. **Do NOT import from:** `internal/remediation/`, `internal/playbook/`, `internal/alert/`, `internal/rca/`. These integrations are deferred to M15_02. The forecast package must be self-contained.

---

## Agent 2: Frontend

### Owns (creates)

| File | Purpose | Est. Lines |
|------|---------|-----------|
| `web/src/hooks/useForecast.ts` | React Query hooks: useETAForInstance, useMaintenanceForecasts, useOperationHistory | ~65 |
| `web/src/components/forecast/ETABadge.tsx` | Inline ETA display with human-readable time formatting | ~80 |
| `web/src/components/forecast/ETAConfidenceIndicator.tsx` | Visual confidence level indicator (high/medium/low/estimating/stalled) | ~35 |
| `web/src/components/forecast/NeedForecastCard.tsx` | Summary card: imminent/overdue/predicted counts, "next" operation | ~100 |
| `web/src/components/forecast/NeedForecastTable.tsx` | Sortable table of per-table forecasts with status badges | ~150 |
| `web/src/components/forecast/OperationHistoryTable.tsx` | Paginated completed operations log | ~120 |

### Owns (modifies)

| File | Change |
|------|--------|
| `web/src/pages/ProgressPage.tsx` | Add ETA column using ETABadge, matched by PID |
| `web/src/pages/InstanceDashboard.tsx` | Add NeedForecastCard to dashboard grid |

### Critical Implementation Rules

#### Hooks

1. **Polling intervals:**
   - `useETAForInstance`: refetch every **15 seconds** (matches progress collector interval).
   - `useMaintenanceForecasts`: refetch every **60 seconds**.
   - `useOperationHistory`: on-demand (no auto-refetch), supports pagination.

2. **Use the existing `useApiQuery` / React Query infrastructure.** Do not introduce new data-fetching patterns. Follow the conventions in existing hooks like `usePlaybooks.ts`.

3. **TypeScript types:** Define interfaces for `OperationETA`, `MaintenanceForecast`, `ForecastSummary`, `MaintenanceOperation` in the hooks file (or a co-located `types.ts`). Match the JSON field names from the API response exactly.

#### ETABadge

4. **Time formatting rules:**
   - `< 60s` → `"< 1 min"`
   - `60s–3599s` → `"Xm"` (e.g., `"41m"`)
   - `3600s–86399s` → `"Xh Ym"` (e.g., `"2h 15m"`)
   - `>= 86400s` → `"Xd Yh"` (e.g., `"1d 4h"`)

5. **Confidence-based rendering:**
   - `"high"` → green text, no qualifier
   - `"medium"` → yellow text, `"~"` prefix
   - `"low"` → gray text (should not occur if min_samples gate works, but handle defensively)
   - `"estimating"` → gray text, pulsing animation, text: `"estimating..."`
   - `"stalled"` → red text, warning icon, text: `"Stalled"`

6. **When `eta_sec == -1`:** Check `confidence` field. If `"stalled"` → show "Stalled". If `"estimating"` → show "estimating...". Never show a negative number or "Infinity."

#### NeedForecastCard

7. **Summary counts** from `summary` field in API response. Color coding:
   - Overdue count: red background/text
   - Imminent count: yellow/amber
   - Predicted count: blue

8. **"Next" operation:** Show the forecast with the smallest `time_until_sec` where `status` is `"imminent"` or `"predicted"`. Format: `"vacuum on orders (~2h)"`.

9. **Empty state:** If no forecasts exist, show: `"No forecasts yet — data accumulating"` in gray text. This is expected for the first 5 minutes after enabling the forecast subsystem.

#### NeedForecastTable

10. **Default sort:** `overdue` first, then `imminent`, then `predicted` by `time_until_sec` ascending, then `insufficient_data`, then `not_needed`.

11. **Status badges:** Use consistent color scheme matching NeedForecastCard. Badges: `overdue` (red), `imminent` (yellow), `predicted` (blue), `not_needed` (gray), `insufficient_data` (gray, dashed border).

12. **Method column:** Display `"Threshold"` for `"threshold_projection"`, `"Threshold + ML"` for `"threshold_projection+ml"`. If ML confidence bounds exist, show them as a range: `"6h–8h"`.

#### ProgressPage Modification

13. **Add an "ETA" column** to the RIGHT side of the existing progress table. Do not reorder existing columns.

14. **Match ETAs to progress rows by PID.** The progress page already renders rows from the `/activity/progress` endpoint. Each row has a `pid` field. The ETA data from `/forecast/eta` also has `pid`. Join them client-side.

15. **If no ETA data exists for a PID** (operation not yet tracked, or forecast disabled), show `"—"` in the ETA column.

#### InstanceDashboard Modification

16. **Position NeedForecastCard** after the alerts summary area. Do not break existing layout.

17. **Conditional rendering:** Only show NeedForecastCard if forecast data is available (API returns data, not a 501 error). Use the hook's error state to detect disabled subsystem.

#### General

18. **Follow existing Tailwind CSS patterns.** Match the color palette, spacing, and component structure used in existing pages like AlertDetailPanel, AdvisorRow, etc.

19. **Responsive:** All new components must work at mobile widths (min 320px). Tables should horizontally scroll on narrow screens.

20. **No new router pages.** M15_01 only extends existing pages. The full Forecast Dashboard page is M15_02.

21. **No new sidebar navigation items.** The forecast data is accessed via existing instance pages. Sidebar changes are M15_02.

---

## Dependency Order

```
Phase 1 (parallel start):
  Backend: migration 019 + types.go + store.go + pgstore.go + nullstore.go
  Backend: internal/ml/linear.go + wma.go + tests
  Frontend: useForecast.ts hooks (can stub API responses initially)
  Frontend: ETABadge.tsx + ETAConfidenceIndicator.tsx (pure display, no API needed)

Phase 2 (Backend-internal dependencies):
  Backend: tracker.go (depends on types, store, ml/wma)
  Backend: threshold.go (depends on connprovider, config)
  Backend: evaluator.go (depends on types, store, ml/linear, threshold)
  Backend: eta.go (depends on types, tracker, ml/wma)
  Backend: engine.go (depends on tracker, evaluator, eta)

Phase 3 (API layer, depends on Phase 2):
  Backend: forecast_maint.go handlers (depends on engine)
  Backend: server.go route registration
  Backend: connprovider.go ConnForDB extension
  Backend: config.go + load.go additions
  Backend: main.go wiring

Phase 4 (Frontend integration, depends on Phase 3):
  Frontend: NeedForecastCard.tsx + NeedForecastTable.tsx
  Frontend: OperationHistoryTable.tsx
  Frontend: ProgressPage.tsx modification (ETA column)
  Frontend: InstanceDashboard.tsx modification (NeedForecastCard)

Phase 5 (testing, parallel):
  Backend: all _test.go files (can write stubs in Phase 1, fill in Phase 2–3)
  Frontend: verify rendering with live API
```

---

## Build Verification

Both agents must verify their work compiles and passes before committing:

```bash
# Backend verification
go build ./cmd/pgpulse-server
go test ./cmd/... ./internal/... -count=1
golangci-lint run ./cmd/... ./internal/...

# Frontend verification
cd web && npm run build && npm run lint && npm run typecheck && cd ..

# Full verification (before final merge)
cd web && npm run build && npm run lint && npm run typecheck && cd .. && go build ./cmd/pgpulse-server && go test ./cmd/... ./internal/... -count=1 && golangci-lint run ./cmd/... ./internal/...
```

---

## Expected File Watch List

After both agents complete, these files MUST exist:

```
# New Go files (21)
internal/forecast/types.go
internal/forecast/store.go
internal/forecast/pgstore.go
internal/forecast/pgstore_test.go
internal/forecast/nullstore.go
internal/forecast/engine.go
internal/forecast/tracker.go
internal/forecast/tracker_test.go
internal/forecast/eta.go
internal/forecast/eta_test.go
internal/forecast/evaluator.go
internal/forecast/evaluator_test.go
internal/forecast/threshold.go
internal/forecast/threshold_test.go
internal/ml/linear.go
internal/ml/linear_test.go
internal/ml/wma.go
internal/ml/wma_test.go
internal/api/forecast_maint.go
internal/api/forecast_maint_test.go
migrations/019_maintenance_forecasting.sql

# New TypeScript files (6)
web/src/hooks/useForecast.ts
web/src/components/forecast/ETABadge.tsx
web/src/components/forecast/ETAConfidenceIndicator.tsx
web/src/components/forecast/NeedForecastCard.tsx
web/src/components/forecast/NeedForecastTable.tsx
web/src/components/forecast/OperationHistoryTable.tsx

# Modified files (7)
internal/config/config.go
internal/config/load.go
internal/api/connprovider.go
internal/api/server.go
cmd/pgpulse-server/main.go
web/src/pages/ProgressPage.tsx
web/src/pages/InstanceDashboard.tsx
```

---

## What NOT to Build (M15_02 scope)

Do not implement ANY of the following. They are explicitly deferred to M15_02:

- Window Feasibility engine (duration prediction model)
- Maintenance Calendar / Planner UI
- Fleet-wide Forecast Dashboard page
- Forecast → Adviser integration (`EvaluateHook()` with `source: "forecast"`)
- Forecast → Playbook integration (maintenance playbooks via Resolver)
- Forecast-based alert rules
- Sidebar navigation for forecasts
- New router pages
- Operation history charts/graphs
- Duration prediction from `maintenance_operations` history

--- CORRECTIONS (from pre-flight analysis, M15_01_corrections.md) ---

--- CORRECTIONS (from pre-flight grep, M15_01_corrections.md) ---

C1: ConnForDB ALREADY EXISTS in connprovider.go. DO NOT re-add it.
    DO NOT modify connprovider.go. Remove from modified files list.

C2: ml.Detector has no GetBaselineStats. Baselines are internal.
    [IF OPTION A]: Add GetBaselineTrend(instanceID, metricKey) to
    ml/detector.go (~15 lines). Add detector.go to modified files.
    [IF OPTION B]: Leave baselineProvider=nil in main.go. Threshold
    projection only. Defer ML enhancement to M15_02.

C3: No ActiveInstanceIDs on orchestrator. Build instanceStoreAdapter
    that wraps storage.InstanceStore.List() and filters to Enabled.
    Pass adapter as InstanceLister in main.go.

C4: No pg.wal.bytes_rate metric exists. Basebackup forecasting stays
    disabled (basebackup_interval=0s). evaluateBasebackup() returns
    immediately when interval is 0. Document as known limitation.

C5: Basebackup progress has NO datname/relname labels. Set Database=""
    and Table="" for basebackup operations. Handle defensively.

C6: ForecastConfig TYPE NAME already exists (M8_04, ml.forecast.*).
    RENAME to MaintenanceForecastConfig everywhere:
    - Struct: MaintenanceForecastConfig
    - Config field: cfg.MaintenanceForecast
    - YAML section: maintenance_forecast:
    - koanf tag: koanf:"maintenance_forecast"

C7: Config defaults are YAML/zero-value based. Add ApplyDefaults()
    method to MaintenanceForecastConfig. Call in main.go after load.

C8: Per-operation work unit mapping (see corrections doc table):
    vacuum=heap_blks_vacuumed, analyze=sample_blks_scanned,
    create_index=blocks_done, basebackup=backup_streamed.
    Use completion_pct for PctDone universally.

C9: ConnFor/ConnForDB return *pgx.Conn NOT *pgxpool.Conn.
    Close with conn.Close(ctx) NOT conn.Release().