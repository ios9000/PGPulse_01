import type { AlertSeverityFilter, AlertStateFilter, InstanceData } from '@/types/models'

interface AlertFiltersProps {
  severity: AlertSeverityFilter
  onSeverityChange: (v: AlertSeverityFilter) => void
  state: AlertStateFilter
  onStateChange: (v: AlertStateFilter) => void
  instanceId: string
  onInstanceChange: (v: string) => void
  instances: InstanceData[]
}

const SEVERITY_OPTIONS: { value: AlertSeverityFilter; label: string }[] = [
  { value: 'all', label: 'All' },
  { value: 'warning', label: 'Warning' },
  { value: 'critical', label: 'Critical' },
]

const STATE_OPTIONS: { value: AlertStateFilter; label: string }[] = [
  { value: 'all', label: 'All' },
  { value: 'firing', label: 'Firing' },
  { value: 'resolved', label: 'Resolved' },
]

function FilterButton<T extends string>({
  value,
  active,
  onClick,
  label,
}: {
  value: T
  active: boolean
  onClick: (v: T) => void
  label: string
}) {
  return (
    <button
      onClick={() => onClick(value)}
      className={`rounded-md px-3 py-1.5 text-sm font-medium transition-colors ${
        active
          ? 'border border-blue-500 bg-blue-500/20 text-blue-400'
          : 'border border-pgp-border bg-pgp-bg-secondary text-pgp-text-secondary hover:bg-pgp-bg-hover hover:text-pgp-text-primary'
      }`}
    >
      {label}
    </button>
  )
}

export function AlertFilters({
  severity,
  onSeverityChange,
  state,
  onStateChange,
  instanceId,
  onInstanceChange,
  instances,
}: AlertFiltersProps) {
  return (
    <div className="flex flex-wrap items-center gap-4 rounded-lg border border-pgp-border bg-pgp-bg-card p-3">
      <div className="flex items-center gap-2">
        <span className="text-xs font-medium uppercase text-pgp-text-muted">Severity</span>
        {SEVERITY_OPTIONS.map((opt) => (
          <FilterButton
            key={opt.value}
            value={opt.value}
            active={severity === opt.value}
            onClick={onSeverityChange}
            label={opt.label}
          />
        ))}
      </div>

      <div className="h-6 w-px bg-pgp-border" />

      <div className="flex items-center gap-2">
        <span className="text-xs font-medium uppercase text-pgp-text-muted">State</span>
        {STATE_OPTIONS.map((opt) => (
          <FilterButton
            key={opt.value}
            value={opt.value}
            active={state === opt.value}
            onClick={onStateChange}
            label={opt.label}
          />
        ))}
      </div>

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
