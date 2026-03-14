import { useState, useMemo } from 'react'
import { Plus } from 'lucide-react'
import { PageHeader } from '@/components/ui/PageHeader'
import { AlertsTabBar } from '@/components/alerts/AlertsTabBar'
import { Spinner } from '@/components/ui/Spinner'
import { EmptyState } from '@/components/ui/EmptyState'
import { RuleRow } from '@/components/alerts/RuleRow'
import { RuleFormModal } from '@/components/alerts/RuleFormModal'
import { DeleteConfirmModal } from '@/components/alerts/DeleteConfirmModal'
import { useAlertRules, useSaveAlertRule } from '@/hooks/useAlertRules'
import type { AlertRule } from '@/types/models'

export function AlertRules() {
  const { data: rules, isLoading } = useAlertRules()
  const saveRule = useSaveAlertRule()

  const [showCreateModal, setShowCreateModal] = useState(false)
  const [editingRule, setEditingRule] = useState<AlertRule | undefined>(undefined)
  const [deletingRule, setDeletingRule] = useState<AlertRule | null>(null)

  const availableChannels = useMemo(() => {
    if (!rules) return []
    return [...new Set(rules.flatMap((r) => r.channels ?? []))]
  }, [rules])

  const handleToggle = (id: string, enabled: boolean) => {
    const rule = rules?.find((r) => r.id === id)
    if (!rule) return
    saveRule.mutate({
      id: rule.id,
      name: rule.name,
      metric: rule.metric,
      operator: rule.operator,
      threshold: rule.threshold,
      severity: rule.severity,
      consecutive_count: rule.consecutive_count,
      cooldown_minutes: rule.cooldown_minutes,
      channels: rule.channels,
      enabled,
    })
  }

  const handleEdit = (rule: AlertRule) => {
    setEditingRule(rule)
  }

  const handleDelete = (rule: AlertRule) => {
    setDeletingRule(rule)
  }

  const handleCloseFormModal = () => {
    setShowCreateModal(false)
    setEditingRule(undefined)
  }

  return (
    <div>
      <PageHeader
        title="Alert Rules"
        actions={
          <button
            onClick={() => setShowCreateModal(true)}
            className="inline-flex items-center gap-1.5 rounded-md bg-pgp-accent px-4 py-2 text-sm font-medium text-white hover:bg-pgp-accent-hover"
          >
            <Plus className="h-4 w-4" />
            Create Rule
          </button>
        }
      />

      <AlertsTabBar activeTab="rules" />

      <div className="rounded-lg border border-pgp-border bg-pgp-bg-card">
        {isLoading ? (
          <div className="flex justify-center py-12">
            <Spinner size="lg" />
          </div>
        ) : !rules?.length ? (
          <EmptyState title="No alert rules" description="Create your first alert rule to get started" />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-left">
              <thead>
                <tr className="border-b border-pgp-border">
                  <th className="px-4 py-3 text-xs font-medium uppercase text-pgp-text-muted">
                    Name
                  </th>
                  <th className="px-4 py-3 text-xs font-medium uppercase text-pgp-text-muted">
                    Metric
                  </th>
                  <th className="px-4 py-3 text-xs font-medium uppercase text-pgp-text-muted">
                    Threshold
                  </th>
                  <th className="px-4 py-3 text-xs font-medium uppercase text-pgp-text-muted">
                    Severity
                  </th>
                  <th className="px-4 py-3 text-xs font-medium uppercase text-pgp-text-muted">
                    Cooldown
                  </th>
                  <th className="px-4 py-3 text-xs font-medium uppercase text-pgp-text-muted">
                    Channels
                  </th>
                  <th className="px-4 py-3 text-xs font-medium uppercase text-pgp-text-muted">
                    Enabled
                  </th>
                  <th className="px-4 py-3 text-xs font-medium uppercase text-pgp-text-muted">
                    Actions
                  </th>
                </tr>
              </thead>
              <tbody>
                {rules.map((rule) => (
                  <RuleRow
                    key={rule.id}
                    rule={rule}
                    onToggle={handleToggle}
                    onEdit={handleEdit}
                    onDelete={handleDelete}
                  />
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {(showCreateModal || editingRule) && (
        <RuleFormModal
          onClose={handleCloseFormModal}
          rule={editingRule}
          availableChannels={availableChannels}
        />
      )}

      {deletingRule && (
        <DeleteConfirmModal
          onClose={() => setDeletingRule(null)}
          rule={deletingRule}
        />
      )}
    </div>
  )
}
