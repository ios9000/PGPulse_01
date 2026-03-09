# M8_04 Team Prompt — Forecast Horizon

Read `CLAUDE.md` and `docs/iterations/M8_04_03092026_forecast-horizon/design.md`
before starting. All context is there.

---

## Task Summary

Add STL-based short-horizon forecasting to PGPulse. This iteration adds three things:

1. **`STLBaseline.Forecast()`** — pure-math N-step prediction with confidence bounds.
2. **`Detector.Forecast()` API method + REST endpoint** — expose predictions over HTTP.
3. **Forecast-based alert rule** — fire when any forecast point crosses a threshold.

No new database tables. Forecasts are computed in-memory from the already-fitted
`STLBaseline` state that was persisted in M8_03.

---

## Spawn 3 Specialists

---

### ML AGENT

**Owns:** `internal/ml/`, `internal/alert/rules.go`

**Step 1 — Read before writing:**
- Read `internal/ml/baseline.go` in full — understand the current `STLBaseline`
  struct fields, especially: `ewma`, `seasonal`, `period`, `totalSeen`, `residuals`.
  Identify whether `trendHistory []float64` and `seasonIdx int` already exist.
  If not, add them.
- Read `internal/ml/detector.go` in full — understand `Evaluate()` and
  `Bootstrap()` before touching them.
- Read `internal/alert/rules.go` — understand the existing `Rule` struct.

**Step 2 — Create `internal/ml/forecast.go`:**

```go
package ml

import "time"

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
```

**Step 3 — Add `Forecast()` method to `STLBaseline` in `baseline.go`:**

Signature:
```go
func (b *STLBaseline) Forecast(n int, z float64, interval time.Duration, now time.Time) []ForecastPoint
```

Algorithm:
```
trend_slope = trendHistory[last] - trendHistory[last-1]   // 0.0 if < 2 history points
last_trend  = ewma (current trend level)
residual_stddev = sample std dev of b.residuals (0 if len < 2)
margin = z * residual_stddev

for k in 1..n:
    val = last_trend + trend_slope * k + seasonal[(seasonIdx + k) % period]
    lower = val - margin
    upper = val + margin
```

Return `nil` if `totalSeen < period` (not yet warm).

You must track `trendHistory` (last 2 values — append on each EWMA update, keep
only last 2) and `seasonIdx` (current position in cycle — increment by 1 each
update, mod period). Add these fields to the struct and populate them in the
existing update path. Do not change `BaselineSnapshot` or the DB schema.

Also add a private helper:
```go
func residualStddev(residuals []float64) float64
```
Sample std dev (n-1 denominator). Return 0 if len < 2.

**Step 4 — Add sentinel errors to `internal/ml/errors.go` (create if not present):**

```go
var (
    ErrNotBootstrapped = errors.New("ml detector not yet bootstrapped")
    ErrNoBaseline      = errors.New("no fitted baseline for this metric")
)
```

