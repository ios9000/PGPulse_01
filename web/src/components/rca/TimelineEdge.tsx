import { ArrowDown } from 'lucide-react'

interface TimelineEdgeProps {
  description: string
}

export function TimelineEdge({ description }: TimelineEdgeProps) {
  if (!description) return null

  return (
    <div className="relative flex gap-4">
      <div className="flex flex-col items-center">
        <div className="w-px flex-1 border-l border-dashed border-pgp-text-muted/30" />
        <ArrowDown className="my-1 h-3 w-3 text-pgp-text-muted" />
        <div className="w-px flex-1 border-l border-dashed border-pgp-text-muted/30" />
      </div>
      <div className="flex items-center py-1">
        <p className="text-xs italic text-pgp-text-muted">{description}</p>
      </div>
    </div>
  )
}
