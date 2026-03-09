import { useState } from 'react'
import { ChevronDown, ChevronRight } from 'lucide-react'
import { usePlanHistory, usePlanRegressions, usePlanDetail, type CapturedPlan } from '@/hooks/usePlanHistory'
import { PlanNodeView, type ExplainNode } from '@/components/PlanNode'
import { Spinner } from '@/components/ui/Spinner'

interface PlanHistoryProps {
  instanceId: string
}

const TRIGGER_STYLES: Record<string, string> = {
  duration_threshold: 'bg-blue-500/20 text-blue-400',
  scheduled_topn: 'bg-gray-500/20 text-gray-400',
  manual: 'bg-green-500/20 text-green-400',
  hash_diff_signal: 'bg-amber-500/20 text-amber-400',
}

const TRIGGER_LABELS: Record<string, string> = {
  duration_threshold: 'Duration',
  scheduled_topn: 'Scheduled',
  manual: 'Manual',
  hash_diff_signal: 'Hash Diff',
}

type TabKey = 'all' | 'regressions'

export function PlanHistory({ instanceId }: PlanHistoryProps) {
  const [tab, setTab] = useState<TabKey>('all')
  const [expandedId, setExpandedId] = useState<number | null>(null)
  const { data: plans, isLoading: plansLoading } = usePlanHistory(instanceId)
  const { data: regressions, isLoading: regsLoading } = usePlanRegressions(instanceId)
  const { data: detail } = usePlanDetail(instanceId, expandedId)

  const items = tab === 'all' ? plans : regressions
  const loading = tab === 'all' ? plansLoading : regsLoading

  const handleToggle = (id: number) => {
    setExpandedId(expandedId === id ? null : id)
  }

  const parsePlanJson = (text: string | undefined): ExplainNode[] | null => {
    if (!text) return null
    try {
      const parsed = JSON.parse(text)
      // EXPLAIN (FORMAT JSON) returns [{ Plan: { ... } }]
      if (Array.isArray(parsed) && parsed.length > 0 && parsed[0].Plan) {
        return [parsed[0].Plan as ExplainNode]
      }
      if (Array.isArray(parsed)) return parsed as ExplainNode[]
      return [parsed as ExplainNode]
    } catch {
      return null
    }
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div className="flex gap-2">
          <button
            onClick={() => setTab('all')}
            className={`rounded-md px-3 py-1.5 text-sm font-medium transition-colors ${
              tab === 'all'
                ? 'bg-pgp-accent text-white'
                : 'text-pgp-text-muted hover:text-pgp-text-secondary'
            }`}
          >
            All Plans
          </button>
          <button
            onClick={() => setTab('regressions')}
            className={`rounded-md px-3 py-1.5 text-sm font-medium transition-colors ${
              tab === 'regressions'
                ? 'bg-pgp-accent text-white'
                : 'text-pgp-text-muted hover:text-pgp-text-secondary'
            }`}
          >
            Regressions
          </button>
        </div>
      </div>

      {loading ? (
        <div className="flex justify-center py-8"><Spinner size="lg" /></div>
      ) : !items?.length ? (
        <div className="rounded-lg border border-pgp-border bg-pgp-bg-card p-8 text-center text-sm text-pgp-text-muted">
          {tab === 'all'
            ? 'No query plans captured. Enable plan_capture in configuration.'
            : 'No plan regressions detected.'}
        </div>
      ) : (
        <div className="overflow-hidden rounded-lg border border-pgp-border">
          <table className="w-full text-sm">
            <thead className="bg-pgp-bg-card text-pgp-text-secondary">
              <tr>
                <th className="w-8 px-3 py-2"></th>
                <th className="px-3 py-2 text-left text-xs font-medium">Query</th>
                <th className="px-3 py-2 text-left text-xs font-medium">Database</th>
                <th className="px-3 py-2 text-left text-xs font-medium">Trigger</th>
                <th className="px-3 py-2 text-right text-xs font-medium">Duration (ms)</th>
                <th className="px-3 py-2 text-left text-xs font-medium">Captured</th>
              </tr>
            </thead>
            <tbody>
              {items.map((plan) => (
                <PlanRow
                  key={plan.id}
                  plan={plan}
                  isExpanded={expandedId === plan.id}
                  onToggle={() => handleToggle(plan.id)}
                  detail={expandedId === plan.id ? detail : undefined}
                  parsePlanJson={parsePlanJson}
                />
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}

function PlanRow({
  plan,
  isExpanded,
  onToggle,
  detail,
  parsePlanJson,
}: {
  plan: CapturedPlan
  isExpanded: boolean
  onToggle: () => void
  detail: CapturedPlan | undefined
  parsePlanJson: (text: string | undefined) => ExplainNode[] | null
}) {
  const truncatedQuery = plan.query_text.length > 60
    ? plan.query_text.substring(0, 60) + '...'
    : plan.query_text

  const triggerStyle = TRIGGER_STYLES[plan.trigger_type] ?? 'bg-gray-500/20 text-gray-400'
  const triggerLabel = TRIGGER_LABELS[plan.trigger_type] ?? plan.trigger_type

  const planNodes = isExpanded ? parsePlanJson(detail?.plan_text) : null

  return (
    <>
      <tr
        className="cursor-pointer border-t border-pgp-border transition-colors hover:bg-pgp-bg-hover"
        onClick={onToggle}
      >
        <td className="px-3 py-2 text-pgp-text-muted">
          {isExpanded ? <ChevronDown className="h-4 w-4" /> : <ChevronRight className="h-4 w-4" />}
        </td>
        <td className="px-3 py-2">
          <span className="block max-w-md truncate font-mono text-xs text-pgp-text-primary" title={plan.query_text}>
            {truncatedQuery}
          </span>
        </td>
        <td className="px-3 py-2 text-pgp-text-secondary">{plan.database_name}</td>
        <td className="px-3 py-2">
          <span className={`inline-block rounded px-1.5 py-0.5 text-xs font-medium ${triggerStyle}`}>
            {triggerLabel}
          </span>
        </td>
        <td className="px-3 py-2 text-right font-mono text-xs text-pgp-text-secondary">
          {plan.duration_ms > 0 ? plan.duration_ms.toLocaleString() : '\u2014'}
        </td>
        <td className="px-3 py-2 text-xs text-pgp-text-muted">
          {new Date(plan.captured_at).toLocaleString()}
        </td>
      </tr>
      {isExpanded && (
        <tr className="border-t border-pgp-border bg-pgp-bg-card/50">
          <td colSpan={6} className="px-4 py-3">
            <div className="space-y-3">
              <div>
                <div className="text-xs font-medium text-pgp-text-secondary mb-1">Full Query</div>
                <pre className="max-h-[200px] overflow-y-auto rounded bg-pgp-bg-primary p-3 font-mono text-xs text-pgp-text-primary">
                  {detail?.query_text ?? plan.query_text}
                </pre>
              </div>
              {planNodes ? (
                <div>
                  <div className="text-xs font-medium text-pgp-text-secondary mb-1">Execution Plan</div>
                  <div className="rounded border border-pgp-border bg-pgp-bg-primary p-2">
                    {planNodes.map((node, i) => (
                      <PlanNodeView key={i} node={node} depth={0} />
                    ))}
                  </div>
                </div>
              ) : isExpanded && !detail ? (
                <div className="flex justify-center py-2"><Spinner size="sm" /></div>
              ) : (
                <p className="text-xs text-pgp-text-muted">Plan details not available</p>
              )}
            </div>
          </td>
        </tr>
      )}
    </>
  )
}
