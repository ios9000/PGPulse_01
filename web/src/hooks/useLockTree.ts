import { useQuery } from '@tanstack/react-query'
import { apiFetch } from '@/lib/api'
import type { LockTreeResponse } from '@/types/models'

export function useLockTree(instanceId: string | undefined) {
  return useQuery({
    queryKey: ['locks', instanceId],
    queryFn: async () => {
      const res = await apiFetch(`/instances/${instanceId}/activity/locks`)
      const json = await res.json()
      return json.data as LockTreeResponse
    },
    refetchInterval: 10_000,
    enabled: !!instanceId,
  })
}
