# MN_01 — Metric Naming Standardization: Design

**Date:** 2026-03-13
**Iteration:** MN_01

---

## 1. Root Cause Analysis

The current naming inconsistency has one root cause: `Base.point()` in the collector
base struct. Every instance-level collector embeds `Base` and calls
`point("connections.active")` which returns `"pgpulse.connections.active"`. The
competitive research established that `pgpulse.` should be reserved for internal/meta
metrics, while PostgreSQL metrics should use `pg.`.

### Current Prefix Behavior

| Collector Type | Current Mechanism | Current Prefix | Target Prefix | Change? |
|---------------|-------------------|----------------|---------------|---------|
| Instance-level (connections, cache, etc.) | `Base.point()` | `pgpulse.` | `pg.` | YES |
| OSSQLCollector | `Base.point()` | `pgpulse.os.*` | `os.*` | YES |
| OSCollector (agent) | Direct string | `os.*` | `os.*` | NO |
| ClusterCollector | Direct string | `cluster.*` | `cluster.*` | NO (D200) |
| DatabaseCollector (per-db) | `Base.point()` | `pgpulse.db.*` | `pg.db.*` | YES (auto) |

---

## 2. Implementation Strategy

Change `Base.point()` from prepending `pgpulse.` to prepending `pg.`.
This fixes ~120 keys in one line. Then handle two exceptions:

1. **OSSQLCollector** — must bypass `Base.point()` and emit `os.*` directly
2. **Disk metrics** — rename `diskstat` to `disk` in both agent and SQL paths

ClusterCollector needs no change (D200).

---

## 3. Complete Metric Key Mapping Table

### 3.1 Cluster Metrics (8 keys) — NO CHANGE (D200)

| Key | Status |
|-----|--------|
| cluster.etcd.leader_count | Keep |
| cluster.etcd.member_count | Keep |
| cluster.etcd.member_healthy | Keep |
| cluster.patroni.leader_count | Keep |
| cluster.patroni.member_count | Keep |
| cluster.patroni.member_lag_bytes | Keep |
| cluster.patroni.member_replica_count | Keep |
| cluster.patroni.member_state | Keep |

### 3.2 OS Metrics — diskstat hierarchy rename (7 keys)

These keys exist in BOTH `os_sql.go` (SQL path) and `os.go` (agent path).
The SQL path additionally has the `pgpulse.` prefix bug (fixed by making
OSSQLCollector bypass `Base.point()`).

| # | Current Key | New Key | Files |
|---|------------|---------|-------|
| 1 | os.diskstat.read_kb | os.disk.read_bytes_per_sec | os_sql.go, os.go, agent/osmetrics_linux.go |
| 2 | os.diskstat.write_kb | os.disk.write_bytes_per_sec | os_sql.go, os.go, agent/osmetrics_linux.go |
| 3 | os.diskstat.reads_completed | os.disk.reads_completed | os_sql.go, os.go, agent/osmetrics_linux.go |
| 4 | os.diskstat.writes_completed | os.disk.writes_completed | os_sql.go, os.go, agent/osmetrics_linux.go |
| 5 | os.diskstat.read_await_ms | os.disk.read_await_ms | os_sql.go, os.go, agent/osmetrics_linux.go |
| 6 | os.diskstat.write_await_ms | os.disk.write_await_ms | os_sql.go, os.go, agent/osmetrics_linux.go |
| 7 | os.diskstat.util_pct | os.disk.util_pct | os_sql.go, os.go, agent/osmetrics_linux.go |

**VERIFY:** Check `ParseDiskStats()` in `agent/osmetrics_linux.go`. If `read_kb`
returns kilobytes (not bytes/sec), multiply by 1024 when renaming to
`read_bytes_per_sec`. If it already returns bytes/sec, only the name changes.

### 3.3 OS Metrics — unchanged keys (18 keys)

Already conform to `os.*` standard. Only change: OSSQLCollector must stop
prepending `pgpulse.` (automatic once it bypasses `Base.point()`).

