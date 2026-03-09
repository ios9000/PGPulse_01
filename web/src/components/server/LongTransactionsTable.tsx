import { useMemo, useCallback } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useLongTransactions } from '@/hooks/useActivity'
import { DataTable, type Column } from '@/components/ui/DataTable'
import { Spinner } from '@/components/ui/Spinner'
import { SessionActions } from '@/components/SessionActions'
import { formatDuration } from '@/lib/formatters'
import type { LongTransaction } from '@/types/models'

interface LongTransactionsTableProps {
  instanceId: string
}

type TxnRow = LongTransaction & Record<string, unknown>

export function LongTransactionsTable({ instanceId }: LongTransactionsTableProps) {
  const { data, isLoading } = useLongTransactions(instanceId)
  const queryClient = useQueryClient()

  const handleRefresh = useCallback(() => {
    queryClient.invalidateQueries({ queryKey: ['activity', 'long-transactions', instanceId] })
  }, [queryClient, instanceId])

  const columns: Column<TxnRow>[] = useMemo(() => [
    { key: 'pid', label: 'PID', mono: true, width: '80px' },
    { key: 'username', label: 'User' },
    { key: 'database', label: 'Database' },
    { key: 'state', label: 'State' },
    {
      key: 'duration_seconds',
      label: 'Duration',
      align: 'right' as const,
      mono: true,
      render: (row: TxnRow) => {
        const dur = row.duration_seconds
        const color = dur >= 300 ? 'text-pgp-critical' : dur >= 60 ? 'text-pgp-warning' : ''
        return <span className={color}>{formatDuration(dur)}</span>
      },
    },
    {
      key: 'query',
      label: 'Query',
      render: (row: TxnRow) => (
        <span
          className="block max-w-xs truncate font-mono text-xs"
          title={row.query}
        >
          {row.query}
        </span>
      ),
    },
    {
      key: '_actions',
      label: '',
      width: '120px',
      render: (row: TxnRow) => (
        <SessionActions
          instanceId={instanceId}
          pid={row.pid}
          applicationName={row.application_name ?? ''}
          onRefresh={handleRefresh}
        />
      ),
    },
  ], [instanceId, handleRefresh])

  return (
    <div className="rounded-lg border border-pgp-border bg-pgp-bg-card p-4">
      <h2 className="mb-4 text-lg font-semibold text-pgp-text-primary">Long-Running Transactions</h2>
      {isLoading ? (
        <div className="flex justify-center py-8"><Spinner size="lg" /></div>
      ) : (
        <DataTable
          columns={columns}
          data={(data?.transactions ?? []) as TxnRow[]}
          emptyMessage="No long-running transactions"
        />
      )}
    </div>
  )
}
