import type { StepResult } from '@/types/playbook'

interface ResultTableProps {
  result: StepResult
}

export function ResultTable({ result }: ResultTableProps) {
  if (!result.columns || result.columns.length === 0) {
    return (
      <p className="text-xs text-pgp-text-muted">Query returned no columns.</p>
    )
  }

  if (result.rows.length === 0) {
    return (
      <p className="text-xs text-pgp-text-muted">Query returned 0 rows.</p>
    )
  }

  return (
    <div className="space-y-1">
      <div className="overflow-x-auto rounded-md border border-pgp-border">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-pgp-border bg-pgp-bg-secondary">
              {result.columns.map((col) => (
                <th
                  key={col}
                  className="px-3 py-2 text-left text-xs font-medium uppercase tracking-wider text-pgp-text-muted"
                >
                  {col}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {result.rows.map((row, idx) => (
              <tr
                key={idx}
                className="border-b border-pgp-border last:border-b-0"
              >
                {row.map((cell, cidx) => (
                  <td
                    key={cidx}
                    className="whitespace-nowrap px-3 py-2 text-pgp-text-primary"
                  >
                    {cell === null ? (
                      <span className="text-pgp-text-muted">NULL</span>
                    ) : (
                      String(cell)
                    )}
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {result.truncated && (
        <p className="text-xs text-pgp-text-muted">
          Showing {result.rows.length} of {result.row_count} rows
        </p>
      )}
    </div>
  )
}
