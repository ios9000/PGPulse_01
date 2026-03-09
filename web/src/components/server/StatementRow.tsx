import type { StatementEntry } from '@/types/models'
import { formatPercent } from '@/lib/formatters'
import { ChevronRight, ChevronDown } from 'lucide-react'
import { InlineQueryPlanViewer } from '@/components/InlineQueryPlanViewer'

interface StatementRowProps {
  entry: StatementEntry
  index: number
  isExpanded: boolean
  onToggle: () => void
  instanceId: string
}

function formatTime(ms: number): string {
  if (ms < 1) return `${ms.toFixed(2)}ms`
  if (ms < 1000) return `${ms.toFixed(1)}ms`
  if (ms < 60_000) return `${(ms / 1000).toFixed(1)}s`
  return `${(ms / 60_000).toFixed(1)}min`
}

function formatNumber(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`
  return n.toLocaleString()
}

export function StatementRow({ entry, index, isExpanded, onToggle, instanceId }: StatementRowProps) {
  const truncatedQuery = entry.query_text.length > 80
    ? entry.query_text.substring(0, 80) + '...'
    : entry.query_text

  return (
    <>
      <tr
        className="cursor-pointer border-t border-pgp-border transition-colors hover:bg-pgp-bg-hover"
        onClick={onToggle}
      >
        <td className="px-3 py-2 text-right text-pgp-text-muted">
          <span className="inline-flex items-center gap-1">
            {isExpanded
              ? <ChevronDown className="h-3 w-3" />
              : <ChevronRight className="h-3 w-3" />}
            {index + 1}
          </span>
        </td>
        <td className="max-w-xs px-3 py-2">
          <span className="block truncate font-mono text-xs" title={entry.query_text}>
            {truncatedQuery}
          </span>
        </td>
        <td className="px-3 py-2 text-xs">{entry.dbname}</td>
        <td className="px-3 py-2 text-xs">{entry.username}</td>
        <td className="px-3 py-2 text-right font-mono text-xs">{formatTime(entry.total_exec_time_ms)}</td>
        <td className="px-3 py-2 text-right font-mono text-xs">{formatTime(entry.mean_exec_time_ms)}</td>
        <td className="px-3 py-2 text-right font-mono text-xs">{formatNumber(entry.calls)}</td>
        <td className="px-3 py-2 text-right font-mono text-xs">{formatNumber(entry.rows)}</td>
        <td className="px-3 py-2 text-right font-mono text-xs">{formatTime(entry.io_time_ms)}</td>
        <td className="px-3 py-2 text-right font-mono text-xs">{formatTime(entry.cpu_time_ms)}</td>
        <td className="px-3 py-2 text-right font-mono text-xs">{formatPercent(entry.hit_ratio, 1)}</td>
      </tr>
      {isExpanded && (
        <tr className="border-t border-pgp-border bg-pgp-bg-secondary/50">
          <td colSpan={11} className="px-4 py-3">
            <div className="space-y-3">
              <div className="space-y-2">
                <div className="text-xs font-medium text-pgp-text-secondary">Full Query</div>
                <pre className="max-h-[200px] overflow-y-auto rounded bg-pgp-bg-primary p-3 font-mono text-xs text-pgp-text-primary">
                  {entry.query_text}
                </pre>
                <div className="flex gap-4 text-xs text-pgp-text-muted">
                  <span>Query ID: {entry.queryid}</span>
                  <span>% of total time: {entry.pct_of_total_time.toFixed(1)}%</span>
                  <span>Shared blocks hit: {formatNumber(entry.shared_blks_hit)}</span>
                  <span>Shared blocks read: {formatNumber(entry.shared_blks_read)}</span>
                </div>
              </div>

              <div className="border-t border-pgp-border pt-3">
                <InlineQueryPlanViewer
                  instanceId={instanceId}
                  queryId={String(entry.queryid)}
                />
              </div>
            </div>
          </td>
        </tr>
      )}
    </>
  )
}
