import { Link, useLocation } from 'react-router-dom'
import { LayoutGrid, Bell, Settings, Database } from 'lucide-react'
import { useLayoutStore } from '@/stores/layoutStore'
import { StatusBadge } from '@/components/ui/StatusBadge'

const navItems = [
  { label: 'Fleet Overview', icon: LayoutGrid, path: '/fleet' },
  { label: 'Alerts', icon: Bell, path: '/alerts' },
  { label: 'Administration', icon: Settings, path: '/admin' },
]

const mockServers = [
  { id: 'prod-1', name: 'prod-primary', status: 'ok' as const },
  { id: 'prod-2', name: 'prod-replica', status: 'ok' as const },
  { id: 'stg-1', name: 'staging', status: 'warning' as const },
]

export function Sidebar() {
  const collapsed = useLayoutStore((s) => s.sidebarCollapsed)
  const location = useLocation()

  const isActive = (path: string) => location.pathname.startsWith(path)

  return (
    <aside
      className={`fixed left-0 top-0 z-40 flex h-screen flex-col border-r border-pgp-border bg-pgp-bg-secondary transition-all duration-200 ${
        collapsed ? 'w-[64px]' : 'w-[240px]'
      }`}
    >
      {/* Logo */}
      <div className="flex h-12 items-center gap-2 border-b border-pgp-border px-4">
        <Database className="h-6 w-6 shrink-0 text-pgp-accent" />
        {!collapsed && (
          <span className="text-lg font-semibold text-pgp-text-primary">PGPulse</span>
        )}
      </div>

      {/* Navigation */}
      <nav className="flex-1 overflow-y-auto py-2">
        <ul className="space-y-1 px-2">
          {navItems.map((item) => {
            const active = isActive(item.path)
            return (
              <li key={item.path}>
                <Link
                  to={item.path}
                  className={`flex items-center gap-3 rounded-md px-3 py-2 text-sm transition-colors ${
                    active
                      ? 'border-l-2 border-pgp-accent bg-pgp-bg-hover text-pgp-text-primary'
                      : 'text-pgp-text-secondary hover:bg-pgp-bg-hover hover:text-pgp-text-primary'
                  } ${collapsed ? 'justify-center px-2' : ''}`}
                  title={collapsed ? item.label : undefined}
                >
                  <item.icon className="h-5 w-5 shrink-0" />
                  {!collapsed && <span>{item.label}</span>}
                </Link>
              </li>
            )
          })}
        </ul>

        {/* Divider */}
        <div className="mx-4 my-3 border-t border-pgp-border" />

        {/* Server tree */}
        <div className="px-2">
          {!collapsed && (
            <p className="mb-2 px-3 text-xs font-medium uppercase tracking-wider text-pgp-text-muted">
              Servers
            </p>
          )}
          <ul className="space-y-1">
            {mockServers.map((server) => (
              <li key={server.id}>
                <Link
                  to={`/servers/${server.id}`}
                  className={`flex items-center gap-2 rounded-md px-3 py-1.5 text-sm text-pgp-text-secondary transition-colors hover:bg-pgp-bg-hover hover:text-pgp-text-primary ${
                    collapsed ? 'justify-center px-2' : ''
                  } ${
                    location.pathname === `/servers/${server.id}`
                      ? 'bg-pgp-bg-hover text-pgp-text-primary'
                      : ''
                  }`}
                  title={collapsed ? server.name : undefined}
                >
                  <StatusBadge status={server.status} size="sm" />
                  {!collapsed && <span className="truncate">{server.name}</span>}
                </Link>
              </li>
            ))}
          </ul>
        </div>
      </nav>
    </aside>
  )
}
