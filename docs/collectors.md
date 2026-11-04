# PGPulse Collectors

<!-- Generated from the structured doc comments on collector types in internal/collector/. -->
<!-- Regenerate rather than editing by hand: the doc comment on each type is the source of truth. -->

PGPulse ships **27 collectors**. Each one answers a single operational question about a monitored PostgreSQL instance and emits the metric points behind anomaly detection, root-cause analysis and alerting. Collectors are grouped into tiers by the layer of the stack they observe.

**Collection** is the orchestrator group a collector runs in: `10s` (high), `60s` (medium) and `300s` (low) are the default group intervals and can be changed per instance in configuration; `per-database` collectors are dispatched once per discovered database every 5 minutes by the per-database runner.

**PG versions** is the version gate. `PG 14+` means the collector runs unchanged on every supported version (14 is the minimum). Collectors with version-specific SQL variants list them; collectors with a known upper bound say so.

## Contents

- [postgres](#postgres) — 6 collectors
- [database](#database) — 3 collectors
- [session](#session) — 10 collectors
- [replication](#replication) — 3 collectors
- [patroni-etcd](#patroni-etcd) — 1 collector
- [pooler](#pooler) — 0 collectors
- [os](#os) — 2 collectors
- [environment](#environment) — 2 collectors

## postgres

Instance-wide server state: cache, checkpoints, I/O, settings, query statistics.

| Collector | Question | Views / sources | Collection | PG versions |
|---|---|---|---|---|
| `cache` (`CacheCollector`, `cache.go`) | Is the shared buffer cache absorbing reads, or is the instance going to disk? | pg_stat_database | `10s` | PG 14+ |
| `checkpoint` (`CheckpointCollector`, `checkpoint.go`) | Are checkpoints and the background writer keeping up, or are backends forced to write buffers themselves? | pg_stat_bgwriter (PG 14–16); pg_stat_checkpointer + pg_stat_bgwriter (PG 17+) | `60s` | PG 14–16 and PG 17+ variants (see checkpointGate) |
| `io_stats` (`IOStatsCollector`, `io_stats.go`) | Which backend types are doing the reads, writes, extends and fsyncs, and how much of that time is spent waiting on I/O? | pg_stat_io | `300s` | PG 16+ (returns nil on older versions where the view does not exist) |
| `settings` (`SettingsCollector`, `settings.go`) | What are the configured ceilings (shared_buffers, lock table, prepared transactions) and is I/O timing being tracked? | pg_settings | `300s` | PG 14+ |
| `statements_config` (`StatementsConfigCollector`, `statements_config.go`) | Can the top-query statistics be trusted — is tracking on, is the entry buffer overflowing, and how long has it been accumulating? | pg_settings, pg_stat_statements, pg_stat_statements_info, pg_extension | `60s` | PG 14+ (pg_stat_statements_info exists from PG 14; returns nil when the extension is not installed) |
| `statements_top` (`StatementsTopCollector`, `statements_top.go`) | Which queries are consuming the most execution time, and is that time going to I/O or CPU? | pg_stat_statements, pg_extension | `60s` | PG 14–16 (pg_stat_statements 1.11 on PG 17 renamed blk_read_time/blk_write_time; see note) |

## database

Per-database and per-relation analysis: sizes, bloat, vacuum need, indexes, transactions.

| Collector | Question | Views / sources | Collection | PG versions |
|---|---|---|---|---|
| `database` (`DatabaseCollector`, `database.go`) | Inside this database, which tables and indexes are growing, bloating, starving for vacuum, or sitting unused? | pg_stat_user_tables, pg_statio_user_tables, pg_stat_user_indexes, pg_index, pg_class, pg_namespace, pg_stats, pg_inherits, pg_sequences, pg_stat_user_functions, pg_largeobject_metadata, pg_largeobject, pg_subscription_rel, pg_subscription | `per-database` | PG 14+ |
| `database_sizes` (`DatabaseSizesCollector`, `database_sizes.go`) | How big is each database, and which ones are growing? | pg_database, pg_database_size() | `300s` | PG 14+ |
| `transactions` (`TransactionsCollector`, `transactions.go`) | Which databases are rolling back an unusual share of transactions or hitting deadlocks? | pg_stat_database, pg_database | `300s` | PG 14+ |

## session

What backends are doing right now: connections, waits, locks, long transactions, running maintenance.

| Collector | Question | Views / sources | Collection | PG versions |
|---|---|---|---|---|
| `connections` (`ConnectionsCollector`, `connections.go`) | How many client connections are open, what are they doing, and how close is the instance to running out of slots? | pg_stat_activity, current_setting('max_connections'), current_setting('superuser_reserved_connections') | `10s` | PG 14+ |
| `lock_tree` (`LockTreeCollector`, `lock_tree.go`) | Is anyone blocked on locks right now, how many sessions are affected, and how deep does the blocking chain go? | pg_stat_activity, pg_blocking_pids() | `10s` | PG 14+ |
| `long_transactions` (`LongTransactionsCollector`, `long_transactions.go`) | Are there long-running transactions holding back vacuum and locks, and are they working or stuck waiting? | pg_stat_activity | `10s` | PG 14+ |
| `progress_analyze` (`AnalyzeProgressCollector`, `progress_maintenance.go`) | Is an ANALYZE running, which table is being sampled, and how far along is it? | pg_stat_progress_analyze, pg_stat_activity | `60s` | PG 14+ |
| `progress_basebackup` (`BasebackupProgressCollector`, `progress_operations.go`) | Is a base backup running, who started it, and how much has been streamed? | pg_stat_progress_basebackup, pg_stat_activity | `60s` | PG 14+ |
| `progress_cluster` (`ClusterProgressCollector`, `progress_maintenance.go`) | Is a CLUSTER or VACUUM FULL running, which table is it rewriting, and how far along is it? | pg_stat_progress_cluster, pg_stat_activity | `60s` | PG 14+ |
| `progress_copy` (`CopyProgressCollector`, `progress_operations.go`) | Is a bulk COPY running, into or out of which table, and how much has it processed? | pg_stat_progress_copy, pg_stat_activity | `60s` | PG 14+ |
| `progress_create_index` (`CreateIndexProgressCollector`, `progress_operations.go`) | Is an index build or rebuild running, which phase is it in, and is it waiting on other sessions? | pg_stat_progress_create_index, pg_stat_activity | `60s` | PG 14+ |
| `progress_vacuum` (`VacuumProgressCollector`, `progress_vacuum.go`) | Which tables are being vacuumed right now, which phase is each vacuum in, and how far along is it? | pg_stat_progress_vacuum, pg_stat_activity | `60s` | PG 14–16 (the dead-tuple columns were renamed in PG 17; see note) |
| `wait_events` (`WaitEventsCollector`, `wait_events.go`) | What are active sessions waiting on — locks, I/O, LWLocks, client — or are they actually running? | pg_stat_activity | `10s` | PG 14+ |

## replication

Physical and logical replication health.

| Collector | Question | Views / sources | Collection | PG versions |
|---|---|---|---|---|
| `replication_lag` (`ReplicationLagCollector`, `replication_lag.go`) | How far behind is each standby, and is the lag in sending, writing, flushing or replaying WAL? | pg_stat_replication, pg_current_wal_lsn(), pg_wal_lsn_diff() | `60s` | PG 14+ (runs on the primary only; returns nil on standbys) |
| `replication_slots` (`ReplicationSlotsCollector`, `replication_slots.go`) | Which replication slots exist, are any inactive, and how much WAL are they pinning on disk? | pg_replication_slots, pg_current_wal_lsn(), pg_wal_lsn_diff() | `60s` | PG 14 / PG 15 / PG 16+ variants (see replicationSlotsGate) |
| `replication_status` (`ReplicationStatusCollector`, `replication_status.go`) | On a primary, which physical standbys are connected and in what sync state; on a standby, is it still streaming from its primary? | pg_stat_replication + pg_replication_slots (primary); pg_stat_wal_receiver + pg_wal_lsn_diff() (standby) | `60s` | PG 14+ |

## patroni-etcd

HA cluster membership and quorum.

| Collector | Question | Views / sources | Collection | PG versions |
|---|---|---|---|---|
| `cluster` (`ClusterCollector`, `cluster.go`) | Does the cluster have exactly one healthy leader, are all members running, and is the etcd quorum intact? | Patroni REST API / patronictl (cluster members, roles, lag); etcd member list and endpoint health | `60s` | PG 14+ (no SQL; depends on Patroni/etcd configuration, not on PostgreSQL version) |

## pooler

Connection pooler state.

_No collectors in this tier yet; the tier is reserved for connection pooler metrics (PgBouncer and similar)._

## os

Host resources of the monitored server.

| Collector | Question | Views / sources | Collection | PG versions |
|---|---|---|---|---|
| `os` (`OSCollector`, `os.go`) | Is the host itself under pressure — CPU, memory, disk space or disk latency — in a way that explains what PostgreSQL is seeing? | procfs/sysfs via a remote pgpulse-agent (/metrics scrape) or, when the DSN points at localhost, read directly | `10s` | PG 14+ (no SQL; registered only when an agent URL is set or the instance is local) |
| `os_sql` (`OSSQLCollector`, `os_sql.go`) | Is the host under CPU, memory or disk-latency pressure, when no agent is deployed next to PostgreSQL? | pg_read_file('/proc/meminfo'), /proc/uptime, /proc/loadavg, /proc/stat, /proc/diskstats | `60s` | PG 14+ (requires GRANT EXECUTE ON pg_read_file to the monitoring role; pg_read_server_files membership alone only lifts the path restriction) |

## environment

Identity and configuration that changes rarely.

| Collector | Question | Views / sources | Collection | PG versions |
|---|---|---|---|---|
| `extensions` (`ExtensionsCollector`, `extensions.go`) | Is pg_stat_statements installed, and is its buffer or reset age about to make query statistics unreliable? | pg_extension, pg_stat_statements, pg_stat_statements_info, current_setting('pg_stat_statements.max') | `300s` | PG 14+ (pg_stat_statements_info exists from PG 14) |
| `server_info` (`ServerInfoCollector`, `server_info.go`) | What is this server (host, OS, uptime, role), and is it approaching XID or multixact wraparound? | pg_postmaster_start_time(), pg_database (age(datfrozenxid), mxid_age(datminmxid)), pg_is_in_backup() on PG 14, pg_read_file('/etc/hostname', '/etc/os-release') | `300s` | PG 14+ (pg_is_in_backup() is gated to PG 14; removed in PG 15, where is_in_backup is emitted as 0) |

## Notes

- **PG 17 column renames.** Two collectors still select pre-17 column names and will fail on PG 17+ until a version gate is added: `progress_vacuum` (`pg_stat_progress_vacuum.max_dead_tuples` → `max_dead_tuple_bytes`, `num_dead_tuples` → `num_dead_item_ids`) and `statements_top` (`pg_stat_statements.blk_read_time` → `shared_blk_read_time`, `blk_write_time` → `shared_blk_write_time`, from pg_stat_statements 1.11). `checkpoint` and `replication_slots` already carry per-version SQL variants and are the pattern to follow.
- **Cadence is the orchestrator's.** Each collector reports an advisory `Interval()`, but the orchestrator ignores it and runs collectors on the shared group tickers listed above; the Collection column reflects the group a collector is actually registered in (`internal/orchestrator/runner.go`).
- **Updating this file.** Edit the doc comment on the collector type (the five `Tier / Question / Reads / Collection / Versions` lines) and regenerate; the tables are derived from those comments.
