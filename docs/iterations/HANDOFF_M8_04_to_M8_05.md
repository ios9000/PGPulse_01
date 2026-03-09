# PGPulse — Iteration Handoff: M8_04 → M8_05

---

## DO NOT RE-DISCUSS

- `STLBaseline.Forecast()` returns nil when `totalSeen < period` — this is correct, not a bug
- `trendHistory` holds exactly 2 values (ring buffer) — slope needs only last 2
- `Forecast()` on `Detector` is public and used directly by the API handler — no wrapper needed
- Forecast is computed in-memory on demand — no caching, no persistence, no new table
- `ErrNotBootstrapped` and `ErrNoBaseline` are the canonical sentinel errors — do not add new ones
- Forecast-threshold alert rules (`RuleTypeForecastThreshold`) exist in the data model but
  `runForecastAlerts()` was deferred — it is NOT called from `Evaluate()` yet
- `bootstrapped` flag on `Detector` is set to true at end of `Bootstrap()` — always check this before calling `Forecast()`
- Session kill has no UI — API is complete, frontend deferred
- Settings diff and query plan UI — API complete from M8_01/M8_02, frontend deferred
- `.claude/worktrees/` is in `.gitignore` — do not remove

---

## What Was Just Completed (M8_04)

### 1. STLBaseline.Forecast()
`internal/ml/baseline.go` now carries `trendHistory [2]float64` and `seasonIdx int`.
`Forecast(n int, z float64, interval time.Duration, now time.Time) []ForecastPoint`
computes N future points: `val = lastTrend + slope*k + seasonal[(seasonIdx+k)%period]`
with `lower/upper = val ∓ z * residualStddev(residuals)`. Returns nil if not warm.

### 2. Detector.Forecast()
`internal/ml/detector.go` has `bootstrapped bool` flag and:
```go
func (d *Detector) Forecast(ctx context.Context, instanceID, metricKey string, horizon int) (*ForecastResult, error)
```
Returns `ErrNotBootstrapped` or `ErrNoBaseline` as appropriate.

### 3. REST Endpoint
`GET /api/v1/instances/{id}/metrics/{metric}/forecast?horizon=N` (viewer role).
Horizon capped at `max(2 * cfg.ForecastHorizon, 240)`. Returns:
```json
{
  "instance_id": "...", "metric": "...", "generated_at": "...",
  "collection_interval_seconds": 60, "horizon": 60, "confidence_z": 1.96,
  "points": [{"offset": 1, "predicted_at": "...", "value": 42.3, "lower": 38.1, "upper": 46.5}]
}
```

### 4. Config additions
`internal/config/config.go`: `ForecastConfig{Horizon int, ConfidenceZ float64}` on `MLConfig`.
`MLMetricConfig.ForecastHorizon int` for per-metric override.
`internal/ml/config.go`: `ForecastZ float64`, `ForecastHorizon int` on `DetectorConfig`.
`configs/pgpulse.example.yml`: `ml.forecast` block + example `forecast_threshold` alert rule.

### 5. Alert data model
`internal/alert/alert.go`: `RuleTypeForecastThreshold = "forecast_threshold"`, `Rule.Type string`,
`Rule.UseLowerBound bool`. Schema only — evaluation not wired yet.

---

## Key Interfaces (actual signatures from committed code)

```go
// internal/ml/forecast.go
type ForecastPoint struct {
    Offset      int
    PredictedAt time.Time
    Value       float64
    Lower       float64
    Upper       float64
}

type ForecastResult struct {
    InstanceID                string
    MetricKey                 string
    GeneratedAt               time.Time
    CollectionIntervalSeconds int
    Horizon                   int
    ConfidenceZ               float64
    Points                    []ForecastPoint
}

// internal/ml/errors.go
var ErrNotBootstrapped = errors.New("ml detector not yet bootstrapped")
var ErrNoBaseline      = errors.New("no fitted baseline for this metric")

// internal/ml/baseline.go  (Forecast signature)
func (b *STLBaseline) Forecast(n int, z float64, interval time.Duration, now time.Time) []ForecastPoint

// internal/ml/detector.go  (Forecast signature)
func (d *Detector) Forecast(ctx context.Context, instanceID, metricKey string, horizon int) (*ForecastResult, error)
```

---

## Files Added/Modified in M8_04

```
internal/ml/
  forecast.go        ← NEW: ForecastPoint, ForecastResult, residualStddev
  errors.go          ← NEW: ErrNotBootstrapped, ErrNoBaseline
  baseline.go        ← MODIFIED: trendHistory, seasonIdx, Forecast()
  detector.go        ← MODIFIED: bootstrapped flag, Forecast() method
  config.go          ← MODIFIED: ForecastZ, ForecastHorizon on DetectorConfig

internal/alert/
  alert.go           ← MODIFIED: RuleTypeForecastThreshold, Rule.Type, Rule.UseLowerBound

internal/api/
  forecast.go        ← NEW: GetMetricForecast handler
  server.go          ← MODIFIED: mlDetector, mlConfig, SetMLDetector(), forecast route

internal/config/
  config.go          ← MODIFIED: ForecastConfig, MLMetricConfig.ForecastHorizon

cmd/pgpulse-server/
  main.go            ← MODIFIED: ForecastZ/ForecastHorizon wired, SetMLDetector called

configs/
  pgpulse.example.yml ← MODIFIED: ml.forecast block
```

---

## Known Issues

| Issue | Status |
|-------|--------|
| `runForecastAlerts()` not called from `Evaluate()` | Deferred — M8_05 scope |
| Forecast chart UI (shaded confidence band) | Deferred — no frontend changes this iteration |
| Session kill UI | Deferred from M8_03 |
| Settings diff + query plan UI | Deferred from M8_01/M8_02 |
| `pg_signal_backend` grant not in deployment docs | Needs doc update |

---

## Next Task: M8_05

M8_05 has two reasonable options. Discuss and pick one at session start:

### Option A — Wire forecast alerts + frontend catch-up (completes M8)
1. `runForecastAlerts()` — implement and call from `Detector.Evaluate()`.
   Iterate all `forecast_threshold` rules, call `Forecast()`, check each point,
   fire via `AlertEvaluator` on first crossing (with cooldown via existing mechanism).
2. Frontend forecast chart — add shaded confidence band to metric time-series charts
   in ServerDetail (ECharts `areaStyle` on upper/lower series).
3. Session kill UI — kill/terminate buttons with confirmation modal on activity rows
   in ServerDetail.
4. Settings diff UI — SettingsDiff.tsx (accordion by category, CSV export).
5. Query plan viewer — QueryPlanViewer.tsx (recursive tree, cost highlighting, raw JSON toggle).

**Scope is heavy.** Consider splitting: M8_05 = forecast alerts + forecast chart only;
M8_06 = UI catch-up (session kill + settings diff + query plan).

### Option B — Skip to M9 (Reports & Export)
Treat the deferred UI items and `runForecastAlerts()` as fast-follow tech debt,
move to M9 to maintain milestone momentum. Circle back in M9_0x.

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
