# M1_03 — Requirements: Progress Monitoring + Checkpoint/BGWriter

**Iteration:** M1_03
**Type:** Feature — new collectors (progress + checkpoint/bgwriter)
**Depends on:** M1_02b committed (replication collectors + InstanceContext interface)
**Estimated effort:** Agent Teams with 2 specialists, ~50 minutes
**Date:** 2026-02-26

---

## Goal

1. Port PGAM queries Q42–Q47 into three progress collector files (six independent
   collector structs) covering VACUUM, CLUSTER, CREATE INDEX, ANALYZE, BASEBACKUP,
   and COPY progress monitoring.

2. Add a new checkpoint/bgwriter stats collector (not from PGAM — new feature)
   that emits both absolute counters and per-second rates, with a version gate
   for the PG 17 split of pg_stat_bgwriter → pg_stat_checkpointer.

---

## Requirements

### R1: VacuumProgressCollector (progress_vacuum.go)

**PGAM source:** Q42
**Role:** Both primary and replica
**Interval:** 10 seconds
**Struct:** VacuumProgressCollector (own Name, Collect, Interval)

**Metrics (per operation in progress):**

| Metric | Type | Labels |
|--------|------|--------|
| `pgpulse.progress.vacuum.heap_blks_total` | float64 | pid, datname, table_name, phase |
| `pgpulse.progress.vacuum.heap_blks_scanned` | float64 | pid, datname, table_name, phase |
| `pgpulse.progress.vacuum.heap_blks_vacuumed` | float64 | pid, datname, table_name, phase |
| `pgpulse.progress.vacuum.index_vacuum_count` | float64 | pid, datname, table_name, phase |
| `pgpulse.progress.vacuum.max_dead_tuples` | float64 | pid, datname, table_name, phase |
| `pgpulse.progress.vacuum.num_dead_tuples` | float64 | pid, datname, table_name, phase |
| `pgpulse.progress.vacuum.completion_pct` | float64 | pid, datname, table_name, phase |

completion_pct = heap_blks_scanned / heap_blks_total * 100 (0 when total = 0).

### R2: ClusterProgressCollector + AnalyzeProgressCollector (progress_maintenance.go)

**PGAM source:** Q43 (CLUSTER/VACUUM FULL), Q45 (ANALYZE)
**Role:** Both primary and replica
**Interval:** 10 seconds each
**Structs:** Two independent collectors in one file

**ClusterProgressCollector metrics:**

| Metric | Labels |
|--------|--------|
| `pgpulse.progress.cluster.heap_tuples_scanned` | pid, datname, table_name, command, phase |
| `pgpulse.progress.cluster.heap_tuples_written` | pid, datname, table_name, command, phase |
| `pgpulse.progress.cluster.heap_blks_total` | pid, datname, table_name, command, phase |
| `pgpulse.progress.cluster.heap_blks_scanned` | pid, datname, table_name, command, phase |
| `pgpulse.progress.cluster.index_rebuild_count` | pid, datname, table_name, command, phase |
| `pgpulse.progress.cluster.completion_pct` | pid, datname, table_name, command, phase |

completion_pct = heap_blks_scanned / heap_blks_total * 100.

**AnalyzeProgressCollector metrics:**

| Metric | Labels |
|--------|--------|
| `pgpulse.progress.analyze.sample_blks_total` | pid, datname, table_name, phase |
| `pgpulse.progress.analyze.sample_blks_scanned` | pid, datname, table_name, phase |
| `pgpulse.progress.analyze.ext_stats_total` | pid, datname, table_name, phase |
| `pgpulse.progress.analyze.ext_stats_computed` | pid, datname, table_name, phase |
| `pgpulse.progress.analyze.child_tables_total` | pid, datname, table_name, phase |
| `pgpulse.progress.analyze.child_tables_done` | pid, datname, table_name, phase |
| `pgpulse.progress.analyze.completion_pct` | pid, datname, table_name, phase |

completion_pct = sample_blks_scanned / sample_blks_total * 100.

### R3: CreateIndexProgressCollector + BasebackupProgressCollector + CopyProgressCollector (progress_operations.go)

