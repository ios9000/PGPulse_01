import { useNavigate } from 'react-router-dom'
import type { InstanceData } from '@/types/models'
import { StatusBadge } from '@/components/ui/StatusBadge'
import { AlertBadge } from '@/components/shared/AlertBadge'
import { formatPGVersion, formatPercent } from '@/lib/formatters'

interface InstanceCardProps {
  instance: InstanceData
}

export function InstanceCard({ instance }: InstanceCardProps) {
  const navigate = useNavigate()
  const m = instance.metrics ?? {}

  const hasMetrics = Object.keys(m).length > 0
  const status = hasMetrics ? 'ok' : 'unknown'

  const versionNum = m['pgpulse.server.version_num']
  const versionStr = versionNum ? formatPGVersion(versionNum) : null
  const isReplica = m['pgpulse.server.is_in_recovery'] === 1

  const active = m['pgpulse.connections.active'] ?? 0
  const maxConns = m['pgpulse.connections.max_connections'] ?? 0
  const connUtil = maxConns > 0 ? (active / maxConns) * 100 : 0

  const cacheHit = m['pgpulse.cache.hit_ratio']
  const replLag = m['pgpulse.replication.replay_lag_bytes']

  const alertCounts = instance.alert_counts ?? {}
  const criticals = alertCounts['critical'] ?? 0
  const warnings = alertCounts['warning'] ?? 0

  return (
    <div
      onClick={() => navigate(`/servers/${instance.id}`)}
      className="cursor-pointer rounded-lg border border-pgp-border bg-pgp-bg-card p-4 transition-colors hover:bg-pgp-bg-hover"
    >
      <div className="flex items-start justify-between">
        <div className="flex items-center gap-2">
          <StatusBadge status={status === 'ok' ? 'ok' : 'unknown'} size="sm" />
          <span className="text-lg font-bold text-pgp-text-primary">{instance.name}</span>
        </div>
        <AlertBadge warnings={warnings} criticals={criticals} />
      </div>

      <p className="mt-1 font-mono text-sm text-pgp-text-muted">
        {instance.host}:{instance.port}
      </p>

      <div className="mt-3 flex flex-wrap items-center gap-2">
        {versionStr && (
          <span className="rounded bg-pgp-bg-secondary px-2 py-0.5 text-xs font-medium text-pgp-text-secondary">
            PG {versionStr}
          </span>
        )}
        {isReplica ? (
          <span className="rounded bg-emerald-500/20 px-2 py-0.5 text-xs font-medium text-emerald-400">
            REPLICA
          </span>
        ) : hasMetrics ? (
          <span className="rounded bg-blue-500/20 px-2 py-0.5 text-xs font-medium text-blue-400">
            PRIMARY
          </span>
        ) : null}
      </div>

      {hasMetrics && (
        <div className="mt-3 flex items-center gap-4 text-xs text-pgp-text-secondary">
          <span>Conn: {formatPercent(connUtil, 0)}</span>
          {cacheHit != null && <span>Cache: {formatPercent(cacheHit)}</span>}
          {isReplica && replLag != null && <span>Lag: {Math.round(replLag / 1024)}KB</span>}
        </div>
      )}
    </div>
  )
}
