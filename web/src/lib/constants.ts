export const STATUS_COLORS = {
  ok: 'text-pgp-ok',
  warning: 'text-pgp-warning',
  critical: 'text-pgp-critical',
  info: 'text-pgp-info',
  unknown: 'text-gray-500',
} as const

export const STATUS_BG_COLORS = {
  ok: 'bg-pgp-ok',
  warning: 'bg-pgp-warning',
  critical: 'bg-pgp-critical',
  info: 'bg-pgp-info',
  unknown: 'bg-gray-500',
} as const

export const SIDEBAR_WIDTH = 240
export const SIDEBAR_COLLAPSED_WIDTH = 64

export const POLLING_INTERVALS = {
  health: 30_000,
  metrics: 10_000,
  alerts: 15_000,
} as const
