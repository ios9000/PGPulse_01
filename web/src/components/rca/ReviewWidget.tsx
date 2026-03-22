import { useState } from 'react'
import { CheckCircle, XCircle, HelpCircle, ChevronDown, ChevronRight } from 'lucide-react'
import { useReviewIncident } from '@/hooks/useRCA'
import { toast } from '@/stores/toastStore'

interface ReviewWidgetProps {
  incidentId: number
  instanceId: string
  currentStatus?: string
  currentComment?: string
}

const STATUS_CONFIG: Record<string, { icon: typeof CheckCircle; label: string; bg: string }> = {
  confirmed: {
    icon: CheckCircle,
    label: 'Confirmed',
    bg: 'bg-green-500/20 text-green-400',
  },
  false_positive: {
    icon: XCircle,
    label: 'False Positive',
    bg: 'bg-red-500/20 text-red-400',
  },
  inconclusive: {
    icon: HelpCircle,
    label: 'Inconclusive',
    bg: 'bg-amber-500/20 text-amber-400',
  },
}

export function ReviewWidget({ incidentId, instanceId, currentStatus, currentComment }: ReviewWidgetProps) {
  const [notesOpen, setNotesOpen] = useState(false)
  const [notes, setNotes] = useState(currentComment ?? '')
  const review = useReviewIncident()

  const handleSubmit = (status: string) => {
    review.mutate(
      { instanceId, incidentId, status, notes: notes || undefined },
      {
        onSuccess: () => toast.success(`Incident marked as ${STATUS_CONFIG[status]?.label ?? status}`),
        onError: () => toast.error('Failed to submit review'),
      },
    )
  }

  const currentConfig = currentStatus ? STATUS_CONFIG[currentStatus] : null

  return (
    <div className="rounded-lg border border-pgp-border bg-pgp-bg-card p-4">
      <div className="flex items-center justify-between">
        <h3 className="text-sm font-medium text-pgp-text-primary">Review</h3>
        {currentConfig && (
          <span
            className={`inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-semibold ${currentConfig.bg}`}
          >
            <currentConfig.icon className="h-3 w-3" />
            {currentConfig.label}
          </span>
        )}
      </div>

      <div className="mt-3 flex flex-wrap items-center gap-2">
        {Object.entries(STATUS_CONFIG).map(([key, cfg]) => {
          const Icon = cfg.icon
          const isActive = currentStatus === key
          return (
            <button
              key={key}
              onClick={() => handleSubmit(key)}
              disabled={review.isPending}
              className={`inline-flex items-center gap-1.5 rounded-md border px-3 py-1.5 text-xs font-medium transition-colors disabled:opacity-50 ${
                isActive
                  ? 'border-pgp-accent bg-pgp-accent/10 text-pgp-accent'
                  : 'border-pgp-border bg-pgp-bg-secondary text-pgp-text-secondary hover:bg-pgp-bg-hover hover:text-pgp-text-primary'
              }`}
            >
              <Icon className="h-3.5 w-3.5" />
              {cfg.label}
            </button>
          )
        })}
      </div>

      <button
        onClick={() => setNotesOpen(!notesOpen)}
        className="mt-3 flex items-center gap-1 text-xs text-pgp-text-muted hover:text-pgp-text-secondary"
      >
        {notesOpen ? <ChevronDown className="h-3 w-3" /> : <ChevronRight className="h-3 w-3" />}
        Add notes
      </button>
      {notesOpen && (
        <textarea
          value={notes}
          onChange={(e) => setNotes(e.target.value)}
          placeholder="Optional review notes..."
          rows={3}
          className="mt-2 w-full rounded-md border border-pgp-border bg-pgp-bg-secondary px-3 py-2 text-sm text-pgp-text-primary placeholder:text-pgp-text-muted focus:border-pgp-accent focus:outline-none focus:ring-1 focus:ring-pgp-accent"
        />
      )}
    </div>
  )
}
