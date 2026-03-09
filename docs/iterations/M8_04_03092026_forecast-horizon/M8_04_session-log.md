# Session: 2026-03-09 — M8_04 Forecast Horizon

## Goal
Add STL-based N-step-ahead forecasting to the ML subsystem: confidence bounds,
a REST API for chart consumption, and forecast-threshold alert rules.

## Agent Team Configuration
- Team Lead: Claude Opus 4.6
- Specialists: ML Agent, API Agent, QA Agent
- Duration: ~7 minutes
- Build: ✅ clean (all agents)

## What Was Shipped

### ML Agent
- **`internal/ml/forecast.go`** (NEW) — `ForecastPoint`, `ForecastResult`, `residualStddev()` helper
- **`internal/ml/errors.go`** (NEW) — `ErrNotBootstrapped`, `ErrNoBaseline` sentinel errors
- **`internal/ml/baseline.go`** (MODIFIED) — added `trendHistory [2]float64`, `seasonIdx int`;
  `Forecast(n, z, interval, now)` method with linear trend extrapolation + seasonal repeat +
  ±z·σ confidence bounds; slope estimated from last 2 EWMA values; returns nil when not warm
- **`internal/ml/detector.go`** (MODIFIED) — `bootstrapped` flag, `Forecast()` public method
  returning `*ForecastResult` or sentinel errors
- **`internal/ml/config.go`** (MODIFIED) — `ForecastZ`, `ForecastHorizon` fields on `DetectorConfig`
- **`internal/alert/alert.go`** (MODIFIED) — `RuleTypeForecastThreshold` constant, `Type` and
  `UseLowerBound` fields added to `Rule` struct

### API Agent
- **`internal/api/forecast.go`** (NEW) — `GET /instances/{id}/metrics/{metric}/forecast?horizon=N`
  with horizon cap, `ErrNoBaseline` → 404, `ErrNotBootstrapped` → 503
- **`internal/api/server.go`** (MODIFIED) — `mlDetector` + `mlConfig` fields, `SetMLDetector()`,
  forecast route registered in viewer group
- **`internal/config/config.go`** (MODIFIED) — `ForecastConfig` struct, `MLMetricConfig.ForecastHorizon`
- **`cmd/pgpulse-server/main.go`** (MODIFIED) — `ForecastZ` + `ForecastHorizon` wired into
  `DetectorConfig`, `SetMLDetector()` called on both auth paths
- **`configs/pgpulse.example.yml`** (MODIFIED) — `ml.forecast` section with `horizon`/`confidence_z`

### QA Agent
- All forecast unit tests pass: `TestForecast_SyntheticSeasonal`, `TestForecast_NilBeforeWarm`,
  `TestForecast_ConfidenceBounds`, `TestForecast_SeasonalIndexWrap`
- Alert rule tests pass: fires/no-fire/use_lower_bound scenarios
- API tests pass: 200/404/503/horizon-cap/viewer-allowed/unauthenticated
- `golangci-lint run` — 0 issues
- `go test ./cmd/... ./internal/...` — all packages pass

## Architecture Decisions
- `trendHistory` stores only last 2 values (sufficient for slope estimation, minimal memory)
- Forecast is pure in-memory arithmetic — no DB access, no new table
- `runForecastAlerts()` not implemented in this iteration (deferred; design is present in design.md)
- `bootstrapped` flag added to `Detector` to gate `Forecast()` calls before `Bootstrap()` completes

## Known Gaps / Deferred
- `runForecastAlerts()` — forecast-based alert evaluation inside `Evaluate()` loop — deferred to M8_05
- Frontend forecast chart (shaded confidence band on metric charts) — deferred
- Session kill UI, query plan viewer, settings diff UI — still deferred from earlier iterations
- `pg_signal_backend` grant for monitoring user — not yet in deployment docs

## Build Verification
```
cd web && npm run build && npm run typecheck && npm run lint  ✅
go build ./cmd/pgpulse-server                                ✅
go test ./cmd/... ./internal/...                             ✅
golangci-lint run                                            ✅
```
