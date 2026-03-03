import type { StatementsConfig } from '@/types/models'
import { formatDuration } from '@/lib/formatters'

interface StatementsConfigBarProps {
  config: StatementsConfig
}

function Badge({ label, value, variant = 'normal' }: {
  label: string
  value: string
  variant?: 'normal' | 'warning' | 'critical'
}) {
  const colors = {
    normal: 'bg-pgp-bg-secondary text-pgp-text-secondary',
    warning: 'bg-yellow-500/20 text-yellow-400',
    critical: 'bg-red-500/20 text-red-400',
  }

  return (
    <span className={`inline-flex items-center gap-1 rounded-full px-2.5 py-0.5 text-xs font-medium ${colors[variant]}`}>
      <span className="text-pgp-text-muted">{label}:</span> {value}
    </span>
  )
}

export function StatementsConfigBar({ config }: StatementsConfigBarProps) {
  const fillVariant = config.fill_pct >= 95 ? 'critical' : config.fill_pct >= 80 ? 'warning' : 'normal'
  const ioVariant = config.io_timing ? 'normal' : 'warning'
  const resetVariant =
    config.stats_reset_age_seconds != null && config.stats_reset_age_seconds >= 86400
      ? 'warning'
      : 'normal'

  const resetLabel = config.stats_reset_age_seconds != null
    ? formatDuration(config.stats_reset_age_seconds)
    : 'never'

  return (
    <div className="mb-3 flex flex-wrap items-center gap-2">
      <Badge label="Statements" value={`${config.current_count} / ${config.max}`} variant={fillVariant} />
      <Badge label="Fill" value={`${config.fill_pct.toFixed(0)}%`} variant={fillVariant} />
      <Badge label="Track" value={config.track} />
      <Badge label="IO Timing" value={config.io_timing ? 'ON' : 'OFF'} variant={ioVariant} />
      <Badge label="Reset Age" value={resetLabel} variant={resetVariant} />
    </div>
  )
}
