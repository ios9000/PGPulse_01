import { Search } from 'lucide-react'

interface PlaybookFiltersProps {
  status: string
  category: string
  search: string
  onStatusChange: (status: string) => void
  onCategoryChange: (category: string) => void
  onSearchChange: (search: string) => void
}

const STATUS_OPTIONS = ['all', 'stable', 'draft', 'deprecated']
const CATEGORY_OPTIONS = [
  'all',
  'replication',
  'storage',
  'connections',
  'locks',
  'vacuum',
  'performance',
  'configuration',
  'general',
]

export function PlaybookFilters({
  status,
  category,
  search,
  onStatusChange,
  onCategoryChange,
  onSearchChange,
}: PlaybookFiltersProps) {
  return (
    <div className="flex flex-wrap items-center gap-3">
      <select
        value={status}
        onChange={(e) => onStatusChange(e.target.value)}
        className="rounded-md border border-pgp-border bg-pgp-bg-secondary px-3 py-1.5 text-sm text-pgp-text-primary"
      >
        {STATUS_OPTIONS.map((opt) => (
          <option key={opt} value={opt}>
            {opt === 'all' ? 'All Statuses' : opt.charAt(0).toUpperCase() + opt.slice(1)}
          </option>
        ))}
      </select>

      <select
        value={category}
        onChange={(e) => onCategoryChange(e.target.value)}
        className="rounded-md border border-pgp-border bg-pgp-bg-secondary px-3 py-1.5 text-sm text-pgp-text-primary"
      >
        {CATEGORY_OPTIONS.map((opt) => (
          <option key={opt} value={opt}>
            {opt === 'all' ? 'All Categories' : opt.charAt(0).toUpperCase() + opt.slice(1)}
          </option>
        ))}
      </select>

      <div className="relative">
        <Search className="absolute left-2.5 top-1/2 h-4 w-4 -translate-y-1/2 text-pgp-text-muted" />
        <input
          type="text"
          value={search}
          onChange={(e) => onSearchChange(e.target.value)}
          placeholder="Search playbooks..."
          className="rounded-md border border-pgp-border bg-pgp-bg-secondary py-1.5 pl-8 pr-3 text-sm text-pgp-text-primary placeholder:text-pgp-text-muted"
        />
      </div>
    </div>
  )
}
