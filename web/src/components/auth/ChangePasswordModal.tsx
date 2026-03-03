import { useState } from 'react'
import { X } from 'lucide-react'
import { useChangePassword } from '@/hooks/useUsers'

interface ChangePasswordModalProps {
  onClose: () => void
}

export function ChangePasswordModal({ onClose }: ChangePasswordModalProps) {
  const [currentPassword, setCurrentPassword] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [error, setError] = useState<string | null>(null)
  const changePassword = useChangePassword()

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError(null)

    if (newPassword !== confirmPassword) {
      setError('New passwords do not match')
      return
    }
    if (newPassword.length < 8) {
      setError('New password must be at least 8 characters')
      return
    }

    try {
      await changePassword.mutateAsync({ currentPassword, newPassword })
      onClose()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to change password')
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
      <div className="w-full max-w-md rounded-lg border border-pgp-border bg-pgp-bg-card p-6">
        <div className="mb-4 flex items-center justify-between">
          <h2 className="text-lg font-semibold text-pgp-text-primary">Change Password</h2>
          <button onClick={onClose} className="text-pgp-text-muted hover:text-pgp-text-primary">
            <X className="h-5 w-5" />
          </button>
        </div>

        <form onSubmit={handleSubmit} className="space-y-4">
          {error && (
            <div className="rounded-md bg-red-500/10 px-3 py-2 text-sm text-red-400">{error}</div>
          )}

          <div>
            <label className="block text-sm font-medium text-pgp-text-secondary">Current Password</label>
            <input
              type="password"
              value={currentPassword}
              onChange={(e) => setCurrentPassword(e.target.value)}
              className="mt-1 block w-full rounded-md border border-pgp-border bg-pgp-bg-primary px-3 py-2 text-sm text-pgp-text-primary focus:border-pgp-accent focus:outline-none focus:ring-1 focus:ring-pgp-accent"
              required
            />
          </div>

          <div>
            <label className="block text-sm font-medium text-pgp-text-secondary">New Password</label>
            <input
              type="password"
              value={newPassword}
              onChange={(e) => setNewPassword(e.target.value)}
              className="mt-1 block w-full rounded-md border border-pgp-border bg-pgp-bg-primary px-3 py-2 text-sm text-pgp-text-primary focus:border-pgp-accent focus:outline-none focus:ring-1 focus:ring-pgp-accent"
              required
            />
          </div>

          <div>
            <label className="block text-sm font-medium text-pgp-text-secondary">Confirm New Password</label>
            <input
              type="password"
              value={confirmPassword}
              onChange={(e) => setConfirmPassword(e.target.value)}
              className="mt-1 block w-full rounded-md border border-pgp-border bg-pgp-bg-primary px-3 py-2 text-sm text-pgp-text-primary focus:border-pgp-accent focus:outline-none focus:ring-1 focus:ring-pgp-accent"
              required
            />
          </div>

          <div className="flex justify-end gap-3">
            <button
              type="button"
              onClick={onClose}
              className="rounded-md px-4 py-2 text-sm text-pgp-text-secondary hover:bg-pgp-bg-hover"
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={changePassword.isPending}
              className="rounded-md bg-pgp-accent px-4 py-2 text-sm font-medium text-white hover:bg-pgp-accent-hover disabled:opacity-50"
            >
              {changePassword.isPending ? 'Changing...' : 'Change Password'}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}
