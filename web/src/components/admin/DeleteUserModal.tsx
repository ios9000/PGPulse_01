import { useState, useEffect, useCallback } from 'react'
import { X, AlertTriangle } from 'lucide-react'
import { useDeleteUser } from '@/hooks/useUsers'

interface UserRecord {
  id: number
  username: string
  role: string
  active: boolean
}

interface DeleteUserModalProps {
  user: UserRecord
  onClose: () => void
}

export function DeleteUserModal({ user, onClose }: DeleteUserModalProps) {
  const [error, setError] = useState<string | null>(null)
  const deleteUser = useDeleteUser()

  const handleClose = useCallback(() => {
    if (!deleteUser.isPending) onClose()
  }, [deleteUser.isPending, onClose])

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
    setError(null)
    try {
      await deleteUser.mutateAsync(user.id)
      onClose()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to delete user')
    }
  }

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
      onClick={handleBackdropClick}
    >
      <div className="w-full max-w-md rounded-lg border border-pgp-border bg-pgp-bg-card p-6">
        <div className="mb-4 flex items-center justify-between">
          <h2 className="text-lg font-semibold text-pgp-text-primary">Delete User</h2>
          <button
            onClick={handleClose}
            className="text-pgp-text-muted hover:text-pgp-text-primary"
          >
            <X className="h-5 w-5" />
          </button>
        </div>

        <div className="space-y-4">
          {error && (
            <div className="rounded-md bg-red-500/10 px-3 py-2 text-sm text-red-400">{error}</div>
          )}

          <p className="text-sm text-pgp-text-secondary">
            Are you sure you want to delete user{' '}
            <span className="font-semibold text-pgp-text-primary">{user.username}</span>?
            This action cannot be undone.
          </p>

          {user.role === 'super_admin' && (
            <div className="flex items-start gap-2 rounded-md bg-yellow-500/10 px-3 py-2">
              <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-yellow-400" />
              <p className="text-sm text-yellow-400">
                This user is a super administrator.
              </p>
            </div>
          )}

          <div className="flex justify-end gap-3 pt-2">
            <button
              type="button"
              onClick={handleClose}
              className="rounded-md px-4 py-2 text-sm text-pgp-text-secondary hover:bg-pgp-bg-hover"
            >
              Cancel
            </button>
            <button
              type="button"
              onClick={handleDelete}
              disabled={deleteUser.isPending}
              className="rounded-md bg-red-600 px-4 py-2 text-sm font-medium text-white hover:bg-red-700 disabled:opacity-50"
            >
              {deleteUser.isPending ? 'Deleting...' : 'Delete'}
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}
