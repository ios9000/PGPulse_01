import { useState } from 'react'
import { useOperationHistory } from '@/hooks/useMaintenanceForecast'
import { formatDuration, formatBytes, formatTimestamp } from '@/lib/formatters'

interface OperationHistoryTableProps {
  instanceId: string
}

const OUTCOME_COLORS: Record<string, string> = {
  completed: 'text-green-400',
  canceled: 'text-yellow-400',
  failed: 'text-red-400',
  disappeared: 'text-orange-400',
  unknown: 'text-slate-400',
}

export function OperationHistoryTable({ instanceId }: OperationHistoryTableProps) {
  const [page, setPage] = useState(1)
  const perPage = 20

  const { data, isLoading, isError } = useOperationHistory(instanceId, { page, perPage })

  if (isLoading) {
    return <div className="py-4 text-center text-sm text-pgp-text-muted">Loading history...</div>
  }
  if (isError || !data) {
    return null
  }

  const { operations, total } = data
  const totalPages = Math.ceil(total / perPage)

  if (!operations.length) {
    return (
      <div className="py-4 text-center text-sm text-pgp-text-muted">
        No completed operations recorded yet
      </div>
    )
  }

  return (
    <div>
      <div className="overflow-x-auto">
        <table className="w-full text-left text-sm">
          <thead>
            <tr className="border-b border-pgp-border text-xs text-pgp-text-muted">
              <th className="pb-2 pr-3">Operation</th>
              <th className="pb-2 pr-3">Outcome</th>
              <th className="pb-2 pr-3">Database</th>
              <th className="pb-2 pr-3">Table</th>
              <th className="pb-2 pr-3 text-right">Size</th>
              <th className="pb-2 pr-3">Started</th>
              <th className="pb-2 pr-3 text-right">Duration</th>
              <th className="pb-2 text-right">Avg Rate</th>
            </tr>
          </thead>
          <tbody>
            {operations.map((op) => (
              <tr key={op.id} className="border-b border-pgp-border/50 text-pgp-text-primary">
                <td className="py-2 pr-3">{op.operation.replace('_', ' ')}</td>
                <td className={`py-2 pr-3 text-xs font-medium ${OUTCOME_COLORS[op.outcome] ?? ''}`}>
                  {op.outcome}
                </td>
                <td className="py-2 pr-3 font-mono text-xs">{op.database || '—'}</td>
                <td className="py-2 pr-3 font-mono text-xs">{op.table_name || '—'}</td>
                <td className="py-2 pr-3 text-right font-mono text-xs">
                  {op.table_size_bytes > 0 ? formatBytes(op.table_size_bytes) : '—'}
                </td>
                <td className="py-2 pr-3 text-xs text-pgp-text-muted">
                  {formatTimestamp(op.started_at)}
                </td>
                <td className="py-2 pr-3 text-right font-mono text-xs">
                  {formatDuration(op.duration_sec)}
                </td>
                <td className="py-2 text-right font-mono text-xs">
                  {op.avg_rate_per_sec > 0 ? `${op.avg_rate_per_sec.toFixed(1)}/s` : '—'}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {totalPages > 1 && (
        <div className="mt-3 flex items-center justify-between text-xs text-pgp-text-muted">
          <span>{total} operations total</span>
          <div className="flex gap-2">
            <button
              onClick={() => setPage((p) => Math.max(1, p - 1))}
              disabled={page <= 1}
              className="rounded border border-pgp-border px-2 py-1 disabled:opacity-30"
            >
              Prev
            </button>
            <span className="py-1">
              {page} / {totalPages}
            </span>
            <button
              onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
              disabled={page >= totalPages}
              className="rounded border border-pgp-border px-2 py-1 disabled:opacity-30"
            >
              Next
            </button>
          </div>
        </div>
      )}
    </div>
  )
}
