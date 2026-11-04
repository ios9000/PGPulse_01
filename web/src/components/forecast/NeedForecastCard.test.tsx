import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { NeedForecastCard } from './NeedForecastCard'
import { useMaintenanceForecasts } from '@/hooks/useMaintenanceForecast'
import type { ForecastSummary, MaintenanceForecast } from '@/hooks/useMaintenanceForecast'
import { makeForecast } from '@/test/fixtures'

vi.mock('@/hooks/useMaintenanceForecast', () => ({ useMaintenanceForecasts: vi.fn() }))
const mockedHook = vi.mocked(useMaintenanceForecasts)

type HookResult = ReturnType<typeof useMaintenanceForecasts>

function setHook(state: Partial<{ data: unknown; isLoading: boolean; isError: boolean }>) {
  mockedHook.mockReturnValue({ data: undefined, isLoading: false, isError: false, ...state } as unknown as HookResult)
}

function summary(overrides: Partial<ForecastSummary> = {}): ForecastSummary {
  return { imminent_count: 0, overdue_count: 0, predicted_count: 0, total_tables_evaluated: 10, ...overrides }
}

function withData(forecasts: MaintenanceForecast[], s: ForecastSummary) {
  setHook({ data: { forecasts, summary: s } })
}

beforeEach(() => {
  mockedHook.mockReset()
})

describe('NeedForecastCard', () => {
  it('passes the instance id to the forecasts hook', () => {
    setHook({ isLoading: true })
    render(<NeedForecastCard instanceId="inst-7" />)
    expect(mockedHook).toHaveBeenCalledWith('inst-7')
  })

  it.each([
    ['loading', { isLoading: true }],
    ['error', { isError: true }],
  ])('renders nothing while %s', (_label, state) => {
    setHook(state)
    const { container } = render(<NeedForecastCard instanceId="inst-1" />)
    expect(container.innerHTML).toBe('')
  })

  it('shows the accumulating message when nothing has been evaluated yet', () => {
    withData([], summary({ total_tables_evaluated: 0 }))
    render(<NeedForecastCard instanceId="inst-1" />)
    expect(screen.getByText('No forecasts yet — data accumulating')).toBeTruthy()
  })

  it('renders nothing when tables were evaluated and none need maintenance', () => {
    withData([makeForecast({ status: 'not_needed' })], summary())
    const { container } = render(<NeedForecastCard instanceId="inst-1" />)
    expect(container.innerHTML).toBe('')
  })

  it('shows only non-zero counters, each with its status colour', () => {
    withData([], summary({ overdue_count: 2, imminent_count: 0, predicted_count: 5 }))
    render(<NeedForecastCard instanceId="inst-1" />)
    expect(screen.getByText('2 Overdue').classList).toContain('text-red-400')
    expect(screen.getByText('5 Predicted').classList).toContain('text-blue-400')
    expect(screen.queryByText(/Imminent/)).toBeNull()
  })

  it('names the soonest imminent/predicted operation as "Next", ignoring overdue and not_needed', () => {
    withData(
      [
        makeForecast({ status: 'overdue', table_name: 'old_tbl', time_until_sec: 0 }),
        makeForecast({ status: 'not_needed', table_name: 'idle_tbl', time_until_sec: 10 }),
        makeForecast({ status: 'predicted', table_name: 'later_tbl', time_until_sec: 3 * 86400 }),
        makeForecast({ status: 'imminent', operation: 'analyze', table_name: 'hot_tbl', time_until_sec: 1800 }),
      ],
      summary({ overdue_count: 1, imminent_count: 1, predicted_count: 1 }),
    )
    render(<NeedForecastCard instanceId="inst-1" />)
    expect(screen.getByText('Next: analyze on hot_tbl (~30m)')).toBeTruthy()
    expect(screen.getByText('1 Imminent').classList).toContain('text-yellow-400')
  })

  it('omits the "Next" line when only overdue forecasts exist', () => {
    withData([makeForecast({ status: 'overdue' })], summary({ overdue_count: 1 }))
    render(<NeedForecastCard instanceId="inst-1" />)
    expect(screen.getByText('1 Overdue')).toBeTruthy()
    expect(screen.queryByText(/^Next:/)).toBeNull()
  })
})
