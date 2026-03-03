import { TrendingUp, TrendingDown, Minus } from 'lucide-react'
import { STATUS_BG_COLORS } from '@/lib/constants'

interface MetricCardProps {
  label: string
  value: string | number
  unit?: string
  trend?: 'up' | 'down' | 'flat'
  trendValue?: string
  status?: 'ok' | 'warning' | 'critical'
}

const trendIcons = {
  up: TrendingUp,
  down: TrendingDown,
  flat: Minus,
}

const trendColors = {
  up: 'text-pgp-ok',
  down: 'text-pgp-critical',
  flat: 'text-pgp-text-muted',
}

export function MetricCard({ label, value, unit, trend, trendValue, status }: MetricCardProps) {
  const TrendIcon = trend ? trendIcons[trend] : null
  const borderColor = status ? STATUS_BG_COLORS[status] : 'bg-pgp-border'

  return (
    <div className="relative overflow-hidden rounded-lg border border-pgp-border bg-pgp-bg-card p-4">
      <div className={`absolute left-0 top-0 h-full w-0.5 ${borderColor}`} />
      <p className="text-sm text-pgp-text-muted">{label}</p>
      <div className="mt-1 flex items-baseline gap-1">
        <span className="text-2xl font-semibold text-pgp-text-primary">{value}</span>
        {unit && <span className="text-sm text-pgp-text-muted">{unit}</span>}
      </div>
      {trend && trendValue && TrendIcon && (
        <div className={`mt-2 flex items-center gap-1 text-sm ${trendColors[trend]}`}>
          <TrendIcon className="h-4 w-4" />
          <span>{trendValue}</span>
        </div>
      )}
    </div>
  )
}
