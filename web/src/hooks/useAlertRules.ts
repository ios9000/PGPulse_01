import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { apiFetch } from '@/lib/api'
import type { AlertRule } from '@/types/models'

export function useAlertRules() {
  return useQuery({
    queryKey: ['alertRules'],
    queryFn: async () => {
      const res = await apiFetch('/alerts/rules')
      const json = await res.json()
      return json.data as AlertRule[]
    },
    refetchInterval: 60_000,
  })
}

interface SaveAlertRulePayload {
  id?: string
  name: string
  description?: string
  metric: string
  operator: AlertRule['operator']
  threshold: number
  severity: AlertRule['severity']
  consecutive_count: number
  cooldown_minutes: number
  channels?: string[]
  enabled: boolean
}

export function useSaveAlertRule() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (payload: SaveAlertRulePayload) => {
      const { id, ...body } = payload
      if (id) {
        const res = await apiFetch(`/alerts/rules/${id}`, {
          method: 'PUT',
          body: JSON.stringify(body),
        })
        return res.json()
      }
      const res = await apiFetch('/alerts/rules', {
        method: 'POST',
        body: JSON.stringify(payload),
      })
      return res.json()
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['alertRules'] })
    },
  })
}

export function useDeleteAlertRule() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (id: string) => {
      await apiFetch(`/alerts/rules/${id}`, { method: 'DELETE' })
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['alertRules'] })
    },
  })
}

interface TestNotificationPayload {
  channel: string
  message: string
}

export function useTestNotification() {
  return useMutation({
    mutationFn: async (payload: TestNotificationPayload) => {
      const res = await apiFetch('/alerts/test', {
        method: 'POST',
        body: JSON.stringify(payload),
      })
      return res.json()
    },
  })
}
