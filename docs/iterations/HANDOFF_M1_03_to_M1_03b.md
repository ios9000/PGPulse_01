# PGPulse — Iteration Handoff: M1_03 → M1_03b

> **Purpose:** Upload this file when starting the next Claude.ai chat.
> Contains EVERYTHING needed to begin M1_03b without re-discovery.
> **Created:** 2026-02-26 (end of M1_03)

---

## DO NOT RE-DISCUSS

These decisions are final. The new chat must not revisit them:

1. **Stack**: Go 1.24.0, pgx v5.8.0, chi v5, koanf, slog, testcontainers-go
2. **Architecture**: Single binary, version-adaptive SQL via Gate pattern, Collector interface with InstanceContext
3. **Granularity**: One file = one collector = one struct implementing Collector. No merging. (Exception: progress collectors grouped by operational similarity — 6 structs in 3 files.)
4. **Module ownership**: Collector Agent owns internal/collector/* and internal/version/*
5. **Agent Teams bash bug**: Claude Code cannot run bash on Windows. Agents create files only. Developer runs go build/test/commit manually.
6. **Go module path**: `github.com/ios9000/PGPulse_01`
7. **Project path**: `C:\Users\Archer\Projects\PGPulse_01`
8. **PG version support**: 14, 15, 16, 17 (18 optional)
9. **Monitoring user**: pg_monitor role, never superuser
10. **OS metrics**: via Go agent (M6), NEVER via COPY TO PROGRAM
11. **Metric naming**: `pgpulse.<category>.<metric>` with labels as map[string]string
12. **Statement timeout**: 5s for live dashboard collectors via context.WithTimeout in queryContext()
13. **golangci-lint**: v2.10.1. Config requires `version: "2"` field. `gosimple` removed (merged into `staticcheck`).
14. **Docker Desktop**: not available on developer workstation. Integration tests run in CI only. Unit tests with mocks work locally.
15. **InstanceContext SSoT**: Orchestrator queries `pg_is_in_recovery()` once per cycle, passes `InstanceContext{IsRecovery: bool}` to all collectors. PG version stays in Base (structural/immutable). IsRecovery is in InstanceContext (dynamic/per-cycle).
16. **Collector registration**: **Explicit in main.go, NOT via init()/RegisterCollector() auto-registration.**
17. **Logical replication Q41**: Deferred — requires per-database connections (PerDatabaseCollector interface).
18. **Checkpoint/bgwriter version gate**: PG ≤ 16 queries pg_stat_bgwriter (combined). PG ≥ 17 queries pg_stat_checkpointer CROSS JOIN pg_stat_bgwriter (reduced). Uses -1 sentinel for unavailable columns.
19. **Stateful collector pattern**: CheckpointCollector introduced `computeMetrics()` pure function for testable rate computation from cumulative counters. Protected by sync.Mutex. Detects stats reset via counter decrease.
20. **Column name**: PG 17 pg_stat_checkpointer uses `restartpoints_req` (NOT `restartpoints_requested`).

---

## What Exists After M1_03

### Repository Structure (collector-related)

```
internal/
├── collector/
│   ├── collector.go              ← interfaces: MetricPoint, InstanceContext, Collector, MetricStore, AlertEvaluator
│   ├── base.go                   ← shared Base struct, point(), queryContext(), constants
│   ├── server_info.go            ← Q2,Q3,Q9,Q10: start time, uptime, recovery, backup
│   ├── connections.go            ← Q11-Q13: per-state counts, max, reserved, utilization
│   ├── cache.go                  ← Q14: global cache hit ratio
│   ├── transactions.go           ← Q15: per-DB commit ratio + deadlocks
│   ├── database_sizes.go         ← Q16: per-DB size bytes
│   ├── settings.go               ← Q17: track_io_timing, shared_buffers, etc.
│   ├── extensions.go             ← Q18-Q19: pgss presence, fill%, stats_reset
│   ├── replication_lag.go        ← Q37,Q38: byte + time lag per replica (primary only)
│   ├── replication_slots.go      ← Q40: WAL retention per slot, version-gated PG 14/15/16+
│   ├── replication_status.go     ← Q20 (primary: active replicas), Q21 (replica: WAL receiver)
│   ├── progress_vacuum.go        ← Q42: VACUUM progress + completionPct() helper [M1_03]
│   ├── progress_maintenance.go   ← Q43,Q45: CLUSTER + ANALYZE progress [M1_03]
│   ├── progress_operations.go    ← Q44,Q46,Q47: CREATE INDEX + BASEBACKUP + COPY progress [M1_03]
│   ├── checkpoint.go             ← Checkpoint/BGWriter stats, stateful, version-gated PG≤16/≥17 [M1_03]
│   ├── registry.go               ← RegisterCollector(), CollectAll() with partial-failure
│   ├── testutil_test.go          ← setupPG(), metric helpers
│   ├── server_info_test.go
│   ├── connections_test.go
│   ├── cache_test.go
│   ├── transactions_test.go
│   ├── database_sizes_test.go
│   ├── settings_test.go
│   ├── extensions_test.go
│   ├── registry_test.go
│   ├── replication_lag_test.go
│   ├── replication_slots_test.go
│   ├── replication_status_test.go
│   ├── progress_vacuum_test.go   ← name/interval + completionPct() tests [M1_03]
│   ├── progress_maintenance_test.go ← cluster + analyze name/interval [M1_03]
│   ├── progress_operations_test.go  ← create_index + basebackup + copy name/interval [M1_03]
│   └── checkpoint_test.go        ← 9 unit tests: gate, rates, reset, conditional metrics [M1_03]
│
└── version/
    ├── version.go                 ← PGVersion, Detect(), AtLeast()
    └── gate.go                    ← Gate, SQLVariant, VersionRange, Select()
```

### Key Interfaces (unchanged from M1_02)

```go
// InstanceContext holds per-scrape-cycle metadata.
type InstanceContext struct {
    IsRecovery bool
}

type MetricPoint struct {
    InstanceID string
    Metric     string
    Value      float64
    Labels     map[string]string
    Timestamp  time.Time
}

type Collector interface {
    Name() string
    Collect(ctx context.Context, conn *pgx.Conn, ic InstanceContext) ([]MetricPoint, error)
    Interval() time.Duration
}
```

### Base Struct Pattern (from base.go)

```go
type Base struct {
    instanceID string
    pgVersion  version.PGVersion
    interval   time.Duration
}

func newBase(instanceID string, v version.PGVersion, interval time.Duration) Base
func (b *Base) point(metric string, value float64, labels map[string]string) MetricPoint
func (b *Base) Interval() time.Duration
func queryContext(ctx context.Context) (context.Context, context.CancelFunc) // 5s timeout
```

### Shared Helper (from progress_vacuum.go)

```go
// completionPct computes percentage safely. Returns 0 when total is 0.
func completionPct(done, total float64) float64
```

### Stateful Pattern (from checkpoint.go — reference for io_stats if needed)

```go
type CheckpointCollector struct {
    Base
    sqlGate  version.Gate
    mu       sync.Mutex
    prev     *checkpointSnapshot
    prevTime time.Time
}

// Pure function — testable without PG connection
func (c *CheckpointCollector) computeMetrics(curr checkpointSnapshot,
    prev *checkpointSnapshot, prevTime time.Time, now time.Time) []MetricPoint
```

### Version Gate (from gate.go — DO NOT MODIFY)

```go
type VersionRange struct {
    MinMajor int
    MinMinor int
    MaxMajor int
    MaxMinor int
}

type SQLVariant struct {
    Range VersionRange
    SQL   string
}

type Gate struct {
    Name     string
    Variants []SQLVariant
}

func (g Gate) Select(v PGVersion) (string, bool)
```

---

## Build & Test Status After M1_03

| Check | Result |
|-------|--------|
| `go build ./...` | ✅ Pass |
| `go vet ./...` | ✅ Pass |
| `golangci-lint run` (v2.10.1) | ✅ 0 issues |
| Unit tests | ✅ 28 pass |
| Integration tests | ⏭️ 10 skipped locally — Docker Desktop not available |

---

## Query Porting Progress

| Source | Queries | Status |
|--------|---------|--------|
| analiz2.php Q1 | PG version string | ✅ Done (M0) |
| analiz2.php Q2–Q3, Q9–Q10 | Server info | ✅ Done (M1_01) |
| analiz2.php Q4–Q8 | OS metrics | ⏭️ Deferred to M6 |
| analiz2.php Q11–Q13 | Connections | ✅ Done (M1_01) |
| analiz2.php Q14 | Cache hit ratio | ✅ Done (M1_01) |
| analiz2.php Q15 | Transactions | ✅ Done (M1_01) |
| analiz2.php Q16 | Database sizes | ✅ Done (M1_01) |
| analiz2.php Q17 | Settings | ✅ Done (M1_01) |
| analiz2.php Q18–Q19 | Extensions/pgss | ✅ Done (M1_01) |
| analiz2.php Q20–Q21 | Replication status | ✅ Done (M1_02b) |
| analiz2.php Q22–Q35 | OS/cluster/overview | 🔲 M6/later |
| analiz2.php Q36, Q39 | Replication PG < 10 | ⏭️ Skipped (below min) |
| analiz2.php Q37–Q38 | Replication lag | ✅ Done (M1_02b) |
| analiz2.php Q40 | Replication slots | ✅ Done (M1_02b) |
| analiz2.php Q41 | Logical replication | ⏭️ Deferred (PerDatabaseCollector) |
| analiz2.php Q42–Q47 | Progress monitoring | ✅ Done (M1_03) |
| analiz2.php Q48–Q52 | pg_stat_statements | 🔲 M1_04 |
| analiz2.php Q53–Q58 | Locks & wait events | 🔲 M1_05 |
| analiz_db.php Q1–Q18 | Per-DB analysis | 🔲 Later milestone |
| — (new) | Checkpoint/BGWriter | ✅ Done (M1_03) |
| **— (new)** | **pg_stat_io** | **🔲 M1_03b — THIS ITERATION** |
| **Total** | **76 PGAM + new** | **24/76 PGAM ported, 1 new done, 1 new pending** |

---

## Next Task: M1_03b — pg_stat_io Collector

### Goal

Add a new `IOStatsCollector` that queries `pg_stat_io` (PG ≥ 16 only). This view provides I/O statistics broken down by backend type, I/O object, and I/O context. Not present in PGAM — a new PGPulse feature.

### Why This Exists

`pg_stat_io` (added PG 16) replaces the coarse `buffers_backend`/`buffers_backend_fsync` columns that were removed from `pg_stat_bgwriter` in PG 17. It provides much more granular I/O attribution: which process types are doing reads/writes/extends/fsyncs, on which object types, in which contexts.

### pg_stat_io View Structure (PG 16+)

```sql
SELECT
    backend_type,   -- 'autovacuum worker', 'client backend', 'background writer', 'checkpointer', etc.
    object,         -- 'relation', 'temp relation'
    context,        -- 'normal', 'vacuum', 'bulkread', 'bulkwrite'
    reads, read_time,
    writes, write_time,
    extends, extend_time,
    hits,
    evictions,
    reuses,
    fsyncs, fsync_time,
    stats_reset
FROM pg_stat_io
```

**Row cardinality:** Bounded at ~30–50 rows on a typical server. One row per (backend_type × object × context) combination. PG omits rows for combinations that never occur (e.g., checkpointer never touches temp relations).

**NULL columns:** Some I/O operations are never performed by certain backend types. For example, `reads` is NULL for background writer (it never reads). `fsyncs` is NULL for temp relations (never fsynced). These NULLs must be handled — emit nothing (skip), not zero.

### Proposed SQL

```sql
SELECT
    backend_type,
    object,
    context,
    COALESCE(reads, -1),
    COALESCE(read_time, -1),
    COALESCE(writes, -1),
    COALESCE(write_time, -1),
    COALESCE(extends, -1),
    COALESCE(extend_time, -1),
    COALESCE(hits, -1),
    COALESCE(evictions, -1),
    COALESCE(reuses, -1),
    COALESCE(fsyncs, -1),
    COALESCE(fsync_time, -1)
FROM pg_stat_io
```

Use -1 sentinel for NULL columns (consistent with checkpoint.go pattern). Only emit metrics where value ≥ 0.

### Design Decisions to Make in M1_03b

1. **Stateless or stateful?**
   - pg_stat_io contains cumulative counters (like checkpoint)
   - Option A: Stateless — emit raw counters, let storage compute rates (simpler, consistent with Prometheus convention)
   - Option B: Stateful — compute rates like CheckpointCollector (richer metrics, but ~50 rows × multiple counters = complex state)
   - **Recommendation: Stateless.** The row cardinality (30–50 rows) makes per-row delta tracking complex and fragile. Raw counters are sufficient — TimescaleDB rate() handles the rest. Checkpoint was worth making stateful because it's a single row with high-value rates. IO stats is many rows where the raw counters are already informative.

2. **Metric naming:**
   - `pgpulse.io.reads`, `pgpulse.io.read_time`, `pgpulse.io.writes`, etc.
   - Labels: `{backend_type: "client backend", object: "relation", context: "normal"}`

3. **Column availability varies by PG minor version within 16.x/17.x?**
   - No — the view schema is stable within major versions. Only needs a simple AtLeast(16, 0) check, not a version gate.

### Complexity Assessment

**Low.** Single query, no version gate (just an availability check), no state, bounded cardinality. The main subtlety is NULL handling for columns that don't apply to certain backend types.

### Suggested File

| File | Content | Version Check | Interval |
|------|---------|---------------|----------|
| `io_stats.go` | IOStatsCollector | PG ≥ 16 via `b.pgVersion.AtLeast(16, 0)` | 60s |
| `io_stats_test.go` | Tests | Gate/availability + name/interval | — |

### Collector Behavior

```
if !b.pgVersion.AtLeast(16, 0) {
    return nil, nil  // view doesn't exist, not an error
}
```

- Query all rows from pg_stat_io
- For each row, emit metrics where value ≥ 0 (skip -1 sentinels)
- Labels: backend_type, object, context
- Metric prefix: `pgpulse.io.<metric>`
- Empty result set → empty slice (not error)
- Uses `_ InstanceContext` (works on both primary and replica)

### Tests for M1_03b

| Test | Assertion |
|------|-----------|
| `TestIOStats_NameAndInterval` | Name="io_stats", Interval=60s |
| `TestIOStats_PG15ReturnsNil` | PGVersion{Major:15} → Collect returns nil, nil |
| `TestIOStats_PG16Available` | PGVersion{Major:16} → does not short-circuit |
| `TestIOStats_NullHandling` | -1 sentinel values → metric not emitted |
| `TestIOStats_Integration` | `//go:build integration` stub |

---

## Known Issues Affecting M1_03b

1. **Docker Desktop unavailable** — integration tests CI-only.
2. **Registration is explicit in main.go** — do NOT use init()/RegisterCollector() pattern.
3. **pg_stat_io NULLs** — some columns are NULL for certain backend_type/object/context combinations. Use COALESCE to -1, then skip in Go.
4. **No `stats_reset` per row** — pg_stat_io has a single `stats_reset` column. Could optionally emit it once, or ignore it.

---

## Workflow for M1_03b

```
1. New Claude.ai chat (this handoff uploaded):
   - Confirm stateless vs stateful decision
   - Review metric naming and NULL handling
   - Produce requirements.md, design.md, team-prompt.md

2. Developer: copy to docs/iterations/M1_03b_.../

3. Claude Code: paste team-prompt.md → agents create files

4. Developer: go build → go vet → golangci-lint → go test

5. Claude.ai: create session-log.md

6. Developer: git commit + push
```

---

## Environment Reference

| Tool | Version | Notes |
|------|---------|-------|
| Go | 1.24.0 windows/amd64 | |
| golangci-lint | 2.10.1 | v2 config: requires `version: "2"`, no `gosimple` |
| Claude Code | 2.1.53 | Bash broken on Windows — file creation only |
| testcontainers-go | 0.40.0 | Requires Docker Desktop (unavailable) |
| Docker Desktop | Not installed | Integration tests → CI only |
| Git | 2.52.0 | |
| Node.js | 22.14.0 | For Claude Code |
