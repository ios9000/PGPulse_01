import { useQuery } from '@tanstack/react-query'
import { apiFetch } from '@/lib/api'
import type { ProgressResponse } from '@/types/models'

export function useProgress(instanceId: string | undefined) {
  return useQuery({
    queryKey: ['progress', instanceId],
    queryFn: async () => {
      const res = await apiFetch(`/instances/${instanceId}/activity/progress`)
      const json = await res.json()
      return json.data as ProgressResponse
    },
    refetchInterval: 5_000,
    enabled: !!instanceId,
  })
}
