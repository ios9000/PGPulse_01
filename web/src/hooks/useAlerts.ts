import { useQuery } from '@tanstack/react-query'
import { apiFetch } from '@/lib/api'
import type { AlertEvent } from '@/types/models'

export function useInstanceAlerts(instanceId: string | undefined) {
  return useQuery({
    queryKey: ['alerts', 'instance', instanceId],
    queryFn: async () => {
      const res = await apiFetch(`/alerts?instance_id=${instanceId}`)
      const json = await res.json()
      return json.data as AlertEvent[]
    },
    refetchInterval: 30_000,
    enabled: !!instanceId,
  })
}
