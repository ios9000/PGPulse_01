import { useEffect } from 'react'
import { Routes, Route, Navigate } from 'react-router-dom'
import { AppShell } from '@/components/layout/AppShell'
import { ProtectedRoute } from '@/components/auth/ProtectedRoute'
import { PermissionGate } from '@/components/auth/PermissionGate'
import { FleetOverview } from '@/pages/FleetOverview'
import { ServerDetail } from '@/pages/ServerDetail'
import { DatabaseDetail } from '@/pages/DatabaseDetail'
import { AlertsDashboard } from '@/pages/AlertsDashboard'
import { AlertRules } from '@/pages/AlertRules'
import { Administration } from '@/pages/Administration'
import { UsersPage } from '@/pages/admin/UsersPage'
import { Login } from '@/pages/Login'
import { NotFound } from '@/pages/NotFound'
import { useAuthStore } from '@/stores/authStore'

export function App() {
  const initialize = useAuthStore((s) => s.initialize)

  useEffect(() => {
    initialize()
  }, [initialize])

  return (
    <Routes>
      <Route path="/login" element={<Login />} />

      <Route element={<ProtectedRoute />}>
        <Route element={<AppShell />}>
          <Route index element={<Navigate to="/fleet" replace />} />
          <Route path="fleet" element={<FleetOverview />} />
          <Route path="servers/:serverId" element={<ServerDetail />} />
          <Route path="servers/:serverId/databases/:dbName" element={<DatabaseDetail />} />
          <Route path="alerts" element={<AlertsDashboard />} />
          <Route path="alerts/rules" element={<AlertRules />} />
          <Route path="admin" element={<PermissionGate permission="user_management"><Administration /></PermissionGate>} />
          <Route path="admin/users" element={<PermissionGate permission="user_management"><UsersPage /></PermissionGate>} />
          <Route path="*" element={<NotFound />} />
        </Route>
      </Route>
    </Routes>
  )
}
