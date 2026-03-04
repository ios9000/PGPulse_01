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

export type AlertSeverityFilter = 'all' | 'warning' | 'critical'
export type AlertStateFilter = 'all' | 'firing' | 'resolved'

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

export interface AlertRule {
  id: string
  name: string
  description?: string
  metric: string
  operator: '>' | '>=' | '<' | '<=' | '==' | '!='
  threshold: number
  severity: 'info' | 'warning' | 'critical'
  labels?: Record<string, string>
  consecutive_count: number
  cooldown_minutes: number
  channels?: string[]
  source: 'builtin' | 'custom'
  enabled: boolean
}

// --- Statements API response types ---

export interface StatementsConfig {
  max: number
  track: string
  io_timing: boolean
  current_count: number
  fill_pct: number
  stats_reset: string | null
  stats_reset_age_seconds: number | null
}

export interface StatementEntry {
  queryid: number
  query_text: string
  dbname: string
  username: string
  calls: number
  total_exec_time_ms: number
  mean_exec_time_ms: number
  rows: number
  blk_read_time_ms: number
  blk_write_time_ms: number
  io_time_ms: number
  cpu_time_ms: number
  shared_blks_hit: number
  shared_blks_read: number
  hit_ratio: number
  pct_of_total_time: number
}

export interface StatementsResponse {
  config: StatementsConfig
  statements: StatementEntry[]
}

export type StatementSortField = 'total_time' | 'io_time' | 'cpu_time' | 'calls' | 'rows'

// --- Lock Tree API response types ---

export interface LockTreeSummary {
  root_blockers: number
  total_blocked: number
  max_depth: number
}

export interface LockEntry {
  pid: number
  depth: number
  usename: string
  datname: string
  state: string
  wait_event_type: string | null
  wait_event: string | null
  duration_seconds: number
  query: string
  blocked_by_count: number
  blocking_count: number
  is_root: boolean
  parent_pid: number | null
}

export interface LockTreeResponse {
  summary: LockTreeSummary
  locks: LockEntry[]
}

// --- Progress API response types ---

export interface ProgressOperation {
  operation_type: 'vacuum' | 'analyze' | 'create_index' | 'cluster' | 'basebackup' | 'copy'
  pid: number
  datname: string
  relname: string | null
  phase: string
  progress_pct: number | null
  duration_seconds: number
  details: Record<string, unknown>
}

export interface ProgressResponse {
  operations: ProgressOperation[]
}
