# PGPulse — Save Point

**Save Point:** M1 (in progress) — Core Collector
**Date:** 2026-02-26
**Commit:** d2528ee (after M1_04)
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
| Language | Go | 1.24.0 | Upgraded from 1.23.6 during M1_01; pgx v5.8.0 requires ≥ 1.24 |
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
│  │  io_stats       │  │           │  │          │   │
│  │  statements_*   │  └───────────┘  └──────────┘   │
│  │  (M1_04)        │                                 │
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
| D7 | Hybrid agent workflow | Claude Code bash broken on Windows; agents create files, dev runs bash | 2026-02-25 |
| D8 | Agent Teams (4 agents max) | Right-sized for 1-dev project | 2026-02-25 |
| D9 | Three-tier persistence | Save Points + Handoffs + Session-logs | 2026-02-25 |
| D10 | Base struct with point() helper | All collectors embed Base; auto-prefixes "pgpulse.", fills InstanceID + Timestamp | 2026-02-25 |
| D11 | Registry pattern for collectors | RegisterCollector() via init(), CollectAll() with partial-failure semantics | 2026-02-25 |
| D12 | 5s statement_timeout for live collectors | Via context.WithTimeout in queryContext() helper, not SQL SET | 2026-02-25 |
| D13 | InstanceContext SSoT for per-cycle state | Orchestrator queries pg_is_in_recovery() once per cycle, passes to all collectors. Avoids redundant queries. | 2026-02-25 |
| D14 | Version in Base, IsRecovery in InstanceContext | PG version is structural (immutable). Recovery state is dynamic (changes on failover). | 2026-02-25 |
| D15 | Defer logical replication Q41 | Requires per-DB connections, breaks single-conn Collector interface. | 2026-02-25 |
| D16 | golangci-lint v2 config format | v1 doesn't support Go 1.24. Config requires `version: "2"` field. `gosimple` removed. | 2026-02-25 |
| D17 | Docker Desktop not available | BIOS virtualization disabled. Integration tests CI-only. | 2026-02-25 |
| D18 | Stateful checkpoint collector with snapshot + rate pattern | Checkpoint/bgwriter counters are cumulative. Need deltas for per-second rates. | 2026-02-26 |
| D19 | -1 sentinel for version-unavailable columns | PG 14-16 lacks restartpoints; PG 17 lacks buffers_backend. Use -1 (not 0, not NULL). | 2026-02-26 |
| D20 | completionPct() shared helper for all progress collectors | Six progress collectors all need safe division. One helper in progress_vacuum.go. | 2026-02-26 |
| D21 | Multiple collectors per file for related operations | Group by similarity: progress_maintenance.go (cluster+analyze), progress_operations.go (index+basebackup+copy). | 2026-02-26 |
| D22 | pg_stat_io deferred to M1_03b | PG 16+ only, high cardinality, needs granularity design. Not in PGAM audit. | 2026-02-26 |
| D23 | pgssAvailable() shared helper in base.go | Both statements collectors need the same EXISTS check. One package-level function avoids duplication. | 2026-02-26 |
| D24 | buildMetrics()/buildTopMetrics() extracted as pure methods | Enables unit testing without DB (same pattern as computeMetrics() in checkpoint). | 2026-02-26 |
| D25 | Top-N unified query (Q50+Q51 combined) | Single SQL CTE ranks by total_exec_time and derives IO/CPU from same scan. Avoids two separate queries. | 2026-02-26 |
| D26 | "Other" bucket uses unclamped sums | other = totals - actual_sum preserves accuracy even when top-N has negative cpu values. | 2026-02-26 |
| D27 | *float64 for nullable stats_reset scan | pgx v5 sets *float64 to nil for NULL. Avoids COALESCE sentinel; metric is simply not emitted. | 2026-02-26 |
| D28 | Single Claude Code session for M1_04 | Scope was 2 collectors + 2 test files + 1 helper. Agent Teams overhead not justified. | 2026-02-26 |

---

## 3. CODEBASE STATE