Check whether these already exist in any ml/*.go file before adding.

**Step 5 — Add `Forecast()` method to `Detector` in `detector.go`:**

```go
func (d *Detector) Forecast(
    ctx context.Context,
    instanceID string,
    metricKey string,
    horizon int,
) (*ForecastResult, error)
```

- Return `ErrNotBootstrapped` if detector has not completed `Bootstrap()`.
- Look up the fitted `*STLBaseline` for this `instanceID + metricKey` from
  the detector's in-memory map (the same one `Evaluate()` uses).
- Return `ErrNoBaseline` if not found.
- Call `baseline.Forecast(horizon, z, interval, now)`.
- Wrap into `ForecastResult` and return.
- `z` and `collectionInterval` come from `DetectorConfig`.

**Step 6 — Add `runForecastAlerts()` private method on `Detector`:**

Called at the end of `Evaluate()` after existing threshold checks.

```go
func (d *Detector) runForecastAlerts(ctx context.Context, instanceID string, now time.Time)
```

Logic:
```
for each forecast_threshold rule in d.cfg.AlertRules:
    result = d.Forecast(ctx, instanceID, rule.Metric, horizon)
    if result == nil: continue
    for each pt in result.Points:
        checkVal = pt.Value
        if rule.UseLowerBound: checkVal = pt.Lower
        if crosses(checkVal, rule.Condition, rule.Threshold):
            d.evaluator.Evaluate(ctx, rule.Metric, checkVal, labels_with_forecast_metadata)
            break   // first crossing only per rule per cycle
```

**Step 7 — Modify `internal/alert/rules.go`:**

Add constant:
```go
const RuleTypeForecastThreshold = "forecast_threshold"
```

Add field to `Rule` struct (additive only, existing fields unchanged):
```go
UseLowerBound bool `yaml:"use_lower_bound" json:"use_lower_bound"`
```

---

### API AGENT

**Owns:** `internal/config/config.go`, `configs/pgpulse.example.yml`,
`internal/api/forecast.go` (NEW), `internal/api/server.go`

**Step 1 — Modify `internal/config/config.go`:**

Add to `MLConfig`:
```go
type ForecastConfig struct {
    Horizon     int     `yaml:"horizon"`
    ConfidenceZ float64 `yaml:"confidence_z"`
}
```

Add `Forecast ForecastConfig` field to `MLConfig`.

Add `ForecastHorizon int` to `MLMetricConfig` (`yaml:"forecast_horizon"`).

In the config loading / defaults code, apply:
- `ForecastConfig.Horizon = 60` if zero
- `ForecastConfig.ConfidenceZ = 1.96` if zero

**Step 2 — Create `internal/api/forecast.go`:**

Route: `GET /api/v1/instances/{id}/metrics/{metric}/forecast`

```go
type forecastResponse struct {
    InstanceID                string           `json:"instance_id"`
    Metric                    string           `json:"metric"`
    GeneratedAt               time.Time        `json:"generated_at"`
    CollectionIntervalSeconds int              `json:"collection_interval_seconds"`
    Horizon                   int              `json:"horizon"`
    ConfidenceZ               float64          `json:"confidence_z"`
    Points                    []forecastPointJSON `json:"points"`
}

type forecastPointJSON struct {
    Offset      int       `json:"offset"`
    PredictedAt time.Time `json:"predicted_at"`
    Value       float64   `json:"value"`
    Lower       float64   `json:"lower"`
    Upper       float64   `json:"upper"`
}
```

Handler logic:
1. Parse `{id}` and `{metric}` from URL params.
2. Parse optional `?horizon=N` query param. If present, parse as int.
   Cap at `max(2 * cfg.ML.Forecast.Horizon, 240)`. If `horizon <= 0` → 400.
3. If no `?horizon` param, use metric-specific config override if set, else
   global `cfg.ML.Forecast.Horizon`.
4. Call `d.Forecast(ctx, id, metric, horizon)`.
5. On `ErrNotBootstrapped` → 503 `{"error": "detector not yet bootstrapped"}`.
6. On `ErrNoBaseline` → 404 `{"error": "no baseline for this metric"}`.
7. On success → 200 JSON.

The handler needs access to the `*ml.Detector`. Pass it via the existing
`Handler` struct pattern (same way `AlertEvaluator` or the pool is threaded
through in other handlers). Check `internal/api/server.go` to see how the
Handler struct is constructed and add `Detector *ml.Detector` if not present.

**Step 3 — Modify `internal/api/server.go`:**

Register in the authenticated viewer group:
```go
r.Get("/instances/{id}/metrics/{metric}/forecast", h.GetMetricForecast)
```

Verify the group uses the JWT middleware that allows viewer role.

**Step 4 — Modify `configs/pgpulse.example.yml`:**

Add under the `ml:` key:
```yaml
  forecast:
    horizon: 60          # points ahead; at 60s collection this is 1 hour
    confidence_z: 1.96   # 95% confidence interval
```

And under `ml.metrics` examples, add a `forecast_horizon` comment showing
the per-metric override.

Also add an example `forecast_threshold` alert rule in the alerts section:
```yaml
    - name: connections_forecast_critical
      type: forecast_threshold
      metric: connections
      condition: ">"
      threshold: 180.0
      use_lower_bound: false
      enabled: true
```

---

### QA AGENT

**Owns:** all `*_test.go` files for this iteration

**Start immediately** — write test structure and helper scaffolding while ML
and API agents write production code. Fill in assertions as code lands.

**File 1 — `internal/ml/forecast_test.go`:**

Test `STLBaseline.Forecast()` with a synthetic baseline:

```go
func TestForecast_SyntheticSeasonal(t *testing.T) {
    // Build baseline with known period=4, fixed seasonal=[1,2,3,4],
    // fixed trend=10.0, slope=0.5, zero residuals.
    // Forecast 4 points.
    // Assert point[0].Value == 10.0 + 0.5*1 + seasonal[(seasonIdx+1)%4]
    // Assert lower == upper == value (zero stddev)
}

func TestForecast_NilBeforeWarm(t *testing.T) {
    // Create fresh baseline with period=8, feed only 4 points.
    // Forecast should return nil (not warm).
}

func TestForecast_ConfidenceBounds(t *testing.T) {
    // Baseline with non-zero residuals.
    // Assert lower < value < upper for all points.
}

func TestForecast_SeasonalIndexWrap(t *testing.T) {
    // Feed period+1 points so seasonIdx wraps.
    // Assert forecast seasonal contributions still correct.
}

func TestResidualStddev_Empty(t *testing.T) {
    // residualStddev(nil) == 0, residualStddev([]float64{5.0}) == 0
}

func TestResidualStddev_Known(t *testing.T) {
    // Known values: [2,4,4,4,5,5,7,9] → stddev == 2.0
}
```

**File 2 — `internal/ml/forecast_alert_test.go`:**

```go
func TestForecastAlert_Fires(t *testing.T) {
    // forecast_threshold rule with threshold=50
    // synthetic baseline where forecast crosses 50 at offset 3
    // assert evaluator.Evaluate called exactly once
}

func TestForecastAlert_NoFire(t *testing.T) {
    // forecast stays at ~10, threshold=50
    // assert evaluator.Evaluate never called
}

func TestForecastAlert_UseLowerBound(t *testing.T) {
    // large confidence interval → lower bound well below value
    // value crosses 50 but lower does not
    // with use_lower_bound=true: no fire
    // with use_lower_bound=false: fires
}
```

**File 3 — `internal/api/forecast_test.go`:**

Use `httptest` + a mock `ml.Detector` (interface or struct with stubbed Forecast).

```go
func TestGetMetricForecast_OK(t *testing.T)            // 200, correct JSON shape
func TestGetMetricForecast_NoBaseline(t *testing.T)     // 404
func TestGetMetricForecast_NotBootstrapped(t *testing.T)// 503
func TestGetMetricForecast_HorizonCap(t *testing.T)     // horizon > cap → capped value
func TestGetMetricForecast_InvalidHorizon(t *testing.T) // horizon=0 → 400
func TestGetMetricForecast_ViewerAllowed(t *testing.T)  // viewer JWT → 200
func TestGetMetricForecast_Unauthenticated(t *testing.T)// no token → 401
```

**After all tests pass:**

1. Run `golangci-lint run` — fix any issues.
2. Confirm no `fmt.Sprintf` or string concatenation used in any SQL query
   (scan the new files).
3. Confirm all exported types and functions have doc comments.
4. Report final: test count, pass/fail, lint status.

---

## Coordination Notes

- ML Agent and API Agent can work in parallel on their respective files.
- API Agent depends on `ForecastResult` and `ErrNoBaseline`/`ErrNotBootstrapped`
  types. ML Agent should commit `forecast.go` and `errors.go` first.
- QA Agent should watch for ML Agent's `forecast.go` commit and fill in
  `forecast_test.go` assertions once types are defined.
- Team Lead: merge order is ML Agent → API Agent → QA Agent. Merge only when
  `go test ./cmd/... ./internal/...` passes.

## Build Verification (Team Lead runs after all merges)

```bash
cd web && npm run build && npm run typecheck && npm run lint
cd .. && go build ./cmd/pgpulse-server && go test ./cmd/... ./internal/...
golangci-lint run
```

All must pass clean before the session is complete.
