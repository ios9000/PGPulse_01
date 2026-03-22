import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { apiFetch } from '@/lib/api'
import type { Recommendation, DiagnoseResponse, RemediationRule } from '@/types/models'

interface UseRecommendationsParams {
  priority?: string
  category?: string
  status?: string
  acknowledged?: boolean
  instanceId?: string
  limit?: number
  source?: string
  order_by?: string
}

export function useRecommendations(params: UseRecommendationsParams = {}) {
  return useQuery({
    queryKey: ['recommendations', params],
    queryFn: async () => {
      const qs = new URLSearchParams()
      if (params.priority) qs.set('priority', params.priority)
      if (params.category) qs.set('category', params.category)
      if (params.status) qs.set('status', params.status)
      if (params.acknowledged !== undefined) qs.set('acknowledged', String(params.acknowledged))
      if (params.instanceId) qs.set('instance_id', params.instanceId)
      if (params.limit) qs.set('limit', String(params.limit))
      if (params.source) qs.set('source', params.source)
      if (params.order_by) qs.set('order_by', params.order_by)
      const q = qs.toString()
      const res = await apiFetch(`/recommendations${q ? `?${q}` : ''}`)
      const json = await res.json()
      return json.data as Recommendation[]
    },
    refetchInterval: 30_000,
  })
}

export function useInstanceRecommendations(instanceId: string | undefined, params: UseRecommendationsParams = {}) {
  return useQuery({
    queryKey: ['recommendations', 'instance', instanceId, params],
    queryFn: async () => {
      const qs = new URLSearchParams()
      if (params.priority) qs.set('priority', params.priority)
      if (params.category) qs.set('category', params.category)
      if (params.acknowledged !== undefined) qs.set('acknowledged', String(params.acknowledged))
      if (params.limit) qs.set('limit', String(params.limit))
      const q = qs.toString()
      const res = await apiFetch(`/instances/${instanceId}/recommendations${q ? `?${q}` : ''}`)
      const json = await res.json()
      return json.data as Recommendation[]
    },
    refetchInterval: 30_000,
    enabled: !!instanceId,
  })
}

export function useDiagnose(instanceId: string | undefined) {
  const queryClient = useQueryClient()
  const mutation = useMutation({
    mutationFn: async () => {
      const res = await apiFetch(`/instances/${instanceId}/diagnose`, {
        method: 'POST',
      })
      const json = await res.json()
      return json.data as DiagnoseResponse
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['recommendations'] })
    },
  })

  return {
    diagnose: mutation.mutateAsync,
    data: mutation.data,
    isPending: mutation.isPending,
    error: mutation.error,
    reset: mutation.reset,
  }
}

export function useAcknowledge() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (id: number) => {
      const res = await apiFetch(`/recommendations/${id}/acknowledge`, {
        method: 'PUT',
      })
      return res.json()
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['recommendations'] })
    },
  })
}

export function useActiveRecommendationCount() {
  return useRecommendations({ status: 'active' })
}

export function useRemediationRules() {
  return useQuery({
    queryKey: ['remediationRules'],
    queryFn: async () => {
      const res = await apiFetch('/recommendations/rules')
      const json = await res.json()
      return json.data as RemediationRule[]
    },
    refetchInterval: 60_000,
  })
}
