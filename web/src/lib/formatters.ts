export function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes === 0) return '0 B'
  if (bytes < 0) return `-${formatBytes(-bytes)}`
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const k = 1024
  if (bytes < 1) return `${bytes.toFixed(1)} B`
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  const idx = Math.min(i, units.length - 1)
  const value = bytes / Math.pow(k, idx)
  return `${value.toFixed(idx === 0 ? 0 : 1)} ${units[idx]}`
}

export function formatUptime(seconds: number): string {
  const days = Math.floor(seconds / 86400)
  const hours = Math.floor((seconds % 86400) / 3600)
  const minutes = Math.floor((seconds % 3600) / 60)

  const parts: string[] = []
  if (days > 0) parts.push(`${days}d`)
  if (hours > 0 || days > 0) parts.push(`${hours}h`)
  parts.push(`${minutes}m`)
  return parts.join(' ')
}

export function formatDuration(seconds: number): string {
  if (seconds < 1) return '< 1s'
  const h = Math.floor(seconds / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  const s = Math.floor(seconds % 60)

  const parts: string[] = []
  if (h > 0) parts.push(`${h}h`)
  if (m > 0) parts.push(`${m}m`)
  if (s > 0 || parts.length === 0) parts.push(`${s}s`)
  return parts.join(' ')
}

export function formatPercent(value: number, decimals = 1): string {
  return `${value.toFixed(decimals)}%`
}

export function formatPGVersion(versionNum: number): string {
  const major = Math.floor(versionNum / 10000)
  const minor = versionNum % 100
  return `${major}.${minor}`
}

export function formatTimestamp(isoString: string): string {
  const date = new Date(isoString)
  const month = date.toLocaleString('en-US', { month: 'short' })
  const day = date.getDate()
  const h = String(date.getHours()).padStart(2, '0')
  const m = String(date.getMinutes()).padStart(2, '0')
  const s = String(date.getSeconds()).padStart(2, '0')
  return `${month} ${day}, ${h}:${m}:${s}`
}

/**
 * Parse a Go duration string like "4h24m36.79747s" into a human-friendly form.
 * Examples: "4h24m36.79747s" -> "4h 25m", "30m0s" -> "30m", "5m30s" -> "5m 30s", "45.123s" -> "45s"
 * Always drops sub-second precision.
 */
export function formatDurationHuman(goStr: string): string {
  const match = goStr.match(/(?:(\d+)h)?(?:(\d+)m)?(?:([\d.]+)s)?/)
  if (!match) return goStr
  const h = match[1] ? parseInt(match[1]) : 0
  const m = match[2] ? parseInt(match[2]) : 0
  const s = match[3] ? Math.round(parseFloat(match[3])) : 0

  const parts: string[] = []
  if (h > 0) {
    const roundedM = s >= 30 ? m + 1 : m
    parts.push(`${h}h`)
    if (roundedM > 0) parts.push(`${roundedM}m`)
  } else if (m > 0) {
    parts.push(`${m}m`)
    if (s > 0) parts.push(`${s}s`)
  } else {
    parts.push(`${s}s`)
  }
  return parts.join(' ')
}

export function thresholdColor(
  value: number,
  warningThreshold: number,
  criticalThreshold: number,
  inverse = false,
): 'ok' | 'warning' | 'critical' {
  if (inverse) {
    if (value <= criticalThreshold) return 'critical'
    if (value <= warningThreshold) return 'warning'
    return 'ok'
  }
  if (value >= criticalThreshold) return 'critical'
  if (value >= warningThreshold) return 'warning'
  return 'ok'
}
