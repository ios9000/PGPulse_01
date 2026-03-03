import { useQuery } from '@tanstack/react-query'
import { apiFetch } from '@/lib/api'
import type { WaitEventsResponse, LongTransactionsResponse } from '@/types/models'

export function useWaitEvents(instanceId: string | undefined) {
  return useQuery({
    queryKey: ['activity', 'wait-events', instanceId],
    queryFn: async () => {
      const res = await apiFetch(`/instances/${instanceId}/activity/wait-events`)
      const json = await res.json()
      return json.data as WaitEventsResponse
    },
    refetchInterval: 10_000,
    enabled: !!instanceId,
  })
}

export function useLongTransactions(instanceId: string | undefined, thresholdSeconds?: number) {
  const params = thresholdSeconds ? `?threshold_seconds=${thresholdSeconds}` : ''

  return useQuery({
    queryKey: ['activity', 'long-transactions', instanceId, thresholdSeconds],
    queryFn: async () => {
      const res = await apiFetch(`/instances/${instanceId}/activity/long-transactions${params}`)
      const json = await res.json()
      return json.data as LongTransactionsResponse
    },
    refetchInterval: 10_000,
    enabled: !!instanceId,
  })
}