| Key | Notes |
|-----|-------|
| os.cpu.idle_pct | Keep |
| os.cpu.iowait_pct | Keep |
| os.cpu.system_pct | Keep |
| os.cpu.user_pct | Keep |
| os.disk.free_bytes | Keep (agent-only, filesystem level) |
| os.disk.inodes_total | Keep (agent-only) |
| os.disk.inodes_used | Keep (agent-only) |
| os.disk.total_bytes | Keep (agent-only) |
| os.disk.used_bytes | Keep (agent-only) |
| os.load.1m | Keep |
| os.load.5m | Keep |
| os.load.15m | Keep |
| os.memory.available_kb | Keep (unit rename deferred) |
| os.memory.commit_limit_kb | Keep |
| os.memory.committed_as_kb | Keep |
| os.memory.total_kb | Keep |
| os.memory.used_kb | Keep |
| os.uptime_seconds | Keep |

### 3.4 PG Metrics — bulk rename via Base.point() (~122 keys)

All change from `pgpulse.{rest}` → `pg.{rest}`. Automatic once `Base.point()`
prefix is updated.

#### Background Writer / Checkpoint (20 keys)

| Current Key | New Key |
|------------|---------|
| pgpulse.bgwriter.buffers_alloc | pg.bgwriter.buffers_alloc |
| pgpulse.bgwriter.buffers_alloc_per_second | pg.bgwriter.buffers_alloc_per_second |
| pgpulse.bgwriter.buffers_backend | pg.bgwriter.buffers_backend |
| pgpulse.bgwriter.buffers_backend_fsync | pg.bgwriter.buffers_backend_fsync |
| pgpulse.bgwriter.buffers_backend_per_second | pg.bgwriter.buffers_backend_per_second |
| pgpulse.bgwriter.buffers_clean | pg.bgwriter.buffers_clean |
| pgpulse.bgwriter.buffers_clean_per_second | pg.bgwriter.buffers_clean_per_second |
| pgpulse.bgwriter.maxwritten_clean | pg.bgwriter.maxwritten_clean |
| pgpulse.checkpoint.buffers_written | pg.checkpoint.buffers_written |
| pgpulse.checkpoint.buffers_written_per_second | pg.checkpoint.buffers_written_per_second |
| pgpulse.checkpoint.requested | pg.checkpoint.requested |
| pgpulse.checkpoint.requested_per_second | pg.checkpoint.requested_per_second |
| pgpulse.checkpoint.restartpoints_done | pg.checkpoint.restartpoints_done |
| pgpulse.checkpoint.restartpoints_req | pg.checkpoint.restartpoints_req |
| pgpulse.checkpoint.restartpoints_timed | pg.checkpoint.restartpoints_timed |
| pgpulse.checkpoint.sync_time_ms | pg.checkpoint.sync_time_ms |
| pgpulse.checkpoint.timed | pg.checkpoint.timed |
| pgpulse.checkpoint.timed_per_second | pg.checkpoint.timed_per_second |
| pgpulse.checkpoint.write_time_ms | pg.checkpoint.write_time_ms |

#### Cache (1 key)

| Current Key | New Key |
|------------|---------|
| pgpulse.cache.hit_ratio | pg.cache.hit_ratio |

Note: MW_01b renamed `cache.hit_ratio_pct` → `cache.hit_ratio` in collector code.
The digest may still show `_pct` — verify in cache.go. The migration script
should handle both variants.

#### Connections (5 keys)

| Current Key | New Key |
|------------|---------|
| pgpulse.connections.by_state | pg.connections.by_state |
| pgpulse.connections.max | pg.connections.max |
| pgpulse.connections.superuser_reserved | pg.connections.superuser_reserved |
| pgpulse.connections.total | pg.connections.total |
| pgpulse.connections.utilization_pct | pg.connections.utilization_pct |

#### Database Sizes (1 key)

| Current Key | New Key |
|------------|---------|
| pgpulse.database.size_bytes | pg.database.size_bytes |

#### Per-Database Metrics (41 keys)

