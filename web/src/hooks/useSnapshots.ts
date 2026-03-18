import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { apiFetch } from '@/lib/api'
import type {
  PGSSSnapshotListResponse,
  PGSSDiffResult,
  PGSSQueryInsight,
  PGSSWorkloadReport,
  PGSSSnapshot,
} from '@/types/models'

export function useSnapshots(instanceId: string, opts?: { limit?: number; offset?: number }) {
  return useQuery({
    queryKey: ['pgss-snapshots', instanceId, opts],
    queryFn: async () => {
      let path = `/instances/${instanceId}/snapshots`
      const params = new URLSearchParams()
      if (opts?.limit) params.set('limit', String(opts.limit))
      if (opts?.offset) params.set('offset', String(opts.offset))
      const qs = params.toString()
      if (qs) path += `?${qs}`
      const res = await apiFetch(path)
      return (await res.json()) as PGSSSnapshotListResponse
    },
    refetchInterval: 60_000,
    enabled: !!instanceId,
  })
}

export function useLatestDiff(instanceId: string, enabled = true) {
  return useQuery({
    queryKey: ['pgss-latest-diff', instanceId],
    queryFn: async () => {
      const res = await apiFetch(`/instances/${instanceId}/snapshots/latest-diff`)
      return (await res.json()) as PGSSDiffResult
    },
    refetchInterval: 30_000,
    enabled: !!instanceId && enabled,
  })
}

export function useSnapshotDiff(
  instanceId: string,
  fromId?: number | null,
  toId?: number | null,
  opts?: { sort?: string; limit?: number; offset?: number },
) {
  return useQuery({
    queryKey: ['pgss-diff', instanceId, fromId, toId, opts],
    queryFn: async () => {
      const params = new URLSearchParams()
      params.set('from', String(fromId))
      params.set('to', String(toId))
      if (opts?.sort) params.set('sort', opts.sort)
      if (opts?.limit) params.set('limit', String(opts.limit))
      if (opts?.offset) params.set('offset', String(opts.offset))
      const res = await apiFetch(`/instances/${instanceId}/snapshots/diff?${params.toString()}`)
      return (await res.json()) as PGSSDiffResult
    },
    enabled: !!instanceId && fromId != null && toId != null,
  })
}

export function useQueryInsights(instanceId: string, queryId?: number | null) {
  return useQuery({
    queryKey: ['pgss-query-insights', instanceId, queryId],
    queryFn: async () => {
      const res = await apiFetch(`/instances/${instanceId}/query-insights/${queryId}`)
      return (await res.json()) as PGSSQueryInsight
    },
    enabled: !!instanceId && queryId != null,
  })
}

export function useWorkloadReport(
  instanceId: string,
  fromId?: number | null,
  toId?: number | null,
) {
  return useQuery({
    queryKey: ['pgss-workload-report', instanceId, fromId, toId],
    queryFn: async () => {
      const params = new URLSearchParams()
      if (fromId != null) params.set('from', String(fromId))
      if (toId != null) params.set('to', String(toId))
      const qs = params.toString()
      const path = `/instances/${instanceId}/workload-report${qs ? `?${qs}` : ''}`
      const res = await apiFetch(path)
      return (await res.json()) as PGSSWorkloadReport
    },
    enabled: !!instanceId && (fromId != null && toId != null),
  })
}

export function useManualCapture(instanceId: string) {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async () => {
      const res = await apiFetch(`/instances/${instanceId}/snapshots/capture`, {
        method: 'POST',
      })
      return (await res.json()) as PGSSSnapshot
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['pgss-snapshots', instanceId] })
      queryClient.invalidateQueries({ queryKey: ['pgss-latest-diff', instanceId] })
    },
  })
}
