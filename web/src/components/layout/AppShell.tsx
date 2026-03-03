import { Outlet } from 'react-router-dom'
import { Sidebar } from './Sidebar'
import { TopBar } from './TopBar'
import { StatusBar } from './StatusBar'
import { useLayoutStore } from '@/stores/layoutStore'
import { ErrorBoundary } from '@/components/ui/ErrorBoundary'

export function AppShell() {
  const collapsed = useLayoutStore((s) => s.sidebarCollapsed)

  return (
    <div className="flex h-screen bg-pgp-bg-primary text-pgp-text-primary">
      <Sidebar />
      <div
        className={`flex flex-1 flex-col transition-all duration-200 ${
          collapsed ? 'ml-[64px]' : 'ml-[240px]'
        }`}
      >
        <TopBar />
        <main className="flex-1 overflow-y-auto p-6 pb-14">
          <ErrorBoundary>
            <Outlet />
          </ErrorBoundary>
        </main>
        <StatusBar />
      </div>
    </div>
  )
}
