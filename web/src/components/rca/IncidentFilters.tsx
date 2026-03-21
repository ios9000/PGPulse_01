import type { InstanceData } from '@/types/models'

interface IncidentFiltersProps {
  instanceId: string
  onInstanceChange: (v: string) => void
  bucket: string
  onBucketChange: (v: string) => void
  triggerKind: string
  onTriggerKindChange: (v: string) => void
  instances: InstanceData[]
}

const BUCKET_OPTIONS = [
  { value: '', label: 'All' },
  { value: 'high', label: 'High' },
  { value: 'medium', label: 'Medium' },
  { value: 'low', label: 'Low' },
]

const TRIGGER_KIND_OPTIONS = [
  { value: '', label: 'All' },
  { value: 'auto', label: 'Auto' },
  { value: 'manual', label: 'Manual' },
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

export function IncidentFilters({
  instanceId,
  onInstanceChange,
  bucket,
  onBucketChange,
  triggerKind,
  onTriggerKindChange,
  instances,
}: IncidentFiltersProps) {
  return (
    <div className="flex flex-wrap items-center gap-4 rounded-lg border border-pgp-border bg-pgp-bg-card p-3">
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
      <div className="h-6 w-px bg-pgp-border" />
      <SelectFilter label="Confidence" value={bucket} onChange={onBucketChange} options={BUCKET_OPTIONS} />
      <div className="h-6 w-px bg-pgp-border" />
      <SelectFilter label="Trigger" value={triggerKind} onChange={onTriggerKindChange} options={TRIGGER_KIND_OPTIONS} />
    </div>
  )
}
