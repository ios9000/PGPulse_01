import { useProgress } from '@/hooks/useProgress'
import { ProgressCard } from './ProgressCard'

interface ProgressSectionProps {
  instanceId: string
}

export function ProgressSection({ instanceId }: ProgressSectionProps) {
  const { data, isLoading, isError } = useProgress(instanceId)

  if (isLoading || isError || !data?.operations?.length) {
    return null
  }

  return (
    <div className="rounded-lg border border-pgp-border bg-pgp-bg-card p-4">
      <h2 className="mb-4 text-lg font-semibold text-pgp-text-primary">Active Operations</h2>
      <div className="space-y-3">
        {data.operations.map((op) => (
          <ProgressCard key={`${op.operation_type}-${op.pid}`} operation={op} />
        ))}
      </div>
    </div>
  )
}
