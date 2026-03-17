import { useState, useMemo } from 'react'
import { Lightbulb } from 'lucide-react'
import { PageHeader } from '@/components/ui/PageHeader'
import { Spinner } from '@/components/ui/Spinner'
import { EmptyState } from '@/components/ui/EmptyState'
import { AdvisorFilters } from '@/components/advisor/AdvisorFilters'
import { AdvisorRow } from '@/components/advisor/AdvisorRow'
import { useRecommendations } from '@/hooks/useRecommendations'
import { useInstances } from '@/hooks/useInstances'

function formatRelativeTime(isoString: string): string {
  const now = Date.now()
  const then = new Date(isoString).getTime()
  const diffSec = Math.floor((now - then) / 1000)
  if (diffSec < 60) return 'just now'
  const diffMin = Math.floor(diffSec / 60)
  if (diffMin < 60) return `${diffMin} minute${diffMin === 1 ? '' : 's'} ago`
  const diffHrs = Math.floor(diffMin / 60)
  if (diffHrs < 24) return `${diffHrs} hour${diffHrs === 1 ? '' : 's'} ago`
  const diffDays = Math.floor(diffHrs / 24)
  return `${diffDays} day${diffDays === 1 ? '' : 's'} ago`
}

export function Advisor() {
  const [priority, setPriority] = useState('')
  const [category, setCategory] = useState('')
  const [status, setStatus] = useState('active')
  const [instanceId, setInstanceId] = useState('')

  const { data: recs, isLoading } = useRecommendations({
    priority: priority || undefined,
    category: category || undefined,
    status: status || undefined,
    instanceId: instanceId || undefined,
  })
  const { data: instances } = useInstances()

  const filtered = recs ?? []
  const count = filtered.length

  const lastEvaluated = useMemo(() => {
    if (!recs || recs.length === 0) return null
    return recs.reduce((latest, r) => {
      if (!r.evaluated_at) return latest
      return !latest || r.evaluated_at > latest ? r.evaluated_at : latest
    }, null as string | null)
  }, [recs])

  return (
    <div className="mx-auto max-w-7xl">
      <PageHeader
        title="Advisor"
        subtitle="Actionable recommendations based on metric analysis"
        actions={
          <div className="flex items-center gap-3">
            {lastEvaluated && (
              <span className="text-xs text-pgp-text-muted">
                Last evaluated: {formatRelativeTime(lastEvaluated)}
              </span>
            )}
            {count > 0 && (
              <span className="inline-flex items-center rounded-full bg-amber-500/20 px-2.5 py-1 text-sm font-medium text-amber-400">
                {count}
              </span>
            )}
          </div>
        }
      />

      <div className="space-y-4">
        <AdvisorFilters
          priority={priority}
          onPriorityChange={setPriority}
          category={category}
          onCategoryChange={setCategory}
          status={status}
          onStatusChange={setStatus}
          instanceId={instanceId}
          onInstanceChange={setInstanceId}
          instances={instances ?? []}
        />

        <div className="rounded-lg border border-pgp-border bg-pgp-bg-card">
          {isLoading ? (
            <div className="flex justify-center py-12">
              <Spinner size="lg" />
            </div>
          ) : !filtered.length ? (
            <EmptyState
              icon={Lightbulb}
              title="No recommendations"
              description="No recommendations matching your filters. Run Diagnose on a server to generate new ones."
            />
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-left">
                <thead>
                  <tr className="border-b border-pgp-border">
                    <th className="w-8 px-4 py-3" />
                    <th className="px-4 py-3 text-xs font-medium uppercase text-pgp-text-muted">Priority</th>
                    <th className="px-4 py-3 text-xs font-medium uppercase text-pgp-text-muted">Title</th>
                    <th className="px-4 py-3 text-xs font-medium uppercase text-pgp-text-muted">Category</th>
                    <th className="px-4 py-3 text-xs font-medium uppercase text-pgp-text-muted">Instance</th>
                    <th className="px-4 py-3 text-xs font-medium uppercase text-pgp-text-muted">Created</th>
                    <th className="px-4 py-3 text-xs font-medium uppercase text-pgp-text-muted">Status</th>
                  </tr>
                </thead>
                <tbody>
                  {filtered.map((rec) => (
                    <AdvisorRow key={rec.id} rec={rec} />
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
