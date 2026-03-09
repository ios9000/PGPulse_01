# M8_05 Team Prompt — Forecast Alerts + Forecast Chart

Read CLAUDE.md and docs/iterations/M8_05_*/design.md before starting.
This prompt is self-contained; refer to the design doc for full signatures and code sketches.

---

## Context

M8_04 shipped:
- `STLBaseline.Forecast()` and `Detector.Forecast()` in `internal/ml/`
- `GET /api/v1/instances/{id}/metrics/{metric}/forecast` REST endpoint
- `RuleTypeForecastThreshold`, `Rule.Type`, `Rule.UseLowerBound` in `internal/alert/alert.go`
- `ErrNotBootstrapped` and `ErrNoBaseline` in `internal/ml/errors.go`

M8_05 wires forecast alerting into the evaluation loop and adds a forecast
confidence-band overlay to metric charts in the frontend.

**IMPORTANT — circular import risk:** `internal/ml` already imports `internal/alert`
(for `AlertEvaluator`). `internal/alert` must NEVER import `internal/ml`.
Use interface injection and a new `internal/mlerrors` package as described below.

---

## Create a team of 4 specialists

---

## COLLECTOR AGENT

**Owns:** `internal/ml/`, `internal/mlerrors/`, `internal/config/config.go` (ML section only)

### Task 1 — Create internal/mlerrors/errors.go

Move sentinel errors here so both `internal/ml` and `internal/alert` can import them
without a cycle:

```go
package mlerrors

import "errors"

var ErrNotBootstrapped = errors.New("ml detector not yet bootstrapped")
var ErrNoBaseline      = errors.New("no fitted baseline for this metric")
```

Update `internal/ml/errors.go` to re-export from `mlerrors` (or replace entirely with
imports of `mlerrors`). Update all existing usages of these errors in `internal/ml/`
to use `mlerrors.ErrNotBootstrapped` and `mlerrors.ErrNoBaseline`.

### Task 2 — Create internal/ml/detector_alert.go

Add the `ForecastForAlert` adapter method to satisfy `alert.ForecastProvider`:

```go
package ml

import (
    "context"
    "github.com/ios9000/PGPulse_01/internal/alert"
)

func (d *Detector) ForecastForAlert(
    ctx context.Context,
    instanceID, metricKey string,
    horizon int,
) ([]alert.ForecastPoint, error) {
    result, err := d.Forecast(ctx, instanceID, metricKey, horizon)
    if err != nil {
        return nil, err
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

### Task 3 — Update internal/config/config.go

Add `AlertMinConsecutive int` to `ForecastConfig`:

```go
type ForecastConfig struct {
    Horizon             int     `koanf:"horizon"`
    ConfidenceZ         float64 `koanf:"confidence_z"`
    AlertMinConsecutive int     `koanf:"alert_min_consecutive"`
}
```

Default value: 3. This must be reflected in `configs/pgpulse.example.yml`:
```yaml
ml:
  forecast:
    horizon: 60
    confidence_z: 1.96
    alert_min_consecutive: 3
```

---

## API & SECURITY AGENT

**Owns:** `internal/alert/`, `cmd/pgpulse-server/main.go`, `migrations/`

### Task 1 — Create internal/alert/forecast.go

```go
package alert

import "context"

// ForecastPoint is a thin mirror of ml.ForecastPoint.
// Defined here to avoid importing internal/ml.
type ForecastPoint struct {
    Offset int
    Value  float64
    Lower  float64
    Upper  float64
}

