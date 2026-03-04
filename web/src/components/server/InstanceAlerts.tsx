import { Link } from 'react-router-dom'
import { useInstanceAlerts } from '@/hooks/useAlerts'
import { StatusBadge } from '@/components/ui/StatusBadge'
import { Spinner } from '@/components/ui/Spinner'
import { formatTimestamp } from '@/lib/formatters'

interface InstanceAlertsProps {
  instanceId: string
}

export function InstanceAlerts({ instanceId }: InstanceAlertsProps) {
  const { data: alerts, isLoading } = useInstanceAlerts(instanceId)

  return (
    <div className="rounded-lg border border-pgp-border bg-pgp-bg-card p-4">
      <h2 className="mb-4 text-lg font-semibold text-pgp-text-primary">Active Alerts</h2>
      {isLoading ? (
        <div className="flex justify-center py-8"><Spinner size="lg" /></div>
      ) : !alerts?.length ? (
        <div className="flex items-center gap-2 py-4 text-sm">
          <StatusBadge status="ok" label="No active alerts" />
        </div>
      ) : (
        <div className="space-y-3">
          {alerts.map((alert) => {
            const severityStatus = alert.severity === 'critical' ? 'critical' as const
              : alert.severity === 'warning' ? 'warning' as const
              : 'info' as const
            return (
              <div
                key={`${alert.rule_id}-${alert.fired_at}`}
                className="flex flex-wrap items-start gap-3 rounded-md border border-pgp-border bg-pgp-bg-secondary p-3"
              >
                <StatusBadge status={severityStatus} label={alert.severity} pulse={alert.severity === 'critical'} />
                <div className="flex-1">
                  <p className="text-sm font-medium text-pgp-text-primary">{alert.rule_name}</p>
                  <p className="text-xs text-pgp-text-muted">
                    {alert.metric}: {alert.value.toFixed(2)} {alert.operator} {alert.threshold}
                  </p>
                </div>
                <span className="text-xs text-pgp-text-muted">
                  {formatTimestamp(alert.fired_at)}
                </span>
              </div>
            )
          })}
        </div>
      )}
      <div className="mt-4 border-t border-pgp-border pt-3">
        <Link
          to={`/alerts?instance_id=${instanceId}`}
          className="text-sm text-blue-400 hover:text-blue-300"
        >
          View all alerts &rarr;
        </Link>
      </div>
    </div>
  )
}
