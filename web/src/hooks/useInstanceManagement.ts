import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { apiFetch } from '@/lib/api'
import type {
  ManagedInstance,
  CreateInstanceRequest,
  UpdateInstanceRequest,
  TestConnectionResult,
  BulkImportResult,
} from '@/types/models'

export function useManagedInstances() {
  return useQuery({
    queryKey: ['managed-instances'],
    queryFn: async () => {
      const res = await apiFetch('/instances')
      const json = await res.json()
      return json.data as ManagedInstance[]
    },
    refetchInterval: 30_000,
  })
}

export function useCreateInstance() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (payload: CreateInstanceRequest) => {
      const res = await apiFetch('/instances', {
        method: 'POST',
        body: JSON.stringify(payload),
      })
      return res.json()
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['managed-instances'] })
      qc.invalidateQueries({ queryKey: ['instances'] })
    },
  })
}

export function useUpdateInstance() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ id, ...payload }: UpdateInstanceRequest & { id: string }) => {
      const res = await apiFetch(`/instances/${id}`, {
        method: 'PUT',
        body: JSON.stringify(payload),
      })
      return res.json()
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['managed-instances'] })
      qc.invalidateQueries({ queryKey: ['instances'] })
    },
  })
}

export function useDeleteInstance() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (id: string) => {
      await apiFetch(`/instances/${id}`, { method: 'DELETE' })
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['managed-instances'] })
      qc.invalidateQueries({ queryKey: ['instances'] })
    },
  })
}

export function useTestConnection() {
  return useMutation({
    mutationFn: async (id: string) => {
      const res = await apiFetch(`/instances/${id}/test`, { method: 'POST' })
      const json = await res.json()
      return json.data as TestConnectionResult
    },
  })
}

export function useBulkImport() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (csv: string) => {
      const res = await apiFetch('/instances/bulk', {
        method: 'POST',
        headers: { 'Content-Type': 'text/csv' },
        body: csv,
      })
      const json = await res.json()
      return json.data as BulkImportResult[]
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['managed-instances'] })
      qc.invalidateQueries({ queryKey: ['instances'] })
    },
  })
}
