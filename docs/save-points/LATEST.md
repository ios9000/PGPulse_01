# PGPulse — Save Point

**Save Point:** M1 (in progress) — Core Collector
**Date:** 2026-02-26
**Commit:** c7db1e8 (after M1_05 + CHANGELOG update)
**Developer:** Evlampiy (ios9000)
**AI Tool:** Claude.ai (Opus 4.6) + Claude Code (Sonnet 4.6, single session)

---

## 1. PROJECT IDENTITY

**Name:** PGPulse — PostgreSQL Health & Activity Monitor
**Repo:** https://github.com/ios9000/PGPulse_01
**Legacy repo:** https://github.com/ios9000/pgam-legacy
**Go module:** github.com/ios9000/PGPulse_01
**License:** TBD

### What PGPulse Does
PGPulse is a real-time PostgreSQL monitoring tool that collects metrics from PG 14–18 instances (connections, cache hit ratio, replication lag, locks, wait events, pg_stat_statements, vacuum progress, bloat), stores them in TimescaleDB, provides alerting via Telegram/Slack/Email/Webhook, and will include ML-based anomaly detection. It's designed as a single Go binary with an embedded web UI, targeting PostgreSQL DBAs who need a lightweight, PG-specialized alternative to heavyweight platforms like PMM or SolarWinds.

### Origin Story
Rewrite of PGAM — a legacy PHP PostgreSQL Activity Monitor used internally at VTB Bank. PGAM had 76 SQL queries across 2 PHP files (analiz2.php + analiz_db.php), zero authentication, SQL injection vulnerabilities via raw GET params, and relied on COPY TO PROGRAM for OS metrics (requiring superuser). PGPulse is a clean-room rewrite in Go that preserves the SQL monitoring knowledge while fixing every architectural and security flaw.

---

## 2. ARCHITECTURE SNAPSHOT

### Tech Stack
| Component | Choice | Version | Why |
|---|---|---|---|
| Language | Go | 1.24.0 | Upgraded from 1.23.6; pgx v5.8.0 requires ≥ 1.24 |
| PG Driver | jackc/pgx v5 | 5.8.0 | Best Go PG driver, parameterized queries, named args |
| HTTP Router | go-chi/chi v5 | — | Lightweight, middleware-friendly |
| Storage | PostgreSQL + TimescaleDB | — | PG-native time-series hypertables |
| Frontend | Svelte + Tailwind | — | Embedded via go:embed (M5) |
| Config | koanf | — | YAML + env vars |
| Logging | log/slog | stdlib | Structured logging |
| Testing | testcontainers-go | 0.40.0 | Real PG instances in CI tests |
| ML (Phase 1) | gonum | — | Pure Go statistics (M8) |
| CI | GitHub Actions | — | Lint + test + build |
| Linter | golangci-lint | v2.10.1 | Upgraded from v1 — Go 1.24 required v2 config format |

### Architecture Diagram
```
┌─────────────────────────────────────────────────────┐
│              PGPulse Server (Go binary)              │
│                                                      │
│  ┌────────────────┐  ┌──────────┐  ┌──────────┐    │
│  │  Collectors     │→ │ Storage  │← │ REST API │    │
│  │  (pgx v5)      │  │ (TSDB)   │  │ (chi+JWT)│    │
│  │                 │  └──────────┘  └────┬─────┘    │
│  │  server_info    │                     │           │
│  │  connections    │               ┌─────▼─────┐    │
│  │  cache          │               │   Auth     │    │
│  │  transactions   │               │  (RBAC)    │    │
│  │  database_sizes │               └───────────┘    │
│  │  settings       │                                 │
│  │  extensions     │  ┌───────────┐  ┌──────────┐   │
│  │  replication_*  │  │  Alert    │  │  Web UI  │   │
│  │  progress_*     │  │  Engine   │  │ (embed)  │   │
│  │  checkpoint     │  │           │  │          │   │
│  │  io_stats       │  └───────────┘  └──────────┘   │
│  │  statements_*   │                                 │
│  │  wait_events    │                                 │
│  │  lock_tree      │                                 │
│  │  long_txns      │  ← M1_05 (complete)             │
│  └───────┬─────────┘                                │
│          │                                           │
│  ┌───────▼─────────┐                                │
│  │  Version Gate   │  ← PG 14/15/16/17/18 SQL      │
│  └───────┬─────────┘                                │
│          │                                           │
│  ┌───────▼─────────┐                                │
│  │  InstanceContext │  ← IsRecovery (per cycle SSoT) │
│  └─────────────────┘                                │
└─────────────────────────────────────────────────────┘
         │
    ┌────▼─────┐
    │ PGPulse  │  (optional, separate binary, M6)
    │  Agent   │  OS metrics via procfs
    └──────────┘
```

