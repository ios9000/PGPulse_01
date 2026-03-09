# M8_05 Design — Forecast Alerts + Forecast Chart

**Iteration:** M8_05
**Date:** 2026-03-09

---

## 1. Dependency Graph (Before and After)

### Before M8_05

```
internal/ml  →  internal/alert   (AlertEvaluator interface, Rule types)
internal/alert  (no ml dependency)
```

### After M8_05 (with interface injection)

```
internal/ml  →  internal/alert   (AlertEvaluator, Rule, ForecastPoint — alert defines them)
internal/alert  →  (no ml import — uses ForecastProvider interface only)
main.go wires: evaluator.SetForecastProvider(mlDetector, n)
```

No circular import. `internal/alert` never imports `internal/ml`.

---

## 2. New Files

```
internal/alert/forecast.go          ← ForecastPoint mirror, ForecastProvider interface
internal/ml/detector_alert.go       ← ForecastForAlert adapter method
migrations/0XX_forecast_alert_consecutive.sql
web/src/hooks/useForecast.ts
web/src/components/ForecastBand.ts
```

---

## 3. Modified Files

```
internal/alert/alert.go             ← Rule.ConsecutivePointsRequired int
internal/alert/evaluator.go         ← forecastProvider field, SetForecastProvider, runForecastAlerts
internal/config/config.go           ← MLConfig.ForecastAlertMinConsecutive int
internal/ml/config.go               ← (if ForecastAlertMinConsecutive flows through DetectorConfig)
cmd/pgpulse-server/main.go          ← evaluator.SetForecastProvider(mlDetector, cfg.ML.ForecastAlertMinConsecutive)
configs/pgpulse.example.yml         ← ml.forecast_alert_min_consecutive: 3
web/src/pages/ServerDetail.tsx      ← useForecast hook + buildForecastSeries integration
```

---

## 4. internal/alert/forecast.go (new)

```go
package alert

import "context"

// ForecastPoint is a thin mirror of ml.ForecastPoint containing only the
// fields that runForecastAlerts needs. Defined here to avoid importing
// internal/ml and creating a circular dependency.
type ForecastPoint struct {
    Offset int
    Value  float64
    Lower  float64
    Upper  float64
}

// ForecastProvider is satisfied by *ml.Detector via its ForecastForAlert
// adapter method. Nil provider disables forecast alert evaluation.
type ForecastProvider interface {
    ForecastForAlert(
        ctx context.Context,
        instanceID, metricKey string,
        horizon int,
    ) ([]ForecastPoint, error)
}
```

---

## 5. internal/ml/detector_alert.go (new)

```go
package ml

import (
    "context"
    "github.com/ios9000/PGPulse_01/internal/alert"
)

// ForecastForAlert satisfies alert.ForecastProvider.
// It converts ml.ForecastResult.Points to []alert.ForecastPoint.
// ErrNotBootstrapped and ErrNoBaseline are passed through unchanged.
func (d *Detector) ForecastForAlert(
    ctx context.Context,
    instanceID, metricKey string,
    horizon int,
) ([]alert.ForecastPoint, error) {
    result, err := d.Forecast(ctx, instanceID, metricKey, horizon)
    if err != nil {
        return nil, err // includes ErrNotBootstrapped, ErrNoBaseline
    }
    out := make([]alert.ForecastPoint, len(result.Points))
    for i, p := range result.Points {
        out[i] = alert.ForecastPoint{
            Offset: p.Offset,
            Value:  p.Value,
            Lower:  p.Lower,
            Upper:  p.Upper,
        }
    }
    return out, nil
}
```

---

## 6. internal/alert/alert.go — Rule addition

Add one field to the existing `Rule` struct:

```go
type Rule struct {
    // ... all existing fields unchanged ...
    Type                      string // "threshold" | "forecast_threshold"
    UseLowerBound             bool
    ConsecutivePointsRequired int    // 0 = use MLConfig.ForecastAlertMinConsecutive
}
```

No other changes to this file.

---

## 7. internal/alert/evaluator.go — changes

### New fields on Evaluator

```go
type Evaluator struct {
    // ... existing fields ...
    forecastProvider      ForecastProvider // nil = disabled
    forecastMinConsecutive int
}
```

### New setter

