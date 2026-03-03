import type { ProgressOperation } from '@/types/models'
import { formatDuration } from '@/lib/formatters'

interface ProgressCardProps {
  operation: ProgressOperation
}

const OPERATION_COLORS: Record<string, string> = {
  vacuum: 'bg-blue-500',
  analyze: 'bg-green-500',
  create_index: 'bg-purple-500',
  cluster: 'bg-orange-500',
  basebackup: 'bg-cyan-500',
  copy: 'bg-yellow-500',
}

export function ProgressCard({ operation }: ProgressCardProps) {
  const badgeColor = OPERATION_COLORS[operation.operation_type] ?? 'bg-slate-500'
  const hasProgress = operation.progress_pct != null

  return (
    <div className="rounded-lg border border-pgp-border bg-pgp-bg-secondary p-3">
      <div className="mb-2 flex items-center justify-between">
        <div className="flex items-center gap-2">
          <span className={`inline-block rounded-full px-2.5 py-0.5 text-xs font-medium text-white ${badgeColor}`}>
            {operation.operation_type.replace('_', ' ')}
          </span>
          <span className="text-sm text-pgp-text-primary">
            {operation.datname}
            {operation.relname && <span className="text-pgp-text-muted"> / {operation.relname}</span>}
          </span>
        </div>
        <span className="font-mono text-xs text-pgp-text-muted">PID {operation.pid}</span>
      </div>

      <div className="mb-2 flex items-center justify-between text-xs text-pgp-text-secondary">
        <span>{operation.phase}</span>
        <span>{formatDuration(operation.duration_seconds)}</span>
      </div>

      <div className="h-2 w-full overflow-hidden rounded-full bg-slate-700">
        {hasProgress ? (
          <div
            className={`h-full rounded-full ${badgeColor} bg-[length:20px_20px] bg-[linear-gradient(45deg,rgba(255,255,255,.15)_25%,transparent_25%,transparent_50%,rgba(255,255,255,.15)_50%,rgba(255,255,255,.15)_75%,transparent_75%,transparent)] animate-[progress-stripes_1s_linear_infinite]`}
            style={{ width: `${operation.progress_pct}%` }}
          />
        ) : (
          <div className="h-full w-full animate-pulse rounded-full bg-slate-600" />
        )}
      </div>

      {hasProgress && (
        <div className="mt-1 text-right text-xs font-mono text-pgp-text-muted">
          {operation.progress_pct!.toFixed(1)}%
        </div>
      )}
    </div>
  )
}
