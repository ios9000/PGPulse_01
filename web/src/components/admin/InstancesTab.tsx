import { useState } from 'react'
import { Plus, Upload, Pencil, Trash2 } from 'lucide-react'
import { useManagedInstances, useUpdateInstance } from '@/hooks/useInstanceManagement'
import { InstanceForm } from '@/components/admin/InstanceForm'
import { DeleteInstanceModal } from '@/components/admin/DeleteInstanceModal'
import { BulkImportModal } from '@/components/admin/BulkImportModal'
import type { ManagedInstance } from '@/types/models'

const SOURCE_COLORS: Record<string, string> = {
  yaml: 'bg-blue-500/20 text-blue-400',
  manual: 'bg-green-500/20 text-green-400',
}

export function InstancesTab() {
  const { data: instances, isLoading } = useManagedInstances()
  const updateInstance = useUpdateInstance()

  const [showCreateModal, setShowCreateModal] = useState(false)
  const [editInstance, setEditInstance] = useState<ManagedInstance | null>(null)
  const [deleteInstance, setDeleteInstance] = useState<ManagedInstance | null>(null)
  const [showBulkImport, setShowBulkImport] = useState(false)

  const handleToggleEnabled = (inst: ManagedInstance) => {
    updateInstance.mutate({ id: inst.id, enabled: !inst.enabled })
  }

  if (isLoading) {
    return <div className="py-8 text-center text-pgp-text-muted">Loading instances...</div>
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h3 className="text-lg font-medium text-pgp-text-primary">Instances</h3>
        <div className="flex gap-2">
          <button
            onClick={() => setShowBulkImport(true)}
            className="flex items-center gap-2 rounded-md border border-pgp-border px-3 py-2 text-sm text-pgp-text-secondary hover:bg-pgp-bg-hover"
          >
            <Upload className="h-4 w-4" />
            Bulk Import
          </button>
          <button
            onClick={() => setShowCreateModal(true)}
            className="flex items-center gap-2 rounded-md bg-pgp-accent px-4 py-2 text-sm font-medium text-white hover:bg-pgp-accent-hover"
          >
            <Plus className="h-4 w-4" />
            Add Instance
          </button>
        </div>
      </div>

      {(!instances || instances.length === 0) ? (
        <div className="rounded-lg border border-pgp-border bg-pgp-bg-card p-8 text-center text-pgp-text-muted">
          No instances configured. Add one to start monitoring.
        </div>
      ) : (
        <div className="overflow-hidden rounded-lg border border-pgp-border">
          <table className="w-full">
            <thead className="bg-pgp-bg-secondary">
              <tr>
                <th className="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-pgp-text-muted">
                  Name
                </th>
                <th className="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-pgp-text-muted">
                  Host:Port
                </th>
                <th className="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-pgp-text-muted">
                  Source
                </th>
                <th className="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-pgp-text-muted">
                  Enabled
                </th>
                <th className="px-4 py-3 text-right text-xs font-medium uppercase tracking-wider text-pgp-text-muted">
                  Actions
                </th>
              </tr>
            </thead>
            <tbody className="divide-y divide-pgp-border">
              {instances.map((inst) => (
                <tr key={inst.id} className="hover:bg-pgp-bg-hover">
                  <td className="px-4 py-3 text-sm text-pgp-text-primary">
                    {inst.name || inst.id}
                  </td>
                  <td className="px-4 py-3 text-sm font-mono text-pgp-text-secondary">
                    {inst.host}:{inst.port}
                  </td>
                  <td className="px-4 py-3">
                    <span
                      className={`inline-flex rounded-full px-2 py-0.5 text-xs font-medium ${SOURCE_COLORS[inst.source] || ''}`}
                    >
                      {inst.source}
                    </span>
                  </td>
                  <td className="px-4 py-3">
                    <button
                      type="button"
                      onClick={() => handleToggleEnabled(inst)}
                      className="relative inline-flex h-5 w-9 items-center rounded-full transition-colors focus:outline-none focus:ring-2 focus:ring-blue-500"
                      role="switch"
                      aria-checked={inst.enabled}
                    >
                      <span
                        className={`absolute inset-0 rounded-full transition-colors ${
                          inst.enabled ? 'bg-blue-600' : 'bg-gray-600'
                        }`}
                      />
                      <span
                        className={`relative inline-block h-3.5 w-3.5 rounded-full bg-white transition-transform ${
                          inst.enabled ? 'translate-x-4.5' : 'translate-x-1'
                        }`}
                      />
                    </button>
                  </td>
                  <td className="px-4 py-3 text-right">
                    <div className="flex items-center justify-end gap-2">
                      <button
                        onClick={() => setEditInstance(inst)}
                        className="rounded p-1 text-pgp-text-muted hover:bg-pgp-bg-hover hover:text-pgp-text-primary"
                        title="Edit"
                      >
                        <Pencil className="h-4 w-4" />
                      </button>
                      <button
                        onClick={() => setDeleteInstance(inst)}
                        className="rounded p-1 text-pgp-text-muted hover:bg-pgp-bg-hover hover:text-red-400"
                        title="Delete"
                      >
                        <Trash2 className="h-4 w-4" />
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {showCreateModal && <InstanceForm onClose={() => setShowCreateModal(false)} />}
      {editInstance && (
        <InstanceForm instance={editInstance} onClose={() => setEditInstance(null)} />
      )}
      {deleteInstance && (
        <DeleteInstanceModal instance={deleteInstance} onClose={() => setDeleteInstance(null)} />
      )}
      {showBulkImport && <BulkImportModal onClose={() => setShowBulkImport(false)} />}
    </div>
  )
}
