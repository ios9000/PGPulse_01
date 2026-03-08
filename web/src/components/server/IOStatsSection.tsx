import { useOSMetrics } from '@/hooks/useOSMetrics'

interface IOStatsSectionProps {
  instanceId: string
}

export function IOStatsSection({ instanceId }: IOStatsSectionProps) {
  const { data, isLoading } = useOSMetrics(instanceId)

  if (isLoading) {
    return (
      <div className="rounded-lg border border-pgp-border bg-pgp-bg-card p-4">
        <h3 className="mb-3 text-sm font-medium text-pgp-text-secondary">I/O Statistics</h3>
        <div className="py-4 text-center text-sm text-pgp-text-muted">Loading...</div>
      </div>
    )
  }

  if (!data?.available || !data.data?.diskstats?.length) return null

  const stats = data.data.diskstats

  return (
    <div className="rounded-lg border border-pgp-border bg-pgp-bg-card p-4">
      <h3 className="mb-3 text-sm font-medium text-pgp-text-secondary">I/O Statistics</h3>
      <div className="overflow-x-auto">
        <table className="w-full text-left text-sm">
          <thead>
            <tr className="border-b border-pgp-border text-xs text-pgp-text-muted">
              <th className="pb-2 pr-4">Device</th>
              <th className="pb-2 pr-4 text-right">Reads</th>
              <th className="pb-2 pr-4 text-right">Writes</th>
              <th className="pb-2 pr-4 text-right">Read KB</th>
              <th className="pb-2 pr-4 text-right">Write KB</th>
              <th className="pb-2 pr-4 text-right">R-Await</th>
              <th className="pb-2 pr-4 text-right">W-Await</th>
              <th className="pb-2 pr-4 text-right">In-Flight</th>
              <th className="pb-2 text-right">Util%</th>
            </tr>
          </thead>
          <tbody>
            {stats.map((s) => {
              const utilColor =
                s.util_pct > 90
                  ? 'text-red-400'
                  : s.util_pct > 70
                    ? 'text-yellow-400'
                    : 'text-pgp-text-primary'
              return (
                <tr key={s.device} className="border-b border-pgp-border/50">
                  <td className="py-2 pr-4 font-mono text-pgp-text-primary">{s.device}</td>
                  <td className="py-2 pr-4 text-right text-pgp-text-secondary">
                    {s.reads_completed.toLocaleString()}
                  </td>
                  <td className="py-2 pr-4 text-right text-pgp-text-secondary">
                    {s.writes_completed.toLocaleString()}
                  </td>
                  <td className="py-2 pr-4 text-right text-pgp-text-secondary">
                    {s.read_kb.toLocaleString()}
                  </td>
                  <td className="py-2 pr-4 text-right text-pgp-text-secondary">
                    {s.write_kb.toLocaleString()}
                  </td>
                  <td className="py-2 pr-4 text-right text-pgp-text-secondary">
                    {s.read_await_ms.toFixed(1)} ms
                  </td>
                  <td className="py-2 pr-4 text-right text-pgp-text-secondary">
                    {s.write_await_ms.toFixed(1)} ms
                  </td>
                  <td className="py-2 pr-4 text-right text-pgp-text-secondary">
                    {s.io_in_progress}
                  </td>
                  <td className={`py-2 text-right font-medium ${utilColor}`}>
                    {s.util_pct.toFixed(1)}%
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
