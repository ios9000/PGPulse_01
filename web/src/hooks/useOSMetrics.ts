import { useQuery } from '@tanstack/react-query'
import { apiFetch } from '@/lib/api'
import type { OSMetrics, ClusterMetrics } from '@/types/models'

interface OSMetricsResponse {
  available: boolean
  reason?: string
  data?: OSMetrics
}

interface ClusterMetricsResponse {
  available: boolean
  patroni?: ClusterMetrics['patroni']
  etcd?: ClusterMetrics['etcd']
}

export function useOSMetrics(instanceId: string | undefined) {
  return useQuery({
    queryKey: ['os-metrics', instanceId],
    queryFn: async () => {
      const res = await apiFetch(`/instances/${instanceId}/os`)
      return (await res.json()) as OSMetricsResponse
    },
    refetchInterval: 10_000,
    enabled: !!instanceId,
  })
}

export function useClusterMetrics(instanceId: string | undefined) {
  return useQuery({
    queryKey: ['cluster-metrics', instanceId],
    queryFn: async () => {
      const res = await apiFetch(`/instances/${instanceId}/cluster`)
      return (await res.json()) as ClusterMetricsResponse
    },
    refetchInterval: 30_000,
    enabled: !!instanceId,
  })
}
