import { useQuery } from '@tanstack/react-query'
import { apiFetch } from '@/lib/api'
import type { AlertEvent, AlertSeverityFilter, AlertStateFilter } from '@/types/models'

interface UseAlertsOptions {
  severity?: AlertSeverityFilter
  state?: AlertStateFilter
  instanceId?: string
}

export function useAlerts(options: UseAlertsOptions = {}) {
  const { severity, state, instanceId } = options

  return useQuery({
    queryKey: ['alerts', 'active', severity, state, instanceId],
    queryFn: async () => {
      const res = await apiFetch('/alerts')
      const json = await res.json()
      let events = json.data as AlertEvent[]

      if (instanceId) {
        events = events.filter((e) => e.instance_id === instanceId)
      }
      if (severity && severity !== 'all') {
        events = events.filter((e) => e.severity === severity)
      }
      if (state && state !== 'all') {
        if (state === 'firing') {
          events = events.filter((e) => !e.resolved_at)
        } else if (state === 'resolved') {
          events = events.filter((e) => !!e.resolved_at)
        }
      }

      return events
    },
    refetchInterval: 30_000,
  })
}

interface UseAlertHistoryOptions {
  severity?: AlertSeverityFilter
  instanceId?: string
  ruleId?: string
  unresolved?: boolean
  limit?: number
  start?: string
  end?: string
}

export function useAlertHistory(options: UseAlertHistoryOptions = {}) {
  return useQuery({
    queryKey: ['alerts', 'history', options],
    queryFn: async () => {
      const params = new URLSearchParams()
      if (options.instanceId) params.set('instance_id', options.instanceId)
      if (options.ruleId) params.set('rule_id', options.ruleId)
      if (options.severity && options.severity !== 'all') params.set('severity', options.severity)
      if (options.unresolved) params.set('unresolved', 'true')
      if (options.limit) params.set('limit', String(options.limit))
      if (options.start) params.set('start', options.start)
      if (options.end) params.set('end', options.end)

      const qs = params.toString()
      const res = await apiFetch(`/alerts/history${qs ? `?${qs}` : ''}`)
      const json = await res.json()
      return json.data as AlertEvent[]
    },
    refetchInterval: 30_000,
  })
}

export function useInstanceAlerts(instanceId: string | undefined) {
  return useQuery({
    queryKey: ['alerts', 'instance', instanceId],
    queryFn: async () => {
      const res = await apiFetch('/alerts')
      const json = await res.json()
      const events = json.data as AlertEvent[]
      return events.filter((e) => e.instance_id === instanceId)
    },
    refetchInterval: 30_000,
    enabled: !!instanceId,
  })
}