// ForecastProvider is satisfied by *ml.Detector via its ForecastForAlert method.
type ForecastProvider interface {
    ForecastForAlert(
        ctx context.Context,
        instanceID, metricKey string,
        horizon int,
    ) ([]ForecastPoint, error)
}
```

### Task 2 — Update internal/alert/alert.go

Add one field to `Rule`:

```go
ConsecutivePointsRequired int // 0 = use global default from MLConfig
```

Do not change any other field or method.

### Task 3 — Update internal/alert/evaluator.go

Add fields to `Evaluator`:
```go
forecastProvider       ForecastProvider
forecastMinConsecutive int
```

Add setter:
```go
func (e *Evaluator) SetForecastProvider(fp ForecastProvider, minConsecutive int) {
    e.forecastProvider = fp
    e.forecastMinConsecutive = minConsecutive
}
```

Add private method `runForecastAlerts(ctx context.Context, instanceID string) error`.
Call it from `Evaluate()` after existing threshold checks.

Full logic for `runForecastAlerts`:
1. Return nil immediately if `e.forecastProvider == nil`.
2. Filter rules: `rule.Type == RuleTypeForecastThreshold`.
3. For each such rule:
   a. `required = rule.ConsecutivePointsRequired`; if <= 0, use `e.forecastMinConsecutive`.
   b. Call `e.forecastProvider.ForecastForAlert(ctx, instanceID, rule.MetricKey, required+4)`.
   c. If `errors.Is(err, mlerrors.ErrNotBootstrapped)` or `errors.Is(err, mlerrors.ErrNoBaseline)` → `continue`.
   d. Any other error → log WARN, `continue`.
   e. Check cooldown key `"forecast:{instanceID}:{metricKey}:{ruleID}"`. If in cooldown → `continue`.
   f. Scan points: use `pt.Lower` if `rule.UseLowerBound`, else `pt.Value`.
      Count consecutive crossings of `rule.Threshold` in `rule.Direction`.
      On reaching `required`: fire alert, set cooldown, break.
      On non-crossing: reset counter to 0.

Import `internal/mlerrors` (not `internal/ml`) for the sentinel error values.

### Task 4 — Migration

Create `migrations/0XX_forecast_alert_consecutive.sql` (use next available number):

```sql
ALTER TABLE alert_rules
    ADD COLUMN IF NOT EXISTS consecutive_points_required INT NOT NULL DEFAULT 0;

COMMENT ON COLUMN alert_rules.consecutive_points_required IS
    'Minimum consecutive forecast points crossing threshold before alert fires. 0 = global default.';
```

### Task 5 — main.go wiring

After both `evaluator` and `mlDetector` are constructed, add:

```go
evaluator.SetForecastProvider(mlDetector, cfg.ML.Forecast.AlertMinConsecutive)
```

Verify `mlDetector` is of type `*ml.Detector` and satisfies `alert.ForecastProvider`
(it will, once Collector Agent adds the `ForecastForAlert` method).

---

## FRONTEND AGENT

**Owns:** `web/src/`

### Task 1 — Create web/src/hooks/useForecast.ts

```typescript
import { useState, useEffect } from 'react'
import { apiFetch } from '../api/client'  // use existing API client

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
    horizon: number
    confidence_z: number
    points: ForecastPoint[]
}

const FORECAST_POLL_MS = 5 * 60 * 1000

export function useForecast(
    instanceId: string,
    metric: string,
    horizon = 60,
): ForecastResult | null {
    const [result, setResult] = useState<ForecastResult | null>(null)

    useEffect(() => {
        const controller = new AbortController()

        const doFetch = async () => {
            try {
                const data = await apiFetch<ForecastResult>(
                    `/api/v1/instances/${instanceId}/metrics/${encodeURIComponent(metric)}/forecast?horizon=${horizon}`,
                    { signal: controller.signal },
                )
                setResult(data)
            } catch {
                setResult(null)
            }
        }

        doFetch()
        const timer = setInterval(doFetch, FORECAST_POLL_MS)
        return () => {
            controller.abort()
            clearInterval(timer)
        }
    }, [instanceId, metric, horizon])

    return result
}
```

### Task 2 — Create web/src/components/ForecastBand.ts

This is a pure TypeScript helper (no JSX). It builds ECharts series config.

```typescript
import type { CustomSeriesRenderItemAPI, CustomSeriesRenderItemParams } from 'echarts'
import type { ForecastPoint } from '../hooks/useForecast'

