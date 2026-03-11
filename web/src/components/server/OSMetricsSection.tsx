import { useMemo } from 'react'
import { useCurrentMetrics, useMetricsHistory } from '@/hooks/useMetrics'
import { useOSMetrics } from '@/hooks/useOSMetrics'
import { MetricCard } from '@/components/ui/MetricCard'
import { TimeSeriesChart } from '@/components/charts/TimeSeriesChart'
import { thresholdColor } from '@/lib/formatters'

interface OSMetricsSectionProps {
  instanceId: string
}

const OS_MEMORY_METRICS = [
  'os.memory.used_kb',
  'os.memory.cached_kb',
  'os.memory.buffers_kb',
  'os.memory.available_kb',
]

const OS_CPU_METRICS = [
  'os.cpu.user_pct',
  'os.cpu.system_pct',
  'os.cpu.iowait_pct',
  'os.cpu.idle_pct',
]

const OS_LOAD_METRICS = ['os.load.1m', 'os.load.5m', 'os.load.15m']

const OS_DISK_METRICS = [
  'os.disk.read_bytes_per_sec',
  'os.disk.write_bytes_per_sec',
  'os.disk.io_util_pct',
]

function formatKB(kb: number): string {
  if (kb >= 1048576) return `${(kb / 1048576).toFixed(1)} GB`
  if (kb >= 1024) return `${(kb / 1024).toFixed(0)} MB`
  return `${kb} KB`
}

function kbToGB(kb: number): number {
  return kb / 1048576
}

function bytesToMB(bytes: number): number {
  return bytes / (1024 * 1024)
}

