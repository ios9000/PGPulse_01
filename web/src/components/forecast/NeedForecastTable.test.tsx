import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import { NeedForecastTable } from './NeedForecastTable'
import { useMaintenanceForecasts } from '@/hooks/useMaintenanceForecast'
import type { MaintenanceForecast } from '@/hooks/useMaintenanceForecast'
import { makeForecast } from '@/test/fixtures'

vi.mock('@/hooks/useMaintenanceForecast', () => ({ useMaintenanceForecasts: vi.fn() }))
const mockedHook = vi.mocked(useMaintenanceForecasts)

type HookResult = ReturnType<typeof useMaintenanceForecasts>

function setHook(state: Partial<{ data: unknown; isLoading: boolean; isError: boolean }>) {
  mockedHook.mockReturnValue({ data: undefined, isLoading: false, isError: false, ...state } as unknown as HookResult)
}

function withForecasts(forecasts: MaintenanceForecast[]) {
  setHook({
    data: {
      forecasts,
      summary: { imminent_count: 0, overdue_count: 0, predicted_count: 0, total_tables_evaluated: forecasts.length },
    },
  })
}

function bodyRows() {
  return screen.getAllByRole('row').slice(1) // drop header row
}

function cells(row: HTMLElement) {
  return within(row).getAllByRole('cell')
}

beforeEach(() => {
  mockedHook.mockReset()
})

describe('NeedForecastTable', () => {
  it('shows a loading message while loading', () => {
    setHook({ isLoading: true })
    render(<NeedForecastTable instanceId="inst-1" />)
    expect(screen.getByText('Loading forecasts...')).toBeTruthy()
  })

  it('renders nothing on error', () => {
    setHook({ isError: true })
    const { container } = render(<NeedForecastTable instanceId="inst-1" />)
    expect(container.innerHTML).toBe('')
  })

  it('shows the accumulating message for an empty forecast list', () => {
    withForecasts([])
    render(<NeedForecastTable instanceId="inst-1" />)
    expect(screen.getByText('No forecasts yet — data accumulating')).toBeTruthy()
    expect(screen.queryByRole('table')).toBeNull()
  })

  it('renders one row per forecast with the expected columns', () => {
    withForecasts([
      makeForecast({
        database: 'app',
        table_name: 'orders',
        operation: 'vacuum',
        time_until_sec: 5400,
        current_value: 1499.6,
        threshold_value: 5000,
        accumulation_rate: 0.256,
        method: 'threshold_projection+ml',
      }),
      makeForecast({ table_name: 'users', operation: 'analyze', method: 'threshold_projection' }),
    ])
    render(<NeedForecastTable instanceId="inst-1" />)

    const rows = bodyRows()
    expect(rows).toHaveLength(2)
    const c = cells(rows[0]).map((td) => td.textContent)
    expect(c.slice(0, 8)).toEqual([
      'predicted',
      'app',
      'orders',
      'vacuum',
      '1h 30m',
      '1500 / 5000',
      '0.26/s',
      'Threshold + ML',
    ])
    expect(cells(rows[1])[7].textContent).toBe('Threshold')
  })

  it.each([
    ['overdue', 'text-red-400', 'bg-red-500/20'],
    ['imminent', 'text-yellow-400', 'bg-yellow-500/20'],
    ['predicted', 'text-blue-400', 'bg-blue-500/20'],
    ['not_needed', 'text-slate-400', 'bg-slate-500/20'],
    ['insufficient_data', 'text-slate-500', 'border-dashed'],
  ] as const)('styles the %s status badge with %s', (status, textClass, extraClass) => {
    withForecasts([makeForecast({ status })])
    render(<NeedForecastTable instanceId="inst-1" />)
    const badge = within(cells(bodyRows()[0])[0]).getByText(status.replace('_', ' '))
    expect(badge.classList).toContain(textClass)
    expect(badge.classList).toContain(extraClass)
  })

  it('shows "overdue" in the Time Until column for overdue rows regardless of seconds', () => {
    withForecasts([makeForecast({ status: 'overdue', time_until_sec: 9999 })])
    render(<NeedForecastTable instanceId="inst-1" />)
    expect(cells(bodyRows()[0])[4].textContent).toBe('overdue')
  })

  it('uses em dashes for missing names, non-positive time, and zero rate', () => {
    withForecasts([
      makeForecast({ status: 'not_needed', database: '', table_name: '', time_until_sec: 0, accumulation_rate: 0 }),
    ])
    render(<NeedForecastTable instanceId="inst-1" />)
    const c = cells(bodyRows()[0]).map((td) => td.textContent)
    expect(c[1]).toBe('—')
    expect(c[2]).toBe('—')
    expect(c[4]).toBe('—')
    expect(c[6]).toBe('—')
  })

  it('passes unknown methods through verbatim', () => {
    withForecasts([makeForecast({ method: 'seasonal_ml' })])
    render(<NeedForecastTable instanceId="inst-1" />)
    expect(cells(bodyRows()[0])[7].textContent).toBe('seasonal_ml')
  })
})
