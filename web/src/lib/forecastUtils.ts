import type { CustomSeriesRenderItemAPI, CustomSeriesRenderItemParams } from 'echarts'
import type { ForecastPoint } from '../hooks/useForecast'

const BAND_FILL = 'rgba(99, 102, 241, 0.15)'
const LINE_COLOR = '#6366f1'
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

  const times = points.map((p) => new Date(p.predicted_at).getTime())
  const values = points.map((p) => p.value)
  const lower = points.map((p) => p.lower)
  const upper = points.map((p) => p.upper)

  return [
    // Confidence band: one quadrilateral per adjacent pair of points
    {
      type: 'custom',
      name: 'forecast_band',
      silent: true,
      z: 1,
      renderItem: (params: CustomSeriesRenderItemParams, api: CustomSeriesRenderItemAPI) => {
        const i = params.dataIndex
        if (i >= points.length - 1) return { type: 'group', children: [] }

        const lo0 = api.coord!([times[i], lower[i]])
        const lo1 = api.coord!([times[i + 1], lower[i + 1]])
        const hi1 = api.coord!([times[i + 1], upper[i + 1]])
        const hi0 = api.coord!([times[i], upper[i]])

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
