import { useMemo } from 'react'
import { useForecast } from './useForecast'
import { buildForecastSeries } from '../components/ForecastBand'

export function useForecastChart(instanceId: string, metric: string) {
  const forecast = useForecast(instanceId, metric)

  const extraSeries = useMemo(() => {
    if (!forecast?.points?.length) return undefined
    return buildForecastSeries(forecast.points)
  }, [forecast])

  const xAxisMax = useMemo(() => {
    if (!forecast?.points?.length) return undefined
    const lastPt = forecast.points[forecast.points.length - 1]
    return new Date(lastPt.predicted_at).getTime()
  }, [forecast])

  const hasForecast = !!forecast?.points?.length

  return { extraSeries, xAxisMax, hasForecast }
}
