import { useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { CheckCircle } from 'lucide-react'
import { PageHeader } from '@/components/ui/PageHeader'
import { Spinner } from '@/components/ui/Spinner'
import { EmptyState } from '@/components/ui/EmptyState'
import { AlertFilters } from '@/components/alerts/AlertFilters'
import { AlertRow } from '@/components/alerts/AlertRow'
import { useAlerts } from '@/hooks/useAlerts'
import { useInstances } from '@/hooks/useInstances'
import type { AlertSeverityFilter, AlertStateFilter } from '@/types/models'

export function AlertsDashboard() {
  const [searchParams] = useSearchParams()
  const initialInstance = searchParams.get('instance_id') ?? ''

  const [severity, setSeverity] = useState<AlertSeverityFilter>('all')
  const [state, setState] = useState<AlertStateFilter>('all')
  const [instanceId, setInstanceId] = useState(initialInstance)

  const { data: alerts, isLoading } = useAlerts({ severity, state, instanceId })
  const { data: instances } = useInstances()

  const count = alerts?.length ?? 0

  return (
    <div>
      <PageHeader
        title="Active Alerts"
        actions={
          count > 0 ? (
            <span className="inline-flex items-center rounded-full bg-red-500/20 px-2.5 py-1 text-sm font-medium text-red-400">
              {count}
            </span>
          ) : null
        }
      />

      <div className="space-y-4">
        <AlertFilters
          severity={severity}
          onSeverityChange={setSeverity}
          state={state}
          onStateChange={setState}
          instanceId={instanceId}
          onInstanceChange={setInstanceId}
          instances={instances ?? []}
        />

        <div className="rounded-lg border border-pgp-border bg-pgp-bg-card">
          {isLoading ? (
            <div className="flex justify-center py-12">
              <Spinner size="lg" />
            </div>
          ) : !alerts?.length ? (
            <EmptyState
              icon={CheckCircle}
              title="All clear"
              description="No active alerts matching your filters"
            />
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-left">
                <thead>
                  <tr className="border-b border-pgp-border">
                    <th className="px-4 py-3 text-xs font-medium uppercase text-pgp-text-muted">
                      Severity
                    </th>
                    <th className="px-4 py-3 text-xs font-medium uppercase text-pgp-text-muted">
                      Rule
                    </th>
                    <th className="px-4 py-3 text-xs font-medium uppercase text-pgp-text-muted">
                      Instance
                    </th>
                    <th className="px-4 py-3 text-xs font-medium uppercase text-pgp-text-muted">
                      Metric
                    </th>
                    <th className="px-4 py-3 text-xs font-medium uppercase text-pgp-text-muted">
                      Value
                    </th>
                    <th className="px-4 py-3 text-xs font-medium uppercase text-pgp-text-muted">
                      State
                    </th>
                    <th className="px-4 py-3 text-xs font-medium uppercase text-pgp-text-muted">
                      Fired
                    </th>
                    <th className="px-4 py-3 text-xs font-medium uppercase text-pgp-text-muted">
                      Duration
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {alerts.map((alert) => (
                    <AlertRow
                      key={`${alert.rule_id}-${alert.instance_id}-${alert.fired_at}`}
                      alert={alert}
                    />
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