### File Tree (after M1_04)
```
.claude/CLAUDE.md
.claude/rules/Chat_Transition_Process.md
.claude/rules/Save_Point_System.md
.claude/rules/architecture.md
.claude/rules/code-style.md
.claude/rules/postgresql.md
.claude/rules/security.md
.claude/settings.local.json
.github/workflows/ci.yml
.gitignore
.golangci.yml
Makefile
README.md
cmd/pgpulse-agent/main.go
cmd/pgpulse-server/main.go
configs/pgpulse.example.yml
deploy/docker/Dockerfile
deploy/docker/docker-compose.yml
deploy/helm/.gitkeep
deploy/systemd/.gitkeep
docs/CHANGELOG.md
docs/PGPulse_Development_Strategy_v2.md
docs/RESTORE_CONTEXT.md
docs/roadmap.md
docs/iterations/HANDOFF_M0_to_M1.md
docs/iterations/HANDOFF_M1_01_to_M1_02.md
docs/iterations/HANDOFF_M1_02_to_M1_03.md
docs/iterations/HANDOFF_M1_03_to_M1_03b.md
docs/iterations/HANDOFF_M1_03b_to_M1_04.md
docs/iterations/M0_01_02262026_project-setup/...
docs/iterations/M1_01_02252026_collector-instance/...
docs/iterations/M1_02_02262026_replication/...
docs/iterations/M1_02a_02252026_interface-refactor/...
docs/iterations/M1_02b_02252026_replication-collectors/...
docs/iterations/M1_03_02262026_progress-checkpoint-bgwriter/...
docs/iterations/M1_04_02262026_statements/design.md
docs/iterations/M1_04_02262026_statements/requirements.md
docs/iterations/M1_04_02262026_statements/team-prompt.md
docs/save-points/LATEST.md
docs/save-points/SAVEPOINT_M0_20260225.md
docs/save-points/SAVEPOINT_M1_20260225.md
docs/save-points/SAVEPOINT_M1_20260226.md
docs/save-points/SAVEPOINT_M1_20260226b.md    ← THIS FILE
go.mod
go.sum
internal/alert/.gitkeep
internal/alert/notifier/.gitkeep
internal/api/.gitkeep
internal/auth/.gitkeep
internal/collector/base.go                    ← MODIFIED (M1_04: pgssAvailable)
internal/collector/cache.go
internal/collector/cache_test.go
internal/collector/checkpoint.go
internal/collector/checkpoint_test.go
internal/collector/collector.go
internal/collector/connections.go
internal/collector/connections_test.go
internal/collector/database_sizes.go
internal/collector/database_sizes_test.go
internal/collector/extensions.go
internal/collector/extensions_test.go
internal/collector/io_stats.go                ← NEW (M1_03b)
internal/collector/io_stats_test.go           ← NEW (M1_03b)
internal/collector/progress_maintenance.go
internal/collector/progress_maintenance_test.go
internal/collector/progress_operations.go
internal/collector/progress_operations_test.go
internal/collector/progress_vacuum.go
internal/collector/progress_vacuum_test.go
internal/collector/registry.go
internal/collector/registry_test.go
internal/collector/replication_lag.go
internal/collector/replication_lag_test.go
internal/collector/replication_slots.go
internal/collector/replication_slots_test.go
internal/collector/replication_status.go
internal/collector/replication_status_test.go
internal/collector/server_info.go
internal/collector/server_info_test.go
internal/collector/settings.go
internal/collector/settings_test.go
internal/collector/statements_config.go       ← NEW (M1_04)
internal/collector/statements_config_test.go  ← NEW (M1_04)
internal/collector/statements_top.go          ← NEW (M1_04)
internal/collector/statements_top_test.go     ← NEW (M1_04)
internal/collector/testutil_test.go
internal/collector/transactions.go
internal/collector/transactions_test.go
internal/config/.gitkeep
internal/ml/.gitkeep
internal/rca/.gitkeep
internal/storage/.gitkeep
internal/version/gate.go
internal/version/version.go
migrations/.gitkeep
web/.gitkeep
```

