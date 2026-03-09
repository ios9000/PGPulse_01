import { useState } from 'react'
import { useStatements } from '@/hooks/useStatements'
import { StatementsConfigBar } from './StatementsConfigBar'
import { StatementRow } from './StatementRow'
import { Spinner } from '@/components/ui/Spinner'
import { EmptyState } from '@/components/ui/EmptyState'
import { ApiError } from '@/lib/api'
import { Info } from 'lucide-react'
import type { StatementSortField } from '@/types/models'

interface StatementsSectionProps {
  instanceId: string
}

const SORT_COLUMNS: { field: StatementSortField; label: string }[] = [
  { field: 'total_time', label: 'Total Time' },
  { field: 'io_time', label: 'IO Time' },
  { field: 'cpu_time', label: 'CPU Time' },
  { field: 'calls', label: 'Calls' },
  { field: 'rows', label: 'Rows' },
]

const COLUMN_HEADERS = [
  { label: '#', align: 'right' as const, sortField: undefined },
  { label: 'Query', align: 'left' as const, sortField: undefined },
  { label: 'DB', align: 'left' as const, sortField: undefined },
  { label: 'User', align: 'left' as const, sortField: undefined },
  { label: 'Total Time', align: 'right' as const, sortField: 'total_time' as StatementSortField },
  { label: 'Mean', align: 'right' as const, sortField: undefined },
  { label: 'Calls', align: 'right' as const, sortField: 'calls' as StatementSortField },
  { label: 'Rows', align: 'right' as const, sortField: 'rows' as StatementSortField },
  { label: 'IO Time', align: 'right' as const, sortField: 'io_time' as StatementSortField },
  { label: 'CPU Time', align: 'right' as const, sortField: 'cpu_time' as StatementSortField },
  { label: 'Hit%', align: 'right' as const, sortField: undefined },
]

export function StatementsSection({ instanceId }: StatementsSectionProps) {
  const [sortField, setSortField] = useState<StatementSortField>('total_time')
  const [expandedRow, setExpandedRow] = useState<number | null>(null)
  const { data, isLoading, error } = useStatements(instanceId, sortField)

  const isExtensionMissing = error instanceof ApiError &&
    (error.message.includes('EXTENSION_NOT_FOUND') || error.status === 404)

  return (
    <div className="rounded-lg border border-pgp-border bg-pgp-bg-card p-4">
      <h2 className="mb-4 text-lg font-semibold text-pgp-text-primary">
        Top Queries (pg_stat_statements)
      </h2>

      {isExtensionMissing ? (
        <div className="flex items-center gap-2 rounded-lg bg-blue-500/10 px-4 py-3 text-sm text-blue-400">
          <Info className="h-4 w-4 shrink-0" />
          <span>pg_stat_statements extension is not enabled on this instance</span>
        </div>
      ) : isLoading ? (
        <div className="flex justify-center py-8"><Spinner size="lg" /></div>
      ) : !data?.statements?.length ? (
        <EmptyState title="No statement data available" />
      ) : (
        <>
          <StatementsConfigBar config={data.config} />

          <div className="mb-3 flex items-center gap-2 text-xs text-pgp-text-muted">
            <span>Sort by:</span>
            {SORT_COLUMNS.map(({ field, label }) => (
              <button
                key={field}
                onClick={() => { setSortField(field); setExpandedRow(null) }}
                className={`rounded px-2 py-0.5 transition-colors ${
                  sortField === field
                    ? 'bg-pgp-accent/20 text-pgp-accent'
                    : 'hover:bg-pgp-bg-hover hover:text-pgp-text-primary'
                }`}
              >
                {label}
              </button>
            ))}
          </div>

          <div className="overflow-x-auto rounded-lg border border-pgp-border">
            <table className="w-full text-sm">
              <thead className="bg-pgp-bg-secondary text-pgp-text-secondary">
                <tr>
                  {COLUMN_HEADERS.map((col) => (
                    <th
                      key={col.label}
                      className={`px-3 py-2 text-xs font-medium ${
                        col.align === 'right' ? 'text-right' : 'text-left'
                      } ${col.sortField ? 'cursor-pointer hover:text-pgp-text-primary' : ''}`}
                      onClick={() => col.sortField && setSortField(col.sortField)}
                    >
                      {col.label}
                      {col.sortField === sortField && ' \u25BC'}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {data.statements.map((entry, idx) => (
                  <StatementRow
                    key={entry.queryid}
                    entry={entry}
                    index={idx}
                    isExpanded={expandedRow === idx}
                    onToggle={() => setExpandedRow(expandedRow === idx ? null : idx)}
                    instanceId={instanceId}
                  />
                ))}
              </tbody>
            </table>
          </div>
        </>
      )}
    </div>
  )
}
