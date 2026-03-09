# Session: 2026-03-09 — M8_05 Forecast Alerts + Forecast Chart

## Goal
Wire `runForecastAlerts()` into the evaluation loop and add a forecast
confidence-band overlay to metric charts in the frontend.

## Agent Team Configuration
- Team Lead: Opus 4.6
- Specialists: Collector, API & Security, Frontend, QA & Review
- All passes: go build, go test (13 new), golangci-lint, npm build + typecheck

## Architecture Decisions Made This Session

| Decision | Rationale |
|----------|-----------|
| `internal/mlerrors` package for sentinel errors | Breaks `alert ↔ ml` circular import without a third abstraction layer |
| Interface injection (`ForecastProvider`) on `Evaluator` | `internal/alert` never imports `internal/ml`; wired in `main.go` |
| Sustained crossing (N consecutive points) for forecast alerts | Noise-resistant; configurable per rule, global default = 3 |
| ECharts custom polygon for confidence band | Dark-mode safe, raw API values in series data, no stack-trick delta pre-computation |
| "Now" markLine on historical series, not forecast series | Correct X position even when forecast starts 1 interval after last historical point |
| Forecast polling interval: 5 minutes | Forecasts update on each collector cycle (60s), but chart refresh can be slow |

## New Files
- `internal/mlerrors/errors.go`
- `internal/alert/forecast.go` — ForecastProvider interface + ForecastPoint mirror
- `internal/ml/detector_alert.go` — ForecastForAlert adapter
- `internal/storage/migrations/011_forecast_alert_consecutive.sql`
- `web/src/hooks/useForecast.ts`
- `web/src/components/ForecastBand.ts`
- `internal/alert/evaluator_forecast_test.go` (9 tests)
- `internal/ml/detector_alert_test.go` (4 tests)

## Modified Files
- `internal/ml/errors.go` — re-exports from mlerrors
- `internal/alert/alert.go` — Rule.ConsecutivePointsRequired
- `internal/alert/evaluator.go` — SetForecastProvider, runForecastAlerts, cooldown tracking
- `internal/config/config.go` + `load.go` — AlertMinConsecutive (default 3)
- `configs/pgpulse.example.yml` — alert_min_consecutive: 3
- `cmd/pgpulse-server/main.go` — SetForecastProvider wiring
- `web/src/components/charts/TimeSeriesChart.tsx` — extraSeries, xAxisMax, nowMarkLine props
- `web/src/pages/ServerDetail.tsx` — useForecast + buildForecastSeries on connections chart

## Known Issues Carried Forward
- `npm run lint` has 1 pre-existing error in `Administration.tsx` — not from M8_05
- Session kill UI — deferred to M8_06
- Settings diff UI — deferred to M8_06
- Query plan viewer UI — deferred to M8_06
- Forecast overlay only wired to `connections_active` chart in ServerDetail — other
  metric charts can be added in M8_06 or as a fast-follow

## Test Results
- 13 new tests, all pass
- No regressions against prior test suite
- `internal/alert` confirmed to not import `internal/ml`
