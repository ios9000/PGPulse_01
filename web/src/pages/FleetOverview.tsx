import { useInstances } from '@/hooks/useInstances'
import { InstanceCard } from '@/components/fleet/InstanceCard'
import { PageHeader } from '@/components/ui/PageHeader'
import { Spinner } from '@/components/ui/Spinner'
import { EmptyState } from '@/components/ui/EmptyState'

export function FleetOverview() {
  const { data: instances, isLoading } = useInstances({ include: ['metrics', 'alerts'] })

  if (isLoading) {
    return (
      <div className="flex justify-center py-12">
        <Spinner size="lg" />
      </div>
    )
  }

  if (!instances?.length) {
    return (
      <EmptyState
        title="No instances configured"
        description="Add PostgreSQL instances to your configuration file."
      />
    )
  }

  return (
    <div>
      <PageHeader
        title="Fleet Overview"
        subtitle={`${instances.length} instance${instances.length !== 1 ? 's' : ''}`}
      />
      <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4">
        {instances.map((inst) => (
          <InstanceCard key={inst.id} instance={inst} />
        ))}
      </div>
    </div>
  )
}
