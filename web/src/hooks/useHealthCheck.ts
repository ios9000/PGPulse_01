import { useQuery } from '@tanstack/react-query'
import { apiFetch } from '@/lib/api'
import type { HealthResponse } from '@/types/models'
import { POLLING_INTERVALS } from '@/lib/constants'

export function useHealthCheck() {
  const { data, isSuccess, dataUpdatedAt } = useQuery({
    queryKey: ['health'],
    queryFn: async () => {
      const res = await apiFetch('/health', { skipAuth: true })
      return res.json() as Promise<HealthResponse>
    },
    refetchInterval: POLLING_INTERVALS.health,
    retry: 1,
  })

  return {
    isConnected: isSuccess,
    status: data?.status ?? null,
    version: data?.version ?? null,
    uptime: data?.uptime ?? null,
    dataUpdatedAt: dataUpdatedAt || null,
  }
}
