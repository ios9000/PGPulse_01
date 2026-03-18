import { useState } from 'react'
import { useParams, Link } from 'react-router-dom'
import { Camera, ArrowLeft } from 'lucide-react'
import { useSnapshots, useLatestDiff, useSnapshotDiff, useManualCapture } from '@/hooks/useSnapshots'
import { useAuth } from '@/hooks/useAuth'
import { SnapshotSelector } from '@/components/snapshots/SnapshotSelector'
import { StatsResetBanner } from '@/components/snapshots/StatsResetBanner'
import { DiffTable } from '@/components/snapshots/DiffTable'
import { QueryDetailPanel } from '@/components/snapshots/QueryDetailPanel'
import type { PGSSDiffEntry } from '@/types/models'

function formatMs(ms: number): string {
  if (ms < 1000) return `${ms.toFixed(0)} ms`
  if (ms < 60_000) return `${(ms / 1000).toFixed(1)} s`
  return `${(ms / 60_000).toFixed(1)} min`
}

function formatNumber(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`
  return String(n)
}

export function QueryInsights() {
  const { serverId = '' } = useParams()
  const { can } = useAuth()

  const [fromId, setFromId] = useState<number | null>(null)
  const [toId, setToId] = useState<number | null>(null)
  const [expandedQueryId, setExpandedQueryId] = useState<number | null>(null)

  const { data: snapshotData, isLoading: snapsLoading } = useSnapshots(serverId, { limit: 50 })
  const snapshots = snapshotData?.snapshots ?? []

  const useLatest = fromId === null || toId === null
  const { data: latestDiff, isLoading: latestLoading } = useLatestDiff(serverId, useLatest)
  const { data: customDiff, isLoading: customLoading } = useSnapshotDiff(
    serverId,
    useLatest ? undefined : fromId,
    useLatest ? undefined : toId,
  )

  const diff = useLatest ? latestDiff : customDiff
  const isLoading = snapsLoading || (useLatest ? latestLoading : customLoading)

  const capture = useManualCapture(serverId)

  const handleRowClick = (entry: PGSSDiffEntry) => {
    setExpandedQueryId(expandedQueryId === entry.queryid ? null : entry.queryid)
  }

  const handleCapture = () => {
    capture.mutate()
  }

  return (
    <div className="mx-auto max-w-7xl space-y-4">
      {/* Header */}
      <div className="flex items-center gap-3">
        <Link
          to={`/servers/${serverId}`}
          className="rounded p-1 text-pgp-text-muted hover:bg-pgp-bg-hover hover:text-pgp-text-primary"
        >
          <ArrowLeft className="h-5 w-5" />
        </Link>
        <h1 className="text-xl font-semibold text-pgp-text-primary">Query Insights</h1>
      </div>

      {/* Controls */}
      <div className="flex flex-wrap items-center justify-between gap-3">
        <SnapshotSelector
          snapshots={snapshots}
          fromId={fromId}
          toId={toId}
          onFromChange={setFromId}
          onToChange={setToId}
        />
        {can('instance_management') && (
          <button
            onClick={handleCapture}
            disabled={capture.isPending}
            className="flex items-center gap-2 rounded-md bg-pgp-accent px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-pgp-accent/80 disabled:opacity-50"
          >
            <Camera className="h-4 w-4" />
            {capture.isPending ? 'Capturing...' : 'Capture Now'}
          </button>
        )}
      </div>

      {/* Empty state */}
      {!isLoading && snapshots.length < 2 && (
        <div className="rounded-lg border border-pgp-border bg-pgp-bg-secondary px-6 py-12 text-center">
          <p className="text-pgp-text-muted">
            At least 2 snapshots are required to show query insights.
          </p>
          {can('instance_management') && (
            <p className="mt-2 text-sm text-pgp-text-muted">
              Click &quot;Capture Now&quot; to create a snapshot.
            </p>
          )}
        </div>
      )}

      {/* Loading */}
      {isLoading && snapshots.length >= 2 && (
        <div className="rounded-lg border border-pgp-border bg-pgp-bg-secondary px-6 py-12 text-center">
          <div className="animate-pulse text-pgp-text-muted">Loading diff...</div>
        </div>
      )}

      {/* Diff content */}
      {diff && (
        <>
          {diff.stats_reset_warning && <StatsResetBanner />}

          {/* Summary */}
          <div className="flex flex-wrap gap-4 text-sm">
            <div className="text-pgp-text-muted">
              <span className="font-medium text-pgp-text-primary">
                {formatNumber(diff.total_calls_delta)}
              </span>{' '}
              calls
            </div>
            <div className="text-pgp-text-muted">
              <span className="font-medium text-pgp-text-primary">
                {formatMs(diff.total_exec_time_delta_ms)}
              </span>{' '}
              total exec
            </div>
            <div className="text-pgp-text-muted">
              <span className="font-medium text-pgp-text-primary">{diff.total_entries}</span>{' '}
              queries
            </div>
          </div>

          {/* Main table */}
          <div className="rounded-lg border border-pgp-border bg-pgp-bg-secondary p-4">
            <h2 className="mb-3 text-sm font-medium text-pgp-text-primary">Queries by Exec Time</h2>
            <DiffTable
              entries={diff.entries}
              onRowClick={handleRowClick}
              expandedQueryId={expandedQueryId}
            />
          </div>

          {/* Query detail panel */}
          {expandedQueryId != null && (
            <QueryDetailPanel
              instanceId={serverId}
              queryId={expandedQueryId}
              onClose={() => setExpandedQueryId(null)}
            />
          )}

          {/* New queries */}
          {diff.new_queries && diff.new_queries.length > 0 && (
            <div className="rounded-lg border border-pgp-border bg-pgp-bg-secondary p-4">
              <h2 className="mb-3 text-sm font-medium text-pgp-text-primary">
                New Queries ({diff.new_queries.length})
              </h2>
              <DiffTable entries={diff.new_queries} compact />
            </div>
          )}

          {/* Evicted queries */}
          {diff.evicted_queries && diff.evicted_queries.length > 0 && (
            <div className="rounded-lg border border-pgp-border bg-pgp-bg-secondary p-4">
              <h2 className="mb-3 text-sm font-medium text-pgp-text-primary">
                Evicted Queries ({diff.evicted_queries.length})
              </h2>
              <DiffTable entries={diff.evicted_queries} compact />
            </div>
          )}
        </>
      )}
    </div>
  )
}
