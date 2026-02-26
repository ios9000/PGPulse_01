# PGPulse — Iteration Handoff: M1_02 → M1_03

> **Purpose:** Upload this file when starting the next Claude.ai chat.
> Contains EVERYTHING needed to begin M1_03 without re-discovery.
> **Created:** 2026-02-26 (end of M1_02)

---

## DO NOT RE-DISCUSS

These decisions are final. The new chat must not revisit them:

1. **Stack**: Go 1.24.0, pgx v5.8.0, chi v5, koanf, slog, testcontainers-go
2. **Architecture**: Single binary, version-adaptive SQL via Gate pattern, Collector interface with InstanceContext
3. **Granularity**: One file = one collector = one struct implementing Collector. No merging.
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
16. **Collector registration**: **Explicit in main.go, NOT via init()/RegisterCollector() auto-registration.** The strategy doc and CLAUDE.md references to auto-registration are incorrect. `RegisterCollector()` exists in registry.go but is not used by any collector. Future collectors follow the explicit pattern.
17. **Logical replication Q41**: Deferred — requires per-database connections (PerDatabaseCollector interface). Will be designed alongside analiz_db.php queries.
18. **Checkpoint/bgwriter stats**: Deferred from M1_01 to M1_03 because PG 17 splits pg_stat_bgwriter → pg_stat_checkpointer. Needs careful version gate.
19. **pg_stat_io**: Deferred from M1_01 to M1_03. PG 16+ only. Not in PGAM audit — new addition.

---

## What Exists After M1_02

### Repository Structure (collector-related)

```
internal/
├── collector/
│   ├── collector.go              ← interfaces: MetricPoint, InstanceContext, Collector, MetricStore, AlertEvaluator [M0 + M1_02a]
│   ├── base.go                   ← shared Base struct, point(), queryContext(), constants [M1_01]
│   ├── server_info.go            ← Q2,Q3,Q9,Q10: start time, uptime, recovery (from ic), backup [M1_01 + M1_02a]
│   ├── connections.go            ← Q11-Q13: per-state counts, max, reserved, utilization [M1_01]
│   ├── cache.go                  ← Q14: global cache hit ratio [M1_01]
│   ├── transactions.go           ← Q15: per-DB commit ratio + deadlocks [M1_01]
│   ├── database_sizes.go         ← Q16: per-DB size bytes [M1_01]
│   ├── settings.go               ← Q17: track_io_timing, shared_buffers, etc. [M1_01]
│   ├── extensions.go             ← Q18-Q19: pgss presence, fill%, stats_reset [M1_01]
│   ├── replication_lag.go        ← Q37,Q38: byte + time lag per replica (primary only) [M1_02b]
│   ├── replication_slots.go      ← Q40: WAL retention per slot, version-gated PG 14/15/16+ [M1_02b]
│   ├── replication_status.go     ← Q20 (primary: active replicas), Q21 (replica: WAL receiver) [M1_02b]
│   ├── registry.go               ← RegisterCollector(), CollectAll() with partial-failure [M1_01 + M1_02a]
│   ├── testutil_test.go          ← setupPG(), metric helpers [M1_01]
│   ├── server_info_test.go       ← PG14 + PG17 version gate tests + IsRecovery routing [M1_01 + M1_02a]
│   ├── connections_test.go       ← self-exclusion, utilization tests [M1_01]
│   ├── cache_test.go             ← hit ratio bounds [M1_01]
│   ├── transactions_test.go      ← per-DB labels [M1_01]
│   ├── database_sizes_test.go    ← size > 0 [M1_01]
│   ├── settings_test.go          ← bool conversion [M1_01]
│   ├── extensions_test.go        ← with/without pgss paths [M1_01]
│   ├── registry_test.go          ← mock-based, no Docker [M1_01 + M1_02a]
│   ├── replication_lag_test.go   ← skip-on-replica, name/interval (2 pass, 1 integration skip) [M1_02b]
│   ├── replication_slots_test.go ← gate selection PG14/15/16/17, name/interval (5 pass, 1 skip) [M1_02b]
│   └── replication_status_test.go ← name/interval (1 pass, 2 integration skip) [M1_02b]
│
└── version/
    ├── version.go                 ← PGVersion, Detect(), AtLeast() [M0]
    └── gate.go                    ← Gate, SQLVariant, VersionRange, Select() [M0]
```

### Key Interfaces

#### InstanceContext + Collector (from collector.go — CURRENT)

```go
// InstanceContext holds per-scrape-cycle metadata.
// Orchestrator queries pg_is_in_recovery() once, passes to all collectors.
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

#### Base Struct Pattern (from base.go)

```go
type Base struct {
    instanceID string
    pgVersion  version.PGVersion  // structural — set at construction, immutable
    interval   time.Duration
}

