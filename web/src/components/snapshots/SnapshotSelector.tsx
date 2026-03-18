import type { PGSSSnapshot } from '@/types/models'

interface SnapshotSelectorProps {
  snapshots: PGSSSnapshot[]
  fromId: number | null
  toId: number | null
  onFromChange: (id: number | null) => void
  onToChange: (id: number | null) => void
}

function formatSnapshotLabel(snap: PGSSSnapshot): string {
  const d = new Date(snap.captured_at)
  const label = d.toLocaleString('en-US', {
    month: 'short',
    day: 'numeric',
    year: 'numeric',
    hour: 'numeric',
    minute: '2-digit',
    hour12: true,
  })
  return `${label} (${snap.total_statements} stmts)`
}

export function SnapshotSelector({
  snapshots,
  fromId,
  toId,
  onFromChange,
  onToChange,
}: SnapshotSelectorProps) {
  return (
    <div className="flex flex-wrap items-center gap-3">
      <div className="flex items-center gap-2">
        <label className="text-sm font-medium text-pgp-text-secondary">From</label>
        <select
          value={fromId ?? ''}
          onChange={(e) => {
            const val = e.target.value
            onFromChange(val ? Number(val) : null)
          }}
          className="rounded-md border border-pgp-border bg-pgp-bg-primary px-3 py-1.5 text-sm text-pgp-text-primary focus:border-pgp-accent focus:outline-none focus:ring-1 focus:ring-pgp-accent"
        >
          <option value="">Latest</option>
          {snapshots.map((snap) => (
            <option key={snap.id} value={snap.id}>
              {formatSnapshotLabel(snap)}
            </option>
          ))}
        </select>
      </div>

      <div className="flex items-center gap-2">
        <label className="text-sm font-medium text-pgp-text-secondary">To</label>
        <select
          value={toId ?? ''}
          onChange={(e) => {
            const val = e.target.value
            onToChange(val ? Number(val) : null)
          }}
          className="rounded-md border border-pgp-border bg-pgp-bg-primary px-3 py-1.5 text-sm text-pgp-text-primary focus:border-pgp-accent focus:outline-none focus:ring-1 focus:ring-pgp-accent"
        >
          <option value="">Latest</option>
          {snapshots.map((snap) => (
            <option key={snap.id} value={snap.id}>
              {formatSnapshotLabel(snap)}
            </option>
          ))}
        </select>
      </div>
    </div>
  )
}
