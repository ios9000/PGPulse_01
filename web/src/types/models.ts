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
  application_name: string
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
  id: number
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
  recommendations?: Recommendation[]
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

// --- Session Kill types (M8_01) ---

export interface SessionKillResult {
  ok: boolean
  error?: string
  note?: string
}

// --- Query Plan types (M8_01) ---

export interface ExplainRequest {
  database: string
  query: string
  analyze: boolean
  buffers: boolean
}

export interface ExplainResponse {
  plan_json: PlanNode[]
  execution_time_ms?: number
  planning_time_ms?: number
}

export interface PlanNode {
  'Node Type': string
  'Relation Name'?: string
  'Alias'?: string
  'Startup Cost': number
  'Total Cost': number
  'Plan Rows': number
  'Actual Rows'?: number
  'Actual Total Time'?: number
  'Shared Hit Blocks'?: number
  'Shared Read Blocks'?: number
  Plans?: PlanNode[]
  [key: string]: unknown
}

// --- Settings Diff types (M8_01) ---

export interface SettingEntry {
  name: string
  category: string
  value_a?: string
  value_b?: string
  unit?: string
}

export interface SettingsDiffResponse {
  instance_a: { id: string; name: string }
  instance_b: { id: string; name: string }
  changed: SettingEntry[]
  only_in_a: SettingEntry[]
  only_in_b: SettingEntry[]
  matching_count: number
}

// --- Logical Replication types (M8_08) ---

export interface LogicalReplicationResponse {
  subscriptions: SubscriptionStatus[]
  total_pending_tables: number
}

export interface SubscriptionStatus {
  database: string
  subscription_name: string
  tables_pending: PendingTable[]
  stats?: SubscriptionStats
}

export interface PendingTable {
  table_name: string
  sync_state: string
  sync_state_label: string
  sync_lsn: string
}

export interface SubscriptionStats {
  pid: number
  received_lsn: string
  latest_end_lsn: string
  latest_end_time: string
  apply_error_count?: number
  sync_error_count?: number
}

// --- Remediation / Advisor types (REM_01b) ---

export type RecommendationPriority = 'info' | 'suggestion' | 'action_required'
export type RecommendationCategory = 'performance' | 'capacity' | 'configuration' | 'replication' | 'maintenance'

export type RecommendationStatus = 'active' | 'resolved' | 'acknowledged'

export interface Recommendation {
  id: number
  rule_id: string
  instance_id: string
  alert_event_id?: number
  metric_key: string
  metric_value: number
  priority: RecommendationPriority
  category: RecommendationCategory
  status: RecommendationStatus
  title: string
  description: string
  doc_url?: string
  created_at: string
  evaluated_at: string
  resolved_at: string | null
  acknowledged_at?: string
  acknowledged_by?: string
  source: 'background' | 'rca' | 'alert' | 'forecast'
  urgency_score: number
  incident_ids: number[]
  last_incident_at?: string
}

export interface DiagnoseResponse {
  recommendations: Recommendation[]
  metrics_evaluated: number
  rules_evaluated: number
}

export interface RemediationRule {
  id: string
  priority: RecommendationPriority
  category: RecommendationCategory
}

// --- PGSS Snapshot types (M11_02) ---

export interface PGSSSnapshot {
  id: number
  instance_id: string
  captured_at: string
  pg_version: number
  stats_reset?: string
  total_statements: number
  total_calls: number
  total_exec_time_ms: number
}

export interface PGSSSnapshotEntry {
  snapshot_id: number
  queryid: number
  userid: number
  dbid: number
  database_name?: string
  user_name?: string
  query: string
  calls: number
  total_exec_time_ms: number
  total_plan_time_ms?: number
  rows: number
  shared_blks_hit: number
  shared_blks_read: number
  shared_blks_dirtied: number
  shared_blks_written: number
  local_blks_hit: number
  local_blks_read: number
  temp_blks_read: number
  temp_blks_written: number
  blk_read_time_ms: number
  blk_write_time_ms: number
  wal_records?: number
  wal_fpi?: number
  wal_bytes?: number
  mean_exec_time_ms?: number
  min_exec_time_ms?: number
  max_exec_time_ms?: number
  stddev_exec_time_ms?: number
}

export interface PGSSDiffResult {
  from_snapshot: PGSSSnapshot
  to_snapshot: PGSSSnapshot
  stats_reset_warning: boolean
  duration: number
  total_calls_delta: number
  total_exec_time_delta_ms: number
  entries: PGSSDiffEntry[]
  new_queries: PGSSDiffEntry[]
  evicted_queries: PGSSDiffEntry[]
  total_entries: number
}

export interface PGSSDiffEntry {
  queryid: number
  userid: number
  dbid: number
  query: string
  database_name?: string
  user_name?: string
  calls_delta: number
  exec_time_delta_ms: number
  plan_time_delta_ms?: number
  rows_delta: number
  shared_blks_read_delta: number
  shared_blks_hit_delta: number
  temp_blks_read_delta: number
  temp_blks_written_delta: number
  blk_read_time_delta_ms: number
  blk_write_time_delta_ms: number
  wal_bytes_delta?: number
  avg_exec_time_per_call_ms: number
  io_time_pct: number
  cpu_time_delta_ms: number
  shared_hit_ratio_pct: number
}

export interface PGSSQueryInsight {
  queryid: number
  query: string
  database_name: string
  user_name: string
  first_seen: string
  points: PGSSQueryInsightPoint[]
}

export interface PGSSQueryInsightPoint {
  captured_at: string
  calls_delta: number
  exec_time_delta_ms: number
  rows_delta: number
  avg_exec_time_ms: number
  shared_hit_ratio_pct: number
}

export interface PGSSWorkloadReport {
  instance_id: string
  from_time: string
  to_time: string
  duration: string
  stats_reset_warning: boolean
  summary: PGSSReportSummary
  top_by_exec_time: PGSSDiffEntry[]
  top_by_calls: PGSSDiffEntry[]
  top_by_rows: PGSSDiffEntry[]
  top_by_io_reads: PGSSDiffEntry[]
  top_by_avg_time: PGSSDiffEntry[]
  new_queries: PGSSDiffEntry[]
  evicted_queries: PGSSDiffEntry[]
}

export interface PGSSReportSummary {
  total_queries: number
  total_calls_delta: number
  total_exec_time_delta_ms: number
  total_rows_delta: number
  unique_queries: number
  new_queries: number
  evicted_queries: number
}

export interface PGSSSnapshotListResponse {
  snapshots: PGSSSnapshot[]
  total: number
}
