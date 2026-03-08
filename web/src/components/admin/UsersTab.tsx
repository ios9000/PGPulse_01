import { useState } from 'react'
import { Plus, Pencil, Trash2, KeyRound } from 'lucide-react'
import { useUsers } from '@/hooks/useUsers'
import { useAuthStore } from '@/stores/authStore'
import { UserFormModal } from '@/components/admin/UserFormModal'
import { DeleteUserModal } from '@/components/admin/DeleteUserModal'
import { ResetPasswordModal } from '@/components/admin/ResetPasswordModal'

interface UserRecord {
  id: number
  username: string
  role: string
  active: boolean
  permissions: string[]
}

const ROLE_LABELS: Record<string, string> = {
  super_admin: 'Super Admin',
  roles_admin: 'Roles Admin',
  dba: 'DBA',
  app_admin: 'App Admin',
}

const ROLE_COLORS: Record<string, string> = {
  super_admin: 'bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-400',
  roles_admin: 'bg-orange-100 text-orange-800 dark:bg-orange-900/30 dark:text-orange-400',
  dba: 'bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-400',
  app_admin: 'bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400',
}

export function UsersTab() {
  const { data: users, isLoading } = useUsers()
  const currentUser = useAuthStore((s) => s.user)

  const [formModal, setFormModal] = useState<{ mode: 'create' | 'edit'; user?: UserRecord } | null>(null)
  const [deleteModal, setDeleteModal] = useState<UserRecord | null>(null)
  const [resetModal, setResetModal] = useState<UserRecord | null>(null)

  const activeSuperAdminCount = users?.filter(
    (u) => u.role === 'super_admin' && u.active,
  ).length ?? 0

  if (isLoading) {
    return <div className="py-8 text-center text-pgp-text-muted">Loading users...</div>
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h3 className="text-lg font-medium text-pgp-text-primary">Users</h3>
        <button
          onClick={() => setFormModal({ mode: 'create' })}
          className="flex items-center gap-2 rounded-md bg-pgp-accent px-4 py-2 text-sm font-medium text-white hover:bg-pgp-accent-hover"
        >
          <Plus className="h-4 w-4" />
          Add User
        </button>
      </div>

      {(!users || users.length === 0) ? (
        <div className="rounded-lg border border-pgp-border bg-pgp-bg-card p-8 text-center text-pgp-text-muted">
          No users found. Add one to get started.
        </div>
      ) : (
        <div className="overflow-hidden rounded-lg border border-pgp-border">
          <table className="w-full">
            <thead className="bg-pgp-bg-secondary">
              <tr>
                <th className="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-pgp-text-muted">
                  Username
                </th>
                <th className="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-pgp-text-muted">
                  Role
                </th>
                <th className="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-pgp-text-muted">
                  Status
                </th>
                <th className="px-4 py-3 text-right text-xs font-medium uppercase tracking-wider text-pgp-text-muted">
                  Actions
                </th>
              </tr>
            </thead>
            <tbody className="divide-y divide-pgp-border">
              {users.map((user) => {
                const isSelf = currentUser?.id === user.id
                const isLastSuperAdmin =
                  user.role === 'super_admin' && user.active && activeSuperAdminCount <= 1

                return (
                  <tr key={user.id} className="hover:bg-pgp-bg-hover">
                    <td className="px-4 py-3 text-sm text-pgp-text-primary">
                      {user.username}
                      {isSelf && (
                        <span className="ml-2 text-xs text-pgp-text-muted">(you)</span>
                      )}
                    </td>
                    <td className="px-4 py-3">
                      <span
                        className={`inline-flex rounded-full px-2 py-0.5 text-xs font-medium ${ROLE_COLORS[user.role] || ''}`}
                      >
                        {ROLE_LABELS[user.role] || user.role}
                      </span>
                    </td>
                    <td className="px-4 py-3">
                      <span
                        className={`inline-flex rounded-full px-2 py-0.5 text-xs font-medium ${
                          user.active
                            ? 'bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400'
                            : 'bg-gray-100 text-gray-600 dark:bg-gray-800/50 dark:text-gray-400'
                        }`}
                      >
                        {user.active ? 'Active' : 'Inactive'}
                      </span>
                    </td>
                    <td className="px-4 py-3 text-right">
                      <div className="flex items-center justify-end gap-2">
                        <button
                          onClick={() => setFormModal({ mode: 'edit', user })}
                          disabled={isSelf}
                          className="rounded p-1 text-pgp-text-muted hover:bg-pgp-bg-hover hover:text-pgp-text-primary disabled:cursor-not-allowed disabled:opacity-30"
                          title={isSelf ? 'Cannot edit your own account' : 'Edit'}
                        >
                          <Pencil className="h-4 w-4" />
                        </button>
                        <button
                          onClick={() => setResetModal(user)}
                          className="rounded p-1 text-pgp-text-muted hover:bg-pgp-bg-hover hover:text-pgp-text-primary"
                          title="Reset password"
                        >
                          <KeyRound className="h-4 w-4" />
                        </button>
                        <button
                          onClick={() => setDeleteModal(user)}
                          disabled={isSelf || isLastSuperAdmin}
                          className="rounded p-1 text-pgp-text-muted hover:bg-pgp-bg-hover hover:text-red-400 disabled:cursor-not-allowed disabled:opacity-30"
                          title={
                            isSelf
                              ? 'Cannot delete your own account'
                              : isLastSuperAdmin
                                ? 'Cannot delete the last active super admin'
                                : 'Delete'
                          }
                        >
                          <Trash2 className="h-4 w-4" />
                        </button>
                      </div>
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      )}

      {formModal && (
        <UserFormModal
          mode={formModal.mode}
          user={formModal.user}
          onClose={() => setFormModal(null)}
        />
      )}
      {deleteModal && (
        <DeleteUserModal user={deleteModal} onClose={() => setDeleteModal(null)} />
      )}
      {resetModal && (
        <ResetPasswordModal user={resetModal} onClose={() => setResetModal(null)} />
      )}
    </div>
  )
}
