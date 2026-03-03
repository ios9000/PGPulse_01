import { Navigate } from 'react-router-dom'
import { useAuthStore } from '@/stores/authStore'
import type { Permission } from '@/lib/permissions'
import { hasPermission } from '@/lib/permissions'

interface PermissionGateProps {
  permission: Permission
  fallback?: string
  children: React.ReactNode
}

export function PermissionGate({ permission, fallback = '/fleet', children }: PermissionGateProps) {
  const user = useAuthStore((s) => s.user)

  if (!user || !hasPermission(user.role, permission)) {
    return <Navigate to={fallback} replace />
  }

  return <>{children}</>
}
