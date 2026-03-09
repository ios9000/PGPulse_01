import { useParams } from 'react-router-dom'
import { PageHeader } from '@/components/ui/PageHeader'
import { Spinner } from '@/components/ui/Spinner'
import { useDatabaseMetrics } from '@/hooks/useDatabaseMetrics'
import { formatBytes } from '@/lib/formatters'
import type { DatabaseMetrics, TableMetric, IndexMetric, VacuumMetric, SchemaMetric, SequenceMetric, FunctionMetric } from '@/types/models'

function formatRelativeTime(isoString: string): string {
  const diffSec = Math.floor((Date.now() - new Date(isoString).getTime()) / 1000)
  if (diffSec < 60) return `${diffSec}s ago`
  if (diffSec < 3600) return `${Math.floor(diffSec / 60)} min ago`
  if (diffSec < 86400) return `${Math.floor(diffSec / 3600)} hr ago`
  return `${Math.floor(diffSec / 86400)} day ago`
}

function formatAge(seconds: number | undefined): string {
  if (seconds === undefined || seconds === null) return 'n/a'
  if (seconds < 60) return `${Math.floor(seconds)}s`
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m`
  if (seconds < 86400) return `${Math.floor(seconds / 3600)}h`
  return `${Math.floor(seconds / 86400)}d`
}

function ageColor(seconds: number | undefined): string {
  if (seconds === undefined || seconds === null) return ''
  if (seconds > 259200) return 'text-red-400'
  if (seconds > 86400) return 'text-yellow-400'
  return ''
}

function bloatBadge(ratio: number | undefined) {
  if (ratio === undefined || ratio === null) return null
  const label = `${ratio.toFixed(1)}x`
  if (ratio > 10) return <span className="rounded bg-red-500/20 px-1.5 py-0.5 text-xs font-medium text-red-400">{label}</span>
  if (ratio >= 2) return <span className="rounded bg-yellow-500/20 px-1.5 py-0.5 text-xs font-medium text-yellow-400">{label}</span>
  return <span className="rounded bg-pgp-bg-hover px-1.5 py-0.5 text-xs text-pgp-text-muted">{label}</span>
}

function Card({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="rounded-lg border border-pgp-border bg-pgp-bg-card p-4">
      <h3 className="mb-3 text-sm font-medium text-pgp-text-secondary">{title}</h3>
      {children}
    </div>
  )
}

function TablesSection({ tables }: { tables: TableMetric[] }) {
  const sorted = [...tables].sort((a, b) => b.total_bytes - a.total_bytes)
  if (sorted.length === 0) return null
  return (
    <Card title="Tables">
      <div className="overflow-x-auto">
        <table className="w-full text-left text-sm">
          <thead>
            <tr className="border-b border-pgp-border text-pgp-text-muted">
              <th className="pb-2 pr-4">Schema</th>
              <th className="pb-2 pr-4">Table</th>
              <th className="pb-2 pr-4 text-right">Size</th>
              <th className="pb-2 pr-4 text-right">Bloat</th>
              <th className="pb-2 text-right">Wasted</th>
            </tr>
          </thead>
          <tbody>
            {sorted.map((t) => (
              <tr key={`${t.schema}.${t.table}`} className="border-b border-pgp-border/50">
                <td className="py-1.5 pr-4 text-pgp-text-muted">{t.schema}</td>
                <td className="py-1.5 pr-4 font-mono text-pgp-text-primary">{t.table}</td>
                <td className="py-1.5 pr-4 text-right text-pgp-text-secondary">{formatBytes(t.total_bytes)}</td>
                <td className="py-1.5 pr-4 text-right">{bloatBadge(t.bloat_ratio)}</td>
                <td className="py-1.5 text-right text-pgp-text-secondary">{t.wasted_bytes != null ? formatBytes(t.wasted_bytes) : ''}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </Card>
  )
}

function VacuumSection({ vacuum }: { vacuum: VacuumMetric[] }) {
  const sorted = [...vacuum].sort((a, b) => b.dead_tuples - a.dead_tuples)
  if (sorted.length === 0) return null
  return (
    <Card title="Vacuum Health">
      <div className="overflow-x-auto">
        <table className="w-full text-left text-sm">
          <thead>
            <tr className="border-b border-pgp-border text-pgp-text-muted">
              <th className="pb-2 pr-4">Schema</th>
              <th className="pb-2 pr-4">Table</th>
              <th className="pb-2 pr-4 text-right">Dead Tuples</th>
              <th className="pb-2 pr-4 text-right">Dead%</th>
              <th className="pb-2 pr-4 text-right">Last Autovacuum</th>
              <th className="pb-2 text-right">Last Analyze</th>
            </tr>
          </thead>
          <tbody>
            {sorted.map((v) => {
              const rowColor = v.dead_pct > 40 ? 'text-red-400' : v.dead_pct > 20 ? 'text-yellow-400' : ''
              return (
                <tr key={`${v.schema}.${v.table}`} className={`border-b border-pgp-border/50 ${rowColor}`}>
                  <td className="py-1.5 pr-4 text-pgp-text-muted">{v.schema}</td>
                  <td className="py-1.5 pr-4 font-mono text-pgp-text-primary">{v.table}</td>
                  <td className="py-1.5 pr-4 text-right">{v.dead_tuples.toLocaleString()}</td>
                  <td className="py-1.5 pr-4 text-right">{v.dead_pct.toFixed(1)}%</td>
                  <td className={`py-1.5 pr-4 text-right ${ageColor(v.autovacuum_age_sec)}`}>{formatAge(v.autovacuum_age_sec)}</td>
                  <td className={`py-1.5 text-right ${ageColor(v.autoanalyze_age_sec)}`}>{formatAge(v.autoanalyze_age_sec)}</td>
                </tr>
              )
            })}
          </tbody>
        </table>
      </div>
    </Card>
  )
}

function IndexesSection({ indexes }: { indexes: IndexMetric[] }) {
  const unused = indexes.filter((i) => i.unused)
  const used = indexes.filter((i) => !i.unused)
  return (
    <Card title="Indexes">
      <div className="space-y-4">
        <div>
          <h4 className="mb-2 flex items-center gap-2 text-xs font-medium text-pgp-text-secondary">
            Unused Indexes
            {unused.length > 0 && (
              <span className="rounded bg-red-500/20 px-1.5 py-0.5 text-xs font-medium text-red-400">
                {unused.length}
              </span>
            )}
          </h4>
          {unused.length === 0 ? (
            <p className="text-xs text-green-400">No unused indexes</p>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-left text-sm">
                <thead>
                  <tr className="border-b border-pgp-border text-pgp-text-muted">
                    <th className="pb-2 pr-4">Schema</th>
                    <th className="pb-2 pr-4">Table</th>
                    <th className="pb-2 pr-4">Index</th>
                    <th className="pb-2 text-right">Size</th>
                  </tr>
                </thead>
                <tbody>
                  {unused.map((idx) => (
                    <tr key={`${idx.schema}.${idx.index}`} className="border-b border-pgp-border/50">
                      <td className="py-1.5 pr-4 text-pgp-text-muted">{idx.schema}</td>
                      <td className="py-1.5 pr-4 text-pgp-text-secondary">{idx.table}</td>
                      <td className="py-1.5 pr-4 font-mono text-pgp-text-primary">{idx.index}</td>
                      <td className="py-1.5 text-right text-pgp-text-secondary">{idx.unused_bytes != null ? formatBytes(idx.unused_bytes) : ''}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>

        <div>
          <h4 className="mb-2 text-xs font-medium text-pgp-text-secondary">Index Usage</h4>
          {used.length === 0 ? (
            <p className="text-xs text-pgp-text-muted">No index usage data</p>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-left text-sm">
                <thead>
                  <tr className="border-b border-pgp-border text-pgp-text-muted">
                    <th className="pb-2 pr-4">Schema</th>
                    <th className="pb-2 pr-4">Table</th>
                    <th className="pb-2 pr-4">Index</th>
                    <th className="pb-2 pr-4 text-right">Scans</th>
                    <th className="pb-2 text-right">Cache Hit%</th>
                  </tr>
                </thead>
                <tbody>
                  {used.map((idx) => (
                    <tr key={`${idx.schema}.${idx.index}`} className="border-b border-pgp-border/50">
                      <td className="py-1.5 pr-4 text-pgp-text-muted">{idx.schema}</td>
                      <td className="py-1.5 pr-4 text-pgp-text-secondary">{idx.table}</td>
                      <td className="py-1.5 pr-4 font-mono text-pgp-text-primary">{idx.index}</td>
                      <td className="py-1.5 pr-4 text-right text-pgp-text-secondary">{idx.scan_count.toLocaleString()}</td>
                      <td className="py-1.5 text-right text-pgp-text-secondary">{idx.cache_hit_pct != null ? `${idx.cache_hit_pct.toFixed(1)}%` : ''}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      </div>
    </Card>
  )
}

function SchemaSizesSection({ schemas }: { schemas: SchemaMetric[] }) {
  const sorted = [...schemas].sort((a, b) => b.size_bytes - a.size_bytes)
  if (sorted.length === 0) return null
  const maxSize = sorted[0]?.size_bytes || 1
  return (
    <Card title="Schema Sizes">
      <div className="space-y-2">
        {sorted.map((s) => {
          const pct = (s.size_bytes / maxSize) * 100
          return (
            <div key={s.schema} className="flex items-center gap-3">
              <span className="w-28 shrink-0 truncate text-sm text-pgp-text-primary">{s.schema}</span>
              <div className="h-5 flex-1 rounded bg-pgp-bg-hover">
                <div
                  className="h-full rounded bg-pgp-accent/70"
                  style={{ width: `${Math.max(pct, 1)}%` }}
                />
              </div>
              <span className="w-20 shrink-0 text-right text-xs text-pgp-text-secondary">{formatBytes(s.size_bytes)}</span>
            </div>
          )
        })}
      </div>
    </Card>
  )
}

function SequencesSection({ sequences }: { sequences: SequenceMetric[] }) {
  if (sequences.length === 0) return null
  return (
    <Card title="Sequences">
      <div className="overflow-x-auto">
        <table className="w-full text-left text-sm">
          <thead>
            <tr className="border-b border-pgp-border text-pgp-text-muted">
              <th className="pb-2 pr-4">Schema</th>
              <th className="pb-2 pr-4">Sequence</th>
              <th className="pb-2 text-right">Last Value</th>
            </tr>
          </thead>
          <tbody>
            {sequences.map((s) => (
              <tr key={`${s.schema}.${s.sequence}`} className="border-b border-pgp-border/50">
                <td className="py-1.5 pr-4 text-pgp-text-muted">{s.schema}</td>
                <td className="py-1.5 pr-4 font-mono text-pgp-text-primary">{s.sequence}</td>
                <td className="py-1.5 text-right text-pgp-text-secondary">{s.last_value.toLocaleString()}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </Card>
  )
}

function FunctionsSection({ functions }: { functions: FunctionMetric[] }) {
  if (functions.length === 0) return null
  return (
    <Card title="Functions">
      <div className="overflow-x-auto">
        <table className="w-full text-left text-sm">
          <thead>
            <tr className="border-b border-pgp-border text-pgp-text-muted">
              <th className="pb-2 pr-4">Schema</th>
              <th className="pb-2 pr-4">Function</th>
              <th className="pb-2 pr-4 text-right">Calls</th>
              <th className="pb-2 text-right">Total Time (ms)</th>
            </tr>
          </thead>
          <tbody>
            {functions.map((f) => (
              <tr key={`${f.schema}.${f.function}`} className="border-b border-pgp-border/50">
                <td className="py-1.5 pr-4 text-pgp-text-muted">{f.schema}</td>
                <td className="py-1.5 pr-4 font-mono text-pgp-text-primary">{f.function}</td>
                <td className="py-1.5 pr-4 text-right text-pgp-text-secondary">{f.calls.toLocaleString()}</td>
                <td className="py-1.5 text-right text-pgp-text-secondary">{f.total_time_ms.toFixed(1)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </Card>
  )
}

function MetricsContent({ data }: { data: DatabaseMetrics }) {
  return (
    <div className="space-y-6">
      <TablesSection tables={data.tables} />

      <VacuumSection vacuum={data.vacuum} />

      <IndexesSection indexes={data.indexes} />

      <SchemaSizesSection schemas={data.schemas} />

      {/* Large Objects */}
      {data.large_object_count > 0 ? (
        <div className="rounded-lg border border-yellow-500/30 bg-yellow-500/10 p-4">
          <h3 className="text-sm font-medium text-yellow-400">Large Objects</h3>
          <p className="mt-1 text-sm text-pgp-text-secondary">
            {data.large_object_count.toLocaleString()} large object{data.large_object_count !== 1 ? 's' : ''}{' '}
            ({formatBytes(data.large_object_size_bytes)})
          </p>
        </div>
      ) : (
        <p className="text-xs text-green-400">No large objects</p>
      )}

      {/* Unlogged Objects */}
      {data.unlogged_count > 0 ? (
        <div className="rounded-lg border border-red-500/30 bg-red-500/10 p-4">
          <h3 className="text-sm font-medium text-red-400">Unlogged Objects</h3>
          <p className="mt-1 text-sm text-pgp-text-secondary">
            {data.unlogged_count.toLocaleString()} unlogged table{data.unlogged_count !== 1 ? 's' : ''} — data will be lost on crash recovery
          </p>
        </div>
      ) : (
        <p className="text-xs text-green-400">No unlogged objects</p>
      )}

      <SequencesSection sequences={data.sequences} />

      <FunctionsSection functions={data.functions} />
    </div>
  )
}

export function DatabaseDetail() {
  const { serverId, dbName } = useParams()
  const { data, isLoading, error, refetch } = useDatabaseMetrics(serverId, dbName)

  const subtitle = [
    `Instance: ${serverId}`,
    data?.collected_at ? `Last collected: ${formatRelativeTime(data.collected_at)}` : '',
  ].filter(Boolean).join(' | ')

  return (
    <div>
      <PageHeader
        title={`Database: ${dbName}`}
        subtitle={subtitle}
        actions={
          <button
            onClick={() => refetch()}
            className="rounded border border-pgp-border bg-pgp-bg-card px-3 py-1.5 text-sm text-pgp-text-secondary hover:bg-pgp-bg-hover"
          >
            Refresh
          </button>
        }
      />

      {isLoading && (
        <div className="flex justify-center py-12">
          <Spinner size="lg" />
        </div>
      )}

      {!isLoading && error && (
        <div className="rounded-lg border border-pgp-border bg-pgp-bg-card p-8 text-center text-pgp-text-muted">
          Failed to load database metrics.
        </div>
      )}

      {!isLoading && !error && !data && (
        <div className="rounded-lg border border-pgp-border bg-pgp-bg-card p-8 text-center text-pgp-text-muted">
          Per-database analysis not yet collected. Data refreshes every 5 minutes.
        </div>
      )}

      {!isLoading && data && <MetricsContent data={data} />}
    </div>
  )
}
