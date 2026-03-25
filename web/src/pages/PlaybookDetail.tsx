import { useState } from 'react'
import { useParams, useNavigate, Link } from 'react-router-dom'
import { ArrowLeft, Play, Edit, ChevronDown, ChevronRight, Clock, Layers, Shield } from 'lucide-react'
import { Spinner } from '@/components/ui/Spinner'
import { TierBadge } from '@/components/playbook/TierBadge'
import { usePlaybook, usePromotePlaybook, useDeprecatePlaybook, useStartRun } from '@/hooks/usePlaybooks'
import { useInstances } from '@/hooks/useInstances'
import { useAuth } from '@/hooks/useAuth'
import { toast } from '@/stores/toastStore'
import type { PlaybookStep } from '@/types/playbook'

function statusBadgeClass(status: string): string {
  if (status === 'stable') return 'bg-green-500/20 text-green-400'
  if (status === 'draft') return 'bg-amber-500/20 text-amber-400'
  return 'bg-gray-500/20 text-gray-400'
}

function StepRow({ step, defaultOpen }: { step: PlaybookStep; defaultOpen?: boolean }) {
  const [open, setOpen] = useState(defaultOpen ?? false)

  return (
    <div className="rounded-md border border-pgp-border bg-pgp-bg-secondary">
      <button
        onClick={() => setOpen(!open)}
        className="flex w-full items-center gap-3 px-4 py-3 text-left"
      >
        <span className="flex h-6 w-6 items-center justify-center rounded-full bg-pgp-bg-hover text-xs font-bold text-pgp-text-primary">
          {step.step_order}
        </span>
        <TierBadge tier={step.safety_tier} />
        <span className="flex-1 text-sm font-medium text-pgp-text-primary">{step.name}</span>
        {open ? (
          <ChevronDown className="h-4 w-4 text-pgp-text-muted" />
        ) : (
          <ChevronRight className="h-4 w-4 text-pgp-text-muted" />
        )}
      </button>
      {open && (
        <div className="space-y-3 border-t border-pgp-border px-4 py-3">
          <p className="text-xs text-pgp-text-secondary">{step.description}</p>
          {step.sql_template && (
            <div>
              <p className="mb-1 text-[10px] font-medium uppercase tracking-wider text-pgp-text-muted">SQL</p>
              <pre className="overflow-x-auto rounded-md bg-pgp-bg-primary p-3 text-xs text-pgp-text-primary">
                {step.sql_template}
              </pre>
            </div>
          )}
          {step.manual_instructions && (
            <div>
              <p className="mb-1 text-[10px] font-medium uppercase tracking-wider text-pgp-text-muted">
                Manual Instructions
              </p>
              <p className="text-xs text-pgp-text-secondary">{step.manual_instructions}</p>
            </div>
          )}
          {step.escalation_contact && (
            <p className="text-xs text-pgp-text-muted">
              Escalation: {step.escalation_contact}
            </p>
          )}
          <div className="flex items-center gap-3 text-[10px] text-pgp-text-muted">
            <span>Timeout: {step.timeout_seconds}s</span>
            {step.result_interpretation && (
              <span>
                {step.result_interpretation.rules.length} interpretation rule(s)
              </span>
            )}
            {step.branch_rules && step.branch_rules.length > 0 && (
              <span>{step.branch_rules.length} branch rule(s)</span>
            )}
          </div>
        </div>
      )}
    </div>
  )
}

