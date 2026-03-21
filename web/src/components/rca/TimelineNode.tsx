import type { RCATimelineEvent } from '@/types/rca'

interface TimelineNodeProps {
  event: RCATimelineEvent
  isFirst: boolean
  isLast: boolean
}

function roleIndicator(role: string) {
  if (role === 'root_cause') {
    return <span className="inline-flex h-4 w-4 items-center justify-center rounded-full bg-red-500" />
  }
  if (role === 'intermediate') {
    return (
      <span className="inline-flex h-4 w-4 items-center justify-center rounded-full border-2 border-amber-400" />
    )
  }
  // symptom
  return (
    <span className="relative inline-flex h-4 w-4 items-center justify-center rounded-full bg-blue-500">
      <span className="h-1.5 w-1.5 rounded-full bg-pgp-bg-card" />
    </span>
  )
}

function layerColor(layer: string): string {
  if (layer === 'db') return 'bg-blue-400/20 text-blue-400'
  if (layer === 'os') return 'bg-green-400/20 text-green-400'
  if (layer === 'workload') return 'bg-purple-400/20 text-purple-400'
  if (layer === 'config') return 'bg-orange-400/20 text-orange-400'
  return 'bg-gray-400/20 text-gray-400'
}

function roleLabel(role: string): string {
  return role.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase())
}

export function TimelineNode({ event, isLast }: TimelineNodeProps) {
  return (
    <div className="relative flex gap-4">
      {/* Vertical line + indicator */}
      <div className="flex flex-col items-center">
        {roleIndicator(event.role)}
        {!isLast && <div className="w-px flex-1 bg-pgp-border" />}
      </div>

      {/* Content */}
      <div className="flex-1 pb-6">
        <div className="flex flex-wrap items-center gap-2">
          <span className="text-sm font-semibold text-pgp-text-primary">{event.node_name}</span>
          <span className={`rounded px-1.5 py-0.5 text-[10px] font-medium ${layerColor(event.layer)}`}>
            {event.layer}
          </span>
          <span className="text-xs text-pgp-text-muted">{roleLabel(event.role)}</span>
        </div>

        <code className="mt-1 block text-xs font-mono text-pgp-text-muted">{event.metric_key}</code>

        <div className="mt-2 flex flex-wrap items-center gap-4 text-sm">
          <span className="text-pgp-text-primary">
            <span className="font-medium">{event.value.toFixed(2)}</span>
            <span className="text-pgp-text-muted"> (baseline: {event.baseline_val.toFixed(2)})</span>
          </span>
          {event.z_score > 0 && (
            <span className="text-xs text-pgp-text-muted">
              z-score: <span className="font-mono">{event.z_score.toFixed(1)}</span>
            </span>
          )}
        </div>

        {/* Strength bar */}
        <div className="mt-2 h-1.5 w-full max-w-xs overflow-hidden rounded-full bg-pgp-bg-secondary">
          <div
            className="h-full rounded-full bg-pgp-accent transition-all"
            style={{ width: `${Math.min(event.strength * 100, 100)}%` }}
          />
        </div>

        <div className="mt-2 flex flex-wrap items-center gap-3 text-xs text-pgp-text-muted">
          <span>{event.evidence === 'required' ? 'Required' : 'Supporting'} evidence</span>
          <span>{new Date(event.timestamp).toLocaleTimeString()}</span>
        </div>

        {event.description && (
          <p className="mt-1 text-xs leading-relaxed text-pgp-text-secondary">{event.description}</p>
        )}
      </div>
    </div>
  )
}
