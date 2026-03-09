# M8_05 Requirements — Forecast Alerts + Forecast Chart

**Iteration:** M8_05
**Date:** 2026-03-09
**Depends on:** M8_04 (STLBaseline.Forecast, Detector.Forecast, REST endpoint)

---

## Goal

Wire forecast-threshold alerting into the evaluation loop and add a
forecast confidence-band overlay to metric charts in the frontend.
No new backend endpoints. No schema changes beyond one column addition.

---

## Scope

| # | Deliverable | Owner |
|---|-------------|-------|
| 1 | `ForecastProvider` interface in `internal/alert` | API & Security Agent |
| 2 | Adapter method `ForecastForAlert` on `ml.Detector` | Collector Agent |
| 3 | `runForecastAlerts()` wired into `Evaluator.Evaluate()` | API & Security Agent |
| 4 | `Rule.ConsecutivePointsRequired` field + migration | API & Security Agent |
| 5 | `MLConfig.ForecastAlertMinConsecutive` config default | Collector Agent |
| 6 | `useForecast` hook | Frontend Agent |
| 7 | `buildForecastSeries` ECharts helper | Frontend Agent |
| 8 | Forecast overlay integrated into ServerDetail metric charts | Frontend Agent |
| 9 | Tests for all of the above | QA Agent |

**Out of scope for M8_05:**
- Session kill UI (deferred to M8_06)
- Settings diff UI (deferred to M8_06)
- Query plan viewer UI (deferred to M8_06)
- Any new REST endpoints

---

## Functional Requirements

### FR-1: ForecastProvider Interface

A new interface `ForecastProvider` must be defined in `internal/alert/forecast.go`.
It must not import `internal/ml`. It carries a thin `ForecastPoint` mirror struct
with only the fields that `runForecastAlerts()` needs: `Offset`, `Value`, `Lower`, `Upper`.

### FR-2: Adapter on ml.Detector

`internal/ml/detector_alert.go` must add method `ForecastForAlert` that satisfies
`alert.ForecastProvider`. It converts `ml.ForecastResult.Points` to `[]alert.ForecastPoint`.
`ErrNotBootstrapped` and `ErrNoBaseline` must be passed through unchanged so callers can
distinguish them from real errors.

### FR-3: Evaluator Injection

`Evaluator` must gain a `SetForecastProvider(fp ForecastProvider, minConsecutive int)` setter.
When `fp` is nil, `runForecastAlerts()` returns immediately — no panic, no log noise.
`main.go` calls `SetForecastProvider` after both `Evaluator` and `ml.Detector` are constructed.

### FR-4: runForecastAlerts() Behaviour

Called from `Evaluate()` after existing threshold checks. For each rule where
`rule.Type == RuleTypeForecastThreshold`:

1. Determine required consecutive crossings:
   `n = rule.ConsecutivePointsRequired` if > 0, else `e.forecastMinConsecutive`.
2. Call `e.forecastProvider.ForecastForAlert(ctx, instanceID, rule.MetricKey, n+4)`.
   The horizon buffer (+4) ensures enough points to detect a sustained crossing
   even if the first point is near a boundary.
3. If error is `ErrNotBootstrapped` or `ErrNoBaseline` → skip silently.
4. Any other error → log at WARN level, skip rule, continue.
5. Scan points in order. For each point:
   - If `rule.UseLowerBound == true`, use `point.Lower`; otherwise use `point.Value`.
   - A crossing is: value compared to `rule.Threshold` in `rule.Direction` (above/below).
   - Increment consecutive counter on crossing; reset to 0 otherwise.
   - On reaching `n` consecutive crossings: fire alert via existing `AlertEvaluator`,
     mark cooldown, break inner loop.
6. Cooldown key must be distinct from standard threshold alerts to avoid cross-suppression.
   Use key format: `forecast:{instanceID}:{metricKey}:{ruleID}`.

