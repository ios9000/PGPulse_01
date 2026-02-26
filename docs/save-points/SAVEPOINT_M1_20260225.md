# PGPulse — Save Point

**Save Point:** M1 (in progress) — Core Collector  
**Date:** 2026-02-26  
**Commit:** c50dbe1 (M1_02a) + [M1_02b commit hash — fill in]  
**Developer:** Evlampiy (ios9000)  
**AI Tool:** Claude.ai (Opus 4.6) + Claude Code 2.1.53 (Agent Teams)

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
│  │  (M1_02b)       │  │  Engine   │  │ (embed)  │   │
│  └───────┬─────────┘  └───────────┘  └──────────┘   │
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
| D13 | InstanceContext SSoT for per-cycle state | Orchestrator queries pg_is_in_recovery() once per cycle, passes to all collectors. Avoids redundant queries. Enables role-aware replication collectors. | 2026-02-25 |
| D14 | Version in Base, IsRecovery in InstanceContext | PG version is structural (immutable for connection lifetime). Recovery state is dynamic (changes on failover). Different scopes → different homes. | 2026-02-25 |
| D15 | Defer logical replication Q41 | Requires per-DB connections, which breaks single-conn Collector interface. Defer until PerDatabaseCollector interface designed (alongside analiz_db.php queries). | 2026-02-25 |
| D16 | golangci-lint v2 config format | v1 doesn't support Go 1.24. Config requires `version: "2"` field. `gosimple` linter removed (merged into `staticcheck`). | 2026-02-25 |
| D17 | Docker Desktop not available | BIOS virtualization disabled. Integration tests (testcontainers) run in CI only. Unit tests with mocks work locally. | 2026-02-25 |

---

## 3. CODEBASE STATE

### File Tree (after M1_02b)
```
.claude/CLAUDE.md
.claude/rules/architecture.md
.claude/rules/code-style.md
.claude/rules/postgresql.md
.claude/rules/security.md
.claude/rules/chat-transition.md
.claude/rules/save-point.md
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
docs/iterations/M0_01_02262026_project-setup/design.md
docs/iterations/M0_01_02262026_project-setup/requirements.md
docs/iterations/M0_01_02262026_project-setup/session-log.md
docs/iterations/M0_01_02262026_project-setup/team-prompt.md
docs/save-points/SAVEPOINT_M0_20260225.md
docs/save-points/SAVEPOINT_M1_20260225.md
docs/save-points/LATEST.md
go.mod
go.sum
internal/alert/.gitkeep
internal/alert/notifier/.gitkeep
internal/api/.gitkeep
internal/auth/.gitkeep
internal/collector/collector.go
internal/collector/base.go
internal/collector/server_info.go
internal/collector/connections.go
internal/collector/cache.go
internal/collector/transactions.go
internal/collector/database_sizes.go
internal/collector/settings.go
internal/collector/extensions.go
internal/collector/replication_lag.go
internal/collector/replication_slots.go
internal/collector/replication_status.go
internal/collector/registry.go
internal/collector/testutil_test.go
internal/collector/server_info_test.go
internal/collector/connections_test.go
internal/collector/cache_test.go
internal/collector/transactions_test.go
internal/collector/database_sizes_test.go
internal/collector/settings_test.go
internal/collector/extensions_test.go
internal/collector/replication_lag_test.go
internal/collector/replication_slots_test.go
internal/collector/replication_status_test.go
internal/collector/registry_test.go
internal/config/.gitkeep
internal/ml/.gitkeep
internal/rca/.gitkeep
internal/storage/.gitkeep
internal/version/gate.go
internal/version/version.go
migrations/.gitkeep
web/.gitkeep
```

### Key Interfaces

#### Collector Interface (current — after M1_02a)

```go
// internal/collector/collector.go

// InstanceContext holds per-scrape-cycle metadata about the PostgreSQL
// instance. Queried once by the orchestrator, passed to all collectors.
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

#### Base Struct Pattern (from base.go)

```go
type Base struct {
    instanceID string
    pgVersion  version.PGVersion
    interval   time.Duration
}

