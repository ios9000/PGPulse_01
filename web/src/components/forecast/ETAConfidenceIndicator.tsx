interface ETAConfidenceIndicatorProps {
  confidence: 'high' | 'medium' | 'estimating' | 'stalled'
}

const CONFIDENCE_STYLES: Record<string, string> = {
  high: 'bg-green-500',
  medium: 'bg-yellow-500',
  estimating: 'bg-slate-400 animate-pulse',
  stalled: 'bg-red-500',
}

export function ETAConfidenceIndicator({ confidence }: ETAConfidenceIndicatorProps) {
  return (
    <span
      className={`inline-block h-2 w-2 rounded-full ${CONFIDENCE_STYLES[confidence] ?? 'bg-slate-400'}`}
      title={`Confidence: ${confidence}`}
    />
  )
}
