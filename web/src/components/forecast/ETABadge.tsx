import type { OperationETA } from '@/hooks/useMaintenanceForecast'
import { ETAConfidenceIndicator } from './ETAConfidenceIndicator'

interface ETABadgeProps {
  eta: OperationETA | undefined
}

function formatETA(seconds: number): string {
  if (seconds < 60) return '< 1 min'
  if (seconds < 3600) return `${Math.round(seconds / 60)}m`
  if (seconds < 86400) {
    const h = Math.floor(seconds / 3600)
    const m = Math.round((seconds % 3600) / 60)
    return m > 0 ? `${h}h ${m}m` : `${h}h`
  }
  const d = Math.floor(seconds / 86400)
  const h = Math.round((seconds % 86400) / 3600)
  return h > 0 ? `${d}d ${h}h` : `${d}d`
}

const CONFIDENCE_TEXT_COLOR: Record<string, string> = {
  high: 'text-green-400',
  medium: 'text-yellow-400',
  estimating: 'text-slate-400',
  stalled: 'text-red-400',
}

export function ETABadge({ eta }: ETABadgeProps) {
  if (!eta) {
    return <span className="text-sm text-pgp-text-muted">&mdash;</span>
  }

  const textColor = CONFIDENCE_TEXT_COLOR[eta.confidence] ?? 'text-slate-400'

  if (eta.eta_sec === -1) {
    if (eta.confidence === 'stalled') {
      return (
        <span className={`inline-flex items-center gap-1 text-sm font-medium ${textColor}`}>
          <ETAConfidenceIndicator confidence="stalled" />
          Stalled
        </span>
      )
    }
    if (eta.confidence === 'estimating') {
      return (
        <span className={`inline-flex items-center gap-1 text-sm ${textColor} animate-pulse`}>
          <ETAConfidenceIndicator confidence="estimating" />
          estimating...
        </span>
      )
    }
    return <span className="text-sm text-pgp-text-muted">&mdash;</span>
  }

  const prefix = eta.confidence === 'medium' ? '~' : ''
  const timeStr = formatETA(eta.eta_sec)

  return (
    <span className={`inline-flex items-center gap-1 text-sm font-medium ${textColor}`}>
      <ETAConfidenceIndicator confidence={eta.confidence} />
      {prefix}{timeStr}
    </span>
  )
}
