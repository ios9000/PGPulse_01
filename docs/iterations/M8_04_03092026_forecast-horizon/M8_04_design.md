# M8_04 Design — Forecast Horizon

**Iteration:** M8_04  
**Date:** 2026-03-09

---

## Design Decisions (Resolved)

| # | Question | Decision | Rationale |
|---|----------|----------|-----------|
| D1 | Horizon: fixed or per-metric? | Global default + per-metric override in config | Different metrics need different horizons. Connections = 60 pts useful. Bloat = maybe 720. One size doesn't fit all. |
| D2 | Point forecast or CI? | Point + upper/lower bounds (±1.96σ) | Bounds unlock smarter alerting (fire only when high-confidence) and proper shaded UI charts. Marginal extra cost. |
| D3 | Which metrics get forecast? | All ML-enabled metrics automatically | Once STL is fitted the forecast is arithmetic. No reason to maintain a separate list. |
| D4 | Alert threshold format? | Absolute value | `connections > 180` is what operators reason about. Rate-of-change is harder to tune and explain. |

---

## New Types

### internal/ml/forecast.go (NEW)

```go
package ml

import "time"

// ForecastPoint is one predicted future sample.
type ForecastPoint struct {
    Offset      int       // steps ahead (1-based)
    PredictedAt time.Time // wall time of the prediction
    Value       float64   // point forecast
    Lower       float64   // lower confidence bound
    Upper       float64   // upper confidence bound
}

// ForecastResult is the full output of a Forecast call.
type ForecastResult struct {
    InstanceID                string
    MetricKey                 string
    GeneratedAt               time.Time
    CollectionIntervalSeconds int
    Horizon                   int
    ConfidenceZ               float64
    Points                    []ForecastPoint
}
```

### internal/ml/baseline.go (MODIFIED — add Forecast method)

```go
// Forecast computes n-step-ahead predictions from the current fitted state.
// z is the confidence multiplier (1.96 = 95%).
// interval is the collection interval, used to compute wall times.
// now is the timestamp of the last observed point.
// Returns nil if the baseline is not yet warm (TotalSeen < period).
func (b *STLBaseline) Forecast(n int, z float64, interval time.Duration, now time.Time) []ForecastPoint
```

Implementation sketch:
```go
func (b *STLBaseline) Forecast(n int, z float64, interval time.Duration, now time.Time) []ForecastPoint {
    if b.totalSeen < b.period || b.period == 0 {
        return nil
    }
    slope := 0.0
    if len(b.trendHistory) >= 2 {
        slope = b.trendHistory[len(b.trendHistory)-1] - b.trendHistory[len(b.trendHistory)-2]
    }
    lastTrend := b.ewma  // current trend level
    stddev := residualStddev(b.residuals)
    margin := z * stddev

    points := make([]ForecastPoint, n)
    for k := 1; k <= n; k++ {
        trendContrib := lastTrend + slope*float64(k)
        seasonIdx := (b.seasonIdx + k) % b.period
        seasonContrib := b.seasonal[seasonIdx]
        val := trendContrib + seasonContrib
        points[k-1] = ForecastPoint{
            Offset:      k,
            PredictedAt: now.Add(time.Duration(k) * interval),
            Value:       val,
            Lower:       val - margin,
            Upper:       val + margin,
        }
    }
    return points
}

func residualStddev(residuals []float64) float64 {
    // sample std dev of residuals slice
}
```

Note: `STLBaseline` will need to track `trendHistory` (last 2 trend values) and
`seasonIdx` (current position in seasonal cycle). Verify which of these already
exist in the struct from M8_01/M8_02 and add only what is missing.

---

## Config Changes

### internal/config/config.go (MODIFIED)

```go
type ForecastConfig struct {
    Horizon     int     `yaml:"horizon"`      // default 60
    ConfidenceZ float64 `yaml:"confidence_z"` // default 1.96
}

type MLConfig struct {
    // existing fields ...
    Forecast ForecastConfig `yaml:"forecast"`
}

type MLMetricConfig struct {
    // existing fields ...
    ForecastHorizon int `yaml:"forecast_horizon"` // 0 = use global default
}
```

### configs/pgpulse.example.yml (MODIFIED)

```yaml
ml:
  forecast:
    horizon: 60          # points ahead (at 60s collection = 1 hour)
    confidence_z: 1.96   # 95% confidence interval
  metrics:
    connections:
      enabled: true
      forecast_horizon: 60    # can override per-metric
    cache_hit_ratio:
      enabled: true
      # forecast_horizon not set — uses global default
```

---

## Detector Changes

### internal/ml/detector.go (MODIFIED)

New exported method:
```go
// Forecast computes a forecast for the given instance and metric.
// Returns ErrNotBootstrapped if the detector has not completed Bootstrap.
// Returns ErrNoBaseline if no fitted baseline exists for this metric on this instance.
func (d *Detector) Forecast(ctx context.Context, instanceID, metricKey string, horizon int) (*ForecastResult, error)
```

Internal: `Evaluate()` gains a `runForecastAlerts()` private method called at
the end of each cycle. It iterates all fitted baselines and checks
`forecast_threshold` rules.

---

## Alert Rule Changes

### internal/alert/rules.go (MODIFIED)

New rule type constant:
```go
const RuleTypeForecastThreshold = "forecast_threshold"
```

