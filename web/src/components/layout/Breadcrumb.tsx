import { Link, useLocation } from 'react-router-dom'
import { ChevronRight } from 'lucide-react'

const routeLabels: Record<string, string> = {
  fleet: 'Fleet Overview',
  servers: 'Servers',
  databases: 'Databases',
  alerts: 'Alerts',
  rules: 'Rules',
  admin: 'Administration',
  users: 'Users',
}

export function Breadcrumb() {
  const location = useLocation()
  const segments = location.pathname.split('/').filter(Boolean)

  if (segments.length === 0) return null

  const crumbs = segments.map((segment, index) => {
    const path = '/' + segments.slice(0, index + 1).join('/')
    const label = routeLabels[segment] || segment
    const isLast = index === segments.length - 1

    return { path, label, isLast }
  })

  return (
    <nav className="flex items-center gap-1 text-sm" aria-label="Breadcrumb">
      {crumbs.map((crumb, idx) => (
        <span key={crumb.path} className="flex items-center gap-1">
          {idx > 0 && <ChevronRight className="h-3.5 w-3.5 text-pgp-text-muted" />}
          {crumb.isLast ? (
            <span className="text-pgp-text-primary">{crumb.label}</span>
          ) : (
            <Link
              to={crumb.path}
              className="text-pgp-text-muted hover:text-pgp-text-primary"
            >
              {crumb.label}
            </Link>
          )}
        </span>
      ))}
    </nav>
  )
}
