# M1_03 — Agent Teams Prompt: Progress Monitoring + Checkpoint/BGWriter

> **Mode:** Agent Teams — 2 specialists
> **Model:** claude-opus-4-6
> **Estimated tokens:** ~500K input, ~250K output
> **Estimated time:** ~45 minutes
> **Prerequisite:** M1_02b committed (replication collectors + InstanceContext)

---

## Prompt (paste into Claude Code)

```
Read CLAUDE.md for project context, then read
docs/iterations/M1_03_02262026_progress-checkpoint-bgwriter/design.md
for the detailed implementation plan.

Build progress monitoring and checkpoint/bgwriter stats collectors for PGPulse.
These port PGAM queries Q42–Q47 and add new checkpoint/bgwriter metrics.

Create a team of 2 specialists:

COLLECTOR AGENT:
Create four new files in internal/collector/:

1. progress_vacuum.go — VacuumProgressCollector (Q42)
   - Struct embeds Base, interval 10s
   - Name() returns "progress_vacuum"
   - Collect(): query pg_stat_progress_vacuum JOIN pg_stat_activity
   - Metrics: heap_blks_total, heap_blks_scanned, heap_blks_vacuumed,
     index_vacuum_count, max_dead_tuples, num_dead_tuples, completion_pct
   - Labels: pid, datname, table_name, phase
   - completion_pct = heap_blks_scanned / heap_blks_total * 100 (0 when total=0)
   - ALSO define a package-level helper:
     func completionPct(done, total float64) float64
     This is used by ALL progress collectors.
   - Use COALESCE(p.relid::regclass::text, 'oid:' || p.relid::text) for table names
   - Empty results → return empty slice, not error
   - Use queryContext(ctx) for 5s timeout, c.point() for metrics
   - SQL as const string, NEVER fmt.Sprintf
   - Uses _ InstanceContext (not role-dependent)

2. progress_maintenance.go — TWO collectors in one file:
   a) ClusterProgressCollector (Q43)
      - Name() returns "progress_cluster", interval 10s
      - Query pg_stat_progress_cluster JOIN pg_stat_activity
      - Metrics: heap_tuples_scanned, heap_tuples_written, heap_blks_total,
        heap_blks_scanned, index_rebuild_count, completion_pct
      - Labels: pid, datname, table_name, command, phase
      - completion_pct = heap_blks_scanned / heap_blks_total

   b) AnalyzeProgressCollector (Q45)
      - Name() returns "progress_analyze", interval 10s
      - Query pg_stat_progress_analyze JOIN pg_stat_activity
      - Metrics: sample_blks_total, sample_blks_scanned, ext_stats_total,
        ext_stats_computed, child_tables_total, child_tables_done, completion_pct
      - Labels: pid, datname, table_name, phase, current_child
      - completion_pct = sample_blks_scanned / sample_blks_total

3. progress_operations.go — THREE collectors in one file:
   a) CreateIndexProgressCollector (Q44)
      - Name() returns "progress_create_index", interval 10s
      - Query pg_stat_progress_create_index JOIN pg_stat_activity
      - Metrics: blocks_total, blocks_done, tuples_total, tuples_done,
        lockers_total, lockers_done, partitions_total, partitions_done, completion_pct
      - Labels: pid, datname, table_name, index_name, command, phase
      - completion_pct = blocks_done / blocks_total

   b) BasebackupProgressCollector (Q46)
      - Name() returns "progress_basebackup", interval 10s
      - Query pg_stat_progress_basebackup JOIN pg_stat_activity
      - Metrics: backup_total, backup_streamed, tablespaces_total,
        tablespaces_streamed, completion_pct
      - Labels: pid, usename, app_name, client_addr, phase
      - completion_pct = backup_streamed / backup_total

   c) CopyProgressCollector (Q47)
      - Name() returns "progress_copy", interval 10s
      - Query pg_stat_progress_copy JOIN pg_stat_activity
      - Metrics: bytes_processed, bytes_total, tuples_processed,
        tuples_excluded, completion_pct
      - Labels: pid, datname, table_name, command, type
      - Note: pg_stat_progress_copy does NOT have a phase column
      - completion_pct = bytes_processed / bytes_total

4. checkpoint.go — STATEFUL collector (new pattern for this project)
   - CheckpointCollector struct embeds Base + sqlGate + sync.Mutex + prev snapshot
   - Name() returns "checkpoint", interval 60s
   - checkpointSnapshot struct with 13 float64 fields (see design.md)

   VERSION GATE — two SQL variants, both return exactly 13 columns:

   PG 14–16 (VersionRange MinMajor:14 MaxMajor:16 MaxMinor:99):
   SELECT checkpoints_timed, checkpoints_req, checkpoint_write_time,
     checkpoint_sync_time, buffers_checkpoint, buffers_clean, maxwritten_clean,
     buffers_alloc, buffers_backend, buffers_backend_fsync,
     -1 AS restartpoints_timed, -1 AS restartpoints_done, -1 AS restartpoints_req
   FROM pg_stat_bgwriter

   PG 17+ (VersionRange MinMajor:17 MaxMajor:99 MaxMinor:99):
   SELECT c.num_timed AS checkpoints_timed, c.num_requested AS checkpoints_req,
     c.write_time AS checkpoint_write_time, c.sync_time AS checkpoint_sync_time,
     c.buffers_written AS buffers_checkpoint,
     b.buffers_clean, b.maxwritten_clean, b.buffers_alloc,
     -1 AS buffers_backend, -1 AS buffers_backend_fsync,
     c.restartpoints_timed, c.restartpoints_done, c.restartpoints_req
   FROM pg_stat_checkpointer c CROSS JOIN pg_stat_bgwriter b

   Use -1 sentinel for unavailable columns (NOT 0, NOT NULL).

   STATEFUL RATE COMPUTATION:
   - Extract pure logic into testable method:
     func (c *CheckpointCollector) computeMetrics(curr checkpointSnapshot,
       prev *checkpointSnapshot, prevTime time.Time, now time.Time) []MetricPoint
   - Collect() calls QueryRow, Scan into snapshot, then computeMetrics(), then updates state
   - absolutePoints(snap) — always emitted:
     checkpoint.timed, checkpoint.requested, checkpoint.write_time_ms,
     checkpoint.sync_time_ms, checkpoint.buffers_written,
     bgwriter.buffers_clean, bgwriter.maxwritten_clean, bgwriter.buffers_alloc
     Conditional (value >= 0): bgwriter.buffers_backend, bgwriter.buffers_backend_fsync (PG ≤ 16)
     Conditional (value >= 0): checkpoint.restartpoints_* (PG ≥ 17)
   - ratePoints(curr, elapsedSec) — only when prev != nil and !isStatsReset:
     checkpoint.timed_per_second, checkpoint.requested_per_second,
     checkpoint.buffers_written_per_second, bgwriter.buffers_clean_per_second,
     bgwriter.buffers_alloc_per_second, bgwriter.buffers_backend_per_second (PG ≤ 16)
   - isStatsReset(curr) — returns true if any counter decreased
   - Protect prev/prevTime with sync.Mutex in Collect()
   - Guard against zero elapsed time in rate division

   Do NOT use init()/RegisterCollector(). Explicit registration in main.go.

ALL PROGRESS COLLECTORS SHARE:
- Embed Base, 10s interval
- Use _ InstanceContext (no role check — works on both primary and replica)
- SQL as const string
- COALESCE on nullable/regclass columns
- Empty result → empty slice (not error)
- Use completionPct() helper from progress_vacuum.go
- Metric prefix: pgpulse.progress.<operation>.<metric>
- Error wrap: fmt.Errorf("progress_<operation>: %w", err)

QA AGENT:
Create four test files in internal/collector/:

1. progress_vacuum_test.go
   - TestVacuumProgress_NameAndInterval: Name="progress_vacuum", Interval=10s
   - TestCompletionPct: test completionPct() helper:
     completionPct(0, 0) == 0
     completionPct(50, 100) == 50
     completionPct(100, 100) == 100
     completionPct(0, 100) == 0
   - TestVacuumProgress_Integration: //go:build integration stub with t.Skip

2. progress_maintenance_test.go
   - TestClusterProgress_NameAndInterval: Name="progress_cluster", Interval=10s
   - TestAnalyzeProgress_NameAndInterval: Name="progress_analyze", Interval=10s
   - Integration stubs for both

3. progress_operations_test.go
   - TestCreateIndexProgress_NameAndInterval: Name="progress_create_index", Interval=10s
   - TestBasebackupProgress_NameAndInterval: Name="progress_basebackup", Interval=10s
   - TestCopyProgress_NameAndInterval: Name="progress_copy", Interval=10s
   - Integration stubs for all three

4. checkpoint_test.go (MOST IMPORTANT — tests the new stateful pattern)
   - TestCheckpoint_NameAndInterval: Name="checkpoint", Interval=60s
   - TestCheckpoint_GateSelectPG14: Gate selects PG 14-16 variant
   - TestCheckpoint_GateSelectPG17: Gate selects PG 17+ variant
   - TestCheckpoint_FirstCycleNoRates:
     Call computeMetrics(curr, nil, time.Time{}, now)
     Assert: returns absolute metrics, NO _per_second metrics
   - TestCheckpoint_SecondCycleEmitsRates:
     prev = snapshot with lower values, curr = snapshot with higher values
     Call computeMetrics(curr, &prev, prevTime, now) where now is 60s after prevTime
     Assert: returns both absolute AND _per_second metrics
     Verify rate math: e.g., if timed went from 10 to 70 over 60s, rate = 1.0/s
   - TestCheckpoint_StatsResetSkipsRates:
     Create prev with values higher than curr (simulates pg_stat_reset)
     Call isStatsReset(curr) on collector where c.prev = &prev
     Assert: returns true
   - TestCheckpoint_PG16NoRestartpoints:
     Snapshot with restartpointsTimed = -1
     Call absolutePoints(snap), assert no restartpoint metrics in result
   - TestCheckpoint_PG17HasRestartpoints:
     Snapshot with restartpointsTimed = 5.0
     Call absolutePoints(snap), assert restartpoint metrics present
   - TestCheckpoint_ZeroElapsedSafe:
     computeMetrics where prevTime == now (0 elapsed)
     Assert: no panic, no rate metrics emitted
   - TestCheckpoint_Integration: //go:build integration stub

   IMPORTANT: For rate/delta tests, call computeMetrics() and absolutePoints()
   directly with crafted checkpointSnapshot values. These methods are in the
   same package so they're accessible. No PG connection needed.

COORDINATION:
- Collector Agent creates all 4 production files
- QA Agent creates all 4 test files
- Both can work in parallel
- QA Agent needs completionPct() and computeMetrics() to be accessible —
  they are (same package, unexported is fine)

CANNOT RUN BASH:
You cannot run shell commands on this platform (Windows bash bug).
Create files only. When both agents are done, list ALL created files
so the developer can run:

go build ./...
go vet ./...
golangci-lint run
go test ./internal/collector/...
```

