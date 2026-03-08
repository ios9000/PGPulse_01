import { useOSMetrics } from '@/hooks/useOSMetrics'
import type { OSDiskInfo } from '@/types/models'

interface DiskSectionProps {
  instanceId: string
}

function formatBytes(bytes: number): string {
  if (bytes >= 1099511627776) return `${(bytes / 1099511627776).toFixed(1)} TB`
  if (bytes >= 1073741824) return `${(bytes / 1073741824).toFixed(1)} GB`
  if (bytes >= 1048576) return `${(bytes / 1048576).toFixed(0)} MB`
  return `${(bytes / 1024).toFixed(0)} KB`
}

function usagePct(disk: OSDiskInfo): number {
  if (disk.total_bytes === 0) return 0
  return (disk.used_bytes / disk.total_bytes) * 100
}

function inodePct(disk: OSDiskInfo): number {
  if (disk.inodes_total === 0) return 0
  return (disk.inodes_used / disk.inodes_total) * 100
}

function usageColor(pct: number): string {
  if (pct > 90) return 'text-red-400'
  if (pct > 80) return 'text-yellow-400'
  return 'text-pgp-text-primary'
}

function barColor(pct: number): string {
  if (pct > 90) return 'bg-red-500'
  if (pct > 80) return 'bg-yellow-500'
  return 'bg-green-500'
}

function UsageBar({ pct }: { pct: number }) {
  return (
    <div className="flex items-center gap-2">
      <div className="h-2 w-20 rounded-full bg-pgp-bg-secondary">
        <div
          className={`h-full rounded-full ${barColor(pct)}`}
          style={{ width: `${Math.min(pct, 100)}%` }}
        />
      </div>
      <span className={`text-xs ${usageColor(pct)}`}>{pct.toFixed(1)}%</span>
    </div>
  )
}

export function DiskSection({ instanceId }: DiskSectionProps) {
  const { data, isLoading } = useOSMetrics(instanceId)

  if (isLoading) {
    return (
      <div className="rounded-lg border border-pgp-border bg-pgp-bg-card p-4">
        <h3 className="mb-3 text-sm font-medium text-pgp-text-secondary">Disk Usage</h3>
        <div className="py-4 text-center text-sm text-pgp-text-muted">Loading...</div>
      </div>
    )
  }

  if (!data?.available || !data.data?.disks?.length) return null

  const disks = data.data.disks

  return (
    <div className="rounded-lg border border-pgp-border bg-pgp-bg-card p-4">
      <h3 className="mb-3 text-sm font-medium text-pgp-text-secondary">Disk Usage</h3>
      <div className="overflow-x-auto">
        <table className="w-full text-left text-sm">
          <thead>
            <tr className="border-b border-pgp-border text-xs text-pgp-text-muted">
              <th className="pb-2 pr-4">Mount</th>
              <th className="pb-2 pr-4">Device</th>
              <th className="pb-2 pr-4">FS</th>
              <th className="pb-2 pr-4 text-right">Used</th>
              <th className="pb-2 pr-4 text-right">Total</th>
              <th className="pb-2 pr-4">Space</th>
              <th className="pb-2">Inodes</th>
            </tr>
          </thead>
          <tbody>
            {disks.map((disk) => {
              const pct = usagePct(disk)
              const ipct = inodePct(disk)
              return (
                <tr key={disk.mount} className="border-b border-pgp-border/50">
                  <td className="py-2 pr-4 font-mono text-pgp-text-primary">{disk.mount}</td>
                  <td className="py-2 pr-4 text-pgp-text-secondary">{disk.device}</td>
                  <td className="py-2 pr-4 text-pgp-text-muted">{disk.fstype}</td>
                  <td className={`py-2 pr-4 text-right ${usageColor(pct)}`}>
                    {formatBytes(disk.used_bytes)}
                  </td>
                  <td className="py-2 pr-4 text-right text-pgp-text-secondary">
                    {formatBytes(disk.total_bytes)}
                  </td>
                  <td className="py-2 pr-4">
                    <UsageBar pct={pct} />
                  </td>
                  <td className="py-2">
                    <UsageBar pct={ipct} />
                  </td>
                </tr>
              )
            })}
          </tbody>
        </table>
      </div>
    </div>
  )
}
