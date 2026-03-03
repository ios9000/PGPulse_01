import { useState } from 'react'
import { UserPlus, Shield, ShieldOff } from 'lucide-react'
import { useUsers, useUpdateUser } from '@/hooks/useUsers'
import { CreateUserModal } from '@/components/admin/CreateUserModal'
import { DeactivateUserDialog } from '@/components/admin/DeactivateUserDialog'
import { useAuthStore } from '@/stores/authStore'

const ROLE_LABELS: Record<string, string> = {
  super_admin: 'Super Admin',
  roles_admin: 'Roles Admin',
  dba: 'DBA',
  app_admin: 'App Admin',
}

const ROLE_COLORS: Record<string, string> = {
  super_admin: 'bg-red-500/20 text-red-400',
  roles_admin: 'bg-purple-500/20 text-purple-400',
  dba: 'bg-blue-500/20 text-blue-400',
  app_admin: 'bg-green-500/20 text-green-400',
}

export function UsersPage() {
  const { data: users, isLoading } = useUsers()
  const updateUser = useUpdateUser()
  const currentUser = useAuthStore((s) => s.user)
  const [showCreateModal, setShowCreateModal] = useState(false)
  const [deactivateUser, setDeactivateUser] = useState<{ id: number; username: string } | null>(null)

  if (isLoading) {
    return <div className="p-6 text-pgp-text-muted">Loading users...</div>
  }

  return (
    <div className="space-y-6 p-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold text-pgp-text-primary">User Management</h1>
        <button
          onClick={() => setShowCreateModal(true)}
          className="flex items-center gap-2 rounded-md bg-pgp-accent px-4 py-2 text-sm font-medium text-white hover:bg-pgp-accent-hover"
        >
          <UserPlus className="h-4 w-4" />
          Create User
        </button>
      </div>

      <div className="overflow-hidden rounded-lg border border-pgp-border">
        <table className="w-full">
          <thead className="bg-pgp-bg-secondary">
            <tr>
              <th className="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-pgp-text-muted">Username</th>
              <th className="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-pgp-text-muted">Role</th>
              <th className="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-pgp-text-muted">Status</th>
              <th className="px-4 py-3 text-right text-xs font-medium uppercase tracking-wider text-pgp-text-muted">Actions</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-pgp-border">
            {users?.map((user) => (
              <tr key={user.id} className="hover:bg-pgp-bg-hover">
                <td className="px-4 py-3 text-sm text-pgp-text-primary">{user.username}</td>
                <td className="px-4 py-3">
                  <span className={`inline-flex rounded-full px-2 py-0.5 text-xs font-medium ${ROLE_COLORS[user.role] || ''}`}>
                    {ROLE_LABELS[user.role] || user.role}
                  </span>
                </td>
                <td className="px-4 py-3">
                  <span className={`inline-flex items-center gap-1 text-xs ${user.active ? 'text-green-400' : 'text-red-400'}`}>
                    {user.active ? <Shield className="h-3 w-3" /> : <ShieldOff className="h-3 w-3" />}
                    {user.active ? 'Active' : 'Inactive'}
                  </span>
                </td>
                <td className="px-4 py-3 text-right">
                  {currentUser?.id !== user.id && (
                    <button
                      onClick={() => {
                        if (user.active) {
                          setDeactivateUser({ id: user.id, username: user.username })
                        } else {
                          updateUser.mutate({ id: user.id, active: true })
                        }
                      }}
                      className="text-xs text-pgp-text-muted hover:text-pgp-text-primary"
                    >
                      {user.active ? 'Deactivate' : 'Activate'}
                    </button>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {showCreateModal && <CreateUserModal onClose={() => setShowCreateModal(false)} />}
      {deactivateUser && (
        <DeactivateUserDialog
          username={deactivateUser.username}
          onConfirm={() => {
            updateUser.mutate({ id: deactivateUser.id, active: false })
            setDeactivateUser(null)
          }}
          onCancel={() => setDeactivateUser(null)}
        />
      )}
    </div>
  )
}
