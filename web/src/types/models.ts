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

// --- Instance Management API types ---

export interface ManagedInstance {
  id: string
  name: string
  dsn_masked: string
  host: string
  port: number
  enabled: boolean
  source: 'yaml' | 'manual'
  max_conns: number
  created_at: string
  updated_at: string
}

export interface CreateInstanceRequest {
  id?: string
  name: string
  dsn: string
  enabled: boolean
  max_conns?: number
}

export interface UpdateInstanceRequest {
  name?: string
  dsn?: string
  enabled?: boolean
  max_conns?: number
}

export interface TestConnectionResult {
  success: boolean
  version?: string
  error?: string
  latency_ms?: number
}

export interface BulkImportResult {
  row: number
  id?: string
  success: boolean
  error?: string
}

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

// --- OS Metrics API response types (M6) ---

export interface OSMetrics {
  collected_at: string
  hostname: string
  os_release: { name: string; version: string; id: string }
  uptime_seconds: number
  load_avg: { '1m': number; '5m': number; '15m': number }
  memory: {
    total_kb: number
    available_kb: number
    used_kb: number
    commit_limit_kb: number
    committed_as_kb: number
  }
  cpu: {
    user_pct: number
    system_pct: number
    iowait_pct: number
    idle_pct: number
    num_cpus: number
  }
  disks: OSDiskInfo[]
  diskstats: OSDiskStatInfo[]
}

export interface OSDiskInfo {
  mount: string
  device: string
  fstype: string
  total_bytes: number
  used_bytes: number
  free_bytes: number
  inodes_total: number
  inodes_used: number
}

export interface OSDiskStatInfo {
  device: string
  reads_completed: number
  writes_completed: number
  read_kb: number
  write_kb: number
  io_in_progress: number
  read_await_ms: number
  write_await_ms: number
  util_pct: number
}

// --- Cluster Metrics API response types (M6) ---

export interface ClusterMetrics {
  patroni: PatroniClusterState | null
  etcd: ETCDState | null
}

export interface PatroniClusterState {
  cluster_name: string
  members: PatroniMember[]
}

export interface PatroniMember {
  name: string
  host: string
  port: number
  role: string
  state: string
  timeline: number
  lag: number
}

export interface ETCDState {
  members: ETCDMember[]
  health: Record<string, boolean>
}

export interface ETCDMember {
  id: string
  name: string
  peer_url: string
  client_url: string
  is_leader: boolean
  status: string
  db_size: number
}

// --- Per-Database Analysis types (M7) ---

export interface DatabaseSummary {
  name: string
  large_object_count: number
  dead_tuples: number
  unused_indexes: number
  max_bloat_ratio: number
  last_collected?: string
}

export interface TableMetric {
  schema: string
  table: string
  total_bytes: number
  table_bytes: number
  bloat_ratio?: number
  wasted_bytes?: number
}

export interface IndexMetric {
  schema: string
  table: string
  index: string
  scan_count: number
  tup_read?: number
  cache_hit_pct?: number
  unused?: boolean
  unused_bytes?: number
  bloat_ratio?: number
  wasted_bytes?: number
}

export interface VacuumMetric {
  schema: string
  table: string
  dead_tuples: number
  dead_pct: number
  autovacuum_age_sec?: number
  autoanalyze_age_sec?: number
}

export interface SchemaMetric {
  schema: string
  size_bytes: number
}

export interface SequenceMetric {
  schema: string
  sequence: string
  last_value: number
}

export interface FunctionMetric {
  schema: string
  function: string
  calls: number
  total_time_ms: number
  self_time_ms: number
}

export interface CatalogMetric {
  table: string
  size_bytes: number
}

export interface DatabaseMetrics {
  database_name: string
  collected_at: string
  tables: TableMetric[]
  indexes: IndexMetric[]
  vacuum: VacuumMetric[]
  schemas: SchemaMetric[]
  sequences: SequenceMetric[]
  functions: FunctionMetric[]
  catalogs: CatalogMetric[]
  large_object_count: number
  large_object_size_bytes: number
  unused_index_count: number
  unlogged_count: number
  partition_count: number
}