### Key Design Decisions

| # | Decision | Rationale | Date |
|---|----------|-----------|------|
| D1 | Single binary deployment | Simplicity, go:embed frontend | 2026-02-25 |
| D2 | pgx v5 (not database/sql) | Named args, COPY protocol, pgx-specific features | 2026-02-25 |
| D3 | Version-adaptive SQL via Gate pattern | Support PG 14-18 without code branches | 2026-02-25 |
| D4 | No COPY TO PROGRAM ever | PGAM's worst security flaw — eliminated | 2026-02-25 |
| D5 | pg_monitor role only | Least privilege, never superuser | 2026-02-25 |
| D6 | One Collector per module file | Testable, enable/disable, independent intervals | 2026-02-25 |
| D7 | Hybrid agent workflow | Claude Code bash works in direct session; broken in Agent Teams subprocess | 2026-02-25 |
| D8 | Agent Teams (4 agents max) | Right-sized for 1-dev project | 2026-02-25 |
| D9 | Three-tier persistence | Save Points + Handoffs + Session-logs | 2026-02-25 |
| D10 | Base struct with point() helper | All collectors embed Base; auto-prefixes "pgpulse.", fills InstanceID + Timestamp | 2026-02-25 |
| D11 | Registry pattern for collectors | CollectAll() with partial-failure semantics; registered explicitly in main.go | 2026-02-25 |
| D12 | 5s statement_timeout for live collectors | Via context.WithTimeout in queryContext() helper, not SQL SET | 2026-02-25 |
| D13 | InstanceContext SSoT for per-cycle state | Orchestrator queries pg_is_in_recovery() once per cycle, passes to all collectors. | 2026-02-25 |
| D14 | Version in Base, IsRecovery in InstanceContext | Version = structural (immutable). Recovery state = dynamic (changes on failover). | 2026-02-25 |
| D15 | Defer logical replication Q41 | Requires per-DB connections, breaks single-conn Collector interface. | 2026-02-25 |
| D16 | golangci-lint v2 config format | v1 doesn't support Go 1.24. Config requires `version: "2"` field. `gosimple` removed. | 2026-02-25 |
| D17 | Docker Desktop not available | BIOS virtualization disabled. Integration tests CI-only. | 2026-02-25 |
| D18 | Stateful checkpoint collector with snapshot + rate pattern | Checkpoint/bgwriter counters are cumulative. Need deltas for per-second rates. | 2026-02-26 |
| D19 | -1 sentinel for version-unavailable columns | PG 14-16 lacks restartpoints; PG 17 lacks buffers_backend. Use -1 (not 0, not NULL). | 2026-02-26 |
| D20 | completionPct() shared helper for all progress collectors | Six progress collectors all need safe division. One helper in progress_vacuum.go. | 2026-02-26 |
| D21 | Multiple collectors per file for related operations | Group by similarity: progress_maintenance.go, progress_operations.go. | 2026-02-26 |
| D22 | pg_stat_io deferred to M1_03b | PG 16+ only, high cardinality, needs granularity design. | 2026-02-26 |
| D23 | pgssAvailable() shared helper in base.go | Both statements collectors need the same EXISTS check. | 2026-02-26 |
| D24 | buildMetrics()/buildTopMetrics() extracted as pure methods | Enables unit testing without DB (same pattern as computeMetrics() in checkpoint). | 2026-02-26 |
| D25 | Top-N unified query (Q50+Q51 combined) | Single CTE ranks by total_exec_time, derives IO and CPU from same scan. | 2026-02-26 |
| D26 | "Other" bucket uses unclamped sums | other = totals - actual_sum preserves accuracy even when top-N has negative cpu. | 2026-02-26 |
| D27 | *float64 for nullable stats_reset scan | pgx v5 sets *float64 to nil for NULL. Metric simply not emitted when nil. | 2026-02-26 |
| D28 | Single Claude Code session for M1_04+ | Scope is small enough; Agent Teams overhead not justified. | 2026-02-26 |
| D29 | pg_blocking_pids() over recursive pg_locks CTE | Simpler, PG-native since 9.6, no SQL recursion complexity. | 2026-02-26 |
| D30 | Lock graph traversal in Go (BFS) not SQL | Pure function = directly unit-testable without DB; cycle protection via visited set. | 2026-02-26 |
| D31 | Deadlock: BlockerCount=0, BlockedCount>0 | All deadlock participants are in both sets → no roots. Signals deadlock correctly. | 2026-02-26 |
| D32 | LongTransactions always emits 4 points | Zero-fill missing types: consistent schema regardless of activity. | 2026-02-26 |
| D33 | Q56+Q57 merged into single parameterized query | CASE WHEN wait_event IS NULL THEN 'active' ELSE 'waiting' eliminates redundant round-trip. | 2026-02-26 |
| D34 | Q58 (lock details) deferred | Q53–Q57 provide operational lock monitoring. Per-lock-detail (Q58) is analytic; defer to analiz_db.php milestone. | 2026-02-26 |

