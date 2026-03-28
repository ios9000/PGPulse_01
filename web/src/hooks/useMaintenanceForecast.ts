import { useQuery } from '@tanstack/react-query'
import { apiFetch } from '@/lib/api'

// --- Types ---

export interface OperationETA {
  instance_id: string
  pid: number
  operation: string
  database: string
  table: string
  phase: string
  percent_done: number
  started_at: string
  elapsed_sec: number
  eta_sec: number
  eta_at: string
  rate_current: number
  confidence: 'high' | 'medium' | 'estimating' | 'stalled'
  sample_count: number
}

export interface MaintenanceForecast {
  id: number
  instance_id: string
  database: string
  table_name: string
  operation: string
  status: 'predicted' | 'imminent' | 'overdue' | 'not_needed' | 'insufficient_data'
  predicted_at: string | null
  time_until_sec: number
  confidence_lower: string | null
  confidence_upper: string | null
  current_value: number
  threshold_value: number
  accumulation_rate: number
  method: string
  evaluated_at: string
}

export interface ForecastSummary {
  imminent_count: number
  overdue_count: number
  predicted_count: number
  total_tables_evaluated: number
}

export interface MaintenanceOperation {
  id: number
  instance_id: string
  operation: string
  outcome: string
  database: string
  table_name: string
  table_size_bytes: number
  started_at: string
  completed_at: string
  duration_sec: number
  final_pct: number
  avg_rate_per_sec: number
  metadata: Record<string, unknown>
  created_at: string
}

// --- Hooks ---

export function useETAForInstance(instanceId: string | undefined) {
  return useQuery({
    queryKey: ['forecast-eta', instanceId],
    queryFn: async () => {
      const res = await apiFetch(`/instances/${instanceId}/forecast/eta`)
      const json = await res.json()
      return json.data as { operations: OperationETA[]; evaluated_at: string }
    },
    refetchInterval: 15_000,
    enabled: !!instanceId,
  })
}

export function useMaintenanceForecasts(
  instanceId: string | undefined,
  filters?: { status?: string[]; operation?: string },
) {
  return useQuery({
    queryKey: ['forecast-needs', instanceId, filters],
    queryFn: async () => {
      const params = new URLSearchParams()
      if (filters?.status?.length) params.set('status', filters.status.join(','))
      if (filters?.operation) params.set('operation', filters.operation)
      const qs = params.toString()
      const res = await apiFetch(
        `/instances/${instanceId}/forecast/needs${qs ? `?${qs}` : ''}`,
      )
      const json = await res.json()
      return json.data as { forecasts: MaintenanceForecast[]; summary: ForecastSummary }
    },
    refetchInterval: 60_000,
    enabled: !!instanceId,
  })
}

export function useOperationHistory(
  instanceId: string | undefined,
  filters?: { operation?: string; page?: number; perPage?: number },
) {
  return useQuery({
    queryKey: ['forecast-history', instanceId, filters],
    queryFn: async () => {
      const params = new URLSearchParams()
      if (filters?.operation) params.set('operation', filters.operation)
      if (filters?.page) params.set('page', String(filters.page))
      if (filters?.perPage) params.set('per_page', String(filters.perPage))
      const qs = params.toString()
      const res = await apiFetch(
        `/instances/${instanceId}/forecast/history${qs ? `?${qs}` : ''}`,
      )
      const json = await res.json()
      return json.data as { operations: MaintenanceOperation[]; total: number; page: number; per_page: number }
    },
    enabled: !!instanceId,
  })
}
