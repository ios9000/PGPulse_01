import { ArrowRight } from 'lucide-react'

interface BranchIndicatorProps {
  targetStep: number
  label: string
}

export function BranchIndicator({ targetStep, label }: BranchIndicatorProps) {
  return (
    <div className="flex items-center gap-1.5 rounded-md bg-blue-500/10 px-2.5 py-1 text-xs text-blue-400">
      <ArrowRight className="h-3 w-3" />
      <span>
        Jumped to Step {targetStep}: {label}
      </span>
    </div>
  )
}
