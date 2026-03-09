import { useState, useMemo } from 'react'
import { useParams, Link } from 'react-router-dom'
import { useInstances } from '@/hooks/useInstances'
import { useCurrentMetrics, useMetricsHistory } from '@/hooks/useMetrics'
import { useForecastChart } from '@/hooks/useForecastChart'
import { HeaderCard } from '@/components/server/HeaderCard'
import { KeyMetricsRow } from '@/components/server/KeyMetricsRow'
import { ReplicationSection } from '@/components/server/ReplicationSection'
import { LogicalReplicationSection } from '@/components/server/LogicalReplicationSection'
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
import { InstanceSettingsDiff } from '@/components/SettingsDiff'
import { PlanHistory } from '@/components/PlanHistory'
import { SettingsTimeline } from '@/components/SettingsTimeline'
import { Spinner } from '@/components/ui/Spinner'
import { formatPercent, formatBytes } from '@/lib/formatters'

type TabKey = 'overview' | 'settings-diff' | 'plan-history' | 'settings-timeline'

export function ServerDetail() {
  const { serverId } = useParams()
  const { data: instances } = useInstances()
  const { data: currentMetrics } = useCurrentMetrics(serverId)
  const [activeTab, setActiveTab] = useState<TabKey>('overview')

  const maxConns = currentMetrics?.metrics?.['pgpulse.connections.max_connections']?.value

  const { data: connHistory, isLoading: connLoading } = useMetricsHistory(serverId, [
    'pgpulse.connections.active',
    'pgpulse.connections.idle',
    'pgpulse.connections.total',
  ])

  const { data: cacheHistory, isLoading: cacheLoading } = useMetricsHistory(serverId, [
    'pgpulse.cache.hit_ratio',
  ])

  const { data: txnHistory, isLoading: txnLoading } = useMetricsHistory(serverId, [
    'pgpulse.transactions.commit_ratio_pct',
  ])

  const { data: replLagHistory, isLoading: replLagLoading } = useMetricsHistory(serverId, [
    'pgpulse.replication.lag.replay_bytes',
  ])

  // Forecast overlays
  const connForecast = useForecastChart(serverId ?? '', 'pgpulse.connections.active')
  const cacheForecast = useForecastChart(serverId ?? '', 'pgpulse.cache.hit_ratio')
  const txnForecast = useForecastChart(serverId ?? '', 'pgpulse.transactions.commit_ratio_pct')
  const replLagForecast = useForecastChart(serverId ?? '', 'pgpulse.replication.lag.replay_bytes')

  const instance = instances?.find((i) => i.id === serverId)

  const connSeries = useMemo(() => {
    if (!connHistory?.series) return []
    return Object.entries(connHistory.series).map(([key, points]) => {
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
  }, [connHistory])

  const cacheSeries = useMemo(() => {
    if (!cacheHistory?.series) return []
    return Object.entries(cacheHistory.series).map(([key, points]) => ({
      name: key.replace('pgpulse.cache.', ''),
      data: points,
      color: '#10b981',
      type: 'area' as const,
    }))
  }, [cacheHistory])

  const txnSeries = useMemo(() => {
    if (!txnHistory?.series) return []
    return Object.entries(txnHistory.series).map(([key, points]) => ({
      name: key.replace('pgpulse.transactions.', ''),
      data: points,
      color: '#8b5cf6',
      type: 'area' as const,
    }))
  }, [txnHistory])

  const replLagSeries = useMemo(() => {
    if (!replLagHistory?.series) return []
    return Object.entries(replLagHistory.series).map(([key, points]) => ({
      name: key.replace('pgpulse.replication.lag.', ''),
      data: points,
      color: '#f59e0b',
      type: 'area' as const,
    }))
  }, [replLagHistory])

  if (!serverId) return null

  if (!instance && !currentMetrics) {
    return (
      <div className="flex justify-center py-12">
        <Spinner size="lg" />
      </div>
    )
  }

  const TABS: { key: TabKey; label: string }[] = [
    { key: 'overview', label: 'Overview' },
    { key: 'plan-history', label: 'Plan History' },
    { key: 'settings-diff', label: 'Settings Diff' },
    { key: 'settings-timeline', label: 'Settings Timeline' },
  ]

  return (
    <div className="space-y-6">
      <HeaderCard
        instanceName={instance?.name ?? serverId}
        host={instance?.host ?? ''}
        port={instance?.port ?? 0}
        currentMetrics={currentMetrics}
      />

      <div className="flex items-center gap-3">
        <Link
          to={`/servers/${serverId}/explain`}
          className="rounded-md border border-pgp-border bg-pgp-bg-card px-3 py-1.5 text-sm text-pgp-text-secondary hover:bg-pgp-bg-hover hover:text-pgp-text-primary"
        >
          Explain Query
        </Link>
      </div>

      {/* Tab Bar */}
      <div className="flex border-b border-pgp-border">
        {TABS.map((tab) => (
          <button
            key={tab.key}
            onClick={() => setActiveTab(tab.key)}
            className={`px-4 py-2 text-sm font-medium transition-colors ${
              activeTab === tab.key
                ? 'border-b-2 border-pgp-accent text-pgp-accent'
                : 'text-pgp-text-muted hover:text-pgp-text-secondary'
            }`}
          >
            {tab.label}
          </button>
        ))}
      </div>

      {activeTab === 'overview' && (
        <>
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
                extraSeries={connForecast.extraSeries}
                xAxisMax={connForecast.xAxisMax}
                nowMarkLine={connForecast.hasForecast ? Date.now() : undefined}
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
                extraSeries={cacheForecast.extraSeries}
                xAxisMax={cacheForecast.xAxisMax}
                nowMarkLine={cacheForecast.hasForecast ? Date.now() : undefined}
              />
            </div>
          </div>

          <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
            <div className="rounded-lg border border-pgp-border bg-pgp-bg-card p-4">
              <h3 className="mb-3 text-sm font-medium text-pgp-text-secondary">Transaction Commit Ratio</h3>
              <TimeSeriesChart
                series={txnSeries}
                yAxisLabel="%"
                yAxisFormat={(v: number) => formatPercent(v, 0)}
                yAxisMin={0}
                yAxisMax={100}
                loading={txnLoading}
                extraSeries={txnForecast.extraSeries}
                xAxisMax={txnForecast.xAxisMax}
                nowMarkLine={txnForecast.hasForecast ? Date.now() : undefined}
              />
            </div>
            <div className="rounded-lg border border-pgp-border bg-pgp-bg-card p-4">
              <h3 className="mb-3 text-sm font-medium text-pgp-text-secondary">Replication Lag</h3>
              <TimeSeriesChart
                series={replLagSeries}
                yAxisLabel="bytes"
                yAxisFormat={(v: number) => formatBytes(v)}
                yAxisMin={0}
                loading={replLagLoading}
                extraSeries={replLagForecast.extraSeries}
                xAxisMax={replLagForecast.xAxisMax}
                nowMarkLine={replLagForecast.hasForecast ? Date.now() : undefined}
              />
            </div>
          </div>

          <ReplicationSection instanceId={serverId} />
          <LogicalReplicationSection instanceId={serverId} />

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
        </>
      )}

      {activeTab === 'plan-history' && (
        <div className="rounded-lg border border-pgp-border bg-pgp-bg-card p-4">
          <h2 className="mb-4 text-lg font-semibold text-pgp-text-primary">
            Plan Capture History
          </h2>
          <PlanHistory instanceId={serverId} />
        </div>
      )}

      {activeTab === 'settings-diff' && (
        <div className="rounded-lg border border-pgp-border bg-pgp-bg-card p-4">
          <h2 className="mb-4 text-lg font-semibold text-pgp-text-primary">
            Settings Diff (vs. Defaults)
          </h2>
          <InstanceSettingsDiff instanceId={serverId} />
        </div>
      )}

      {activeTab === 'settings-timeline' && (
        <div className="rounded-lg border border-pgp-border bg-pgp-bg-card p-4">
          <h2 className="mb-4 text-lg font-semibold text-pgp-text-primary">
            Settings Timeline
          </h2>
          <SettingsTimeline instanceId={serverId} />
        </div>
      )}
    </div>
  )
}
