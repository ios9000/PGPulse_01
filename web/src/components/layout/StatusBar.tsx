import { useHealthCheck } from '@/hooks/useHealthCheck'

export function StatusBar() {
  const { isConnected, dataUpdatedAt } = useHealthCheck()

  const secondsAgo = dataUpdatedAt
    ? Math.round((Date.now() - dataUpdatedAt) / 1000)
    : null

  return (
    <footer className="fixed bottom-0 right-0 z-20 flex h-8 items-center justify-between border-t border-pgp-border bg-pgp-bg-secondary px-4 text-xs text-pgp-text-muted"
      style={{ left: 'inherit', width: 'calc(100%)' }}
    >
      <div className="flex items-center gap-2">
        <span
          className={`inline-block h-2 w-2 rounded-full ${
            isConnected ? 'bg-pgp-ok' : 'bg-pgp-critical'
          }`}
        />
        <span>{isConnected ? 'Connected' : 'Disconnected'}</span>
      </div>
      {secondsAgo !== null && (
        <span>Last refresh: {secondsAgo}s ago</span>
      )}
    </footer>
  )
}
