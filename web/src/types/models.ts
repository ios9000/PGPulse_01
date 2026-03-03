export interface Server {
  id: string
  name: string
  host: string
  port: number
  status: 'online' | 'offline' | 'degraded' | 'unknown'
  pg_version?: string
  is_primary?: boolean
}

export interface Database {
  name: string
  server_id: string
  size_bytes: number
  cache_hit_ratio?: number
  connections: number
}

export type AlertSeverity = 'critical' | 'warning' | 'info'
export type AlertState = 'firing' | 'pending' | 'resolved'

export interface Alert {
  id: string
  rule_slug: string
  severity: AlertSeverity
  state: AlertState
  message: string
  instance_id: string
  fired_at: string
  resolved_at?: string
}

export type UserRole = 'super_admin' | 'roles_admin' | 'dba' | 'app_admin'

export interface User {
  id: string
  username: string
  role: UserRole
  email?: string
}

export interface HealthResponse {
  status: string
  version: string
  uptime: string
}
