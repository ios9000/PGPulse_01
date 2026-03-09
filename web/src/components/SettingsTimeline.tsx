import { useState } from 'react'
import { ChevronDown, ChevronRight, Camera, AlertTriangle } from 'lucide-react'
import {
  useSettingsSnapshots,
  useSettingsDiffBetween,
  usePendingRestart,
  useManualSnapshot,
  type SettingChange,
} from '@/hooks/useSettingsTimeline'
import { Spinner } from '@/components/ui/Spinner'
import { useAuth } from '@/hooks/useAuth'
import { toast } from '@/stores/toastStore'

interface SettingsTimelineProps {
  instanceId: string
}

export function SettingsTimeline({ instanceId }: SettingsTimelineProps) {
  const { can } = useAuth()
  const canSnapshot = can('instance_management')

  const { data: snapshots, isLoading: snapshotsLoading } = useSettingsSnapshots(instanceId)
  const { data: pendingRestart } = usePendingRestart(instanceId)
  const manualSnapshot = useManualSnapshot(instanceId)

  const [fromId, setFromId] = useState<number | null>(null)
  const [toId, setToId] = useState<number | null>(null)
  const [showDiff, setShowDiff] = useState(false)
  const [showPending, setShowPending] = useState(false)

  const { data: diffResult, isLoading: diffLoading } = useSettingsDiffBetween(
    instanceId,
    showDiff ? fromId : null,
    showDiff ? toId : null,
  )

  const handleCompare = () => {
    if (fromId === null || toId === null) {
      toast.error('Select two snapshots to compare')
      return
    }
    if (fromId === toId) {
      toast.error('Select two different snapshots')
      return
    }
    setShowPending(false)
    setShowDiff(true)
  }

  const handleTakeSnapshot = async () => {
    try {
      await manualSnapshot.mutateAsync()
      toast.success('Settings snapshot captured')
    } catch {
      toast.error('Failed to capture settings snapshot')
    }
  }

  if (snapshotsLoading) {
    return <div className="flex justify-center py-8"><Spinner size="lg" /></div>
  }

  if (!snapshots?.length) {
    return (
      <div className="rounded-lg border border-pgp-border bg-pgp-bg-card p-8 text-center">
        <p className="text-sm text-pgp-text-muted">No settings snapshots captured yet.</p>
        {canSnapshot && (
          <button
            onClick={handleTakeSnapshot}
            disabled={manualSnapshot.isPending}
            className="mt-4 inline-flex items-center gap-2 rounded-md bg-pgp-accent px-4 py-2 text-sm font-medium text-white hover:opacity-90 disabled:opacity-50"
          >
            <Camera className="h-4 w-4" />
            Take Snapshot
          </button>
        )}
      </div>
    )
  }

  return (
    <div className="space-y-4">
      {/* Controls */}
      <div className="flex flex-wrap items-end gap-4">
        <div className="flex-1 min-w-[200px]">
          <label className="mb-1 block text-xs font-medium text-pgp-text-secondary">Snapshot A (from)</label>
          <select
            value={fromId ?? ''}
            onChange={(e) => { setFromId(e.target.value ? Number(e.target.value) : null); setShowDiff(false) }}
            className="w-full rounded-md border border-pgp-border bg-pgp-bg-card px-3 py-2 text-sm text-pgp-text-primary"
          >
            <option value="">Select snapshot...</option>
            {snapshots.map((s) => (
              <option key={s.id} value={s.id}>
                {new Date(s.captured_at).toLocaleString()} — {s.trigger_type}
              </option>
            ))}
          </select>
        </div>

        <div className="flex-1 min-w-[200px]">
          <label className="mb-1 block text-xs font-medium text-pgp-text-secondary">Snapshot B (to)</label>
          <select
            value={toId ?? ''}
            onChange={(e) => { setToId(e.target.value ? Number(e.target.value) : null); setShowDiff(false) }}
            className="w-full rounded-md border border-pgp-border bg-pgp-bg-card px-3 py-2 text-sm text-pgp-text-primary"
          >
            <option value="">Select snapshot...</option>
            {snapshots.map((s) => (
              <option key={s.id} value={s.id}>
                {new Date(s.captured_at).toLocaleString()} — {s.trigger_type}
              </option>
            ))}
          </select>
        </div>

        <button
          onClick={handleCompare}
          disabled={fromId === null || toId === null}
          className="rounded-md bg-pgp-accent px-4 py-2 text-sm font-medium text-white hover:opacity-90 disabled:opacity-50"
        >
          Compare
        </button>

        {pendingRestart && pendingRestart.length > 0 && (
          <button
            onClick={() => { setShowPending(!showPending); setShowDiff(false) }}
            className="inline-flex items-center gap-1.5 rounded-md border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-sm font-medium text-amber-400 hover:bg-amber-500/20"
          >
            <AlertTriangle className="h-4 w-4" />
            Pending Restart ({pendingRestart.length})
          </button>
        )}

        {canSnapshot && (
          <button
            onClick={handleTakeSnapshot}
            disabled={manualSnapshot.isPending}
            className="inline-flex items-center gap-1.5 rounded-md border border-pgp-border bg-pgp-bg-card px-3 py-2 text-sm text-pgp-text-secondary hover:bg-pgp-bg-hover disabled:opacity-50"
          >
            <Camera className="h-4 w-4" />
            {manualSnapshot.isPending ? 'Capturing...' : 'Take Snapshot'}
          </button>
        )}
      </div>

      {/* Snapshot list */}
      <div className="rounded-lg border border-pgp-border">
        <div className="border-b border-pgp-border bg-pgp-bg-card px-4 py-2">
          <span className="text-xs font-medium text-pgp-text-secondary">
            {snapshots.length} snapshot{snapshots.length === 1 ? '' : 's'}
          </span>
        </div>
        <div className="max-h-[200px] overflow-y-auto">
          <table className="w-full text-sm">
            <thead className="sticky top-0 bg-pgp-bg-secondary text-pgp-text-secondary">
              <tr>
                <th className="px-4 py-1.5 text-left text-xs font-medium">ID</th>
                <th className="px-4 py-1.5 text-left text-xs font-medium">Captured At</th>
                <th className="px-4 py-1.5 text-left text-xs font-medium">Trigger</th>
                <th className="px-4 py-1.5 text-left text-xs font-medium">PG Version</th>
              </tr>
            </thead>
            <tbody>
              {snapshots.map((s) => (
                <tr key={s.id} className="border-t border-pgp-border/50 hover:bg-pgp-bg-hover">
                  <td className="px-4 py-1.5 font-mono text-xs text-pgp-text-muted">{s.id}</td>
                  <td className="px-4 py-1.5 text-xs text-pgp-text-primary">
                    {new Date(s.captured_at).toLocaleString()}
                  </td>
                  <td className="px-4 py-1.5 text-xs text-pgp-text-secondary">{s.trigger_type}</td>
                  <td className="px-4 py-1.5 text-xs text-pgp-text-muted">{s.pg_version}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>

      {/* Pending Restart View */}
      {showPending && pendingRestart && pendingRestart.length > 0 && (
        <div className="rounded-lg border border-amber-500/30 bg-amber-500/5 p-4">
          <h3 className="mb-3 text-sm font-medium text-amber-400">Settings Pending Restart</h3>
          <table className="w-full text-sm">
            <thead className="text-pgp-text-secondary">
              <tr>
                <th className="px-3 py-1.5 text-left text-xs font-medium">Setting</th>
                <th className="px-3 py-1.5 text-left text-xs font-medium">Value</th>
                <th className="px-3 py-1.5 text-left text-xs font-medium">Unit</th>
                <th className="px-3 py-1.5 text-left text-xs font-medium">Source</th>
              </tr>
            </thead>
            <tbody>
              {pendingRestart.map((s) => (
                <tr key={s.name} className="border-t border-pgp-border/50">
                  <td className="px-3 py-1.5 font-mono text-xs text-pgp-text-primary">{s.name}</td>
                  <td className="px-3 py-1.5 font-mono text-xs text-pgp-text-primary">{s.new_value}</td>
                  <td className="px-3 py-1.5 text-xs text-pgp-text-muted">{s.unit}</td>
                  <td className="px-3 py-1.5 text-xs text-pgp-text-muted">{s.source}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {/* Diff Results */}
      {showDiff && (
        <div className="space-y-3">
          {diffLoading ? (
            <div className="flex justify-center py-4"><Spinner size="lg" /></div>
          ) : diffResult ? (
            <>
              {diffResult.changed.length === 0 && diffResult.added.length === 0 && diffResult.removed.length === 0 ? (
                <div className="rounded-lg border border-pgp-border bg-pgp-bg-card p-6 text-center text-sm text-pgp-text-muted">
                  No differences between the selected snapshots.
                </div>
              ) : (
                <div className="space-y-3">
                  {diffResult.changed.length > 0 && (
                    <DiffSection
                      title={`Changed (${diffResult.changed.length})`}
                      items={diffResult.changed}
                      type="changed"
                      pendingNames={diffResult.pending_restart}
                    />
                  )}
                  {diffResult.added.length > 0 && (
                    <DiffSection
                      title={`Added (${diffResult.added.length})`}
                      items={diffResult.added}
                      type="added"
                      pendingNames={diffResult.pending_restart}
                    />
                  )}
                  {diffResult.removed.length > 0 && (
                    <DiffSection
                      title={`Removed (${diffResult.removed.length})`}
                      items={diffResult.removed}
                      type="removed"
                      pendingNames={diffResult.pending_restart}
                    />
                  )}
                </div>
              )}
            </>
          ) : null}
        </div>
      )}
    </div>
  )
}

function DiffSection({
  title,
  items,
  type,
  pendingNames,
}: {
  title: string
  items: SettingChange[]
  type: 'changed' | 'added' | 'removed'
  pendingNames: string[]
}) {
  const [open, setOpen] = useState(true)

  const borderClass =
    type === 'added' ? 'border-l-green-500' :
    type === 'removed' ? 'border-l-red-500' :
    'border-l-amber-500'

  return (
    <div className={`rounded-lg border border-pgp-border border-l-4 ${borderClass}`}>
      <button
        onClick={() => setOpen(!open)}
        className="flex w-full items-center justify-between px-4 py-3 text-left hover:bg-pgp-bg-hover"
      >
        <div className="flex items-center gap-2">
          {open ? <ChevronDown className="h-4 w-4 text-pgp-text-muted" /> : <ChevronRight className="h-4 w-4 text-pgp-text-muted" />}
          <span className="text-sm font-medium text-pgp-text-primary">{title}</span>
        </div>
      </button>
      {open && (
        <div className="border-t border-pgp-border">
          <table className="w-full text-sm">
            <thead className="bg-pgp-bg-secondary text-pgp-text-secondary">
              <tr>
                <th className="px-4 py-2 text-left text-xs font-medium">Setting</th>
                <th className="px-4 py-2 text-left text-xs font-medium">
                  {type === 'removed' ? 'Old Value' : type === 'added' ? 'New Value' : 'Value A'}
                </th>
                {type === 'changed' && (
                  <th className="px-4 py-2 text-left text-xs font-medium">Value B</th>
                )}
                <th className="px-4 py-2 text-left text-xs font-medium">Unit</th>
              </tr>
            </thead>
            <tbody>
              {items.map((item) => (
                <tr key={item.name} className="border-t border-pgp-border/50 hover:bg-pgp-bg-hover">
                  <td className="px-4 py-2 font-mono text-xs text-pgp-text-primary">
                    {type === 'removed' ? (
                      <span className="line-through">{item.name}</span>
                    ) : (
                      item.name
                    )}
                    {pendingNames.includes(item.name) && (
                      <span className="ml-2 rounded bg-amber-500/20 px-1.5 py-0.5 text-xs font-medium text-amber-400">
                        Restart Required
                      </span>
                    )}
                  </td>
                  <td className={`px-4 py-2 font-mono text-xs ${
                    type === 'changed' ? 'text-pgp-text-muted' :
                    type === 'removed' ? 'text-red-400 line-through' :
                    'text-green-400'
                  }`}>
                    {type === 'removed' ? item.old_value : type === 'added' ? item.new_value : item.old_value}
                  </td>
                  {type === 'changed' && (
                    <td className="px-4 py-2 font-mono text-xs text-amber-400 bg-amber-500/5">
                      {item.new_value}
                    </td>
                  )}
                  <td className="px-4 py-2 text-xs text-pgp-text-muted">{item.unit}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}