### Key Interfaces (unchanged since M1_02a)

```go
// internal/collector/collector.go

type InstanceContext struct {
    IsRecovery bool // true when instance is a replica (standby)
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

type MetricStore interface {
    Write(ctx context.Context, points []MetricPoint) error
    Query(ctx context.Context, query MetricQuery) ([]MetricPoint, error)
    Close() error
}

type AlertEvaluator interface {
    Evaluate(ctx context.Context, metric string, value float64, labels map[string]string) error
}
```

```go
// internal/collector/base.go

type Base struct {
    instanceID string
    pgVersion  version.PGVersion
    interval   time.Duration
}

func newBase(instanceID string, v version.PGVersion, interval time.Duration) Base
func (b *Base) point(metric string, value float64, labels map[string]string) MetricPoint
func (b *Base) Interval() time.Duration
func queryContext(ctx context.Context) (context.Context, context.CancelFunc) // 5s timeout
func pgssAvailable(ctx context.Context, conn *pgx.Conn) (bool, error)       // NEW M1_04
```

```go
// internal/version/gate.go (unchanged since M0)

type VersionRange struct { MinMajor, MinMinor, MaxMajor, MaxMinor int }
type SQLVariant struct { Range VersionRange; SQL string }
type Gate struct { Name string; Variants []SQLVariant }
func (g Gate) Select(v PGVersion) (string, bool)
```

### Dependencies (go.mod — unchanged since M1_03b)
```
module github.com/ios9000/PGPulse_01

go 1.24.0

require (
    github.com/go-chi/chi/v5 v5.2.1
    github.com/jackc/pgx/v5 v5.8.0
    github.com/stretchr/testify v1.10.0
    github.com/testcontainers/testcontainers-go v0.40.0
    github.com/testcontainers/testcontainers-go/modules/postgres v0.40.0
    gonum.org/v1/gonum v0.15.1
)
```

---

## 4. LEGACY KNOWLEDGE (PGAM)

### Query Porting Status

| Source | Queries | Target | Status |
|--------|---------|--------|--------|
| analiz2.php Q1 | PG version string | version.Detect() | ✅ Done (M0) |
| analiz2.php Q2–Q3, Q9–Q10 | Start time, uptime, recovery, backup | server_info.go | ✅ Done (M1_01) |
| analiz2.php Q4–Q8 | OS metrics (hostname, distro, uptime, time, RAM) | — | ⏭️ Deferred to M6 (Go agent via procfs) |
| analiz2.php Q11–Q13 | Connections per-state, max, reserved, utilization | connections.go | ✅ Done (M1_01) |
| analiz2.php Q14 | Global cache hit ratio | cache.go | ✅ Done (M1_01) |
| analiz2.php Q15 | Per-DB commit ratio + deadlocks | transactions.go | ✅ Done (M1_01) |
| analiz2.php Q16 | Per-DB size bytes | database_sizes.go | ✅ Done (M1_01) |
| analiz2.php Q17 | track_io_timing, shared_buffers, etc. | settings.go | ✅ Done (M1_01) |
| analiz2.php Q18–Q19 | pgss presence, fill%, stats_reset | extensions.go | ✅ Done (M1_01) |
| analiz2.php Q20–Q21 | Active replicas, WAL receiver | replication_status.go | ✅ Done (M1_02b) |
| analiz2.php Q22–Q35 | Memory, top, df, iostat, Patroni, ETCD, etc. | — | 🔲 M6/later milestones |
| analiz2.php Q36 | Replication lag PG < 10 | — | ⏭️ Skipped (below min PG 14) |
| analiz2.php Q37–Q38 | Replication lag (bytes + time) | replication_lag.go | ✅ Done (M1_02b) |
| analiz2.php Q39 | Replication slots PG < 10 | — | ⏭️ Skipped (below min PG 14) |
| analiz2.php Q40 | Replication slots PG ≥ 10 | replication_slots.go | ✅ Done (M1_02b) |
| analiz2.php Q41 | Logical replication sync | — | ⏭️ Deferred (needs PerDatabaseCollector) |
| analiz2.php Q42 | Vacuum progress | progress_vacuum.go | ✅ Done (M1_03) |
| analiz2.php Q43 | Cluster/vacuum full progress | progress_maintenance.go | ✅ Done (M1_03) |
| analiz2.php Q44 | Create index progress | progress_operations.go | ✅ Done (M1_03) |
| analiz2.php Q45 | Analyze progress | progress_maintenance.go | ✅ Done (M1_03) |
| analiz2.php Q46 | Basebackup progress | progress_operations.go | ✅ Done (M1_03) |
| analiz2.php Q47 | Copy progress | progress_operations.go | ✅ Done (M1_03) |
| analiz2.php Q48–Q49 | pgss settings, fill%, stats_reset age | statements_config.go | ✅ Done (M1_04) |
| analiz2.php Q50–Q51 | Top-N by IO time + CPU time | statements_top.go | ✅ Done (M1_04) |
| analiz2.php Q52 | Normalized query stats | — | ⏭️ Deferred — queryid-level breakdown in Q50/Q51 covers the use case |
| analiz2.php Q53–Q58 | Locks & wait events | collector/locks.go | 🔲 M1_05 |
| analiz_db.php Q1–Q18 | Per-DB analysis | collector/database.go | 🔲 Later milestone |
| — (new) | Checkpoint/bgwriter stats | checkpoint.go | ✅ Done (M1_03) |
| — (new) | pg_stat_io | io_stats.go | ✅ Done (M1_03b) |
| **Total: 76** | | | **28 done, 10 deferred/skipped, 38 remaining** |

