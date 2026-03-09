# M8_04 Requirements — Forecast Horizon

**Iteration:** M8_04  
**Date:** 2026-03-09  
**Milestone:** M8 — ML Phase 1  
**Depends on:** M8_03 (STLBaseline fitted, PersistenceStore in place)

---

## Goal

Add short-horizon forecasting to the ML subsystem. Given a fully fitted
`STLBaseline` for any metric, produce N-step-ahead predictions with 95%
confidence bounds. Expose forecasts via a REST API and wire alert evaluation
so an alert fires when any forecast point within the horizon is predicted to
cross a configured threshold.

---

## Functional Requirements

### FR-1 — Forecast Computation

- Given a fitted `STLBaseline`, compute `N` future point predictions.
- Method: linear trend extrapolation + repeating seasonal component.
  ```
  forecast[t+k] = trend[t] + slope * k + seasonal[(t+k) mod period]
  ```
  where `slope = trend[t] - trend[t-1]` (last observed trend delta).
- Each forecast point includes a 95% confidence interval:
  ```
  lower = forecast[t+k] - 1.96 * residual_stddev
  upper = forecast[t+k] + 1.96 * residual_stddev
  ```
  `residual_stddev` is derived from the rolling residuals stored in
  `BaselineSnapshot.Residuals`.
- Forecast is computed on-demand (not stored). No new table required.

### FR-2 — Horizon Configuration

- Global default `ml.forecast.horizon` in YAML config (default: `60` points).
- Per-metric override: `ml.metrics[name].forecast_horizon` (optional).
  If absent, global default applies.
- Confidence Z-score configurable: `ml.forecast.confidence_z` (default: `1.96`).
- All three config keys are optional (system works with defaults if absent).

### FR-3 — REST API

New endpoint (JWT-required, viewer role sufficient — read-only):

```
GET /api/v1/instances/{id}/metrics/{metric}/forecast
```

Query parameters:
- `horizon` (optional integer) — overrides configured horizon for this request.
  Capped at `max(2 * configured_horizon, 240)` to prevent abuse.

Response (200 OK):
```json
{
  "instance_id": "abc",
  "metric": "connections",
  "generated_at": "2026-03-09T12:00:00Z",
  "collection_interval_seconds": 60,
  "horizon": 60,
  "confidence_z": 1.96,
  "points": [
    {
      "offset": 1,
      "predicted_at": "2026-03-09T12:01:00Z",
      "value": 42.3,
      "lower": 38.1,
      "upper": 46.5
    }
  ]
}
```

Error cases:
- `404` — instance not found or metric has no fitted baseline yet.
- `400` — invalid `horizon` parameter.
- `503` — detector not yet bootstrapped.

### FR-4 — Forecast-Based Alert Rules

New alert rule type `forecast_threshold`:

```yaml
alerts:
  rules:
    - name: connections_forecast_critical
      type: forecast_threshold
      metric: connections
      condition: ">"
      threshold: 180.0
      use_lower_bound: false   # true = fire only when lower CI bound crosses (high confidence)
      enabled: true
```

Evaluation logic (called from `Detector.Evaluate` after each collect cycle):
1. Compute forecast for the metric.
2. If any point in `[1..horizon]` crosses the threshold → fire alert.
3. Alert payload includes: first crossing offset, predicted value, bounds,
   predicted timestamp.
4. Alert subject: `"FORECAST {metric} predicted to exceed {threshold} in {offset} points ({duration})"`
5. Use existing `AlertEvaluator` interface — no changes to alert dispatch.
6. Cooldown: re-uses existing alert cooldown mechanism; same metric+rule does
   not re-fire within the configured cooldown window.

### FR-5 — Detector Integration

- `Detector.Evaluate()` runs forecast alert checks after standard threshold checks.
- Only runs if `PersistenceStore` is non-nil (baseline must be fitted).
- Only runs for metrics that have a fully bootstrapped baseline
  (`TotalSeen >= period`).
- `Detector` exposes a new method for the API handler to call directly:

```go
func (d *Detector) Forecast(
    ctx context.Context,
    instanceID string,
    metricKey string,
    horizon int,
) (*ForecastResult, error)
```

---

## Non-Functional Requirements

- Forecast computation must complete in < 1ms per metric per call (pure math,
  no DB access).
- API response time target: < 50ms (forecast computed in-memory from fitted state
  loaded from the detector's in-memory cache).
- No new database tables — forecasts are ephemeral.
- No changes to `BaselineSnapshot` schema or `ml_baseline_snapshots` table.

---

## Out of Scope (M8_04)

- UI charting of forecast curves (deferred — no frontend changes).
- Session kill UI (deferred from M8_03).
- Slope / rate-of-change alert rules (deferred).
- Multi-step ahead that accounts for feedback (e.g. ARIMA). STL linear
  extrapolation only.
- Forecast accuracy metrics / backtesting.

---

## Acceptance Criteria

1. `STLBaseline.Forecast(n int, z float64) []ForecastPoint` returns correct
   values verified by unit test with a synthetic seasonal series.
2. `GET /api/v1/instances/{id}/metrics/{metric}/forecast` returns 200 with
   correct JSON shape when baseline exists; 404 when it does not.
3. `forecast_threshold` alert rule fires when a forecast point crosses threshold,
   does not fire when forecast stays below.
4. Config parsing: missing `ml.forecast` block does not crash; defaults apply.
5. `go build ./cmd/pgpulse-server`, `go test ./cmd/... ./internal/...`,
   and `golangci-lint run` all pass clean.