func newBase(instanceID string, v version.PGVersion, interval time.Duration) Base
func (b *Base) point(metric string, value float64, labels map[string]string) MetricPoint
func (b *Base) Interval() time.Duration

// Shared 5s timeout for all live collectors
func queryContext(ctx context.Context) (context.Context, context.CancelFunc)
```

#### Version Gate (from gate.go — unchanged since M0)

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

#### PGVersion (from version.go — unchanged since M0)

```go
type PGVersion struct {
    Major int
    Minor int
    Num   int
    Full  string
}

func Detect(ctx context.Context, conn *pgx.Conn) (PGVersion, error)
func (v PGVersion) AtLeast(major, minor int) bool
```

#### Registry (from registry.go)

```go
// RegisterCollector registers a factory that creates a Collector.
// NOTE: This function exists but is NOT used by any collector.
// All collectors are registered EXPLICITLY in main.go.
// The init()/RegisterCollector() auto-registration pattern described
// in the strategy doc and CLAUDE.md is incorrect.
func RegisterCollector(factory func(instanceID string, v version.PGVersion) Collector)

// CollectAll runs all registered collectors, returns partial results on errors.
func CollectAll(ctx context.Context, conn *pgx.Conn, ic InstanceContext, collectors []Collector) ([]MetricPoint, []error)
```

**⚠️ IMPORTANT:** Collectors are registered explicitly in main.go, NOT via init(). The strategy doc, CLAUDE.md, and earlier design docs reference auto-registration — this is wrong. Follow the explicit pattern for all new collectors.

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
| analiz2.php Q22–Q35 | Memory, top, df, iostat, Patroni, ETCD, databases overview, event triggers | — | 🔲 M6/later milestones |
| analiz2.php Q36 | Replication lag PG < 10 | — | ⏭️ Skipped (below min PG 14) |
| analiz2.php Q37–Q38 | Replication lag (bytes + time) | replication_lag.go | ✅ Done (M1_02b) |
| analiz2.php Q39 | Replication slots PG < 10 | — | ⏭️ Skipped (below min PG 14) |
| analiz2.php Q40 | Replication slots PG ≥ 10 | replication_slots.go | ✅ Done (M1_02b) |
| analiz2.php Q41 | Logical replication sync | — | ⏭️ Deferred (needs PerDatabaseCollector) |
| analiz2.php Q42–Q47 | Progress monitoring | collector/progress.go | 🔲 M1_03 |
| analiz2.php Q48–Q52 | pg_stat_statements | collector/statements.go | 🔲 M1_04 |
| analiz2.php Q53–Q58 | Locks & wait events | collector/locks.go | 🔲 M1_05 |
| analiz_db.php Q1–Q18 | Per-DB analysis | collector/database.go | 🔲 Later milestone |
| **Total: 76** | | | **18 done, 9 deferred/skipped, 49 remaining** |

### PGAM Bugs Fixed During Port

| # | Query | Bug | Fix |
|---|-------|-----|-----|
| 1 | Q11 | Connection count includes own monitoring connection | Added `WHERE pid != pg_backend_pid()` |
| 2 | Q14 | Cache hit ratio division by zero when blks_hit + blks_read = 0 | Added `NULLIF(blks_hit + blks_read, 0)` guard |
| 3 | Q4-Q8 | OS metrics via COPY TO PROGRAM requires superuser | Eliminated entirely — Go agent via procfs (M6) |
| 4 | Q10 | pg_is_in_backup() called unconditionally (removed in PG 15) | Version-gated: skip for PG ≥ 15 |

### Version Gates Implemented

| # | Query | Gate | Variants |
|---|-------|------|----------|
| 1 | Q10 | is_in_backup | PG ≤ 14: `SELECT pg_is_in_backup()` / PG ≥ 15: skip (removed) |
| 2 | Q19 | pgss_info | PG ≤ 13: skip / PG ≥ 14: `SELECT * FROM pg_stat_statements_info` |
| 3 | Q40 | replication_slots | PG 14: base cols / PG 15: + `two_phase` / PG 16+: + `conflicting` |

### Version Gates Planned (M1_03)

| # | Query | Gate | PG 14–16 | PG 17 |
|---|-------|------|----------|-------|
| 4 | New | checkpoint_stats | `pg_stat_bgwriter` (combined) | `pg_stat_checkpointer` + reduced `pg_stat_bgwriter` |

---

## 5. MILESTONE STATUS

### Roadmap

| Milestone | Name | Status | Completion Date |
|---|---|---|---|
| M0 | Project Setup | ✅ Done | 2026-02-25 |
| M1 | Core Collector | 🔶 In progress (M1_01 done, M1_02 planned) | — |
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
| M1_03 | Progress monitoring: vacuum, analyze, index, cluster, basebackup, copy (Q42–Q47) + checkpoint/bgwriter stats | 🔲 Not started |
| M1_04 | pg_stat_statements: IO-sorted, CPU-sorted, normalized total (Q48–Q52) | 🔲 Not started |
| M1_05 | Locks & wait events: wait event summary, blocking tree, long transactions (Q53–Q58) | 🔲 Not started |

### What Was Just Completed (M1_01 + M1_02)

**M1_01 — Instance metrics collector suite (13 PGAM queries ported).**

Created 8 collector files (server_info, connections, cache, transactions, database_sizes, settings, extensions) plus base.go (shared helpers) and registry.go (orchestration). Each collector embeds Base struct, uses point() for metric creation, queryContext() for 5s timeout. Registry supports partial-failure.

**M1_02a — InstanceContext interface refactor.**

Added InstanceContext struct with IsRecovery field to Collector interface. Mechanical signature update across 18 files. ServerInfoCollector now reads ic.IsRecovery instead of querying pg_is_in_recovery() directly. Commit c50dbe1.

**M1_02b — Replication collectors (5 PGAM queries ported).**

Created 3 new collectors: ReplicationLagCollector (Q37+Q38, primary only, 10s), ReplicationSlotsCollector (Q40, version-gated PG 14/15/16+, 60s), ReplicationStatusCollector (Q20 primary + Q21 replica, 60s). All use InstanceContext.IsRecovery for role-aware behavior. 8 unit tests pass, 3 integration stubs skip cleanly.

**Discovery:** RegisterCollector/init() auto-registration pattern from strategy doc is not used. Actual registration is explicit in main.go.

### What's Next (M1_03)

Progress monitoring collectors (Q42–Q47: vacuum, cluster, create index, analyze, basebackup, copy) plus checkpoint/bgwriter stats (PG 17 splits pg_stat_bgwriter) and optionally pg_stat_io (PG 16+ only). Planning docs not yet created — start with M1_03 planning chat using HANDOFF_M1_02_to_M1_03.md.

---

## 6. DEVELOPMENT ENVIRONMENT

### Developer Workstation
| Component | Value |
|---|---|
| OS | Windows 10 |
| Shell | Git Bash (MSYS2) + PowerShell |
| Go | 1.24.0 windows/amd64 |
| Node.js | 22.14.0 |
| Claude Code | 2.1.53 |
| Git | 2.52.0 |
| golangci-lint | v2.10.1 (built with go1.26.0) |
| testcontainers-go | 0.40.0 (requires Docker Desktop) |
| Docker Desktop | Not installed (BIOS virtualization disabled) |
| Agent Teams | Enabled (in-process mode, no tmux) |

### Development Method
- **Two-contour model:** Claude.ai (Brain — architecture, planning) + Claude Code (Hands — implementation)
- **Agent Teams:** Enabled but bash broken on Windows (EINVAL temp path bug)
- **Hybrid workflow:** Agents create files, developer runs go build/test/commit manually
- **One chat per iteration** in Claude.ai
- **Project Knowledge** contains: strategy doc, PGAM audit, chat transition process, save point system
- **Iteration Handoff** documents bridge between chats

### Known Environment Issues

| Issue | Status | Workaround |
|---|---|---|
| Claude Code bash EINVAL on Windows | Unresolved (v2.1.53) | Agents create files, dev runs bash manually |
| LF/CRLF warnings | Needs .gitattributes | Add `* text=auto eol=lf` |
| WSL2 unavailable | BIOS virtualization disabled | Using native Git Bash |
| Docker Desktop unavailable | BIOS virtualization disabled | Integration tests run in CI only |
| Go upgraded from 1.23.6 → 1.24.0 | Accepted | pgx v5.8.0 requires ≥ 1.24 |
| golangci-lint v1 → v2 | Upgraded to v2.10.1 | Config requires `version: "2"`, `gosimple` removed |

---

## 7. KEY LEARNINGS & DECISIONS LOG

### Architecture Decisions (chronological)

| Date | Decision | Alternatives Considered | Why This Choice |
|---|---|---|---|
| 2026-02-25 | Go over Rust | Rust has steeper learning curve, Go goroutines natural for collectors | Faster development, good enough performance |
| 2026-02-25 | pgx over database/sql | database/sql lacks PG-specific features | Named args, COPY, notifications |
| 2026-02-25 | TimescaleDB over InfluxDB | InfluxDB requires separate service | PG-native, one less dependency |
| 2026-02-25 | Agent Teams (4 agents) | 7 agents (enterprise template) | Right-sized for 1-dev project |
| 2026-02-25 | Hybrid mode (agents + manual bash) | Pure Agent Teams | Windows bash bug forced this |
| 2026-02-25 | One collector per file (not per function) | Large collector files with multiple queries | Testability, clear ownership, independent intervals |
| 2026-02-25 | Base struct with point() helper | Each collector builds MetricPoint manually | DRY — auto-prefix, auto-timestamp, consistent naming |
| 2026-02-25 | Registry with init() auto-registration | Manual collector list in main.go | Adding a collector = creating a file, no wiring needed |
| 2026-02-25 | queryContext() with 5s timeout | SQL SET statement_timeout | Go-idiomatic context cancellation, per-query timeout |
| 2026-02-25 | InstanceContext SSoT pattern | Option A (query in each collector), Option B (constructor flag), Option C (context.Value) | Single query per cycle, explicit typed parameter, interface-level contract |
| 2026-02-25 | Version stays in Base, IsRecovery in InstanceContext | Move both to InstanceContext (full refactor) | Version = structural (immutable), IsRecovery = dynamic (changes on failover). Different semantics. |
| 2026-02-25 | Defer Q41 logical replication | Shoehorn multi-DB into single-conn interface | Clean architecture boundary — design PerDatabaseCollector when needed |
| 2026-02-25 | Checkpoint/bgwriter stats deferred to M1_03 | Include in M1_01 | PG 17 splits pg_stat_bgwriter → pg_stat_checkpointer; needs careful version gate |
| 2026-02-25 | pg_stat_io deferred to M1_03 | Include in M1_01 | PG 16+ only; not in PGAM audit, new addition |

### Issues & Resolutions

| Date | Issue | Resolution |
|---|---|---|
| 2026-02-25 | Claude Code bash EINVAL on Windows | Not resolved — hybrid workflow adopted |
| 2026-02-25 | GitHub PAT missing workflow scope | Added workflow scope to PAT |
| 2026-02-25 | WSL2 install failed (BIOS virtualization) | Abandoned WSL, using native Git Bash |
| 2026-02-25 | New chat lost all context | Created three-tier persistence system (save points + handoffs + session-logs) |
| 2026-02-25 | Strategy doc used as history log | Separated: strategy=rules, handoff=transition, session-log=history |
| 2026-02-25 | Agent Teams proposed with 7 agents | Reduced to 4 (right-sized for 1-dev) |
| 2026-02-25 | TMPDIR fix attempted for bash bug | Failed — Claude Code uses internal path, not env var |
| 2026-02-25 | golangci-lint v1 incompatible with Go 1.24 | Upgraded to v2.10.1. Config `version: "2"`, removed `gosimple` |
| 2026-02-25 | Docker Desktop not available on workstation | Integration tests CI-only. Unit tests with mocks locally. |
| 2026-02-25 | Design doc showed Gate with int min/max | Actual M0 code uses VersionRange{MinMajor, MinMinor, MaxMajor, MaxMinor}. Use struct form. |
| 2026-02-26 | RegisterCollector/init() auto-registration not used | Actual code registers collectors explicitly in main.go. Strategy doc, CLAUDE.md, and design docs are incorrect. All future docs corrected. |

### Competitive Intelligence Summary
- **pgwatch v3:** Go-based, SQL metrics, 4 storage backends — closest architectural cousin
- **Percona PMM:** Heavyweight, QAN is gold standard for query analytics
- **pganalyze:** SaaS $149/mo, HypoPG index advisor — bar for query-level analytics
- **Tantor Platform:** Most feature-rich Russian solution, microservice arch, Kubernetes
- **SolarWinds DPA:** ML anomaly detection with seasonal baselines — our ML target

---

## 8. COLLECTOR IMPLEMENTATION PATTERNS

This section captures the patterns established in M1_01 that all future collectors must follow. If restoring from scratch, implement these patterns first.

### Pattern: Collector File Structure

```go
package collector

