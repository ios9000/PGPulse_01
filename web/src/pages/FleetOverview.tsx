import { useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { PageHeader } from '@/components/ui/PageHeader'
import { MetricCard } from '@/components/ui/MetricCard'
import { EChartWrapper } from '@/components/ui/EChartWrapper'
import { DataTable, type Column } from '@/components/ui/DataTable'
import { StatusBadge } from '@/components/ui/StatusBadge'
import type { EChartsOption } from 'echarts'

interface MockServer extends Record<string, unknown> {
  name: string
  host: string
  port: number
  pg_version: string
  status: 'ok' | 'warning' | 'critical'
  connections: number
}

const mockServers: MockServer[] = [
  { name: 'prod-primary', host: 'db-primary.example.com', port: 5432, pg_version: '16.2', status: 'ok', connections: 24 },
  { name: 'prod-replica', host: 'db-replica.example.com', port: 5432, pg_version: '16.2', status: 'ok', connections: 12 },
  { name: 'staging', host: 'db-staging.internal', port: 5432, pg_version: '15.4', status: 'warning', connections: 11 },
]

const mockChartData = Array.from({ length: 24 }, (_, i) => ({
  time: `${String(i).padStart(2, '0')}:00`,
  value: Math.floor(Math.random() * 30) + 30,
}))

export function FleetOverview() {
  const navigate = useNavigate()
  const [sortColumn, setSortColumn] = useState<string>('name')
  const [sortDirection, setSortDirection] = useState<'asc' | 'desc'>('asc')

  const columns: Column<MockServer>[] = useMemo(
    () => [
      { key: 'name', label: 'Name', sortable: true },
      {
        key: 'host',
        label: 'Host:Port',
        render: (row) => `${row.host}:${row.port}`,
        mono: true,
      },
      { key: 'pg_version', label: 'PG Version', mono: true },
      {
        key: 'status',
        label: 'Status',
        render: (row) => <StatusBadge status={row.status} label={row.status} pulse={row.status === 'critical'} />,
      },
      { key: 'connections', label: 'Connections', align: 'right' as const, mono: true },
    ],
    [],
  )

  const sortedData = useMemo(() => {
    const sorted = [...mockServers].sort((a, b) => {
      const aVal = a[sortColumn as keyof MockServer]
      const bVal = b[sortColumn as keyof MockServer]
      if (typeof aVal === 'string' && typeof bVal === 'string') {
        return sortDirection === 'asc' ? aVal.localeCompare(bVal) : bVal.localeCompare(aVal)
      }
      return sortDirection === 'asc'
        ? Number(aVal) - Number(bVal)
        : Number(bVal) - Number(aVal)
    })
    return sorted
  }, [sortColumn, sortDirection])

  const handleSort = (column: string) => {
    if (sortColumn === column) {
      setSortDirection((d) => (d === 'asc' ? 'desc' : 'asc'))
    } else {
      setSortColumn(column)
      setSortDirection('asc')
    }
  }

  const chartOption: EChartsOption = {
    title: { text: 'Connections (mock data)', left: 'center' },
    tooltip: { trigger: 'axis' },
    xAxis: {
      type: 'category',
      data: mockChartData.map((d) => d.time),
    },
    yAxis: { type: 'value', min: 20, max: 70 },
    series: [
      {
        name: 'Connections',
        type: 'line',
        data: mockChartData.map((d) => d.value),
        smooth: true,
        areaStyle: { opacity: 0.1 },
      },
    ],
    grid: { left: '3%', right: '4%', bottom: '3%', containLabel: true },
  }

  return (
    <div>
      <PageHeader title="Fleet Overview" subtitle="Monitor all PostgreSQL instances" />

      <div className="mb-6 grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <MetricCard label="Servers" value={3} status="ok" trend="flat" trendValue="0%" />
        <MetricCard label="Active Alerts" value={2} status="warning" trend="up" trendValue="+1" />
        <MetricCard label="Avg Cache Hit" value="99.2" unit="%" status="ok" trend="up" trendValue="+0.1%" />
        <MetricCard label="Connections" value="47/200" status="ok" trend="down" trendValue="-3" />
      </div>

      <div className="mb-6 rounded-lg border border-pgp-border bg-pgp-bg-card p-4">
        <EChartWrapper option={chartOption} height={280} />
      </div>

      <DataTable
        columns={columns}
        data={sortedData}
        sortColumn={sortColumn}
        sortDirection={sortDirection}
        onSort={handleSort}
        onRowClick={(row) => navigate(`/servers/${row.name}`)}
      />
    </div>
  )
}
