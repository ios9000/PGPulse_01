import { useState } from 'react'
import { ChevronDown, ChevronRight } from 'lucide-react'

export interface ExplainNode {
  'Node Type': string
  'Startup Cost'?: number
  'Total Cost'?: number
  'Actual Total Time'?: number
  'Plan Rows'?: number
  'Actual Rows'?: number
  'Plan Width'?: number
  Plans?: ExplainNode[]
  [key: string]: unknown
}

interface PlanNodeProps {
  node: ExplainNode
  depth: number
}

function getHighlightClass(node: ExplainNode): string {
  const slowTime = (node['Actual Total Time'] ?? 0) > 100
  const planRows = node['Plan Rows'] ?? 0
  const actualRows = node['Actual Rows']
  const rowError =
    actualRows !== undefined && planRows > 0
      ? Math.max(actualRows / planRows, planRows / actualRows) > 10
      : false

  if (slowTime && rowError) return 'bg-red-500/5 border-l-4 border-l-red-500'
  if (rowError) return 'border-l-4 border-l-red-500'
  if (slowTime) return 'bg-amber-500/5 border-l-4 border-l-amber-500'
  return ''
}

export function PlanNodeView({ node, depth }: PlanNodeProps) {
  const [expanded, setExpanded] = useState(true)
  const hasChildren = node.Plans && node.Plans.length > 0
  const highlight = getHighlightClass(node)

  return (
    <div>
      <div
        className={`flex items-start gap-2 rounded px-2 py-1.5 text-sm hover:bg-pgp-bg-hover ${highlight}`}
        style={{ paddingLeft: depth * 24 + 8 }}
      >
        {hasChildren ? (
          <button
            onClick={() => setExpanded(!expanded)}
            className="mt-0.5 shrink-0 text-pgp-text-muted hover:text-pgp-text-secondary"
          >
            {expanded ? (
              <ChevronDown className="h-3.5 w-3.5" />
            ) : (
              <ChevronRight className="h-3.5 w-3.5" />
            )}
          </button>
        ) : (
          <span className="mt-0.5 inline-block h-3.5 w-3.5 shrink-0" />
        )}

        <div className="min-w-0 flex-1">
          <span className="font-medium text-pgp-text-primary">{node['Node Type']}</span>
          <div className="mt-0.5 flex flex-wrap gap-x-4 gap-y-0.5 text-xs text-pgp-text-muted">
            {node['Startup Cost'] !== undefined && node['Total Cost'] !== undefined && (
              <span>Cost: {node['Startup Cost'].toFixed(2)} -&gt; {node['Total Cost'].toFixed(2)}</span>
            )}
            {node['Actual Total Time'] !== undefined && (
              <span className={node['Actual Total Time'] > 100 ? 'text-amber-400' : ''}>
                Time: {node['Actual Total Time'].toFixed(3)}ms
              </span>
            )}
            {node['Plan Rows'] !== undefined && (
              <span>
                Rows: {node['Plan Rows'].toLocaleString()} est
                {node['Actual Rows'] !== undefined && <> / {node['Actual Rows'].toLocaleString()} actual</>}
              </span>
            )}
            {node['Plan Width'] !== undefined && <span>Width: {node['Plan Width']}</span>}
          </div>
        </div>
      </div>

      {expanded &&
        hasChildren &&
        node.Plans!.map((child, i) => <PlanNodeView key={i} node={child} depth={depth + 1} />)}
    </div>
  )
}