import (
    "context"
    "fmt"
    "time"

    "github.com/jackc/pgx/v5"
    "github.com/ios9000/PGPulse_01/internal/version"
)

const myCollectorSQL = `SELECT ... FROM pg_stat_something`

// MyCollector collects [description].
// PGAM source: analiz2.php Q[X].
type MyCollector struct {
    Base
    // optional: sqlGate version.Gate (if version-gated SQL)
}

func NewMyCollector(instanceID string, v version.PGVersion) *MyCollector {
    return &MyCollector{
        Base: newBase(instanceID, v, 60*time.Second),
    }
}

func (c *MyCollector) Name() string { return "my_collector" }

func (c *MyCollector) Collect(ctx context.Context, conn *pgx.Conn, ic InstanceContext) ([]MetricPoint, error) {
    // Role check (if applicable):
    // if ic.IsRecovery { return nil, nil }

    qCtx, cancel := queryContext(ctx) // 5s timeout
    defer cancel()

    // Query and scan...
    // Use c.point("category.metric", value, labels) for metric creation
    // Return empty slice (not error) for empty result sets
    return points, nil
}

// NOTE: Collectors are registered EXPLICITLY in main.go.
// Do NOT use init()/RegisterCollector() auto-registration.
// The strategy doc references to auto-registration are incorrect.
```

### Pattern: Version-Gated SQL

```go
var myGate = version.Gate{
    Name: "my_gate",
    Variants: []version.SQLVariant{
        {
            Range: version.VersionRange{MinMajor: 14, MinMinor: 0, MaxMajor: 14, MaxMinor: 99},
            SQL:   `SELECT ... -- PG 14 variant`,
        },
        {
            Range: version.VersionRange{MinMajor: 15, MinMinor: 0, MaxMajor: 99, MaxMinor: 99},
            SQL:   `SELECT ... -- PG 15+ variant`,
        },
    },
}

