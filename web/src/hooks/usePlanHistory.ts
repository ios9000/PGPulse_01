import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { apiFetch } from '@/lib/api'

export interface CapturedPlan {
  id: number
  instance_id: string
  database_name: string
  query_fingerprint: string
  plan_hash: string
  plan_text?: string
  trigger_type: 'duration_threshold' | 'manual' | 'scheduled_topn' | 'hash_diff_signal'
  duration_ms: number
  query_text: string
  truncated: boolean
  metadata?: Record<string, unknown>
  captured_at: string
}

export function usePlanHistory(instanceId: string, options?: { trigger?: string; database?: string }) {
  const params = new URLSearchParams()
  if (options?.trigger) params.set('trigger', options.trigger)
  if (options?.database) params.set('database', options.database)
  const qs = params.toString()

  return useQuery({
    queryKey: ['plans', instanceId, options?.trigger, options?.database],
    queryFn: async () => {
      const res = await apiFetch(`/instances/${instanceId}/plans${qs ? `?${qs}` : ''}`)
      const json = await res.json()
      return json.data as CapturedPlan[]
    },
    refetchInterval: 30_000,
    enabled: !!instanceId,
  })
}

export function usePlanDetail(instanceId: string, planId: number | null) {
  return useQuery({
    queryKey: ['plans', instanceId, 'detail', planId],
    queryFn: async () => {
      const res = await apiFetch(`/instances/${instanceId}/plans/${planId}`)
      const json = await res.json()
      return json.data as CapturedPlan
    },
    enabled: !!instanceId && planId !== null,
  })
}

export function usePlanRegressions(instanceId: string) {
  return useQuery({
    queryKey: ['plans', instanceId, 'regressions'],
    queryFn: async () => {
      const res = await apiFetch(`/instances/${instanceId}/plans/regressions`)
      const json = await res.json()
      return json.data as CapturedPlan[]
    },
    refetchInterval: 60_000,
    enabled: !!instanceId,
  })
}

export function useManualCapture(instanceId: string) {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async ({ query, database }: { query: string; database: string }) => {
      const res = await apiFetch(`/instances/${instanceId}/plans/capture`, {
        method: 'POST',
        body: JSON.stringify({ query, database }),
      })
      return res.json()
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['plans', instanceId] })
    },
  })
}
