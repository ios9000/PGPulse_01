import { useState, useEffect, useCallback, useRef } from 'react'
import { X, Upload, Eye } from 'lucide-react'
import { useBulkImport } from '@/hooks/useInstanceManagement'
import type { BulkImportResult } from '@/types/models'

interface BulkImportModalProps {
  onClose: () => void
}

interface PreviewRow {
  id?: string
  name: string
  dsn: string
  enabled: string
}

function parseCSV(text: string): PreviewRow[] {
  const lines = text.trim().split('\n')
  if (lines.length < 2) return []

  const rows: PreviewRow[] = []
  for (let i = 1; i < lines.length; i++) {
    const line = lines[i].trim()
    if (!line) continue

    // Simple CSV parsing: handle quoted fields
    const fields: string[] = []
    let current = ''
    let inQuotes = false
    for (const ch of line) {
      if (ch === '"') {
        inQuotes = !inQuotes
      } else if (ch === ',' && !inQuotes) {
        fields.push(current.trim())
        current = ''
      } else {
        current += ch
      }
    }
    fields.push(current.trim())

    rows.push({
      id: fields[0] || undefined,
      name: fields[1] || '',
      dsn: fields[2] || '',
      enabled: fields[3] || 'true',
    })
  }
  return rows
}

export function BulkImportModal({ onClose }: BulkImportModalProps) {
  const [csvText, setCsvText] = useState('')
  const [preview, setPreview] = useState<PreviewRow[] | null>(null)
  const [results, setResults] = useState<BulkImportResult[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  const fileInputRef = useRef<HTMLInputElement>(null)

  const bulkImport = useBulkImport()

  const handleClose = useCallback(() => {
    if (!bulkImport.isPending) onClose()
  }, [bulkImport.isPending, onClose])

  useEffect(() => {
    const handleEsc = (e: KeyboardEvent) => {
      if (e.key === 'Escape') handleClose()
    }
    document.addEventListener('keydown', handleEsc)
    return () => document.removeEventListener('keydown', handleEsc)
  }, [handleClose])

  const handleBackdropClick = (e: React.MouseEvent<HTMLDivElement>) => {
    if (e.target === e.currentTarget) handleClose()
  }

  const handleFileUpload = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    if (!file) return
    const reader = new FileReader()
    reader.onload = (ev) => {
      const text = ev.target?.result as string
      setCsvText(text)
      setPreview(null)
      setResults(null)
    }
    reader.readAsText(file)
  }

  const handlePreview = () => {
    setError(null)
    if (!csvText.trim()) {
      setError('CSV content is empty')
      return
    }
    const rows = parseCSV(csvText)
    if (rows.length === 0) {
      setError('No data rows found. Make sure there is a header row and at least one data row.')
      return
    }
    setPreview(rows)
  }

  const handleImport = async () => {
    setError(null)
    setResults(null)
    try {
      const importResults = await bulkImport.mutateAsync(csvText)
      setResults(importResults)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Import failed')
    }
  }

  const inputClass =
    'mt-1 block w-full rounded-md border border-pgp-border bg-pgp-bg-primary px-3 py-2 text-sm text-pgp-text-primary focus:border-pgp-accent focus:outline-none focus:ring-1 focus:ring-pgp-accent'

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
      onClick={handleBackdropClick}
    >
      <div className="w-full max-w-2xl rounded-lg border border-pgp-border bg-pgp-bg-card p-6">
        <div className="mb-4 flex items-center justify-between">
          <h2 className="text-lg font-semibold text-pgp-text-primary">Bulk Import Instances</h2>
          <button
            onClick={handleClose}
            className="text-pgp-text-muted hover:text-pgp-text-primary"
          >
            <X className="h-5 w-5" />
          </button>
        </div>

        <div className="space-y-4">
          {error && (
            <div className="rounded-md bg-red-500/10 px-3 py-2 text-sm text-red-400">{error}</div>
          )}

          <p className="text-sm text-pgp-text-muted">
            CSV format: <code className="rounded bg-pgp-bg-secondary px-1">id,name,dsn,enabled</code> — header row required, id is optional.
          </p>

          <div className="flex items-center gap-2">
            <button
              onClick={() => fileInputRef.current?.click()}
              className="flex items-center gap-2 rounded-md border border-pgp-border px-3 py-2 text-sm text-pgp-text-secondary hover:bg-pgp-bg-hover"
            >
              <Upload className="h-4 w-4" />
              Upload CSV
            </button>
            <input
              ref={fileInputRef}
              type="file"
              accept=".csv"
              onChange={handleFileUpload}
              className="hidden"
            />
          </div>

          <textarea
            value={csvText}
            onChange={(e) => {
              setCsvText(e.target.value)
              setPreview(null)
              setResults(null)
            }}
            className={`${inputClass} h-32 font-mono text-xs`}
            placeholder={'id,name,dsn,enabled\n,Production,postgres://user:pass@host:5432/db,true'}
          />

          {preview && preview.length > 0 && (
            <div className="max-h-48 overflow-auto rounded-lg border border-pgp-border">
              <table className="w-full text-xs">
                <thead className="bg-pgp-bg-secondary">
                  <tr>
                    <th className="px-3 py-2 text-left text-pgp-text-muted">Row</th>
                    <th className="px-3 py-2 text-left text-pgp-text-muted">ID</th>
                    <th className="px-3 py-2 text-left text-pgp-text-muted">Name</th>
                    <th className="px-3 py-2 text-left text-pgp-text-muted">DSN</th>
                    <th className="px-3 py-2 text-left text-pgp-text-muted">Enabled</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-pgp-border">
                  {preview.map((row, i) => (
                    <tr key={i}>
                      <td className="px-3 py-1.5 text-pgp-text-muted">{i + 1}</td>
                      <td className="px-3 py-1.5 font-mono text-pgp-text-secondary">
                        {row.id || '(auto)'}
                      </td>
                      <td className="px-3 py-1.5 text-pgp-text-primary">{row.name}</td>
                      <td className="max-w-[200px] truncate px-3 py-1.5 font-mono text-pgp-text-secondary">
                        {row.dsn}
                      </td>
                      <td className="px-3 py-1.5 text-pgp-text-secondary">{row.enabled}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}

          {results && (
            <div className="space-y-1">
              <h4 className="text-sm font-medium text-pgp-text-primary">Import Results</h4>
              {results.map((r, i) => (
                <div
                  key={i}
                  className={`rounded px-3 py-1.5 text-xs ${
                    r.success
                      ? 'bg-green-500/10 text-green-400'
                      : 'bg-red-500/10 text-red-400'
                  }`}
                >
                  Row {r.row}: {r.success ? `Imported${r.id ? ` (${r.id})` : ''}` : r.error}
                </div>
              ))}
            </div>
          )}

          <div className="flex justify-end gap-3 pt-2">
            <button
              type="button"
              onClick={handleClose}
              className="rounded-md px-4 py-2 text-sm text-pgp-text-secondary hover:bg-pgp-bg-hover"
            >
              {results ? 'Close' : 'Cancel'}
            </button>
            {!results && (
              <>
                <button
                  type="button"
                  onClick={handlePreview}
                  disabled={!csvText.trim()}
                  className="flex items-center gap-2 rounded-md border border-pgp-border px-3 py-2 text-sm text-pgp-text-secondary hover:bg-pgp-bg-hover disabled:opacity-50"
                >
                  <Eye className="h-4 w-4" />
                  Preview
                </button>
                <button
                  type="button"
                  onClick={handleImport}
                  disabled={bulkImport.isPending || !csvText.trim()}
                  className="rounded-md bg-pgp-accent px-4 py-2 text-sm font-medium text-white hover:bg-pgp-accent-hover disabled:opacity-50"
                >
                  {bulkImport.isPending ? 'Importing...' : 'Import'}
                </button>
              </>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}
