import { useState } from 'react'
import { useAuth } from '@/hooks/useAuth'
import { apiFetch, ApiError } from '@/lib/api'
import { ConfirmModal } from './ConfirmModal'
import { toast } from '@/stores/toastStore'

interface SessionActionsProps {
  instanceId: string
  pid: number
  applicationName: string
  onRefresh?: () => void
}

type ActionType = 'cancel' | 'terminate'

export function SessionActions({ instanceId, pid, applicationName, onRefresh }: SessionActionsProps) {
  const { can } = useAuth()
  const [action, setAction] = useState<ActionType | null>(null)
  const [loading, setLoading] = useState(false)
  const [overrideSelfGuard, setOverrideSelfGuard] = useState(false)

  if (!can('instance_management')) return null

  const isSelf = applicationName.startsWith('pgpulse_')
  if (isSelf && !overrideSelfGuard) {
    return (
      <button
        onClick={(e) => { e.stopPropagation(); setOverrideSelfGuard(true) }}
        className="text-xs text-pgp-text-muted hover:text-pgp-text-secondary"
      >
        Show actions
      </button>
    )
  }

  const handleConfirm = async () => {
    if (!action) return
    setLoading(true)

    try {
      await apiFetch(`/instances/${instanceId}/sessions/${pid}/${action}`, {
        method: 'POST',
      })

      if (action === 'cancel') {
        toast.success(`Query cancelled for PID ${pid}`)
      } else {
        toast.success(`Session terminated for PID ${pid}`)
      }
      onRefresh?.()
    } catch (err) {
      if (err instanceof ApiError) {
        if (err.status === 403) {
          toast.error('Insufficient permissions')
        } else if (err.status === 404) {
          toast.warning('Session no longer active')
        } else {
          toast.error(`Failed to ${action} session`)
        }
      } else {
        toast.error(`Failed to ${action} session`)
      }
    } finally {
      setLoading(false)
      setAction(null)
    }
  }

  return (
    <>
      <div className="flex items-center gap-1">
        <button
          onClick={(e) => { e.stopPropagation(); setAction('cancel') }}
          className="rounded px-2 py-0.5 text-xs font-medium text-amber-400 hover:bg-amber-500/10"
          title="Cancel query"
        >
          Cancel
        </button>
        <button
          onClick={(e) => { e.stopPropagation(); setAction('terminate') }}
          className="rounded px-2 py-0.5 text-xs font-medium text-red-400 hover:bg-red-500/10"
          title="Terminate session"
        >
          Kill
        </button>
      </div>

      <ConfirmModal
        open={action === 'cancel'}
        title="Cancel Query"
        message={`Cancel the running query for PID ${pid}?`}
        confirmLabel="Cancel Query"
        confirmVariant="warning"
        onConfirm={handleConfirm}
        onCancel={() => setAction(null)}
        loading={loading}
      />

      <ConfirmModal
        open={action === 'terminate'}
        title="Terminate Session"
        message={`Terminate the entire session for PID ${pid}? This will disconnect the client.`}
        confirmLabel="Terminate"
        confirmVariant="danger"
        onConfirm={handleConfirm}
        onCancel={() => setAction(null)}
        loading={loading}
      />
    </>
  )
}
