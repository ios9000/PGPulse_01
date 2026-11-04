import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import type { QueryClient, QueryKey } from '@tanstack/react-query'
import { useETAForInstance, useMaintenanceForecasts, useOperationHistory } from './useMaintenanceForecast'
import { createTestQueryClient, makeWrapper } from '@/test/queryWrapper'

function jsonResponse(data: unknown) {
  return new Response(JSON.stringify({ data }), {
    status: 200,
    headers: { 'Content-Type': 'application/json' },
  })
}

function observerOptions(client: QueryClient, queryKey: QueryKey) {
  const query = client.getQueryCache().find({ queryKey, exact: true })
  expect(query, `query ${JSON.stringify(queryKey)} should be in the cache`).toBeDefined()
  return query!.observers[0].options
}

let fetchMock: ReturnType<typeof vi.fn>
let client: QueryClient

beforeEach(() => {
  fetchMock = vi.fn()
  vi.stubGlobal('fetch', fetchMock)
  client = createTestQueryClient()
})

afterEach(() => {
  client.clear()
  vi.unstubAllGlobals()
})

describe('useETAForInstance', () => {
  it('fetches the ETA endpoint, unwraps .data, and polls every 15s', async () => {
    const payload = { operations: [], evaluated_at: '2026-11-04T10:00:00Z' }
    fetchMock.mockResolvedValue(jsonResponse(payload))

    const { result } = renderHook(() => useETAForInstance('inst-1'), { wrapper: makeWrapper(client) })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(result.current.data).toEqual(payload)
    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(fetchMock.mock.calls[0][0]).toBe('/api/v1/instances/inst-1/forecast/eta')
    expect(observerOptions(client, ['forecast-eta', 'inst-1']).refetchInterval).toBe(15_000)
  })

  it('does not fetch when instanceId is undefined', async () => {
    const { result } = renderHook(() => useETAForInstance(undefined), { wrapper: makeWrapper(client) })
    expect(result.current.fetchStatus).toBe('idle')
    await new Promise((r) => setTimeout(r, 20))
    expect(fetchMock).not.toHaveBeenCalled()
  })
})

describe('useMaintenanceForecasts', () => {
  const payload = {
    forecasts: [],
    summary: { imminent_count: 0, overdue_count: 0, predicted_count: 0, total_tables_evaluated: 0 },
  }

  it('uses the needs endpoint with no query string when unfiltered, polling every 60s', async () => {
    fetchMock.mockResolvedValue(jsonResponse(payload))
    const { result } = renderHook(() => useMaintenanceForecasts('inst-1'), { wrapper: makeWrapper(client) })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(fetchMock.mock.calls[0][0]).toBe('/api/v1/instances/inst-1/forecast/needs')
    expect(observerOptions(client, ['forecast-needs', 'inst-1', undefined]).refetchInterval).toBe(60_000)
  })

  it('encodes status (comma-joined) and operation filters and includes them in the query key', async () => {
    fetchMock.mockResolvedValue(jsonResponse(payload))
    const filters = { status: ['imminent', 'overdue'], operation: 'vacuum' }
    const { result } = renderHook(() => useMaintenanceForecasts('inst-1', filters), {
      wrapper: makeWrapper(client),
    })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    const url = new URL(fetchMock.mock.calls[0][0] as string, 'http://localhost')
    expect(url.pathname).toBe('/api/v1/instances/inst-1/forecast/needs')
    expect(url.searchParams.get('status')).toBe('imminent,overdue')
    expect(url.searchParams.get('operation')).toBe('vacuum')
    expect(client.getQueryCache().find({ queryKey: ['forecast-needs', 'inst-1', filters], exact: true })).toBeDefined()
  })

  it('surfaces HTTP errors as query errors', async () => {
    fetchMock.mockResolvedValue(new Response('boom', { status: 500 }))
    const { result } = renderHook(() => useMaintenanceForecasts('inst-1'), { wrapper: makeWrapper(client) })
    await waitFor(() => expect(result.current.isError).toBe(true))
    expect(result.current.error?.message).toBe('boom')
  })
})

describe('useOperationHistory', () => {
  it('maps perPage to per_page, keys by filters, and does not poll', async () => {
    const payload = { operations: [], total: 0, page: 2, per_page: 25 }
    fetchMock.mockResolvedValue(jsonResponse(payload))
    const filters = { operation: 'analyze', page: 2, perPage: 25 }
    const { result } = renderHook(() => useOperationHistory('inst-9', filters), { wrapper: makeWrapper(client) })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    const url = new URL(fetchMock.mock.calls[0][0] as string, 'http://localhost')
    expect(url.pathname).toBe('/api/v1/instances/inst-9/forecast/history')
    expect(Object.fromEntries(url.searchParams)).toEqual({ operation: 'analyze', page: '2', per_page: '25' })
    expect(observerOptions(client, ['forecast-history', 'inst-9', filters]).refetchInterval).toBeUndefined()
    expect(result.current.data).toEqual(payload)
  })
})
