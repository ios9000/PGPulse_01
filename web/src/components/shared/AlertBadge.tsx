interface AlertBadgeProps {
  warnings: number
  criticals: number
}

export function AlertBadge({ warnings, criticals }: AlertBadgeProps) {
  if (criticals === 0 && warnings === 0) return null

  if (criticals > 0) {
    return (
      <span className="inline-flex items-center rounded-full bg-red-500/20 px-2 py-0.5 text-xs font-medium text-red-400">
        {criticals} critical
      </span>
    )
  }

  return (
    <span className="inline-flex items-center rounded-full bg-amber-500/20 px-2 py-0.5 text-xs font-medium text-amber-400">
      {warnings} warning
    </span>
  )
}
