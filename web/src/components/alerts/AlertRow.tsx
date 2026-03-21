import { useNavigate } from 'react-router-dom'
import { Search, Loader2 } from 'lucide-react'
import { StatusBadge } from '@/components/ui/StatusBadge'
import { formatTimestamp, formatDuration } from '@/lib/formatters'
import { useRCAAnalyze } from '@/hooks/useRCA'
import type { AlertEvent } from '@/types/models'

interface AlertRowProps {
  alert: AlertEvent
  onClick?: (alert: AlertEvent) => void
}

function alertDuration(alert: AlertEvent): string {
  const start = new Date(alert.fired_at).getTime()
  const end = alert.resolved_at ? new Date(alert.resolved_at).getTime() : Date.now()
  const seconds = Math.max(0, Math.floor((end - start) / 1000))
  return formatDuration(seconds)
}

function severityStatus(severity: string): 'ok' | 'warning' | 'critical' | 'info' {
  if (severity === 'critical') return 'critical'
  if (severity === 'warning') return 'warning'
  return 'info'
}

function severityBorderClass(severity: string): string {
  if (severity === 'critical') return 'border-l-4 border-l-red-500'
  if (severity === 'warning') return 'border-l-4 border-l-amber-500'
  return 'border-l-4 border-l-blue-500'
}

export function AlertRow({ alert, onClick }: AlertRowProps) {
  const navigate = useNavigate()
  const analyzeMutation = useRCAAnalyze()
  const isResolved = !!alert.resolved_at

  const handleInvestigate = async (e: React.MouseEvent) => {
    e.stopPropagation()
    const result = await analyzeMutation.mutateAsync({
      instanceId: alert.instance_id,
      metric: alert.metric,
      value: alert.value,
    })
    navigate(`/servers/${alert.instance_id}/rca/incidents/${result.id}`)
  }

  return (
    <tr
      onClick={() => onClick?.(alert)}
      className={`cursor-pointer border-b border-pgp-border transition-colors hover:bg-pgp-bg-hover ${severityBorderClass(alert.severity)}`}
    >
      <td className="py-3 pl-4 pr-2">
        <StatusBadge
          status={severityStatus(alert.severity)}
          label={alert.severity}
          pulse={alert.severity === 'critical' && !isResolved}
        />
      </td>
      <td className="px-4 py-3 text-sm font-medium text-pgp-text-primary">{alert.rule_name}</td>
      <td className="px-4 py-3 text-sm text-pgp-text-secondary">{alert.instance_id}</td>
      <td className="px-4 py-3 text-sm text-pgp-text-secondary">{alert.metric}</td>
      <td className="px-4 py-3 text-sm text-pgp-text-secondary">
        {alert.value.toFixed(2)} {alert.operator} {alert.threshold}
      </td>
      <td className="px-4 py-3">
        {isResolved ? (
          <span className="inline-flex items-center rounded-full bg-green-500/20 px-2 py-0.5 text-xs font-medium text-green-400">
            Resolved
          </span>
        ) : (
          <span className="inline-flex items-center rounded-full bg-red-500/20 px-2 py-0.5 text-xs font-medium text-red-400">
            Firing
          </span>
        )}
      </td>
      <td className="px-4 py-3 text-right text-sm text-pgp-text-muted">
        {formatTimestamp(alert.fired_at)}
      </td>
      <td className="px-4 py-3 text-right text-sm text-pgp-text-muted">
        {alertDuration(alert)}
      </td>
      <td className="px-4 py-3 text-right">
        <button
          onClick={handleInvestigate}
          disabled={analyzeMutation.isPending}
          className="inline-flex items-center justify-center rounded-md p-1.5 text-pgp-text-muted transition-colors hover:bg-pgp-bg-hover hover:text-pgp-text-primary disabled:cursor-not-allowed disabled:opacity-50"
          title="Investigate root cause"
        >
          {analyzeMutation.isPending ? (
            <Loader2 className="h-4 w-4 animate-spin" />
          ) : (
            <Search className="h-4 w-4" />
          )}
        </button>
      </td>
    </tr>
  )
}