| Current Key | New Key |
|------------|---------|
| pgpulse.db.autovacuum.enabled | pg.db.autovacuum.enabled |
| pgpulse.db.bloat.index_ratio | pg.db.bloat.index_ratio |
| pgpulse.db.bloat.index_wasted_bytes | pg.db.bloat.index_wasted_bytes |
| pgpulse.db.bloat.table_ratio | pg.db.bloat.table_ratio |
| pgpulse.db.bloat.table_wasted_bytes | pg.db.bloat.table_wasted_bytes |
| pgpulse.db.catalog.size_bytes | pg.db.catalog.size_bytes |
| pgpulse.db.functions.calls | pg.db.functions.calls |
| pgpulse.db.functions.self_time_ms | pg.db.functions.self_time_ms |
| pgpulse.db.functions.total_time_ms | pg.db.functions.total_time_ms |
| pgpulse.db.index.scans | pg.db.index.scans |
| pgpulse.db.index.size_bytes | pg.db.index.size_bytes |
| pgpulse.db.index.tuples_fetched | pg.db.index.tuples_fetched |
| pgpulse.db.index.tuples_read | pg.db.index.tuples_read |
| pgpulse.db.index.unused_scans | pg.db.index.unused_scans |
| pgpulse.db.index.unused_size_bytes | pg.db.index.unused_size_bytes |
| pgpulse.db.large_objects.count | pg.db.large_objects.count |
| pgpulse.db.large_objects.total_bytes | pg.db.large_objects.total_bytes |
| pgpulse.db.large_objects.total_count | pg.db.large_objects.total_count |
| pgpulse.db.logical_replication.pending_sync_tables | pg.db.logical_replication.pending_sync_tables |
| pgpulse.db.partition.child_bytes | pg.db.partition.child_bytes |
| pgpulse.db.schema.size_bytes | pg.db.schema.size_bytes |
| pgpulse.db.sequences.last_value | pg.db.sequences.last_value |
| pgpulse.db.sequences.pct_used | pg.db.sequences.pct_used |
| pgpulse.db.table.cache_hit_pct | pg.db.table.cache_hit_pct |
| pgpulse.db.table.data_bytes | pg.db.table.data_bytes |
| pgpulse.db.table.heap_blks_hit | pg.db.table.heap_blks_hit |
| pgpulse.db.table.heap_blks_read | pg.db.table.heap_blks_read |
| pgpulse.db.table.idx_blks_hit | pg.db.table.idx_blks_hit |
| pgpulse.db.table.idx_blks_read | pg.db.table.idx_blks_read |
| pgpulse.db.table.index_bytes | pg.db.table.index_bytes |
| pgpulse.db.table.live_tuples | pg.db.table.live_tuples |
| pgpulse.db.table.toast_bytes | pg.db.table.toast_bytes |
| pgpulse.db.table.total_bytes | pg.db.table.total_bytes |
| pgpulse.db.toast.size_bytes | pg.db.toast.size_bytes |
| pgpulse.db.unlogged.size_bytes | pg.db.unlogged.size_bytes |
| pgpulse.db.vacuum.autoanalyze_age_sec | pg.db.vacuum.autoanalyze_age_sec |
| pgpulse.db.vacuum.autovacuum_age_sec | pg.db.vacuum.autovacuum_age_sec |
| pgpulse.db.vacuum.dead_pct | pg.db.vacuum.dead_pct |
| pgpulse.db.vacuum.dead_tuples | pg.db.vacuum.dead_tuples |
| pgpulse.db.vacuum.live_tuples | pg.db.vacuum.live_tuples |
| pgpulse.db.vacuum.mod_since_analyze | pg.db.vacuum.mod_since_analyze |

#### Extensions (3 keys)

| Current Key | New Key |
|------------|---------|
| pgpulse.extensions.pgss_fill_pct | pg.extensions.pgss_fill_pct |
| pgpulse.extensions.pgss_installed | pg.extensions.pgss_installed |
| pgpulse.extensions.pgss_stats_reset_unix | pg.extensions.pgss_stats_reset_unix |

#### IO Stats (11 keys, PG 16+)

| Current Key | New Key |
|------------|---------|
| pgpulse.io.evictions | pg.io.evictions |
| pgpulse.io.extend_time | pg.io.extend_time |
| pgpulse.io.extends | pg.io.extends |
| pgpulse.io.fsync_time | pg.io.fsync_time |
| pgpulse.io.fsyncs | pg.io.fsyncs |
| pgpulse.io.hits | pg.io.hits |
| pgpulse.io.read_time | pg.io.read_time |
| pgpulse.io.reads | pg.io.reads |
| pgpulse.io.reuses | pg.io.reuses |
| pgpulse.io.write_time | pg.io.write_time |
| pgpulse.io.writes | pg.io.writes |

#### Locks & Long Transactions (5 keys)

