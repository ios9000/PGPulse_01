# PGPulse — Iteration Handoff: M1_04 → M1_05

> **Purpose:** Upload this file when starting the next Claude.ai chat.
> Contains EVERYTHING needed to begin M1_05 without re-discovery.
> **Created:** 2026-02-26 (end of M1_04)

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
23. **pgss availability check**: `pgssAvailable()` helper in base.go. Returns false → collector returns nil, nil. Loose coupling (not in InstanceContext).
24. **Q50+Q51 combined**: Single query with ROW_NUMBER + CROSS JOIN totals. "Other" bucket computed in Go by subtraction. Fixed LIMIT 20 (not PGAM's 0.5% threshold).
25. **Q52 deferred**: Normalized text report → M2/API layer. PG 14+ queryid handles normalization natively.

---

## What Exists After M1_04

### Repository Structure (collector-related)

```
internal/
├── collector/
│   ├── collector.go              ← interfaces: MetricPoint, InstanceContext, Collector, MetricStore, AlertEvaluator
│   ├── base.go                   ← shared Base struct, point(), queryContext(), pgssAvailable() [M1_04]
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
│   ├── progress_vacuum.go        ← Q42: VACUUM progress + completionPct() helper
│   ├── progress_maintenance.go   ← Q43,Q45: CLUSTER + ANALYZE progress
│   ├── progress_operations.go    ← Q44,Q46,Q47: CREATE INDEX + BASEBACKUP + COPY
│   ├── checkpoint.go             ← Checkpoint/BGWriter, stateful, version-gated PG≤16/≥17
│   ├── io_stats.go               ← pg_stat_io (PG ≥ 16), stateless, per-row metrics
│   ├── statements_config.go      ← Q48,Q49: pgss settings, fill%, reset age [M1_04]
│   ├── statements_top.go         ← Q50,Q51: top-N by IO+CPU time, "other" bucket [M1_04]
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
│   ├── progress_vacuum_test.go
│   ├── progress_maintenance_test.go
│   ├── progress_operations_test.go
│   ├── checkpoint_test.go
│   ├── io_stats_test.go
│   ├── statements_config_test.go  [M1_04]
│   └── statements_top_test.go     [M1_04]
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

// Added in M1_04:
func pgssAvailable(ctx context.Context, conn *pgx.Conn) (bool, error)
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

## Build & Test Status After M1_04

| Check | Result |
|-------|--------|
| `go build ./...` | ✅ Pass |
| `go vet ./...` | ✅ Pass |
| `golangci-lint run` (v2.10.1) | ✅ 0 issues |
| Statements unit tests (11 run, 2 skip) | ✅ 9 pass, 2 skip (integration) |
| Full collector suite | ✅ All pass |

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
| analiz2.php Q48–Q49 | pgss config/reset | ✅ Done (M1_04) |
| analiz2.php Q50–Q51 | pgss top queries | ✅ Done (M1_04) |
| analiz2.php Q52 | Normalized report | ⏭️ Deferred to M2/API (covered by Q50+Q51 at metric level) |
| **analiz2.php Q53–Q57** | **Locks & wait events** | **🔲 M1_05 — THIS ITERATION** |
| analiz2.php Q58 | Extensions list | ✅ Already covered by Q18 (extensions.go) |
| analiz_db.php Q1–Q18 | Per-DB analysis | 🔲 Later milestone |
| — (new) | Checkpoint/BGWriter | ✅ Done (M1_03) |
| — (new) | pg_stat_io | ✅ Done (M1_03b) |
| **Total** | **76 PGAM + new** | **28/76 PGAM ported, 2 new done** |

---

## Next Task: M1_05 — Locks & Wait Events Collectors

### Goal

Port PGAM queries Q53–Q57 covering locks and activity monitoring. This is the **final M1 iteration** for analiz2.php instance-level queries.

### PGAM Queries Q53–Q57 (from PGAM_FEATURE_AUDIT.md)

**Q53 — Wait event summary (verbose)**
```sql
SELECT wait_event_type, wait_event, count(*)
FROM pg_stat_activity
GROUP BY 1, 2
ORDER BY 3 DESC
```
In PGAM, this is joined in PHP with wait-event descriptions from an internal VTB database (`$con_spec`). PGPulse does NOT replicate this — descriptions can be added at the API/UI layer if needed.

**Q54 — Wait event summary (minimal)**
Same query as Q53, without the description join. In PGPulse, Q53 and Q54 collapse into a single collector since we're not doing the PHP-side join.

**Q55 — Lock tree (blocking tree)**
Recursive CTE on `pg_locks JOIN pg_stat_activity`:
- Builds blocked → blocker tree with depth level
- Counts blocked processes per root blocker
- Includes: pid, usename, datname, query, wait_event_type, wait_event, state, query_start, xact_start, blocked_by_pid, lock_mode, relation

This is the most complex query in the M1 scope — a recursive CTE with multiple joins.

**Q56 — Long active transactions**
```sql
SELECT pid, usename, datname, client_addr, 
       now() - xact_start AS xact_duration,
       now() - query_start AS query_duration,
       state, wait_event_type, wait_event,
       left(query, 1000) AS query
FROM pg_stat_activity
WHERE xact_start < now() - interval '5 seconds'
  AND state = 'active'
  AND wait_event IS NULL
  AND pid != pg_backend_pid()
ORDER BY xact_start
```

**Q57 — Long waiting transactions**
```sql
SELECT pid, usename, datname, client_addr,
       now() - xact_start AS xact_duration,
       now() - query_start AS query_duration,
       state, wait_event_type, wait_event,
       left(query, 1000) AS query
FROM pg_stat_activity
WHERE xact_start < now() - interval '5 seconds'
  AND state = 'active'
  AND wait_event IS NOT NULL
  AND pid != pg_backend_pid()
ORDER BY xact_start
```

### Version Gates Required for M1_05

None — all views/columns used (pg_stat_activity, pg_locks) are stable across PG 14–17. No version-specific SQL needed.

### Complexity Assessment

**Q53/Q54 (wait events): Low.** Single GROUP BY query. Q53 and Q54 merge into one collector.

**Q55 (lock tree): High.** Recursive CTE is the most complex SQL in M1. Key challenges:
- Recursive query structure: base case (blockers) + recursive case (blocked by blockers)
- Multiple joins: pg_locks ↔ pg_stat_activity
- Tree depth tracking and blocked-process counting
- The output is hierarchical — how to represent as flat MetricPoint records?

**Q56/Q57 (long transactions): Low.** Simple filtered queries on pg_stat_activity. Could be one collector with two metric groups (active vs waiting) or two separate collectors.

### Design Questions for M1_05 Planning

1. **Q53+Q54 → one collector?** Since PGPulse doesn't replicate the VTB description join, these are identical. One `WaitEventsCollector` emitting per-event-type counts.

2. **Q55 lock tree — metric representation?** The PGAM lock tree is a visual tree (HTML table with indentation). As metrics, options:
   - (a) Flat metrics: count of blocking PIDs, count of blocked PIDs, max tree depth, total blocked processes
   - (b) Per-blocker metrics: one metric set per root blocker with blocked_count label
   - (c) Full tree as structured data — deferred to API layer (not MetricPoint-shaped)
   
   The lock tree is fundamentally a relational/structural query, not a numeric metric. The most useful metric-level data might be summary stats (how many blockers, how many blocked, max depth), with the full tree served by the API.

3. **Q56+Q57 → one or two collectors?** Structurally identical except for `wait_event IS NULL` vs `IS NOT NULL`. Could be one `LongTransactionsCollector` with a label distinguishing active vs waiting.

4. **Long transaction threshold:** PGAM hardcodes 5 seconds. Should this be configurable at the collector level?

5. **Query text in labels?** Q55, Q56, Q57 include query text. Same rule as statements: use pid as the identifier, query text is a lookup dimension for the API layer. But should we include truncated query text (e.g., first 100 chars) as a label for debugging context?

### Suggested Collector Files for M1_05

| File | Queries | Interval | Complexity |
|------|---------|----------|------------|
| `wait_events.go` | Q53/Q54 | 10s | Low |
| `lock_tree.go` | Q55 | 10s | High (recursive CTE) |
| `long_transactions.go` | Q56/Q57 | 10s | Low |

Note: These are high-frequency collectors (10s interval per the strategy doc: "High frequency (10s): connections, locks, wait events").

---

## Known Issues Affecting M1_05

1. **Docker Desktop unavailable** — integration tests CI-only.
2. **Lock tree is structural, not purely numeric** — need to decide what goes into MetricPoint vs what's deferred to API.
3. **Query text cardinality** — same principle as statements: avoid raw query text as metric labels.
4. **VTB wait-event descriptions** — not ported. API layer can add descriptions later if desired.
5. **PGAM Q55 is the most complex single query in the project** — the recursive CTE needs careful testing.

---

## Workflow for M1_05

```
1. New Claude.ai chat (this handoff uploaded):
   - Discuss lock tree metric representation (summary vs per-blocker vs defer full tree)
   - Discuss Q56/Q57 merge and threshold configurability
   - Discuss query text in labels (pid-only vs truncated text)
   - Produce requirements.md, design.md, team-prompt.md

2. Developer: copy to docs/iterations/M1_05_.../

3. Claude Code: paste team-prompt → create files
   (Consider Agent Teams for this one — recursive CTE + 3 collectors + 3 test files)

4. Developer: go build → go vet → golangci-lint → go test

5. Claude.ai: create session-log.md

6. Developer: git commit + push
```

---

## M1 Milestone Completion After M1_05

After M1_05, all analiz2.php instance-level queries will be ported (except OS/cluster queries deferred to M6, logical replication deferred to PerDatabaseCollector, and Q52 deferred to M2/API). This completes the M1 milestone:

| Iteration | Scope | Status |
|-----------|-------|--------|
| M1_01 | Instance metrics Q1-Q19 | ✅ Done |
| M1_02b | Replication Q20-Q21, Q37-Q38, Q40 | ✅ Done |
| M1_03 | Progress Q42-Q47, Checkpoint/BGWriter | ✅ Done |
| M1_03b | pg_stat_io (new, PG 16+) | ✅ Done |
| M1_04 | pg_stat_statements Q48-Q51 | ✅ Done |
| **M1_05** | **Locks & wait events Q53-Q57** | **🔲 Next** |

---

## Environment Reference

| Tool | Version | Notes |
|------|---------|-------|
| Go | 1.24.0 windows/amd64 | |
| golangci-lint | 2.10.1 | v2 config: requires `version: "2"`, no `gosimple` |
| Claude Code | 2.1.59 | Bash still broken on Windows |
| testcontainers-go | 0.40.0 | Requires Docker Desktop (unavailable) |
| Docker Desktop | Not installed | Integration tests → CI only |
| Git | 2.52.0 | |
| Node.js | 22.14.0 | For Claude Code |
