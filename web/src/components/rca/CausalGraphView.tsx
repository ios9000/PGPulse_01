import { useMemo } from 'react'
import { EChartWrapper } from '@/components/ui/EChartWrapper'
import { Spinner } from '@/components/ui/Spinner'
import { EmptyState } from '@/components/ui/EmptyState'
import { GitBranch } from 'lucide-react'
import { useRCAGraph } from '@/hooks/useRCA'
import type { EChartsOption } from 'echarts'

const LAYER_COLORS: Record<string, string> = {
  db: '#60a5fa',
  os: '#4ade80',
  workload: '#c084fc',
  config: '#fb923c',
}

export function CausalGraphView() {
  const { data: graph, isLoading } = useRCAGraph()

  const option = useMemo((): EChartsOption | null => {
    if (!graph || Object.keys(graph.nodes).length === 0) return null

    const nodes = Object.values(graph.nodes).map((node) => ({
      id: node.id,
      name: node.name,
      symbolSize: 40,
      itemStyle: {
        color: LAYER_COLORS[node.layer] ?? '#9ca3af',
      },
      category: node.layer,
      label: {
        show: true,
        fontSize: 10,
      },
      tooltip: {
        formatter: () => {
          const keys = node.metric_keys?.join('<br/>') ?? 'none'
          return `<b>${node.name}</b><br/>Layer: ${node.layer}<br/>Metrics:<br/>${keys}`
        },
      },
    }))

    const edges = graph.edges.map((edge) => ({
      source: edge.from_node,
      target: edge.to_node,
      lineStyle: {
        width: Math.max(1, edge.base_confidence * 3),
        curveness: 0.2,
      },
      tooltip: {
        formatter: () => {
          const lag =
            edge.min_lag_seconds === edge.max_lag_seconds
              ? `${edge.min_lag_seconds}s`
              : `${edge.min_lag_seconds}s - ${edge.max_lag_seconds}s`
          return `${edge.description}<br/>Lag: ${lag}<br/>Confidence: ${Math.round(edge.base_confidence * 100)}%`
        },
      },
    }))

    const categories = ['db', 'os', 'workload', 'config'].map((layer) => ({
      name: layer,
      itemStyle: { color: LAYER_COLORS[layer] },
    }))

    return {
      tooltip: { trigger: 'item' },
      legend: {
        data: categories.map((c) => c.name),
        top: 10,
        textStyle: { fontSize: 11 },
      },
      series: [
        {
          type: 'graph',
          layout: 'force',
          data: nodes,
          links: edges,
          categories,
          roam: true,
          draggable: true,
          force: {
            repulsion: 200,
            edgeLength: [80, 160],
            gravity: 0.1,
          },
          edgeSymbol: ['none', 'arrow'],
          edgeSymbolSize: 8,
          label: {
            show: true,
            position: 'bottom',
            fontSize: 10,
          },
          lineStyle: {
            color: 'source',
            opacity: 0.6,
          },
          emphasis: {
            focus: 'adjacency',
            lineStyle: { width: 4 },
          },
        },
      ],
    }
  }, [graph])

  if (isLoading) {
    return (
      <div className="flex justify-center py-12">
        <Spinner size="lg" />
      </div>
    )
  }

  if (!option) {
    return (
      <EmptyState
        icon={GitBranch}
        title="No causal graph data"
        description="The causal knowledge graph has not been populated yet."
      />
    )
  }

  return <EChartWrapper option={option} height={500} />
}
