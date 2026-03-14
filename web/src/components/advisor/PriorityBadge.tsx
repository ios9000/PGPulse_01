import type { RecommendationPriority } from '@/types/models'

interface PriorityBadgeProps {
  priority: RecommendationPriority
}

const STYLES: Record<RecommendationPriority, { bg: string; label: string }> = {
  action_required: {
    bg: 'bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-200',
    label: 'Action Required',
  },
  suggestion: {
    bg: 'bg-amber-100 text-amber-800 dark:bg-amber-900 dark:text-amber-200',
    label: 'Suggestion',
  },
  info: {
    bg: 'bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200',
    label: 'Info',
  },
}

export function PriorityBadge({ priority }: PriorityBadgeProps) {
  const style = STYLES[priority] ?? STYLES.info
  return (
    <span className={`inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium ${style.bg}`}>
      {style.label}
    </span>
  )
}
