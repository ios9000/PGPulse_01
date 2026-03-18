import { useState } from 'react'
import { Copy, Check, ChevronDown, ChevronRight } from 'lucide-react'

interface QueryTextProps {
  query: string
  maxLength?: number
  className?: string
}

export function QueryText({ query, maxLength = 120, className = '' }: QueryTextProps) {
  const [expanded, setExpanded] = useState(false)
  const [copied, setCopied] = useState(false)

  const truncated = query.length > maxLength && !expanded

  const handleCopy = async (e: React.MouseEvent) => {
    e.stopPropagation()
    try {
      await navigator.clipboard.writeText(query)
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    } catch {
      // Clipboard API may fail in some contexts
    }
  }

  const handleToggle = (e: React.MouseEvent) => {
    e.stopPropagation()
    setExpanded(!expanded)
  }

  return (
    <div className={`group relative ${className}`}>
      <pre className="whitespace-pre-wrap break-all font-mono text-xs leading-relaxed text-pgp-text-primary">
        {truncated ? query.slice(0, maxLength) + '...' : query}
      </pre>
      <div className="mt-1 flex items-center gap-2">
        {query.length > maxLength && (
          <button
            onClick={handleToggle}
            className="flex items-center gap-1 text-xs text-pgp-text-muted hover:text-pgp-accent"
          >
            {expanded ? (
              <ChevronDown className="h-3 w-3" />
            ) : (
              <ChevronRight className="h-3 w-3" />
            )}
            {expanded ? 'Less' : 'More'}
          </button>
        )}
        <button
          onClick={handleCopy}
          className="flex items-center gap-1 text-xs text-pgp-text-muted hover:text-pgp-accent"
          title="Copy query"
        >
          {copied ? <Check className="h-3 w-3" /> : <Copy className="h-3 w-3" />}
          {copied ? 'Copied' : 'Copy'}
        </button>
      </div>
    </div>
  )
}
