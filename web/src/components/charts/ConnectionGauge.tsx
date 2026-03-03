import { useMemo } from 'react'
import type { EChartsOption } from 'echarts'
import { EChartWrapper } from '@/components/ui/EChartWrapper'

interface ConnectionGaugeProps {
  value: number
  max: number
  size?: number
}

export function ConnectionGauge({ value, max, size = 200 }: ConnectionGaugeProps) {
  const option = useMemo((): EChartsOption => ({
    series: [
      {
        type: 'gauge',
        startAngle: 200,
        endAngle: -20,
        min: 0,
        max,
        splitNumber: 5,
        pointer: {
          show: true,
          length: '60%',
          width: 4,
          itemStyle: { color: 'auto' },
        },
        axisLine: {
          lineStyle: {
            width: 16,
            color: [
              [0.7, '#10b981'],
              [0.9, '#f59e0b'],
              [1, '#ef4444'],
            ],
          },
        },
        axisTick: { show: false },
        splitLine: {
          length: 8,
          lineStyle: { color: '#475569', width: 1 },
        },
        axisLabel: { show: false },
        detail: {
          valueAnimation: true,
          formatter: `{value} / ${max}`,
          fontSize: 14,
          color: '#cbd5e1',
          offsetCenter: [0, '70%'],
        },
        data: [{ value }],
      },
    ],
  }), [value, max])

  return <EChartWrapper option={option} height={size} />
}
