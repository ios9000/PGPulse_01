import { useWaitEvents } from '@/hooks/useActivity'
import { WaitEventsChart } from '@/components/charts/WaitEventsChart'
import { Spinner } from '@/components/ui/Spinner'
import { EmptyState } from '@/components/ui/EmptyState'

interface WaitEventsSectionProps {
  instanceId: string
}

export function WaitEventsSection({ instanceId }: WaitEventsSectionProps) {
  const { data, isLoading } = useWaitEvents(instanceId)

  return (
    <div className="rounded-lg border border-pgp-border bg-pgp-bg-card p-4">
      <h2 className="mb-4 text-lg font-semibold text-pgp-text-primary">Wait Events</h2>
      {isLoading ? (
        <div className="flex justify-center py-8"><Spinner size="lg" /></div>
      ) : !data?.events?.length ? (
        <EmptyState title="No active wait events" />
      ) : (
        <WaitEventsChart events={data.events} />
      )}
    </div>
  )
}