export function OSMetricsSection({ instanceId }: OSMetricsSectionProps) {
  const { data: currentMetrics } = useCurrentMetrics(instanceId)
  const { data: osAgentData } = useOSMetrics(instanceId)

  const m = currentMetrics?.metrics ?? {}

  // Check if any os.* metrics exist in current metrics
  const osMetricKeys = Object.keys(m).filter((k) => k.startsWith('os.'))
  const hasOSMetrics = osMetricKeys.length > 0

  // History queries - only enabled when os metrics exist
  const { data: memHistory, isLoading: memLoading } = useMetricsHistory(
    hasOSMetrics ? instanceId : undefined,
    OS_MEMORY_METRICS,
  )
  const { data: cpuHistory, isLoading: cpuLoading } = useMetricsHistory(
    hasOSMetrics ? instanceId : undefined,
    OS_CPU_METRICS,
  )
  const { data: loadHistory, isLoading: loadLoading } = useMetricsHistory(
    hasOSMetrics ? instanceId : undefined,
    OS_LOAD_METRICS,
  )
  const { data: diskHistory, isLoading: diskLoading } = useMetricsHistory(
    hasOSMetrics ? instanceId : undefined,
    OS_DISK_METRICS,
  )

  // Current values for stat cards
  const memUsed = m['os.memory.used_kb']?.value
  const memTotal = m['os.memory.total_kb']?.value
  const memPct = memUsed != null && memTotal && memTotal > 0 ? (memUsed / memTotal) * 100 : undefined

  const cpuUser = m['os.cpu.user_pct']?.value ?? 0
  const cpuSys = m['os.cpu.system_pct']?.value ?? 0
  const cpuTotal = cpuUser + cpuSys

  const load1 = m['os.load.1m']?.value
  const load5 = m['os.load.5m']?.value
  const load15 = m['os.load.15m']?.value

  const diskUtil = m['os.disk.io_util_pct']?.value

  // Chart series
  const memSeries = useMemo(() => {
    if (!memHistory?.series) return []
    const mapping: Record<string, { name: string; color: string }> = {
      'os.memory.used_kb': { name: 'Used', color: '#3b82f6' },
      'os.memory.cached_kb': { name: 'Cached', color: '#10b981' },
      'os.memory.buffers_kb': { name: 'Buffers', color: '#14b8a6' },
      'os.memory.available_kb': { name: 'Available', color: '#6b7280' },
    }
    return Object.entries(memHistory.series)
      .filter(([key]) => mapping[key])
      .map(([key, points]) => ({
        name: mapping[key].name,
        data: points.map((p) => ({ t: p.t, v: kbToGB(p.v) })),
        color: mapping[key].color,
        type: 'area' as const,
      }))
  }, [memHistory])

  const cpuSeries = useMemo(() => {
    if (!cpuHistory?.series) return []
    const mapping: Record<string, { name: string; color: string; dashed?: boolean; type: 'area' | 'line' }> = {
      'os.cpu.user_pct': { name: 'User', color: '#3b82f6', type: 'area' },
      'os.cpu.system_pct': { name: 'System', color: '#f97316', type: 'area' },
      'os.cpu.iowait_pct': { name: 'IOWait', color: '#ef4444', type: 'area' },
      'os.cpu.idle_pct': { name: 'Idle', color: '#6b7280', dashed: true, type: 'line' },
    }
    return Object.entries(cpuHistory.series)
      .filter(([key]) => mapping[key])
      .map(([key, points]) => ({
        name: mapping[key].name,
        data: points,
        color: mapping[key].color,
        type: mapping[key].type,
        dashed: mapping[key].dashed,
      }))
  }, [cpuHistory])

  const loadSeries = useMemo(() => {
    if (!loadHistory?.series) return []
    const mapping: Record<string, { name: string; color: string; dashed?: boolean }> = {
      'os.load.1m': { name: 'Load 1m', color: '#3b82f6' },
      'os.load.5m': { name: 'Load 5m', color: '#10b981' },
      'os.load.15m': { name: 'Load 15m', color: '#6b7280', dashed: true },
    }
    return Object.entries(loadHistory.series)
      .filter(([key]) => mapping[key])
      .map(([key, points]) => ({
        name: mapping[key].name,
        data: points,
        color: mapping[key].color,
        type: 'line' as const,
        dashed: mapping[key].dashed,
      }))
  }, [loadHistory])

  const diskSeries = useMemo(() => {
    if (!diskHistory?.series) return []
    const result: Array<{
      name: string
      data: Array<{ t: string; v: number }>
      color: string
      type: 'line' | 'area'
      dashed?: boolean
    }> = []

    const readPoints = diskHistory.series['os.disk.read_bytes_per_sec']
    if (readPoints) {
      result.push({
        name: 'Read MB/s',
        data: readPoints.map((p) => ({ t: p.t, v: bytesToMB(p.v) })),
        color: '#3b82f6',
        type: 'area',
      })
    }
    const writePoints = diskHistory.series['os.disk.write_bytes_per_sec']
    if (writePoints) {
      result.push({
        name: 'Write MB/s',
        data: writePoints.map((p) => ({ t: p.t, v: bytesToMB(p.v) })),
        color: '#10b981',
        type: 'area',
      })
    }
    const utilPoints = diskHistory.series['os.disk.io_util_pct']
    if (utilPoints) {
      result.push({
        name: 'Util %',
        data: utilPoints,
        color: '#f59e0b',
        type: 'line',
        dashed: true,
      })
    }
    return result
  }, [diskHistory])

  // Agent configured?
  const agentConfigured = osAgentData?.available === true

  if (!hasOSMetrics) {
    return (
      <div className="rounded-lg border border-pgp-border bg-pgp-bg-card p-4">
        <h3 className="mb-3 text-sm font-medium text-pgp-text-secondary">OS Metrics</h3>
        <div className="rounded-md bg-pgp-bg-secondary px-4 py-3 text-sm text-pgp-text-muted">
          No OS metrics available. Enable{' '}
          <code className="rounded bg-pgp-bg-card px-1">os_metrics_method: sql</code> in configuration
          or deploy pgpulse-agent.
        </div>
      </div>
    )
  }

  const totalRAMgb = memTotal ? kbToGB(memTotal) : undefined

  return (
    <div className="space-y-4">
      {/* Stat Cards Row */}
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <MetricCard
          label="Memory"
          value={
            memUsed != null && memTotal
              ? `${formatKB(memUsed)} / ${formatKB(memTotal)}`
              : '--'
          }
          unit={memPct != null ? `${memPct.toFixed(1)}%` : undefined}
          status={memPct != null ? thresholdColor(memPct, 70, 90) : undefined}
        />
        <MetricCard
          label="CPU"
          value={hasOSMetrics ? `${cpuTotal.toFixed(1)}%` : '--'}
          unit={`usr ${cpuUser.toFixed(1)}% + sys ${cpuSys.toFixed(1)}%`}
          status={thresholdColor(cpuTotal, 70, 90)}
        />
        <MetricCard
          label="Load Average"
          value={load1 != null ? load1.toFixed(2) : '--'}
          unit={
            load5 != null && load15 != null
              ? `${load5.toFixed(2)} / ${load15.toFixed(2)}`
              : undefined
          }
        />
        <MetricCard
          label="Disk I/O Util"
          value={diskUtil != null ? `${diskUtil.toFixed(1)}%` : '--'}
          status={diskUtil != null ? thresholdColor(diskUtil, 70, 90) : undefined}
        />
      </div>

      {/* Charts 2x2 Grid */}
      <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
        <div className="rounded-lg border border-pgp-border bg-pgp-bg-card p-4">
          <h3 className="mb-3 text-sm font-medium text-pgp-text-secondary">Memory Usage</h3>
          <TimeSeriesChart
            series={memSeries}
            yAxisLabel="GB"
            yAxisMin={0}
            referenceLine={
              totalRAMgb
                ? { value: totalRAMgb, label: `Total: ${totalRAMgb.toFixed(1)} GB`, color: '#f59e0b' }
                : undefined
            }
            loading={memLoading}
          />
        </div>
        <div className="rounded-lg border border-pgp-border bg-pgp-bg-card p-4">
          <h3 className="mb-3 text-sm font-medium text-pgp-text-secondary">CPU Usage</h3>
          <TimeSeriesChart
            series={cpuSeries}
            yAxisLabel="%"
            yAxisMin={0}
            yAxisMax={100}
            loading={cpuLoading}
          />
        </div>
      </div>

      <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
        <div className="rounded-lg border border-pgp-border bg-pgp-bg-card p-4">
          <h3 className="mb-3 text-sm font-medium text-pgp-text-secondary">Load Average</h3>
          <TimeSeriesChart
            series={loadSeries}
            yAxisLabel="load"
            yAxisMin={0}
            loading={loadLoading}
          />
        </div>
        <div className="rounded-lg border border-pgp-border bg-pgp-bg-card p-4">
          <h3 className="mb-3 text-sm font-medium text-pgp-text-secondary">Disk I/O</h3>
          <TimeSeriesChart
            series={diskSeries}
            yAxisLabel="MB/s"
            yAxisMin={0}
            loading={diskLoading}
          />
        </div>
      </div>

      {/* Agent info badge */}
      {!agentConfigured && (
        <div className="rounded-md bg-pgp-bg-secondary px-4 py-2 text-xs text-pgp-text-muted">
          Additional OS metrics (process list, filesystem layout, network) available with pgpulse-agent.
        </div>
      )}
    </div>
  )
}