### FR-5: Rule Schema Addition

`Rule.ConsecutivePointsRequired int` added to the Rule struct and persisted.
Migration `ALTER TABLE alert_rules ADD COLUMN consecutive_points_required INT NOT NULL DEFAULT 0`.
Zero value means "use global default" — this is the correct semantic, not an error.

### FR-6: Config Addition

`MLConfig.ForecastAlertMinConsecutive int` added to `internal/config/config.go`.
Default value: 3.
Wired into `DetectorConfig` and passed to `SetForecastProvider`.
`configs/pgpulse.example.yml` updated with `ml.forecast_alert_min_consecutive: 3`.

### FR-7: useForecast Hook

`web/src/hooks/useForecast.ts` fetches from
`GET /api/v1/instances/{id}/metrics/{metric}/forecast?horizon=N`.
Returns `ForecastResult | null`. Null on any error, including 404 and not-bootstrapped responses.
Polling interval: 5 minutes via `setInterval`. Abort controller cleanup on unmount.
`horizon` parameter: use `cfg.ML.ForecastHorizon` from app config, or hardcode 60 as default.

### FR-8: buildForecastSeries Helper

`web/src/components/ForecastBand.ts` exports a pure function:
```typescript
buildForecastSeries(points: ForecastPoint[], nowMs: number): EChartsOption['series']
```
Returns three series:
1. A `custom` series rendering a closed polygon (confidence band) between `lower` and `upper`
   using `api.coord()` for coordinate transforms. Fill colour: `rgba(99,102,241,0.15)`.
   `z: 1`, `silent: true` (no tooltip on the band itself).
2. A `line` series for the forecast centre (`value`). Dashed, colour `#6366f1`, `symbol: 'none'`, `z: 2`.
3. The "now" boundary `markLine` must be placed on the **last historical data series**
   (handled in ServerDetail, not in this helper).

### FR-9: ServerDetail Integration

For each metric chart that has forecast data:
- Call `useForecast(instanceId, metric, 60)`.
- If result is null, render chart normally — no empty state, no error message.
- If result is non-null, call `buildForecastSeries(result.points, Date.now())` and
  append the returned series to the chart's `option.series` array.
- Extend the X-axis `max` to cover the last forecast point's `predicted_at` timestamp.
- Add a `markLine` at `nowMs` on the last historical series: vertical dashed line,
  colour `#94a3b8`, label `"now"`.

---

## Non-Functional Requirements

- No circular imports. Build must pass with `go build ./cmd/pgpulse-server`.
- Forecast alert evaluation must not block the main evaluation loop.
  `runForecastAlerts` is called synchronously but returns quickly on nil provider.
- Frontend: forecast overlay must not cause chart re-render when forecast data has not changed.
  Memoize `buildForecastSeries` output with `useMemo`.
- Dark-mode safe: custom polygon fill uses rgba, not a named colour that inverts.

---

## Acceptance Criteria

| # | Criterion |
|---|-----------|
| AC-1 | `go build ./cmd/pgpulse-server` passes with no import cycle errors |
| AC-2 | `runForecastAlerts` is called from `Evaluate()` and does not panic when `forecastProvider` is nil |
| AC-3 | A `forecast_threshold` rule with `consecutive_points_required=3` does not fire on 2 consecutive crossings |
| AC-4 | A `forecast_threshold` rule fires and respects cooldown on 3 consecutive crossings |
| AC-5 | `ErrNotBootstrapped` and `ErrNoBaseline` are silently skipped — no log output at ERROR level |
| AC-6 | Migration runs cleanly on existing schema |
| AC-7 | Frontend chart renders without errors when forecast endpoint returns 404 |
| AC-8 | Confidence band renders correctly in dark mode (no white box artefact) |
| AC-9 | `npm run build && npm run typecheck && npm run lint` pass with no new errors |
| AC-10 | `go test ./cmd/... ./internal/...` passes |