| Current Key | New Key |
|------------|---------|
| pgpulse.locks.blocked_count | pg.locks.blocked_count |
| pgpulse.locks.blocker_count | pg.locks.blocker_count |
| pgpulse.locks.max_chain_depth | pg.locks.max_chain_depth |
| pgpulse.long_transactions.count | pg.long_transactions.count |
| pgpulse.long_transactions.oldest_seconds | pg.long_transactions.oldest_seconds |

#### Progress (28 keys)

All `pgpulse.progress.*` → `pg.progress.*`. Sub-categories (vacuum, analyze,
cluster, create_index, basebackup, copy) preserved as-is. Full list:

| Current Key | New Key |
|------------|---------|
| pgpulse.progress.analyze.child_tables_done | pg.progress.analyze.child_tables_done |
| pgpulse.progress.analyze.child_tables_total | pg.progress.analyze.child_tables_total |
| pgpulse.progress.analyze.completion_pct | pg.progress.analyze.completion_pct |
| pgpulse.progress.analyze.ext_stats_computed | pg.progress.analyze.ext_stats_computed |
| pgpulse.progress.analyze.ext_stats_total | pg.progress.analyze.ext_stats_total |
| pgpulse.progress.analyze.sample_blks_scanned | pg.progress.analyze.sample_blks_scanned |
| pgpulse.progress.analyze.sample_blks_total | pg.progress.analyze.sample_blks_total |
| pgpulse.progress.basebackup.backup_streamed | pg.progress.basebackup.backup_streamed |
| pgpulse.progress.basebackup.backup_total | pg.progress.basebackup.backup_total |
| pgpulse.progress.basebackup.completion_pct | pg.progress.basebackup.completion_pct |
| pgpulse.progress.basebackup.tablespaces_streamed | pg.progress.basebackup.tablespaces_streamed |
| pgpulse.progress.basebackup.tablespaces_total | pg.progress.basebackup.tablespaces_total |
| pgpulse.progress.cluster.completion_pct | pg.progress.cluster.completion_pct |
| pgpulse.progress.cluster.heap_blks_scanned | pg.progress.cluster.heap_blks_scanned |
| pgpulse.progress.cluster.heap_blks_total | pg.progress.cluster.heap_blks_total |
| pgpulse.progress.cluster.heap_tuples_scanned | pg.progress.cluster.heap_tuples_scanned |
| pgpulse.progress.cluster.heap_tuples_written | pg.progress.cluster.heap_tuples_written |
| pgpulse.progress.cluster.index_rebuild_count | pg.progress.cluster.index_rebuild_count |
| pgpulse.progress.copy.bytes_processed | pg.progress.copy.bytes_processed |
| pgpulse.progress.copy.bytes_total | pg.progress.copy.bytes_total |
| pgpulse.progress.copy.completion_pct | pg.progress.copy.completion_pct |
| pgpulse.progress.copy.tuples_excluded | pg.progress.copy.tuples_excluded |
| pgpulse.progress.copy.tuples_processed | pg.progress.copy.tuples_processed |
| pgpulse.progress.create_index.blocks_done | pg.progress.create_index.blocks_done |
| pgpulse.progress.create_index.blocks_total | pg.progress.create_index.blocks_total |
| pgpulse.progress.create_index.completion_pct | pg.progress.create_index.completion_pct |
| pgpulse.progress.create_index.lockers_done | pg.progress.create_index.lockers_done |
| pgpulse.progress.create_index.lockers_total | pg.progress.create_index.lockers_total |
| pgpulse.progress.create_index.partitions_done | pg.progress.create_index.partitions_done |
| pgpulse.progress.create_index.partitions_total | pg.progress.create_index.partitions_total |
| pgpulse.progress.create_index.tuples_done | pg.progress.create_index.tuples_done |
| pgpulse.progress.create_index.tuples_total | pg.progress.create_index.tuples_total |
| pgpulse.progress.vacuum.completion_pct | pg.progress.vacuum.completion_pct |
| pgpulse.progress.vacuum.heap_blks_scanned | pg.progress.vacuum.heap_blks_scanned |
| pgpulse.progress.vacuum.heap_blks_total | pg.progress.vacuum.heap_blks_total |
| pgpulse.progress.vacuum.heap_blks_vacuumed | pg.progress.vacuum.heap_blks_vacuumed |
| pgpulse.progress.vacuum.index_vacuum_count | pg.progress.vacuum.index_vacuum_count |
| pgpulse.progress.vacuum.max_dead_tuples | pg.progress.vacuum.max_dead_tuples |
| pgpulse.progress.vacuum.num_dead_tuples | pg.progress.vacuum.num_dead_tuples |