---

## 3. CODEBASE STATE

### File Tree (after M1_05)
```
.claude/CLAUDE.md
.claude/rules/...
.github/workflows/ci.yml
.gitignore / .golangci.yml / Makefile / README.md
cmd/pgpulse-agent/main.go
cmd/pgpulse-server/main.go
configs/pgpulse.example.yml
deploy/...
docs/iterations/...
docs/roadmap.md
docs/save-points/LATEST.md
docs/save-points/SAVEPOINT_M0_20260225.md
docs/save-points/SAVEPOINT_M1_20260225.md
docs/save-points/SAVEPOINT_M1_20260226.md
docs/save-points/SAVEPOINT_M1_20260226b.md
docs/save-points/SAVEPOINT_M1_20260226c.md   ← THIS FILE
go.mod / go.sum
internal/collector/
  base.go                      pgssAvailable(), point(), queryContext(), Base struct
  cache.go / cache_test.go
  checkpoint.go / _test.go     Stateful, version-gated PG≤16/≥17
  collector.go                 Interfaces: MetricPoint, InstanceContext, Collector, MetricStore
  connections.go / _test.go
  database_sizes.go / _test.go
  extensions.go / _test.go
  io_stats.go / _test.go       pg_stat_io, PG 16+
  lock_tree.go / _test.go      NEW M1_05 — blocking tree, BFS graph
  long_transactions.go / _test.go  NEW M1_05 — Q56/Q57 merged
  progress_maintenance.go / _test.go
  progress_operations.go / _test.go
  progress_vacuum.go / _test.go
  registry.go / _test.go
  replication_lag.go / _test.go
  replication_slots.go / _test.go
  replication_status.go / _test.go
  server_info.go / _test.go
  settings.go / _test.go
  statements_config.go / _test.go   NEW M1_04 — Q48/Q49
  statements_top.go / _test.go      NEW M1_04 — Q50/Q51
  testutil_test.go             Integration test helpers (//go:build integration)
  transactions.go / _test.go
  wait_events.go / _test.go    NEW M1_05 — Q53/Q54
internal/version/
  gate.go                      Gate, SQLVariant, VersionRange, Select()
  version.go                   PGVersion, Detect(), AtLeast()
internal/alert|api|auth|config|ml|rca|storage/  (.gitkeep — future milestones)
migrations/.gitkeep / web/.gitkeep
```

