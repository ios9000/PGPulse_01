import { useMemo } from 'react'
import type { EChartsOption } from 'echarts'
import { EChartWrapper } from '@/components/ui/EChartWrapper'
import type { WaitEvent } from '@/types/models'

interface WaitEventsChartProps {
  events: WaitEvent[]
}

const WAIT_TYPE_COLORS: Record<string, string> = {
  Lock: '#f43f5e',
  IO: '#3b82f6',
  LWLock: '#f59e0b',
  Client: '#94a3b8',
  Activity: '#10b981',
}
const DEFAULT_COLOR = '#8b5cf6'

export function WaitEventsChart({ events }: WaitEventsChartProps) {
  const option = useMemo((): EChartsOption => {
    const sorted = [...events]
      .sort((a, b) => b.count - a.count)
      .slice(0, 15)
      .reverse()

    return {
      tooltip: {
        trigger: 'axis',
        axisPointer: { type: 'shadow' },
      },
      grid: {
        left: '3%',
        right: '8%',
        bottom: '3%',
        top: '3%',
        containLabel: true,
      },
      xAxis: {
        type: 'value',
      },
      yAxis: {
        type: 'category',
        data: sorted.map((e) => e.wait_event),
        axisLabel: {
          width: 120,
          overflow: 'truncate',
        },
      },
      series: [
        {
          type: 'bar',
          data: sorted.map((e) => ({
            value: e.count,
            itemStyle: {
              color: WAIT_TYPE_COLORS[e.wait_event_type] ?? DEFAULT_COLOR,
            },
          })),
          barMaxWidth: 20,
        },
      ],
    }
  }, [events])

  return <EChartWrapper option={option} height={Math.max(200, events.length * 28)} />
}
