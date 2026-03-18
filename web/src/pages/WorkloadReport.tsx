import { useState } from 'react'
import { useParams, Link } from 'react-router-dom'
import { ArrowLeft, FileDown, Printer } from 'lucide-react'
import { useSnapshots, useWorkloadReport } from '@/hooks/useSnapshots'
import { apiFetch } from '@/lib/api'
import { SnapshotSelector } from '@/components/snapshots/SnapshotSelector'
import { StatsResetBanner } from '@/components/snapshots/StatsResetBanner'
import { ReportSummaryCard } from '@/components/snapshots/ReportSummaryCard'
import { ReportSection } from '@/components/snapshots/ReportSection'

export function WorkloadReport() {
  const { serverId = '' } = useParams()

  const [fromId, setFromId] = useState<number | null>(null)
  const [toId, setToId] = useState<number | null>(null)
  const [exporting, setExporting] = useState(false)

  const { data: snapshotData, isLoading: snapsLoading } = useSnapshots(serverId, { limit: 50 })
  const snapshots = snapshotData?.snapshots ?? []

  // When both are null, useWorkloadReport won't fire (enabled is false).
  // The backend defaults to latest 2 snapshots when no from/to is provided,
  // so we call with a special case: pass the two most recent snapshot IDs.
  const autoFromId = snapshots.length >= 2 ? snapshots[snapshots.length - 1].id : null
  const autoToId = snapshots.length >= 2 ? snapshots[0].id : null

  const effectiveFromId = fromId ?? autoFromId
  const effectiveToId = toId ?? autoToId

  const { data: report, isLoading: reportLoading } = useWorkloadReport(
    serverId,
    effectiveFromId,
    effectiveToId,
  )

  const isLoading = snapsLoading || reportLoading

  const handleExportHTML = async () => {
    if (!effectiveFromId || !effectiveToId) return
    setExporting(true)
    try {
      const params = new URLSearchParams({
        from: String(effectiveFromId),
        to: String(effectiveToId),
      })
      const res = await apiFetch(`/instances/${serverId}/workload-report/html?${params}`)
      const blob = await res.blob()
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = `workload-report-${serverId}-${new Date().toISOString().slice(0, 10)}.html`
      a.click()
      URL.revokeObjectURL(url)
    } catch (err) {
      console.error('Export failed:', err)
    } finally {
      setExporting(false)
    }
  }

  const handlePrint = () => {
    window.print()
  }

  return (
    <div className="mx-auto max-w-7xl space-y-6">
      {/* Header */}
      <div className="flex items-center gap-3">
        <Link
          to={`/servers/${serverId}`}
          className="rounded p-1 text-pgp-text-muted hover:bg-pgp-bg-hover hover:text-pgp-text-primary"
        >
          <ArrowLeft className="h-5 w-5" />
        </Link>
        <h1 className="text-xl font-semibold text-pgp-text-primary">Workload Report</h1>
      </div>

      {/* Controls */}
      <div className="flex flex-wrap items-center justify-between gap-3 print:hidden">
        <SnapshotSelector
          snapshots={snapshots}
          fromId={fromId}
          toId={toId}
          onFromChange={setFromId}
          onToChange={setToId}
        />
        <div className="flex items-center gap-2">
          <button
            onClick={handleExportHTML}
            disabled={!report || exporting}
            className="flex items-center gap-2 rounded-md border border-pgp-border px-4 py-2 text-sm font-medium text-pgp-text-secondary transition-colors hover:bg-pgp-bg-hover disabled:opacity-50"
          >
            <FileDown className="h-4 w-4" />
            {exporting ? 'Exporting...' : 'Export HTML'}
          </button>
          <button
            onClick={handlePrint}
            disabled={!report}
            className="flex items-center gap-2 rounded-md px-3 py-2 text-sm text-pgp-text-muted transition-colors hover:bg-pgp-bg-hover hover:text-pgp-text-secondary disabled:opacity-50"
          >
            <Printer className="h-4 w-4" />
            Print
          </button>
        </div>
      </div>

      {/* Empty state */}
      {!isLoading && snapshots.length < 2 && (
        <div className="rounded-lg border border-pgp-border bg-pgp-bg-secondary px-6 py-12 text-center">
          <p className="text-pgp-text-muted">
            At least 2 snapshots are required to generate a workload report.
          </p>
        </div>
      )}

      {/* Loading */}
      {isLoading && snapshots.length >= 2 && (
        <div className="rounded-lg border border-pgp-border bg-pgp-bg-secondary px-6 py-12 text-center">
          <div className="animate-pulse text-pgp-text-muted">Generating report...</div>
        </div>
      )}

      {/* Report content */}
      {report && (
        <>
          {report.stats_reset_warning && <StatsResetBanner />}

          <ReportSummaryCard summary={report.summary} duration={report.duration} />

          <div className="space-y-6">
            <ReportSection
              title="Top by Execution Time"
              entries={report.top_by_exec_time}
              defaultExpanded
            />
            <ReportSection title="Top by Calls" entries={report.top_by_calls} />
            <ReportSection title="Top by Rows" entries={report.top_by_rows} />
            <ReportSection title="Top by I/O Reads" entries={report.top_by_io_reads} />
            <ReportSection title="Top by Avg Time" entries={report.top_by_avg_time} />

            {report.new_queries && report.new_queries.length > 0 && (
              <ReportSection title="New Queries" entries={report.new_queries} />
            )}
            {report.evicted_queries && report.evicted_queries.length > 0 && (
              <ReportSection title="Evicted Queries" entries={report.evicted_queries} />
            )}
          </div>
        </>
      )}
    </div>
  )
}
