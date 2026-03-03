import { useParams } from 'react-router-dom'
import { PageHeader } from '@/components/ui/PageHeader'

export function DatabaseDetail() {
  const { serverId, dbName } = useParams()

  return (
    <div>
      <PageHeader
        title={`Database: ${dbName}`}
        subtitle={`on ${serverId} — Database detail view coming in M5_04`}
      />
      <div className="rounded-lg border border-pgp-border bg-pgp-bg-card p-8 text-center text-pgp-text-muted">
        Database detail view for <span className="font-mono text-pgp-text-secondary">{dbName}</span> on{' '}
        <span className="font-mono text-pgp-text-secondary">{serverId}</span> coming in M5_04
      </div>
    </div>
  )
}
