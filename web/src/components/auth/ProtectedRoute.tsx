import { Navigate, Outlet } from 'react-router-dom'
import { useAuthStore } from '@/stores/authStore'
import { Loader2 } from 'lucide-react'

export function ProtectedRoute() {
  const isAuthenticated = useAuthStore((s) => s.isAuthenticated)
  const isLoading = useAuthStore((s) => s.isLoading)

  if (isLoading) {
    return (
      <div className="flex h-screen items-center justify-center bg-pgp-bg-primary">
        <Loader2 className="h-8 w-8 animate-spin text-pgp-accent" />
      </div>
    )
  }

  if (!isAuthenticated) {
    return <Navigate to="/login" replace />
  }

  return <Outlet />
}
