import { MetricCard } from '@/components/ui/MetricCard'
import { formatPercent, formatBytes, thresholdColor } from '@/lib/formatters'
import type { CurrentMetricsResult } from '@/types/models'

interface KeyMetricsRowProps {
  currentMetrics: CurrentMetricsResult | undefined
}

export function KeyMetricsRow({ currentMetrics }: KeyMetricsRowProps) {
  const m = currentMetrics?.metrics ?? {}

  const active = m['pg.connections.active']?.value ?? 0
  const maxConns = m['pg.connections.max_connections']?.value ?? 0
  const connUtil = maxConns > 0 ? (active / maxConns) * 100 : 0
  const connStatus = thresholdColor(connUtil, 70, 90)

  const cacheHit = m['pg.cache.hit_ratio']?.value
  const cacheStatus = cacheHit != null ? thresholdColor(cacheHit, 95, 90, true) : undefined

  const activeTxns = m['pg.transactions.active']?.value ?? 0

  const replLag = m['pg.replication.replay_lag_bytes']?.value

  return (
    <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
      <MetricCard
        label="Connections"
        value={`${Math.round(active)} / ${Math.round(maxConns)}`}
        unit={formatPercent(connUtil, 0)}
        status={connStatus}
      />
      <MetricCard
        label="Cache Hit Ratio"
        value={cacheHit != null ? formatPercent(cacheHit) : '--'}
        status={cacheStatus}
      />
      <MetricCard
        label="Active Transactions"
        value={Math.round(activeTxns)}
      />
      <MetricCard
        label="Replication Lag"
        value={replLag != null ? formatBytes(replLag) : 'N/A'}
        status={replLag != null ? thresholdColor(replLag, 1024 * 1024, 10 * 1024 * 1024) : undefined}
      />
    </div>
  )
}
