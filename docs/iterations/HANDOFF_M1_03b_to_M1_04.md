# PGPulse — Iteration Handoff: M1_03b → M1_04

> **Purpose:** Upload this file when starting the next Claude.ai chat.
> Contains EVERYTHING needed to begin M1_04 without re-discovery.
> **Created:** 2026-02-26 (end of M1_03b)

---

## DO NOT RE-DISCUSS

These decisions are final. The new chat must not revisit them:

1. **Stack**: Go 1.24.0, pgx v5.8.0, chi v5, koanf, slog, testcontainers-go
2. **Architecture**: Single binary, version-adaptive SQL via Gate pattern, Collector interface with InstanceContext
3. **Granularity**: One file = one collector = one struct implementing Collector. (Exception: progress collectors grouped by operational similarity.)
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
15. **InstanceContext SSoT**: Orchestrator queries `pg_is_in_recovery()` once per cycle, passes `InstanceContext{IsRecovery: bool}` to all collectors.
16. **Collector registration**: **Explicit in main.go, NOT via init()/RegisterCollector() auto-registration.**
17. **Logical replication Q41**: Deferred — requires per-database connections (PerDatabaseCollector interface).
18. **Checkpoint version gate**: PG ≤ 16 → pg_stat_bgwriter (combined). PG ≥ 17 → pg_stat_checkpointer CROSS JOIN pg_stat_bgwriter (reduced). Uses -1 sentinel for unavailable columns.
19. **Stateful collector pattern**: CheckpointCollector introduced `computeMetrics()` pure function for testable rate computation. Protected by sync.Mutex. Detects stats reset via counter decrease.
20. **PG 17 column name**: `restartpoints_req` (NOT `restartpoints_requested`).
21. **pg_stat_io**: Stateless, COALESCE to -1 sentinel, skip metrics where value < 0. Labels: backend_type, object, context. PG < 16 returns nil, nil.
22. **Claude Code**: Updated to v2.1.59. Single Sonnet session works well for simple 1–2 file tasks. Save Agent Teams for complex multi-file iterations.

---

## What Exists After M1_03b

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
│   ├── replication_lag.go        ← Q37,Q38: byte + time lag per replica
│   ├── replication_slots.go      ← Q40: WAL retention per slot, version-gated
│   ├── replication_status.go     ← Q20,Q21: active replicas / WAL receiver
│   ├── progress_vacuum.go        ← Q42: VACUUM progress + completionPct() helper [M1_03]
│   ├── progress_maintenance.go   ← Q43,Q45: CLUSTER + ANALYZE progress [M1_03]
│   ├── progress_operations.go    ← Q44,Q46,Q47: CREATE INDEX + BASEBACKUP + COPY [M1_03]
│   ├── checkpoint.go             ← Checkpoint/BGWriter, stateful, version-gated PG≤16/≥17 [M1_03]
│   ├── io_stats.go               ← pg_stat_io (PG ≥ 16), stateless, per-row metrics [M1_03b]
│   ├── registry.go               ← RegisterCollector(), CollectAll() with partial-failure
│   ├── testutil_test.go
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
│   ├── progress_vacuum_test.go   [M1_03]
│   ├── progress_maintenance_test.go [M1_03]
│   ├── progress_operations_test.go  [M1_03]
│   ├── checkpoint_test.go        [M1_03]
│   └── io_stats_test.go          [M1_03b]
│
└── version/
    ├── version.go                 ← PGVersion, Detect(), AtLeast()
    └── gate.go                    ← Gate, SQLVariant, VersionRange, Select()
```

### Key Interfaces (unchanged)

```go
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

### Version Gate (from gate.go — DO NOT MODIFY)

```go
type Gate struct {
    Name     string
    Variants []SQLVariant
}
func (g Gate) Select(v PGVersion) (string, bool)
```

---

## Build & Test Status After M1_03b

| Check | Result |
|-------|--------|
| `go build ./...` | ✅ Pass |
| `go vet ./...` | ✅ Pass |
| `golangci-lint run` (v2.10.1) | ✅ 0 issues |
| Unit tests | ✅ All pass |
| Integration tests | ⏭️ Skipped locally |

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
| **analiz2.php Q48–Q52** | **pg_stat_statements** | **🔲 M1_04 — THIS ITERATION** |
| analiz2.php Q53–Q58 | Locks & wait events | 🔲 M1_05 |
| analiz_db.php Q1–Q18 | Per-DB analysis | 🔲 Later milestone |
| — (new) | Checkpoint/BGWriter | ✅ Done (M1_03) |
| — (new) | pg_stat_io | ✅ Done (M1_03b) |
| **Total** | **76 PGAM + new** | **24/76 PGAM ported, 2 new done** |

---

## Next Task: M1_04 — pg_stat_statements Collectors

### Goal

Port PGAM queries Q48–Q52 covering pg_stat_statements monitoring:
- Q48: pg_stat_statements settings (max, track, track_io_timing) + fill percentage
- Q49: Stats reset info from pg_stat_statements_info (PG ≥ 14)
- Q50: Query stats sorted by I/O time (top queries by blk_read_time + blk_write_time)
- Q51: Query stats sorted by CPU time (total_exec_time - blk_read_time - blk_write_time)
- Q52: Normalized query stats with per-query breakdown

### PGAM Queries Q48–Q52 (from PGAM_FEATURE_AUDIT.md)

