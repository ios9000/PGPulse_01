import { useState, useEffect, useCallback } from 'react'
import { X, Plug } from 'lucide-react'
import { useCreateInstance, useUpdateInstance, useTestConnection } from '@/hooks/useInstanceManagement'
import type { ManagedInstance, TestConnectionResult } from '@/types/models'

interface InstanceFormProps {
  onClose: () => void
  instance?: ManagedInstance
}

export function InstanceForm({ onClose, instance }: InstanceFormProps) {
  const isEdit = !!instance

  const [name, setName] = useState(instance?.name ?? '')
  const [dsn, setDsn] = useState('')
  const [maxConns, setMaxConns] = useState(String(instance?.max_conns ?? 5))
  const [enabled, setEnabled] = useState(instance?.enabled ?? true)
  const [error, setError] = useState<string | null>(null)
  const [testResult, setTestResult] = useState<TestConnectionResult | null>(null)

  const createInstance = useCreateInstance()
  const updateInstance = useUpdateInstance()
  const testConnection = useTestConnection()

  const isSaving = createInstance.isPending || updateInstance.isPending

  const handleClose = useCallback(() => {
    if (!isSaving) onClose()
  }, [isSaving, onClose])

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

  const handleTestConnection = async () => {
    if (!instance) return
    setTestResult(null)
    try {
      const result = await testConnection.mutateAsync(instance.id)
      setTestResult(result)
    } catch {
      setTestResult({ success: false, error: 'Failed to test connection' })
    }
  }

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError(null)

    if (!name.trim()) {
      setError('Name is required')
      return
    }
    if (!isEdit && !dsn.trim()) {
      setError('DSN is required')
      return
    }

    try {
      if (isEdit) {
        await updateInstance.mutateAsync({
          id: instance.id,
          name,
          ...(dsn.trim() ? { dsn } : {}),
          enabled,
          max_conns: parseInt(maxConns, 10) || 5,
        })
      } else {
        await createInstance.mutateAsync({
          name,
          dsn,
          enabled,
          max_conns: parseInt(maxConns, 10) || 5,
        })
      }
      onClose()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save instance')
    }
  }

  const inputClass =
    'mt-1 block w-full rounded-md border border-pgp-border bg-pgp-bg-primary px-3 py-2 text-sm text-pgp-text-primary focus:border-pgp-accent focus:outline-none focus:ring-1 focus:ring-pgp-accent'
  const labelClass = 'block text-sm font-medium text-pgp-text-secondary'

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
      onClick={handleBackdropClick}
    >
      <div className="w-full max-w-lg rounded-lg border border-pgp-border bg-pgp-bg-card p-6">
        <div className="mb-4 flex items-center justify-between">
          <h2 className="text-lg font-semibold text-pgp-text-primary">
            {isEdit ? 'Edit Instance' : 'Add Instance'}
          </h2>
          <button
            onClick={handleClose}
            className="text-pgp-text-muted hover:text-pgp-text-primary"
          >
            <X className="h-5 w-5" />
          </button>
        </div>

        <form onSubmit={handleSubmit} className="space-y-4">
          {error && (
            <div className="rounded-md bg-red-500/10 px-3 py-2 text-sm text-red-400">{error}</div>
          )}

          <div>
            <label className={labelClass}>Name</label>
            <input
              type="text"
              value={name}
              onChange={(e) => setName(e.target.value)}
              className={inputClass}
              placeholder="Production Primary"
              required
            />
          </div>

          <div>
            <label className={labelClass}>
              DSN {isEdit && <span className="text-pgp-text-muted">(leave empty to keep current)</span>}
            </label>
            <input
              type="text"
              value={dsn}
              onChange={(e) => setDsn(e.target.value)}
              className={`${inputClass} font-mono`}
              placeholder="postgres://user:pass@host:5432/dbname?sslmode=require"
              required={!isEdit}
            />
            {isEdit && instance.dsn_masked && (
              <p className="mt-1 text-xs text-pgp-text-muted">
                Current: <span className="font-mono">{instance.dsn_masked}</span>
              </p>
            )}
          </div>

          <div>
            <label className={labelClass}>Max Connections</label>
            <input
              type="number"
              value={maxConns}
              onChange={(e) => setMaxConns(e.target.value)}
              className={inputClass}
              min="1"
              max="20"
            />
          </div>

          <div>
            <label className={labelClass}>Enabled</label>
            <button
              type="button"
              onClick={() => setEnabled(!enabled)}
              className="relative mt-1 inline-flex h-5 w-9 items-center rounded-full transition-colors focus:outline-none focus:ring-2 focus:ring-blue-500"
              role="switch"
              aria-checked={enabled}
            >
              <span
                className={`absolute inset-0 rounded-full transition-colors ${
                  enabled ? 'bg-blue-600' : 'bg-gray-600'
                }`}
              />
              <span
                className={`relative inline-block h-3.5 w-3.5 rounded-full bg-white transition-transform ${
                  enabled ? 'translate-x-4.5' : 'translate-x-1'
                }`}
              />
            </button>
          </div>

          {isEdit && (
            <div>
              <button
                type="button"
                onClick={handleTestConnection}
                disabled={testConnection.isPending}
                className="flex items-center gap-2 rounded-md border border-pgp-border px-3 py-2 text-sm text-pgp-text-secondary hover:bg-pgp-bg-hover disabled:opacity-50"
              >
                <Plug className="h-4 w-4" />
                {testConnection.isPending ? 'Testing...' : 'Test Connection'}
              </button>
              {testResult && (
                <div className="mt-2">
                  {testResult.success ? (
                    <p className="text-sm text-green-400">
                      Connected — {testResult.version} ({testResult.latency_ms}ms)
                    </p>
                  ) : (
                    <p className="text-sm text-red-400">{testResult.error}</p>
                  )}
                </div>
              )}
            </div>
          )}

          <div className="flex justify-end gap-3 pt-2">
            <button
              type="button"
              onClick={handleClose}
              className="rounded-md px-4 py-2 text-sm text-pgp-text-secondary hover:bg-pgp-bg-hover"
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={isSaving}
              className="rounded-md bg-pgp-accent px-4 py-2 text-sm font-medium text-white hover:bg-pgp-accent-hover disabled:opacity-50"
            >
              {isSaving ? 'Saving...' : isEdit ? 'Save Changes' : 'Add Instance'}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}