New fields on `Rule` struct (additive — existing rules unaffected):
```go
type Rule struct {
    // existing fields ...
    UseLowerBound bool `yaml:"use_lower_bound"` // only for forecast_threshold
}
```

Evaluation in `Detector.runForecastAlerts()`:
```go
for _, rule := range forecastRules {
    points := baseline.Forecast(horizon, z, interval, now)
    for _, pt := range points {
        checkVal := pt.Value
        if rule.UseLowerBound {
            checkVal = pt.Lower
        }
        if crosses(checkVal, rule.Condition, rule.Threshold) {
            fireAlert(rule, pt, instanceID, metricKey)
            break // first crossing only
        }
    }
}
```

---

## REST API

### internal/api/forecast.go (NEW)

Route: `GET /api/v1/instances/{id}/metrics/{metric}/forecast`

Handler flow:
1. Parse `{id}` and `{metric}` from URL.
2. Parse optional `horizon` query param; validate ≤ cap; default to config value.
3. Call `d.Forecast(ctx, id, metric, horizon)`.
4. Map `*ForecastResult` to JSON response struct.
5. Return 200. Return 404 if `ErrNoBaseline`. Return 503 if `ErrNotBootstrapped`.

```go
type forecastResponse struct {
    InstanceID                string          `json:"instance_id"`
    Metric                    string          `json:"metric"`
    GeneratedAt               time.Time       `json:"generated_at"`
    CollectionIntervalSeconds int             `json:"collection_interval_seconds"`
    Horizon                   int             `json:"horizon"`
    ConfidenceZ               float64         `json:"confidence_z"`
    Points                    []forecastPoint `json:"points"`
}

type forecastPoint struct {
    Offset      int       `json:"offset"`
    PredictedAt time.Time `json:"predicted_at"`
    Value       float64   `json:"value"`
    Lower       float64   `json:"lower"`
    Upper       float64   `json:"upper"`
}
```

### internal/api/server.go (MODIFIED)

Register under viewer group (read-only, JWT required):
```go
r.Get("/instances/{id}/metrics/{metric}/forecast", h.GetMetricForecast)
```

---

## Error Sentinel Values

### internal/ml/errors.go (NEW or appended)

```go
var (
    ErrNotBootstrapped = errors.New("ml detector not yet bootstrapped")
    ErrNoBaseline      = errors.New("no fitted baseline for this metric")
)
```

---

## Files to Create / Modify

| File | Action | Owner |
|------|--------|-------|
| `internal/ml/forecast.go` | NEW | ML Agent |
| `internal/ml/baseline.go` | MODIFIED: add `Forecast()`, `trendHistory`, `seasonIdx` | ML Agent |
| `internal/ml/detector.go` | MODIFIED: `Forecast()` method, `runForecastAlerts()` | ML Agent |
| `internal/ml/errors.go` | NEW (or append) | ML Agent |
| `internal/alert/rules.go` | MODIFIED: new type + `UseLowerBound` | ML Agent |
| `internal/config/config.go` | MODIFIED: `ForecastConfig`, `MLMetricConfig.ForecastHorizon` | API Agent |
| `configs/pgpulse.example.yml` | MODIFIED | API Agent |
| `internal/api/forecast.go` | NEW | API Agent |
| `internal/api/server.go` | MODIFIED: register forecast route | API Agent |
| `internal/ml/forecast_test.go` | NEW | QA Agent |
| `internal/ml/baseline_forecast_test.go` | NEW | QA Agent |
| `internal/api/forecast_test.go` | NEW | QA Agent |

---

## Test Plan

### Unit Tests (internal/ml/forecast_test.go)

1. `TestForecast_SyntheticSeasonal` — build a synthetic `STLBaseline` with known
   trend and seasonal values; assert forecast values match expected arithmetic.
2. `TestForecast_NilBeforeWarm` — baseline with `TotalSeen < period` returns nil.
3. `TestForecast_ConfidenceBounds` — lower < value < upper for all points when
   stddev > 0; lower == upper == value when residuals are all zero.
4. `TestForecast_SeasonalWrap` — forecast period > 1 wraps seasonal index correctly.

### Unit Tests (alert evaluation)

5. `TestForecastAlert_Fires` — forecast crosses threshold → alert fired.
6. `TestForecastAlert_NoFire` — forecast stays below threshold → no alert.
7. `TestForecastAlert_UseLowerBound` — `use_lower_bound: true` delays firing until
   high-confidence crossing.

### API Tests (internal/api/forecast_test.go)

8. `TestGetMetricForecast_OK` — mock detector returns ForecastResult → 200 JSON.
9. `TestGetMetricForecast_NoBaseline` — ErrNoBaseline → 404.
10. `TestGetMetricForecast_NotBootstrapped` — ErrNotBootstrapped → 503.
11. `TestGetMetricForecast_HorizonCap` — horizon > cap → capped, not rejected.
12. `TestGetMetricForecast_ViewerAllowed` — viewer JWT → 200.
13. `TestGetMetricForecast_Unauthenticated` — no token → 401.

---

## Dependency Order for Agents

```
ML Agent (forecast.go, baseline.go, detector.go, errors.go, rules.go)
    ↓ defines ForecastResult, Detector.Forecast(), sentinel errors
API Agent (config.go, api/forecast.go, server.go)
    ↓ depends on Detector.Forecast() and ForecastResult types
QA Agent (all *_test.go)
    ↓ can start unit test stubs immediately, fill API tests once above land
```
