import { Routes, Route, Navigate } from 'react-router-dom'
import { AppShell } from '@/components/layout/AppShell'
import { FleetOverview } from '@/pages/FleetOverview'
import { ServerDetail } from '@/pages/ServerDetail'
import { DatabaseDetail } from '@/pages/DatabaseDetail'
import { AlertsDashboard } from '@/pages/AlertsDashboard'
import { AlertRules } from '@/pages/AlertRules'
import { Administration } from '@/pages/Administration'
import { UserManagement } from '@/pages/UserManagement'
import { Login } from '@/pages/Login'
import { NotFound } from '@/pages/NotFound'

export function App() {
  return (
    <Routes>
      <Route path="/login" element={<Login />} />

      <Route element={<AppShell />}>
        <Route index element={<Navigate to="/fleet" replace />} />
        <Route path="fleet" element={<FleetOverview />} />
        <Route path="servers/:serverId" element={<ServerDetail />} />
        <Route path="servers/:serverId/databases/:dbName" element={<DatabaseDetail />} />
        <Route path="alerts" element={<AlertsDashboard />} />
        <Route path="alerts/rules" element={<AlertRules />} />
        <Route path="admin" element={<Administration />} />
        <Route path="admin/users" element={<UserManagement />} />
        <Route path="*" element={<NotFound />} />
      </Route>
    </Routes>
  )
}
