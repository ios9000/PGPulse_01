import { useEffect, useRef } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { X, ExternalLink, AlertTriangle, AlertCircle, Info, Search, Loader2 } from 'lucide-react'
import { formatTimestamp } from '@/lib/formatters'
import { getMetricDescription, getMetricPageLink } from '@/lib/metricDescriptions'
import { PriorityBadge } from '@/components/advisor/PriorityBadge'
import { ConfidenceBadge } from '@/components/rca/ConfidenceBadge'
import { useInstanceRCAIncidents, useRCAAnalyze } from '@/hooks/useRCA'
import type { AlertEvent, AlertRule } from '@/types/models'

interface AlertDetailPanelProps {
  alert: AlertEvent
  rules: AlertRule[]
  onClose: () => void
}

function severityIcon(severity: string) {
  if (severity === 'critical')
    return <AlertCircle className="h-5 w-5 text-red-400" />
  if (severity === 'warning')
    return <AlertTriangle className="h-5 w-5 text-amber-400" />
  return <Info className="h-5 w-5 text-blue-400" />
}

function severityBorderColor(severity: string): string {
  if (severity === 'critical') return 'border-l-red-500'
  if (severity === 'warning') return 'border-l-amber-500'
  return 'border-l-blue-500'
}

function severityBadgeBg(severity: string): string {
  if (severity === 'critical') return 'bg-red-500/20 text-red-400'
  if (severity === 'warning') return 'bg-amber-500/20 text-amber-400'
  return 'bg-blue-500/20 text-blue-400'
}

function operatorLabel(op: string): string {
  const map: Record<string, string> = {
    '>': 'greater than',
    '>=': 'greater than or equal to',
    '<': 'less than',
    '<=': 'less than or equal to',
    '==': 'equal to',
    '!=': 'not equal to',
  }
  return map[op] ?? op
}

