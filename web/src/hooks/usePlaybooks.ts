import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { apiFetch } from '@/lib/api'
import type {
  Playbook,
  PlaybookRun,
  ResolverResult,
  ExecuteStepResponse,
} from '@/types/playbook'

// --- CRUD ---

export function usePlaybooks(filters?: { status?: string; category?: string; search?: string }) {
  return useQuery({
    queryKey: ['playbooks', filters],
    queryFn: async () => {
      const qs = new URLSearchParams()
      if (filters?.status) qs.set('status', filters.status)
      if (filters?.category) qs.set('category', filters.category)
      if (filters?.search) qs.set('search', filters.search)
      const q = qs.toString()
      const res = await apiFetch(`/playbooks${q ? `?${q}` : ''}`)
      const json = await res.json()
      return json.data as Playbook[]
    },
  })
}

export function usePlaybook(id: number | undefined) {
  return useQuery({
    queryKey: ['playbook', id],
    queryFn: async () => {
      const res = await apiFetch(`/playbooks/${id}`)
      const json = await res.json()
      return json.data as Playbook
    },
    enabled: !!id,
  })
}

export function useCreatePlaybook() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (playbook: Partial<Playbook>) => {
      const res = await apiFetch('/playbooks', {
        method: 'POST',
        body: JSON.stringify(playbook),
      })
      const json = await res.json()
      return json.data as Playbook
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['playbooks'] })
    },
  })
}

export function useUpdatePlaybook() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (params: { id: number; playbook: Partial<Playbook> }) => {
      const res = await apiFetch(`/playbooks/${params.id}`, {
        method: 'PUT',
        body: JSON.stringify(params.playbook),
      })
      const json = await res.json()
      return json.data as Playbook
    },
    onSuccess: (_data, vars) => {
      queryClient.invalidateQueries({ queryKey: ['playbooks'] })
      queryClient.invalidateQueries({ queryKey: ['playbook', vars.id] })
    },
  })
}

export function useDeletePlaybook() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (id: number) => {
      await apiFetch(`/playbooks/${id}`, { method: 'DELETE' })
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['playbooks'] })
    },
  })
}

export function usePromotePlaybook() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (id: number) => {
      const res = await apiFetch(`/playbooks/${id}/promote`, { method: 'POST' })
      const json = await res.json()
      return json.data as Playbook
    },
    onSuccess: (_data, id) => {
      queryClient.invalidateQueries({ queryKey: ['playbooks'] })
      queryClient.invalidateQueries({ queryKey: ['playbook', id] })
    },
  })
}

export function useDeprecatePlaybook() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (id: number) => {
      const res = await apiFetch(`/playbooks/${id}/deprecate`, { method: 'POST' })
      const json = await res.json()
      return json.data as Playbook
    },
    onSuccess: (_data, id) => {
      queryClient.invalidateQueries({ queryKey: ['playbooks'] })
      queryClient.invalidateQueries({ queryKey: ['playbook', id] })
    },
  })
}

// --- Resolver ---

export function useResolvePlaybook(
  params:
    | {
        hook?: string
        root_cause?: string
        metric?: string
        adviser_rule?: string
        instance_id?: string
      }
    | undefined,
) {
  return useQuery({
    queryKey: ['playbook-resolve', params],
    queryFn: async () => {
      const qs = new URLSearchParams()
      if (params?.hook) qs.set('hook', params.hook)
      if (params?.root_cause) qs.set('root_cause', params.root_cause)
      if (params?.metric) qs.set('metric', params.metric)
      if (params?.adviser_rule) qs.set('adviser_rule', params.adviser_rule)
      if (params?.instance_id) qs.set('instance_id', params.instance_id)
      const q = qs.toString()
      const res = await apiFetch(`/playbooks/resolve${q ? `?${q}` : ''}`)
      const json = await res.json()
      return json.data as ResolverResult
    },
    enabled:
      !!params &&
      !!(params.hook || params.root_cause || params.metric || params.adviser_rule),
    staleTime: 60_000,
  })
}

// --- Runs ---

export function useStartRun() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (params: {
      instanceId: string
      playbookId: number
      triggerSource?: string
      triggerId?: string
    }) => {
      const res = await apiFetch(
        `/instances/${params.instanceId}/playbooks/${params.playbookId}/run`,
        {
          method: 'POST',
          body: JSON.stringify({
            trigger_source: params.triggerSource,
            trigger_id: params.triggerId,
          }),
        },
      )
      const json = await res.json()
      return json.data as PlaybookRun
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['playbook-runs'] })
    },
  })
}

export function usePlaybookRun(runId: number | undefined) {
  return useQuery({
    queryKey: ['playbook-run', runId],
    queryFn: async () => {
      const res = await apiFetch(`/playbook-runs/${runId}`)
      const json = await res.json()
      return json.data as PlaybookRun
    },
    enabled: !!runId,
    refetchInterval: (query) => {
      const run = query.state.data
      if (run && run.status === 'in_progress') return 5_000
      return false
    },
  })
}

