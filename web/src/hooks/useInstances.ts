import { useQuery } from '@tanstack/react-query'
import { apiFetch } from '@/lib/api'
import type { InstanceData } from '@/types/models'

interface UseInstancesOptions {
  include?: ('metrics' | 'alerts')[]
}

export function useInstances(options: UseInstancesOptions = {}) {
  const includeParam = options.include?.length ? `?include=${options.include.join(',')}` : ''

  return useQuery({
    queryKey: ['instances', options.include],
    queryFn: async () => {
      const res = await apiFetch(`/instances${includeParam}`)
      const json = await res.json()
      return json.data as InstanceData[]
    },
    refetchInterval: 30_000,
  })
}
