import { useQuery } from '@tanstack/react-query'
import { apiFetch } from '@/lib/api'
import type { DatabaseSummary, DatabaseMetrics } from '@/types/models'

export function useDatabaseList(instanceId: string | undefined) {
  return useQuery({
    queryKey: ['instances', instanceId, 'databases'],
    queryFn: async () => {
      const res = await apiFetch(`/instances/${instanceId}/databases`)
      return (await res.json()) as DatabaseSummary[]
    },
    refetchInterval: 5 * 60_000,
    enabled: !!instanceId,
  })
}

export function useDatabaseMetrics(instanceId: string | undefined, dbName: string | undefined) {
  return useQuery({
    queryKey: ['instances', instanceId, 'databases', dbName, 'metrics'],
    queryFn: async () => {
      const res = await apiFetch(`/instances/${instanceId}/databases/${dbName}/metrics`)
      return (await res.json()) as DatabaseMetrics
    },
    refetchInterval: 5 * 60_000,
    enabled: !!instanceId && !!dbName,
  })
}