// In Collect():
sql, ok := c.sqlGate.Select(c.pgVersion)
if !ok {
    return nil, fmt.Errorf("my_collector: no SQL for PG %s", c.pgVersion.Full)
}
```

### Pattern: Metric Naming

```
pgpulse.<category>.<metric>
```
Examples:
- `pgpulse.server.uptime_seconds`
- `pgpulse.server.is_in_recovery`
- `pgpulse.connections.active`
- `pgpulse.connections.utilization_ratio`
- `pgpulse.cache.hit_ratio`
- `pgpulse.transactions.commit_ratio` (label: `db_name`)
- `pgpulse.database.size_bytes` (label: `db_name`)
- `pgpulse.replication.lag.pending_bytes` (labels: `app_name`, `client_addr`, `state`)
- `pgpulse.replication.lag.write_seconds` (labels: `app_name`, `client_addr`)
- `pgpulse.replication.lag.total_bytes` (labels: `app_name`, `client_addr`, `state`)
- `pgpulse.replication.slot.retained_bytes` (labels: `slot_name`, `slot_type`, `active`, `two_phase`*, `conflicting`*)
- `pgpulse.replication.slot.active` (labels: `slot_name`, `slot_type`)
- `pgpulse.replication.active_replicas` (aggregate count, no labels)
- `pgpulse.replication.replica.connected` (labels: `app_name`, `client_addr`, `state`, `sync_state`)
- `pgpulse.replication.wal_receiver.connected` (labels: `sender_host`, `sender_port`)
- `pgpulse.replication.wal_receiver.lag_bytes` (labels: `sender_host`, `sender_port`)

\* Version-conditional: `two_phase` on PG 15+, `conflicting` on PG 16+.

### Pattern: Test File Structure

```go
package collector

