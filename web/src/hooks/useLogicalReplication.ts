import { useQuery } from '@tanstack/react-query'
import { apiFetch } from '@/lib/api'
import type { LogicalReplicationResponse } from '@/types/models'

export function useLogicalReplication(instanceId: string | undefined) {
  return useQuery({
    queryKey: ['logical-replication', instanceId],
    queryFn: async () => {
      const res = await apiFetch(`/instances/${instanceId}/logical-replication`)
      const json = await res.json()
      return json.data as LogicalReplicationResponse
    },
    refetchInterval: 30_000,
    enabled: !!instanceId,
  })
}