### PGAM Bugs Fixed During Port

| # | Query | Bug | Fix |
|---|-------|-----|-----|
| 1 | Q11 | Connection count includes own monitoring connection | Added `WHERE pid != pg_backend_pid()` |
| 2 | Q14 | Cache hit ratio division by zero when blks_hit + blks_read = 0 | Added `NULLIF(blks_hit + blks_read, 0)` guard |
| 3 | Q4-Q8 | OS metrics via COPY TO PROGRAM requires superuser | Eliminated entirely — Go agent via procfs (M6) |
| 4 | Q10 | pg_is_in_backup() called unconditionally (removed in PG 15) | Version-gated: skip for PG ≥ 15 |
| 5 | Q50-Q51 | Separate IO and CPU queries require two round-trips | Unified into single CTE ranking on total_exec_time, deriving both IO and CPU |

### Version Gates Implemented

| # | Query | Gate | Variants |
|---|-------|------|----------|
| 1 | Q10 | is_in_backup | PG ≤ 14: `SELECT pg_is_in_backup()` / PG ≥ 15: skip (removed) |
| 2 | Q19 | pgss_info | PG ≤ 13: skip / PG ≥ 14: `SELECT * FROM pg_stat_statements_info` |
| 3 | Q40 | replication_slots | PG 14: base cols / PG 15: + `two_phase` / PG 16+: + `conflicting` |
| 4 | new | checkpoint_stats | PG 14–16: `pg_stat_bgwriter` (combined) / PG 17+: `pg_stat_checkpointer` CROSS JOIN `pg_stat_bgwriter` |
| 5 | new | io_stats | PG < 16: return nil, nil / PG ≥ 16: query `pg_stat_io` |

---

## 5. MILESTONE STATUS

### Roadmap

| Milestone | Name | Status | Completion Date |
|---|---|---|---|
| M0 | Project Setup | ✅ Done | 2026-02-25 |
| M1 | Core Collector | 🔶 In progress (M1_01–M1_04 done, M1_05 next) | — |
| M2 | Storage & API | 🔲 Not started | — |
| M3 | Auth & Security | 🔲 Not started | — |
| M4 | Alerting | 🔲 Not started | — |
| M5 | Web UI (MVP) | 🔲 Not started | — |
| M6 | Agent Mode | 🔲 Not started | — |
| M7 | P1 Features | 🔲 Not started | — |
| M8 | ML Phase 1 | 🔲 Not started | — |
| M9 | Reports & Export | 🔲 Not started | — |
| M10 | Polish & Release | 🔲 Not started | — |

