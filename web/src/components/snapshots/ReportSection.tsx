import { useState } from 'react'
import { ChevronDown, ChevronRight } from 'lucide-react'
import type { PGSSDiffEntry } from '@/types/models'
import { DiffTable } from './DiffTable'

interface ReportSectionProps {
  title: string
  entries: PGSSDiffEntry[]
  defaultExpanded?: boolean
}

export function ReportSection({ title, entries, defaultExpanded = false }: ReportSectionProps) {
  const [expanded, setExpanded] = useState(defaultExpanded)

  return (
    <div className="report-section overflow-hidden rounded-lg border border-pgp-border bg-pgp-bg-secondary shadow-sm">
      <button
        onClick={() => setExpanded(!expanded)}
        className="flex w-full items-center justify-between px-5 py-4 text-left transition-colors hover:bg-pgp-bg-hover"
      >
        <div className="flex items-center gap-2.5">
          {expanded ? (
            <ChevronDown className="h-4 w-4 text-pgp-text-muted" />
          ) : (
            <ChevronRight className="h-4 w-4 text-pgp-text-muted" />
          )}
          <span className="text-base font-semibold text-pgp-text-primary">{title}</span>
        </div>
        <span className="rounded-full bg-pgp-accent/10 px-2.5 py-0.5 text-xs font-semibold tabular-nums text-pgp-accent">
          {entries.length}
        </span>
      </button>
      {expanded && (
        <div className="border-t border-pgp-border px-5 py-4">
          <DiffTable entries={entries} compact pageSize={10} />
        </div>
      )}
    </div>
  )
}
