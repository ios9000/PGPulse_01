import { useState, useEffect, useCallback } from 'react'
import { X } from 'lucide-react'
import { useCreateUser, useUpdateUser } from '@/hooks/useUsers'
import { useAuthStore } from '@/stores/authStore'
import type { UserRole } from '@/types/models'

interface UserRecord {
  id: number
  username: string
  role: string
  active: boolean
  permissions: string[]
}

interface UserFormModalProps {
  mode: 'create' | 'edit'
  user?: UserRecord
  onClose: () => void
}

const ALL_ROLES: { value: UserRole; label: string }[] = [
  { value: 'dba', label: 'DBA' },
  { value: 'app_admin', label: 'App Admin' },
  { value: 'roles_admin', label: 'Roles Admin' },
  { value: 'super_admin', label: 'Super Admin' },
]

function getAssignableRoles(currentUserRole: string): { value: UserRole; label: string }[] {
  if (currentUserRole === 'super_admin') return ALL_ROLES
  if (currentUserRole === 'roles_admin') {
    return ALL_ROLES.filter((r) => r.value === 'dba' || r.value === 'app_admin')
  }
  return []
}

export function UserFormModal({ mode, user, onClose }: UserFormModalProps) {
  const currentUser = useAuthStore((s) => s.user)
  const createUser = useCreateUser()
  const updateUser = useUpdateUser()

  const assignableRoles = getAssignableRoles(currentUser?.role ?? '')

  const [username, setUsername] = useState(user?.username ?? '')
  const [password, setPassword] = useState('')
  const [role, setRole] = useState(user?.role ?? assignableRoles[0]?.value ?? 'dba')
  const [active, setActive] = useState(user?.active ?? true)
  const [error, setError] = useState<string | null>(null)

  const isPending = createUser.isPending || updateUser.isPending

  const handleClose = useCallback(() => {
    if (!isPending) onClose()
  }, [isPending, onClose])

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

    try {
      if (mode === 'create') {
        if (password.length < 8) {
          setError('Password must be at least 8 characters')
          return
        }
        await createUser.mutateAsync({ username, password, role })
      } else if (user) {
        await updateUser.mutateAsync({ id: user.id, role, active })
      }
      onClose()
    } catch (err) {
      setError(err instanceof Error ? err.message : `Failed to ${mode} user`)
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
            {mode === 'create' ? 'Create User' : 'Edit User'}
          </h2>
          <button onClick={handleClose} className="text-pgp-text-muted hover:text-pgp-text-primary">
            <X className="h-5 w-5" />
          </button>
        </div>

        <form onSubmit={handleSubmit} className="space-y-4">
          {error && (
            <div className="rounded-md bg-red-500/10 px-3 py-2 text-sm text-red-400">{error}</div>
          )}

          <div>
            <label className="block text-sm font-medium text-pgp-text-secondary">Username</label>
            {mode === 'create' ? (
              <input
                type="text"
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                className="mt-1 block w-full rounded-md border border-pgp-border bg-pgp-bg-primary px-3 py-2 text-sm text-pgp-text-primary focus:border-pgp-accent focus:outline-none focus:ring-1 focus:ring-pgp-accent"
                required
              />
            ) : (
              <p className="mt-1 text-sm text-pgp-text-primary">{user?.username}</p>
            )}
          </div>

          {mode === 'create' && (
            <div>
              <label className="block text-sm font-medium text-pgp-text-secondary">Password</label>
              <input
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                className="mt-1 block w-full rounded-md border border-pgp-border bg-pgp-bg-primary px-3 py-2 text-sm text-pgp-text-primary focus:border-pgp-accent focus:outline-none focus:ring-1 focus:ring-pgp-accent"
                required
                minLength={8}
              />
              <p className="mt-1 text-xs text-pgp-text-muted">Minimum 8 characters</p>
            </div>
          )}

          <div>
            <label className="block text-sm font-medium text-pgp-text-secondary">Role</label>
            <select
              value={role}
              onChange={(e) => setRole(e.target.value)}
              className="mt-1 block w-full rounded-md border border-pgp-border bg-pgp-bg-primary px-3 py-2 text-sm text-pgp-text-primary focus:border-pgp-accent focus:outline-none focus:ring-1 focus:ring-pgp-accent"
            >
              {assignableRoles.map((r) => (
                <option key={r.value} value={r.value}>
                  {r.label}
                </option>
              ))}
            </select>
          </div>

          {mode === 'edit' && (
            <div className="flex items-center gap-3">
              <label className="text-sm font-medium text-pgp-text-secondary">Active</label>
              <button
                type="button"
                onClick={() => setActive(!active)}
                className="relative inline-flex h-5 w-9 items-center rounded-full transition-colors focus:outline-none focus:ring-2 focus:ring-blue-500"
                role="switch"
                aria-checked={active}
              >
                <span
                  className={`absolute inset-0 rounded-full transition-colors ${
                    active ? 'bg-blue-600' : 'bg-gray-600'
                  }`}
                />
                <span
                  className={`relative inline-block h-3.5 w-3.5 rounded-full bg-white transition-transform ${
                    active ? 'translate-x-4.5' : 'translate-x-1'
                  }`}
                />
              </button>
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
              type="submit"
              disabled={isPending}
              className="rounded-md bg-pgp-accent px-4 py-2 text-sm font-medium text-white hover:bg-pgp-accent-hover disabled:opacity-50"
            >
              {isPending
                ? mode === 'create'
                  ? 'Creating...'
                  : 'Saving...'
                : mode === 'create'
                  ? 'Create User'
                  : 'Save Changes'}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}
