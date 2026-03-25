import { useNavigate } from 'react-router-dom'
import { Play, Loader2 } from 'lucide-react'
import { useResolvePlaybook, useStartRun } from '@/hooks/usePlaybooks'
import { toast } from '@/stores/toastStore'

interface ResolverButtonProps {
  hook?: string
  rootCause?: string
  metric?: string
  adviserRule?: string
  instanceId: string
  triggerSource?: string
  triggerId?: string
  variant?: 'button' | 'compact'
}

export function ResolverButton({
  hook,
  rootCause,
  metric,
  adviserRule,
  instanceId,
  triggerSource,
  triggerId,
  variant = 'button',
}: ResolverButtonProps) {
  const navigate = useNavigate()
  const { data: resolved, isLoading: resolving } = useResolvePlaybook(
    hook || rootCause || metric || adviserRule
      ? { hook, root_cause: rootCause, metric, adviser_rule: adviserRule, instance_id: instanceId }
      : undefined,
  )
  const startRun = useStartRun()

  if (resolving) {
    return null
  }

  if (!resolved?.playbook) {
    return null
  }

  const playbook = resolved.playbook

  const handleClick = async () => {
    try {
      const run = await startRun.mutateAsync({
        instanceId,
        playbookId: playbook.id,
        triggerSource,
        triggerId,
      })
      navigate(`/servers/${instanceId}/playbook-runs/${run.id}`)
    } catch {
      toast.error('Failed to start playbook run')
    }
  }

  if (variant === 'compact') {
    return (
      <button
        onClick={(e) => {
          e.stopPropagation()
          handleClick()
        }}
        disabled={startRun.isPending}
        className="inline-flex items-center gap-1.5 rounded-md border border-pgp-border bg-pgp-bg-secondary px-2 py-1 text-xs text-pgp-text-secondary hover:bg-pgp-bg-hover hover:text-pgp-text-primary disabled:opacity-50"
      >
        {startRun.isPending ? (
          <Loader2 className="h-3 w-3 animate-spin" />
        ) : (
          <Play className="h-3 w-3" />
        )}
        Remediate
      </button>
    )
  }

  return (
    <button
      onClick={handleClick}
      disabled={startRun.isPending}
      className="inline-flex items-center gap-2 rounded-md border border-pgp-border px-3 py-2 text-sm text-pgp-text-secondary transition-colors hover:bg-pgp-bg-hover hover:text-pgp-text-primary disabled:opacity-50"
    >
      {startRun.isPending ? (
        <Loader2 className="h-4 w-4 animate-spin" />
      ) : (
        <Play className="h-4 w-4" />
      )}
      Run Playbook: {playbook.name}
    </button>
  )
}
