import { useState } from 'react'
import { useParams, Link } from 'react-router-dom'
import { ArrowLeft, ChevronDown, ChevronRight, Clock, Lightbulb, Zap } from 'lucide-react'
import { PageHeader } from '@/components/ui/PageHeader'
import { Spinner } from '@/components/ui/Spinner'
import { ConfidenceBadge } from '@/components/rca/ConfidenceBadge'
import { ChainSummaryCard } from '@/components/rca/ChainSummaryCard'
import { QualityBanner } from '@/components/rca/QualityBanner'
import { IncidentTimeline } from '@/components/rca/IncidentTimeline'
import { RemediationHooks } from '@/components/rca/RemediationHooks'
import { ReviewWidget } from '@/components/rca/ReviewWidget'
import { PriorityBadge } from '@/components/advisor/PriorityBadge'
import { useRCAIncident } from '@/hooks/useRCA'
import { useRecommendationsByIncident } from '@/hooks/useRecommendationsByIncident'
import { ResolverButton } from '@/components/playbook/ResolverButton'
import { formatTimestamp } from '@/lib/formatters'

export function RCAIncidentDetail() {
  const { serverId, incidentId } = useParams<{ serverId: string; incidentId: string }>()
  const numericId = incidentId ? parseInt(incidentId, 10) : undefined
  const { data: incident, isLoading } = useRCAIncident(serverId, numericId)
  const { data: recommendations } = useRecommendationsByIncident(numericId)

  const [qualityDismissed, setQualityDismissed] = useState(false)
  const [altChainOpen, setAltChainOpen] = useState(false)
  const [metadataOpen, setMetadataOpen] = useState(false)

  if (isLoading) {
    return (
      <div className="flex justify-center py-12">
        <Spinner size="lg" />
      </div>
    )
  }

  if (!incident) {
    return (
      <div className="mx-auto max-w-4xl">
        <PageHeader title="Incident Not Found" subtitle="The requested RCA incident could not be loaded." />
      </div>
    )
  }

  return (
    <div className="mx-auto max-w-4xl">
      {/* Back link */}
      <Link
        to={serverId ? `/servers/${serverId}/rca/incidents` : '/rca/incidents'}
        className="mb-4 inline-flex items-center gap-1 text-sm text-pgp-text-muted transition-colors hover:text-pgp-text-primary"
      >
        <ArrowLeft className="h-4 w-4" />
        Back to incidents
      </Link>

      <PageHeader
        title={`RCA Incident #${incident.id}`}
        actions={
          <div className="flex items-center gap-2">
            <ConfidenceBadge bucket={incident.confidence_bucket} score={incident.confidence} />
            {incident.auto_triggered ? (
              <span className="inline-flex items-center rounded-full bg-blue-500/20 px-2 py-0.5 text-xs font-medium text-blue-400">
                Auto
              </span>
            ) : (
              <span className="inline-flex items-center rounded-full bg-purple-500/20 px-2 py-0.5 text-xs font-medium text-purple-400">
                Manual
              </span>
            )}
          </div>
        }
      />

      <div className="space-y-4">
        {/* Header card */}
        <div className="rounded-lg border border-pgp-border bg-pgp-bg-card p-4">
          <div className="grid grid-cols-2 gap-4 text-sm sm:grid-cols-4">
            <div>
              <p className="text-xs text-pgp-text-muted">Instance</p>
              <Link
                to={`/servers/${incident.instance_id}`}
                className="font-medium text-pgp-accent hover:text-pgp-accent/80"
              >
                {incident.instance_id}
              </Link>
            </div>
            <div>
              <p className="text-xs text-pgp-text-muted">Trigger Metric</p>
              <code className="text-xs font-mono text-pgp-text-primary">{incident.trigger_metric}</code>
            </div>
            <div>
              <p className="text-xs text-pgp-text-muted">Trigger Value</p>
              <span className="font-medium text-pgp-text-primary">{incident.trigger_value.toFixed(2)}</span>
            </div>
            <div>
              <p className="text-xs text-pgp-text-muted">Trigger Time</p>
              <span className="text-pgp-text-secondary">{formatTimestamp(incident.trigger_time)}</span>
            </div>
          </div>
        </div>

        {/* Review widget */}
        <ReviewWidget
          incidentId={incident.id}
          instanceId={incident.instance_id}
          currentStatus={incident.review_status}
          currentComment={incident.review_comment}
        />

        {/* Summary */}
        <ChainSummaryCard
          summary={incident.summary}
          confidence={incident.confidence}
          bucket={incident.confidence_bucket}
        />

        {/* Quality banner */}
        {incident.quality && !qualityDismissed && (
          <QualityBanner quality={incident.quality} onDismiss={() => setQualityDismissed(true)} />
        )}

        {/* Primary chain timeline */}
        <IncidentTimeline events={incident.timeline} primaryChain={incident.primary_chain} />

        {/* Recommended actions */}
        <div className="rounded-lg border border-pgp-border bg-pgp-bg-card p-4">
          <h3 className="mb-3 flex items-center gap-2 text-sm font-medium text-pgp-text-primary">
            <Lightbulb className="h-4 w-4" />
            Recommended Actions
          </h3>
          {recommendations && recommendations.length > 0 ? (
            <div className="space-y-3">
              {recommendations.map((rec) => (
                <div
                  key={rec.id}
                  className="rounded-md border border-pgp-border bg-pgp-bg-secondary p-3"
                >
                  <div className="mb-1 flex items-center gap-2">
                    <PriorityBadge priority={rec.priority} />
                    <span className="text-sm font-medium text-pgp-text-primary">{rec.title}</span>
                  </div>
                  <p className="text-xs text-pgp-text-secondary">{rec.description}</p>
                </div>
              ))}
            </div>
          ) : (
            <p className="text-xs text-pgp-text-muted">
              No automated remediation available for this root cause
            </p>
          )}
        </div>

        {/* Alternative chain (collapsible) */}
        {incident.alternative_chain && (
          <div className="rounded-lg border border-pgp-border bg-pgp-bg-card">
            <button
              onClick={() => setAltChainOpen(!altChainOpen)}
              className="flex w-full items-center justify-between px-4 py-3 text-left"
            >
              <span className="flex items-center gap-2 text-sm font-medium text-pgp-text-primary">
                <Zap className="h-4 w-4" />
                Alternative Chain: {incident.alternative_chain.chain_name}
                <span className="text-xs text-pgp-text-muted">
                  (score: {(incident.alternative_chain.score * 100).toFixed(0)}%)
                </span>
              </span>
              {altChainOpen ? (
                <ChevronDown className="h-4 w-4 text-pgp-text-muted" />
              ) : (
                <ChevronRight className="h-4 w-4 text-pgp-text-muted" />
              )}
            </button>
            {altChainOpen && (
              <div className="border-t border-pgp-border p-4">
                <IncidentTimeline events={incident.alternative_chain.events} />
              </div>
            )}
          </div>
        )}

        {/* Guided remediation playbook */}
        {incident.primary_chain && (
          <div className="rounded-lg border border-pgp-border bg-pgp-bg-card p-4">
            <h3 className="mb-3 flex items-center gap-2 text-sm font-medium text-pgp-text-primary">
              Guided Remediation
            </h3>
            <ResolverButton
              hook={incident.remediation_hooks?.[0]}
              rootCause={incident.primary_chain.root_cause_key}
              instanceId={serverId ?? ''}
              triggerSource="rca"
              triggerId={String(incident.id)}
            />
          </div>
        )}

        {/* Remediation hooks */}
        {incident.remediation_hooks && <RemediationHooks hooks={incident.remediation_hooks} />}

        {/* Analysis metadata (collapsible) */}
        <div className="rounded-lg border border-pgp-border bg-pgp-bg-card">
          <button
            onClick={() => setMetadataOpen(!metadataOpen)}
            className="flex w-full items-center justify-between px-4 py-3 text-left"
          >
            <span className="flex items-center gap-2 text-sm font-medium text-pgp-text-primary">
              <Clock className="h-4 w-4" />
              Analysis Metadata
            </span>
            {metadataOpen ? (
              <ChevronDown className="h-4 w-4 text-pgp-text-muted" />
            ) : (
              <ChevronRight className="h-4 w-4 text-pgp-text-muted" />
            )}
          </button>
          {metadataOpen && (
            <div className="space-y-1 border-t border-pgp-border px-4 py-3 text-sm">
              <div className="flex justify-between">
                <span className="text-pgp-text-muted">Analysis Window</span>
                <span className="text-pgp-text-secondary">
                  {formatTimestamp(incident.analysis_window.from)} &mdash;{' '}
                  {formatTimestamp(incident.analysis_window.to)}
                </span>
              </div>
              <div className="flex justify-between">
                <span className="text-pgp-text-muted">Chain Version</span>
                <span className="font-mono text-xs text-pgp-text-secondary">{incident.chain_version}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-pgp-text-muted">Anomaly Mode</span>
                <span className="text-pgp-text-secondary">{incident.anomaly_mode}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-pgp-text-muted">Created At</span>
                <span className="text-pgp-text-secondary">{formatTimestamp(incident.created_at)}</span>
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
