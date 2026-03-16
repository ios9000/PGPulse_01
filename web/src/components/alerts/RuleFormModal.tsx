import { useState, useEffect, useCallback } from 'react'
import { X } from 'lucide-react'
import { useSaveAlertRule, useTestNotification } from '@/hooks/useAlertRules'
import type { AlertRule } from '@/types/models'

export interface RuleFormDefaults {
  name?: string
  description?: string
  metric?: string
  operator?: AlertRule['operator']
  threshold?: number
  severity?: AlertRule['severity']
}

interface RuleFormModalProps {
  onClose: () => void
  rule?: AlertRule
  defaults?: RuleFormDefaults
  availableChannels: string[]
}

const OPERATORS: AlertRule['operator'][] = ['>', '>=', '<', '<=', '==', '!=']
const SEVERITIES: AlertRule['severity'][] = ['info', 'warning', 'critical']

function slugify(name: string): string {
  return name
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '_')
    .replace(/^_|_$/g, '')
}

export function RuleFormModal({ onClose, rule, defaults, availableChannels }: RuleFormModalProps) {
  const isEdit = !!rule
  const isBuiltin = rule?.source === 'builtin'
  const d = defaults

  const [name, setName] = useState(rule?.name ?? d?.name ?? '')
  const [description, setDescription] = useState(rule?.description ?? d?.description ?? '')
  const [metric, setMetric] = useState(rule?.metric ?? d?.metric ?? '')
  const [operator, setOperator] = useState<AlertRule['operator']>(rule?.operator ?? d?.operator ?? '>')
  const [threshold, setThreshold] = useState(String(rule?.threshold ?? d?.threshold ?? ''))
  const [severity, setSeverity] = useState<AlertRule['severity']>(rule?.severity ?? d?.severity ?? 'warning')
  const [consecutiveCount, setConsecutiveCount] = useState(String(rule?.consecutive_count ?? 3))
  const [cooldownMinutes, setCooldownMinutes] = useState(String(rule?.cooldown_minutes ?? 5))
  const [channels, setChannels] = useState<string[]>(rule?.channels ?? [])
  const [enabled, setEnabled] = useState(rule?.enabled ?? true)
  const [error, setError] = useState<string | null>(null)

  const saveRule = useSaveAlertRule()
  const testNotification = useTestNotification()

  const handleClose = useCallback(() => {
    if (!saveRule.isPending) onClose()
  }, [saveRule.isPending, onClose])

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

  const toggleChannel = (ch: string) => {
    setChannels((prev) => (prev.includes(ch) ? prev.filter((c) => c !== ch) : [...prev, ch]))
  }

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError(null)

    if (!name.trim()) {
      setError('Name is required')
      return
    }
    if (!metric.trim()) {
      setError('Metric is required')
      return
    }
    const thresholdNum = parseFloat(threshold)
    if (isNaN(thresholdNum) || thresholdNum <= 0) {
      setError('Threshold must be greater than 0')
      return
    }

    try {
      await saveRule.mutateAsync({
        id: isEdit ? rule.id : slugify(name),
        name,
        description: description || undefined,
        metric,
        operator,
        threshold: thresholdNum,
        severity,
        consecutive_count: parseInt(consecutiveCount, 10) || 1,
        cooldown_minutes: parseInt(cooldownMinutes, 10) || 1,
        channels: channels.length > 0 ? channels : undefined,
        enabled,
      })
      onClose()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save rule')
    }
  }

  const handleTestNotification = async () => {
    if (!channels.length) return
    for (const ch of channels) {
      try {
        await testNotification.mutateAsync({
          channel: ch,
          message: `Test notification for rule: ${name}`,
        })
      } catch {
        setError(`Failed to send test to ${ch}`)
        return
      }
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
            {isEdit ? 'Edit Rule' : 'Create Rule'}
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
              disabled={isBuiltin}
              className={`${inputClass} ${isBuiltin ? 'opacity-60' : ''}`}
              required
            />
          </div>

          <div>
            <label className={labelClass}>Description</label>
            <input
              type="text"
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              disabled={isBuiltin}
              className={`${inputClass} ${isBuiltin ? 'opacity-60' : ''}`}
            />
          </div>

          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className={labelClass}>Metric</label>
              <input
                type="text"
                value={metric}
                onChange={(e) => setMetric(e.target.value)}
                disabled={isBuiltin}
                className={`${inputClass} ${isBuiltin ? 'opacity-60' : ''}`}
                required
              />
            </div>
            <div>
              <label className={labelClass}>Operator</label>
              <select
                value={operator}
                onChange={(e) => setOperator(e.target.value as AlertRule['operator'])}
                disabled={isBuiltin}
                className={`${inputClass} ${isBuiltin ? 'opacity-60' : ''}`}
              >
                {OPERATORS.map((op) => (
                  <option key={op} value={op}>
                    {op}
                  </option>
                ))}
              </select>
            </div>
          </div>

          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className={labelClass}>Threshold</label>
              <input
                type="number"
                value={threshold}
                onChange={(e) => setThreshold(e.target.value)}
                className={inputClass}
                step="any"
                min="0"
                required
              />
            </div>
            <div>
              <label className={labelClass}>Severity</label>
              <select
                value={severity}
                onChange={(e) => setSeverity(e.target.value as AlertRule['severity'])}
                disabled={isBuiltin}
                className={`${inputClass} ${isBuiltin ? 'opacity-60' : ''}`}
              >
                {SEVERITIES.map((s) => (
                  <option key={s} value={s}>
                    {s}
                  </option>
                ))}
              </select>
            </div>
          </div>

          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className={labelClass}>Consecutive count</label>
              <input
                type="number"
                value={consecutiveCount}
                onChange={(e) => setConsecutiveCount(e.target.value)}
                className={inputClass}
                min="1"
              />
            </div>
            <div>
              <label className={labelClass}>Cooldown (minutes)</label>
              <input
                type="number"
                value={cooldownMinutes}
                onChange={(e) => setCooldownMinutes(e.target.value)}
                className={inputClass}
                min="1"
              />
            </div>
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

          {availableChannels.length > 0 && (
            <div>
              <label className={labelClass}>Notification channels</label>
              <div className="mt-1 flex flex-wrap gap-2">
                {availableChannels.map((ch) => (
                  <label
                    key={ch}
                    className="flex cursor-pointer items-center gap-1.5 rounded-md border border-pgp-border bg-pgp-bg-secondary px-2.5 py-1.5 text-sm text-pgp-text-secondary transition-colors hover:bg-pgp-bg-hover"
                  >
                    <input
                      type="checkbox"
                      checked={channels.includes(ch)}
                      onChange={() => toggleChannel(ch)}
                      className="rounded border-pgp-border"
                    />
                    {ch}
                  </label>
                ))}
              </div>
            </div>
          )}

          <div className="flex items-center justify-between pt-2">
            <div>
              {isEdit && channels.length > 0 && (
                <button
                  type="button"
                  onClick={handleTestNotification}
                  disabled={testNotification.isPending}
                  className="rounded-md border border-pgp-border px-3 py-2 text-sm text-pgp-text-secondary hover:bg-pgp-bg-hover disabled:opacity-50"
                >
                  {testNotification.isPending ? 'Sending...' : 'Test Notification'}
                </button>
              )}
            </div>
            <div className="flex gap-3">
              <button
                type="button"
                onClick={handleClose}
                className="rounded-md px-4 py-2 text-sm text-pgp-text-secondary hover:bg-pgp-bg-hover"
              >
                Cancel
              </button>
              <button
                type="submit"
                disabled={saveRule.isPending}
                className="rounded-md bg-pgp-accent px-4 py-2 text-sm font-medium text-white hover:bg-pgp-accent-hover disabled:opacity-50"
              >
                {saveRule.isPending ? 'Saving...' : isEdit ? 'Save Changes' : 'Create Rule'}
              </button>
            </div>
          </div>
        </form>
      </div>
    </div>
  )
}