export function useExecuteStep() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (params: { runId: number; stepOrder: number }) => {
      const res = await apiFetch(
        `/playbook-runs/${params.runId}/steps/${params.stepOrder}/execute`,
        { method: 'POST' },
      )
      const json = await res.json()
      return json.data as ExecuteStepResponse
    },
    onSuccess: (_data, vars) => {
      queryClient.invalidateQueries({ queryKey: ['playbook-run', vars.runId] })
    },
  })
}

export function useConfirmStep() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (params: { runId: number; stepOrder: number }) => {
      const res = await apiFetch(
        `/playbook-runs/${params.runId}/steps/${params.stepOrder}/confirm`,
        { method: 'POST' },
      )
      const json = await res.json()
      return json.data as ExecuteStepResponse
    },
    onSuccess: (_data, vars) => {
      queryClient.invalidateQueries({ queryKey: ['playbook-run', vars.runId] })
    },
  })
}

export function useApproveStep() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (params: { runId: number; stepOrder: number }) => {
      const res = await apiFetch(
        `/playbook-runs/${params.runId}/steps/${params.stepOrder}/approve`,
        { method: 'POST' },
      )
      const json = await res.json()
      return json.data as ExecuteStepResponse
    },
    onSuccess: (_data, vars) => {
      queryClient.invalidateQueries({ queryKey: ['playbook-run', vars.runId] })
    },
  })
}

export function useSkipStep() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (params: { runId: number; stepOrder: number }) => {
      const res = await apiFetch(
        `/playbook-runs/${params.runId}/steps/${params.stepOrder}/skip`,
        { method: 'POST' },
      )
      const json = await res.json()
      return json.data as ExecuteStepResponse
    },
    onSuccess: (_data, vars) => {
      queryClient.invalidateQueries({ queryKey: ['playbook-run', vars.runId] })
    },
  })
}

export function useRetryStep() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (params: { runId: number; stepOrder: number }) => {
      const res = await apiFetch(
        `/playbook-runs/${params.runId}/steps/${params.stepOrder}/retry`,
        { method: 'POST' },
      )
      const json = await res.json()
      return json.data as ExecuteStepResponse
    },
    onSuccess: (_data, vars) => {
      queryClient.invalidateQueries({ queryKey: ['playbook-run', vars.runId] })
    },
  })
}

export function useRequestApproval() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (params: { runId: number; stepOrder: number }) => {
      const res = await apiFetch(
        `/playbook-runs/${params.runId}/steps/${params.stepOrder}/request-approval`,
        { method: 'POST' },
      )
      const json = await res.json()
      return json.data as ExecuteStepResponse
    },
    onSuccess: (_data, vars) => {
      queryClient.invalidateQueries({ queryKey: ['playbook-run', vars.runId] })
    },
  })
}

export function useAbandonRun() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (runId: number) => {
      const res = await apiFetch(`/playbook-runs/${runId}/abandon`, { method: 'POST' })
      const json = await res.json()
      return json.data as PlaybookRun
    },
    onSuccess: (_data, runId) => {
      queryClient.invalidateQueries({ queryKey: ['playbook-run', runId] })
      queryClient.invalidateQueries({ queryKey: ['playbook-runs'] })
    },
  })
}

export function useSubmitFeedback() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (params: {
      runId: number
      helpful: boolean | null
      resolved: boolean | null
      notes: string
    }) => {
      const res = await apiFetch(`/playbook-runs/${params.runId}/feedback`, {
        method: 'POST',
        body: JSON.stringify({
          helpful: params.helpful,
          resolved: params.resolved,
          notes: params.notes,
        }),
      })
      return res.json()
    },
    onSuccess: (_data, vars) => {
      queryClient.invalidateQueries({ queryKey: ['playbook-run', vars.runId] })
    },
  })
}

export function usePlaybookRuns(filters?: {
  status?: string
  instance_id?: string
  playbook_id?: number
}) {
  return useQuery({
    queryKey: ['playbook-runs', filters],
    queryFn: async () => {
      const qs = new URLSearchParams()
      if (filters?.status) qs.set('status', filters.status)
      if (filters?.instance_id) qs.set('instance_id', filters.instance_id)
      if (filters?.playbook_id) qs.set('playbook_id', String(filters.playbook_id))
      const q = qs.toString()
      const res = await apiFetch(`/playbook-runs${q ? `?${q}` : ''}`)
      const json = await res.json()
      return json.data as PlaybookRun[]
    },
    refetchInterval: 30_000,
  })
}

export function useInstancePlaybookRuns(instanceId: string | undefined) {
  return useQuery({
    queryKey: ['playbook-runs', 'instance', instanceId],
    queryFn: async () => {
      const res = await apiFetch(`/instances/${instanceId}/playbook-runs`)
      const json = await res.json()
      return json.data as PlaybookRun[]
    },
    enabled: !!instanceId,
    refetchInterval: 30_000,
  })
}
