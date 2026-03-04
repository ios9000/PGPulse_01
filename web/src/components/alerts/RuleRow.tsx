import { Pencil, Trash2 } from 'lucide-react'
import { StatusBadge } from '@/components/ui/StatusBadge'
import type { AlertRule } from '@/types/models'

interface RuleRowProps {
  rule: AlertRule
  onToggle: (id: string, enabled: boolean) => void
  onEdit: (rule: AlertRule) => void
  onDelete: (rule: AlertRule) => void
}

function severityStatus(severity: string): 'ok' | 'warning' | 'critical' | 'info' {
  if (severity === 'critical') return 'critical'
  if (severity === 'warning') return 'warning'
  return 'info'
}

export function RuleRow({ rule, onToggle, onEdit, onDelete }: RuleRowProps) {
  return (
    <tr className="border-b border-pgp-border transition-colors hover:bg-pgp-bg-hover">
      <td className="px-4 py-3">
        <div className="flex items-center gap-2">
          <span className="text-sm font-medium text-pgp-text-primary">{rule.name}</span>
          {rule.source === 'builtin' && (
            <span className="inline-flex items-center rounded-full bg-blue-500/20 px-2 py-0.5 text-xs font-medium text-blue-400">
              System
            </span>
          )}
        </div>
      </td>
      <td className="px-4 py-3 text-sm text-pgp-text-secondary">{rule.metric}</td>
      <td className="px-4 py-3 text-sm text-pgp-text-secondary">
        {rule.operator} {rule.threshold}
      </td>
      <td className="px-4 py-3">
        <StatusBadge status={severityStatus(rule.severity)} label={rule.severity} />
      </td>
      <td className="px-4 py-3 text-sm text-pgp-text-muted">{rule.cooldown_minutes}m</td>
      <td className="px-4 py-3">
        <div className="flex flex-wrap gap-1">
          {rule.channels?.length ? (
            rule.channels.map((ch) => (
              <span
                key={ch}
                className="inline-flex items-center rounded-full bg-pgp-bg-secondary px-2 py-0.5 text-xs text-pgp-text-secondary"
              >
                {ch}
              </span>
            ))
          ) : (
            <span className="text-xs text-pgp-text-muted">--</span>
          )}
        </div>
      </td>
      <td className="px-4 py-3">
        <button
          onClick={() => onToggle(rule.id, !rule.enabled)}
          className="relative inline-flex h-5 w-9 items-center rounded-full transition-colors focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-1 focus:ring-offset-pgp-bg-card"
          role="switch"
          aria-checked={rule.enabled}
        >
          <span
            className={`absolute inset-0 rounded-full transition-colors ${
              rule.enabled ? 'bg-blue-600' : 'bg-gray-600'
            }`}
          />
          <span
            className={`relative inline-block h-3.5 w-3.5 rounded-full bg-white transition-transform ${
              rule.enabled ? 'translate-x-4.5' : 'translate-x-1'
            }`}
          />
        </button>
      </td>
      <td className="px-4 py-3">
        <div className="flex items-center gap-1">
          <button
            onClick={() => onEdit(rule)}
            className="rounded p-1.5 text-pgp-text-muted transition-colors hover:bg-pgp-bg-hover hover:text-pgp-text-primary"
            title="Edit rule"
          >
            <Pencil className="h-4 w-4" />
          </button>
          {rule.source === 'custom' && (
            <button
              onClick={() => onDelete(rule)}
              className="rounded p-1.5 text-pgp-text-muted transition-colors hover:bg-red-500/10 hover:text-red-400"
              title="Delete rule"
            >
              <Trash2 className="h-4 w-4" />
            </button>
          )}
        </div>
      </td>
    </tr>
  )
}
