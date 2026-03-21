import { formatTimestamp } from '@/lib/formatters'
import { ConfidenceBadge } from '@/components/rca/ConfidenceBadge'
import type { RCAIncident } from '@/types/rca'

interface IncidentRowProps {
  incident: RCAIncident
  onClick: () => void
}

function truncate(text: string, max: number): string {
  if (text.length <= max) return text
  return text.slice(0, max) + '...'
}

export function IncidentRow({ incident, onClick }: IncidentRowProps) {
  return (
    <tr
      onClick={onClick}
      className="cursor-pointer border-b border-pgp-border transition-colors hover:bg-pgp-bg-hover"
    >
      <td className="px-4 py-3 text-sm text-pgp-text-muted">
        {formatTimestamp(incident.trigger_time)}
      </td>
      <td className="px-4 py-3 text-sm text-pgp-text-secondary">{incident.instance_id}</td>
      <td className="px-4 py-3">
        <code className="rounded bg-pgp-bg-secondary px-1.5 py-0.5 text-xs font-mono text-pgp-text-secondary">
          {truncate(incident.trigger_metric, 30)}
        </code>
      </td>
      <td className="px-4 py-3 text-sm text-pgp-text-secondary">
        {truncate(incident.summary, 60)}
      </td>
      <td className="px-4 py-3">
        <ConfidenceBadge bucket={incident.confidence_bucket} score={incident.confidence} />
      </td>
      <td className="px-4 py-3">
        {incident.auto_triggered ? (
          <span className="inline-flex items-center rounded-full bg-blue-500/20 px-2 py-0.5 text-xs font-medium text-blue-400">
            Auto
          </span>
        ) : (
          <span className="inline-flex items-center rounded-full bg-purple-500/20 px-2 py-0.5 text-xs font-medium text-purple-400">
            Manual
          </span>
        )}
      </td>
    </tr>
  )
}