**PGAM source:** Q44 (CREATE INDEX), Q46 (BASEBACKUP), Q47 (COPY)
**Role:** Both primary and replica
**Interval:** 10 seconds each
**Structs:** Three independent collectors in one file

**CreateIndexProgressCollector metrics:**

| Metric | Labels |
|--------|--------|
| `pgpulse.progress.create_index.blocks_total` | pid, datname, table_name, index_name, command, phase |
| `pgpulse.progress.create_index.blocks_done` | pid, datname, table_name, index_name, command, phase |
| `pgpulse.progress.create_index.tuples_total` | pid, datname, table_name, index_name, command, phase |
| `pgpulse.progress.create_index.tuples_done` | pid, datname, table_name, index_name, command, phase |
| `pgpulse.progress.create_index.lockers_total` | pid, datname, table_name, index_name, command, phase |
| `pgpulse.progress.create_index.lockers_done` | pid, datname, table_name, index_name, command, phase |
| `pgpulse.progress.create_index.partitions_total` | pid, datname, table_name, index_name, command, phase |
| `pgpulse.progress.create_index.partitions_done` | pid, datname, table_name, index_name, command, phase |
| `pgpulse.progress.create_index.completion_pct` | pid, datname, table_name, index_name, command, phase |

completion_pct = blocks_done / blocks_total * 100.

**BasebackupProgressCollector metrics:**

| Metric | Labels |
|--------|--------|
| `pgpulse.progress.basebackup.backup_total` | pid, usename, app_name, client_addr, phase |
| `pgpulse.progress.basebackup.backup_streamed` | pid, usename, app_name, client_addr, phase |
| `pgpulse.progress.basebackup.tablespaces_total` | pid, usename, app_name, client_addr, phase |
| `pgpulse.progress.basebackup.tablespaces_streamed` | pid, usename, app_name, client_addr, phase |
| `pgpulse.progress.basebackup.completion_pct` | pid, usename, app_name, client_addr, phase |

completion_pct = backup_streamed / backup_total * 100.

**CopyProgressCollector metrics:**

| Metric | Labels |
|--------|--------|
| `pgpulse.progress.copy.bytes_processed` | pid, datname, table_name, command, type, phase |
| `pgpulse.progress.copy.bytes_total` | pid, datname, table_name, command, type, phase |
| `pgpulse.progress.copy.tuples_processed` | pid, datname, table_name, command, type, phase |
| `pgpulse.progress.copy.tuples_excluded` | pid, datname, table_name, command, type, phase |
| `pgpulse.progress.copy.completion_pct` | pid, datname, table_name, command, type, phase |

completion_pct = bytes_processed / bytes_total * 100.

### R4: CheckpointCollector (checkpoint.go)

**PGAM source:** Not in PGAM — new feature for PGPulse
**Role:** Both primary and replica
**Interval:** 60 seconds
**Stateful:** Yes — maintains previous snapshot for delta/rate computation

**Version gate required:**

| PG Version | Source View(s) |
|------------|---------------|
| 14–16 | `pg_stat_bgwriter` (combined — checkpoint + bgwriter columns) |
| 17+ | `pg_stat_checkpointer` + `pg_stat_bgwriter` (reduced) |

**Absolute counter metrics:**

| Metric | Source (PG ≤ 16) | Source (PG ≥ 17) |
|--------|-------------------|-------------------|
| `pgpulse.checkpoint.timed` | checkpoints_timed | num_timed |
| `pgpulse.checkpoint.requested` | checkpoints_req | num_requested |
| `pgpulse.checkpoint.write_time_ms` | checkpoint_write_time | write_time |
| `pgpulse.checkpoint.sync_time_ms` | checkpoint_sync_time | sync_time |
| `pgpulse.checkpoint.buffers_written` | buffers_checkpoint | buffers_written |
| `pgpulse.bgwriter.buffers_clean` | buffers_clean | buffers_clean |
| `pgpulse.bgwriter.maxwritten_clean` | maxwritten_clean | maxwritten_clean |
| `pgpulse.bgwriter.buffers_alloc` | buffers_alloc | buffers_alloc |
| `pgpulse.bgwriter.buffers_backend` | buffers_backend | — (PG ≤ 16 only) |
| `pgpulse.bgwriter.buffers_backend_fsync` | buffers_backend_fsync | — (PG ≤ 16 only) |
| `pgpulse.checkpoint.restartpoints_timed` | — | restartpoints_timed (PG ≥ 17 only) |
| `pgpulse.checkpoint.restartpoints_done` | — | restartpoints_done (PG ≥ 17 only) |
| `pgpulse.checkpoint.restartpoints_requested` | — | restartpoints_requested (PG ≥ 17 only) |

