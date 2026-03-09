import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { apiFetch } from '@/lib/api'
import { PlanNodeView, type ExplainNode } from './PlanNode'
import { Spinner } from '@/components/ui/Spinner'

interface InlineQueryPlanViewerProps {
  instanceId: string
  queryId: string
}

export function InlineQueryPlanViewer({ instanceId, queryId }: InlineQueryPlanViewerProps) {
  const [showRawJson, setShowRawJson] = useState(false)

  const { data, isLoading, error } = useQuery({
    queryKey: ['statement-plan', instanceId, queryId],
    queryFn: async () => {
      const res = await apiFetch(`/instances/${instanceId}/statements/${queryId}/plan`)
      const json = await res.json()
      return json.data as { plan_json: ExplainNode[] }
    },
    enabled: !!instanceId && !!queryId,
  })

  if (isLoading) {
    return (
      <div className="flex justify-center py-4">
        <Spinner size="sm" />
      </div>
    )
  }

  if (error || !data?.plan_json?.length) {
    return (
      <p className="py-3 text-sm text-pgp-text-muted">
        Plan unavailable -- the statement may no longer exist
      </p>
    )
  }

  return (
    <div className="space-y-2">
      <div className="flex items-center gap-2">
        <span className="text-xs font-medium text-pgp-text-secondary">Query Plan</span>
        <button
          onClick={() => setShowRawJson(!showRawJson)}
          className="rounded px-2 py-0.5 text-xs text-pgp-text-muted hover:bg-pgp-bg-hover hover:text-pgp-text-secondary"
        >
          {showRawJson ? 'Hide Raw JSON' : 'Show Raw JSON'}
        </button>
      </div>

      {!showRawJson ? (
        <div className="rounded border border-pgp-border bg-pgp-bg-primary p-2">
          {data.plan_json.map((rootNode, i) => (
            <PlanNodeView key={i} node={rootNode} depth={0} />
          ))}
        </div>
      ) : (
        <pre className="mt-2 overflow-auto rounded bg-pgp-bg-secondary p-4 text-xs font-mono text-pgp-text-secondary">
          {JSON.stringify(data.plan_json, null, 2)}
        </pre>
      )}
    </div>
  )
}
