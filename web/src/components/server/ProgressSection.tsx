import { useProgress } from '@/hooks/useProgress'
import { useETAForInstance } from '@/hooks/useMaintenanceForecast'
import { ProgressCard } from './ProgressCard'
import { ETABadge } from '@/components/forecast/ETABadge'

interface ProgressSectionProps {
  instanceId: string
}

export function ProgressSection({ instanceId }: ProgressSectionProps) {
  const { data, isLoading, isError } = useProgress(instanceId)
  const { data: etaData } = useETAForInstance(instanceId)

  if (isLoading || isError || !data?.operations?.length) {
    return null
  }

  // Build PID→ETA lookup map.
  const etaByPid = new Map(
    (etaData?.operations ?? []).map((e) => [e.pid, e]),
  )

  return (
    <div className="rounded-lg border border-pgp-border bg-pgp-bg-card p-4">
      <h2 className="mb-4 text-lg font-semibold text-pgp-text-primary">Active Operations</h2>
      <div className="space-y-3">
        {data.operations.map((op) => (
          <div key={`${op.operation_type}-${op.pid}`} className="flex items-start gap-3">
            <div className="flex-1">
              <ProgressCard operation={op} />
            </div>
            <div className="flex w-28 shrink-0 items-center justify-end pt-2">
              <ETABadge eta={etaByPid.get(op.pid)} />
            </div>
          </div>
        ))}
      </div>
    </div>
  )
}
