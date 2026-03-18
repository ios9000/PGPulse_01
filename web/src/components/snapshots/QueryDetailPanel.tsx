import { useQueryInsights } from '@/hooks/useSnapshots'
import { QueryText } from './QueryText'
import { EChartWrapper } from '@/components/ui/EChartWrapper'
import { formatTimestamp } from '@/lib/formatters'
import type { EChartsOption } from 'echarts'
import type { PGSSQueryInsightPoint } from '@/types/models'

interface QueryDetailPanelProps {
  instanceId: string
  queryId: number
  onClose: () => void
}

function buildMiniChart(
  points: PGSSQueryInsightPoint[],
  yKey: keyof PGSSQueryInsightPoint,
  title: string,
  color: string,
  yAxisFormatter?: (v: number) => string,
): EChartsOption {
  const xData = points.map((p) => p.captured_at)
  const yData = points.map((p) => p[yKey] as number)

  return {
    title: { text: title, textStyle: { fontSize: 12 }, left: 'center', top: 0 },
    grid: { top: 30, right: 10, bottom: 24, left: 50 },
    xAxis: {
      type: 'category',
      data: xData,
      axisLabel: {
        fontSize: 9,
        formatter: (v: string) => {
          const d = new Date(v)
          return `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`
        },
      },
    },
    yAxis: {
      type: 'value',
      axisLabel: {
        fontSize: 9,
        formatter: yAxisFormatter || ((v: number) => String(v)),
      },
    },
    series: [
      {
        type: 'line',
        data: yData,
        smooth: true,
        symbol: 'circle',
        symbolSize: 4,
        lineStyle: { width: 2, color },
        itemStyle: { color },
        areaStyle: { color, opacity: 0.08 },
      },
    ],
    tooltip: {
      trigger: 'axis',
      formatter: (params: unknown) => {
        const p = (params as { data: number; name: string }[])[0]
        if (!p) return ''
        const d = new Date(p.name)
        const ts = d.toLocaleString()
        const val = yAxisFormatter ? yAxisFormatter(p.data) : String(p.data)
        return `${ts}<br/>${title}: <b>${val}</b>`
      },
    },
  }
}

export function QueryDetailPanel({ instanceId, queryId, onClose }: QueryDetailPanelProps) {
  const { data: insight, isLoading, error } = useQueryInsights(instanceId, queryId)

  if (isLoading) {
    return (
      <div className="rounded-lg border border-pgp-border bg-pgp-bg-secondary p-4">
        <div className="animate-pulse text-sm text-pgp-text-muted">Loading query insights...</div>
      </div>
    )
  }

  if (error || !insight) {
    return (
      <div className="rounded-lg border border-pgp-border bg-pgp-bg-secondary p-4">
        <div className="text-sm text-red-400">
          Failed to load query insights.{' '}
          <button onClick={onClose} className="underline hover:text-pgp-accent">
            Close
          </button>
        </div>
      </div>
    )
  }

  const points = insight.points ?? []

  const callsChart = buildMiniChart(points, 'calls_delta', 'Calls / Interval', '#3b82f6')
  const execChart = buildMiniChart(
    points,
    'exec_time_delta_ms',
    'Exec Time / Interval',
    '#f59e0b',
    (v) => (v < 1000 ? `${v.toFixed(0)} ms` : `${(v / 1000).toFixed(1)} s`),
  )
  const avgChart = buildMiniChart(
    points,
    'avg_exec_time_ms',
    'Avg Exec Time',
    '#8b5cf6',
    (v) => (v < 1 ? `${(v * 1000).toFixed(0)} us` : `${v.toFixed(1)} ms`),
  )
  const hitChart = buildMiniChart(
    points,
    'shared_hit_ratio_pct',
    'Shared Hit Ratio',
    '#10b981',
    (v) => `${v.toFixed(1)}%`,
  )

  return (
    <div className="rounded-lg border border-pgp-border bg-pgp-bg-secondary p-4">
      <div className="mb-3 flex items-start justify-between">
        <div>
          <div className="mb-1 text-xs text-pgp-text-muted">
            queryid: {insight.queryid} | {insight.database_name} | {insight.user_name}
            {insight.first_seen && ` | First seen: ${formatTimestamp(insight.first_seen)}`}
          </div>
        </div>
        <button
          onClick={onClose}
          className="rounded px-2 py-1 text-xs text-pgp-text-muted hover:bg-pgp-bg-hover hover:text-pgp-text-primary"
        >
          Close
        </button>
      </div>

      <QueryText query={insight.query} maxLength={300} className="mb-4" />

      {points.length > 0 ? (
        <div className="grid grid-cols-2 gap-3">
          <EChartWrapper option={callsChart} height={180} />
          <EChartWrapper option={execChart} height={180} />
          <EChartWrapper option={avgChart} height={180} />
          <EChartWrapper option={hitChart} height={180} />
        </div>
      ) : (
        <div className="py-6 text-center text-sm text-pgp-text-muted">
          Not enough data points for charts
        </div>
      )}
    </div>
  )
}