```go
// SetForecastProvider wires in the ML detector for forecast-threshold alert
// evaluation. Must be called before the first Evaluate() call.
// minConsecutive is the global default for rules that specify 0.
func (e *Evaluator) SetForecastProvider(fp ForecastProvider, minConsecutive int) {
    e.forecastProvider = fp
    e.forecastMinConsecutive = minConsecutive
}
```

### Call site in Evaluate()

```go
func (e *Evaluator) Evaluate(ctx context.Context, instanceID string, ...) error {
    // ... existing threshold evaluation ...

    if err := e.runForecastAlerts(ctx, instanceID); err != nil {
        slog.Warn("forecast alert evaluation error", "instance", instanceID, "err", err)
    }
    return nil
}
```

### runForecastAlerts() — full implementation

```go
func (e *Evaluator) runForecastAlerts(ctx context.Context, instanceID string) error {
    if e.forecastProvider == nil {
        return nil
    }

    rules := e.forecastRules() // filter: rule.Type == RuleTypeForecastThreshold
    for _, rule := range rules {
        required := rule.ConsecutivePointsRequired
        if required <= 0 {
            required = e.forecastMinConsecutive
        }

        // Request extra points as buffer for edge cases near period boundaries.
        points, err := e.forecastProvider.ForecastForAlert(
            ctx, instanceID, rule.MetricKey, required+4,
        )
        if err != nil {
            if errors.Is(err, ml.ErrNotBootstrapped) || errors.Is(err, ml.ErrNoBaseline) {
                continue // not an error condition; detector still warming up
            }
            slog.Warn("forecast provider error", "rule", rule.ID, "err", err)
            continue
        }

        cooldownKey := fmt.Sprintf("forecast:%s:%s:%s", instanceID, rule.MetricKey, rule.ID)
        if e.inCooldown(cooldownKey) {
            continue
        }

        consecutive := 0
        for _, pt := range points {
            val := pt.Value
            if rule.UseLowerBound {
                val = pt.Lower
            }
            if crosses(val, rule.Threshold, rule.Direction) {
                consecutive++
                if consecutive >= required {
                    e.fire(ctx, rule, instanceID, val)
                    e.setCooldown(cooldownKey, rule.CooldownDuration)
                    break
                }
            } else {
                consecutive = 0
            }
        }
    }
    return nil
}
```

**Note on `errors.Is` with ml sentinel errors:** The import of `internal/ml` for the
error values would reintroduce the cycle. Resolve by one of:
- Defining `ErrNotBootstrapped` and `ErrNoBaseline` in `internal/alert` (they already express
  alert-domain semantics) and having `internal/ml` import them from there, OR
- Comparing by error string: `err.Error() == "ml detector not yet bootstrapped"` (fragile), OR
- Defining the sentinel errors in a third zero-dependency package `internal/mlerrors`
  that both `internal/ml` and `internal/alert` import.

**Recommended:** Move `ErrNotBootstrapped` and `ErrNoBaseline` to
`internal/mlerrors/errors.go`. Both `internal/ml` and `internal/alert` import that package.
No cycle, no string comparison.

```go
// internal/mlerrors/errors.go
package mlerrors

import "errors"

var ErrNotBootstrapped = errors.New("ml detector not yet bootstrapped")
var ErrNoBaseline      = errors.New("no fitted baseline for this metric")
```

Update `internal/ml/errors.go` to re-export from `mlerrors` (or just replace with imports).

---

## 8. Migration

```sql
-- migrations/0XX_forecast_alert_consecutive.sql
ALTER TABLE alert_rules
    ADD COLUMN IF NOT EXISTS consecutive_points_required INT NOT NULL DEFAULT 0;

COMMENT ON COLUMN alert_rules.consecutive_points_required IS
    'Minimum consecutive forecast points that must cross threshold before alert fires. 0 = use global default.';
```

Migration number: use the next available sequence in `migrations/`. Agent must check existing
files to pick the correct number.

---

## 9. Config additions

### internal/config/config.go

```go
type ForecastConfig struct {
    Horizon     int     `koanf:"horizon"`
    ConfidenceZ float64 `koanf:"confidence_z"`
    AlertMinConsecutive int `koanf:"alert_min_consecutive"` // new
}
```