func newBase(instanceID string, v version.PGVersion, interval time.Duration) Base
func (b *Base) point(metric string, value float64, labels map[string]string) MetricPoint
// point() auto-prefixes "pgpulse.", fills InstanceID + Timestamp
func (b *Base) Interval() time.Duration
func queryContext(ctx context.Context) (context.Context, context.CancelFunc) // 5s timeout
```

#### Version Gate (from gate.go — DO NOT MODIFY)

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

#### Registry (from registry.go)

```go
func RegisterCollector(factory func(instanceID string, v version.PGVersion) Collector)
func CollectAll(ctx context.Context, conn *pgx.Conn, ic InstanceContext, collectors []Collector) ([]MetricPoint, []error)
```

**Note:** `RegisterCollector()` exists but collectors are registered explicitly in main.go, not via init(). Follow the explicit pattern for new collectors.

---

## Build & Test Status After M1_02

| Check | Result |
|-------|--------|
| `go build ./...` | ✅ Pass |
| `go vet ./...` | ✅ Pass |
| `golangci-lint run` (v2.10.1) | ✅ 0 issues |
| Unit tests | ✅ 13 pass (5 registry + 8 replication) |
| Integration tests | ❌ Skipped locally — Docker Desktop not available |

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
| **analiz2.php Q42–Q47** | **Progress monitoring** | **🔲 M1_03 — THIS ITERATION** |
| analiz2.php Q48–Q52 | pg_stat_statements | 🔲 M1_04 |
| analiz2.php Q53–Q58 | Locks & wait events | 🔲 M1_05 |
| analiz_db.php Q1–Q18 | Per-DB analysis | 🔲 Later milestone |
| **Total** | **76** | **18 done, 9 deferred/skipped, 49 remaining** |

---

## Next Task: M1_03 — Progress Monitoring + Checkpoint/BGWriter

### Goal

Port PGAM queries Q42–Q47 into progress monitoring collectors, plus add checkpoint and background writer stats that were deferred from M1_01.

### Scope Overview

**Progress monitoring (Q42–Q47):**
- VACUUM progress (Q42)
- CLUSTER/VACUUM FULL progress (Q43, PG ≥ 12)
- CREATE INDEX progress (Q44, PG ≥ 12)
- ANALYZE progress (Q45, PG ≥ 13)
- BASEBACKUP progress (Q46, PG ≥ 13)
- COPY progress (Q47, PG ≥ 14)

All progress queries use `pg_stat_progress_*` views joined with `pg_stat_activity`.

**Checkpoint/BGWriter stats (new — not in PGAM but needed):**
- `pg_stat_bgwriter` (PG ≤ 16): checkpoints_timed, checkpoints_req, buffers_checkpoint, buffers_clean, buffers_backend, etc.
- `pg_stat_checkpointer` (PG ≥ 17): split out from pg_stat_bgwriter
- `pg_stat_bgwriter` (PG ≥ 17): retains only bgwriter-specific columns

**pg_stat_io (PG ≥ 16 only, new — not in PGAM):**
- I/O statistics by backend type, object, context

### PGAM Queries Q42–Q47 (from PGAM_FEATURE_AUDIT.md in Project Knowledge)

**Q42 — VACUUM progress**
```sql
SELECT
    a.pid, a.datname, a.query,
    p.relid::regclass AS table_name,
    p.phase,
    p.heap_blks_total, p.heap_blks_scanned, p.heap_blks_vacuumed,
    p.index_vacuum_count, p.max_dead_tuples, p.num_dead_tuples
FROM pg_stat_progress_vacuum p
JOIN pg_stat_activity a ON a.pid = p.pid;
```

**Q43 — CLUSTER/VACUUM FULL progress (PG ≥ 12)**
```sql
SELECT
    a.pid, a.datname, a.query,
    p.relid::regclass AS table_name,
    p.command, p.phase,
    p.heap_tuples_scanned, p.heap_tuples_written,
    p.heap_blks_total, p.heap_blks_scanned,
    p.index_rebuild_count
FROM pg_stat_progress_cluster p
JOIN pg_stat_activity a ON a.pid = p.pid;
```

**Q44 — CREATE INDEX progress (PG ≥ 12)**
```sql
SELECT
    a.pid, a.datname, a.query,
    p.relid::regclass AS table_name,
    p.index_relid::regclass AS index_name,
    p.command, p.phase,
    p.lockers_total, p.lockers_done,
    p.blocks_total, p.blocks_done,
    p.tuples_total, p.tuples_done,
    p.partitions_total, p.partitions_done
FROM pg_stat_progress_create_index p
JOIN pg_stat_activity a ON a.pid = p.pid;
```

**Q45 — ANALYZE progress (PG ≥ 13)**
```sql
SELECT
    a.pid, a.datname, a.query,
    p.relid::regclass AS table_name,
    p.phase,
    p.sample_blks_total, p.sample_blks_scanned,
    p.ext_stats_total, p.ext_stats_computed,
    p.child_tables_total, p.child_tables_done,
    p.current_child_table_relid::regclass AS current_child
