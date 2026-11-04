# PGPulse Collectors

> Generated from the doc comments in `internal/collector/*.go`. Edit the comments, then regenerate this file.

PGPulse ships **27 collectors**. Each one answers one question a DBA asks during an incident. They are grouped below by the tier of the stack they observe.

**Collection** is the schedule the orchestrator actually runs the collector on (`internal/orchestrator/runner.go`): the high (default 10s), medium (default 60s) and low (default 300s) interval groups, configurable per instance under `intervals`, or the per-database runner (every 5 minutes, once per discovered database).

**PG versions** gives the supported range and the version gate, if any. The minimum supported version is PostgreSQL 14.

| Tier | Collectors |
|------|-----------:|
| [postgres](#postgres) | 8 |
| [database](#database) | 3 |
| [session](#session) | 10 |
| [replication](#replication) | 3 |
| [patroni-etcd](#patroni-etcd) | 1 |
| [pooler](#pooler) | 0 |
| [os](#os) | 2 |
| [environment](#environment) | 0 |
| **Total** | **27** |

## postgres

| Collector | Question | Views | Collection | PG versions |
|---|---|---|---|---|
| `cache` (`CacheCollector`, [cache.go](../internal/collector/cache.go)) | Is the working set still fitting in shared_buffers, or are reads going to disk? | pg_stat_database | 10s (high group) | 14+ (no version gate) |
| `checkpoint` (`CheckpointCollector`, [checkpoint.go](../internal/collector/checkpoint.go)) | Are checkpoints being forced by WAL volume instead of the timer, and who is writing dirty buffers? | pg_stat_bgwriter (14–16); pg_stat_checkpointer + pg_stat_bgwriter (17+) | 60s (medium group) | 14–16 / 17+ via checkpointGate |
| `extensions` (`ExtensionsCollector`, [extensions.go](../internal/collector/extensions.go)) | Is pg_stat_statements installed, how full is its buffer, and when were its stats last reset? | pg_extension, pg_stat_statements, pg_stat_statements_info | 300s (low group) | 14+ (stats reset read guarded by AtLeast(14, 0)) |
| `io_stats` (`IOStatsCollector`, [io_stats.go](../internal/collector/io_stats.go)) | Which backend types and I/O contexts are doing the reads, writes, extends and fsyncs? | pg_stat_io | 300s (low group) | 16+ (Major < 16 check in Collect; no-op below 16) |
| `server_info` (`ServerInfoCollector`, [server_info.go](../internal/collector/server_info.go)) | When did this server start, is it a primary or standby, and how close is it to XID or multixact wraparound? | pg_postmaster_start_time(), pg_is_in_backup() (14 only), pg_database (datfrozenxid, datminmxid), pg_read_file('/etc/hostname'), pg_read_file('/etc/os-release') | 300s (low group) | 14 / 15+ via backupStateGate (pg_is_in_backup() was removed in 15; is_in_backup reports 0 there) |
| `settings` (`SettingsCollector`, [settings.go](../internal/collector/settings.go)) | Are the settings that bound memory, locking and I/O timing configured the way we expect? | pg_settings (track_io_timing, shared_buffers, max_locks_per_transaction, max_prepared_transactions) | 300s (low group) | 14+ (no version gate) |
| `statements_config` (`StatementsConfigCollector`, [statements_config.go](../internal/collector/statements_config.go)) | Can pg_stat_statements data be trusted — is the buffer full, is tracking on, and when was it last reset? | pg_settings, pg_stat_statements, pg_stat_statements_info | 60s (medium group); no-op when pg_stat_statements is not installed | 14+ (no version gate; pg_stat_statements_info exists from 14) |
| `statements_top` (`StatementsTopCollector`, [statements_top.go](../internal/collector/statements_top.go)) | Which queries consume the most execution time, and how much of it is I/O versus CPU? | pg_stat_statements | 60s (medium group); no-op when pg_stat_statements is not installed | 14+ (no version gate; reads blk_read_time/blk_write_time, which PG 17 renamed to shared_blk_*_time) |

## database

| Collector | Question | Views | Collection | PG versions |
|---|---|---|---|---|
| `database` (`DatabaseCollector`, [database.go](../internal/collector/database.go)) | Inside each database, which tables and indexes are bloated, unused, oversized or overdue for vacuum, and which sequences are close to exhaustion? | pg_class, pg_namespace, pg_stats, pg_index, pg_inherits, pg_sequences, pg_stat_user_tables, pg_statio_user_tables, pg_stat_user_indexes, pg_stat_user_functions, pg_largeobject, pg_largeobject_metadata, pg_subscription, pg_subscription_rel | per-database, every 5 minutes (DBRunner), with a 60s statement_timeout | 14+ (no version gate; logical replication status degrades to 0 when pg_subscription is unreadable) |
| `database_sizes` (`DatabaseSizesCollector`, [database_sizes.go](../internal/collector/database_sizes.go)) | How large is each database, and which ones are growing? | pg_database, pg_database_size() | 300s (low group) | 14+ (no version gate) |
| `transactions` (`TransactionsCollector`, [transactions.go](../internal/collector/transactions.go)) | Which databases are rolling back an unusual share of transactions or hitting deadlocks? | pg_stat_database, pg_database | 300s (low group) | 14+ (no version gate) |

## session

| Collector | Question | Views | Collection | PG versions |
|---|---|---|---|---|
| `connections` (`ConnectionsCollector`, [connections.go](../internal/collector/connections.go)) | How close is the instance to running out of connection slots, and what are the connections doing? | pg_stat_activity, current_setting('max_connections'), current_setting('superuser_reserved_connections') | 10s (high group) | 14+ (no version gate) |
| `lock_tree` (`LockTreeCollector`, [lock_tree.go](../internal/collector/lock_tree.go)) | Is anyone blocking other sessions, how many are stuck behind them, and how deep is the chain? | pg_stat_activity, pg_blocking_pids() | 10s (high group) | 14+ (no version gate) |
| `long_transactions` (`LongTransactionsCollector`, [long_transactions.go](../internal/collector/long_transactions.go)) | Which transactions have been open longer than 5 seconds, and are they running or waiting? | pg_stat_activity | 10s (high group) | 14+ (no version gate) |
| `progress_analyze` (`AnalyzeProgressCollector`, [progress_maintenance.go](../internal/collector/progress_maintenance.go)) | Which ANALYZE runs are in progress, and how much of their sample have they read? | pg_stat_progress_analyze, pg_stat_activity | 60s (medium group) | 14+ (no version gate) |
| `progress_basebackup` (`BasebackupProgressCollector`, [progress_operations.go](../internal/collector/progress_operations.go)) | Is a base backup streaming from this server, who started it, and how much is left? | pg_stat_progress_basebackup, pg_stat_activity | 60s (medium group) | 14+ (no version gate) |
| `progress_cluster` (`ClusterProgressCollector`, [progress_maintenance.go](../internal/collector/progress_maintenance.go)) | Which CLUSTER or VACUUM FULL rewrites are running, and how far along are they? | pg_stat_progress_cluster, pg_stat_activity | 60s (medium group) | 14+ (no version gate) |
| `progress_copy` (`CopyProgressCollector`, [progress_operations.go](../internal/collector/progress_operations.go)) | Which COPY loads or exports are running, and how many bytes and tuples have they processed? | pg_stat_progress_copy, pg_stat_activity | 60s (medium group) | 14+ (no version gate) |
| `progress_create_index` (`CreateIndexProgressCollector`, [progress_operations.go](../internal/collector/progress_operations.go)) | Which index builds are running, in which phase, and how far along are they? | pg_stat_progress_create_index, pg_stat_activity | 60s (medium group) | 14+ (no version gate) |
| `progress_vacuum` (`VacuumProgressCollector`, [progress_vacuum.go](../internal/collector/progress_vacuum.go)) | Which VACUUMs are running now, in which phase, and how far along are they? | pg_stat_progress_vacuum, pg_stat_activity | 60s (medium group) | 14+ (no version gate; reads max_dead_tuples/num_dead_tuples, which PG 17 renamed to max_dead_tuple_bytes/num_dead_item_ids) |
| `wait_events` (`WaitEventsCollector`, [wait_events.go](../internal/collector/wait_events.go)) | What are active client backends waiting on right now (locks, I/O, LWLocks, or running on CPU)? | pg_stat_activity | 10s (high group) | 14+ (no version gate) |

## replication

| Collector | Question | Views | Collection | PG versions |
|---|---|---|---|---|
| `replication_lag` (`ReplicationLagCollector`, [replication_lag.go](../internal/collector/replication_lag.go)) | How far behind is each replica, in bytes and seconds, and at which pipeline stage (send, write, flush, replay)? | pg_stat_replication, pg_current_wal_lsn(), pg_wal_lsn_diff() | 60s (medium group); skipped on standbys | 14+ (no version gate) |
| `replication_slots` (`ReplicationSlotsCollector`, [replication_slots.go](../internal/collector/replication_slots.go)) | Is any replication slot inactive or retaining enough WAL to threaten the disk? | pg_replication_slots, pg_current_wal_lsn(), pg_wal_lsn_diff() | 60s (medium group) | 14 / 15 / 16+ via replicationSlotsGate (15 adds two_phase, 16 adds conflicting) |
| `replication_status` (`ReplicationStatusCollector`, [replication_status.go](../internal/collector/replication_status.go)) | Which replicas are attached to this primary, or is this standby actually streaming from its upstream? | pg_stat_replication, pg_replication_slots (primary); pg_stat_wal_receiver (standby) | 60s (medium group) | 14+ (no version gate) |

## patroni-etcd

| Collector | Question | Views | Collection | PG versions |
|---|---|---|---|---|
| `cluster` (`ClusterCollector`, [cluster.go](../internal/collector/cluster.go)) | Does the HA cluster have exactly one leader, are all members running, and is the etcd quorum healthy? | none — Patroni REST API / patronictl and etcd endpoints | 60s (medium group); no-op for providers that are not configured | any (does not query PostgreSQL) |

## pooler

_No collectors in this tier yet._

## os

| Collector | Question | Views | Collection | PG versions |
|---|---|---|---|---|
| `os` (`OSCollector`, [os.go](../internal/collector/os.go)) | Is the host running out of CPU, memory or disk, independent of what PostgreSQL reports? | none — pgpulse-agent HTTP endpoint, or local /proc when the DSN host is this machine | 10s (high group); no-op when neither an agent URL nor a local DSN is configured | any (does not query PostgreSQL) |
| `os_sql` (`OSSQLCollector`, [os_sql.go](../internal/collector/os_sql.go)) | Is the database host short on CPU, memory or disk throughput, when no agent is installed? | pg_read_file() on /proc/meminfo, /proc/uptime, /proc/loadavg, /proc/stat, /proc/diskstats | 60s (medium group); registered only when os_metrics_method is "sql" | 14+ (no version gate; needs pg_read_server_files) |

## environment

_No collectors in this tier yet._
