import { useQuery } from '@tanstack/react-query'
import { apiFetch } from '@/lib/api'
import type { StatementsResponse, StatementSortField } from '@/types/models'

export function useStatements(
  instanceId: string | undefined,
  sort: StatementSortField = 'total_time',
  limit: number = 25,
) {
  return useQuery({
    queryKey: ['statements', instanceId, sort, limit],
    queryFn: async () => {
      const res = await apiFetch(
        `/instances/${instanceId}/activity/statements?sort=${sort}&limit=${limit}`,
      )
      const json = await res.json()
      return json.data as StatementsResponse
    },
    refetchInterval: 10_000,
    enabled: !!instanceId,
  })
}
