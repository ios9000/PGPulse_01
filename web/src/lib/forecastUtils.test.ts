import { describe, expect, it, vi } from 'vitest'
import type { CustomSeriesRenderItemAPI, CustomSeriesRenderItemParams } from 'echarts'
import { buildForecastSeries, getNowMarkLine } from './forecastUtils'
import type { ForecastPoint } from '@/hooks/useForecast'

interface BandSeries {
  type: string
  name: string
  data: number[][]
  renderItem: (params: CustomSeriesRenderItemParams, api: CustomSeriesRenderItemAPI) => unknown
}
interface LineSeries {
  type: string
  name: string
  data: number[][]
  lineStyle: { type: string }
}

function point(iso: string, value: number, lower = value - 1, upper = value + 1, offset = 0): ForecastPoint {
  return { offset, predicted_at: iso, value, lower, upper }
}

// Fake ECharts API: coord() echoes the data pair so polygon vertices are inspectable.
function fakeApi() {
  return { coord: vi.fn((pair: number[]) => pair) } as unknown as CustomSeriesRenderItemAPI
}
function params(dataIndex: number) {
  return { dataIndex } as CustomSeriesRenderItemParams
}

const T0 = '2026-11-04T10:00:00Z'
const T1 = '2026-11-04T10:01:00Z'
const T2 = '2026-11-04T10:02:00Z'
const ms = (iso: string) => new Date(iso).getTime()

describe('buildForecastSeries', () => {
  it('returns no series for an empty forecast', () => {
    expect(buildForecastSeries([])).toEqual([])
  })

  it('returns a band series followed by a dashed centre line', () => {
    const [band, line] = buildForecastSeries([point(T0, 10), point(T1, 20)]) as [BandSeries, LineSeries]
    expect(band.type).toBe('custom')
    expect(band.name).toBe('forecast_band')
    expect(line.type).toBe('line')
    expect(line.name).toBe('forecast_value')
    expect(line.lineStyle.type).toBe('dashed')
  })

  it('maps predicted_at to epoch ms and carries value/lower/upper through', () => {
    const [band, line] = buildForecastSeries([point(T0, 10, 8, 12), point(T1, 20, 15, 25)]) as [
      BandSeries,
      LineSeries,
    ]
    expect(line.data).toEqual([
      [ms(T0), 10],
      [ms(T1), 20],
    ])
    expect(band.data).toEqual([
      [ms(T0), 8, 12],
      [ms(T1), 15, 25],
    ])
  })

  it('handles a single point: one datum, and the band renders an empty group', () => {
    const [band, line] = buildForecastSeries([point(T0, 5)]) as [BandSeries, LineSeries]
    expect(line.data).toEqual([[ms(T0), 5]])
    const api = fakeApi()
    expect(band.renderItem(params(0), api)).toEqual({ type: 'group', children: [] })
    expect(api.coord).not.toHaveBeenCalled()
  })

  it('band renders a quadrilateral lower(i) → lower(i+1) → upper(i+1) → upper(i)', () => {
    const [band] = buildForecastSeries([point(T0, 10, 8, 12), point(T1, 20, 15, 25)]) as [BandSeries]
    const shape = band.renderItem(params(0), fakeApi()) as {
      type: string
      shape: { points: number[][] }
    }
    expect(shape.type).toBe('polygon')
    expect(shape.shape.points).toEqual([
      [ms(T0), 8],
      [ms(T1), 15],
      [ms(T1), 25],
      [ms(T0), 12],
    ])
  })

  it('band renders nothing for the last point (no segment after it)', () => {
    const [band] = buildForecastSeries([point(T0, 1), point(T1, 2), point(T2, 3)]) as [BandSeries]
    expect(band.renderItem(params(2), fakeApi())).toEqual({ type: 'group', children: [] })
  })

  it('preserves input order and does not re-sort timestamps', () => {
    // Callers are expected to pass points in offset order; the builder must not silently reorder them.
    const [, line] = buildForecastSeries([point(T2, 3), point(T0, 1), point(T1, 2)]) as [unknown, LineSeries]
    expect(line.data.map((d) => d[0])).toEqual([ms(T2), ms(T0), ms(T1)])
  })

  it('passes NaN values and unparseable timestamps through without throwing', () => {
    const [band, line] = buildForecastSeries([
      point('not-a-date', Number.NaN, Number.NaN, Number.NaN),
      point(T1, 2),
    ]) as [BandSeries, LineSeries]
    expect(Number.isNaN(line.data[0][0])).toBe(true)
    expect(Number.isNaN(line.data[0][1])).toBe(true)
    expect(line.data[1]).toEqual([ms(T1), 2])
    expect(() => band.renderItem(params(0), fakeApi())).not.toThrow()
  })
})

describe('getNowMarkLine', () => {
  it('places a dashed divider at the given timestamp labelled "now"', () => {
    const now = ms(T1)
    const mark = getNowMarkLine(now) as {
      xAxis: number
      lineStyle: { type: string }
      label: { formatter: string }
    }
    expect(mark.xAxis).toBe(now)
    expect(mark.lineStyle.type).toBe('dashed')
    expect(mark.label.formatter).toBe('now')
  })

  it('returns a fresh object per call', () => {
    const a = getNowMarkLine(1) as { xAxis: number }
    const b = getNowMarkLine(2) as { xAxis: number }
    expect(a).not.toBe(b)
    expect([a.xAxis, b.xAxis]).toEqual([1, 2])
  })
})
