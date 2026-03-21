import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { apiFetch } from '@/lib/api'
import type { RCAIncident, RCACausalGraph } from '@/types/rca'

export function useRCAIncidents(params: { limit?: number; offset?: number } = {}) {
  return useQuery({
    queryKey: ['rca-incidents', params],
    queryFn: async () => {
      const qs = new URLSearchParams()
      if (params.limit) qs.set('limit', String(params.limit))
      if (params.offset) qs.set('offset', String(params.offset))
      const q = qs.toString()
      const res = await apiFetch(`/rca/incidents${q ? `?${q}` : ''}`)
      const json = await res.json()
      return json.data as { incidents: RCAIncident[]; total: number }
    },
    refetchInterval: 30_000,
  })
}

export function useInstanceRCAIncidents(
  instanceId: string | undefined,
  params: { limit?: number; offset?: number } = {},
) {
  return useQuery({
    queryKey: ['rca-incidents', 'instance', instanceId, params],
    queryFn: async () => {
      const qs = new URLSearchParams()
      if (params.limit) qs.set('limit', String(params.limit))
      if (params.offset) qs.set('offset', String(params.offset))
      const q = qs.toString()
      const res = await apiFetch(`/instances/${instanceId}/rca/incidents${q ? `?${q}` : ''}`)
      const json = await res.json()
      return json.data as { incidents: RCAIncident[]; total: number }
    },
    refetchInterval: 30_000,
    enabled: !!instanceId,
  })
}

export function useRCAIncident(instanceId: string | undefined, incidentId: number | undefined) {
  return useQuery({
    queryKey: ['rca-incident', instanceId, incidentId],
    queryFn: async () => {
      const res = await apiFetch(`/instances/${instanceId}/rca/incidents/${incidentId}`)
      const json = await res.json()
      return json.data as RCAIncident
    },
    enabled: !!instanceId && !!incidentId,
  })
}

export function useRCAGraph() {
  return useQuery({
    queryKey: ['rca-graph'],
    queryFn: async () => {
      const res = await apiFetch('/rca/graph')
      const json = await res.json()
      return json.data as RCACausalGraph
    },
    staleTime: 5 * 60 * 1000,
  })
}

export function useRCAAnalyze() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (params: {
      instanceId: string
      metric: string
      value?: number
      timestamp?: string
    }) => {
      const res = await apiFetch(`/instances/${params.instanceId}/rca/analyze`, {
        method: 'POST',
        body: JSON.stringify({
          metric: params.metric,
          value: params.value ?? 0,
          timestamp: params.timestamp,
        }),
      })
      const json = await res.json()
      return json.data as RCAIncident
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['rca-incidents'] })
    },
  })
}
