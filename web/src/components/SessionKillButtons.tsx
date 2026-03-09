import { useState } from 'react'
import { apiFetch } from '@/lib/api'
import type { SessionKillResult } from '@/types/models'

interface SessionKillButtonsProps {
  pid: number
  applicationName: string
  instanceId: string
  onSuccess: () => void
}

export function SessionKillButtons({ pid, applicationName, instanceId, onSuccess }: SessionKillButtonsProps) {
  const [showCancelModal, setShowCancelModal] = useState(false)
  const [showTerminateModal, setShowTerminateModal] = useState(false)
  const [loading, setLoading] = useState<'cancel' | 'terminate' | null>(null)
  const [error, setError] = useState<string | null>(null)

  const handleKill = async (operation: 'cancel' | 'terminate') => {
    setLoading(operation)
    setError(null)
    try {
      const res = await apiFetch(`/instances/${instanceId}/sessions/${pid}/${operation}`, {
        method: 'POST',
      })
      const json = await res.json()
      const result = json.data as SessionKillResult
      if (!result.ok && result.error) {
        setError(result.error)
      } else {
        onSuccess()
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Operation failed')
    } finally {
      setLoading(null)
      setShowCancelModal(false)
      setShowTerminateModal(false)
    }
  }

  return (
    <>
      <div className="flex items-center gap-1">
        <button
          onClick={() => setShowCancelModal(true)}
          disabled={loading !== null}
          className="rounded p-1 text-pgp-text-muted hover:bg-pgp-bg-hover hover:text-pgp-text-secondary"
          title="Cancel query"
        >
          {loading === 'cancel' ? (
            <span className="inline-block h-4 w-4 animate-spin rounded-full border-2 border-current border-t-transparent" />
          ) : (
            <svg className="h-4 w-4" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="2">
              <line x1="4" y1="4" x2="12" y2="12" />
              <line x1="12" y1="4" x2="4" y2="12" />
            </svg>
          )}
        </button>
        <button
          onClick={() => setShowTerminateModal(true)}
          disabled={loading !== null}
          className="rounded p-1 text-pgp-text-muted hover:bg-red-900/20 hover:text-red-400"
          title="Terminate session"
        >
          {loading === 'terminate' ? (
            <span className="inline-block h-4 w-4 animate-spin rounded-full border-2 border-current border-t-transparent" />
          ) : (
            <svg className="h-4 w-4" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="2">
              <path d="M8 2L2 14h12L8 2z" />
              <line x1="8" y1="6" x2="8" y2="10" />
              <circle cx="8" cy="12" r="0.5" fill="currentColor" />
            </svg>
          )}
        </button>
      </div>

      {error && (
        <span className="text-xs text-red-400">{error}</span>
      )}

      {/* Cancel Modal */}
      {showCancelModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50" onClick={() => setShowCancelModal(false)}>
          <div className="mx-4 w-full max-w-md rounded-lg border border-pgp-border bg-pgp-bg-card p-6" onClick={(e) => e.stopPropagation()}>
            <h3 className="mb-2 text-lg font-medium text-pgp-text-primary">Cancel Query</h3>
            <p className="mb-4 text-sm text-pgp-text-secondary">
              Cancel query for PID {pid} ({applicationName})? The connection will remain open.
            </p>
            <div className="flex justify-end gap-3">
              <button
                onClick={() => setShowCancelModal(false)}
                className="rounded-md px-4 py-2 text-sm text-pgp-text-secondary hover:bg-pgp-bg-hover"
              >
                Close
              </button>
              <button
                onClick={() => handleKill('cancel')}
                disabled={loading !== null}
                className="rounded-md bg-pgp-accent px-4 py-2 text-sm font-medium text-white hover:bg-pgp-accent/80"
              >
                Cancel Query
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Terminate Modal */}
      {showTerminateModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50" onClick={() => setShowTerminateModal(false)}>
          <div className="mx-4 w-full max-w-md rounded-lg border border-red-500/30 bg-pgp-bg-card p-6" onClick={(e) => e.stopPropagation()}>
            <h3 className="mb-2 text-lg font-medium text-red-400">Terminate Session</h3>
            <p className="mb-4 text-sm text-pgp-text-secondary">
              Terminate session PID {pid} ({applicationName})? <strong className="text-red-400">This will close the connection.</strong>
            </p>
            <div className="flex justify-end gap-3">
              <button
                onClick={() => setShowTerminateModal(false)}
                className="rounded-md px-4 py-2 text-sm text-pgp-text-secondary hover:bg-pgp-bg-hover"
              >
                Close
              </button>
              <button
                onClick={() => handleKill('terminate')}
                disabled={loading !== null}
                className="rounded-md bg-red-600 px-4 py-2 text-sm font-medium text-white hover:bg-red-700"
              >
                Terminate
              </button>
            </div>
          </div>
        </div>
      )}
    </>
  )
}
