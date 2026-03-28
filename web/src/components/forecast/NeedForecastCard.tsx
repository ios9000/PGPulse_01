import { useMaintenanceForecasts } from '@/hooks/useMaintenanceForecast'
import type { MaintenanceForecast } from '@/hooks/useMaintenanceForecast'

interface NeedForecastCardProps {
  instanceId: string
}

function formatTimeUntil(seconds: number): string {
  if (seconds < 60) return '< 1 min'
  if (seconds < 3600) return `~${Math.round(seconds / 60)}m`
  if (seconds < 86400) {
    const h = Math.floor(seconds / 3600)
    return `~${h}h`
  }
  const d = Math.floor(seconds / 86400)
  return `~${d}d`
}

function findNextOperation(forecasts: MaintenanceForecast[]): string | null {
  const actionable = forecasts
    .filter((f) => f.status === 'imminent' || f.status === 'predicted')
    .sort((a, b) => a.time_until_sec - b.time_until_sec)
  if (!actionable.length) return null
  const f = actionable[0]
  const time = formatTimeUntil(f.time_until_sec)
  return `${f.operation} on ${f.table_name} (${time})`
}

export function NeedForecastCard({ instanceId }: NeedForecastCardProps) {
  const { data, isLoading, isError } = useMaintenanceForecasts(instanceId)

  if (isLoading || isError || !data) {
    return null
  }

  const { summary, forecasts } = data

  if (summary.overdue_count === 0 && summary.imminent_count === 0 && summary.predicted_count === 0) {
    if (summary.total_tables_evaluated === 0) {
      return (
        <div className="rounded-lg border border-pgp-border bg-pgp-bg-card p-4">
          <h3 className="mb-2 text-sm font-medium text-pgp-text-secondary">Maintenance Forecast</h3>
          <p className="text-sm text-pgp-text-muted">No forecasts yet — data accumulating</p>
        </div>
      )
    }
    return null
  }

  const next = findNextOperation(forecasts)

  return (
    <div className="rounded-lg border border-pgp-border bg-pgp-bg-card p-4">
      <h3 className="mb-3 text-sm font-medium text-pgp-text-secondary">Maintenance Forecast</h3>
      <div className="flex flex-wrap gap-4">
        {summary.overdue_count > 0 && (
          <div className="flex items-center gap-1.5">
            <span className="inline-block h-2.5 w-2.5 rounded-full bg-red-500" />
            <span className="text-sm font-medium text-red-400">{summary.overdue_count} Overdue</span>
          </div>
        )}
        {summary.imminent_count > 0 && (
          <div className="flex items-center gap-1.5">
            <span className="inline-block h-2.5 w-2.5 rounded-full bg-yellow-500" />
            <span className="text-sm font-medium text-yellow-400">{summary.imminent_count} Imminent</span>
          </div>
        )}
        {summary.predicted_count > 0 && (
          <div className="flex items-center gap-1.5">
            <span className="inline-block h-2.5 w-2.5 rounded-full bg-blue-500" />
            <span className="text-sm font-medium text-blue-400">{summary.predicted_count} Predicted</span>
          </div>
        )}
      </div>
      {next && (
        <p className="mt-2 text-xs text-pgp-text-muted">
          Next: {next}
        </p>
      )}
    </div>
  )
}
