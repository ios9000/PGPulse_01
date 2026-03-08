import { useState, useEffect, useCallback } from 'react'
import { X } from 'lucide-react'
import { useResetUserPassword } from '@/hooks/useUsers'

interface UserRecord {
  id: number
  username: string
  role: string
}

interface ResetPasswordModalProps {
  user: UserRecord
  onClose: () => void
}

export function ResetPasswordModal({ user, onClose }: ResetPasswordModalProps) {
  const [newPassword, setNewPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [error, setError] = useState<string | null>(null)
  const resetPassword = useResetUserPassword()

  const handleClose = useCallback(() => {
    if (!resetPassword.isPending) onClose()
  }, [resetPassword.isPending, onClose])

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

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError(null)

    if (newPassword.length < 8) {
      setError('Password must be at least 8 characters')
      return
    }
    if (newPassword !== confirmPassword) {
      setError('Passwords do not match')
      return
    }

    try {
      await resetPassword.mutateAsync({ id: user.id, newPassword })
      onClose()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to reset password')
    }
  }

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
      onClick={handleBackdropClick}
    >
      <div className="w-full max-w-md rounded-lg border border-pgp-border bg-pgp-bg-card p-6">
        <div className="mb-4 flex items-center justify-between">
          <h2 className="text-lg font-semibold text-pgp-text-primary">
            Reset Password &mdash; {user.username}
          </h2>
          <button
            onClick={handleClose}
            className="text-pgp-text-muted hover:text-pgp-text-primary"
          >
            <X className="h-5 w-5" />
          </button>
        </div>

        <form onSubmit={handleSubmit} className="space-y-4">
          {error && (
            <div className="rounded-md bg-red-500/10 px-3 py-2 text-sm text-red-400">{error}</div>
          )}

          <div>
            <label className="block text-sm font-medium text-pgp-text-secondary">
              New Password
            </label>
            <input
              type="password"
              value={newPassword}
              onChange={(e) => setNewPassword(e.target.value)}
              className="mt-1 block w-full rounded-md border border-pgp-border bg-pgp-bg-primary px-3 py-2 text-sm text-pgp-text-primary focus:border-pgp-accent focus:outline-none focus:ring-1 focus:ring-pgp-accent"
              required
              minLength={8}
            />
            <p className="mt-1 text-xs text-pgp-text-muted">Minimum 8 characters</p>
          </div>

          <div>
            <label className="block text-sm font-medium text-pgp-text-secondary">
              Confirm Password
            </label>
            <input
              type="password"
              value={confirmPassword}
              onChange={(e) => setConfirmPassword(e.target.value)}
              className="mt-1 block w-full rounded-md border border-pgp-border bg-pgp-bg-primary px-3 py-2 text-sm text-pgp-text-primary focus:border-pgp-accent focus:outline-none focus:ring-1 focus:ring-pgp-accent"
              required
              minLength={8}
            />
          </div>

          <div className="flex justify-end gap-3 pt-2">
            <button
              type="button"
              onClick={handleClose}
              className="rounded-md px-4 py-2 text-sm text-pgp-text-secondary hover:bg-pgp-bg-hover"
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={resetPassword.isPending}
              className="rounded-md bg-pgp-accent px-4 py-2 text-sm font-medium text-white hover:bg-pgp-accent-hover disabled:opacity-50"
            >
              {resetPassword.isPending ? 'Resetting...' : 'Reset Password'}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}
