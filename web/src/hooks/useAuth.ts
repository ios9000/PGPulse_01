import { useAuthStore } from '@/stores/authStore'
import type { Permission } from '@/lib/permissions'
import { hasPermission } from '@/lib/permissions'

export function useAuth() {
  const user = useAuthStore((s) => s.user)
  const isAuthenticated = useAuthStore((s) => s.isAuthenticated)
  const isLoading = useAuthStore((s) => s.isLoading)
  const permissions = useAuthStore((s) => s.permissions)
  const logout = useAuthStore((s) => s.logout)

  const can = (permission: Permission) => {
    if (!user) return false
    return hasPermission(user.role, permission)
  }

  return { user, isAuthenticated, isLoading, permissions, logout, can }
}
