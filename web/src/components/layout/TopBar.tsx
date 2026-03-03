import { useState, useRef, useEffect } from 'react'
import { Menu, Search, Bell, LogOut, KeyRound, ChevronDown } from 'lucide-react'
import { useLayoutStore } from '@/stores/layoutStore'
import { ThemeToggle } from '@/components/ui/ThemeToggle'
import { Breadcrumb } from './Breadcrumb'
import { useAuth } from '@/hooks/useAuth'
import { ChangePasswordModal } from '@/components/auth/ChangePasswordModal'

const ROLE_LABELS: Record<string, string> = {
  super_admin: 'Super Admin',
  roles_admin: 'Roles Admin',
  dba: 'DBA',
  app_admin: 'App Admin',
}

export function TopBar() {
  const toggleSidebar = useLayoutStore((s) => s.toggleSidebar)
  const { user, logout } = useAuth()
  const [showDropdown, setShowDropdown] = useState(false)
  const [showChangePassword, setShowChangePassword] = useState(false)
  const dropdownRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const handleClickOutside = (e: MouseEvent) => {
      if (dropdownRef.current && !dropdownRef.current.contains(e.target as Node)) {
        setShowDropdown(false)
      }
    }
    document.addEventListener('mousedown', handleClickOutside)
    return () => document.removeEventListener('mousedown', handleClickOutside)
  }, [])

  return (
    <>
      <header className="sticky top-0 z-30 flex h-12 items-center justify-between border-b border-pgp-border bg-pgp-bg-secondary px-4">
        <div className="flex items-center gap-3">
          <button
            onClick={toggleSidebar}
            className="rounded-md p-1.5 text-pgp-text-secondary hover:bg-pgp-bg-hover hover:text-pgp-text-primary"
            aria-label="Toggle sidebar"
          >
            <Menu className="h-5 w-5" />
          </button>
          <Breadcrumb />
        </div>

        <div className="flex items-center gap-1">
          <button
            className="rounded-md p-2 text-pgp-text-secondary hover:bg-pgp-bg-hover hover:text-pgp-text-primary"
            aria-label="Search"
            title="Search"
          >
            <Search className="h-5 w-5" />
          </button>
          <button
            className="rounded-md p-2 text-pgp-text-secondary hover:bg-pgp-bg-hover hover:text-pgp-text-primary"
            aria-label="Notifications"
            title="Notifications"
          >
            <Bell className="h-5 w-5" />
          </button>
          <ThemeToggle />

          {/* User dropdown */}
          <div className="relative ml-2" ref={dropdownRef}>
            <button
              onClick={() => setShowDropdown(!showDropdown)}
              className="flex items-center gap-2 rounded-md px-2 py-1 hover:bg-pgp-bg-hover"
            >
              <div className="flex h-7 w-7 items-center justify-center rounded-full bg-pgp-accent text-xs font-medium text-white">
                {user?.username?.charAt(0).toUpperCase() || 'U'}
              </div>
              <span className="hidden text-sm text-pgp-text-primary sm:inline">{user?.username}</span>
              <ChevronDown className="h-3 w-3 text-pgp-text-muted" />
            </button>

            {showDropdown && (
              <div className="absolute right-0 top-full mt-1 w-56 rounded-md border border-pgp-border bg-pgp-bg-card py-1 shadow-lg">
                <div className="border-b border-pgp-border px-4 py-2">
                  <p className="text-sm font-medium text-pgp-text-primary">{user?.username}</p>
                  <p className="text-xs text-pgp-text-muted">{user?.role ? ROLE_LABELS[user.role] || user.role : ''}</p>
                </div>
                <button
                  onClick={() => { setShowChangePassword(true); setShowDropdown(false) }}
                  className="flex w-full items-center gap-2 px-4 py-2 text-sm text-pgp-text-secondary hover:bg-pgp-bg-hover hover:text-pgp-text-primary"
                >
                  <KeyRound className="h-4 w-4" />
                  Change Password
                </button>
                <button
                  onClick={() => { logout(); setShowDropdown(false) }}
                  className="flex w-full items-center gap-2 px-4 py-2 text-sm text-red-400 hover:bg-pgp-bg-hover"
                >
                  <LogOut className="h-4 w-4" />
                  Sign Out
                </button>
              </div>
            )}
          </div>
        </div>
      </header>

      {showChangePassword && <ChangePasswordModal onClose={() => setShowChangePassword(false)} />}
    </>
  )
}