### Key Interfaces (unchanged since M1_02a)

```go
// internal/collector/collector.go

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

```go
// internal/collector/base.go

func newBase(instanceID string, v version.PGVersion, interval time.Duration) Base
func (b *Base) point(metric string, value float64, labels map[string]string) MetricPoint
func (b *Base) Interval() time.Duration
func queryContext(ctx context.Context) (context.Context, context.CancelFunc) // 5s timeout
func pgssAvailable(ctx context.Context, conn *pgx.Conn) (bool, error)
```

```go
// internal/version/gate.go

type VersionRange struct { MinMajor, MinMinor, MaxMajor, MaxMinor int }
type SQLVariant struct { Range VersionRange; SQL string }
type Gate struct { Name string; Variants []SQLVariant }
func (g Gate) Select(v PGVersion) (string, bool)
```

---

## 4. LEGACY KNOWLEDGE (PGAM)

### Query Porting Status

| Source | Queries | Target | Status |
|--------|---------|--------|--------|
| analiz2.php Q1 | PG version string | version.Detect() | ✅ Done (M0) |
| analiz2.php Q2–Q3, Q9–Q10 | Server info | server_info.go | ✅ Done (M1_01) |
| analiz2.php Q4–Q8 | OS metrics | — | ⏭️ Deferred to M6 |
| analiz2.php Q11–Q13 | Connections | connections.go | ✅ Done (M1_01) |
| analiz2.php Q14 | Cache hit ratio | cache.go | ✅ Done (M1_01) |
| analiz2.php Q15 | Transactions | transactions.go | ✅ Done (M1_01) |
| analiz2.php Q16 | Database sizes | database_sizes.go | ✅ Done (M1_01) |
| analiz2.php Q17 | Settings | settings.go | ✅ Done (M1_01) |
| analiz2.php Q18–Q19 | Extensions/pgss | extensions.go | ✅ Done (M1_01) |
| analiz2.php Q20–Q21 | Replication status | replication_status.go | ✅ Done (M1_02b) |
| analiz2.php Q22–Q35 | OS/cluster/overview | — | 🔲 M6/later |
| analiz2.php Q36, Q39 | Replication PG < 10 | — | ⏭️ Skipped (below min) |
| analiz2.php Q37–Q38 | Replication lag | replication_lag.go | ✅ Done (M1_02b) |
| analiz2.php Q40 | Replication slots | replication_slots.go | ✅ Done (M1_02b) |
| analiz2.php Q41 | Logical replication | — | ⏭️ Deferred (PerDatabaseCollector) |
| analiz2.php Q42–Q47 | Progress monitoring | progress_*.go | ✅ Done (M1_03) |
| analiz2.php Q48–Q49 | pgss settings/fill%/reset | statements_config.go | ✅ Done (M1_04) |
| analiz2.php Q50–Q51 | Top-N by IO/CPU time | statements_top.go | ✅ Done (M1_04) |
| analiz2.php Q52 | Normalized query stats | — | ⏭️ Deferred (queryid breakdown in Q50/Q51 covers it) |
| analiz2.php Q53–Q54 | Wait event summary | wait_events.go | ✅ Done (M1_05) |
| analiz2.php Q55 | Lock blocking tree | lock_tree.go | ✅ Done (M1_05) |
| analiz2.php Q56–Q57 | Long transactions | long_transactions.go | ✅ Done (M1_05) |
| analiz2.php Q58 | Lock details (per-lock) | — | ⏭️ Deferred to analiz_db.php milestone |
| analiz_db.php Q1–Q18 | Per-DB analysis | — | 🔲 Later milestone |
| — (new) | Checkpoint/bgwriter | checkpoint.go | ✅ Done (M1_03) |
| — (new) | pg_stat_io | io_stats.go | ✅ Done (M1_03b) |
| **Total: 76** | | | **33 done, 11 deferred/skipped, 32 remaining** |

### PGAM Bugs Fixed During Port

| # | Query | Bug | Fix |
|---|-------|-----|-----|
| 1 | Q11 | Counts own monitoring connection | `WHERE pid != pg_backend_pid()` |
| 2 | Q14 | Division by zero on empty cache | `NULLIF(blks_hit + blks_read, 0)` guard |
| 3 | Q4-Q8 | OS metrics via COPY TO PROGRAM (superuser) | Eliminated — Go agent via procfs (M6) |
| 4 | Q10 | pg_is_in_backup() removed in PG 15 | Version-gated: skip for PG ≥ 15 |
| 5 | Q50-Q51 | Two separate round-trips for IO/CPU | Unified into single CTE |
| 6 | Q55 | Recursive pg_locks CTE — complex, fragile | Replaced with pg_blocking_pids() + Go BFS |
| 7 | Q56-Q57 | Two queries differing only in wait_event IS NULL | Merged with CASE WHEN into one query |

### Version Gates Implemented

| # | Feature | Gate | Variants |
|---|---------|------|----------|
| 1 | Q10 | is_in_backup | PG ≤ 14: `pg_is_in_backup()` / PG ≥ 15: skip |
| 2 | Q19 | pgss_info | PG ≤ 13: skip / PG ≥ 14: `pg_stat_statements_info` |
| 3 | Q40 | replication_slots | PG 14 / PG 15 (+two_phase) / PG 16+ (+conflicting) |
| 4 | new | checkpoint_stats | PG 14–16: `pg_stat_bgwriter` / PG 17+: `pg_stat_checkpointer` CROSS JOIN |
| 5 | new | io_stats | PG < 16: nil,nil / PG ≥ 16: `pg_stat_io` |

---

## 5. MILESTONE STATUS

### Roadmap

| Milestone | Name | Status | Completion Date |
|---|---|---|---|
| M0 | Project Setup | ✅ Done | 2026-02-25 |
| M1 | Core Collector | 🔶 In progress (M1_01–M1_05 done) | — |
| M2 | Storage & API | 🔲 Not started | — |
| M3 | Auth & Security | 🔲 Not started | — |
| M4 | Alerting | 🔲 Not started | — |
| M5 | Web UI (MVP) | 🔲 Not started | — |
| M6 | Agent Mode | 🔲 Not started | — |
| M7–M10 | P1 Features / ML / Reports / Polish | 🔲 Not started | — |

### M1 Sub-Iteration Status

| Sub-Iteration | Scope | Status |
|---|---|---|
| M1_01 | Instance metrics (8 collectors) | ✅ Done |
| M1_02a | InstanceContext interface refactor | ✅ Done (c50dbe1) |
| M1_02b | Replication collectors (3 files) | ✅ Done |
| M1_03 | Progress monitoring + checkpoint/bgwriter | ✅ Done (f96ce2f) |
| M1_03b | pg_stat_io | ✅ Done (7963eff) |
| M1_04 | pg_stat_statements config + top-N | ✅ Done (d2528ee) |
| M1_05 | Locks & wait events | ✅ Done (c7db1e8) |

### What Was Just Completed (M1_05)

**M1_05 — Locks & wait events (Q53–Q57). Commit ea7e444. 6 files, 821 lines.**
**CHANGELOG updated. Commit c7db1e8.**

**`WaitEventsCollector` (wait_events.go) — Q53/Q54:**
- 10s interval; filters to `client backend` (reduces noise vs PGAM which included all backends)
- `COALESCE(wait_event_type, 'CPU')` / `COALESCE(wait_event, 'Running')` maps CPU-active backends to a visible label
- `buildMetrics(rows []waitEventRow) []MetricPoint` pure method for unit testing
- Emits `wait_events.count` per row (labels: wait_event_type, wait_event) + always emits `wait_events.total_backends`

**`LockTreeCollector` (lock_tree.go) — Q55:**
- 10s interval; uses `pg_blocking_pids()` instead of PGAM's recursive pg_locks CTE
- `lockEdge{BlockedPID, BlockerPID}` + `lockStats{BlockerCount, BlockedCount, MaxChainDepth}`
- `computeLockStats(edges []lockEdge) lockStats` — pure function, full unit-test coverage
- `bfsMaxDepth(startPID, blocks)` — BFS with visited set for cycle protection
- Deadlock detection: all deadlock participants appear in both sets → BlockerCount=0, BlockedCount>0
- `statsToPoints(stats lockStats) []MetricPoint` — always emits 3 points (zeros when no blocking)
- Emits: `locks.blocker_count`, `locks.blocked_count`, `locks.max_chain_depth`

**`LongTransactionsCollector` (long_transactions.go) — Q56/Q57:**
- 10s interval; threshold `"5 seconds"` as parameterized `$1::interval`
- Merges PGAM's two queries (active vs waiting) into one CASE WHEN query
- `buildMetrics(rows []longTxnRow) []MetricPoint` pure method; zero-fills missing types
- Always emits exactly 4 points: count + oldest_seconds for each of "active" and "waiting"

**Tests (23 new, all pass):**
- WaitEvents: 5 tests (name, interval, 3-row collect, empty, CPU/Running label)
- LockTree: 11 tests (2 collector + 7 pure computeLockStats graph cases + 2 edge cases)
  - Graph cases: NoEdges, SingleBlocker, Chain, Wide, MultiRoot, Diamond, Cycle (deadlock)
- LongTransactions: 6 tests (name, interval, Both, ActiveOnly, None, WaitingOnly)

**All checks green:** build, vet, golangci-lint (0 issues), full suite.

### What's Next

**M1 is functionally complete for analiz2.php.** Remaining analiz2.php items are either deferred (Q4–Q8 OS metrics → M6, Q41 logical replication → PerDatabaseCollector, Q52/Q58 → later) or skipped (Q36/Q39 below PG 14 min).

Options for next iteration:
1. **M1_06** — analiz_db.php Q1–Q18 (per-database analysis; needs PerDatabaseCollector interface design)
2. **Move to M2** — Storage & API layer (TimescaleDB schema, MetricStore implementation, REST endpoints)
3. **M1_06** — Wire up collectors in cmd/pgpulse-server/main.go (currently collectors exist but aren't connected to anything)

---

## 6. DEVELOPMENT ENVIRONMENT

### Developer Workstation
| Component | Value |
|---|---|
| OS | Windows 10 |
| Shell | Git Bash (MSYS2) |
| Go | 1.24.0 windows/amd64 |
| Claude Code | 2.1.59 |
| Git | 2.52.0 |
| golangci-lint | v2.10.1 |
| Docker Desktop | Not installed (BIOS virtualization disabled) |

### Development Method
- **Single Claude Code session** (Sonnet 4.6) for all M1_04+ iterations — no Agent Teams overhead
- **Hybrid workflow:** Claude Code creates files and runs bash directly; Agent Teams subprocess mode has bash bug
- **One chat per iteration** in Claude.ai for planning; Claude Code for implementation

### Known Environment Issues

| Issue | Status | Workaround |
|---|---|---|
| Claude Code bash EINVAL in Agent Teams subprocess mode | Unresolved | Use direct single-session Claude Code |
| LF/CRLF warnings on git add | Cosmetic | Add `.gitattributes` with `* text=auto eol=lf` |
| Docker Desktop unavailable | BIOS virtualization disabled | Integration tests CI-only |

---

## 7. COLLECTOR IMPLEMENTATION PATTERNS (COMPLETE)

### Pattern Summary

```go
// 1. Basic stateless collector (all M1_01–M1_05 collectors except checkpoint)
type XxxCollector struct { Base }
func NewXxxCollector(instanceID string, v version.PGVersion) *XxxCollector
func (c *XxxCollector) Name() string { return "xxx" }
func (c *XxxCollector) Collect(ctx, conn, ic) ([]MetricPoint, error) {
    qCtx, cancel := queryContext(ctx); defer cancel()
    rows, err := conn.Query(qCtx, sql, args...)
    // scan → buildMetrics(scanned) or statsToPoints(computeFn(scanned))
}
func (c *XxxCollector) buildMetrics(rows []xxxRow) []MetricPoint { /* pure, testable */ }