---

## Developer Post-Session Checklist

After Agent Teams finish:

```bash
cd ~/Projects/PGPulse_01

# 1. Verify new files exist
ls internal/collector/progress_*.go internal/collector/checkpoint*.go

# 2. Build
go build ./...

# 3. Vet
go vet ./...

# 4. Lint
golangci-lint run

# 5. Unit tests
go test -v ./internal/collector/...

# 6. If all pass — commit
git add internal/collector/progress_*.go internal/collector/checkpoint*.go
git commit -m "feat(collector): add progress monitoring and checkpoint/bgwriter collectors

- VacuumProgressCollector (Q42): heap blocks, dead tuples, completion%
- ClusterProgressCollector (Q43): CLUSTER/VACUUM FULL progress
- AnalyzeProgressCollector (Q45): sample blocks, ext stats, child tables
- CreateIndexProgressCollector (Q44): blocks, tuples, lockers, partitions
- BasebackupProgressCollector (Q46): backup streamed, tablespaces
- CopyProgressCollector (Q47): bytes/tuples processed
- CheckpointCollector: version-gated (PG ≤16 vs ≥17), stateful delta/rate
  computation for checkpoint and bgwriter cumulative counters
- New pattern: stateful collector with snapshot + rate computation
- New helper: completionPct() shared by all progress collectors
- Version gate: pg_stat_bgwriter (combined) vs pg_stat_checkpointer + bgwriter

PGAM queries ported: Q42, Q43, Q44, Q45, Q46, Q47 (6 queries)
Running total: 24/76 queries ported (18 prev + 6 M1_03)"

git push origin main
```

## Success Criteria

| Check | Expected |
|-------|----------|
| `go build ./...` | ✅ No errors |
| `go vet ./...` | ✅ No warnings |
| `golangci-lint run` | ✅ 0 issues |
| `go test ./internal/collector/...` | ✅ All pass (unit tests) |
| 8 new files | 4 prod + 4 test |
| No existing files modified | ✅ Clean addition |
| No fmt.Sprintf in SQL | ✅ All SQL as string constants |
| COALESCE on regclass casts | ✅ |
| Version gate PG 14–17+ | ✅ 2 variants in checkpoint gate |
| completionPct() safe for zero | ✅ Returns 0 |
| computeMetrics() testable without PG | ✅ |
| Stats reset detection works | ✅ |
