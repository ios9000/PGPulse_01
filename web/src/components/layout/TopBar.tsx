import { Menu, Search, Bell } from 'lucide-react'
import { useLayoutStore } from '@/stores/layoutStore'
import { ThemeToggle } from '@/components/ui/ThemeToggle'
import { Breadcrumb } from './Breadcrumb'

export function TopBar() {
  const toggleSidebar = useLayoutStore((s) => s.toggleSidebar)

  return (
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
        <div
          className="ml-2 flex h-8 w-8 items-center justify-center rounded-full bg-pgp-accent text-sm font-medium text-white"
          title="User"
        >
          U
        </div>
      </div>
    </header>
  )
}
