import { useState } from 'react'
import { useMutation } from '@tanstack/react-query'
import { apiFetch } from '@/lib/api'
import { useInstances } from '@/hooks/useInstances'
import { PageHeader } from '@/components/ui/PageHeader'
import { Spinner } from '@/components/ui/Spinner'
import type { SettingsDiffResponse, SettingEntry } from '@/types/models'

function groupByCategory(entries: SettingEntry[]): Record<string, SettingEntry[]> {
  const groups: Record<string, SettingEntry[]> = {}
  for (const entry of entries) {
    const cat = entry.category || 'Uncategorized'
    if (!groups[cat]) groups[cat] = []
    groups[cat].push(entry)
  }
  return groups
}

function exportCsv(data: SettingsDiffResponse) {
  const rows: string[] = [
    ['Section', 'Category', 'Setting', `Value (${data.instance_a.name})`, `Value (${data.instance_b.name})`, 'Unit'].join(','),
  ]

  for (const entry of data.changed) {
    rows.push(['Changed', entry.category, entry.name, entry.value_a ?? '', entry.value_b ?? '', entry.unit ?? ''].map(csvEscape).join(','))
  }
  for (const entry of data.only_in_a) {
    rows.push([`Only in ${data.instance_a.name}`, entry.category, entry.name, entry.value_a ?? '', '', entry.unit ?? ''].map(csvEscape).join(','))
  }
  for (const entry of data.only_in_b) {
    rows.push([`Only in ${data.instance_b.name}`, entry.category, entry.name, '', entry.value_b ?? '', entry.unit ?? ''].map(csvEscape).join(','))
  }

  const blob = new Blob([rows.join('\n')], { type: 'text/csv' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `settings-diff-${data.instance_a.id}-vs-${data.instance_b.id}.csv`
  a.click()
  URL.revokeObjectURL(url)
}

function csvEscape(val: string): string {
  if (val.includes(',') || val.includes('"') || val.includes('\n')) {
    return `"${val.replace(/"/g, '""')}"`
  }
  return val
}

function SettingsTable({ entries, labelA, labelB, showBoth }: {
  entries: SettingEntry[]
  labelA: string
  labelB: string
  showBoth: boolean
}) {
  const grouped = groupByCategory(entries)
  const categories = Object.keys(grouped).sort()

  if (entries.length === 0) {
    return <p className="py-2 text-sm text-pgp-text-muted">None</p>
  }

  return (
    <div className="space-y-4">
      {categories.map((cat) => (
        <div key={cat}>
          <h4 className="mb-2 text-xs font-medium uppercase tracking-wider text-pgp-text-muted">{cat}</h4>
          <div className="overflow-x-auto">
            <table className="w-full text-left text-sm">
              <thead>
                <tr className="border-b border-pgp-border text-pgp-text-muted">
                  <th className="pb-2 pr-4">Setting</th>
                  <th className="pb-2 pr-4">{labelA}</th>
                  {showBoth && <th className="pb-2 pr-4">{labelB}</th>}
                  <th className="pb-2">Unit</th>
                </tr>
              </thead>
              <tbody>
                {grouped[cat].map((entry) => (
                  <tr key={entry.name} className="border-b border-pgp-border/50">
                    <td className="py-1.5 pr-4 font-mono text-pgp-text-primary">{entry.name}</td>
                    <td className="py-1.5 pr-4 text-pgp-text-secondary">{entry.value_a ?? ''}</td>
                    {showBoth && (
                      <td className="py-1.5 pr-4 text-pgp-text-secondary">{entry.value_b ?? ''}</td>
                    )}
                    <td className="py-1.5 text-pgp-text-muted">{entry.unit ?? ''}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      ))}
    </div>
  )
}

function CollapsibleSection({ title, count, children, defaultOpen = false }: {
  title: string
  count: number
  children: React.ReactNode
  defaultOpen?: boolean
}) {
  const [open, setOpen] = useState(defaultOpen)

  return (
    <div className="rounded-lg border border-pgp-border bg-pgp-bg-card">
      <button
        onClick={() => setOpen(!open)}
        className="flex w-full items-center justify-between px-4 py-3 text-left"
      >
        <div className="flex items-center gap-2">
          <svg className={`h-4 w-4 text-pgp-text-muted transition-transform ${open ? 'rotate-90' : ''}`} viewBox="0 0 16 16" fill="currentColor">
            <path d="M6 4l4 4-4 4V4z" />
          </svg>
          <span className="text-sm font-medium text-pgp-text-primary">{title}</span>
        </div>
        <span className="rounded-full bg-pgp-bg-hover px-2 py-0.5 text-xs text-pgp-text-muted">{count}</span>
      </button>
      {open && (
        <div className="border-t border-pgp-border px-4 py-3">
          {children}
        </div>
      )}
    </div>
  )
}

export function SettingsDiff() {
  const { data: instances, isLoading: instancesLoading } = useInstances()
  const [instanceA, setInstanceA] = useState('')
  const [instanceB, setInstanceB] = useState('')

  const compareMutation = useMutation({
    mutationFn: async () => {
      const res = await apiFetch(`/settings/compare?instance_a=${encodeURIComponent(instanceA)}&instance_b=${encodeURIComponent(instanceB)}`)
      const json = await res.json()
      return json.data as SettingsDiffResponse
    },
  })

  return (
    <div>
      <PageHeader
        title="Settings Diff"
        subtitle="Compare PostgreSQL settings between two instances"
        actions={
          compareMutation.data ? (
            <button
              onClick={() => exportCsv(compareMutation.data!)}
              className="rounded border border-pgp-border bg-pgp-bg-card px-3 py-1.5 text-sm text-pgp-text-secondary hover:bg-pgp-bg-hover"
            >
              Export CSV
            </button>
          ) : undefined
        }
      />

      {/* Instance Selectors */}
      <div className="mb-6 rounded-lg border border-pgp-border bg-pgp-bg-card p-4">
        <div className="flex flex-col gap-4 sm:flex-row sm:items-end">
          <div className="flex-1">
            <label className="mb-1.5 block text-sm font-medium text-pgp-text-secondary">Instance A</label>
            {instancesLoading ? (
              <div className="flex h-9 items-center"><Spinner size="sm" /></div>
            ) : (
              <select
                value={instanceA}
                onChange={(e) => setInstanceA(e.target.value)}
                className="w-full rounded-md border border-pgp-border bg-pgp-bg-secondary px-3 py-2 text-sm text-pgp-text-primary focus:border-pgp-accent focus:outline-none"
              >
                <option value="">Select instance...</option>
                {(instances ?? []).map((inst) => (
                  <option key={inst.id} value={inst.id}>{inst.name || inst.id}</option>
                ))}
              </select>
            )}
          </div>

          <div className="flex-1">
            <label className="mb-1.5 block text-sm font-medium text-pgp-text-secondary">Instance B</label>
            {instancesLoading ? (
              <div className="flex h-9 items-center"><Spinner size="sm" /></div>
            ) : (
              <select
                value={instanceB}
                onChange={(e) => setInstanceB(e.target.value)}
                className="w-full rounded-md border border-pgp-border bg-pgp-bg-secondary px-3 py-2 text-sm text-pgp-text-primary focus:border-pgp-accent focus:outline-none"
              >
                <option value="">Select instance...</option>
                {(instances ?? []).map((inst) => (
                  <option key={inst.id} value={inst.id}>{inst.name || inst.id}</option>
                ))}
              </select>
            )}
          </div>

          <button
            onClick={() => compareMutation.mutate()}
            disabled={!instanceA || !instanceB || instanceA === instanceB || compareMutation.isPending}
            className="rounded-md bg-pgp-accent px-4 py-2 text-sm font-medium text-white hover:bg-pgp-accent/80 disabled:cursor-not-allowed disabled:opacity-50"
          >
            {compareMutation.isPending ? 'Comparing...' : 'Compare'}
          </button>
        </div>

        {instanceA && instanceB && instanceA === instanceB && (
          <p className="mt-2 text-xs text-yellow-400">Select two different instances to compare.</p>
        )}
      </div>

      {/* Loading */}
      {compareMutation.isPending && (
        <div className="flex justify-center py-12">
          <Spinner size="lg" />
        </div>
      )}

      {/* Error */}
      {compareMutation.isError && (
        <div className="mb-6 rounded-lg border border-red-500/30 bg-red-500/10 p-4">
          <p className="text-sm text-red-400">
            {compareMutation.error instanceof Error ? compareMutation.error.message : 'Comparison failed'}
          </p>
        </div>
      )}

      {/* Results */}
      {compareMutation.data && (
        <div className="space-y-4">
          {/* Summary */}
          <div className="flex flex-wrap gap-4 text-sm">
            <span className="text-pgp-text-secondary">
              Changed: <span className="font-medium text-yellow-400">{compareMutation.data.changed.length}</span>
            </span>
            <span className="text-pgp-text-secondary">
              Only in {compareMutation.data.instance_a.name}: <span className="font-medium text-pgp-text-primary">{compareMutation.data.only_in_a.length}</span>
            </span>
            <span className="text-pgp-text-secondary">
              Only in {compareMutation.data.instance_b.name}: <span className="font-medium text-pgp-text-primary">{compareMutation.data.only_in_b.length}</span>
            </span>
            <span className="text-pgp-text-secondary">
              Matching: <span className="font-medium text-green-400">{compareMutation.data.matching_count}</span>
            </span>
          </div>

          {/* Changed */}
          <CollapsibleSection title="Changed Settings" count={compareMutation.data.changed.length} defaultOpen={true}>
            <SettingsTable
              entries={compareMutation.data.changed}
              labelA={compareMutation.data.instance_a.name}
              labelB={compareMutation.data.instance_b.name}
              showBoth={true}
            />
          </CollapsibleSection>

          {/* Only in A */}
          <CollapsibleSection title={`Only in ${compareMutation.data.instance_a.name}`} count={compareMutation.data.only_in_a.length}>
            <SettingsTable
              entries={compareMutation.data.only_in_a}
              labelA={compareMutation.data.instance_a.name}
              labelB={compareMutation.data.instance_b.name}
              showBoth={false}
            />
          </CollapsibleSection>

          {/* Only in B */}
          <CollapsibleSection title={`Only in ${compareMutation.data.instance_b.name}`} count={compareMutation.data.only_in_b.length}>
            <SettingsTable
              entries={compareMutation.data.only_in_b}
              labelA={compareMutation.data.instance_b.name}
              labelB={compareMutation.data.instance_a.name}
              showBoth={false}
            />
          </CollapsibleSection>
        </div>
      )}
    </div>
  )
}
