import { useMemo } from 'react'
import { useReplication } from '@/hooks/useReplication'
import { DataTable, type Column } from '@/components/ui/DataTable'
import { EmptyState } from '@/components/ui/EmptyState'
import { Spinner } from '@/components/ui/Spinner'
import { formatBytes } from '@/lib/formatters'
import type { ReplicaInfo, SlotInfo } from '@/types/models'

interface ReplicationSectionProps {
  instanceId: string
}

type ReplicaRow = ReplicaInfo & Record<string, unknown>
type SlotRow = SlotInfo & Record<string, unknown>

export function ReplicationSection({ instanceId }: ReplicationSectionProps) {
  const { data, isLoading } = useReplication(instanceId)

  const replicaColumns: Column<ReplicaRow>[] = useMemo(() => [
    { key: 'application_name', label: 'Application', sortable: true },
    { key: 'client_addr', label: 'Client', mono: true },
    { key: 'state', label: 'State' },
    { key: 'sync_state', label: 'Sync' },
    {
      key: 'lag',
      label: 'Replay Lag',
      align: 'right' as const,
      mono: true,
      render: (row: ReplicaRow) => formatBytes(row.lag.replay_bytes),
    },
    {
      key: 'write_lag',
      label: 'Write Lag',
      align: 'right' as const,
      mono: true,
      render: (row: ReplicaRow) => row.lag.write_lag ?? '--',
    },
  ], [])

  const slotColumns: Column<SlotRow>[] = useMemo(() => [
    { key: 'slot_name', label: 'Slot Name', sortable: true },
    { key: 'slot_type', label: 'Type' },
    {
      key: 'active',
      label: 'Active',
      render: (row: SlotRow) => (
        <span className={row.active ? 'text-pgp-ok' : 'text-pgp-critical'}>
          {row.active ? 'Yes' : 'No'}
        </span>
      ),
    },
    {
      key: 'wal_retained_bytes',
      label: 'WAL Retained',
      align: 'right' as const,
      mono: true,
      render: (row: SlotRow) => formatBytes(row.wal_retained_bytes),
    },
  ], [])

  if (isLoading) {
    return (
      <div className="rounded-lg border border-pgp-border bg-pgp-bg-card p-4">
        <h2 className="mb-4 text-lg font-semibold text-pgp-text-primary">Replication</h2>
        <div className="flex justify-center py-8"><Spinner size="lg" /></div>
      </div>
    )
  }

  if (!data) {
    return (
      <div className="rounded-lg border border-pgp-border bg-pgp-bg-card p-4">
        <h2 className="mb-4 text-lg font-semibold text-pgp-text-primary">Replication</h2>
        <EmptyState title="No replication data" />
      </div>
    )
  }

  const isPrimary = data.role === 'primary'

  return (
    <div className="rounded-lg border border-pgp-border bg-pgp-bg-card p-4">
      <h2 className="mb-4 text-lg font-semibold text-pgp-text-primary">
        Replication
        <span className="ml-2 text-sm font-normal text-pgp-text-muted">({data.role})</span>
      </h2>

      {isPrimary && data.replicas && data.replicas.length > 0 && (
        <div className="mb-4">
          <h3 className="mb-2 text-sm font-medium text-pgp-text-secondary">Connected Replicas</h3>
          <DataTable columns={replicaColumns} data={data.replicas as ReplicaRow[]} />
        </div>
      )}

      {isPrimary && data.slots && data.slots.length > 0 && (
        <div className="mb-4">
          <h3 className="mb-2 text-sm font-medium text-pgp-text-secondary">Replication Slots</h3>
          <DataTable columns={slotColumns} data={data.slots as SlotRow[]} />
        </div>
      )}

      {!isPrimary && data.wal_receiver && (
        <div className="rounded-lg border border-pgp-border bg-pgp-bg-secondary p-4">
          <h3 className="mb-2 text-sm font-medium text-pgp-text-secondary">WAL Receiver</h3>
          <div className="grid grid-cols-2 gap-2 text-sm">
            <span className="text-pgp-text-muted">Sender:</span>
            <span className="font-mono text-pgp-text-primary">
              {data.wal_receiver.sender_host}:{data.wal_receiver.sender_port}
            </span>
            <span className="text-pgp-text-muted">Status:</span>
            <span className="text-pgp-text-primary">{data.wal_receiver.status}</span>
          </div>
        </div>
      )}

      {isPrimary && (!data.replicas || data.replicas.length === 0) && (!data.slots || data.slots.length === 0) && (
        <EmptyState title="No replication configured" description="This primary has no connected replicas or replication slots." />
      )}

      {!isPrimary && !data.wal_receiver && (
        <EmptyState title="WAL receiver not active" />
      )}
    </div>
  )
}
