import { useParams } from 'react-router-dom'
import { useInstances } from '@/hooks/useInstances'
import { useCurrentMetrics, useMetricsHistory } from '@/hooks/useMetrics'
import { HeaderCard } from '@/components/server/HeaderCard'
import { KeyMetricsRow } from '@/components/server/KeyMetricsRow'
import { ReplicationSection } from '@/components/server/ReplicationSection'
import { WaitEventsSection } from '@/components/server/WaitEventsSection'
import { LongTransactionsTable } from '@/components/server/LongTransactionsTable'
import { StatementsSection } from '@/components/server/StatementsSection'
import { LockTreeSection } from '@/components/server/LockTreeSection'
import { ProgressSection } from '@/components/server/ProgressSection'
import { InstanceAlerts } from '@/components/server/InstanceAlerts'
import { OSSystemSection } from '@/components/server/OSSystemSection'
import { DiskSection } from '@/components/server/DiskSection'
import { IOStatsSection } from '@/components/server/IOStatsSection'
import { ClusterSection } from '@/components/server/ClusterSection'
import { TimeRangeSelector } from '@/components/shared/TimeRangeSelector'
import { TimeSeriesChart } from '@/components/charts/TimeSeriesChart'
import { Spinner } from '@/components/ui/Spinner'
import { formatPercent } from '@/lib/formatters'

export function ServerDetail() {
  const { serverId } = useParams()
  const { data: instances } = useInstances()
  const { data: currentMetrics } = useCurrentMetrics(serverId)

  const maxConns = currentMetrics?.metrics?.['pgpulse.connections.max_connections']?.value

  const { data: connHistory, isLoading: connLoading } = useMetricsHistory(serverId, [
    'pgpulse.connections.active',
    'pgpulse.connections.idle',
    'pgpulse.connections.total',
  ])

  const { data: cacheHistory, isLoading: cacheLoading } = useMetricsHistory(serverId, [
    'pgpulse.cache.hit_ratio',
  ])

  const instance = instances?.find((i) => i.id === serverId)

  if (!serverId) return null

  if (!instance && !currentMetrics) {
    return (
      <div className="flex justify-center py-12">
        <Spinner size="lg" />
      </div>
    )
  }

  const connSeries = connHistory?.series
    ? Object.entries(connHistory.series).map(([key, points]) => {
        const shortName = key.replace('pgpulse.connections.', '')
        const colors: Record<string, string> = {
          active: '#3b82f6',
          idle: '#94a3b8',
          total: '#10b981',
        }
        return {
          name: shortName,
          data: points,
          color: colors[shortName],
          type: shortName === 'total' ? 'line' as const : 'area' as const,
          dashed: shortName === 'total',
        }
      })
    : []

  const cacheSeries = cacheHistory?.series
    ? Object.entries(cacheHistory.series).map(([key, points]) => ({
        name: key.replace('pgpulse.cache.', ''),
        data: points,
        color: '#10b981',
        type: 'area' as const,
      }))
    : []

  return (
    <div className="space-y-6">
      <HeaderCard
        instanceName={instance?.name ?? serverId}
        host={instance?.host ?? ''}
        port={instance?.port ?? 0}
        currentMetrics={currentMetrics}
      />

      <ProgressSection instanceId={serverId} />

      <KeyMetricsRow currentMetrics={currentMetrics} />

      <TimeRangeSelector />

      <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
        <div className="rounded-lg border border-pgp-border bg-pgp-bg-card p-4">
          <h3 className="mb-3 text-sm font-medium text-pgp-text-secondary">Connections</h3>
          <TimeSeriesChart
            series={connSeries}
            referenceLine={maxConns ? { value: maxConns, label: `max: ${maxConns}`, color: '#ef4444' } : undefined}
            yAxisLabel="connections"
            yAxisMin={0}
            loading={connLoading}
          />
        </div>
        <div className="rounded-lg border border-pgp-border bg-pgp-bg-card p-4">
          <h3 className="mb-3 text-sm font-medium text-pgp-text-secondary">Cache Hit Ratio</h3>
          <TimeSeriesChart
            series={cacheSeries}
            referenceLine={{ value: 95, label: '95%', color: '#f59e0b' }}
            yAxisLabel="%"
            yAxisFormat={(v: number) => formatPercent(v, 0)}
            yAxisMin={0}
            yAxisMax={100}
            loading={cacheLoading}
          />
        </div>
      </div>

      <ReplicationSection instanceId={serverId} />

      <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
        <WaitEventsSection instanceId={serverId} />
        <LongTransactionsTable instanceId={serverId} />
      </div>

      <StatementsSection instanceId={serverId} />

      <LockTreeSection instanceId={serverId} />

      <InstanceAlerts instanceId={serverId} />

      <OSSystemSection instanceId={serverId} />

      <DiskSection instanceId={serverId} />

      <IOStatsSection instanceId={serverId} />

      <ClusterSection instanceId={serverId} />
    </div>
  )
}
