import type { PlaybookRunStep, PlaybookStep } from '@/types/playbook'

interface RunProgressBarProps {
  steps: PlaybookStep[]
  runSteps: PlaybookRunStep[]
  currentStepOrder: number
}

function stepStatusClass(
  runStep: PlaybookRunStep | undefined,
  stepOrder: number,
  currentStepOrder: number,
): string {
  if (runStep?.status === 'completed') return 'bg-green-500'
  if (runStep?.status === 'skipped') return 'bg-gray-500'
  if (runStep?.status === 'failed') return 'bg-red-500'
  if (stepOrder === currentStepOrder) return 'bg-pgp-accent animate-pulse'
  return 'bg-pgp-border'
}

export function RunProgressBar({ steps, runSteps, currentStepOrder }: RunProgressBarProps) {
  const total = steps.length
  const completed = runSteps.filter(
    (rs) => rs.status === 'completed' || rs.status === 'skipped',
  ).length

  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between text-xs text-pgp-text-muted">
        <span>
          Step {currentStepOrder} of {total}
        </span>
        <span>
          {completed} completed
        </span>
      </div>
      <div className="flex items-center gap-1.5">
        {steps.map((step) => {
          const runStep = runSteps.find((rs) => rs.step_order === step.step_order)
          return (
            <div
              key={step.step_order}
              className="flex flex-1 flex-col items-center gap-1"
            >
              <div
                className={`h-2.5 w-2.5 rounded-full ${stepStatusClass(runStep, step.step_order, currentStepOrder)}`}
                title={`Step ${step.step_order}: ${step.name}`}
              />
              {total <= 8 && (
                <span className="text-[10px] text-pgp-text-muted truncate max-w-full">
                  {step.step_order}
                </span>
              )}
            </div>
          )
        })}
      </div>
    </div>
  )
}
