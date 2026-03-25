import { useState } from 'react'
import { X, ThumbsUp, ThumbsDown } from 'lucide-react'
import { useSubmitFeedback } from '@/hooks/usePlaybooks'
import { toast } from '@/stores/toastStore'

interface FeedbackModalProps {
  runId: number
  onClose: () => void
}

export function FeedbackModal({ runId, onClose }: FeedbackModalProps) {
  const [helpful, setHelpful] = useState<boolean | null>(null)
  const [resolved, setResolved] = useState<boolean | null>(null)
  const [notes, setNotes] = useState('')
  const submit = useSubmitFeedback()

  const handleSubmit = async () => {
    try {
      await submit.mutateAsync({ runId, helpful, resolved, notes })
      toast.success('Feedback submitted')
      onClose()
    } catch {
      toast.error('Failed to submit feedback')
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
      <div className="w-full max-w-md rounded-lg border border-pgp-border bg-pgp-bg-card p-6 shadow-xl">
        <div className="mb-4 flex items-center justify-between">
          <h3 className="text-sm font-semibold text-pgp-text-primary">Playbook Feedback</h3>
          <button onClick={onClose} className="text-pgp-text-muted hover:text-pgp-text-primary">
            <X className="h-4 w-4" />
          </button>
        </div>

        <div className="space-y-4">
          {/* Helpful */}
          <div>
            <p className="mb-2 text-xs text-pgp-text-secondary">Was this playbook helpful?</p>
            <div className="flex gap-2">
              <button
                onClick={() => setHelpful(true)}
                className={`inline-flex items-center gap-1.5 rounded-md border px-3 py-1.5 text-sm transition-colors ${
                  helpful === true
                    ? 'border-green-500 bg-green-500/10 text-green-400'
                    : 'border-pgp-border text-pgp-text-secondary hover:bg-pgp-bg-hover'
                }`}
              >
                <ThumbsUp className="h-4 w-4" /> Yes
              </button>
              <button
                onClick={() => setHelpful(false)}
                className={`inline-flex items-center gap-1.5 rounded-md border px-3 py-1.5 text-sm transition-colors ${
                  helpful === false
                    ? 'border-red-500 bg-red-500/10 text-red-400'
                    : 'border-pgp-border text-pgp-text-secondary hover:bg-pgp-bg-hover'
                }`}
              >
                <ThumbsDown className="h-4 w-4" /> No
              </button>
            </div>
          </div>

          {/* Resolved */}
          <div>
            <p className="mb-2 text-xs text-pgp-text-secondary">Did it resolve the issue?</p>
            <div className="flex gap-2">
              <button
                onClick={() => setResolved(true)}
                className={`rounded-md border px-3 py-1.5 text-sm transition-colors ${
                  resolved === true
                    ? 'border-green-500 bg-green-500/10 text-green-400'
                    : 'border-pgp-border text-pgp-text-secondary hover:bg-pgp-bg-hover'
                }`}
              >
                Yes
              </button>
              <button
                onClick={() => setResolved(false)}
                className={`rounded-md border px-3 py-1.5 text-sm transition-colors ${
                  resolved === false
                    ? 'border-red-500 bg-red-500/10 text-red-400'
                    : 'border-pgp-border text-pgp-text-secondary hover:bg-pgp-bg-hover'
                }`}
              >
                No
              </button>
            </div>
          </div>

          {/* Notes */}
          <div>
            <p className="mb-2 text-xs text-pgp-text-secondary">Notes (optional)</p>
            <textarea
              value={notes}
              onChange={(e) => setNotes(e.target.value)}
              rows={3}
              className="w-full rounded-md border border-pgp-border bg-pgp-bg-secondary px-3 py-2 text-sm text-pgp-text-primary placeholder:text-pgp-text-muted"
              placeholder="Any additional feedback..."
            />
          </div>

          <div className="flex justify-end gap-2">
            <button
              onClick={onClose}
              className="rounded-md border border-pgp-border px-3 py-1.5 text-sm text-pgp-text-secondary hover:bg-pgp-bg-hover"
            >
              Skip
            </button>
            <button
              onClick={handleSubmit}
              disabled={submit.isPending}
              className="rounded-md bg-pgp-accent px-3 py-1.5 text-sm font-medium text-white hover:bg-pgp-accent/80 disabled:opacity-50"
            >
              {submit.isPending ? 'Submitting...' : 'Submit Feedback'}
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}
