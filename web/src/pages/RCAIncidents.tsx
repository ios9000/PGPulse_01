import { useState, useMemo } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { Search, ChevronLeft, ChevronRight } from 'lucide-react'
import { PageHeader } from '@/components/ui/PageHeader'
import { Spinner } from '@/components/ui/Spinner'
import { EmptyState } from '@/components/ui/EmptyState'
import { IncidentRow } from '@/components/rca/IncidentRow'
import { IncidentFilters } from '@/components/rca/IncidentFilters'
import { useRCAIncidents, useInstanceRCAIncidents } from '@/hooks/useRCA'
import { useInstances } from '@/hooks/useInstances'

const PAGE_SIZE = 20

export function RCAIncidents() {
  const { serverId } = useParams<{ serverId?: string }>()
  const navigate = useNavigate()

  const [page, setPage] = useState(0)
  const [filterInstance, setFilterInstance] = useState('')
  const [filterBucket, setFilterBucket] = useState('')
  const [filterTriggerKind, setFilterTriggerKind] = useState('')

  const params = { limit: PAGE_SIZE, offset: page * PAGE_SIZE }

  // Use instance-scoped hook when on a server route, otherwise fleet-wide
  const fleetQuery = useRCAIncidents(serverId ? undefined : params)
  const instanceQuery = useInstanceRCAIncidents(serverId, serverId ? params : undefined)

  const { data: instances } = useInstances()

  const queryResult = serverId ? instanceQuery : fleetQuery
  const { data, isLoading } = queryResult

  const total = data?.total ?? 0

  // Client-side filtering for bucket and trigger kind
  const filtered = useMemo(() => {
    let result = data?.incidents ?? []
    if (filterInstance) {
      result = result.filter((i) => i.instance_id === filterInstance)
    }
    if (filterBucket) {
      result = result.filter((i) => i.confidence_bucket === filterBucket)
    }
    if (filterTriggerKind) {
      const isAuto = filterTriggerKind === 'auto'
      result = result.filter((i) => i.auto_triggered === isAuto)
    }
    return result
  }, [data?.incidents, filterInstance, filterBucket, filterTriggerKind])

  const serverName = serverId
    ? instances?.find((i) => i.id === serverId)?.name ?? serverId
    : undefined

  const title = serverName ? `RCA Incidents \u2014 ${serverName}` : 'RCA Incidents'

  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE))
  const canPrev = page > 0
  const canNext = page < totalPages - 1

  return (
    <div className="mx-auto max-w-7xl">
      <PageHeader
        title={title}
        subtitle="Root cause analysis incidents from metric anomalies"
      />

      <div className="space-y-4">
        {!serverId && (
          <IncidentFilters
            instanceId={filterInstance}
            onInstanceChange={setFilterInstance}
            bucket={filterBucket}
            onBucketChange={setFilterBucket}
            triggerKind={filterTriggerKind}
            onTriggerKindChange={setFilterTriggerKind}
            instances={instances ?? []}
          />
        )}

        <div className="rounded-lg border border-pgp-border bg-pgp-bg-card">
          {isLoading ? (
            <div className="flex justify-center py-12">
              <Spinner size="lg" />
            </div>
          ) : filtered.length === 0 ? (
            <EmptyState
              icon={Search}
              title="No incidents recorded yet"
              description="RCA incidents will appear here when triggered by alerts or manual investigation."
            />
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-left">
                <thead>
                  <tr className="border-b border-pgp-border">
                    <th className="px-4 py-3 text-xs font-medium uppercase text-pgp-text-muted">Time</th>
                    <th className="px-4 py-3 text-xs font-medium uppercase text-pgp-text-muted">Instance</th>
                    <th className="px-4 py-3 text-xs font-medium uppercase text-pgp-text-muted">Trigger</th>
                    <th className="px-4 py-3 text-xs font-medium uppercase text-pgp-text-muted">Summary</th>
                    <th className="px-4 py-3 text-xs font-medium uppercase text-pgp-text-muted">Confidence</th>
                    <th className="px-4 py-3 text-xs font-medium uppercase text-pgp-text-muted">Source</th>
                  </tr>
                </thead>
                <tbody>
                  {filtered.map((incident) => (
                    <IncidentRow
                      key={incident.id}
                      incident={incident}
                      onClick={() =>
                        navigate(`/servers/${incident.instance_id}/rca/incidents/${incident.id}`)
                      }
                    />
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>

        {/* Pagination */}
        {total > PAGE_SIZE && (
          <div className="flex items-center justify-between px-1">
            <span className="text-sm text-pgp-text-muted">
              Showing {page * PAGE_SIZE + 1}–{Math.min((page + 1) * PAGE_SIZE, total)} of {total}
            </span>
            <div className="flex items-center gap-2">
              <button
                onClick={() => setPage((p) => p - 1)}
                disabled={!canPrev}
                className="inline-flex items-center gap-1 rounded-md border border-pgp-border px-3 py-1.5 text-sm text-pgp-text-secondary transition-colors hover:bg-pgp-bg-hover disabled:cursor-not-allowed disabled:opacity-40"
              >
                <ChevronLeft className="h-4 w-4" />
                Prev
              </button>
              <span className="text-sm text-pgp-text-muted">
                Page {page + 1} of {totalPages}
              </span>
              <button
                onClick={() => setPage((p) => p + 1)}
                disabled={!canNext}
                className="inline-flex items-center gap-1 rounded-md border border-pgp-border px-3 py-1.5 text-sm text-pgp-text-secondary transition-colors hover:bg-pgp-bg-hover disabled:cursor-not-allowed disabled:opacity-40"
              >
                Next
                <ChevronRight className="h-4 w-4" />
              </button>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
