import { useState } from 'react'
import { ChevronDown, ChevronRight, Check, Bell } from 'lucide-react'
import { PriorityBadge } from '@/components/advisor/PriorityBadge'
import { RCABadge } from '@/components/advisor/RCABadge'
import { RuleFormModal } from '@/components/alerts/RuleFormModal'
import { formatTimestamp } from '@/lib/formatters'
import { useAcknowledge } from '@/hooks/useRecommendations'
import { useAuth } from '@/hooks/useAuth'
import { toast } from '@/stores/toastStore'
import type { Recommendation, RecommendationPriority, AlertRule } from '@/types/models'

function priorityToSeverity(priority: RecommendationPriority): AlertRule['severity'] {
  switch (priority) {
    case 'action_required':
      return 'critical'
    case 'suggestion':
      return 'warning'
    case 'info':
      return 'info'
  }
}

interface AdvisorRowProps {
  rec: Recommendation
}

function priorityBorderClass(priority: RecommendationPriority): string {
  if (priority === 'action_required') return 'border-l-4 border-l-red-500'
  if (priority === 'suggestion') return 'border-l-4 border-l-amber-500'
  return 'border-l-4 border-l-blue-500'
}

export function AdvisorRow({ rec }: AdvisorRowProps) {
  const [expanded, setExpanded] = useState(false)
  const [showRuleModal, setShowRuleModal] = useState(false)
  const ack = useAcknowledge()
  const { can } = useAuth()

  const handleAcknowledge = (e: React.MouseEvent) => {
    e.stopPropagation()
    ack.mutate(rec.id, {
      onSuccess: () => toast.success('Recommendation acknowledged'),
      onError: () => toast.error('Failed to acknowledge'),
    })
  }

  return (
    <>
      <tr
        onClick={() => setExpanded(!expanded)}
        className={`cursor-pointer border-b border-pgp-border transition-colors hover:bg-pgp-bg-hover ${priorityBorderClass(rec.priority)}`}
      >
        <td className="px-4 py-3">
          {expanded ? (
            <ChevronDown className="h-4 w-4 text-pgp-text-muted" />
          ) : (
            <ChevronRight className="h-4 w-4 text-pgp-text-muted" />
          )}
        </td>
        <td className="px-4 py-3">
          <PriorityBadge priority={rec.priority} />
        </td>
        <td className="px-4 py-3 text-sm font-medium text-pgp-text-primary">{rec.title}</td>
        <td className="px-4 py-3 text-sm text-pgp-text-secondary">{rec.category}</td>
        <td className="px-4 py-3 text-sm text-pgp-text-secondary">{rec.instance_id}</td>
        <td className="px-4 py-3 text-sm text-pgp-text-muted">{formatTimestamp(rec.created_at)}</td>
        <td className="px-4 py-3">
          {rec.acknowledged_at ? (
            <span className="inline-flex items-center gap-1 rounded-full bg-green-500/20 px-2 py-0.5 text-xs font-medium text-green-400">
              <Check className="h-3 w-3" /> Ack
            </span>
          ) : (
            <button
              onClick={handleAcknowledge}
              disabled={ack.isPending}
              className="rounded-md border border-pgp-border bg-pgp-bg-secondary px-2 py-1 text-xs text-pgp-text-secondary hover:bg-pgp-bg-hover hover:text-pgp-text-primary disabled:opacity-50"
            >
              Acknowledge
            </button>
          )}
        </td>
      </tr>
      {expanded && (
        <tr className="border-b border-pgp-border bg-pgp-bg-secondary">
          <td colSpan={7} className="px-8 py-4">
            <div className="space-y-2 text-sm">
              <p className="text-pgp-text-primary">{rec.description}</p>
              <div className="flex flex-wrap items-center gap-4 text-xs text-pgp-text-muted">
                {rec.incident_ids?.length > 0 && (
                  <RCABadge incidentIds={rec.incident_ids} lastIncidentAt={rec.last_incident_at} />
                )}
                <span>Metric: {rec.metric_key} = {rec.metric_value.toFixed(2)}</span>
                <span>Rule: {rec.rule_id}</span>
                {rec.doc_url && (
                  <a
                    href={rec.doc_url}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="text-blue-400 hover:text-blue-300"
                    onClick={(e) => e.stopPropagation()}
                  >
                    Documentation
                  </a>
                )}
                {can('alert_management') && (
                  <button
                    onClick={(e) => {
                      e.stopPropagation()
                      setShowRuleModal(true)
                    }}
                    className="inline-flex items-center gap-1 rounded-md border border-pgp-border bg-pgp-bg-primary px-2 py-1 text-xs text-pgp-text-secondary hover:bg-pgp-bg-hover hover:text-pgp-text-primary"
                  >
                    <Bell className="h-3 w-3" />
                    Create Alert Rule
                  </button>
                )}
              </div>
            </div>
          </td>
        </tr>
      )}
      {showRuleModal && (
        <RuleFormModal
          onClose={() => {
            setShowRuleModal(false)
            toast.success('Alert rule created from recommendation')
          }}
          defaults={{
            name: `Auto: ${rec.title}`,
            description: rec.description,
            metric: rec.metric_key,
            operator: '>',
            threshold: rec.metric_value,
            severity: priorityToSeverity(rec.priority),
          }}
          availableChannels={[]}
        />
      )}
    </>
  )
}