`AlertMinConsecutive` default: 3. Wired into `Evaluator.SetForecastProvider` from `main.go`.

### configs/pgpulse.example.yml

```yaml
ml:
  forecast:
    horizon: 60
    confidence_z: 1.96
    alert_min_consecutive: 3   # new line
```

### main.go addition

```go
evaluator.SetForecastProvider(mlDetector, cfg.ML.Forecast.AlertMinConsecutive)
```

Called after both `evaluator` and `mlDetector` are constructed, before the HTTP server starts.

---

## 10. web/src/hooks/useForecast.ts (new)

```typescript
import { useState, useEffect } from 'react'
import { apiFetch } from '../api/client'

export interface ForecastPoint {
    offset: number
    predicted_at: string
    value: number
    lower: number
    upper: number
}

export interface ForecastResult {
    instance_id: string
    metric: string
    generated_at: string
    collection_interval_seconds: number
    horizon: int
    confidence_z: number
    points: ForecastPoint[]
}

const FORECAST_POLL_MS = 5 * 60 * 1000 // 5 minutes

export function useForecast(
    instanceId: string,
    metric: string,
    horizon: number = 60,
): ForecastResult | null {
    const [result, setResult] = useState<ForecastResult | null>(null)

    useEffect(() => {
        const controller = new AbortController()

        const fetch = async () => {
            try {
                const data = await apiFetch<ForecastResult>(
                    `/api/v1/instances/${instanceId}/metrics/${encodeURIComponent(metric)}/forecast?horizon=${horizon}`,
                    { signal: controller.signal },
                )
                setResult(data)
            } catch {
                setResult(null) // 404, not bootstrapped, network error — all treated as no data
            }
        }

        fetch()
        const timer = setInterval(fetch, FORECAST_POLL_MS)
        return () => {
            controller.abort()
            clearInterval(timer)
        }
    }, [instanceId, metric, horizon])

    return result
}
```

---

## 11. web/src/components/ForecastBand.ts (new)

```typescript
import type { EChartsOption } from 'echarts'
import type { ForecastPoint } from '../hooks/useForecast'

const BAND_FILL    = 'rgba(99, 102, 241, 0.15)'
const LINE_COLOR   = '#6366f1'
const DIVIDER_COLOR = '#94a3b8'

/**
 * Builds three ECharts series for a forecast overlay:
 *   1. Custom polygon — confidence band between lower and upper bounds
 *   2. Dashed line — forecast centre (value)
 *
 * The "now" markLine should be placed on the last historical series
 * by the caller, not here, to avoid duplicate rendering.
 *
 * @param points  ForecastPoint array from the forecast API
 */
export function buildForecastSeries(
    points: ForecastPoint[],
): EChartsOption['series'] {
    if (!points.length) return []

    const times  = points.map(p => new Date(p.predicted_at).getTime())
    const values = points.map(p => p.value)
    const lower  = points.map(p => p.lower)
    const upper  = points.map(p => p.upper)

    return [
        // 1. Confidence band — custom polygon
        {
            type: 'custom' as const,
            name: 'forecast_band',
            silent: true,
            z: 1,
            renderItem: (_params: unknown, api: {
                value: (idx: number) => number
                coord: (point: [number, number]) => [number, number]
                style: (opts: object) => object
            }) => {
                const idx = (_params as { dataIndex: number }).dataIndex
                const loCoord = api.coord([times[idx], lower[idx]])
                const hiCoord = api.coord([times[idx], upper[idx]])

                // Build polygon on first item; subsequent items extend the path.
                // ECharts calls renderItem once per data point for custom series.
                // Polygon approach: accumulate all coords, draw on last item.
                // Simpler alternative: draw a rectangle per data point (column band).
                const nextIdx = idx + 1
                if (nextIdx >= points.length) return { type: 'group', children: [] }

                const loNext = api.coord([times[nextIdx], lower[nextIdx]])
                const hiNext = api.coord([times[nextIdx], upper[nextIdx]])

                return {
                    type: 'polygon',
                    shape: {
                        points: [loCoord, loNext, hiNext, hiCoord],
                    },
                    style: api.style({ fill: BAND_FILL, stroke: 'none' }),
                }
            },
            // data drives renderItem calls — one entry per segment
            data: times.map((t, i) => [t, lower[i], upper[i]]),
            encode: { x: 0 },
        },

        // 2. Forecast centre line (dashed)
        {
            type: 'line' as const,
            name: 'forecast_value',
            data: times.map((t, i) => [t, values[i]]),
            lineStyle: { type: 'dashed', color: LINE_COLOR, width: 1.5 },
            itemStyle: { color: LINE_COLOR },
            symbol: 'none',
            z: 2,
            tooltip: { show: false },
        },
    ]
}

/**
 * Returns a markLine data entry for the "now" boundary divider.
 * Add this to the existing historical series' markLine.data array:
 *
 *   markLine: { data: [getNowMarkLine()], silent: true }
 */
export function getNowMarkLine(nowMs: number): object {
    return {
        xAxis: nowMs,
        lineStyle: { type: 'dashed', color: DIVIDER_COLOR, width: 1 },
        label: { formatter: 'now', position: 'insideStartTop', color: DIVIDER_COLOR },
    }
}
```

