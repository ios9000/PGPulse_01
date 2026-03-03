import { useQuery } from '@tanstack/react-query'
import { apiFetch } from '@/lib/api'
import type { ReplicationResponse } from '@/types/models'

export function useReplication(instanceId: string | undefined) {
  return useQuery({
    queryKey: ['replication', instanceId],
    queryFn: async () => {
      const res = await apiFetch(`/instances/${instanceId}/replication`)
      const json = await res.json()
      return json.data as ReplicationResponse
    },
    refetchInterval: 10_000,
    enabled: !!instanceId,
  })
}