export function PlaybookDetail() {
  const { playbookId } = useParams<{ playbookId: string }>()
  const numericId = playbookId ? parseInt(playbookId, 10) : undefined
  const navigate = useNavigate()
  const { can } = useAuth()
  const { data: playbook, isLoading } = usePlaybook(numericId)
  const { data: instances } = useInstances()
  const promote = usePromotePlaybook()
  const deprecate = useDeprecatePlaybook()
  const startRun = useStartRun()
  const [instanceDropdown, setInstanceDropdown] = useState(false)

  if (isLoading || !playbook) {
    return (
      <div className="flex justify-center py-12">
        <Spinner size="lg" />
      </div>
    )
  }

  const handleStartRun = async (instanceId: string) => {
    setInstanceDropdown(false)
    try {
      const run = await startRun.mutateAsync({
        instanceId,
        playbookId: playbook.id,
        triggerSource: 'manual',
      })
      navigate(`/servers/${instanceId}/playbook-runs/${run.id}`)
    } catch {
      toast.error('Failed to start playbook run')
    }
  }

  const handlePromote = async () => {
    try {
      await promote.mutateAsync(playbook.id)
      toast.success('Playbook promoted to stable')
    } catch {
      toast.error('Failed to promote playbook')
    }
  }

  const handleDeprecate = async () => {
    try {
      await deprecate.mutateAsync(playbook.id)
      toast.success('Playbook deprecated')
    } catch {
      toast.error('Failed to deprecate playbook')
    }
  }

  const triggerBindings = playbook.trigger_bindings
  const hasTriggers =
    (triggerBindings.hooks?.length ?? 0) > 0 ||
    (triggerBindings.root_causes?.length ?? 0) > 0 ||
    (triggerBindings.metrics?.length ?? 0) > 0 ||
    (triggerBindings.adviser_rules?.length ?? 0) > 0

  return (
    <div className="space-y-6">
      <Link
        to="/playbooks"
        className="inline-flex items-center gap-1 text-sm text-pgp-text-secondary hover:text-pgp-text-primary"
      >
        <ArrowLeft className="h-4 w-4" /> Back to Playbooks
      </Link>

      {/* Header */}
      <div className="flex items-start justify-between gap-4">
        <div>
          <div className="mb-1 flex items-center gap-2">
            <h1 className="text-xl font-semibold text-pgp-text-primary">{playbook.name}</h1>
            <span
              className={`rounded-full px-2 py-0.5 text-[10px] font-semibold uppercase ${statusBadgeClass(playbook.status)}`}
            >
              {playbook.status}
            </span>
          </div>
          <div className="flex items-center gap-4 text-xs text-pgp-text-muted">
            <span>v{playbook.version}</span>
            <span className="flex items-center gap-1">
              <Layers className="h-3 w-3" />
              {(playbook.steps ?? []).length} steps
            </span>
            {playbook.estimated_duration && (
              <span className="flex items-center gap-1">
                <Clock className="h-3 w-3" />
                {playbook.estimated_duration}
              </span>
            )}
            <span>{playbook.category}</span>
            {playbook.requires_permission && (
              <span className="flex items-center gap-1">
                <Shield className="h-3 w-3" />
                Requires: {playbook.requires_permission}
              </span>
            )}
          </div>
        </div>

        <div className="flex items-center gap-2">
          {/* Run on Instance */}
          <div className="relative">
            <button
              onClick={() => setInstanceDropdown(!instanceDropdown)}
              className="inline-flex items-center gap-2 rounded-md bg-pgp-accent px-3 py-2 text-sm font-medium text-white transition-colors hover:bg-pgp-accent/80"
            >
              <Play className="h-4 w-4" />
              Run on Instance
            </button>
            {instanceDropdown && (
              <div className="absolute right-0 z-10 mt-1 w-56 rounded-md border border-pgp-border bg-pgp-bg-card shadow-lg">
                {(instances ?? []).map((inst) => (
                  <button
                    key={inst.id}
                    onClick={() => handleStartRun(inst.id)}
                    className="block w-full px-3 py-2 text-left text-sm text-pgp-text-primary hover:bg-pgp-bg-hover"
                  >
                    {inst.name || inst.id}
                  </button>
                ))}
              </div>
            )}
          </div>

          {can('alert_management') && (
            <Link
              to={`/playbooks/${playbook.id}/edit`}
              className="inline-flex items-center gap-2 rounded-md border border-pgp-border px-3 py-2 text-sm text-pgp-text-secondary hover:bg-pgp-bg-hover hover:text-pgp-text-primary"
            >
              <Edit className="h-4 w-4" />
              Edit
            </Link>
          )}

          {can('user_management') && playbook.status === 'draft' && (
            <button
              onClick={handlePromote}
              disabled={promote.isPending}
              className="rounded-md border border-green-500/50 px-3 py-2 text-sm text-green-400 hover:bg-green-500/10 disabled:opacity-50"
            >
              Promote
            </button>
          )}

          {can('alert_management') && playbook.status === 'stable' && (
            <button
              onClick={handleDeprecate}
              disabled={deprecate.isPending}
              className="rounded-md border border-pgp-border px-3 py-2 text-sm text-pgp-text-muted hover:bg-pgp-bg-hover disabled:opacity-50"
            >
              Deprecate
            </button>
          )}
        </div>
      </div>

      {/* Description */}
      <div className="rounded-lg border border-pgp-border bg-pgp-bg-card p-4">
        <p className="text-sm text-pgp-text-secondary">{playbook.description}</p>
      </div>

      {/* Steps */}
      <div>
        <h2 className="mb-3 text-sm font-semibold text-pgp-text-primary">
          Steps ({(playbook.steps ?? []).length})
        </h2>
        <div className="space-y-2">
          {(playbook.steps ?? []).map((step) => (
            <StepRow key={step.id} step={step} />
          ))}
        </div>
      </div>

      {/* Trigger Bindings */}
      {hasTriggers && (
        <div className="rounded-lg border border-pgp-border bg-pgp-bg-card p-4">
          <h2 className="mb-3 text-sm font-semibold text-pgp-text-primary">
            Trigger Bindings
          </h2>
          <div className="space-y-2 text-xs">
            {triggerBindings.hooks && triggerBindings.hooks.length > 0 && (
              <div>
                <span className="font-medium text-pgp-text-muted">Hooks:</span>{' '}
                <span className="text-pgp-text-secondary">
                  {triggerBindings.hooks.join(', ')}
                </span>
              </div>
            )}
            {triggerBindings.root_causes && triggerBindings.root_causes.length > 0 && (
              <div>
                <span className="font-medium text-pgp-text-muted">Root Causes:</span>{' '}
                <span className="text-pgp-text-secondary">
                  {triggerBindings.root_causes.join(', ')}
                </span>
              </div>
            )}
            {triggerBindings.metrics && triggerBindings.metrics.length > 0 && (
              <div>
                <span className="font-medium text-pgp-text-muted">Metrics:</span>{' '}
                <span className="text-pgp-text-secondary">
                  {triggerBindings.metrics.join(', ')}
                </span>
              </div>
            )}
            {triggerBindings.adviser_rules && triggerBindings.adviser_rules.length > 0 && (
              <div>
                <span className="font-medium text-pgp-text-muted">Adviser Rules:</span>{' '}
                <span className="text-pgp-text-secondary">
                  {triggerBindings.adviser_rules.join(', ')}
                </span>
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  )
}
