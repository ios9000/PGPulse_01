import { Link } from 'react-router-dom'
import { Clock, Layers } from 'lucide-react'
import { TierBadge } from '@/components/playbook/TierBadge'
import type { Playbook, SafetyTier } from '@/types/playbook'

function statusBadgeClass(status: string): string {
  if (status === 'stable') return 'bg-green-500/20 text-green-400'
  if (status === 'draft') return 'bg-amber-500/20 text-amber-400'
  return 'bg-gray-500/20 text-gray-400'
}

interface PlaybookCardProps {
  playbook: Playbook
}

export function PlaybookCard({ playbook }: PlaybookCardProps) {
  const uniqueTiers = Array.from(
    new Set((playbook.steps ?? []).map((s) => s.safety_tier)),
  ) as SafetyTier[]

  return (
    <Link
      to={`/playbooks/${playbook.id}`}
      className="block rounded-lg border border-pgp-border bg-pgp-bg-card p-4 transition-colors hover:bg-pgp-bg-hover"
    >
      <div className="mb-2 flex items-start justify-between gap-2">
        <h3 className="text-sm font-medium text-pgp-text-primary line-clamp-1">
          {playbook.name}
        </h3>
        <span
          className={`shrink-0 rounded-full px-2 py-0.5 text-[10px] font-semibold uppercase ${statusBadgeClass(playbook.status)}`}
        >
          {playbook.status}
        </span>
      </div>

      <p className="mb-3 text-xs text-pgp-text-secondary line-clamp-2">
        {playbook.description}
      </p>

      <div className="flex flex-wrap items-center gap-2">
        <span className="rounded-md bg-pgp-bg-secondary px-2 py-0.5 text-[10px] font-medium text-pgp-text-muted">
          {playbook.category}
        </span>

        {uniqueTiers.map((tier) => (
          <TierBadge key={tier} tier={tier} />
        ))}
      </div>

      <div className="mt-3 flex items-center gap-4 text-[10px] text-pgp-text-muted">
        <span className="flex items-center gap-1">
          <Layers className="h-3 w-3" />
          {(playbook.steps ?? []).length} steps
        </span>
        {playbook.estimated_duration && (
          <span className="flex items-center gap-1">
            <Clock className="h-3 w-3" />
            {playbook.estimated_duration}
          </span>
        )}
        <span>v{playbook.version}</span>
      </div>
    </Link>
  )
}
