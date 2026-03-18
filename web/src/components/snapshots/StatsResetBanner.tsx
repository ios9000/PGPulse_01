import { AlertTriangle } from 'lucide-react'

export function StatsResetBanner() {
  return (
    <div className="flex items-center gap-2 rounded-md border border-amber-500/30 bg-amber-500/10 px-4 py-3 text-sm text-amber-400">
      <AlertTriangle className="h-4 w-4 shrink-0" />
      <span>
        <strong>pg_stat_statements was reset</strong> between these snapshots. Delta values may be
        inaccurate.
      </span>
    </div>
  )
}
