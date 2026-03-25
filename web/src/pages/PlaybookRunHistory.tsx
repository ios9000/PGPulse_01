import { useState } from 'react'
import { Link } from 'react-router-dom'
import { PageHeader } from '@/components/ui/PageHeader'
import { Spinner } from '@/components/ui/Spinner'
import { EmptyState } from '@/components/ui/EmptyState'
import { usePlaybookRuns } from '@/hooks/usePlaybooks'
import { formatTimestamp } from '@/lib/formatters'
import type { RunStatus } from '@/types/playbook'

function statusBadge(status: RunStatus) {
  switch (status) {
    case 'completed':
      return <span className="rounded-full bg-green-500/20 px-2 py-0.5 text-xs text-green-400">Completed</span>
    case 'abandoned':
      return <span className="rounded-full bg-gray-500/20 px-2 py-0.5 text-xs text-gray-400">Abandoned</span>
    case 'escalated':
      return <span className="rounded-full bg-amber-500/20 px-2 py-0.5 text-xs text-amber-400">Escalated</span>
    default:
      return <span className="rounded-full bg-blue-500/20 px-2 py-0.5 text-xs text-blue-400">In Progress</span>
  }
}

export function PlaybookRunHistory() {
  const [statusFilter, setStatusFilter] = useState('')
  const { data: runs, isLoading } = usePlaybookRuns({
    status: statusFilter || undefined,
  })

  return (
    <div className="space-y-6">
      <PageHeader title="Playbook Runs" subtitle="History of all playbook executions" />

      <div className="flex items-center gap-3">
        <select
          value={statusFilter}
          onChange={(e) => setStatusFilter(e.target.value)}
          className="rounded-md border border-pgp-border bg-pgp-bg-secondary px-3 py-1.5 text-sm text-pgp-text-primary"
        >
          <option value="">All Statuses</option>
          <option value="in_progress">In Progress</option>
          <option value="completed">Completed</option>
          <option value="abandoned">Abandoned</option>
          <option value="escalated">Escalated</option>
        </select>
      </div>

      {isLoading ? (
        <div className="flex justify-center py-12">
          <Spinner size="lg" />
        </div>
      ) : !runs || runs.length === 0 ? (
        <EmptyState title="No runs found" description="No playbook runs match the current filters." />
      ) : (
        <div className="overflow-x-auto rounded-lg border border-pgp-border">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-pgp-border bg-pgp-bg-secondary">
                <th className="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-pgp-text-muted">
                  Playbook
                </th>
                <th className="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-pgp-text-muted">
                  Instance
                </th>
                <th className="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-pgp-text-muted">
                  Started By
                </th>
                <th className="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-pgp-text-muted">
                  Status
                </th>
                <th className="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-pgp-text-muted">
                  Started
                </th>
                <th className="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-pgp-text-muted">
                  Completed
                </th>
              </tr>
            </thead>
            <tbody>
              {runs.map((run) => (
                <tr key={run.id} className="border-b border-pgp-border hover:bg-pgp-bg-hover">
                  <td className="px-4 py-3">
                    <Link
                      to={`/servers/${run.instance_id}/playbook-runs/${run.id}`}
                      className="font-medium text-pgp-text-primary hover:text-pgp-accent"
                    >
                      {run.playbook_name}
                    </Link>
                    <span className="ml-1 text-xs text-pgp-text-muted">v{run.playbook_version}</span>
                  </td>
                  <td className="px-4 py-3 text-pgp-text-secondary">{run.instance_id}</td>
                  <td className="px-4 py-3 text-pgp-text-secondary">{run.started_by}</td>
                  <td className="px-4 py-3">{statusBadge(run.status)}</td>
                  <td className="px-4 py-3 text-pgp-text-muted">{formatTimestamp(run.started_at)}</td>
                  <td className="px-4 py-3 text-pgp-text-muted">
                    {run.completed_at ? formatTimestamp(run.completed_at) : '-'}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}
