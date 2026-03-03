import type { LockEntry } from '@/types/models'
import { formatDuration } from '@/lib/formatters'

interface LockTreeRowProps {
  lock: LockEntry
}

export function LockTreeRow({ lock }: LockTreeRowProps) {
  const durationColor =
    lock.duration_seconds >= 300
      ? 'text-red-400'
      : lock.duration_seconds >= 60
        ? 'text-yellow-400'
        : ''

  const rootStyle = lock.is_root
    ? 'border-l-4 border-red-500 bg-red-500/5'
    : ''

  const truncatedQuery = lock.query.length > 60
    ? lock.query.substring(0, 60) + '...'
    : lock.query

  return (
    <tr className={`border-t border-pgp-border transition-colors hover:bg-pgp-bg-hover ${rootStyle}`}>
      <td
        className="whitespace-nowrap px-3 py-2 font-mono text-xs"
        style={{ paddingLeft: `${lock.depth * 24 + 12}px` }}
      >
        {lock.pid}
      </td>
      <td className="px-3 py-2 text-xs">{lock.usename}</td>
      <td className="px-3 py-2 text-xs">{lock.datname}</td>
      <td className="px-3 py-2 text-xs">{lock.state}</td>
      <td className="px-3 py-2 text-xs text-pgp-text-muted">
        {lock.wait_event_type && lock.wait_event
          ? `${lock.wait_event_type}:${lock.wait_event}`
          : '-'}
      </td>
      <td className={`px-3 py-2 text-right font-mono text-xs ${durationColor}`}>
        {formatDuration(lock.duration_seconds)}
      </td>
      <td className="px-3 py-2 text-right text-xs">
        {lock.blocking_count > 0 ? (
          <span className="inline-flex items-center rounded-full bg-red-500/20 px-2 py-0.5 text-xs font-medium text-red-400">
            {lock.blocking_count}
          </span>
        ) : (
          <span className="text-pgp-text-muted">0</span>
        )}
      </td>
      <td className="max-w-xs px-3 py-2">
        <span className="block truncate font-mono text-xs" title={lock.query}>
          {truncatedQuery}
        </span>
      </td>
    </tr>
  )
}