const BAND_FILL     = 'rgba(99, 102, 241, 0.15)'
const LINE_COLOR    = '#6366f1'
const DIVIDER_COLOR = '#94a3b8'

/**
 * Returns two ECharts series for a forecast overlay:
 *   1. Custom polygon per segment — confidence band
 *   2. Dashed line — forecast centre value
 *
 * Place the "now" markLine on the last historical series using getNowMarkLine().
 */
export function buildForecastSeries(points: ForecastPoint[]): object[] {
    if (!points.length) return []

    const times  = points.map(p => new Date(p.predicted_at).getTime())
    const values = points.map(p => p.value)
    const lower  = points.map(p => p.lower)
    const upper  = points.map(p => p.upper)

    return [
        // Confidence band: one quadrilateral per adjacent pair of points
        {
            type: 'custom',
            name: 'forecast_band',
            silent: true,
            z: 1,
            renderItem: (
                params: CustomSeriesRenderItemParams,
                api: CustomSeriesRenderItemAPI,
            ) => {
                const i = params.dataIndex
                if (i >= points.length - 1) return { type: 'group', children: [] }

                const lo0 = api.coord([times[i],     lower[i]])
                const lo1 = api.coord([times[i + 1], lower[i + 1]])
                const hi1 = api.coord([times[i + 1], upper[i + 1]])
                const hi0 = api.coord([times[i],     upper[i]])

                return {
                    type: 'polygon',
                    shape: { points: [lo0, lo1, hi1, hi0] },
                    style: { fill: BAND_FILL, stroke: 'none' },
                }
            },
            data: times.map((t, i) => [t, lower[i], upper[i]]),
            encode: { x: 0 },
        },

        // Forecast centre line
        {
            type: 'line',
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
 * Usage — add to the historical series:
 *   markLine: { silent: true, data: [getNowMarkLine(Date.now())] }
 */
export function getNowMarkLine(nowMs: number): object {
    return {
        xAxis: nowMs,
        lineStyle: { type: 'dashed', color: DIVIDER_COLOR, width: 1 },
        label: {
            formatter: 'now',
            position: 'insideStartTop',
            color: DIVIDER_COLOR,
            fontSize: 11,
        },
    }
}
```

### Task 3 — Integrate into ServerDetail.tsx

For each metric chart in ServerDetail that should show a forecast:

1. Add `useForecast` call at component level (one per metric). Start with
   `connections_active` as the first integration. Add others if the component
   already has multiple ECharts instances.

2. Memoize series:
   ```typescript
   const forecastSeries = useMemo(
       () => forecastData ? buildForecastSeries(forecastData.points) : [],
       [forecastData],
   )
   ```

3. Extend X-axis max:
   ```typescript
   const xMax = forecastData?.points.at(-1)
       ? new Date(forecastData.points.at(-1)!.predicted_at).getTime()
       : undefined
   ```

4. Merge into ECharts option:
   ```typescript
   const option = {
       xAxis: { type: 'time', ...(xMax ? { max: xMax } : {}) },
       series: [
           {
               // existing historical series
               type: 'line',
               data: historicalData,
               ...(forecastData ? {
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

5. If `forecastData` is null, the chart must render identically to before — no
   empty state message, no spinner, no layout shift.

---

## QA AGENT

**Owns:** `internal/alert/*_test.go`, `internal/ml/*_test.go`, frontend test files

### Backend tests — internal/alert/evaluator_forecast_test.go (new file)

Write table-driven tests covering:

| Test name | Setup | Expected |
|-----------|-------|----------|
| `NilProvider` | `SetForecastProvider(nil, 3)` | returns nil, no panic |
| `NotBootstrapped` | provider returns `mlerrors.ErrNotBootstrapped` | rule skipped, no alert fired |
| `NoBaseline` | provider returns `mlerrors.ErrNoBaseline` | rule skipped, no alert fired |
| `InsufficientCrossings` | 2 crossings, required=3 | no alert fired |
| `ExactlyRequired` | 3 crossings, required=3 | alert fired once |
| `CooldownRespected` | fire, then call again within cooldown | second call does not fire |
| `UseLowerBound_True` | `pt.Lower` crosses, `pt.Value` does not | fires when flag is true |
| `UseLowerBound_False` | `pt.Value` does not cross | does not fire |
| `CounterResetOnNonCrossing` | 2 crossings, 1 non-crossing, 3 crossings | fires on second run |

Use a mock `ForecastProvider` (interface implementation in test file, not a separate file).

### Backend tests — internal/ml/detector_alert_test.go (new file)

| Test name | What it covers |
|-----------|----------------|
| `TestForecastForAlert_ConvertsPoints` | output slice matches input points field-by-field |
| `TestForecastForAlert_PassesErrNotBootstrapped` | error passes through unchanged |
| `TestForecastForAlert_PassesErrNoBaseline` | error passes through unchanged |
| `TestForecastForAlert_EmptyPoints` | returns empty slice, nil error |

These are unit tests (no testcontainers needed).

### Frontend tests

- `buildForecastSeries([])` returns `[]` (no crash on empty input).
- `buildForecastSeries(samplePoints)` returns array of length 2.
- `useForecast` hook: mock `apiFetch` throwing → result is null.
- `useForecast` hook: mock `apiFetch` returning valid object → result is non-null.

Place frontend tests next to the files:
- `web/src/components/ForecastBand.test.ts`
- `web/src/hooks/useForecast.test.ts`

### Verification

After all agents commit:

1. Confirm no import cycle: `go build ./cmd/pgpulse-server` must succeed.
2. Check `internal/alert` does NOT import `internal/ml` anywhere.
3. Check `internal/ml` does NOT import `internal/alert` except through `mlerrors`.
   Actually: `internal/ml/detector_alert.go` imports `internal/alert` — this is acceptable
   because the dependency runs `ml → alert`, not the other way.
   The cycle would be `alert → ml → alert`. Confirm this does not occur.
4. Run `go test ./cmd/... ./internal/...` — all pass.
5. Run `golangci-lint run` — 0 issues.
6. Run `cd web && npm run build && npm run typecheck && npm run lint`.

---

## Coordination Notes

**Dependency order:**
1. Collector Agent creates `internal/mlerrors/` first — both other backend agents depend on it.
2. API & Security Agent can start `internal/alert/forecast.go` immediately (no ml dependency).
3. API & Security Agent can write `runForecastAlerts` skeleton with a stub ForecastProvider
   before Collector Agent finishes the adapter — fill in the real wiring in main.go last.
4. Frontend Agent works independently — no backend dependency.
5. QA Agent writes test structure immediately, fills assertions as code lands.

**Team Lead merge order:**
1. `internal/mlerrors/` (zero dependencies — merge first)
2. `internal/alert/forecast.go` and `alert.go` change (foundation for evaluator)
3. `internal/ml/detector_alert.go` (depends on alert.ForecastPoint)
4. `internal/alert/evaluator.go` changes
5. `main.go` wiring (depends on both evaluator and detector)
6. Migration file
7. Frontend files (independent, can merge any time after QA confirms no TS errors)
8. All test files last (validates everything together)

**Do not merge any agent's work until `go build ./cmd/pgpulse-server` passes.**

---

## Build Verification (developer runs manually)

```bash
cd web && npm run build && npm run typecheck && npm run lint
cd .. && go build ./cmd/pgpulse-server && go test ./cmd/... ./internal/...
golangci-lint run
```

List all created and modified files when done so the developer can verify the watch list.
