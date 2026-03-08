import { useClusterMetrics } from '@/hooks/useOSMetrics'
import type { PatroniMember, ETCDMember } from '@/types/models'

interface ClusterSectionProps {
  instanceId: string
}

function roleBadge(role: string) {
  const colors: Record<string, string> = {
    leader: 'bg-green-500/20 text-green-400',
    master: 'bg-green-500/20 text-green-400',
    primary: 'bg-green-500/20 text-green-400',
    replica: 'bg-blue-500/20 text-blue-400',
    sync_standby: 'bg-purple-500/20 text-purple-400',
    standby_leader: 'bg-yellow-500/20 text-yellow-400',
  }
  const cls = colors[role] ?? 'bg-pgp-bg-secondary text-pgp-text-muted'
  return (
    <span className={`inline-block rounded px-2 py-0.5 text-xs font-medium ${cls}`}>{role}</span>
  )
}

function stateBadge(state: string) {
  const running = state === 'running' || state === 'streaming'
  return (
    <span
      className={`inline-block rounded px-2 py-0.5 text-xs font-medium ${
        running ? 'bg-green-500/20 text-green-400' : 'bg-red-500/20 text-red-400'
      }`}
    >
      {state}
    </span>
  )
}

function formatBytes(bytes: number): string {
  if (bytes >= 1073741824) return `${(bytes / 1073741824).toFixed(1)} GB`
  if (bytes >= 1048576) return `${(bytes / 1048576).toFixed(0)} MB`
  if (bytes >= 1024) return `${(bytes / 1024).toFixed(0)} KB`
  return `${bytes} B`
}

function PatroniTable({ members }: { members: PatroniMember[] }) {
  return (
    <div>
      <h4 className="mb-2 text-xs font-medium text-pgp-text-muted">Patroni Cluster</h4>
      <table className="w-full text-left text-sm">
        <thead>
          <tr className="border-b border-pgp-border text-xs text-pgp-text-muted">
            <th className="pb-2 pr-4">Name</th>
            <th className="pb-2 pr-4">Host</th>
            <th className="pb-2 pr-4">Role</th>
            <th className="pb-2 pr-4">State</th>
            <th className="pb-2 pr-4 text-right">Timeline</th>
            <th className="pb-2 text-right">Lag</th>
          </tr>
        </thead>
        <tbody>
          {members.map((m) => (
            <tr key={m.name} className="border-b border-pgp-border/50">
              <td className="py-2 pr-4 font-mono text-pgp-text-primary">{m.name}</td>
              <td className="py-2 pr-4 text-pgp-text-secondary">
                {m.host}:{m.port}
              </td>
              <td className="py-2 pr-4">{roleBadge(m.role)}</td>
              <td className="py-2 pr-4">{stateBadge(m.state)}</td>
              <td className="py-2 pr-4 text-right text-pgp-text-secondary">{m.timeline}</td>
              <td className="py-2 text-right text-pgp-text-secondary">{m.lag}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

function ETCDTable({ members, health }: { members: ETCDMember[]; health: Record<string, boolean> }) {
  return (
    <div>
      <h4 className="mb-2 text-xs font-medium text-pgp-text-muted">ETCD Cluster</h4>
      <table className="w-full text-left text-sm">
        <thead>
          <tr className="border-b border-pgp-border text-xs text-pgp-text-muted">
            <th className="pb-2 pr-4">Name</th>
            <th className="pb-2 pr-4">Client URL</th>
            <th className="pb-2 pr-4">Role</th>
            <th className="pb-2 pr-4">Health</th>
            <th className="pb-2 text-right">DB Size</th>
          </tr>
        </thead>
        <tbody>
          {members.map((m) => {
            const isHealthy = health[m.id] ?? false
            return (
              <tr key={m.id} className="border-b border-pgp-border/50">
                <td className="py-2 pr-4 font-mono text-pgp-text-primary">{m.name}</td>
                <td className="py-2 pr-4 text-pgp-text-secondary">{m.client_url}</td>
                <td className="py-2 pr-4">
                  {m.is_leader ? roleBadge('leader') : roleBadge('follower')}
                </td>
                <td className="py-2 pr-4">
                  <span
                    className={`inline-block h-2 w-2 rounded-full ${isHealthy ? 'bg-green-500' : 'bg-red-500'}`}
                  />
                  <span className="ml-1.5 text-xs text-pgp-text-secondary">
                    {isHealthy ? 'healthy' : 'unhealthy'}
                  </span>
                </td>
                <td className="py-2 text-right text-pgp-text-secondary">
                  {formatBytes(m.db_size)}
                </td>
              </tr>
            )
          })}
        </tbody>
      </table>
    </div>
  )
}

export function ClusterSection({ instanceId }: ClusterSectionProps) {
  const { data, isLoading } = useClusterMetrics(instanceId)

  if (isLoading) {
    return (
      <div className="rounded-lg border border-pgp-border bg-pgp-bg-card p-4">
        <h3 className="mb-3 text-sm font-medium text-pgp-text-secondary">HA Cluster</h3>
        <div className="py-4 text-center text-sm text-pgp-text-muted">Loading...</div>
      </div>
    )
  }

  if (!data?.available) return null

  const hasPatroni = data.patroni && data.patroni.members.length > 0
  const hasETCD = data.etcd && data.etcd.members.length > 0

  if (!hasPatroni && !hasETCD) return null

  return (
    <div className="rounded-lg border border-pgp-border bg-pgp-bg-card p-4">
      <h3 className="mb-3 text-sm font-medium text-pgp-text-secondary">HA Cluster</h3>
      <div className="space-y-4">
        {hasPatroni && <PatroniTable members={data.patroni!.members} />}
        {hasETCD && <ETCDTable members={data.etcd!.members} health={data.etcd!.health} />}
      </div>
    </div>
  )
}
