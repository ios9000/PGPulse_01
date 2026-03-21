interface ConfidenceBadgeProps {
  bucket: string
  score: number
}

function bucketColor(bucket: string): string {
  if (bucket === 'high') return 'bg-green-500/20 text-green-400'
  if (bucket === 'medium') return 'bg-amber-500/20 text-amber-400'
  return 'bg-red-500/20 text-red-400'
}

export function ConfidenceBadge({ bucket, score }: ConfidenceBadgeProps) {
  const label = bucket.charAt(0).toUpperCase() + bucket.slice(1)
  const pct = Math.round(score * 100)

  return (
    <span
      className={`inline-flex items-center rounded-full px-2 py-0.5 text-xs font-semibold ${bucketColor(bucket)}`}
    >
      {label} {pct}%
    </span>
  )
}
