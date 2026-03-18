import type { PGSSReportSummary } from '@/types/models'

interface ReportSummaryCardProps {
  summary: PGSSReportSummary
  duration: string
}

function formatLargeNumber(n: number): string {
  if (n >= 1_000_000_000) return `${(n / 1_000_000_000).toFixed(1)}B`
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`
  return String(n)
}

function formatMs(ms: number): string {
  if (ms < 1000) return `${ms.toFixed(0)} ms`
  if (ms < 60_000) return `${(ms / 1000).toFixed(1)} s`
  if (ms < 3_600_000) return `${(ms / 60_000).toFixed(1)} min`
  return `${(ms / 3_600_000).toFixed(1)} h`
}

export function ReportSummaryCard({ summary, duration }: ReportSummaryCardProps) {
  const items = [
    { label: 'Period', value: duration },
    { label: 'Unique Queries', value: String(summary.unique_queries) },
    { label: 'Total Calls', value: formatLargeNumber(summary.total_calls_delta) },
    { label: 'Total Exec Time', value: formatMs(summary.total_exec_time_delta_ms) },
    { label: 'Total Rows', value: formatLargeNumber(summary.total_rows_delta) },
    { label: 'New Queries', value: String(summary.new_queries) },
    { label: 'Evicted Queries', value: String(summary.evicted_queries) },
  ]

  return (
    <div className="grid grid-cols-2 gap-4 sm:grid-cols-4 lg:grid-cols-7">
      {items.map((item) => (
        <div
          key={item.label}
          className="rounded-lg border border-pgp-border bg-pgp-bg-secondary px-4 py-4 shadow-sm"
        >
          <div className="text-[11px] font-medium uppercase tracking-wider text-pgp-text-muted">
            {item.label}
          </div>
          <div className="mt-1.5 text-xl font-bold tabular-nums text-pgp-text-primary">
            {item.value}
          </div>
        </div>
      ))}
    </div>
  )
}
