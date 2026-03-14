import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { ChevronDown, ChevronRight } from 'lucide-react'
import { StatusBadge } from '@/components/ui/StatusBadge'
import { PriorityBadge } from '@/components/advisor/PriorityBadge'
import { formatTimestamp, formatDuration } from '@/lib/formatters'
import type { AlertEvent } from '@/types/models'

interface AlertRowProps {
  alert: AlertEvent
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

export function AlertRow({ alert }: AlertRowProps) {
  const navigate = useNavigate()
  const [expanded, setExpanded] = useState(false)
  const isResolved = !!alert.resolved_at
  const hasRecs = !!alert.recommendations?.length

  const handleRowClick = () => {
    if (hasRecs) {
      setExpanded(!expanded)
    } else {
      navigate(`/servers/${alert.instance_id}`)
    }
  }

  return (
    <>
      <tr
        onClick={handleRowClick}
        className="cursor-pointer border-b border-pgp-border transition-colors hover:bg-pgp-bg-hover"
      >
        <td className="w-8 px-4 py-3">
          {hasRecs ? (
            expanded ? (
              <ChevronDown className="h-4 w-4 text-pgp-text-muted" />
            ) : (
              <ChevronRight className="h-4 w-4 text-pgp-text-muted" />
            )
          ) : null}
        </td>
        <td className="px-4 py-3">
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
        <td className="px-4 py-3 text-sm text-pgp-text-muted">{formatTimestamp(alert.fired_at)}</td>
        <td className="px-4 py-3 text-sm text-pgp-text-muted">{alertDuration(alert)}</td>
      </tr>
      {expanded && alert.recommendations && (
        <tr className="border-b border-pgp-border bg-pgp-bg-secondary">
          <td colSpan={9} className="px-8 py-4">
            <p className="mb-2 text-xs font-medium uppercase text-pgp-text-muted">Recommendations</p>
            <div className="space-y-2">
              {alert.recommendations.map((rec) => (
                <div
                  key={rec.id}
                  className="flex items-start gap-3 rounded-md border border-pgp-border bg-pgp-bg-card p-3"
                >
                  <PriorityBadge priority={rec.priority} />
                  <div className="flex-1">
                    <p className="text-sm font-medium text-pgp-text-primary">{rec.title}</p>
                    <p className="mt-0.5 text-xs text-pgp-text-muted">{rec.description}</p>
                  </div>
                </div>
              ))}
            </div>
          </td>
        </tr>
      )}
    </>
  )
}
