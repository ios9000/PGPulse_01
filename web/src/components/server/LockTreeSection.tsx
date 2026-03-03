import { useLockTree } from '@/hooks/useLockTree'
import { LockTreeRow } from './LockTreeRow'
import { Spinner } from '@/components/ui/Spinner'
import { AlertTriangle, CheckCircle } from 'lucide-react'

interface LockTreeSectionProps {
  instanceId: string
}

export function LockTreeSection({ instanceId }: LockTreeSectionProps) {
  const { data, isLoading } = useLockTree(instanceId)

  const hasBlocking = data && data.summary.root_blockers > 0

  return (
    <div className="rounded-lg border border-pgp-border bg-pgp-bg-card p-4">
      <h2 className="mb-4 text-lg font-semibold text-pgp-text-primary">Lock Tree</h2>

      {isLoading ? (
        <div className="flex justify-center py-8"><Spinner size="lg" /></div>
      ) : !data?.locks?.length ? (
        <div className="flex items-center gap-2 px-1 py-2 text-sm text-green-400">
          <CheckCircle className="h-4 w-4" />
          <span>No blocking locks detected</span>
        </div>
      ) : (
        <>
          {hasBlocking && (
            <div className="mb-3 flex items-center gap-2 rounded-lg bg-orange-500/10 px-3 py-2 text-sm text-orange-400">
              <AlertTriangle className="h-4 w-4 shrink-0" />
              <span>
                {data.summary.root_blockers} root blocker{data.summary.root_blockers !== 1 ? 's' : ''},
                {' '}{data.summary.total_blocked} blocked session{data.summary.total_blocked !== 1 ? 's' : ''},
                max depth {data.summary.max_depth}
              </span>
            </div>
          )}

          <div className="overflow-x-auto rounded-lg border border-pgp-border">
            <table className="w-full text-sm">
              <thead className="bg-pgp-bg-secondary text-pgp-text-secondary">
                <tr>
                  <th className="px-3 py-2 text-left text-xs font-medium">PID</th>
                  <th className="px-3 py-2 text-left text-xs font-medium">User</th>
                  <th className="px-3 py-2 text-left text-xs font-medium">Database</th>
                  <th className="px-3 py-2 text-left text-xs font-medium">State</th>
                  <th className="px-3 py-2 text-left text-xs font-medium">Wait Event</th>
                  <th className="px-3 py-2 text-right text-xs font-medium">Duration</th>
                  <th className="px-3 py-2 text-right text-xs font-medium">Blocking</th>
                  <th className="px-3 py-2 text-left text-xs font-medium">Query</th>
                </tr>
              </thead>
              <tbody>
                {data.locks.map((lock) => (
                  <LockTreeRow key={`${lock.pid}-${lock.depth}`} lock={lock} />
                ))}
              </tbody>
            </table>
          </div>
        </>
      )}
    </div>
  )
}
