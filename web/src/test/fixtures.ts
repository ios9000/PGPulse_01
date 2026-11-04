import type { MaintenanceForecast, OperationETA } from '@/hooks/useMaintenanceForecast'

export function makeETA(overrides: Partial<OperationETA> = {}): OperationETA {
  return {
    instance_id: 'inst-1',
    pid: 4242,
    operation: 'vacuum',
    database: 'app',
    table: 'orders',
    phase: 'scanning heap',
    percent_done: 40,
    started_at: '2026-11-04T10:00:00Z',
    elapsed_sec: 120,
    eta_sec: 300,
    eta_at: '2026-11-04T10:07:00Z',
    rate_current: 1.5,
    confidence: 'high',
    sample_count: 12,
    ...overrides,
  }
}

export function makeForecast(overrides: Partial<MaintenanceForecast> = {}): MaintenanceForecast {
  return {
    id: 1,
    instance_id: 'inst-1',
    database: 'app',
    table_name: 'orders',
    operation: 'vacuum',
    status: 'predicted',
    predicted_at: '2026-11-05T10:00:00Z',
    time_until_sec: 7200,
    confidence_lower: null,
    confidence_upper: null,
    current_value: 1500,
    threshold_value: 5000,
    accumulation_rate: 0.25,
    method: 'threshold_projection',
    evaluated_at: '2026-11-04T10:00:00Z',
    ...overrides,
  }
}
