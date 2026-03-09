import { useState } from 'react'
import { useParams, Link } from 'react-router-dom'
import { useQuery, useMutation } from '@tanstack/react-query'
import { apiFetch } from '@/lib/api'
import { PageHeader } from '@/components/ui/PageHeader'
import { Spinner } from '@/components/ui/Spinner'
import type { DatabaseSummary, ExplainResponse, PlanNode } from '@/types/models'

function PlanNodeRow({ node, depth = 0 }: { node: PlanNode; depth?: number }) {
  const [expanded, setExpanded] = useState(true)
  const hasChildren = node.Plans && node.Plans.length > 0

  const rowDiscrepancy = node['Actual Rows'] !== undefined && node['Plan Rows'] > 0
    ? Math.abs(node['Actual Rows'] - node['Plan Rows']) / node['Plan Rows']
    : 0
  const costHighlight = node['Total Cost'] > 10000
  const rowHighlight = rowDiscrepancy > 10

  return (
    <div>
      <div
        className={`flex items-start gap-2 rounded px-2 py-1 text-sm hover:bg-pgp-bg-hover ${
          costHighlight || rowHighlight ? 'bg-yellow-500/5' : ''
        }`}
        style={{ paddingLeft: `${depth * 24 + 8}px` }}
      >
        {hasChildren ? (
          <button
            onClick={() => setExpanded(!expanded)}
            className="mt-0.5 shrink-0 text-pgp-text-muted hover:text-pgp-text-secondary"
          >
            <svg className={`h-4 w-4 transition-transform ${expanded ? 'rotate-90' : ''}`} viewBox="0 0 16 16" fill="currentColor">
              <path d="M6 4l4 4-4 4V4z" />
            </svg>
          </button>
        ) : (
          <span className="mt-0.5 inline-block h-4 w-4 shrink-0" />
        )}

        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
            <span className="font-medium text-pgp-text-primary">
              {node['Node Type']}
            </span>
            {node['Relation Name'] && (
              <span className="text-pgp-accent">
                on {node['Relation Name']}{node['Alias'] && node['Alias'] !== node['Relation Name'] ? ` (${node['Alias']})` : ''}
              </span>
            )}
          </div>
          <div className="mt-0.5 flex flex-wrap gap-x-4 gap-y-0.5 text-xs text-pgp-text-muted">
            <span className={costHighlight ? 'text-yellow-400' : ''}>
              cost: {node['Startup Cost'].toFixed(2)}..{node['Total Cost'].toFixed(2)}
            </span>
            <span className={rowHighlight ? 'text-red-400' : ''}>
              rows: {node['Plan Rows'].toLocaleString()}
              {node['Actual Rows'] !== undefined && (
                <> / actual: {node['Actual Rows'].toLocaleString()}</>
              )}
            </span>
            {node['Actual Total Time'] !== undefined && (
              <span>time: {node['Actual Total Time'].toFixed(3)} ms</span>
            )}
            {node['Shared Hit Blocks'] !== undefined && (
              <span>hits: {node['Shared Hit Blocks'].toLocaleString()}</span>
            )}
            {node['Shared Read Blocks'] !== undefined && node['Shared Read Blocks'] > 0 && (
              <span className="text-yellow-400">reads: {node['Shared Read Blocks'].toLocaleString()}</span>
            )}
          </div>
        </div>
      </div>

      {expanded && hasChildren && node.Plans!.map((child, i) => (
        <PlanNodeRow key={i} node={child} depth={depth + 1} />
      ))}
    </div>
  )
}

