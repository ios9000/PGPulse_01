import type { InstanceData } from '@/types/models'

interface AdvisorFiltersProps {
  priority: string
  onPriorityChange: (v: string) => void
  category: string
  onCategoryChange: (v: string) => void
  status: string
  onStatusChange: (v: string) => void
  instanceId: string
  onInstanceChange: (v: string) => void
  instances: InstanceData[]
}

const PRIORITY_OPTIONS = [
  { value: '', label: 'All' },
  { value: 'action_required', label: 'Action Required' },
  { value: 'suggestion', label: 'Suggestion' },
  { value: 'info', label: 'Info' },
]

const CATEGORY_OPTIONS = [
  { value: '', label: 'All' },
  { value: 'performance', label: 'Performance' },
  { value: 'capacity', label: 'Capacity' },
  { value: 'configuration', label: 'Configuration' },
  { value: 'replication', label: 'Replication' },
  { value: 'maintenance', label: 'Maintenance' },
]

const STATUS_OPTIONS = [
  { value: '', label: 'All' },
  { value: 'pending', label: 'Pending' },
  { value: 'acknowledged', label: 'Acknowledged' },
]

function SelectFilter({
  label,
  value,
  onChange,
  options,
}: {
  label: string
  value: string
  onChange: (v: string) => void
  options: { value: string; label: string }[]
}) {
  return (
    <div className="flex items-center gap-2">
      <span className="text-xs font-medium uppercase text-pgp-text-muted">{label}</span>
      <select
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className="rounded-md border border-pgp-border bg-pgp-bg-secondary px-3 py-1.5 text-sm text-pgp-text-primary focus:border-pgp-accent focus:outline-none focus:ring-1 focus:ring-pgp-accent"
      >
        {options.map((opt) => (
          <option key={opt.value} value={opt.value}>
            {opt.label}
          </option>
        ))}
      </select>
    </div>
  )
}

export function AdvisorFilters({
  priority,
  onPriorityChange,
  category,
  onCategoryChange,
  status,
  onStatusChange,
  instanceId,
  onInstanceChange,
  instances,
}: AdvisorFiltersProps) {
  return (
    <div className="flex flex-wrap items-center gap-4 rounded-lg border border-pgp-border bg-pgp-bg-card p-3">
      <SelectFilter label="Priority" value={priority} onChange={onPriorityChange} options={PRIORITY_OPTIONS} />
      <div className="h-6 w-px bg-pgp-border" />
      <SelectFilter label="Category" value={category} onChange={onCategoryChange} options={CATEGORY_OPTIONS} />
      <div className="h-6 w-px bg-pgp-border" />
      <SelectFilter label="Status" value={status} onChange={onStatusChange} options={STATUS_OPTIONS} />
      <div className="h-6 w-px bg-pgp-border" />
      <div className="flex items-center gap-2">
        <span className="text-xs font-medium uppercase text-pgp-text-muted">Instance</span>
        <select
          value={instanceId}
          onChange={(e) => onInstanceChange(e.target.value)}
          className="rounded-md border border-pgp-border bg-pgp-bg-secondary px-3 py-1.5 text-sm text-pgp-text-primary focus:border-pgp-accent focus:outline-none focus:ring-1 focus:ring-pgp-accent"
        >
          <option value="">All instances</option>
          {instances.map((inst) => (
            <option key={inst.id} value={inst.id}>
              {inst.name}
            </option>
          ))}
        </select>
      </div>
    </div>
  )
}
