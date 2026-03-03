import ReactECharts from 'echarts-for-react'
import type { EChartsOption } from 'echarts'
import { useThemeStore } from '@/stores/themeStore'
import { pgpDarkTheme, pgpLightTheme } from '@/lib/echarts-theme'

interface EChartWrapperProps {
  option: EChartsOption
  height?: string | number
  loading?: boolean
  className?: string
}

export function EChartWrapper({
  option,
  height = 300,
  loading,
  className,
}: EChartWrapperProps) {
  const resolvedTheme = useThemeStore((s) => s.resolvedTheme)
  const theme = resolvedTheme === 'dark' ? pgpDarkTheme : pgpLightTheme

  return (
    <ReactECharts
      option={{ ...option, backgroundColor: 'transparent' }}
      theme={theme}
      style={{ height, width: '100%' }}
      opts={{ renderer: 'canvas' }}
      showLoading={loading}
      className={className}
      notMerge={true}
    />
  )
}