export function QueryPlanViewer() {
  const { serverId } = useParams()
  const [database, setDatabase] = useState('')
  const [queryText, setQueryText] = useState('')
  const [analyze, setAnalyze] = useState(false)
  const [buffers, setBuffers] = useState(false)
  const [showRawJson, setShowRawJson] = useState(false)

  const { data: databases, isLoading: dbLoading } = useQuery({
    queryKey: ['instances', serverId, 'databases'],
    queryFn: async () => {
      const res = await apiFetch(`/instances/${serverId}/databases`)
      return (await res.json()) as DatabaseSummary[]
    },
    enabled: !!serverId,
  })

  const explainMutation = useMutation({
    mutationFn: async () => {
      const res = await apiFetch(`/instances/${serverId}/explain`, {
        method: 'POST',
        body: JSON.stringify({
          database,
          query: queryText,
          analyze,
          buffers,
        }),
      })
      const json = await res.json()
      return json.data as ExplainResponse
    },
  })

  if (!serverId) return null

  return (
    <div>
      <PageHeader
        title="Explain Query"
        subtitle={`Instance: ${serverId}`}
        actions={
          <Link
            to={`/servers/${serverId}`}
            className="rounded border border-pgp-border bg-pgp-bg-card px-3 py-1.5 text-sm text-pgp-text-secondary hover:bg-pgp-bg-hover"
          >
            Back to Server
          </Link>
        }
      />

      {/* Input Form */}
      <div className="mb-6 rounded-lg border border-pgp-border bg-pgp-bg-card p-4">
        <div className="mb-4 grid grid-cols-1 gap-4 sm:grid-cols-2">
          <div>
            <label className="mb-1.5 block text-sm font-medium text-pgp-text-secondary">Database</label>
            {dbLoading ? (
              <div className="flex h-9 items-center"><Spinner size="sm" /></div>
            ) : (
              <select
                value={database}
                onChange={(e) => setDatabase(e.target.value)}
                className="w-full rounded-md border border-pgp-border bg-pgp-bg-secondary px-3 py-2 text-sm text-pgp-text-primary focus:border-pgp-accent focus:outline-none"
              >
                <option value="">Select database...</option>
                {(databases ?? []).map((db) => (
                  <option key={db.name} value={db.name}>{db.name}</option>
                ))}
              </select>
            )}
          </div>

          <div className="flex items-end gap-4">
            <label className="flex items-center gap-2 text-sm text-pgp-text-secondary">
              <input
                type="checkbox"
                checked={analyze}
                onChange={(e) => setAnalyze(e.target.checked)}
                className="rounded border-pgp-border"
              />
              ANALYZE
            </label>
            <label className="flex items-center gap-2 text-sm text-pgp-text-secondary">
              <input
                type="checkbox"
                checked={buffers}
                onChange={(e) => setBuffers(e.target.checked)}
                className="rounded border-pgp-border"
              />
              BUFFERS
            </label>
          </div>
        </div>

        <div className="mb-4">
          <label className="mb-1.5 block text-sm font-medium text-pgp-text-secondary">SQL Query</label>
          <textarea
            value={queryText}
            onChange={(e) => setQueryText(e.target.value)}
            rows={6}
            placeholder="SELECT * FROM ..."
            className="w-full rounded-md border border-pgp-border bg-pgp-bg-secondary px-3 py-2 font-mono text-sm text-pgp-text-primary placeholder:text-pgp-text-muted focus:border-pgp-accent focus:outline-none"
          />
        </div>

        <div className="flex items-center gap-3">
          <button
            onClick={() => explainMutation.mutate()}
            disabled={!database || !queryText.trim() || explainMutation.isPending}
            className="rounded-md bg-pgp-accent px-4 py-2 text-sm font-medium text-white hover:bg-pgp-accent/80 disabled:cursor-not-allowed disabled:opacity-50"
          >
            {explainMutation.isPending ? 'Running...' : 'Run EXPLAIN'}
          </button>
          {analyze && (
            <span className="text-xs text-yellow-400">
              ANALYZE will execute the query. Use with caution on write operations.
            </span>
          )}
        </div>
      </div>

      {/* Error */}
      {explainMutation.isError && (
        <div className="mb-6 rounded-lg border border-red-500/30 bg-red-500/10 p-4">
          <p className="text-sm text-red-400">
            {explainMutation.error instanceof Error ? explainMutation.error.message : 'Explain failed'}
          </p>
        </div>
      )}

      {/* Results */}
      {explainMutation.data && (
        <div className="space-y-4">
          {/* Timing */}
          {(explainMutation.data.planning_time_ms !== undefined || explainMutation.data.execution_time_ms !== undefined) && (
            <div className="flex gap-4 text-sm">
              {explainMutation.data.planning_time_ms !== undefined && (
                <span className="text-pgp-text-secondary">
                  Planning: <span className="font-medium text-pgp-text-primary">{explainMutation.data.planning_time_ms.toFixed(3)} ms</span>
                </span>
              )}
              {explainMutation.data.execution_time_ms !== undefined && (
                <span className="text-pgp-text-secondary">
                  Execution: <span className="font-medium text-pgp-text-primary">{explainMutation.data.execution_time_ms.toFixed(3)} ms</span>
                </span>
              )}
            </div>
          )}

          {/* Toggle */}
          <div className="flex items-center gap-2">
            <button
              onClick={() => setShowRawJson(false)}
              className={`rounded-md px-3 py-1.5 text-sm ${!showRawJson ? 'bg-pgp-accent text-white' : 'text-pgp-text-secondary hover:bg-pgp-bg-hover'}`}
            >
              Tree View
            </button>
            <button
              onClick={() => setShowRawJson(true)}
              className={`rounded-md px-3 py-1.5 text-sm ${showRawJson ? 'bg-pgp-accent text-white' : 'text-pgp-text-secondary hover:bg-pgp-bg-hover'}`}
            >
              Raw JSON
            </button>
          </div>

          {/* Plan Tree */}
          {!showRawJson ? (
            <div className="rounded-lg border border-pgp-border bg-pgp-bg-card p-4">
              {explainMutation.data.plan_json.map((rootNode, i) => (
                <PlanNodeRow key={i} node={rootNode} />
              ))}
            </div>
          ) : (
            <div className="rounded-lg border border-pgp-border bg-pgp-bg-card p-4">
              <pre className="overflow-x-auto whitespace-pre text-xs text-pgp-text-secondary">
                {JSON.stringify(explainMutation.data.plan_json, null, 2)}
              </pre>
            </div>
          )}
        </div>
      )}
    </div>
  )
}