**Q48 — pg_stat_statements settings**
```sql
SELECT name, setting
FROM pg_settings
WHERE name IN ('pg_stat_statements.max', 'pg_stat_statements.track', 'track_io_timing')
```
Plus fill percentage:
```sql
SELECT count(*) FROM pg_stat_statements
```
Combined with `pg_stat_statements.max` to compute fill%.

**Q49 — Stats reset age (PG ≥ 14)**
```sql
SELECT stats_reset, now() - stats_reset AS stats_age
FROM pg_stat_statements_info
```

**Q50 — Query stats by IO time**
CTE on pg_stat_statements:
- `sum(blk_read_time + blk_write_time)` per query
- `blk_read_time / blk_write_time` breakdown
- Top queries ranked by IO time
- Includes totals row + "other" bucket for remaining queries

**Q51 — Query stats by CPU time**
CTE on pg_stat_statements:
- PG ≤ 12: `sum(total_time - blk_read_time - blk_write_time)`
- PG ≥ 13: `sum(total_exec_time - blk_read_time - blk_write_time)`
- Top queries ranked by CPU time

**Q52 — Normalized total stats**
Complex CTE: normalizes parameters via regexp, groups by `md5(query_normalized)`,
produces per-query aggregates including database name, user name.

### Version Gates Required for M1_04

| Feature | PG 14 | PG 15 | PG 16 | PG 17 |
|---------|-------|-------|-------|-------|
| pg_stat_statements (basic) | ✅ | ✅ | ✅ | ✅ |
| pg_stat_statements_info | ✅ | ✅ | ✅ | ✅ |
| total_exec_time (not total_time) | ✅ | ✅ | ✅ | ✅ |
| total_plan_time | ✅ | ✅ | ✅ | ✅ |

Note: Since our minimum is PG 14, we always have `total_exec_time` and `pg_stat_statements_info`. The PG ≤ 12 `total_time` column path from PGAM can be dropped entirely.

### Complexity Assessment

**Q48–Q49: Low.** Simple settings queries + fill percentage. Q49 uses pg_stat_statements_info which is always available (PG ≥ 14).

**Q50–Q51: Medium.** Top-N query ranking by IO/CPU time. Need to decide:
- How many top queries to return (PGAM uses a 0.5% threshold cutoff)
- Whether to compute the "other" bucket (aggregation of remaining queries)
- PG version handling: only `total_exec_time` needed (PG 14+ minimum)

**Q52: Medium-High.** Query normalization, grouping, per-database/per-user breakdown. The PGAM version does regex normalization in PHP — we need to decide whether to do this in SQL or Go. pg_stat_statements already normalizes via queryid in PG 14+, which may make the regex approach unnecessary.

### Design Questions for M1_04 Planning

1. **Q48 + Q49: One collector or two?** Both are lightweight "pg_stat_statements health" queries. Could be one `StatementsConfigCollector` or split into `StatementsSettingsCollector` + `StatementsResetCollector`.

2. **Q50 + Q51: Separate or combined?** Both are top-N queries from pg_stat_statements, differing only in sort column. Could be one collector with two metric groups, or two collectors.

3. **Q52: Scope question.** The PGAM normalized total report is a text-based report, not numeric metrics. Should PGPulse:
   - (a) Port it as a structured metric collector (queryid-level metrics)
   - (b) Port it as a text report endpoint (later, API layer)
   - (c) Skip it — queryid-level breakdowns in Q50/Q51 already cover the use case

4. **pg_stat_statements availability:** The extension might not be installed. All collectors must check for its presence (extensions.go already tracks this via Q18). How should statements collectors handle missing pgss? Return nil, nil? Return an error?

5. **Query text in metrics?** pg_stat_statements contains query text. Should the collector include truncated query text as a label? High cardinality risk. Alternative: use queryid as label, query text as a separate lookup.

### Suggested Collector Files for M1_04

| File | Queries | Interval | Notes |
|------|---------|----------|-------|
| `statements_config.go` | Q48, Q49 | 60s | Settings + fill% + reset age |
| `statements_top.go` | Q50, Q51 | 60s | Top queries by IO and CPU time |
| `statements_detail.go` | Q52 (if porting) | 300s | Per-queryid breakdown (optional) |

---

## Known Issues Affecting M1_04

1. **Docker Desktop unavailable** — integration tests CI-only.
2. **pg_stat_statements might not be installed** — collectors must handle gracefully.
3. **Query text cardinality** — avoid using raw query text as metric labels.
4. **PGAM's total_time vs total_exec_time** — irrelevant for us (PG 14+ only uses total_exec_time). Drop the PG ≤ 12 code path.
5. **PGAM's regex normalization (Q52)** — likely unnecessary since PG 14+ has native queryid-based normalization.

---

## Workflow for M1_04

```
1. New Claude.ai chat (this handoff uploaded):
   - Discuss collector structure (how many files, what each covers)
   - Discuss Q52 scope (port as metrics vs skip vs defer)
   - Discuss pgss availability handling
   - Produce requirements.md, design.md, team-prompt.md

2. Developer: copy to docs/iterations/M1_04_.../

3. Claude Code: paste team-prompt → agents create files
   (Consider Agent Teams for this one — 3+ files with version gates)

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
| Claude Code | 2.1.59 | Updated from 2.1.53. Bash still broken on Windows. |
| testcontainers-go | 0.40.0 | Requires Docker Desktop (unavailable) |
| Docker Desktop | Not installed | Integration tests → CI only |
| Git | 2.52.0 | |
| Node.js | 22.14.0 | For Claude Code |
