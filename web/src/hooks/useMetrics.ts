import { useQuery } from '@tanstack/react-query'
import { apiFetch } from '@/lib/api'
import { useTimeRangeStore, type PresetKey } from '@/stores/timeRangeStore'
import type { CurrentMetricsResult, HistoryResult } from '@/types/models'

export function useCurrentMetrics(instanceId: string | undefined) {
  return useQuery({
    queryKey: ['metrics', 'current', instanceId],
    queryFn: async () => {
      const res = await apiFetch(`/instances/${instanceId}/metrics/current`)
      const json = await res.json()
      return json.data as CurrentMetricsResult
    },
    refetchInterval: 10_000,
    enabled: !!instanceId,
  })
}

const STEP_MAP: Record<PresetKey, string> = {
  '15m': '1m',
  '1h': '1m',
  '6h': '5m',
  '24h': '15m',
  '7d': '1h',
}

function computeStep(from: Date, to: Date): string {
  const diffMs = to.getTime() - from.getTime()
  const diffMinutes = diffMs / 60_000
  if (diffMinutes <= 60) return '1m'
  if (diffMinutes <= 360) return '5m'
  if (diffMinutes <= 1440) return '15m'
  return '1h'
}

export function useMetricsHistory(instanceId: string | undefined, metrics: string[]) {
  const range = useTimeRangeStore((s) => s.range)
  const getEffectiveRange = useTimeRangeStore((s) => s.getEffectiveRange)

  return useQuery({
    queryKey: ['metrics', 'history', instanceId, metrics, range],
    queryFn: async () => {
      const { from, to } = getEffectiveRange()
      const step = range.preset !== 'custom'
        ? STEP_MAP[range.preset as PresetKey]
        : computeStep(from, to)

      const params = new URLSearchParams({
        metrics: metrics.join(','),
        from: from.toISOString(),
        to: to.toISOString(),
        step,
      })

      const res = await apiFetch(`/instances/${instanceId}/metrics/history?${params.toString()}`)
      const json = await res.json()
      return json.data as HistoryResult
    },
    refetchInterval: range.preset !== 'custom' ? 10_000 : false,
    enabled: !!instanceId && metrics.length > 0,
  })
}
