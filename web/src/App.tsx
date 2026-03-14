import { useEffect } from 'react'
import { Routes, Route, Navigate } from 'react-router-dom'
import * as echarts from 'echarts/core'
import { AppShell } from '@/components/layout/AppShell'
import { ProtectedRoute } from '@/components/auth/ProtectedRoute'
import { PermissionGate } from '@/components/auth/PermissionGate'
import { FleetOverview } from '@/pages/FleetOverview'
import { ServerDetail } from '@/pages/ServerDetail'
import { DatabaseDetail } from '@/pages/DatabaseDetail'
import { AlertsDashboard } from '@/pages/AlertsDashboard'
import { Advisor } from '@/pages/Advisor'
import { AlertRules } from '@/pages/AlertRules'
import { Administration } from '@/pages/Administration'
import { UsersPage } from '@/pages/admin/UsersPage'
import { Login } from '@/pages/Login'
import { QueryPlanViewer } from '@/pages/QueryPlanViewer'
import { SettingsDiff } from '@/pages/SettingsDiff'
import { NotFound } from '@/pages/NotFound'
import { useAuthStore } from '@/stores/authStore'
import { pgpulseTheme } from '@/lib/echartsTheme'
import { SystemModeProvider } from '@/hooks/useSystemMode'

export function App() {
  const initialize = useAuthStore((s) => s.initialize)

  useEffect(() => {
    initialize()
    echarts.registerTheme('pgpulse', pgpulseTheme)
  }, [initialize])

  return (
    <SystemModeProvider>
    <Routes>
      <Route path="/login" element={<Login />} />

      <Route element={<ProtectedRoute />}>
        <Route element={<AppShell />}>
          <Route index element={<Navigate to="/fleet" replace />} />
          <Route path="fleet" element={<FleetOverview />} />
          <Route path="servers/:serverId" element={<ServerDetail />} />
          <Route path="servers/:serverId/databases/:dbName" element={<DatabaseDetail />} />
          <Route path="servers/:serverId/explain" element={<QueryPlanViewer />} />
          <Route path="settings/diff" element={<SettingsDiff />} />
          <Route path="alerts" element={<AlertsDashboard />} />
          <Route path="advisor" element={<Advisor />} />
          <Route path="alerts/rules" element={<AlertRules />} />
          <Route path="admin" element={<Administration />} />
          <Route path="admin/users" element={<PermissionGate permission="user_management"><UsersPage /></PermissionGate>} />
          <Route path="*" element={<NotFound />} />
        </Route>
      </Route>
    </Routes>
    </SystemModeProvider>
  )
}