**Per-second rate metrics (computed from deltas):**
- `pgpulse.checkpoint.timed_per_second`
- `pgpulse.checkpoint.requested_per_second`
- `pgpulse.checkpoint.buffers_written_per_second`
- `pgpulse.bgwriter.buffers_clean_per_second`
- `pgpulse.bgwriter.buffers_alloc_per_second`
- `pgpulse.bgwriter.buffers_backend_per_second` (PG ≤ 16 only)

**Rate computation rules:**
- First cycle: emit only absolute counters. Store snapshot.
- Subsequent cycles: emit absolute counters AND per-second rates.
- Stats reset detection: if any current counter < prev counter, reset snapshot, skip rates.
- Guard against zero elapsed time.
- Use `sync.Mutex` to protect prev snapshot.

### R5: Collector Registration

All collectors registered explicitly in main.go (NOT via init()/RegisterCollector()):
- `NewVacuumProgressCollector(instanceID, pgVersion)`
- `NewClusterProgressCollector(instanceID, pgVersion)`
- `NewAnalyzeProgressCollector(instanceID, pgVersion)`
- `NewCreateIndexProgressCollector(instanceID, pgVersion)`
- `NewBasebackupProgressCollector(instanceID, pgVersion)`
- `NewCopyProgressCollector(instanceID, pgVersion)`
- `NewCheckpointCollector(instanceID, pgVersion)`

### R6: Unit Tests

**progress_vacuum_test.go:**
- TestVacuumProgress_NameAndInterval
- TestVacuumProgress_Integration (//go:build integration stub)

**progress_maintenance_test.go:**
- TestClusterProgress_NameAndInterval
- TestAnalyzeProgress_NameAndInterval
- TestClusterProgress_Integration (stub)
- TestAnalyzeProgress_Integration (stub)

**progress_operations_test.go:**
- TestCreateIndexProgress_NameAndInterval
- TestBasebackupProgress_NameAndInterval
- TestCopyProgress_NameAndInterval
- Integration stubs for all three

**checkpoint_test.go:**
- TestCheckpoint_NameAndInterval
- TestCheckpoint_GateSelectPG14 / PG17
- TestCheckpoint_FirstCycleNoRates
- TestCheckpoint_SecondCycleEmitsRates
- TestCheckpoint_StatsResetSkipsRates
- TestCheckpoint_PG16NoRestartpoints / PG17HasRestartpoints
- TestCheckpoint_ZeroElapsedSafe
- TestCheckpoint_Integration (stub)

### R7: Validation Gates

- `go build ./...`, `go vet ./...`, `golangci-lint run` pass
- `go test ./internal/collector/...` passes
- No SQL string concatenation
- COALESCE on nullable columns and regclass casts
- -1 sentinel for version-unavailable checkpoint columns

---

## Explicitly Deferred

| Item | Reason | When |
|------|--------|------|
| pg_stat_io | PG 16+ only, high cardinality, needs granularity design | M1_03b |
| Q41 — Logical replication sync | Requires PerDatabaseCollector interface | Later |
| Alert rules for checkpoint/progress | M4 scope | M4 |

---

## Shared Rules for All Progress Collectors

- SQL as const string — NEVER fmt.Sprintf
- Use `COALESCE(p.relid::regclass::text, 'oid:' || p.relid::text)` for table names
- Empty result sets are normal — return empty slice, not error
- completion_pct = 0 when total = 0 (guard division by zero)
- All queries JOIN `pg_stat_progress_*` with `pg_stat_activity` on pid
- Use queryContext(ctx) for 5s timeout
- No version gates needed — all views available on PG 14+
