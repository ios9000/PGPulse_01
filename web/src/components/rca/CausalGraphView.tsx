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
    if (!graph || Object.keys(graph.Nodes).length === 0) return null

    const nodes = Object.values(graph.Nodes).map((node) => ({
      id: node.ID,
      name: node.Name,
      symbolSize: 40,
      itemStyle: {
        color: LAYER_COLORS[node.Layer] ?? '#9ca3af',
      },
      category: node.Layer,
      label: {
        show: true,
        fontSize: 10,
      },
      tooltip: {
        formatter: () => {
          const keys = node.MetricKeys?.join('<br/>') ?? 'none'
          return `<b>${node.Name}</b><br/>Layer: ${node.Layer}<br/>Metrics:<br/>${keys}`
        },
      },
    }))

    const edges = graph.Edges.map((edge) => ({
      source: edge.FromNode,
      target: edge.ToNode,
      lineStyle: {
        width: Math.max(1, edge.BaseConfidence * 3),
        curveness: 0.2,
      },
      tooltip: {
        formatter: () => {
          const lag =
            edge.MinLag === edge.MaxLag
              ? `${edge.MinLag}s`
              : `${edge.MinLag}s - ${edge.MaxLag}s`
          return `${edge.Description}<br/>Lag: ${lag}<br/>Confidence: ${Math.round(edge.BaseConfidence * 100)}%`
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
