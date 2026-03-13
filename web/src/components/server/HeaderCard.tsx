import { StatusBadge } from '@/components/ui/StatusBadge'
import { formatPGVersion, formatUptime } from '@/lib/formatters'
import type { CurrentMetricsResult } from '@/types/models'

interface HeaderCardProps {
  instanceName: string
  host: string
  port: number
  currentMetrics: CurrentMetricsResult | undefined
}

export function HeaderCard({ instanceName, host, port, currentMetrics }: HeaderCardProps) {
  const m = currentMetrics?.metrics ?? {}

  const versionNum = m['pg.server.version_num']?.value
  const versionStr = versionNum ? formatPGVersion(versionNum) : null
  const isReplica = m['pg.server.is_in_recovery']?.value === 1
  const uptimeSeconds = m['pg.server.uptime_seconds']?.value
  const hasData = Object.keys(m).length > 0

  return (
    <div className="rounded-lg border border-pgp-border bg-pgp-bg-card p-6">
      <div className="flex flex-wrap items-center gap-4">
        <StatusBadge status={hasData ? 'ok' : 'unknown'} size="md" />
        <h1 className="text-2xl font-bold text-pgp-text-primary">{instanceName}</h1>
        <span className="font-mono text-sm text-pgp-text-muted">{host}:{port}</span>

        <div className="flex flex-wrap items-center gap-2">
          {versionStr && (
            <span className="rounded bg-pgp-bg-secondary px-2 py-0.5 text-xs font-medium text-pgp-text-secondary">
              PG {versionStr}
            </span>
          )}
          {isReplica ? (
            <span className="rounded bg-emerald-500/20 px-2 py-0.5 text-xs font-medium text-emerald-400">
              REPLICA
            </span>
          ) : hasData ? (
            <span className="rounded bg-blue-500/20 px-2 py-0.5 text-xs font-medium text-blue-400">
              PRIMARY
            </span>
          ) : null}
        </div>

        {uptimeSeconds != null && (
          <span className="text-sm text-pgp-text-muted">
            Uptime: {formatUptime(uptimeSeconds)}
          </span>
        )}
      </div>
    </div>
  )
}
