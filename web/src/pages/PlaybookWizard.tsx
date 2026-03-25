import { useState, useEffect, useRef } from 'react'
import { useParams, Link } from 'react-router-dom'
import { ArrowLeft, Clock, XCircle } from 'lucide-react'
import { Spinner } from '@/components/ui/Spinner'
import { RunProgressBar } from '@/components/playbook/RunProgressBar'
import { StepCard } from '@/components/playbook/StepCard'
import { FeedbackModal } from '@/components/playbook/FeedbackModal'
import {
  usePlaybookRun,
  useExecuteStep,
  useConfirmStep,
  useApproveStep,
  useRequestApproval,
  useSkipStep,
  useRetryStep,
  useAbandonRun,
} from '@/hooks/usePlaybooks'
import { toast } from '@/stores/toastStore'

function formatElapsed(startedAt: string): string {
  const diffMs = Date.now() - new Date(startedAt).getTime()
  const seconds = Math.floor(diffMs / 1000)
  const minutes = Math.floor(seconds / 60)
  const hours = Math.floor(minutes / 60)
  if (hours > 0) return `${hours}h ${minutes % 60}m`
  if (minutes > 0) return `${minutes}m ${seconds % 60}s`
  return `${seconds}s`
}

export function PlaybookWizard() {
  const { serverId, runId: runIdStr } = useParams<{ serverId: string; runId: string }>()
  const runId = runIdStr ? parseInt(runIdStr, 10) : undefined
  const { data: run, isLoading } = usePlaybookRun(runId)
  const executeStep = useExecuteStep()
  const confirmStep = useConfirmStep()
  const approveStep = useApproveStep()
  const requestApproval = useRequestApproval()
  const skipStep = useSkipStep()
  const retryStep = useRetryStep()
  const abandonRun = useAbandonRun()

  const [showFeedback, setShowFeedback] = useState(false)
  const [elapsed, setElapsed] = useState('')
  const feedbackShownRef = useRef(false)

  // Elapsed time ticker
  useEffect(() => {
    if (!run) return
    const tick = () => setElapsed(formatElapsed(run.started_at))
    tick()
    const interval = setInterval(tick, 1000)
    return () => clearInterval(interval)
  }, [run])

  // Show feedback when run completes
  useEffect(() => {
    if (
      run &&
      (run.status === 'completed' || run.status === 'escalated') &&
      !feedbackShownRef.current &&
      run.feedback_helpful === null
    ) {
      feedbackShownRef.current = true
      setShowFeedback(true)
    }
  }, [run])

  if (isLoading || !run) {
    return (
      <div className="flex justify-center py-12">
        <Spinner size="lg" />
      </div>
    )
  }

  const stepDefs = run.step_definitions ?? []
  const runSteps = run.steps ?? []
  const isActive = run.status === 'in_progress'
  const isExecuting =
    executeStep.isPending ||
    confirmStep.isPending ||
    approveStep.isPending ||
    requestApproval.isPending ||
    skipStep.isPending ||
    retryStep.isPending

  const handleExecute = async (stepOrder: number) => {
    try {
      await executeStep.mutateAsync({ runId: run.id, stepOrder })
    } catch {
      toast.error('Failed to execute step')
    }
  }

  const handleConfirm = async (stepOrder: number) => {
    try {
      await confirmStep.mutateAsync({ runId: run.id, stepOrder })
    } catch {
      toast.error('Failed to confirm step')
    }
  }

  const handleApprove = async (stepOrder: number) => {
    try {
      await approveStep.mutateAsync({ runId: run.id, stepOrder })
    } catch {
      toast.error('Failed to approve step')
    }
  }

  const handleRequestApproval = async (stepOrder: number) => {
    try {
      await requestApproval.mutateAsync({ runId: run.id, stepOrder })
      toast.success('Approval requested. A DBA can approve this step from this URL.')
    } catch {
      toast.error('Failed to request approval')
    }
  }

  const handleSkip = async (stepOrder: number) => {
    try {
      await skipStep.mutateAsync({ runId: run.id, stepOrder })
    } catch {
      toast.error('Failed to skip step')
    }
  }

  const handleRetry = async (stepOrder: number) => {
    try {
      await retryStep.mutateAsync({ runId: run.id, stepOrder })
    } catch {
      toast.error('Failed to retry step')
    }
  }

  const handleAbandon = async () => {
    try {
      await abandonRun.mutateAsync(run.id)
      toast.warning('Run abandoned')
    } catch {
      toast.error('Failed to abandon run')
    }
  }

  const statusBadge = () => {
    switch (run.status) {
      case 'completed':
        return <span className="rounded-full bg-green-500/20 px-2 py-0.5 text-xs text-green-400">Completed</span>
      case 'abandoned':
        return <span className="rounded-full bg-gray-500/20 px-2 py-0.5 text-xs text-gray-400">Abandoned</span>
      case 'escalated':
        return <span className="rounded-full bg-amber-500/20 px-2 py-0.5 text-xs text-amber-400">Escalated</span>
      default:
        return <span className="rounded-full bg-blue-500/20 px-2 py-0.5 text-xs text-blue-400">In Progress</span>
    }
  }

  return (
    <div className="space-y-6">
      <Link
        to={`/servers/${serverId}`}
        className="inline-flex items-center gap-1 text-sm text-pgp-text-secondary hover:text-pgp-text-primary"
      >
        <ArrowLeft className="h-4 w-4" /> Back to Server
      </Link>

      {/* Header */}
      <div className="flex items-start justify-between gap-4">
        <div>
          <div className="mb-1 flex items-center gap-2">
            <h1 className="text-xl font-semibold text-pgp-text-primary">
              {run.playbook_name}
            </h1>
            {statusBadge()}
          </div>
          <div className="flex items-center gap-4 text-xs text-pgp-text-muted">
            <span>Instance: {run.instance_id}</span>
            <span>Started by: {run.started_by}</span>
            <span className="flex items-center gap-1">
              <Clock className="h-3 w-3" />
              {elapsed}
            </span>
            <span>v{run.playbook_version}</span>
          </div>
        </div>

        {isActive && (
          <button
            onClick={handleAbandon}
            disabled={abandonRun.isPending}
            className="inline-flex items-center gap-1.5 rounded-md border border-pgp-border px-3 py-2 text-sm text-pgp-text-muted transition-colors hover:bg-pgp-bg-hover hover:text-pgp-text-primary disabled:opacity-50"
          >
            <XCircle className="h-4 w-4" />
            Abandon
          </button>
        )}
      </div>

      {/* Progress bar */}
      <RunProgressBar
        steps={stepDefs}
        runSteps={runSteps}
        currentStepOrder={run.current_step_order}
      />

      {/* Steps */}
      <div className="space-y-3">
        {stepDefs.map((step) => {
          const runStep = runSteps.find((rs) => rs.step_order === step.step_order)
          const isCurrent = isActive && step.step_order === run.current_step_order
          const isFuture = step.step_order > run.current_step_order && isActive

          // Skip rendering future steps as full cards — show them as gray list items
          if (isFuture) {
            return (
              <div
                key={step.step_order}
                className="flex items-center gap-3 rounded-lg border border-pgp-border bg-pgp-bg-card px-4 py-3 opacity-50"
              >
                <span className="flex h-7 w-7 items-center justify-center rounded-full bg-pgp-bg-hover text-xs font-bold text-pgp-text-muted">
                  {step.step_order}
                </span>
                <span className="text-sm text-pgp-text-muted">{step.name}</span>
              </div>
            )
          }

          return (
            <StepCard
              key={step.step_order}
              step={step}
              runStep={runStep}
              isCurrent={isCurrent}
              onExecute={() => handleExecute(step.step_order)}
              onConfirm={() => handleConfirm(step.step_order)}
              onApprove={() => handleApprove(step.step_order)}
              onRequestApproval={() => handleRequestApproval(step.step_order)}
              onSkip={() => handleSkip(step.step_order)}
              onRetry={() => handleRetry(step.step_order)}
              isExecuting={isExecuting}
            />
          )
        })}
      </div>

      {/* Feedback modal */}
      {showFeedback && (
        <FeedbackModal runId={run.id} onClose={() => setShowFeedback(false)} />
      )}
    </div>
  )
}