// 2. Version-gated SQL (checkpoint, replication_slots, server_info)
var myGate = version.Gate{Variants: []version.SQLVariant{
    {Range: version.VersionRange{MinMajor:14, ..., MaxMajor:16, ...}, SQL: `...`},
    {Range: version.VersionRange{MinMajor:17, ..., MaxMajor:99, ...}, SQL: `...`},
}}

// 3. Stateful collector (checkpoint — rate computation)
type MyStateful struct {
    Base; sqlGate version.Gate; mu sync.Mutex; prev *mySnapshot; prevTime time.Time
}
func (c *MyStateful) computeMetrics(curr, prev, prevTime, now) []MetricPoint { /* pure */ }

// 4. pgss guard (statements collectors)
ok, err := pgssAvailable(ctx, conn)
if !ok { return nil, nil }

// 5. Pure graph function (lock_tree)
func computeLockStats(edges []lockEdge) lockStats { /* BFS, no DB */ }
func bfsMaxDepth(startPID int, blocks map[int]map[int]bool) int { /* cycle-safe BFS */ }

// 6. Always-emit with zero-fill (long_transactions)
for _, t := range []string{"active", "waiting"} {
    if !seen[t] { /* emit zeros */ }
}
```

### Metric Naming (complete list)

```
pgpulse.server.*          — uptime, is_in_recovery, is_in_backup, pg_version_num
pgpulse.connections.*     — active, idle, idle_in_transaction, waiting, total, max, utilization_ratio
pgpulse.cache.hit_ratio
pgpulse.transactions.*    — commit_ratio, deadlocks (label: db_name)
pgpulse.database.*        — size_bytes (label: db_name)
pgpulse.extensions.*      — pgss_installed, pgss_fill_pct, pgss_stats_reset_unix
pgpulse.replication.*     — lag.*, slot.*, active_replicas, replica.*, wal_receiver.*
pgpulse.progress.*        — vacuum.*, cluster.*, analyze.*, create_index.*, basebackup.*, copy.*
pgpulse.checkpoint.*      — timed, requested, write/sync_time_ms, buffers_written, restartpoints_*
pgpulse.bgwriter.*        — buffers_clean, maxwritten_clean, buffers_alloc, buffers_backend*
pgpulse.io.*              — reads, read_time, writes, write_time, extends, hits, evictions, fsyncs*
pgpulse.statements.*      — max, fill_pct, track, track_io_timing, count, stats_reset_age_seconds
pgpulse.statements.top.*  — total_time_ms, io_time_ms, cpu_time_ms, calls, rows, avg_time_ms
pgpulse.wait_events.*     — count (labels: wait_event_type, wait_event), total_backends
pgpulse.locks.*           — blocker_count, blocked_count, max_chain_depth
pgpulse.long_transactions.* — count, oldest_seconds (label: type=active|waiting)
```

---

## 8. HOW TO RESTORE THIS SAVE POINT

### Option A: Continue in Same Claude.ai Project
1. Open new chat in the PGPulse project
2. Upload this save point file
3. Say: "Restoring from save point M1. M1_05 is done. Decide next: M1_06 (analiz_db.php), M2 (Storage & API), or wiring main.go."

### Option B: New Claude.ai Project / Different Tool
1. Clone: `git clone https://github.com/ios9000/PGPulse_01.git` (commit c7db1e8)
2. Read this file for complete context
3. Key files: `internal/collector/collector.go`, `internal/collector/base.go`, `internal/version/gate.go`
4. All collector code is in `internal/collector/` — 20 production files, fully tested