import (
    "testing"
    "github.com/ios9000/PGPulse_01/internal/version"
)

func TestMyCollector_NameAndInterval(t *testing.T) {
    c := NewMyCollector("test", version.PGVersion{Major: 16, Minor: 0})
    if c.Name() != "my_collector" { t.Errorf(...) }
    if c.Interval() != 60*time.Second { t.Errorf(...) }
}

func TestMyCollector_GateSelection(t *testing.T) {
    // Test version gate selects correct SQL variant
}

// //go:build integration
func TestMyCollector_Integration_PG16(t *testing.T) {
    // Uses setupPG() from testutil_test.go (testcontainers)
}
```

---

## 9. HOW TO RESTORE THIS SAVE POINT

### Option A: Continue in Same Claude.ai Project
1. Open new chat in the PGPulse project
2. Upload this save point file
3. Say: "Restoring from save point M1. Continue with M1_03 (progress monitoring)."
4. Project Knowledge already has: strategy doc, PGAM audit, chat transition, save point system

### Option B: New Claude.ai Project from Scratch
1. Create new Claude.ai Project named "PGPulse"
2. Upload to Project Knowledge:
   - This save point file
   - PGAM_FEATURE_AUDIT.md (76 queries — essential reference)
   - PGPulse_Development_Strategy_v2.md (process rules)
   - Chat_Transition_Process.md
   - Save_Point_System.md
3. Open new chat, upload this save point
4. Say: "Restoring PGPulse from save point M1. All context is in this file."

### Option C: Different AI Tool / New Developer
1. Clone: `git clone https://github.com/ios9000/PGPulse_01.git`
2. Read this save point file — it contains complete project context
3. Read `.claude/CLAUDE.md` for module ownership and rules
4. Read `docs/roadmap.md` for current milestone status
5. Key files to understand first:
   - `internal/collector/collector.go` — interfaces
   - `internal/collector/base.go` — shared patterns
   - `internal/collector/registry.go` — orchestration
   - `internal/version/gate.go` — version-adaptive SQL
6. Continue from "What's Next" section above

### Option D: Complete Disaster Recovery
If the repo is lost:
1. This save point contains all interfaces, patterns, and key code snippets
2. Section 2 has the architecture diagram and all decisions
3. Section 4 has the full PGAM query porting status (reference PGAM_FEATURE_AUDIT.md for actual SQL)
4. Section 8 has the implementation patterns to rebuild collectors
5. Rebuild sequence: interfaces → version gate → base → registry → collectors one by one
