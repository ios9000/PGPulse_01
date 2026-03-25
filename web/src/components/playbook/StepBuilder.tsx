import { Plus, Trash2, ArrowUp, ArrowDown } from 'lucide-react'
import type { PlaybookStep, SafetyTier } from '@/types/playbook'

type DraftStep = Omit<PlaybookStep, 'id' | 'playbook_id'>

interface StepBuilderProps {
  steps: DraftStep[]
  onChange: (steps: DraftStep[]) => void
}

const TIER_OPTIONS: { value: SafetyTier; label: string }[] = [
  { value: 'diagnostic', label: 'Diagnostic (read-only)' },
  { value: 'remediate', label: 'Remediate (requires confirmation)' },
  { value: 'dangerous', label: 'Dangerous (requires DBA approval)' },
  { value: 'external', label: 'External / Manual' },
]

function emptyStep(order: number): DraftStep {
  return {
    step_order: order,
    name: '',
    description: '',
    safety_tier: 'diagnostic',
    sql_template: '',
    timeout_seconds: 5,
    result_interpretation: null,
    branch_rules: null,
    manual_instructions: null,
    escalation_contact: null,
    requires_permission: null,
  }
}

export function StepBuilder({ steps, onChange }: StepBuilderProps) {
  const updateStep = (index: number, patch: Partial<DraftStep>) => {
    const updated = steps.map((s, i) => (i === index ? { ...s, ...patch } : s))
    onChange(updated)
  }

  const addStep = () => {
    onChange([...steps, emptyStep(steps.length + 1)])
  }

  const removeStep = (index: number) => {
    const updated = steps
      .filter((_, i) => i !== index)
      .map((s, i) => ({ ...s, step_order: i + 1 }))
    onChange(updated)
  }

  const moveUp = (index: number) => {
    if (index === 0) return
    const updated = [...steps]
    const tmp = updated[index - 1]
    updated[index - 1] = updated[index]
    updated[index] = tmp
    onChange(updated.map((s, i) => ({ ...s, step_order: i + 1 })))
  }

  const moveDown = (index: number) => {
    if (index >= steps.length - 1) return
    const updated = [...steps]
    const tmp = updated[index + 1]
    updated[index + 1] = updated[index]
    updated[index] = tmp
    onChange(updated.map((s, i) => ({ ...s, step_order: i + 1 })))
  }

  return (
    <div className="space-y-4">
      {steps.map((step, idx) => (
        <div
          key={idx}
          className="rounded-lg border border-pgp-border bg-pgp-bg-secondary p-4"
        >
          <div className="mb-3 flex items-center justify-between">
            <span className="text-sm font-medium text-pgp-text-primary">
              Step {step.step_order}
            </span>
            <div className="flex items-center gap-1">
              <button
                type="button"
                onClick={() => moveUp(idx)}
                disabled={idx === 0}
                className="rounded p-1 text-pgp-text-muted hover:text-pgp-text-primary disabled:opacity-30"
              >
                <ArrowUp className="h-4 w-4" />
              </button>
              <button
                type="button"
                onClick={() => moveDown(idx)}
                disabled={idx === steps.length - 1}
                className="rounded p-1 text-pgp-text-muted hover:text-pgp-text-primary disabled:opacity-30"
              >
                <ArrowDown className="h-4 w-4" />
              </button>
              <button
                type="button"
                onClick={() => removeStep(idx)}
                className="rounded p-1 text-red-400 hover:text-red-300"
              >
                <Trash2 className="h-4 w-4" />
              </button>
            </div>
          </div>

          <div className="grid grid-cols-2 gap-3">
            <div className="col-span-2 sm:col-span-1">
              <label className="mb-1 block text-[10px] font-medium uppercase tracking-wider text-pgp-text-muted">
                Name
              </label>
              <input
                type="text"
                value={step.name}
                onChange={(e) => updateStep(idx, { name: e.target.value })}
                className="w-full rounded-md border border-pgp-border bg-pgp-bg-primary px-3 py-1.5 text-sm text-pgp-text-primary"
              />
            </div>

            <div className="col-span-2 sm:col-span-1">
              <label className="mb-1 block text-[10px] font-medium uppercase tracking-wider text-pgp-text-muted">
                Safety Tier
              </label>
              <select
                value={step.safety_tier}
                onChange={(e) => updateStep(idx, { safety_tier: e.target.value as SafetyTier })}
                className="w-full rounded-md border border-pgp-border bg-pgp-bg-primary px-3 py-1.5 text-sm text-pgp-text-primary"
              >
                {TIER_OPTIONS.map((opt) => (
                  <option key={opt.value} value={opt.value}>
                    {opt.label}
                  </option>
                ))}
              </select>
            </div>

            <div className="col-span-2">
              <label className="mb-1 block text-[10px] font-medium uppercase tracking-wider text-pgp-text-muted">
                Description
              </label>
              <textarea
                value={step.description}
                onChange={(e) => updateStep(idx, { description: e.target.value })}
                rows={2}
                className="w-full rounded-md border border-pgp-border bg-pgp-bg-primary px-3 py-1.5 text-sm text-pgp-text-primary"
              />
            </div>

            {step.safety_tier !== 'external' && (
              <div className="col-span-2">
                <label className="mb-1 block text-[10px] font-medium uppercase tracking-wider text-pgp-text-muted">
                  SQL Template
                </label>
                <textarea
                  value={step.sql_template ?? ''}
                  onChange={(e) => updateStep(idx, { sql_template: e.target.value || null })}
                  rows={4}
                  className="w-full rounded-md border border-pgp-border bg-pgp-bg-primary px-3 py-1.5 font-mono text-xs text-pgp-text-primary"
                />
              </div>
            )}

            <div className="col-span-2 sm:col-span-1">
              <label className="mb-1 block text-[10px] font-medium uppercase tracking-wider text-pgp-text-muted">
                Timeout (seconds)
              </label>
              <input
                type="number"
                value={step.timeout_seconds}
                onChange={(e) => updateStep(idx, { timeout_seconds: parseInt(e.target.value, 10) || 5 })}
                min={1}
                className="w-full rounded-md border border-pgp-border bg-pgp-bg-primary px-3 py-1.5 text-sm text-pgp-text-primary"
              />
            </div>

            {step.safety_tier === 'external' && (
              <>
                <div className="col-span-2">
                  <label className="mb-1 block text-[10px] font-medium uppercase tracking-wider text-pgp-text-muted">
                    Manual Instructions
                  </label>
                  <textarea
                    value={step.manual_instructions ?? ''}
                    onChange={(e) => updateStep(idx, { manual_instructions: e.target.value || null })}
                    rows={3}
                    className="w-full rounded-md border border-pgp-border bg-pgp-bg-primary px-3 py-1.5 text-sm text-pgp-text-primary"
                  />
                </div>
                <div className="col-span-2 sm:col-span-1">
                  <label className="mb-1 block text-[10px] font-medium uppercase tracking-wider text-pgp-text-muted">
                    Escalation Contact
                  </label>
                  <input
                    type="text"
                    value={step.escalation_contact ?? ''}
                    onChange={(e) => updateStep(idx, { escalation_contact: e.target.value || null })}
                    className="w-full rounded-md border border-pgp-border bg-pgp-bg-primary px-3 py-1.5 text-sm text-pgp-text-primary"
                  />
                </div>
              </>
            )}
          </div>
        </div>
      ))}

      <button
        type="button"
        onClick={addStep}
        className="inline-flex items-center gap-2 rounded-md border border-dashed border-pgp-border px-4 py-2 text-sm text-pgp-text-secondary hover:bg-pgp-bg-hover hover:text-pgp-text-primary"
      >
        <Plus className="h-4 w-4" />
        Add Step
      </button>
    </div>
  )
}
