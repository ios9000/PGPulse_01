import { X, CheckCircle } from 'lucide-react'
import { PriorityBadge } from '@/components/advisor/PriorityBadge'
import { useAcknowledge } from '@/hooks/useRecommendations'
import { toast } from '@/stores/toastStore'
import type { DiagnoseResponse } from '@/types/models'

interface DiagnosePanelProps {
  data: DiagnoseResponse
  onClose: () => void
}

export function DiagnosePanel({ data, onClose }: DiagnosePanelProps) {
  const ack = useAcknowledge()

  const handleAcknowledge = (id: number) => {
    ack.mutate(id, {
      onSuccess: () => toast.success('Recommendation acknowledged'),
      onError: () => toast.error('Failed to acknowledge'),
    })
  }

  return (
    <div className="rounded-lg border border-pgp-border bg-pgp-bg-card p-4">
      <div className="mb-4 flex items-center justify-between">
        <div className="flex items-center gap-3">
          <h3 className="text-lg font-semibold text-pgp-text-primary">Diagnosis Results</h3>
          <span className="text-sm text-pgp-text-muted">
            {data.metrics_evaluated} metrics, {data.rules_evaluated} rules evaluated
          </span>
        </div>
        <button
          onClick={onClose}
          className="rounded-md p-1 text-pgp-text-muted hover:bg-pgp-bg-hover hover:text-pgp-text-primary"
        >
          <X className="h-5 w-5" />
        </button>
      </div>

      {data.recommendations.length === 0 ? (
        <div className="flex items-center gap-2 py-4 text-sm">
          <CheckCircle className="h-5 w-5 text-green-400" />
          <span className="text-pgp-text-secondary">No issues found. Everything looks healthy.</span>
        </div>
      ) : (
        <div className="space-y-3">
          {data.recommendations.map((rec) => (
            <div
              key={rec.id}
              className="flex flex-wrap items-start gap-3 rounded-md border border-pgp-border bg-pgp-bg-secondary p-3"
            >
              <PriorityBadge priority={rec.priority} />
              <div className="flex-1">
                <p className="text-sm font-medium text-pgp-text-primary">{rec.title}</p>
                <p className="mt-1 text-xs text-pgp-text-muted">{rec.description}</p>
                <div className="mt-1 flex flex-wrap gap-3 text-xs text-pgp-text-muted">
                  <span>{rec.metric_key} = {rec.metric_value.toFixed(2)}</span>
                  <span className="rounded bg-pgp-bg-secondary px-1.5 py-0.5">{rec.category}</span>
                  {rec.doc_url && (
                    <a
                      href={rec.doc_url}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="text-blue-400 hover:text-blue-300"
                    >
                      Docs
                    </a>
                  )}
                </div>
              </div>
              {!rec.acknowledged_at && (
                <button
                  onClick={() => handleAcknowledge(rec.id)}
                  disabled={ack.isPending}
                  className="rounded-md border border-pgp-border bg-pgp-bg-secondary px-2 py-1 text-xs text-pgp-text-secondary hover:bg-pgp-bg-hover hover:text-pgp-text-primary disabled:opacity-50"
                >
                  Acknowledge
                </button>
              )}
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
