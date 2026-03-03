import { useParams } from 'react-router-dom'
import { PageHeader } from '@/components/ui/PageHeader'

export function ServerDetail() {
  const { serverId } = useParams()

  return (
    <div>
      <PageHeader title={`Server: ${serverId}`} subtitle="Server detail view coming in M5_03" />
      <div className="rounded-lg border border-pgp-border bg-pgp-bg-card p-8 text-center text-pgp-text-muted">
        Server detail view for <span className="font-mono text-pgp-text-secondary">{serverId}</span> coming in M5_03
      </div>
    </div>
  )
}
