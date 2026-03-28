import { useMaintenanceForecasts } from '@/hooks/useMaintenanceForecast'
import type { MaintenanceForecast } from '@/hooks/useMaintenanceForecast'
import { formatTimestamp } from '@/lib/formatters'

interface NeedForecastTableProps {
  instanceId: string
}

const STATUS_BADGE: Record<string, { bg: string; text: string; border?: string }> = {
  overdue: { bg: 'bg-red-500/20', text: 'text-red-400' },
  imminent: { bg: 'bg-yellow-500/20', text: 'text-yellow-400' },
  predicted: { bg: 'bg-blue-500/20', text: 'text-blue-400' },
  not_needed: { bg: 'bg-slate-500/20', text: 'text-slate-400' },
  insufficient_data: { bg: 'bg-slate-500/10', text: 'text-slate-500', border: 'border border-dashed border-slate-600' },
}

function StatusBadge({ status }: { status: string }) {
  const style = STATUS_BADGE[status] ?? STATUS_BADGE.not_needed
  return (
    <span className={`inline-block rounded px-2 py-0.5 text-xs font-medium ${style.bg} ${style.text} ${style.border ?? ''}`}>
      {status.replace('_', ' ')}
    </span>
  )
}

function formatTimeUntil(seconds: number): string {
  if (seconds <= 0) return '—'
  if (seconds < 60) return '< 1 min'
  if (seconds < 3600) return `${Math.round(seconds / 60)}m`
  if (seconds < 86400) {
    const h = Math.floor(seconds / 3600)
    const m = Math.round((seconds % 3600) / 60)
    return m > 0 ? `${h}h ${m}m` : `${h}h`
  }
  const d = Math.floor(seconds / 86400)
  const h = Math.round((seconds % 86400) / 3600)
  return h > 0 ? `${d}d ${h}h` : `${d}d`
}

function formatMethod(method: string): string {
  if (method === 'threshold_projection') return 'Threshold'
  if (method === 'threshold_projection+ml') return 'Threshold + ML'
  return method
}

export function NeedForecastTable({ instanceId }: NeedForecastTableProps) {
  const { data, isLoading, isError } = useMaintenanceForecasts(instanceId)

  if (isLoading) {
    return <div className="py-4 text-center text-sm text-pgp-text-muted">Loading forecasts...</div>
  }
  if (isError || !data) {
    return null
  }

  const { forecasts } = data

  if (!forecasts.length) {
    return (
      <div className="py-4 text-center text-sm text-pgp-text-muted">
        No forecasts yet — data accumulating
      </div>
    )
  }

  return (
    <div className="overflow-x-auto">
      <table className="w-full text-left text-sm">
        <thead>
          <tr className="border-b border-pgp-border text-xs text-pgp-text-muted">
            <th className="pb-2 pr-3">Status</th>
            <th className="pb-2 pr-3">Database</th>
            <th className="pb-2 pr-3">Table</th>
            <th className="pb-2 pr-3">Operation</th>
            <th className="pb-2 pr-3 text-right">Time Until</th>
            <th className="pb-2 pr-3 text-right">Current / Threshold</th>
            <th className="pb-2 pr-3 text-right">Rate</th>
            <th className="pb-2 pr-3">Method</th>
            <th className="pb-2">Evaluated</th>
          </tr>
        </thead>
        <tbody>
          {forecasts.map((f: MaintenanceForecast) => (
            <tr
              key={`${f.database}-${f.table_name}-${f.operation}`}
              className="border-b border-pgp-border/50 text-pgp-text-primary"
            >
              <td className="py-2 pr-3"><StatusBadge status={f.status} /></td>
              <td className="py-2 pr-3 font-mono text-xs">{f.database || '—'}</td>
              <td className="py-2 pr-3 font-mono text-xs">{f.table_name || '—'}</td>
              <td className="py-2 pr-3">{f.operation}</td>
              <td className="py-2 pr-3 text-right font-mono text-xs">
                {f.status === 'overdue' ? 'overdue' : formatTimeUntil(f.time_until_sec)}
              </td>
              <td className="py-2 pr-3 text-right font-mono text-xs">
                {f.current_value.toFixed(0)} / {f.threshold_value.toFixed(0)}
              </td>
              <td className="py-2 pr-3 text-right font-mono text-xs">
                {f.accumulation_rate > 0 ? `${f.accumulation_rate.toFixed(2)}/s` : '—'}
              </td>
              <td className="py-2 pr-3 text-xs text-pgp-text-muted">{formatMethod(f.method)}</td>
              <td className="py-2 text-xs text-pgp-text-muted">{formatTimestamp(f.evaluated_at)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}