---

## 12. ServerDetail.tsx integration pattern

For each metric chart (example shows connections metric):

```typescript
// At component level:
const forecastConnections = useForecast(instanceId, 'connections_active', 60)

// X-axis max extension:
const xMax = forecastConnections
    ? new Date(forecastConnections.points.at(-1)!.predicted_at).getTime()
    : undefined

// Series assembly:
const forecastSeries = forecastConnections
    ? buildForecastSeries(forecastConnections.points)
    : []

// Final option passed to ECharts:
const option: EChartsOption = {
    xAxis: { type: 'time', max: xMax },
    series: [
        {
            type: 'line',
            name: 'connections_active',
            data: historicalData,
            // Add "now" divider only when forecast data is present:
            ...(forecastConnections ? {
                markLine: {
                    silent: true,
                    data: [getNowMarkLine(Date.now())],
                },
            } : {}),
        },
        ...forecastSeries,
    ],
}
```

`useMemo` wrapping:

```typescript
const forecastSeries = useMemo(
    () => forecastConnections ? buildForecastSeries(forecastConnections.points) : [],
    [forecastConnections],
)
```

---

## 13. Test Coverage

### Backend (QA Agent)

| Test | Location | What it covers |
|------|----------|----------------|
| `TestRunForecastAlerts_NilProvider` | `internal/alert/evaluator_test.go` | Returns nil, no panic |
| `TestRunForecastAlerts_NotBootstrapped` | same | ErrNotBootstrapped silently skipped |
| `TestRunForecastAlerts_NoBaseline` | same | ErrNoBaseline silently skipped |
| `TestRunForecastAlerts_InsufficientCrossings` | same | 2 of 3 required → no fire |
| `TestRunForecastAlerts_SufficientCrossings` | same | 3 of 3 → fires, cooldown set |
| `TestRunForecastAlerts_CooldownRespected` | same | Second call within cooldown → no double-fire |
| `TestRunForecastAlerts_UseLowerBound` | same | Lower bound used when flag set |
| `TestForecastForAlert_Adapter` | `internal/ml/detector_alert_test.go` | Correct struct conversion |
| `TestForecastForAlert_PassthroughErrors` | same | Sentinel errors passed through |

### Frontend (QA Agent)

- `buildForecastSeries` unit tests: empty input returns `[]`, non-empty returns 2 series.
- `useForecast` hook: mock fetch returning 404 → result is null (no throw).
- `useForecast` hook: mock fetch returning valid JSON → result is non-null.

---

## 14. File Watch List (for developer checklist)

```
internal/mlerrors/errors.go                      NEW
internal/alert/forecast.go                       NEW
internal/ml/detector_alert.go                    NEW
internal/alert/alert.go                          MODIFIED
internal/alert/evaluator.go                      MODIFIED
internal/config/config.go                        MODIFIED
cmd/pgpulse-server/main.go                       MODIFIED
configs/pgpulse.example.yml                      MODIFIED
migrations/0XX_forecast_alert_consecutive.sql    NEW (number TBD by agent)
web/src/hooks/useForecast.ts                     NEW
web/src/components/ForecastBand.ts               NEW
web/src/pages/ServerDetail.tsx                   MODIFIED
```
