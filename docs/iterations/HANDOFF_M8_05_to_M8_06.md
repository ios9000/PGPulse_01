# PGPulse — Iteration Handoff: M8_05 → M8_06

---

## DO NOT RE-DISCUSS

- `internal/alert` must NEVER import `internal/ml` — use `alert.ForecastProvider` interface
- `internal/mlerrors` is the canonical home for `ErrNotBootstrapped` and `ErrNoBaseline`;
  `internal/ml/errors.go` re-exports them — do not move the definitions back
- `ForecastPoint` in `internal/alert/forecast.go` is intentionally a thin mirror (4 fields only);
  do not merge it with `ml.ForecastPoint`
- Sustained crossing (N consecutive) is the only supported mode for forecast alerts — no
  first-crossing mode; `ConsecutivePointsRequired = 0` means "use global default (3)", not "first crossing"
- Forecast polling in the frontend is 5 minutes — do not change without a product reason
- `TimeSeriesChart.tsx` now accepts `extraSeries`, `xAxisMax`, `nowMarkLine` props — use these
  for all future forecast integrations, do not add new props for the same purpose
- The pre-existing `Administration.tsx` lint error is not our debt — do not fix it in M8_06
  unless it's in scope

---

## What Was Just Completed (M8_05)

### Forecast Alert Wiring
- `internal/mlerrors/errors.go` — shared sentinel errors, breaks circular import
- `internal/alert/forecast.go` — `ForecastProvider` interface + `ForecastPoint` mirror struct
- `internal/ml/detector_alert.go` — `ForecastForAlert` adapter; `*ml.Detector` satisfies
  `alert.ForecastProvider`
- `internal/alert/evaluator.go` — `SetForecastProvider(fp, minConsecutive)` setter +
  `runForecastAlerts()` called from `Evaluate()` after existing threshold checks
- `internal/alert/alert.go` — `Rule.ConsecutivePointsRequired int` added
- `migrations/011_forecast_alert_consecutive.sql` — column added with `DEFAULT 0`
- `internal/config/config.go` + `load.go` — `ForecastConfig.AlertMinConsecutive int`, default 3
- `cmd/pgpulse-server/main.go` — `evaluator.SetForecastProvider(mlDetector, cfg.ML.Forecast.AlertMinConsecutive)`

### Forecast Chart
- `web/src/hooks/useForecast.ts` — polls `GET /api/v1/instances/{id}/metrics/{metric}/forecast`
  every 5 minutes; returns `ForecastResult | null`
- `web/src/components/ForecastBand.ts` — `buildForecastSeries(points)` returns 2 ECharts series:
  custom polygon (confidence band) + dashed line (centre value);
  `getNowMarkLine(nowMs)` for the "now" divider on the historical series
- `web/src/components/charts/TimeSeriesChart.tsx` — new props: `extraSeries`, `xAxisMax`,
  `nowMarkLine`
- `web/src/pages/ServerDetail.tsx` — forecast wired to `connections_active` chart only

---

## Key Interfaces (actual signatures from committed code)

```go
// internal/mlerrors/errors.go
var ErrNotBootstrapped = errors.New("ml detector not yet bootstrapped")
var ErrNoBaseline      = errors.New("no fitted baseline for this metric")

// internal/alert/forecast.go
type ForecastPoint struct {
    Offset int
    Value  float64
    Lower  float64
    Upper  float64
}

type ForecastProvider interface {
    ForecastForAlert(ctx context.Context, instanceID, metricKey string, horizon int) ([]ForecastPoint, error)
}

// internal/alert/alert.go — Rule (additions only)
ConsecutivePointsRequired int  // 0 = use global default

// internal/alert/evaluator.go
func (e *Evaluator) SetForecastProvider(fp ForecastProvider, minConsecutive int)
// runForecastAlerts called from Evaluate() — not exported
```

```typescript
// web/src/hooks/useForecast.ts
export function useForecast(instanceId: string, metric: string, horizon?: number): ForecastResult | null

// web/src/components/ForecastBand.ts
export function buildForecastSeries(points: ForecastPoint[]): object[]
export function getNowMarkLine(nowMs: number): object

// web/src/components/charts/TimeSeriesChart.tsx — new props
extraSeries?: object[]
xAxisMax?: number
nowMarkLine?: object
```

---

## Known Issues

| Issue | Status |
|-------|--------|
| Session kill UI | Deferred from M8_03 |
| Settings diff UI | Deferred from M8_01/M8_02 |
| Query plan viewer UI | Deferred from M8_01/M8_02 |
| Forecast overlay wired to `connections_active` only | Other charts need the same treatment |
| Pre-existing lint error in `Administration.tsx` | Not our debt |

---

## Next Task: M8_06 — UI Catch-Up

M8_06 clears the three deferred UI items and extends the forecast overlay to
all metric charts. This completes the M8 milestone.

### 1. Session Kill UI
API already complete from M8_03:
- `POST /api/v1/instances/{id}/sessions/{pid}/cancel` — sends pg_cancel_backend
- `POST /api/v1/instances/{id}/sessions/{pid}/terminate` — sends pg_terminate_backend

Frontend work:
- Add Kill / Terminate buttons to activity table rows in ServerDetail
- Confirmation modal (text: "Cancel query for PID {pid}?" / "Terminate session for PID {pid}?")
- On confirm: call the API, show success/error toast, refresh activity list
- Viewer role: buttons disabled or hidden (API returns 403, handle gracefully)
- `pg_signal_backend` grant is already documented as required in deployment notes

### 2. Settings Diff UI
API already complete from M8_01:
- `GET /api/v1/instances/{id}/settings/diff` — returns changed settings vs defaults

Frontend work:
- `SettingsDiff.tsx` component: accordion grouped by `pg_settings.category`
- Each row: setting name, current value, default value, unit, pending_restart badge
- "Export CSV" button (client-side, from the diff data already in memory)
- Add as a tab or section in ServerDetail

### 3. Query Plan Viewer UI
API already complete from M8_02:
- `GET /api/v1/instances/{id}/statements/{queryid}/plan` — returns EXPLAIN JSON

Frontend work:
- `QueryPlanViewer.tsx`: recursive tree rendering from EXPLAIN JSON
- Cost highlighting: nodes with high `actual_time` or high row estimate error highlighted
- Raw JSON toggle (show/hide the raw EXPLAIN output)
- Accessible from the top queries table in ServerDetail

### 4. Forecast Overlay — Remaining Charts
Extend the `useForecast` + `buildForecastSeries` pattern (already working on
`connections_active`) to all metric charts in ServerDetail that have a corresponding
metric key:

| Chart | Metric key |
|-------|------------|
| Cache hit ratio | `cache_hit_ratio` |
| Transactions/sec | `transactions_per_sec` |
| Replication lag | `replication_lag_bytes` |
| Active sessions | `sessions_active` |

Same pattern as `connections_active`: `useForecast` hook, `useMemo` for series,
`xAxisMax` + `nowMarkLine` props on `TimeSeriesChart`.

---

## Workflow Reminder

```bash
cd C:\Users\Archer\Projects\PGPulse_01
claude --model claude-opus-4-6

# Build verification
cd web && npm run build && npm run typecheck && npm run lint
cd .. && go build ./cmd/pgpulse-server && go test ./cmd/... ./internal/...
golangci-lint run

# Commit
git add . && git commit -m "..." && git push
```
