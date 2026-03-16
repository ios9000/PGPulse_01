import { useState } from 'react'
import { Link, useLocation } from 'react-router-dom'
import { LayoutGrid, Bell, Lightbulb, Settings, Database, GitCompareArrows, ChevronDown, ChevronRight } from 'lucide-react'
import { useLayoutStore } from '@/stores/layoutStore'
import { StatusBadge } from '@/components/ui/StatusBadge'
import type { Permission } from '@/lib/permissions'
import { useAuth } from '@/hooks/useAuth'
import { useInstances } from '@/hooks/useInstances'
import { useActiveRecommendationCount } from '@/hooks/useRecommendations'

interface NavItem {
  label: string
  icon: typeof LayoutGrid
  path: string
  permission?: Permission
}

const navItems: NavItem[] = [
  { label: 'Fleet Overview', icon: LayoutGrid, path: '/fleet' },
  // Alerts is handled separately as an expandable group
  { label: 'Advisor', icon: Lightbulb, path: '/advisor' },
  { label: 'Settings Diff', icon: GitCompareArrows, path: '/settings/diff' },
  { label: 'Administration', icon: Settings, path: '/admin' },
]

const alertSubItems = [
  { label: 'Dashboard', path: '/alerts' },
  { label: 'Rules', path: '/alerts/rules' },
]

export function Sidebar() {
  const collapsed = useLayoutStore((s) => s.sidebarCollapsed)
  const location = useLocation()
  const { can } = useAuth()
  const { data: instances } = useInstances()
  const { data: activeRecs } = useActiveRecommendationCount()
  const activeRecCount = activeRecs?.length ?? 0

  const isActive = (path: string) => location.pathname.startsWith(path)
  const alertsExpanded = isActive('/alerts')
  const [alertsOpen, setAlertsOpen] = useState(alertsExpanded)

  // Keep open when navigating to alerts routes
  const isAlertsOpen = alertsOpen || alertsExpanded

  const visibleNavItems = navItems.filter((item) => {
    if (item.path === '/admin') return can('user_management') || can('instance_management')
    return !item.permission || can(item.permission)
  })

  // Split items: before alerts (Fleet Overview) and after alerts (Advisor, Settings Diff, Admin)
  const beforeAlerts = visibleNavItems.filter((item) => item.path === '/fleet')
  const afterAlerts = visibleNavItems.filter((item) => item.path !== '/fleet')

  const renderNavLink = (item: NavItem) => {
    const active = isActive(item.path)
    const showBadge = item.path === '/advisor' && activeRecCount > 0
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
          {!collapsed && <span className="flex-1">{item.label}</span>}
          {showBadge && (
            <span className="inline-flex h-5 min-w-5 items-center justify-center rounded-full bg-red-500 px-1.5 text-[10px] font-bold text-white">
              {activeRecCount > 99 ? '99+' : activeRecCount}
            </span>
          )}
        </Link>
      </li>
    )
  }

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
          {beforeAlerts.map(renderNavLink)}

          {/* Alerts expandable group */}
          <li>
            {collapsed ? (
              <Link
                to="/alerts"
                className={`flex items-center justify-center rounded-md px-2 py-2 text-sm transition-colors ${
                  alertsExpanded
                    ? 'border-l-2 border-pgp-accent bg-pgp-bg-hover text-pgp-text-primary'
                    : 'text-pgp-text-secondary hover:bg-pgp-bg-hover hover:text-pgp-text-primary'
                }`}
                title="Alerts"
              >
                <Bell className="h-5 w-5 shrink-0" />
              </Link>
            ) : (
              <>
                <button
                  onClick={() => setAlertsOpen(!isAlertsOpen)}
                  className={`flex w-full items-center gap-3 rounded-md px-3 py-2 text-sm transition-colors ${
                    alertsExpanded
                      ? 'border-l-2 border-pgp-accent bg-pgp-bg-hover text-pgp-text-primary'
                      : 'text-pgp-text-secondary hover:bg-pgp-bg-hover hover:text-pgp-text-primary'
                  }`}
                >
                  <Bell className="h-5 w-5 shrink-0" />
                  <span className="flex-1 text-left">Alerts</span>
                  {isAlertsOpen ? (
                    <ChevronDown className="h-4 w-4 shrink-0" />
                  ) : (
                    <ChevronRight className="h-4 w-4 shrink-0" />
                  )}
                </button>
                {isAlertsOpen && (
                  <ul className="ml-5 mt-1 space-y-0.5">
                    {alertSubItems.map((sub) => {
                      const subActive =
                        sub.path === '/alerts'
                          ? location.pathname === '/alerts'
                          : location.pathname.startsWith(sub.path)
                      return (
                        <li key={sub.path}>
                          <Link
                            to={sub.path}
                            className={`block rounded-md px-3 py-1.5 text-xs transition-colors ${
                              subActive
                                ? 'bg-pgp-bg-hover text-pgp-text-primary font-medium'
                                : 'text-pgp-text-secondary hover:bg-pgp-bg-hover hover:text-pgp-text-primary'
                            }`}
                          >
                            {sub.label}
                          </Link>
                        </li>
                      )
                    })}
                  </ul>
                )}
              </>
            )}
          </li>

          {afterAlerts.map(renderNavLink)}
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
            {(instances ?? []).map((server) => (
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
                  title={collapsed ? (server.name || server.id || `${server.host}:${server.port}`) : undefined}
                >
                  <StatusBadge status="ok" size="sm" />
                  {!collapsed && <span className="truncate">{server.name || server.id || `${server.host}:${server.port}`}</span>}
                </Link>
              </li>
            ))}
          </ul>
        </div>
      </nav>
    </aside>
  )
}
