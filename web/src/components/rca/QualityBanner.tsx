import { X, Info } from 'lucide-react'
import type { RCAQualityStatus } from '@/types/rca'

interface QualityBannerProps {
  quality: RCAQualityStatus
  onDismiss?: () => void
}

export function QualityBanner({ quality, onDismiss }: QualityBannerProps) {
  const completeness = Math.round(quality.telemetry_completeness * 100)
  const sourceLabel =
    quality.anomaly_source_mode === 'ml' ? 'ML-enhanced' : 'Threshold fallback'

  return (
    <div className="relative flex items-start gap-3 rounded-lg border border-blue-500/20 bg-blue-500/10 px-4 py-3">
      <Info className="mt-0.5 h-4 w-4 shrink-0 text-blue-400" />
      <div className="flex-1 space-y-1 text-sm text-pgp-text-secondary">
        <p>
          Telemetry: <span className="font-medium text-pgp-text-primary">{completeness}%</span>{' '}
          complete
        </p>
        <p>
          Source: <span className="font-medium text-pgp-text-primary">{sourceLabel}</span>
        </p>
        {quality.scope_limitations && quality.scope_limitations.length > 0 && (
          <div>
            <p className="text-xs font-medium text-pgp-text-muted">Scope limitations:</p>
            <ul className="ml-4 list-disc text-xs text-pgp-text-muted">
              {quality.scope_limitations.map((lim) => (
                <li key={lim}>{lim}</li>
              ))}
            </ul>
          </div>
        )}
      </div>
      {onDismiss && (
        <button
          onClick={onDismiss}
          className="rounded p-1 text-pgp-text-muted transition-colors hover:bg-pgp-bg-hover hover:text-pgp-text-primary"
        >
          <X className="h-4 w-4" />
        </button>
      )}
    </div>
  )
}
