import { ConfidenceBadge } from '@/components/rca/ConfidenceBadge'

interface ChainSummaryCardProps {
  summary: string
  confidence: number
  bucket: string
}

function bucketBorderColor(bucket: string): string {
  if (bucket === 'high') return 'border-l-green-500'
  if (bucket === 'medium') return 'border-l-amber-500'
  return 'border-l-red-500'
}

export function ChainSummaryCard({ summary, confidence, bucket }: ChainSummaryCardProps) {
  return (
    <div
      className={`rounded-lg border border-pgp-border border-l-4 bg-pgp-bg-card p-4 ${bucketBorderColor(bucket)}`}
    >
      <div className="flex items-start justify-between gap-4">
        <p className="text-sm leading-relaxed text-pgp-text-primary">{summary}</p>
        <ConfidenceBadge bucket={bucket} score={confidence} />
      </div>
    </div>
  )
}
