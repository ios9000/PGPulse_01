import { useState, useEffect } from 'react'
import { apiFetch } from '@/lib/api'

export interface ForecastPoint {
  offset: number
  predicted_at: string
  value: number
  lower: number
  upper: number
}

export interface ForecastResult {
  instance_id: string
  metric: string
  generated_at: string
  collection_interval_seconds: number
  horizon: number
  confidence_z: number
  points: ForecastPoint[]
}

const FORECAST_POLL_MS = 5 * 60 * 1000

export function useForecast(
  instanceId: string,
  metric: string,
  horizon = 60,
): ForecastResult | null {
  const [result, setResult] = useState<ForecastResult | null>(null)

  useEffect(() => {
    const controller = new AbortController()

    const doFetch = async () => {
      try {
        const res = await apiFetch(
          `/instances/${instanceId}/metrics/${encodeURIComponent(metric)}/forecast?horizon=${horizon}`,
          { signal: controller.signal },
        )
        const json = await res.json()
        setResult(json.data as ForecastResult)
      } catch {
        setResult(null)
      }
    }

    doFetch()
    const timer = setInterval(doFetch, FORECAST_POLL_MS)
    return () => {
      controller.abort()
      clearInterval(timer)
    }
  }, [instanceId, metric, horizon])

  return result
}
