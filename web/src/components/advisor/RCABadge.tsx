import { useNavigate } from 'react-router-dom'
import { Link2 } from 'lucide-react'

interface RCABadgeProps {
  incidentIds: number[]
  lastIncidentAt?: string
}

export function RCABadge({ incidentIds, lastIncidentAt }: RCABadgeProps) {
  const navigate = useNavigate()

  if (incidentIds.length === 0) return null

  const latestId = incidentIds[incidentIds.length - 1]

  return (
    <button
      onClick={(e) => {
        e.stopPropagation()
        navigate(`/rca/incidents/${latestId}`)
      }}
      className="inline-flex items-center gap-1 rounded-full bg-purple-500/20 px-2 py-0.5 text-xs font-medium text-purple-400 hover:bg-purple-500/30"
      title={lastIncidentAt ? `Last incident: ${lastIncidentAt}` : undefined}
    >
      <Link2 className="h-3 w-3" />
      Linked to {incidentIds.length} incident{incidentIds.length === 1 ? '' : 's'}
    </button>
  )
}
