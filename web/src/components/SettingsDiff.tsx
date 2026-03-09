import { useState, useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { apiFetch } from '@/lib/api'
import { Spinner } from '@/components/ui/Spinner'
import { EmptyState } from '@/components/ui/EmptyState'
import { ChevronDown, ChevronRight, Download } from 'lucide-react'

interface SettingsDiffProps {
  instanceId: string
}

interface SettingDiff {
  name: string
  current_value: string
  default_value: string
  unit: string
  context: string
  category: string
  pending_restart: boolean
}

function groupByCategory(entries: SettingDiff[]): Record<string, SettingDiff[]> {
  const groups: Record<string, SettingDiff[]> = {}
  for (const entry of entries) {
    const cat = entry.category || 'Uncategorized'
    if (!groups[cat]) groups[cat] = []
    groups[cat].push(entry)
  }
  return groups
}

function csvEscape(val: string): string {
  if (val.includes(',') || val.includes('"') || val.includes('\n')) {
    return `"${val.replace(/"/g, '""')}"`
  }
  return val
}

function exportCsv(data: SettingDiff[]) {
  const header = ['Name', 'Current Value', 'Default Value', 'Unit', 'Context', 'Category', 'Pending Restart']
  const rows = [
    header.join(','),
    ...data.map((d) =>
      [d.name, d.current_value, d.default_value, d.unit, d.context, d.category, String(d.pending_restart)]
        .map(csvEscape)
        .join(','),
    ),
  ]
  const blob = new Blob([rows.join('\n')], { type: 'text/csv' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = 'settings-diff.csv'
  a.click()
  URL.revokeObjectURL(url)
}

function CategorySection({
  category,
  entries,
  defaultOpen,
}: {
  category: string
  entries: SettingDiff[]
  defaultOpen: boolean
}) {
  const [open, setOpen] = useState(defaultOpen)

  return (
    <div className="rounded-lg border border-pgp-border">
      <button
        onClick={() => setOpen(!open)}
        className="flex w-full items-center justify-between px-4 py-3 text-left hover:bg-pgp-bg-hover"
      >
        <div className="flex items-center gap-2">
          {open ? (
            <ChevronDown className="h-4 w-4 text-pgp-text-muted" />
          ) : (
            <ChevronRight className="h-4 w-4 text-pgp-text-muted" />
          )}
          <span className="text-sm font-medium text-pgp-text-primary">{category}</span>
        </div>
        <span className="rounded-full bg-pgp-bg-hover px-2 py-0.5 text-xs text-pgp-text-muted">
          {entries.length}
        </span>
      </button>
      {open && (
        <div className="border-t border-pgp-border">
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead className="bg-pgp-bg-secondary text-pgp-text-secondary">
                <tr>
                  <th className="px-4 py-2 text-left text-xs font-medium">Name</th>
                  <th className="px-4 py-2 text-left text-xs font-medium">Current</th>
                  <th className="px-4 py-2 text-left text-xs font-medium">Default</th>
                  <th className="px-4 py-2 text-left text-xs font-medium">Unit</th>
                  <th className="px-4 py-2 text-left text-xs font-medium">Context</th>
                </tr>
              </thead>
              <tbody>
                {entries.map((entry) => (
                  <tr key={entry.name} className="border-t border-pgp-border/50 hover:bg-pgp-bg-hover">
                    <td className="px-4 py-2 font-mono text-xs text-pgp-text-primary">
                      <span className="flex items-center gap-2">
                        {entry.name}
                        {entry.pending_restart && (
                          <span className="rounded bg-amber-500/20 px-1.5 py-0.5 text-xs font-medium text-amber-400">
                            Restart Required
                          </span>
                        )}
                      </span>
                    </td>
                    <td className="px-4 py-2 font-mono text-xs text-pgp-text-primary">{entry.current_value}</td>
                    <td className="px-4 py-2 font-mono text-xs text-pgp-text-muted">{entry.default_value}</td>
                    <td className="px-4 py-2 text-xs text-pgp-text-muted">{entry.unit}</td>
                    <td className="px-4 py-2 text-xs text-pgp-text-muted">{entry.context}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}
    </div>
  )
}

export function InstanceSettingsDiff({ instanceId }: SettingsDiffProps) {
  const { data, isLoading, error } = useQuery({
    queryKey: ['settings-diff', instanceId],
    queryFn: async () => {
      const res = await apiFetch(`/instances/${instanceId}/settings/diff`)
      const json = await res.json()
      return json.data as SettingDiff[]
    },
    enabled: !!instanceId,
  })

  const grouped = useMemo(() => {
    if (!data?.length) return {}
    return groupByCategory(data)
  }, [data])

  const categories = useMemo(() => Object.keys(grouped).sort(), [grouped])

  if (isLoading) {
    return (
      <div className="flex justify-center py-8">
        <Spinner size="lg" />
      </div>
    )
  }

  if (error) {
    return (
      <div className="rounded-lg border border-red-500/30 bg-red-500/10 p-4">
        <p className="text-sm text-red-400">
          {error instanceof Error ? error.message : 'Failed to load settings diff'}
        </p>
      </div>
    )
  }

  if (!data?.length) {
    return <EmptyState title="All settings match defaults" />
  }

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <span className="text-sm text-pgp-text-muted">
          {data.length} setting{data.length === 1 ? '' : 's'} differ from defaults
        </span>
        <button
          onClick={() => exportCsv(data)}
          className="inline-flex items-center gap-1.5 rounded border border-pgp-border bg-pgp-bg-card px-3 py-1.5 text-sm text-pgp-text-secondary hover:bg-pgp-bg-hover"
        >
          <Download className="h-3.5 w-3.5" />
          Export CSV
        </button>
      </div>

      {categories.map((cat, idx) => (
        <CategorySection
          key={cat}
          category={cat}
          entries={grouped[cat]}
          defaultOpen={idx === 0}
        />
      ))}
    </div>
  )
}
