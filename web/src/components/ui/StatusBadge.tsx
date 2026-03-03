import { STATUS_BG_COLORS } from '@/lib/constants'

interface StatusBadgeProps {
  status: 'ok' | 'warning' | 'critical' | 'info' | 'unknown'
  label?: string
  pulse?: boolean
  size?: 'sm' | 'md'
}

const dotSize = {
  sm: 'h-2 w-2',
  md: 'h-2.5 w-2.5',
}

export function StatusBadge({ status, label, pulse = false, size = 'md' }: StatusBadgeProps) {
  const bgColor = STATUS_BG_COLORS[status]

  return (
    <span className="inline-flex items-center gap-1.5">
      <span className="relative flex">
        {pulse && status === 'critical' && (
          <span
            className={`absolute inline-flex h-full w-full animate-ping rounded-full ${bgColor} opacity-75`}
          />
        )}
        <span className={`relative inline-flex rounded-full ${dotSize[size]} ${bgColor}`} />
      </span>
      {label && (
        <span className="text-sm text-pgp-text-secondary">{label}</span>
      )}
    </span>
  )
}
