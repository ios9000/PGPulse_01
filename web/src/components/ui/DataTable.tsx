import { type ReactNode } from 'react'
import { ChevronUp, ChevronDown } from 'lucide-react'
import { Spinner } from './Spinner'
import { EmptyState } from './EmptyState'

export interface Column<T> {
  key: keyof T | string
  label: string
  sortable?: boolean
  render?: (row: T) => ReactNode
  width?: string
  align?: 'left' | 'center' | 'right'
  mono?: boolean
}

interface DataTableProps<T> {
  columns: Column<T>[]
  data: T[]
  loading?: boolean
  emptyMessage?: string
  onRowClick?: (row: T) => void
  sortColumn?: string
  sortDirection?: 'asc' | 'desc'
  onSort?: (column: string) => void
}

export function DataTable<T extends Record<string, unknown>>({
  columns,
  data,
  loading = false,
  emptyMessage = 'No data available',
  onRowClick,
  sortColumn,
  sortDirection,
  onSort,
}: DataTableProps<T>) {
  const alignClass = {
    left: 'text-left',
    center: 'text-center',
    right: 'text-right',
  }

  return (
    <div className="relative overflow-x-auto rounded-lg border border-pgp-border">
      {loading && (
        <div className="absolute inset-0 z-10 flex items-center justify-center bg-pgp-bg-primary/50">
          <Spinner size="lg" />
        </div>
      )}
      <table className="w-full text-sm">
        <thead className="sticky top-0 bg-pgp-bg-secondary text-pgp-text-secondary">
          <tr>
            {columns.map((col) => (
              <th
                key={String(col.key)}
                className={`px-4 py-3 font-medium ${alignClass[col.align || 'left']} ${
                  col.sortable ? 'cursor-pointer select-none hover:text-pgp-text-primary' : ''
                }`}
                style={col.width ? { width: col.width } : undefined}
                onClick={() => col.sortable && onSort?.(String(col.key))}
              >
                <span className="inline-flex items-center gap-1">
                  {col.label}
                  {col.sortable && sortColumn === String(col.key) && (
                    sortDirection === 'asc' ? (
                      <ChevronUp className="h-4 w-4" />
                    ) : (
                      <ChevronDown className="h-4 w-4" />
                    )
                  )}
                </span>
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {data.length === 0 && !loading ? (
            <tr>
              <td colSpan={columns.length} className="px-4 py-12">
                <EmptyState title={emptyMessage} />
              </td>
            </tr>
          ) : (
            data.map((row, idx) => (
              <tr
                key={idx}
                className={`border-t border-pgp-border transition-colors hover:bg-pgp-bg-hover ${
                  onRowClick ? 'cursor-pointer' : ''
                }`}
                onClick={() => onRowClick?.(row)}
              >
                {columns.map((col) => (
                  <td
                    key={String(col.key)}
                    className={`px-4 py-3 ${alignClass[col.align || 'left']} ${
                      col.mono ? 'font-mono' : ''
                    }`}
                  >
                    {col.render
                      ? col.render(row)
                      : String(row[col.key as keyof T] ?? '')}
                  </td>
                ))}
              </tr>
            ))
          )}
        </tbody>
      </table>
    </div>
  )
}
