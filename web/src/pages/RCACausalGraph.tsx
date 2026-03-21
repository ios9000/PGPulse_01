import { PageHeader } from '@/components/ui/PageHeader'
import { CausalGraphView } from '@/components/rca/CausalGraphView'

export function RCACausalGraph() {
  return (
    <div className="mx-auto max-w-7xl">
      <PageHeader
        title="RCA Causal Knowledge Graph"
        subtitle="Visual representation of known causal relationships between metrics, symptoms, and root causes"
      />

      <div className="rounded-lg border border-pgp-border bg-pgp-bg-card p-4">
        <CausalGraphView />
      </div>
    </div>
  )
}
