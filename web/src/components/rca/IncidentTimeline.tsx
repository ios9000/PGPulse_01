import { GitBranch } from 'lucide-react'
import { EmptyState } from '@/components/ui/EmptyState'
import { TimelineNode } from '@/components/rca/TimelineNode'
import { TimelineEdge } from '@/components/rca/TimelineEdge'
import type { RCATimelineEvent, RCACausalChainResult } from '@/types/rca'

interface IncidentTimelineProps {
  events?: RCATimelineEvent[]
  primaryChain?: RCACausalChainResult
}

export function IncidentTimeline({ events, primaryChain }: IncidentTimelineProps) {
  // Prefer primary chain events, fall back to top-level events
  const timelineEvents = primaryChain?.events ?? events ?? []

  if (timelineEvents.length === 0) {
    return (
      <EmptyState
        icon={GitBranch}
        title="No causal chain identified"
        description="Insufficient data to build a causal chain for this incident."
      />
    )
  }

  // Sort ascending by timestamp (root cause first, symptom last)
  const sorted = [...timelineEvents].sort(
    (a, b) => new Date(a.timestamp).getTime() - new Date(b.timestamp).getTime(),
  )

  return (
    <div className="rounded-lg border border-pgp-border bg-pgp-bg-card p-4">
      <h3 className="mb-4 text-xs font-semibold uppercase tracking-wider text-pgp-text-muted">
        Causal Chain
      </h3>
      <div>
        {sorted.map((event, idx) => {
          const isFirst = idx === 0
          const isLast = idx === sorted.length - 1
          const nextEvent = sorted[idx + 1]

          return (
            <div key={`${event.node_id}-${event.timestamp}`}>
              <TimelineNode event={event} isFirst={isFirst} isLast={isLast} />
              {!isLast && nextEvent && event.edge_desc && (
                <TimelineEdge description={event.edge_desc} />
              )}
            </div>
          )
        })}
      </div>
    </div>
  )
}
