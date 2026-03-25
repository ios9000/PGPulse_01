import { useState } from 'react'
import { ChevronDown, ChevronRight, Play, AlertTriangle, Lock, ClipboardCheck, RotateCw, SkipForward } from 'lucide-react'
import { TierBadge } from '@/components/playbook/TierBadge'
import { VerdictBadge } from '@/components/playbook/VerdictBadge'
import { ResultTable } from '@/components/playbook/ResultTable'
import { BranchIndicator } from '@/components/playbook/BranchIndicator'
import type { PlaybookStep, PlaybookRunStep, Verdict } from '@/types/playbook'
import { useAuth } from '@/hooks/useAuth'

interface StepCardProps {
  step: PlaybookStep
  runStep?: PlaybookRunStep
  isCurrent: boolean
  branchedTo?: { step: number; label: string }
  onExecute: () => void
  onConfirm: () => void
  onApprove: () => void
  onRequestApproval: () => void
  onSkip: () => void
  onRetry: () => void
  isExecuting: boolean
}

export function StepCard({
  step,
  runStep,
  isCurrent,
  branchedTo,
  onExecute,
  onConfirm,
  onApprove,
  onRequestApproval,
  onSkip,
  onRetry,
  isExecuting,
}: StepCardProps) {
  const [sqlOpen, setSqlOpen] = useState(false)
  const { can } = useAuth()

  const status = runStep?.status ?? 'pending'
  const isCompleted = status === 'completed'
  const isFailed = status === 'failed'
  const isSkipped = status === 'skipped'
  const isPendingApproval = status === 'pending_approval' || status === 'awaiting_approval'
  const isAwaitingConfirmation = status === 'awaiting_confirmation'

  const borderClass = isCurrent
    ? 'border-pgp-accent'
    : isCompleted
      ? 'border-green-500/30'
      : isFailed
        ? 'border-red-500/30'
        : 'border-pgp-border'

  return (
    <div className={`rounded-lg border bg-pgp-bg-card ${borderClass}`}>
      {/* Header */}
      <div className="flex items-center gap-3 px-4 py-3">
        <span
          className={`flex h-7 w-7 items-center justify-center rounded-full text-xs font-bold ${
            isCompleted
              ? 'bg-green-500/20 text-green-400'
              : isCurrent
                ? 'bg-pgp-accent/20 text-pgp-accent'
                : 'bg-pgp-bg-hover text-pgp-text-muted'
          }`}
        >
          {step.step_order}
        </span>
        <TierBadge tier={step.safety_tier} />
        <span className="flex-1 text-sm font-medium text-pgp-text-primary">{step.name}</span>
        {isCompleted && runStep?.verdict && (
          <VerdictBadge verdict={runStep.verdict as Verdict} message={runStep.verdict_message ?? undefined} />
        )}
        {isSkipped && (
          <span className="rounded-full bg-gray-500/20 px-2 py-0.5 text-xs text-gray-400">Skipped</span>
        )}
        {isFailed && (
          <span className="rounded-full bg-red-500/20 px-2 py-0.5 text-xs text-red-400">Failed</span>
        )}
      </div>

      {/* Body — only show for current step or completed steps when expanded */}
      {(isCurrent || isCompleted || isFailed || isSkipped) && (
        <div className="space-y-3 border-t border-pgp-border px-4 py-3">
          <p className="text-xs text-pgp-text-secondary">{step.description}</p>

          {/* SQL toggle */}
          {step.sql_template && (
            <div>
              <button
                onClick={() => setSqlOpen(!sqlOpen)}
                className="flex items-center gap-1 text-[10px] font-medium text-pgp-text-muted hover:text-pgp-text-secondary"
              >
                {sqlOpen ? <ChevronDown className="h-3 w-3" /> : <ChevronRight className="h-3 w-3" />}
                Show SQL
              </button>
              {sqlOpen && (
                <pre className="mt-1 overflow-x-auto rounded-md bg-pgp-bg-primary p-3 text-xs text-pgp-text-primary">
                  {step.sql_template}
                </pre>
              )}
            </div>
          )}

          {/* Manual instructions for Tier 4 */}
          {step.safety_tier === 'external' && step.manual_instructions && (
            <div className="rounded-md border border-pgp-border bg-pgp-bg-secondary p-3">
              <p className="mb-1 text-[10px] font-medium uppercase tracking-wider text-pgp-text-muted">
                Manual Instructions
              </p>
              <p className="text-xs text-pgp-text-secondary">{step.manual_instructions}</p>
              {step.escalation_contact && (
                <p className="mt-2 text-xs text-pgp-text-muted">
                  Escalation: {step.escalation_contact}
                </p>
              )}
            </div>
          )}

          {/* Error banner for failed steps (C6) */}
          {isFailed && runStep?.error && (
            <div className="rounded-md border border-red-500/30 bg-red-500/10 px-3 py-2">
              <p className="text-xs font-medium text-red-400">Error: {runStep.error}</p>
            </div>
          )}

          {/* Results for completed steps */}
          {isCompleted && runStep?.result_json && (
            <ResultTable result={runStep.result_json} />
          )}

          {/* Branch indicator */}
          {branchedTo && (
            <BranchIndicator targetStep={branchedTo.step} label={branchedTo.label} />
          )}

          {/* Action buttons for current step */}
          {isCurrent && !isCompleted && (
            <div className="flex items-center gap-2 pt-1">
              {/* Tier 1: Diagnostic — auto-run */}
              {step.safety_tier === 'diagnostic' && status === 'pending' && (
                <button
                  onClick={onExecute}
                  disabled={isExecuting}
                  className="inline-flex items-center gap-2 rounded-md bg-green-600 px-3 py-2 text-sm font-medium text-white transition-colors hover:bg-green-700 disabled:opacity-50"
                >
                  <Play className="h-4 w-4" />
                  {isExecuting ? 'Running...' : 'Run Diagnostic'}
                </button>
              )}

              {/* Tier 2: Remediate — confirmation required */}
              {step.safety_tier === 'remediate' && (status === 'pending' || isAwaitingConfirmation) && (
                <button
                  onClick={onConfirm}
                  disabled={isExecuting}
                  className="inline-flex items-center gap-2 rounded-md bg-amber-600 px-3 py-2 text-sm font-medium text-white transition-colors hover:bg-amber-700 disabled:opacity-50"
                >
                  <AlertTriangle className="h-4 w-4" />
                  {isExecuting ? 'Executing...' : 'Execute'}
                </button>
              )}

              {/* Tier 3: Dangerous — needs DBA approval */}
              {step.safety_tier === 'dangerous' && status === 'pending' && (
                <>
                  {can('instance_management') ? (
                    <button
                      onClick={onApprove}
                      disabled={isExecuting}
                      className="inline-flex items-center gap-2 rounded-md bg-red-600 px-3 py-2 text-sm font-medium text-white transition-colors hover:bg-red-700 disabled:opacity-50"
                    >
                      <Lock className="h-4 w-4" />
                      {isExecuting ? 'Executing...' : 'Approve and Execute'}
                    </button>
                  ) : (
                    <button
                      onClick={onRequestApproval}
                      disabled={isExecuting}
                      className="inline-flex items-center gap-2 rounded-md border border-red-500/50 px-3 py-2 text-sm text-red-400 transition-colors hover:bg-red-500/10 disabled:opacity-50"
                    >
                      <Lock className="h-4 w-4" />
                      Request DBA Approval
                    </button>
                  )}
                </>
              )}

              {/* Tier 3: Pending approval state */}
              {step.safety_tier === 'dangerous' && isPendingApproval && (
                <>
                  {can('instance_management') ? (
                    <button
                      onClick={onApprove}
                      disabled={isExecuting}
                      className="inline-flex items-center gap-2 rounded-md bg-red-600 px-3 py-2 text-sm font-medium text-white transition-colors hover:bg-red-700 disabled:opacity-50"
                    >
                      <Lock className="h-4 w-4" />
                      {isExecuting ? 'Executing...' : 'Approve and Execute'}
                    </button>
                  ) : (
                    <span className="text-xs text-amber-400">
                      Approval requested. A DBA can approve this step from this run URL.
                    </span>
                  )}
                </>
              )}

              {/* Tier 4: Manual — mark as done */}
              {step.safety_tier === 'external' && status === 'pending' && (
                <button
                  onClick={onExecute}
                  disabled={isExecuting}
                  className="inline-flex items-center gap-2 rounded-md border border-pgp-border px-3 py-2 text-sm text-pgp-text-secondary transition-colors hover:bg-pgp-bg-hover disabled:opacity-50"
                >
                  <ClipboardCheck className="h-4 w-4" />
                  Mark as Done
                </button>
              )}

              {/* Failed — retry button (C6) */}
              {isFailed && (
                <button
                  onClick={onRetry}
                  disabled={isExecuting}
                  className="inline-flex items-center gap-2 rounded-md border border-pgp-border px-3 py-2 text-sm text-pgp-text-secondary transition-colors hover:bg-pgp-bg-hover disabled:opacity-50"
                >
                  <RotateCw className="h-4 w-4" />
                  Retry Step
                </button>
              )}

              {/* Skip button — always available for current step */}
              {(status === 'pending' || isPendingApproval) && (
                <button
                  onClick={onSkip}
                  disabled={isExecuting}
                  className="inline-flex items-center gap-1 text-xs text-pgp-text-muted hover:text-pgp-text-secondary"
                >
                  <SkipForward className="h-3 w-3" />
                  Skip
                </button>
              )}
            </div>
          )}
        </div>
      )}
    </div>
  )
}
