import { useLogicalReplication } from '@/hooks/useLogicalReplication'
import { Spinner } from '@/components/ui/Spinner'
import type { SubscriptionStatus } from '@/types/models'

interface LogicalReplicationSectionProps {
  instanceId: string
}

function syncStateBadge(state: string, label: string) {
  const colors: Record<string, string> = {
    i: 'bg-blue-100 text-blue-800',
    d: 'bg-amber-100 text-amber-800',
    s: 'bg-green-100 text-green-800',
    f: 'bg-teal-100 text-teal-800',
  }
  const cls = colors[state] ?? 'bg-gray-100 text-gray-800'
  return <span className={`inline-block rounded px-1.5 py-0.5 text-xs font-medium ${cls}`}>{label}</span>
}

function SubscriptionCard({ sub }: { sub: SubscriptionStatus }) {
  return (
    <details className="rounded-lg border border-pgp-border bg-pgp-bg-secondary">
      <summary className="cursor-pointer p-3 text-sm font-medium">
        <span className="font-semibold text-pgp-text-primary">{sub.subscription_name}</span>
        <span className="ml-2 rounded bg-blue-100 px-1.5 py-0.5 text-xs text-blue-800">{sub.database}</span>
        {sub.stats && <span className="ml-2 text-pgp-text-muted">PID: {sub.stats.pid}</span>}
        <span className="ml-2 text-pgp-text-muted">
          ({sub.tables_pending.length} table{sub.tables_pending.length !== 1 ? 's' : ''} pending)
        </span>
      </summary>
      <div className="border-t border-pgp-border p-3 space-y-3">
        {sub.stats && (
          <div className="grid grid-cols-3 gap-2 text-sm">
            <div>
              <span className="text-pgp-text-muted">Received LSN: </span>
              <span className="font-mono text-pgp-text-primary">{sub.stats.received_lsn}</span>
            </div>
            <div>
              <span className="text-pgp-text-muted">Latest End LSN: </span>
              <span className="font-mono text-pgp-text-primary">{sub.stats.latest_end_lsn}</span>
            </div>
            <div>
              <span className="text-pgp-text-muted">Latest End Time: </span>
              <span className="text-pgp-text-primary">{sub.stats.latest_end_time}</span>
            </div>
          </div>
        )}

        {sub.stats && ((sub.stats.apply_error_count ?? 0) > 0 || (sub.stats.sync_error_count ?? 0) > 0) && (
          <div className="flex gap-2">
            {(sub.stats.apply_error_count ?? 0) > 0 && (
              <span className="rounded bg-red-100 px-1.5 py-0.5 text-xs font-medium text-red-800">
                Apply Errors: {sub.stats.apply_error_count}
              </span>
            )}
            {(sub.stats.sync_error_count ?? 0) > 0 && (
              <span className="rounded bg-red-100 px-1.5 py-0.5 text-xs font-medium text-red-800">
                Sync Errors: {sub.stats.sync_error_count}
              </span>
            )}
          </div>
        )}

        {sub.tables_pending.length > 0 && (
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-pgp-border text-left text-pgp-text-muted">
                <th className="pb-1">Table Name</th>
                <th className="pb-1">Sync State</th>
                <th className="pb-1">Sync LSN</th>
              </tr>
            </thead>
            <tbody>
              {sub.tables_pending.map((tbl) => (
                <tr key={tbl.table_name} className="border-b border-pgp-border">
                  <td className="py-1 font-mono text-pgp-text-primary">{tbl.table_name}</td>
                  <td className="py-1">{syncStateBadge(tbl.sync_state, tbl.sync_state_label)}</td>
                  <td className="py-1 font-mono text-pgp-text-muted">{tbl.sync_lsn || '--'}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </details>
  )
}

export function LogicalReplicationSection({ instanceId }: LogicalReplicationSectionProps) {
  const { data, isLoading } = useLogicalReplication(instanceId)

  if (isLoading) {
    return (
      <div className="rounded-lg border border-pgp-border bg-pgp-bg-card p-4">
        <h2 className="mb-4 text-lg font-semibold text-pgp-text-primary">Logical Replication</h2>
        <div className="flex justify-center py-8"><Spinner size="lg" /></div>
      </div>
    )
  }

  if (!data || !data.subscriptions || data.subscriptions.length === 0) {
    return (
      <div className="rounded-lg border border-pgp-border bg-pgp-bg-card p-4">
        <h2 className="mb-4 text-lg font-semibold text-pgp-text-primary">Logical Replication</h2>
        <div className="rounded-lg border border-blue-200 bg-blue-50 p-3 text-sm text-blue-800">
          No logical subscriptions configured on this instance
        </div>
      </div>
    )
  }

  if (data.total_pending_tables === 0) {
    return (
      <div className="rounded-lg border border-pgp-border bg-pgp-bg-card p-4">
        <h2 className="mb-4 text-lg font-semibold text-pgp-text-primary">Logical Replication</h2>
        <div className="rounded-lg border border-green-200 bg-green-50 p-3 text-sm text-green-800">
          All logical replication tables synchronized
        </div>
      </div>
    )
  }

  return (
    <div className="rounded-lg border border-pgp-border bg-pgp-bg-card p-4">
      <h2 className="mb-4 text-lg font-semibold text-pgp-text-primary">Logical Replication</h2>
      <p className="mb-3 text-sm text-pgp-text-secondary">
        {data.subscriptions.length} subscription{data.subscriptions.length !== 1 ? 's' : ''},{' '}
        {data.total_pending_tables} table{data.total_pending_tables !== 1 ? 's' : ''} pending sync
      </p>
      <div className="space-y-3">
        {data.subscriptions.map((sub) => (
          <SubscriptionCard key={`${sub.database}-${sub.subscription_name}`} sub={sub} />
        ))}
      </div>
    </div>
  )
}
