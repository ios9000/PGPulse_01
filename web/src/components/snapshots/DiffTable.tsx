import { useState } from 'react'
import { ChevronUp, ChevronDown } from 'lucide-react'
import type { PGSSDiffEntry } from '@/types/models'
import { formatPercent } from '@/lib/formatters'

type SortField =
  | 'calls_delta'
  | 'exec_time_delta_ms'
  | 'avg_exec_time_per_call_ms'
  | 'rows_delta'
  | 'io_time_pct'
  | 'shared_hit_ratio_pct'

type SortDir = 'asc' | 'desc'

interface DiffTableProps {
  entries: PGSSDiffEntry[]
  compact?: boolean
  onRowClick?: (entry: PGSSDiffEntry) => void
  expandedQueryId?: number | null
  pageSize?: number
}

function formatMs(ms: number): string {
  if (ms < 1) return `${(ms * 1000).toFixed(0)} us`
  if (ms < 1000) return `${ms.toFixed(1)} ms`
  if (ms < 60_000) return `${(ms / 1000).toFixed(2)} s`
  return `${(ms / 60_000).toFixed(1)} min`
}

function formatNumber(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`
  return String(n)
}

const columns: { key: SortField; label: string; shortLabel: string }[] = [
  { key: 'calls_delta', label: 'Calls', shortLabel: 'Calls' },
  { key: 'exec_time_delta_ms', label: 'Exec Time', shortLabel: 'Exec' },
  { key: 'avg_exec_time_per_call_ms', label: 'Avg Time', shortLabel: 'Avg' },
  { key: 'rows_delta', label: 'Rows', shortLabel: 'Rows' },
  { key: 'io_time_pct', label: 'I/O %', shortLabel: 'I/O' },
  { key: 'shared_hit_ratio_pct', label: 'Hit Ratio', shortLabel: 'Hit%' },
]

export function DiffTable({
  entries,
  compact = false,
  onRowClick,
  expandedQueryId,
  pageSize = 20,
}: DiffTableProps) {
  const [sortField, setSortField] = useState<SortField>('exec_time_delta_ms')
  const [sortDir, setSortDir] = useState<SortDir>('desc')
  const [page, setPage] = useState(0)

  const handleSort = (field: SortField) => {
    if (sortField === field) {
      setSortDir(sortDir === 'desc' ? 'asc' : 'desc')
    } else {
      setSortField(field)
      setSortDir('desc')
    }
    setPage(0)
  }

  const sorted = [...entries].sort((a, b) => {
    const av = a[sortField]
    const bv = b[sortField]
    return sortDir === 'desc' ? bv - av : av - bv
  })

  const totalPages = Math.ceil(sorted.length / pageSize)
  const pageEntries = sorted.slice(page * pageSize, (page + 1) * pageSize)

  const renderSortIcon = (field: SortField) => {
    if (sortField !== field) return null
    return sortDir === 'desc' ? (
      <ChevronDown className="ml-0.5 inline h-3 w-3" />
    ) : (
      <ChevronUp className="ml-0.5 inline h-3 w-3" />
    )
  }

  return (
    <div>
      <div className="overflow-x-auto">
        <table className="w-full text-left text-sm">
          <thead>
            <tr className="sticky top-0 border-b-2 border-pgp-border bg-pgp-bg-secondary text-xs font-semibold uppercase tracking-wider text-pgp-text-muted">
              <th className="w-10 px-3 py-3">#</th>
              <th className="min-w-[280px] px-3 py-3">Query</th>
              <th className="px-3 py-3">Database</th>
              {columns.map((col) => (
                <th
                  key={col.key}
                  className="cursor-pointer whitespace-nowrap px-3 py-3 text-right hover:text-pgp-accent"
                  onClick={() => handleSort(col.key)}
                >
                  {col.shortLabel}
                  {renderSortIcon(col.key)}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {pageEntries.map((entry, idx) => {
              const isExpanded = expandedQueryId === entry.queryid
              const rowNum = page * pageSize + idx + 1
              return (
                <tr
                  key={`${entry.queryid}-${entry.dbid}`}
                  className={`border-b border-pgp-border/30 transition-colors ${
                    isExpanded
                      ? 'bg-pgp-bg-hover'
                      : idx % 2 === 1
                        ? 'bg-pgp-bg-primary/50'
                        : ''
                  } ${!compact && onRowClick ? 'cursor-pointer hover:bg-pgp-bg-hover' : 'hover:bg-pgp-bg-hover/50'}`}
                  onClick={() => !compact && onRowClick?.(entry)}
                >
                  <td className="px-3 py-3 text-pgp-text-muted">{rowNum}</td>
                  <td className="max-w-[480px] px-3 py-3">
                    <pre className="overflow-hidden text-ellipsis whitespace-pre-wrap break-words font-mono text-xs leading-relaxed text-pgp-text-primary">
                      {entry.query.length > 120 ? entry.query.slice(0, 120) + '...' : entry.query}
                    </pre>
                  </td>
                  <td className="whitespace-nowrap px-3 py-3 text-pgp-text-secondary">
                    {entry.database_name || '-'}
                  </td>
                  <td className="whitespace-nowrap px-3 py-3 text-right font-mono tabular-nums">
                    {formatNumber(entry.calls_delta)}
                  </td>
                  <td className="whitespace-nowrap px-3 py-3 text-right font-mono tabular-nums">
                    {formatMs(entry.exec_time_delta_ms)}
                  </td>
                  <td className="whitespace-nowrap px-3 py-3 text-right font-mono tabular-nums">
                    {formatMs(entry.avg_exec_time_per_call_ms)}
                  </td>
                  <td className="whitespace-nowrap px-3 py-3 text-right font-mono tabular-nums">
                    {formatNumber(entry.rows_delta)}
                  </td>
                  <td className="whitespace-nowrap px-3 py-3 text-right font-mono tabular-nums">
                    {formatPercent(entry.io_time_pct)}
                  </td>
                  <td className="whitespace-nowrap px-3 py-3 text-right font-mono tabular-nums">
                    {formatPercent(entry.shared_hit_ratio_pct)}
                  </td>
                </tr>
              )
            })}
            {pageEntries.length === 0 && (
              <tr>
                <td colSpan={9} className="px-4 py-8 text-center text-pgp-text-muted">
                  No entries
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      {totalPages > 1 && (
        <div className="mt-3 flex items-center justify-between text-sm">
          <span className="text-pgp-text-muted">
            {sorted.length} entries, page {page + 1} of {totalPages}
          </span>
          <div className="flex gap-2">
            <button
              disabled={page === 0}
              onClick={() => setPage(page - 1)}
              className="rounded border border-pgp-border px-3 py-1 text-pgp-text-secondary hover:bg-pgp-bg-hover disabled:opacity-40"
            >
              Prev
            </button>
            <button
              disabled={page >= totalPages - 1}
              onClick={() => setPage(page + 1)}
              className="rounded border border-pgp-border px-3 py-1 text-pgp-text-secondary hover:bg-pgp-bg-hover disabled:opacity-40"
            >
              Next
            </button>
          </div>
        </div>
      )}
    </div>
  )
}