### M1 Sub-Iteration Status

| Sub-Iteration | Scope | Status |
|---|---|---|
| M1_01 | Instance metrics: server_info, connections, cache, transactions, database_sizes, settings, extensions, registry | ✅ Done |
| M1_02a | Interface refactor: add InstanceContext to Collector.Collect() signature | ✅ Done (c50dbe1) |
| M1_02b | Replication collectors: replication_lag, replication_slots, replication_status | ✅ Done |
| M1_03 | Progress monitoring: vacuum, analyze, index, cluster, basebackup, copy (Q42–Q47) + checkpoint/bgwriter stats | ✅ Done (f96ce2f) |
| M1_03b | pg_stat_io collector (PG 16+) | ✅ Done (7963eff) |
| M1_04 | pg_stat_statements: config/fill%/reset age (Q48–Q49) + top-N by time/IO/CPU (Q50–Q51) | ✅ Done (d2528ee) |
| M1_05 | Locks & wait events: wait event summary, blocking tree, long transactions (Q53–Q58) | 🔲 Not started |

### What Was Just Completed (M1_04)

**M1_04 — pg_stat_statements collectors (Q48–Q51). Commit d2528ee.**

4 production files modified/created + 2 test files (743 lines total).

**`pgssAvailable()` helper (base.go):**
- Package-level function, shared by both statements collectors
- Queries `pg_extension` with EXISTS for efficiency
- Uses standard `queryContext()` 5s timeout
- Returns `(bool, error)` — callers return `nil, nil` when false

**`StatementsConfigCollector` (statements_config.go) — Q48 + Q49:**
- 60s interval
- Three sequential queries: pg_settings (3 rows), count(*) from pgss, EXTRACT(EPOCH...) from pgss_info
- `buildMetrics()` pure method for unit testing
- Emits 6 metrics normally; 5 when stats_reset is NULL (uses `*float64` nullable scan)
- Metrics: `statements.max`, `statements.fill_pct`, `statements.track` (with label), `statements.track_io_timing`, `statements.count`, `statements.stats_reset_age_seconds`

**`StatementsTopCollector` (statements_top.go) — Q50 + Q51 unified:**
- 60s interval, configurable limit (default 20)
- Single CTE query: ranks by total_exec_time, CROSS JOINs totals for "other" computation
- `buildTopMetrics()` pure method for unit testing
- Emits 6 metrics per top-N row: `total_time_ms`, `io_time_ms`, `cpu_time_ms`, `calls`, `rows`, `avg_time_ms` (labels: `queryid`, `dbid`, `userid`)
- Emits 6 "other" bucket metrics when top-N doesn't cover all calls (labels: `queryid=other`, `dbid=all`, `userid=all`)
- `cpu_time_ms` clamped to 0 when negative (io timing can exceed exec time at short durations)
- Q52 (normalized stats) deferred — queryid-level breakdowns in Q50/Q51 cover the use case

**Tests (11 run, 2 skip, 0 fail):**
- Config: NameAndInterval, Normal (6 metrics), NullStatsReset (5 metrics), TrackIoTimingOn (1.0), TrackIoTimingOff (0.0)
- Top: NameAndInterval+DefaultLimit, EmptyPgss, NormalTopN (18 metrics), FewerThanLimit (no other), NegativeCpuTime (clamped), OtherBucketArithmetic (verified math)
- 2 integration stubs skip (PgssNotInstalled) — require Docker

**All checks green:** `go build`, `go vet`, `golangci-lint run` (0 issues), full collector suite.

### Previously Completed

**M1_01** — 8 collector files, 13 PGAM queries, base.go, registry.go.
**M1_02a** — InstanceContext interface refactor (c50dbe1).
**M1_02b** — 3 replication collectors, 5 PGAM queries.
**M1_03** — 6 progress collectors + CheckpointCollector (stateful, version-gated).
**M1_03b** — IOStatsCollector (pg_stat_io, PG 16+, per-row metrics).

