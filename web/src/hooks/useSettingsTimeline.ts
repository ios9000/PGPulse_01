import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { apiFetch } from '@/lib/api'

export interface SnapshotMeta {
  id: number
  instance_id: string
  captured_at: string
  trigger_type: string
  pg_version: string
}

export interface SettingChange {
  name: string
  old_value?: string
  new_value?: string
  unit?: string
  source?: string
}

export interface SettingsDiffResult {
  changed: SettingChange[]
  added: SettingChange[]
  removed: SettingChange[]
  pending_restart: string[]
}

export function useSettingsSnapshots(instanceId: string) {
  return useQuery({
    queryKey: ['settings', 'history', instanceId],
    queryFn: async () => {
      const res = await apiFetch(`/instances/${instanceId}/settings/history`)
      const json = await res.json()
      return json.data as SnapshotMeta[]
    },
    refetchInterval: 60_000,
    enabled: !!instanceId,
  })
}

export function useSettingsDiffBetween(instanceId: string, fromId: number | null, toId: number | null) {
  return useQuery({
    queryKey: ['settings', 'diff-between', instanceId, fromId, toId],
    queryFn: async () => {
      const res = await apiFetch(`/instances/${instanceId}/settings/diff?from=${fromId}&to=${toId}`)
      const json = await res.json()
      return json.data as SettingsDiffResult
    },
    enabled: !!instanceId && fromId !== null && toId !== null,
  })
}

export function usePendingRestart(instanceId: string) {
  return useQuery({
    queryKey: ['settings', 'pending-restart', instanceId],
    queryFn: async () => {
      const res = await apiFetch(`/instances/${instanceId}/settings/pending-restart`)
      const json = await res.json()
      return json.data as SettingChange[]
    },
    refetchInterval: 60_000,
    enabled: !!instanceId,
  })
}

export function useManualSnapshot(instanceId: string) {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async () => {
      const res = await apiFetch(`/instances/${instanceId}/settings/snapshot`, {
        method: 'POST',
      })
      return res.json()
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['settings', 'history', instanceId] })
    },
  })
}