#### Replication (14 keys)

| Current Key | New Key |
|------------|---------|
| pgpulse.replication.active_replicas | pg.replication.active_replicas |
| pgpulse.replication.lag.flush_bytes | pg.replication.lag.flush_bytes |
| pgpulse.replication.lag.flush_seconds | pg.replication.lag.flush_seconds |
| pgpulse.replication.lag.pending_bytes | pg.replication.lag.pending_bytes |
| pgpulse.replication.lag.replay_bytes | pg.replication.lag.replay_bytes |
| pgpulse.replication.lag.replay_seconds | pg.replication.lag.replay_seconds |
| pgpulse.replication.lag.total_bytes | pg.replication.lag.total_bytes |
| pgpulse.replication.lag.write_bytes | pg.replication.lag.write_bytes |
| pgpulse.replication.lag.write_seconds | pg.replication.lag.write_seconds |
| pgpulse.replication.replica.connected | pg.replication.replica.connected |
| pgpulse.replication.slot.active | pg.replication.slot.active |
| pgpulse.replication.slot.retained_bytes | pg.replication.slot.retained_bytes |
| pgpulse.replication.wal_receiver.connected | pg.replication.wal_receiver.connected |
| pgpulse.replication.wal_receiver.lag_bytes | pg.replication.wal_receiver.lag_bytes |

#### Server Info (6 keys)

| Current Key | New Key |
|------------|---------|
| pgpulse.server.hostname | pg.server.hostname |
| pgpulse.server.is_in_backup | pg.server.is_in_backup |
| pgpulse.server.is_in_recovery | pg.server.is_in_recovery |
| pgpulse.server.os | pg.server.os |
| pgpulse.server.start_time_unix | pg.server.start_time_unix |
| pgpulse.server.uptime_seconds | pg.server.uptime_seconds |

#### Settings (4 keys)

| Current Key | New Key |
|------------|---------|
| pgpulse.settings.max_locks_per_tx | pg.settings.max_locks_per_tx |
| pgpulse.settings.max_prepared_tx | pg.settings.max_prepared_tx |
| pgpulse.settings.shared_buffers_8kb | pg.settings.shared_buffers_8kb |
| pgpulse.settings.track_io_timing | pg.settings.track_io_timing |

#### Statements (12 keys)

| Current Key | New Key |
|------------|---------|
| pgpulse.statements.count | pg.statements.count |
| pgpulse.statements.fill_pct | pg.statements.fill_pct |
| pgpulse.statements.max | pg.statements.max |
| pgpulse.statements.stats_reset_age_seconds | pg.statements.stats_reset_age_seconds |
| pgpulse.statements.top.avg_time_ms | pg.statements.top.avg_time_ms |
| pgpulse.statements.top.calls | pg.statements.top.calls |
| pgpulse.statements.top.cpu_time_ms | pg.statements.top.cpu_time_ms |
| pgpulse.statements.top.io_time_ms | pg.statements.top.io_time_ms |
| pgpulse.statements.top.rows | pg.statements.top.rows |
| pgpulse.statements.top.total_time_ms | pg.statements.top.total_time_ms |
| pgpulse.statements.track | pg.statements.track |
| pgpulse.statements.track_io_timing | pg.statements.track_io_timing |

#### Transactions (2 keys)

| Current Key | New Key |
|------------|---------|
| pgpulse.transactions.commit_ratio_pct | pg.transactions.commit_ratio_pct |
| pgpulse.transactions.deadlocks | pg.transactions.deadlocks |

#### Wait Events (2 keys)

| Current Key | New Key |
|------------|---------|
| pgpulse.wait_events.count | pg.wait_events.count |
| pgpulse.wait_events.total_backends | pg.wait_events.total_backends |

---

## 4. Implementation Plan

### Step 1: Change Base.point() prefix

**File:** `internal/collector/base.go` (or wherever Base struct is defined)

```go
// BEFORE
func (b *Base) point(name string) string {
    return "pgpulse." + name
}

// AFTER
func (b *Base) point(name string) string {
    return "pg." + name
}
```

This single change renames ~120 keys atomically.