### What's Next (M1_05)

Locks & wait events (Q53–Q58): wait event summary, blocking/blocked query tree, long-running transactions. Likely 2–3 collector files.

PGAM queries to port:
- Q53: Wait event summary (`SELECT wait_event_type, wait_event, count(*) FROM pg_stat_activity GROUP BY 1, 2`)
- Q54: Blocking tree (self-join on `pg_stat_activity.wait_event = 'relation'` + `pg_locks`)
- Q55–Q58: Long transactions, idle-in-transaction, lock acquisition times

---

## 6. DEVELOPMENT ENVIRONMENT

### Developer Workstation
| Component | Value |
|---|---|
| OS | Windows 10 |
| Shell | Git Bash (MSYS2) + PowerShell |
| Go | 1.24.0 windows/amd64 |
| Node.js | 22.14.0 |
| Claude Code | 2.1.59 |
| Git | 2.52.0 |
| golangci-lint | v2.10.1 (built with go1.26.0) |
| testcontainers-go | 0.40.0 (requires Docker Desktop) |
| Docker Desktop | Not installed (BIOS virtualization disabled) |

### Development Method
- **Two-contour model:** Claude.ai (Brain — architecture, planning) + Claude Code (Hands — implementation)
- **M1_04 ran as single Claude Code session** (Sonnet 4.6) — no Agent Teams overhead for small scope
- **Hybrid workflow:** Claude Code creates files; developer runs go build/test/commit manually (bash works in direct session, broken in Agent Teams subprocess mode)
- **One chat per iteration** in Claude.ai
- **Iteration Handoff** documents bridge between chats

### Known Environment Issues

| Issue | Status | Workaround |
|---|---|---|
| Claude Code bash EINVAL in Agent Teams subprocess mode | Unresolved | Single-session Claude Code bash works; avoid spawning agents |
| LF/CRLF warnings | Needs .gitattributes | Add `* text=auto eol=lf` |
| WSL2 unavailable | BIOS virtualization disabled | Using native Git Bash |
| Docker Desktop unavailable | BIOS virtualization disabled | Integration tests run in CI only |
| Go upgraded from 1.23.6 → 1.24.0 | Accepted | pgx v5.8.0 requires ≥ 1.24 |
| golangci-lint v1 → v2 | Upgraded to v2.10.1 | Config requires `version: "2"`, `gosimple` removed |

---

## 7. KEY LEARNINGS & DECISIONS LOG

### Issues & Resolutions

| Date | Issue | Resolution |
|---|---|---|
| 2026-02-25 | Claude Code bash EINVAL on Windows (Agent Teams) | Not resolved — single-session mode works fine |
| 2026-02-25 | GitHub PAT missing workflow scope | Added workflow scope to PAT |
| 2026-02-25 | WSL2 install failed (BIOS virtualization) | Abandoned WSL, using native Git Bash |
| 2026-02-25 | New chat lost all context | Created three-tier persistence system (save points + handoffs + session-logs) |
| 2026-02-25 | golangci-lint v1 incompatible with Go 1.24 | Upgraded to v2.10.1. Config `version: "2"`, removed `gosimple` |
| 2026-02-25 | Docker Desktop not available | Integration tests CI-only. Unit tests with mocks locally. |
| 2026-02-25 | Design doc showed Gate with int min/max | Actual code uses VersionRange{MinMajor, MinMinor, MaxMajor, MaxMinor}. |
| 2026-02-26 | RegisterCollector/init() auto-registration not used | Collectors registered explicitly in main.go. Strategy doc was incorrect. |
| 2026-02-26 | Agent Teams QA agent wrote test with wrong field names | Field names mismatched (checkpointsRequested vs checkpointsReq). Fixed before commit. |
| 2026-02-26 | Q52 normalized stats — port or skip? | Deferred: queryid-level breakdown in Q50/Q51 covers the use case. PGAM's regex normalization unnecessary since PG 14+ has native queryid. |

---

## 8. COLLECTOR IMPLEMENTATION PATTERNS

### All Established Patterns (cumulative)

