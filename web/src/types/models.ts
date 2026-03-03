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
  id: number
  username: string
  role: UserRole
  active: boolean
  permissions?: string[]
}

export interface HealthResponse {
  status: string
  version: string
  uptime: string
}

// --- Instance API response types ---

export interface InstanceData {
  id: string
  name: string
  host: string
  port: number
  description: string
  metrics?: Record<string, number>
  alert_counts?: Record<string, number>
}

// --- Metrics API response types ---

export interface MetricValue {
  value: number
  labels?: Record<string, string>
}

export interface CurrentMetricsResult {
  instance_id: string
  collected_at: string
  metrics: Record<string, MetricValue>
}

export interface TimeSeriesPoint {
  t: string
  v: number
}

export interface HistoryResult {
  instance_id: string
  from: string
  to: string
  step?: string
  series: Record<string, TimeSeriesPoint[]>
}

// --- Replication API response types ---

export interface ReplicaLag {
  pending_bytes: number
  write_bytes: number
  flush_bytes: number
  replay_bytes: number
  write_lag?: string
  flush_lag?: string
  replay_lag?: string
}

export interface ReplicaInfo {
  client_addr: string
  application_name: string
  state: string
  sync_state: string
  lag: ReplicaLag
}

export interface SlotInfo {
  slot_name: string
  slot_type: string
  active: boolean
  wal_retained_bytes: number
}

export interface WALReceiverInfo {
  sender_host: string
  sender_port: number
  status: string
}

export interface ReplicationResponse {
  instance_id: string
  role: string
  replicas?: ReplicaInfo[]
  slots?: SlotInfo[]
  wal_receiver?: WALReceiverInfo | null
}

// --- Activity API response types ---

export interface WaitEvent {
  wait_event_type: string
  wait_event: string
  count: number
}

export interface WaitEventsResponse {
  events: WaitEvent[]
  total_backends: number
}

export interface LongTransaction {
  pid: number
  username: string
  database: string
  state: string
  waiting: boolean
  duration_seconds: number
  query: string
  xact_start: string
}

export interface LongTransactionsResponse {
  transactions: LongTransaction[]
}

// --- Alert API response types ---

export interface AlertEvent {
  rule_id: string
  rule_name: string
  instance_id: string
  severity: string
  value: number
  threshold: number
  operator: string
  metric: string
  labels: Record<string, string>
  channels: string[]
  fired_at: string
  resolved_at?: string
  is_resolution: boolean
}