### Step 2: OSSQLCollector — bypass Base.point()

After Step 1, `Base.point("os.cpu.user_pct")` would produce `"pg.os.cpu.user_pct"` —
wrong. OSSQLCollector must emit `"os.cpu.user_pct"` directly without using `point()`.

Search for all `b.point("os.` or similar patterns in `os_sql.go` and replace with
direct string construction: `"os." + name`.

### Step 3: Disk metric hierarchy rename

In BOTH `os_sql.go`, `os.go`, and `agent/osmetrics_linux.go`:

| Find | Replace |
|------|---------|
| `os.diskstat.read_kb` | `os.disk.read_bytes_per_sec` |
| `os.diskstat.write_kb` | `os.disk.write_bytes_per_sec` |
| `os.diskstat.reads_completed` | `os.disk.reads_completed` |
| `os.diskstat.writes_completed` | `os.disk.writes_completed` |
| `os.diskstat.read_await_ms` | `os.disk.read_await_ms` |
| `os.diskstat.write_await_ms` | `os.disk.write_await_ms` |
| `os.diskstat.util_pct` | `os.disk.util_pct` |

**VERIFY VALUE UNITS:** If `ParseDiskStats()` returns kilobytes for `read_kb`,
multiply by 1024 when renaming to `read_bytes_per_sec`. If already bytes/sec,
name-only change.

### Step 4: Frontend — update all metric key references

**Strategy:** If `web/src/lib/constants.ts` centralizes metric keys, update there
first. Then grep for any remaining hardcoded `"pgpulse."` or `"os.diskstat."` strings.

Key files from Codebase Digest Section 5:

| File | Keys Referenced |
|------|----------------|
| constants.ts | All metric key constants |
| KeyMetricsRow.tsx | connections, cache, server info |
| ConnectionsChart.tsx | pg.connections.by_state, .total, .max |
| CacheHitRatioChart.tsx | pg.cache.hit_ratio |
| TransactionCommitRatioChart.tsx | pg.transactions.commit_ratio_pct |
| ReplicationLagChart.tsx | pg.replication.lag.replay_bytes |
| OSMetricsSection.tsx | All os.* keys including diskstat→disk |
| ClusterSection.tsx | cluster.* keys (NO CHANGE needed) |
| forecastUtils.ts | Metric keys in forecast queries |
| useForecast.ts / useForecastChart.ts | Metric key parameters |
| useOSMetrics.ts | OS metric key references |
| InstanceCard / FleetOverview | Metric key lookups for cards |

### Step 5: Alert rules & seeds

**File:** `internal/alert/seed.go` — update all metric key strings.
Test files: update metric key assertions.

### Step 6: ML configuration & detector

**Files:** `internal/ml/detector.go`, `internal/ml/config.go`, test files.
Also `config.sample.yaml` ML section.

### Step 7: API handlers

Check all `internal/api/*.go` for hardcoded metric keys. Common places:
`instances.go`, `metrics.go`, `os.go`, `forecast.go`.

### Step 8: SQL migration script

Create `migrations/NNN_metric_naming_standardization.sql`.
Check current highest migration number first.

```sql
BEGIN;

-- 1. Bulk rename pgpulse.* → pg.* in metrics table (except OS metrics)
UPDATE metrics
SET metric = 'pg.' || substring(metric FROM 9)
WHERE metric LIKE 'pgpulse.%'
  AND metric NOT LIKE 'pgpulse.os.%';

-- 2. Fix OS metrics from SQL path: pgpulse.os.* → os.*
UPDATE metrics
SET metric = substring(metric FROM 9)
WHERE metric LIKE 'pgpulse.os.%';

-- 3. Rename os.diskstat.* → os.disk.*
UPDATE metrics
SET metric = replace(metric, 'os.diskstat.', 'os.disk.')
WHERE metric LIKE 'os.diskstat.%';

-- 4. Specific diskstat unit renames
UPDATE metrics SET metric = 'os.disk.read_bytes_per_sec'
WHERE metric IN ('os.disk.read_kb', 'os.diskstat.read_kb');
UPDATE metrics SET metric = 'os.disk.write_bytes_per_sec'
WHERE metric IN ('os.disk.write_kb', 'os.diskstat.write_kb');

-- 5. Alert rules — same renames
UPDATE alert_rules
SET metric = 'pg.' || substring(metric FROM 9)
WHERE metric LIKE 'pgpulse.%'
  AND metric NOT LIKE 'pgpulse.os.%';

UPDATE alert_rules
SET metric = substring(metric FROM 9)
WHERE metric LIKE 'pgpulse.os.%';

UPDATE alert_rules
SET metric = replace(metric, 'os.diskstat.', 'os.disk.')
WHERE metric LIKE 'os.diskstat.%';

-- 6. ML baseline snapshots — same renames
UPDATE ml_baseline_snapshots
SET metric_key = 'pg.' || substring(metric_key FROM 9)
WHERE metric_key LIKE 'pgpulse.%'
  AND metric_key NOT LIKE 'pgpulse.os.%';

UPDATE ml_baseline_snapshots
SET metric_key = substring(metric_key FROM 9)
WHERE metric_key LIKE 'pgpulse.os.%';

UPDATE ml_baseline_snapshots
SET metric_key = replace(metric_key, 'os.diskstat.', 'os.disk.')
WHERE metric_key LIKE 'os.diskstat.%';

COMMIT;
```

