import { useState, useEffect, useCallback } from 'react'
import { useDeleteAlertRule } from '@/hooks/useAlertRules'
import type { AlertRule } from '@/types/models'

interface DeleteConfirmModalProps {
  onClose: () => void
  rule: AlertRule | null
}

export function DeleteConfirmModal({ onClose, rule }: DeleteConfirmModalProps) {
  const [error, setError] = useState<string | null>(null)
  const deleteRule = useDeleteAlertRule()

  const handleClose = useCallback(() => {
    if (!deleteRule.isPending) onClose()
  }, [deleteRule.isPending, onClose])

  useEffect(() => {
    const handleEsc = (e: KeyboardEvent) => {
      if (e.key === 'Escape') handleClose()
    }
    document.addEventListener('keydown', handleEsc)
    return () => document.removeEventListener('keydown', handleEsc)
  }, [handleClose])

  const handleBackdropClick = (e: React.MouseEvent<HTMLDivElement>) => {
    if (e.target === e.currentTarget) handleClose()
  }

  const handleDelete = async () => {
    if (!rule) return
    setError(null)
    try {
      await deleteRule.mutateAsync(rule.id)
      onClose()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to delete rule')
    }
  }

  if (!rule) return null

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
      onClick={handleBackdropClick}
    >
      <div className="w-full max-w-sm rounded-lg border border-pgp-border bg-pgp-bg-card p-6">
        <h2 className="mb-2 text-lg font-semibold text-pgp-text-primary">Delete Rule</h2>
        <p className="mb-6 text-sm text-pgp-text-secondary">
          Are you sure you want to delete{' '}
          <span className="font-medium text-pgp-text-primary">{rule.name}</span>? This action
          cannot be undone.
        </p>

        {error && (
          <div className="mb-4 rounded-md bg-red-500/10 px-3 py-2 text-sm text-red-400">
            {error}
          </div>
        )}

        <div className="flex justify-end gap-3">
          <button
            onClick={handleClose}
            className="rounded-md px-4 py-2 text-sm text-pgp-text-secondary hover:bg-pgp-bg-hover"
          >
            Cancel
          </button>
          <button
            onClick={handleDelete}
            disabled={deleteRule.isPending}
            className="rounded-md bg-red-500 px-4 py-2 text-sm font-medium text-white hover:bg-red-600 disabled:opacity-50"
          >
            {deleteRule.isPending ? 'Deleting...' : 'Delete'}
          </button>
        </div>
      </div>
    </div>
  )
}