export function AlertDetailPanel({ alert, rules, onClose }: AlertDetailPanelProps) {
  const panelRef = useRef<HTMLDivElement>(null)
  const navigate = useNavigate()
  const metricDesc = getMetricDescription(alert.metric)
  const rule = rules.find((r) => r.id === alert.rule_id)
  const pageLink = getMetricPageLink(alert.metric, alert.instance_id)

  const { data: rcaData } = useInstanceRCAIncidents(alert.instance_id, { limit: 3 })
  const analyzeMutation = useRCAAnalyze()

  const matchingIncidents = (rcaData?.incidents ?? []).filter(
    (i) => i.trigger_metric === alert.metric,
  )

  // Close on click outside
  useEffect(() => {
    function handleClick(e: MouseEvent) {
      if (panelRef.current && !panelRef.current.contains(e.target as Node)) {
        onClose()
      }
    }
    function handleKey(e: KeyboardEvent) {
      if (e.key === 'Escape') onClose()
    }
    document.addEventListener('mousedown', handleClick)
    document.addEventListener('keydown', handleKey)
    return () => {
      document.removeEventListener('mousedown', handleClick)
      document.removeEventListener('keydown', handleKey)
    }
  }, [onClose])

  const isResolved = !!alert.resolved_at

  return (
    <div className="fixed inset-0 z-50 flex justify-end bg-black/40">
      <div
        ref={panelRef}
        className={`h-full w-full max-w-lg overflow-y-auto border-l-4 bg-pgp-bg-card shadow-xl ${severityBorderColor(alert.severity)}`}
      >
        {/* Header */}
        <div className="flex items-start justify-between border-b border-pgp-border px-6 py-4">
          <div className="flex items-start gap-3">
            {severityIcon(alert.severity)}
            <div>
              <div className="flex items-center gap-2">
                <span
                  className={`inline-flex items-center rounded-full px-2 py-0.5 text-xs font-semibold uppercase ${severityBadgeBg(alert.severity)}`}
                >
                  {alert.severity}
                </span>
                {isResolved ? (
                  <span className="inline-flex items-center rounded-full bg-green-500/20 px-2 py-0.5 text-xs font-medium text-green-400">
                    Resolved
                  </span>
                ) : (
                  <span className="inline-flex items-center rounded-full bg-red-500/20 px-2 py-0.5 text-xs font-medium text-red-400">
                    Firing
                  </span>
                )}
              </div>
              <h2 className="mt-1 text-lg font-semibold text-pgp-text-primary">
                {metricDesc?.name ?? alert.metric}
              </h2>
              <p className="text-sm text-pgp-text-muted">{alert.rule_name}</p>
            </div>
          </div>
          <button
            onClick={onClose}
            className="rounded p-1 text-pgp-text-muted transition-colors hover:bg-pgp-bg-hover hover:text-pgp-text-primary"
          >
            <X className="h-5 w-5" />
          </button>
        </div>

        {/* Alert info */}
        <div className="space-y-1 border-b border-pgp-border px-6 py-4">
          <div className="flex justify-between text-sm">
            <span className="text-pgp-text-muted">Instance</span>
            <span className="font-medium text-pgp-text-primary">{alert.instance_id}</span>
          </div>
          <div className="flex justify-between text-sm">
            <span className="text-pgp-text-muted">Fired at</span>
            <span className="text-pgp-text-secondary">{formatTimestamp(alert.fired_at)}</span>
          </div>
          {alert.resolved_at && (
            <div className="flex justify-between text-sm">
              <span className="text-pgp-text-muted">Resolved at</span>
              <span className="text-pgp-text-secondary">{formatTimestamp(alert.resolved_at)}</span>
            </div>
          )}
        </div>

        {/* Metric Details */}
        <div className="border-b border-pgp-border px-6 py-4">
          <h3 className="mb-3 text-xs font-semibold uppercase tracking-wider text-pgp-text-muted">
            Metric Details
          </h3>
          <div className="space-y-2">
            <div className="flex justify-between text-sm">
              <span className="text-pgp-text-muted">Metric key</span>
              <code className="rounded bg-pgp-bg-secondary px-2 py-0.5 font-mono text-xs text-pgp-text-secondary">
                {alert.metric}
              </code>
            </div>
            <div className="flex justify-between text-sm">
              <span className="text-pgp-text-muted">Current value</span>
              <span className="font-mono font-medium text-pgp-text-primary">
                {alert.value.toFixed(2)}
              </span>
            </div>
            <div className="flex justify-between text-sm">
              <span className="text-pgp-text-muted">Threshold</span>
              <span className="font-mono text-pgp-text-secondary">
                {operatorLabel(alert.operator)} {alert.threshold}
              </span>
            </div>
          </div>

          {metricDesc && (
            <div className="mt-4 space-y-3">
              <div>
                <p className="mb-1 text-xs font-medium text-pgp-text-muted">What is this?</p>
                <p className="text-sm leading-relaxed text-pgp-text-secondary">
                  {metricDesc.description}
                </p>
              </div>
              <div>
                <p className="mb-1 text-xs font-medium text-pgp-text-muted">Why it matters</p>
                <p className="text-sm leading-relaxed text-pgp-text-secondary">
                  {metricDesc.significance}
                </p>
              </div>
            </div>
          )}

          {rule?.description && (
            <div className="mt-3">
              <p className="mb-1 text-xs font-medium text-pgp-text-muted">Rule description</p>
              <p className="text-sm leading-relaxed text-pgp-text-secondary">
                {rule.description}
              </p>
            </div>
          )}
        </div>

        {/* Recommendations */}
        {alert.recommendations && alert.recommendations.length > 0 && (
          <div className="border-b border-pgp-border px-6 py-4">
            <h3 className="mb-3 text-xs font-semibold uppercase tracking-wider text-pgp-text-muted">
              Recommendations
            </h3>
            <div className="space-y-2">
              {alert.recommendations.map((rec) => (
                <div
                  key={rec.id}
                  className="flex items-start gap-3 rounded-md border border-pgp-border bg-pgp-bg-secondary p-3"
                >
                  <PriorityBadge priority={rec.priority} />
                  <div className="flex-1">
                    <p className="text-sm font-medium text-pgp-text-primary">{rec.title}</p>
                    <p className="mt-0.5 text-xs text-pgp-text-muted">{rec.description}</p>
                  </div>
                </div>
              ))}
              <Link
                to={`/advisor?instance_id=${alert.instance_id}`}
                className="inline-flex items-center gap-1 text-sm text-pgp-accent hover:text-pgp-accent/80"
              >
                View all recommendations
                <ExternalLink className="h-3.5 w-3.5" />
              </Link>
            </div>
          </div>
        )}

        {/* Root Cause Analysis */}
        <div className="border-b border-pgp-border px-6 py-4">
          <h3 className="mb-3 text-xs font-semibold uppercase tracking-wider text-pgp-text-muted">
            Root Cause Analysis
          </h3>
          {matchingIncidents.length > 0 ? (
            <div className="space-y-2">
              {matchingIncidents.map((incident) => (
                <div
                  key={incident.id}
                  className="flex items-start justify-between gap-3 rounded-md border border-pgp-border bg-pgp-bg-secondary p-3"
                >
                  <div className="flex-1">
                    <p className="text-sm text-pgp-text-primary">{incident.summary}</p>
                  </div>
                  <ConfidenceBadge bucket={incident.confidence_bucket} score={incident.confidence} />
                </div>
              ))}
              <Link
                to={`/servers/${alert.instance_id}/rca/incidents/${matchingIncidents[0].id}`}
                className="inline-flex items-center gap-1 text-sm text-pgp-accent hover:text-pgp-accent/80"
              >
                View full analysis
                <ExternalLink className="h-3.5 w-3.5" />
              </Link>
            </div>
          ) : (
            <button
              onClick={async () => {
                const result = await analyzeMutation.mutateAsync({
                  instanceId: alert.instance_id,
                  metric: alert.metric,
                  value: alert.value,
                })
                navigate(`/servers/${alert.instance_id}/rca/incidents/${result.id}`)
              }}
              disabled={analyzeMutation.isPending}
              className="inline-flex items-center gap-2 rounded-md border border-pgp-border px-3 py-2 text-sm text-pgp-text-secondary transition-colors hover:bg-pgp-bg-hover hover:text-pgp-text-primary disabled:cursor-not-allowed disabled:opacity-50"
            >
              {analyzeMutation.isPending ? (
                <Loader2 className="h-4 w-4 animate-spin" />
              ) : (
                <Search className="h-4 w-4" />
              )}
              {analyzeMutation.isPending ? 'Analyzing...' : 'Investigate Root Cause'}
            </button>
          )}
        </div>

        {/* Quick Links */}
        <div className="px-6 py-4">
          <h3 className="mb-3 text-xs font-semibold uppercase tracking-wider text-pgp-text-muted">
            Quick Links
          </h3>
          <div className="flex flex-col gap-2">
            <Link
              to={`/servers/${alert.instance_id}`}
              className="inline-flex items-center gap-2 rounded-md border border-pgp-border px-3 py-2 text-sm text-pgp-text-secondary transition-colors hover:bg-pgp-bg-hover hover:text-pgp-text-primary"
            >
              <ExternalLink className="h-4 w-4" />
              View Server Dashboard
            </Link>
            {pageLink && (
              <Link
                to={pageLink}
                className="inline-flex items-center gap-2 rounded-md border border-pgp-border px-3 py-2 text-sm text-pgp-text-secondary transition-colors hover:bg-pgp-bg-hover hover:text-pgp-text-primary"
              >
                <ExternalLink className="h-4 w-4" />
                View Query Insights
              </Link>
            )}
            <Link
              to="/alerts?view=rules"
              className="inline-flex items-center gap-2 rounded-md border border-pgp-border px-3 py-2 text-sm text-pgp-text-secondary transition-colors hover:bg-pgp-bg-hover hover:text-pgp-text-primary"
            >
              <ExternalLink className="h-4 w-4" />
              View Alert Rules
            </Link>
          </div>
        </div>
      </div>
    </div>
  )
}