FROM pg_stat_progress_analyze p
JOIN pg_stat_activity a ON a.pid = p.pid;
```

**Q46 — BASEBACKUP progress (PG ≥ 13)**
```sql
SELECT
    a.pid, a.usename, a.application_name, a.client_addr,
    p.phase,
    p.backup_total, p.backup_streamed,
    p.tablespaces_total, p.tablespaces_streamed
FROM pg_stat_progress_basebackup p
JOIN pg_stat_activity a ON a.pid = p.pid;
```

**Q47 — COPY progress (PG ≥ 14)**
```sql
SELECT
    a.pid, a.datname, a.query,
    p.relid::regclass AS table_name,
    p.command, p.type,
    p.bytes_processed, p.bytes_total,
    p.tuples_processed, p.tuples_excluded
FROM pg_stat_progress_copy p
JOIN pg_stat_activity a ON a.pid = p.pid;
```

### Version Gates Required for M1_03

| Feature | PG 14 | PG 15 | PG 16 | PG 17 |
|---------|-------|-------|-------|-------|
| pg_stat_progress_vacuum | ✅ | ✅ | ✅ | ✅ |
| pg_stat_progress_cluster | ✅ | ✅ | ✅ | ✅ |
| pg_stat_progress_create_index | ✅ | ✅ | ✅ | ✅ |
| pg_stat_progress_analyze | ✅ | ✅ | ✅ | ✅ |
| pg_stat_progress_basebackup | ✅ | ✅ | ✅ | ✅ |
| pg_stat_progress_copy | ✅ | ✅ | ✅ | ✅ |
| pg_stat_bgwriter (combined) | ✅ | ✅ | ✅ | ❌ Split |
| pg_stat_checkpointer | ❌ | ❌ | ❌ | ✅ New view |
| pg_stat_bgwriter (reduced) | ❌ | ❌ | ❌ | ✅ Fewer columns |
| pg_stat_io | ❌ | ❌ | ✅ New view | ✅ |

### Complexity Assessment

**Progress collectors (Q42–Q47):** Low-medium. All use the same pattern: join progress view with pg_stat_activity. Since our minimum is PG 14, all progress views exist — no version gates needed for these. The main complexity is that each has different columns.

**Checkpoint/bgwriter:** Medium-high. PG 17 splits `pg_stat_bgwriter` into two views. Need a version gate: PG ≤ 16 queries one view, PG ≥ 17 queries two views. Column names differ.

**pg_stat_io:** Medium. PG 16+ only. Many rows (one per backend_type × object × context). Need to decide on metric granularity — emit per-row or aggregate.

### Suggested Collector Files for M1_03

| File | Queries | Interval | Notes |
|------|---------|----------|-------|
| `progress_vacuum.go` | Q42 | 10s | Always available (PG ≥ 9.6) |
| `progress_operations.go` | Q43, Q44, Q45, Q46, Q47 | 10s | All PG ≥ 14 — could be one file or split |
| `checkpoint.go` | New | 60s | Version-gated: PG ≤ 16 vs PG ≥ 17 |
| `io_stats.go` | New | 60s | PG ≥ 16 only — return nil on PG < 16 |

### Design Questions for M1_03 Planning

1. **Progress collectors: one file per operation or one combined file?** There are 6 progress views. Each follows the same pattern but has different columns. One combined `progress.go` with 6 internal methods would keep the file count down. Six separate files would follow the M1_01 one-file-per-collector pattern strictly.

2. **pg_stat_io granularity:** The view has many rows. Should we emit one metric per row (high cardinality) or aggregate by backend_type?

3. **Checkpoint stats accumulator:** `pg_stat_bgwriter`/`pg_stat_checkpointer` are cumulative counters. Should the collector emit raw counters (let storage/alerting compute rates) or compute deltas in the collector?

---

## Known Issues Affecting M1_03

1. **Docker Desktop unavailable** — integration tests CI-only.
2. **Registration is explicit in main.go** — do NOT use init()/RegisterCollector() pattern.
3. **Progress views return 0 rows when no operation is running** — collectors must handle empty results gracefully (return empty slice, not error).
4. **regclass casting** — `p.relid::regclass` can fail if the table was dropped mid-operation. Use `COALESCE(p.relid::regclass::text, 'unknown')` or catch the error.

---

## Workflow for M1_03

```
1. New Claude.ai chat (this handoff uploaded):
   - Discuss progress collector design (one vs many files)
   - Discuss checkpoint version gate design
   - Discuss pg_stat_io granularity
   - Produce requirements.md, design.md, team-prompt.md

2. Developer: copy to docs/iterations/M1_03_.../

3. Claude Code: paste team-prompt.md → agents create files

4. Developer: go mod tidy → go build → go vet → golangci-lint run → fix cycle

5. Developer: go test ./internal/collector/... (unit tests)

6. Claude.ai: create session-log.md

7. Developer: git commit + push
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
