import { useEffect, useCallback } from 'react'
import { Loader2 } from 'lucide-react'

interface ConfirmModalProps {
  open: boolean
  title: string
  message: string
  confirmLabel: string
  confirmVariant: 'warning' | 'danger'
  onConfirm: () => void
  onCancel: () => void
  loading?: boolean
}

const VARIANT_STYLES: Record<ConfirmModalProps['confirmVariant'], string> = {
  warning: 'bg-amber-500 hover:bg-amber-600 text-white',
  danger: 'bg-red-500 hover:bg-red-600 text-white',
}

export function ConfirmModal({
  open,
  title,
  message,
  confirmLabel,
  confirmVariant,
  onConfirm,
  onCancel,
  loading = false,
}: ConfirmModalProps) {
  const handleKeyDown = useCallback(
    (e: KeyboardEvent) => {
      if (e.key === 'Escape') onCancel()
    },
    [onCancel],
  )

  useEffect(() => {
    if (open) {
      document.addEventListener('keydown', handleKeyDown)
      return () => document.removeEventListener('keydown', handleKeyDown)
    }
  }, [open, handleKeyDown])

  if (!open) return null

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
      onClick={onCancel}
    >
      <div
        className="mx-4 w-full max-w-md rounded-lg border border-pgp-border bg-pgp-bg-card p-6 shadow-xl"
        onClick={(e) => e.stopPropagation()}
      >
        <h3 className="mb-2 text-lg font-semibold text-pgp-text-primary">{title}</h3>
        <p className="mb-6 text-sm text-pgp-text-secondary">{message}</p>

        <div className="flex justify-end gap-3">
          <button
            onClick={onCancel}
            disabled={loading}
            className="rounded-md border border-pgp-border px-4 py-2 text-sm text-pgp-text-secondary hover:bg-pgp-bg-hover disabled:opacity-50"
          >
            Cancel
          </button>
          <button
            onClick={onConfirm}
            disabled={loading}
            className={`inline-flex items-center gap-2 rounded-md px-4 py-2 text-sm font-medium disabled:opacity-50 ${VARIANT_STYLES[confirmVariant]}`}
          >
            {loading && <Loader2 className="h-4 w-4 animate-spin" />}
            {confirmLabel}
          </button>
        </div>
      </div>
    </div>
  )
}
