import { useMemo } from 'react'
import type { EChartsOption } from 'echarts'
import { EChartWrapper } from '@/components/ui/EChartWrapper'
import { Spinner } from '@/components/ui/Spinner'

interface SeriesData {
  name: string
  data: Array<{ t: string; v: number }>
  color?: string
  type?: 'line' | 'area'
  dashed?: boolean
}

interface TimeSeriesChartProps {
  series: SeriesData[]
  referenceLine?: { value: number; label: string; color?: string }
  yAxisLabel?: string
  yAxisFormat?: (v: number) => string
  yAxisMin?: number
  yAxisMax?: number
  loading?: boolean
  height?: number
}

export function TimeSeriesChart({
  series,
  referenceLine,
  yAxisLabel,
  yAxisFormat,
  yAxisMin,
  yAxisMax,
  loading = false,
  height = 300,
}: TimeSeriesChartProps) {
  const option = useMemo((): EChartsOption => {
    if (series.length === 0 || series.every((s) => s.data.length === 0)) {
      return {}
    }

    const echartseries = series.map((s, idx) => {
      const base: Record<string, unknown> = {
        name: s.name,
        type: 'line',
        data: s.data.map((p) => [p.t, p.v]),
        smooth: true,
        symbol: 'none',
        lineStyle: {
          width: 2,
          ...(s.color ? { color: s.color } : {}),
          ...(s.dashed ? { type: 'dashed' as const } : {}),
        },
        ...(s.color ? { itemStyle: { color: s.color } } : {}),
      }

      if (s.type === 'area') {
        base.areaStyle = { opacity: 0.15 }
      }

      if (idx === 0 && referenceLine) {
        base.markLine = {
          silent: true,
          symbol: 'none',
          label: {
            formatter: referenceLine.label,
            position: 'insideEndTop' as const,
            color: referenceLine.color || '#f59e0b',
          },
          lineStyle: {
            type: 'dashed' as const,
            color: referenceLine.color || '#f59e0b',
          },
          data: [{ yAxis: referenceLine.value }],
        }
      }

      return base
    })

    return {
      tooltip: {
        trigger: 'axis',
        axisPointer: { type: 'cross' },
      },
      legend: {
        bottom: 0,
        data: series.map((s) => s.name),
      },
      grid: {
        left: '3%',
        right: '4%',
        bottom: series.length > 1 ? '15%' : '8%',
        top: '8%',
        containLabel: true,
      },
      xAxis: {
        type: 'time',
      },
      yAxis: {
        type: 'value',
        name: yAxisLabel,
        min: yAxisMin,
        max: yAxisMax,
        axisLabel: yAxisFormat
          ? { formatter: (v: number) => yAxisFormat(v) }
          : undefined,
      },
      series: echartseries,
    }
  }, [series, referenceLine, yAxisLabel, yAxisFormat, yAxisMin, yAxisMax])

  const isEmpty = series.length === 0 || series.every((s) => s.data.length === 0)

  if (loading) {
    return (
      <div className="flex items-center justify-center" style={{ height }}>
        <Spinner size="lg" />
      </div>
    )
  }

  if (isEmpty) {
    return (
      <div className="flex items-center justify-center text-pgp-text-muted" style={{ height }}>
        No data available
      </div>
    )
  }

  return <EChartWrapper option={option} height={height} />
}
