import type { Verdict } from '@/types/playbook'

const verdictConfig: Record<Verdict, { className: string }> = {
  green: { className: 'bg-green-500/20 text-green-400' },
  yellow: { className: 'bg-amber-500/20 text-amber-400' },
  red: { className: 'bg-red-500/20 text-red-400' },
}

interface VerdictBadgeProps {
  verdict: Verdict
  message?: string
}

export function VerdictBadge({ verdict, message }: VerdictBadgeProps) {
  const config = verdictConfig[verdict] ?? verdictConfig.yellow
  const label = verdict.charAt(0).toUpperCase() + verdict.slice(1)
  return (
    <span
      className={`inline-flex items-center gap-1.5 rounded-full px-2.5 py-0.5 text-xs font-semibold ${config.className}`}
    >
      <span className="h-1.5 w-1.5 rounded-full bg-current" />
      {message ?? label}
    </span>
  )
}
