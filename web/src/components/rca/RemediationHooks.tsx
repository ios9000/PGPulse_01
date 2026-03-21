import { Wrench } from 'lucide-react'

interface RemediationHooksProps {
  hooks: string[]
}

function formatHook(hook: string): string {
  return hook
    .replace(/^remediation\./, '')
    .replace(/_/g, ' ')
    .replace(/\b\w/g, (c) => c.toUpperCase())
}

export function RemediationHooks({ hooks }: RemediationHooksProps) {
  if (!hooks || hooks.length === 0) return null

  return (
    <div className="rounded-lg border border-pgp-border bg-pgp-bg-card p-4">
      <h3 className="mb-3 flex items-center gap-2 text-xs font-semibold uppercase tracking-wider text-pgp-text-muted">
        <Wrench className="h-4 w-4" />
        Recommended Actions
      </h3>
      <ul className="space-y-2">
        {hooks.map((hook) => (
          <li
            key={hook}
            className="flex items-center gap-2 text-sm text-pgp-text-secondary"
          >
            <span className="h-1.5 w-1.5 shrink-0 rounded-full bg-pgp-accent" />
            {formatHook(hook)}
          </li>
        ))}
      </ul>
    </div>
  )
}