```go
// Pattern 1: Basic stateless collector
type MyCollector struct { Base }
func NewMyCollector(instanceID string, v version.PGVersion) *MyCollector {
    return &MyCollector{Base: newBase(instanceID, v, 60*time.Second)}
}
func (c *MyCollector) Name() string { return "my_collector" }
func (c *MyCollector) Collect(ctx context.Context, conn *pgx.Conn, ic InstanceContext) ([]MetricPoint, error) {
    qCtx, cancel := queryContext(ctx)
    defer cancel()
    // query, scan, return c.point(...) slices
}

// Pattern 2: Version-gated SQL
var myGate = version.Gate{
    Name: "my_gate",
    Variants: []version.SQLVariant{
        {Range: version.VersionRange{MinMajor: 14, MinMinor: 0, MaxMajor: 16, MaxMinor: 99}, SQL: `...`},
        {Range: version.VersionRange{MinMajor: 17, MinMinor: 0, MaxMajor: 99, MaxMinor: 99}, SQL: `...`},
    },
}

// Pattern 3: Stateful collector (rate computation)
type MyStateful struct {
    Base; sqlGate version.Gate; mu sync.Mutex; prev *mySnapshot; prevTime time.Time
}
func (c *MyStateful) computeMetrics(curr mySnapshot, prev *mySnapshot, prevTime, now time.Time) []MetricPoint {
    // absolutePoints always; ratePoints only when prev != nil && elapsed > 0 && !isStatsReset
}

// Pattern 4: pgss-availability guard (M1_04)
func (c *MyCollector) Collect(...) ([]MetricPoint, error) {
    ok, err := pgssAvailable(ctx, conn)
    if err != nil { return nil, err }
    if !ok { return nil, nil }
    // ... rest of collection
}

// Pattern 5: Pure buildMetrics method for testability (M1_04)
func (c *MyCollector) buildMetrics(data someStruct) []MetricPoint {
    // No DB access; all inputs are pre-scanned values
}
```

### Metric Naming Convention

```
pgpulse.<category>.<metric>

// M1_04 additions:
pgpulse.statements.max                          (no labels)
pgpulse.statements.fill_pct                     (no labels)
pgpulse.statements.track                        (label: value=all|top|none)
pgpulse.statements.track_io_timing              (no labels; 1=on, 0=off)
pgpulse.statements.count                        (no labels)
pgpulse.statements.stats_reset_age_seconds      (no labels; omitted when NULL)
pgpulse.statements.top.total_time_ms            (labels: queryid, dbid, userid)
pgpulse.statements.top.io_time_ms               (labels: queryid, dbid, userid)
pgpulse.statements.top.cpu_time_ms              (labels: queryid, dbid, userid)
pgpulse.statements.top.calls                    (labels: queryid, dbid, userid)
pgpulse.statements.top.rows                     (labels: queryid, dbid, userid)
pgpulse.statements.top.avg_time_ms              (labels: queryid, dbid, userid)
// "other" bucket uses queryid=other, dbid=all, userid=all
```

---

## 9. HOW TO RESTORE THIS SAVE POINT

### Option A: Continue in Same Claude.ai Project
1. Open new chat in the PGPulse project
2. Upload this save point file
3. Say: "Restoring from save point M1. Continue with M1_05 (locks & wait events)."

### Option B: New Claude.ai Project from Scratch
1. Create new Claude.ai Project named "PGPulse"
2. Upload to Project Knowledge: this file + PGAM_FEATURE_AUDIT.md + PGPulse_Development_Strategy_v2.md
3. Open new chat, upload this save point
4. Say: "Restoring PGPulse from save point M1. All context is in this file."

### Option C: Different AI Tool / New Developer
1. Clone: `git clone https://github.com/ios9000/PGPulse_01.git`
2. Read this save point file — it contains complete project context
3. Key files: `internal/collector/collector.go` (interfaces), `internal/collector/base.go` (helpers), `internal/version/gate.go` (version-adaptive SQL)
4. Continue from M1_05 (locks & wait events, Q53–Q58)