**NOTE:** `cluster.*` keys need NO migration — unchanged per D200.

### Step 9: Config files

Update `config.sample.yaml` and example configs:
- `ml.metrics[].key` values: `pgpulse.*` → `pg.*`
- Any hardcoded metric keys in comments

---

## 5. Verification Checklist

After all changes:

1. `grep -rn '"pgpulse\.' internal/ cmd/` — ZERO matches (except comments)
2. `grep -rn 'pgpulse\.' web/src/` — ZERO matches
3. `grep -rn 'os\.diskstat' internal/ cmd/ web/src/` — ZERO matches
4. Full build:
   ```bash
   cd web && npm run build && npm run typecheck && npm run lint
   cd .. && go build ./cmd/pgpulse-server ./cmd/pgpulse-agent
   go test ./cmd/... ./internal/... -count=1
   golangci-lint run ./cmd/... ./internal/...
   ```
5. Charts render with correct data in the UI

---

## 6. Risk Register

| Risk | Mitigation |
|------|-----------|
| Missed metric key reference in frontend | Systematic grep for `pgpulse.` and `diskstat` after changes |
| Test assertions use old key names | Agents must update `*_test.go` files too |
| ML baseline snapshots become orphaned | Migration script updates `ml_baseline_snapshots` table |
| Unit conversion needed for disk read_kb→read_bytes_per_sec | Verify actual ParseDiskStats values; multiply by 1024 if needed |
| MemoryStore (live mode) has no persistent data | No migration needed — keys change on next collection cycle |
| YAML config has old metric keys | Update config.sample.yaml |
| cache.hit_ratio vs cache.hit_ratio_pct mismatch | Migration handles both variants |

---

## 7. Files Modified (Expected)

### Go Backend — Collector Agent territory
- `internal/collector/base.go` — `point()` prefix change
- `internal/collector/os_sql.go` — bypass Base.point() for os.* keys
- `internal/collector/os.go` — `os.diskstat.*` → `os.disk.*`
- `internal/agent/osmetrics_linux.go` — diskstat key renames
- `internal/agent/osmetrics.go` — diskstat key renames in struct/constants
- `internal/agent/osmetrics_test.go` — test assertion updates
- `internal/agent/scraper.go` — if metric keys referenced
- `internal/agent/scraper_test.go` — test assertion updates
- All `internal/collector/*_test.go` — metric key assertions

### Go Backend — API & Security Agent territory
- `internal/alert/seed.go` — default rule metric keys
- `internal/alert/evaluator.go` — if metric keys hardcoded
- `internal/alert/evaluator_test.go` — test assertions
- `internal/alert/evaluator_forecast_test.go` — test assertions
- `internal/ml/detector.go` — metric key references
- `internal/ml/config.go` — if keys referenced
- `internal/api/*.go` — hardcoded metric key references
- `migrations/NNN_metric_naming_standardization.sql` — NEW

### Frontend — Frontend Agent territory
- `web/src/lib/constants.ts` — metric key constants
- `web/src/components/**/*.tsx` — chart/display components
- `web/src/hooks/*.ts` — metric key parameters in API calls
- `web/src/pages/**/*.tsx` — inline metric key references

### Config
- `config.sample.yaml` — ML metric key references
