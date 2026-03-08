import { useOSMetrics } from '@/hooks/useOSMetrics'
import type { OSMetrics } from '@/types/models'

interface OSSystemSectionProps {
  instanceId: string
}

function formatUptime(seconds: number): string {
  const days = Math.floor(seconds / 86400)
  const hours = Math.floor((seconds % 86400) / 3600)
  if (days > 0) return `${days}d ${hours}h`
  const mins = Math.floor((seconds % 3600) / 60)
  return `${hours}h ${mins}m`
}

function formatKB(kb: number): string {
  if (kb >= 1048576) return `${(kb / 1048576).toFixed(1)} GB`
  if (kb >= 1024) return `${(kb / 1024).toFixed(0)} MB`
  return `${kb} KB`
}

function UsageBar({ pct, label }: { pct: number; label: string }) {
  const color = pct > 80 ? 'bg-red-500' : pct > 60 ? 'bg-yellow-500' : 'bg-green-500'
  return (
    <div className="space-y-1">
      <div className="flex items-center justify-between text-xs text-pgp-text-secondary">
        <span>{label}</span>
        <span>{pct.toFixed(1)}%</span>
      </div>
      <div className="h-2 w-full rounded-full bg-pgp-bg-secondary">
        <div
          className={`h-full rounded-full ${color}`}
          style={{ width: `${Math.min(pct, 100)}%` }}
        />
      </div>
    </div>
  )
}

function OSDetails({ data }: { data: OSMetrics }) {
  const cpuPct = data.cpu.user_pct + data.cpu.system_pct + data.cpu.iowait_pct
  const memPct =
    data.memory.total_kb > 0
      ? ((data.memory.used_kb / data.memory.total_kb) * 100)
      : 0

  return (
    <div className="space-y-4">
      <div className="grid grid-cols-2 gap-4 text-sm lg:grid-cols-4">
        <div>
          <span className="text-pgp-text-muted">Hostname</span>
          <p className="font-mono text-pgp-text-primary">{data.hostname}</p>
        </div>
        <div>
          <span className="text-pgp-text-muted">OS</span>
          <p className="text-pgp-text-primary">
            {data.os_release.name} {data.os_release.version}
          </p>
        </div>
        <div>
          <span className="text-pgp-text-muted">Uptime</span>
          <p className="text-pgp-text-primary">{formatUptime(data.uptime_seconds)}</p>
        </div>
        <div>
          <span className="text-pgp-text-muted">CPUs</span>
          <p className="text-pgp-text-primary">{data.cpu.num_cpus}</p>
        </div>
      </div>

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        <UsageBar pct={cpuPct} label="CPU Usage" />
        <UsageBar
          pct={memPct}
          label={`Memory (${formatKB(data.memory.used_kb)} / ${formatKB(data.memory.total_kb)})`}
        />
      </div>

      <div className="text-sm text-pgp-text-secondary">
        Load Average: {data.load_avg['1m'].toFixed(2)} / {data.load_avg['5m'].toFixed(2)} / {data.load_avg['15m'].toFixed(2)}{' '}
        <span className="text-pgp-text-muted">(1m/5m/15m)</span>
      </div>
    </div>
  )
}

export function OSSystemSection({ instanceId }: OSSystemSectionProps) {
  const { data, isLoading } = useOSMetrics(instanceId)

  if (isLoading) {
    return (
      <div className="rounded-lg border border-pgp-border bg-pgp-bg-card p-4">
        <h3 className="mb-3 text-sm font-medium text-pgp-text-secondary">System</h3>
        <div className="py-4 text-center text-sm text-pgp-text-muted">Loading OS metrics...</div>
      </div>
    )
  }

  if (!data?.available || !data.data) {
    return (
      <div className="rounded-lg border border-pgp-border bg-pgp-bg-card p-4">
        <h3 className="mb-3 text-sm font-medium text-pgp-text-secondary">OS Metrics</h3>
        <div className="rounded-md bg-yellow-500/10 px-4 py-3 text-sm text-yellow-400">
          OS Agent not configured. To enable OS metrics, deploy pgpulse-agent on this host
          and set <code className="rounded bg-pgp-bg-secondary px-1">agent_url</code> in the
          instance configuration.
        </div>
      </div>
    )
  }

  return (
    <div className="rounded-lg border border-pgp-border bg-pgp-bg-card p-4">
      <h3 className="mb-3 text-sm font-medium text-pgp-text-secondary">System</h3>
      <OSDetails data={data.data} />
    </div>
  )
}
