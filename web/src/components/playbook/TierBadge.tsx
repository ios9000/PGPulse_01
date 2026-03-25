import type { SafetyTier } from '@/types/playbook'

const tierConfig: Record<SafetyTier, { label: string; className: string }> = {
  diagnostic: {
    label: 'Diagnostic',
    className: 'bg-green-500/20 text-green-400',
  },
  remediate: {
    label: 'Remediate',
    className: 'bg-amber-500/20 text-amber-400',
  },
  dangerous: {
    label: 'Dangerous',
    className: 'bg-red-500/20 text-red-400',
  },
  external: {
    label: 'Manual',
    className: 'bg-gray-500/20 text-gray-400',
  },
}

interface TierBadgeProps {
  tier: SafetyTier
}

export function TierBadge({ tier }: TierBadgeProps) {
  const config = tierConfig[tier] ?? tierConfig.diagnostic
  return (
    <span
      className={`inline-flex items-center rounded-full px-2 py-0.5 text-xs font-semibold ${config.className}`}
    >
      {config.label}
    </span>
  )
}
