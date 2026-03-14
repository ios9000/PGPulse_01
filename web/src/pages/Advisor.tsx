import { useState, useMemo } from 'react'
import { Lightbulb } from 'lucide-react'
import { PageHeader } from '@/components/ui/PageHeader'
import { Spinner } from '@/components/ui/Spinner'
import { EmptyState } from '@/components/ui/EmptyState'
import { AdvisorFilters } from '@/components/advisor/AdvisorFilters'
import { AdvisorRow } from '@/components/advisor/AdvisorRow'
import { useRecommendations } from '@/hooks/useRecommendations'
import { useInstances } from '@/hooks/useInstances'

export function Advisor() {
  const [priority, setPriority] = useState('')
  const [category, setCategory] = useState('')
  const [status, setStatus] = useState('')
  const [instanceId, setInstanceId] = useState('')

  const { data: recs, isLoading } = useRecommendations({
    priority: priority || undefined,
    category: category || undefined,
    instanceId: instanceId || undefined,
  })
  const { data: instances } = useInstances()

  const filtered = useMemo(() => {
    if (!recs) return []
    let list = recs
    if (status === 'pending') {
      list = list.filter((r) => !r.acknowledged_at)
    } else if (status === 'acknowledged') {
      list = list.filter((r) => !!r.acknowledged_at)
    }
    return list
  }, [recs, status])

  const count = filtered.length

  return (
    <div>
      <PageHeader
        title="Advisor"
        subtitle="Actionable recommendations based on metric analysis"
        actions={
          count > 0 ? (
            <span className="inline-flex items-center rounded-full bg-amber-500/20 px-2.5 py-1 text-sm font-medium text-amber-400">
              {count}
            </span>
          ) : null
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
