# PGPulse — Save Point

**Save Point:** M2 (in progress) — M2_01 Config & Orchestrator
**Date:** 2026-02-26
**Commit:** 98b06f1
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
| HTTP Router | go-chi/chi v5 | — | Lightweight, middleware-friendly (M2_02+) |
| Storage | PostgreSQL + TimescaleDB | — | PG-native time-series hypertables (M2_02+) |
| Config | koanf v2 | 2.3.2 | YAML + env var overrides; mapstructure for duration parsing |
| Frontend | Svelte + Tailwind | — | Embedded via go:embed (M5) |
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
│  ┌──────────────────────────────────────────────┐   │
│  │  Orchestrator (NEW M2_01)                     │   │
│  │  ┌─────────────┐  ┌────────────────────────┐ │   │
│  │  │instanceRunner│  │instanceRunner          │ │   │
│  │  │ ┌─────────┐ │  │ ┌──────┐ ┌──────────┐ │ │   │
│  │  │ │high(10s)│ │  │ │med   │ │low(300s) │ │ │   │
│  │  │ │group    │ │  │ │(60s) │ │group     │ │ │   │
│  │  │ └────┬────┘ │  │ └──┬───┘ └────┬─────┘ │ │   │
│  │  └──────┼──────┘  └────┼──────────┼───────┘ │   │
│  └─────────┼──────────────┼──────────┼──────────┘   │
│            ↓              ↓          ↓               │
│  ┌────────────────────────────────────────────────┐  │
│  │  Collectors (20 files, 23 constructors)         │  │
│  │  connections, cache, wait_events, lock_tree,    │  │
│  │  long_txns, replication_*, checkpoint,          │  │
│  │  statements_*, progress_*, server_info, ...     │  │
│  └───────┬─────────────────────────────────────────┘  │
│          ↓                                            │
│  ┌───────▼─────────┐   ┌──────────┐  ┌──────────┐   │
│  │  Version Gate   │   │ Storage  │← │ REST API │   │
│  │  PG 14–18 SQL  │   │ (TSDB)   │  │ (chi+JWT)│   │
│  └─────────────────┘   │ LogStore │  └──────────┘   │
│                         │ (M2_01)  │                  │
│                         └──────────┘                  │
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
| D35 | koanf v2 for config loading | YAML + env var override pattern; mapstructure v2 handles time.Duration from strings ("10s"). | 2026-02-26 |
| D36 | DSN-based instance config | Single connection string per instance (pgx.ParseConfig). Cleaner than host/port/user/pass fields. | 2026-02-26 |
| D37 | *bool for Enabled field in InstanceConfig | Pointer distinguishes "not set" (nil → default true) from "explicitly false". | 2026-02-26 |
| D38 | Orchestrator stores cancel func, not context | ctx passed to Start(); cancel stored for Stop(). Avoids leaking ctx across method boundary. | 2026-02-26 |
| D39 | icFunc injectable in intervalGroup | field `icFunc icQueryFunc` defaults to queryInstanceContext; replaced in tests with staticICFunc. Enables unit testing collect() without real DB. | 2026-02-26 |
| D40 | .gitignore binary entries root-anchored | `/pgpulse-server` not `pgpulse-server` — plain name matched cmd/pgpulse-server/ directory, preventing cmd/ from being tracked. | 2026-02-26 |
| D41 | LogStore stub for dev/M2_01 | Write() logs at debug level; Query() returns nil. Real TimescaleDB store comes in M2_02+. | 2026-02-26 |

---

## 3. CODEBASE STATE

### File Tree (after M2_01)
```
.claude/CLAUDE.md
.claude/rules/...
.github/workflows/ci.yml
.gitignore / .golangci.yml / Makefile / README.md
cmd/pgpulse-agent/main.go
cmd/pgpulse-server/main.go        ← UPDATED M2_01: real main (config+orchestrator+signals)
configs/pgpulse.example.yml       ← UPDATED M2_01: DSN-based, interval tiers
deploy/...
docs/iterations/...
docs/roadmap.md
docs/save-points/LATEST.md
docs/save-points/SAVEPOINT_M0_20260225.md
docs/save-points/SAVEPOINT_M1_20260225.md
docs/save-points/SAVEPOINT_M1_20260226.md
docs/save-points/SAVEPOINT_M1_20260226b.md
docs/save-points/SAVEPOINT_M1_20260226c.md
docs/save-points/SAVEPOINT_M2_20260226.md  ← THIS FILE
go.mod / go.sum
internal/collector/
  base.go                      pgssAvailable(), point(), queryContext(), Base struct
  cache.go / cache_test.go
  checkpoint.go / _test.go     Stateful, version-gated PG≤16/≥17
  collector.go                 Interfaces: MetricPoint, InstanceContext, Collector, MetricStore, MetricQuery
  connections.go / _test.go
  database_sizes.go / _test.go
  extensions.go / _test.go
  io_stats.go / _test.go       pg_stat_io, PG 16+
  lock_tree.go / _test.go      Q55 — blocking tree, BFS graph
  long_transactions.go / _test.go  Q56/Q57 merged
  progress_maintenance.go / _test.go
  progress_operations.go / _test.go
  progress_vacuum.go / _test.go
  registry.go / _test.go
  replication_lag.go / _test.go
  replication_slots.go / _test.go
  replication_status.go / _test.go
  server_info.go / _test.go
  settings.go / _test.go
  statements_config.go / _test.go
  statements_top.go / _test.go
  testutil_test.go             Integration test helpers (//go:build integration)
  transactions.go / _test.go
  wait_events.go / _test.go    Q53/Q54
internal/config/               ← NEW M2_01
  config.go                    Config, ServerConfig, StorageConfig, InstanceConfig, IntervalConfig
  config_test.go               7 unit tests
  load.go                      Load(), transformEnvKey(), validate()
internal/orchestrator/         ← NEW M2_01
  group.go                     intervalGroup, run(), collect(), queryInstanceContext(), icFunc injection
  group_test.go                4 unit tests (mockCollector, mockStore, staticICFunc)
  logstore.go                  LogStore stub (Write logs, Query returns nil)
  logstore_test.go             4 unit tests
  orchestrator.go              Orchestrator, New(), Start(), Stop()
  orchestrator_test.go         1 unit test (TestNew)
  runner.go                    instanceRunner, connect(), buildCollectors(), start(), close()
internal/version/
  gate.go                      Gate, SQLVariant, VersionRange, Select()
  version.go                   PGVersion, Detect(), AtLeast()
internal/alert|api|auth|ml|rca|storage/  (.gitkeep — future milestones)
migrations/.gitkeep / web/.gitkeep
```

### Key Interfaces

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

type MetricQuery struct {
    InstanceID string
    Metric     string            // optional: filter by metric name prefix
    Labels     map[string]string // optional: filter by label values
    Start      time.Time         // time range start
    End        time.Time         // time range end
    Limit      int               // max results (0 = no limit)
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
```

```go
// internal/config/config.go

type Config struct {
    Server    ServerConfig     `koanf:"server"`
    Storage   StorageConfig    `koanf:"storage"`
    Instances []InstanceConfig `koanf:"instances"`
}

type InstanceConfig struct {
    ID        string         `koanf:"id"`
    DSN       string         `koanf:"dsn"`
    Enabled   *bool          `koanf:"enabled"` // nil → default true
    Intervals IntervalConfig `koanf:"intervals"`
}

type IntervalConfig struct {
    High   time.Duration `koanf:"high"`   // default 10s
    Medium time.Duration `koanf:"medium"` // default 60s
    Low    time.Duration `koanf:"low"`    // default 300s
}

// internal/config/load.go
func Load(path string) (Config, error)
// PGPULSE_SERVER_LISTEN=:9090 → server.listen=":9090"
```

```go
// internal/orchestrator/orchestrator.go

func New(cfg config.Config, store collector.MetricStore, logger *slog.Logger) *Orchestrator
func (o *Orchestrator) Start(ctx context.Context) error  // error if 0 instances connect
func (o *Orchestrator) Stop()                            // cancel + wg.Wait + close conns
```

```go
// internal/orchestrator/group.go

type icQueryFunc func(ctx context.Context, conn *pgx.Conn) (collector.InstanceContext, error)

// intervalGroup has icFunc field — swap for staticICFunc in tests:
//   g.icFunc = func(ctx, conn) (InstanceContext, error) { return InstanceContext{}, nil }
```

```go
// internal/version/gate.go

type VersionRange struct { MinMajor, MinMinor, MaxMajor, MaxMinor int }
type SQLVariant struct { Range VersionRange; SQL string }
type Gate struct { Name string; Variants []SQLVariant }
func (g Gate) Select(v PGVersion) (string, bool)
```

### Dependencies (key additions in M2_01)
```
github.com/knadh/koanf/v2 v2.3.2
github.com/knadh/koanf/parsers/yaml v1.1.0
github.com/knadh/koanf/providers/file v1.2.1
github.com/knadh/koanf/providers/env v1.1.0
github.com/go-viper/mapstructure/v2 v2.4.0  (koanf transitive dep)
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
| analiz2.php Q52 | Normalized query stats | — | ⏭️ Deferred |
| analiz2.php Q53–Q54 | Wait event summary | wait_events.go | ✅ Done (M1_05) |
| analiz2.php Q55 | Lock blocking tree | lock_tree.go | ✅ Done (M1_05) |
| analiz2.php Q56–Q57 | Long transactions | long_transactions.go | ✅ Done (M1_05) |
| analiz2.php Q58 | Lock details (per-lock) | — | ⏭️ Deferred |
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
| M1 | Core Collector | ✅ Done | 2026-02-26 |
| M2 | Storage & API | 🔶 In progress (M2_01 done) | — |
| M3 | Auth & Security | 🔲 Not started | — |
| M4 | Alerting | 🔲 Not started | — |
| M5 | Web UI (MVP) | 🔲 Not started | — |
| M6 | Agent Mode | 🔲 Not started | — |
| M7–M10 | P1 Features / ML / Reports / Polish | 🔲 Not started | — |

### M2 Sub-Iteration Status

| Sub-Iteration | Scope | Status |
|---|---|---|
| M2_01 | Config loader + Orchestrator + LogStore stub | ✅ Done (31a696b) |
| M2_02 | TimescaleDB storage (MetricStore implementation) | 🔲 Not started |
| M2_03 | REST API (chi router, metric query endpoints) | 🔲 Not started |

### What Was Just Completed (M2_01)

**M2_01 — Config & Orchestrator. Commit 31a696b. 12 files created/modified, 1046 insertions.**

**`internal/config` (config.go + load.go):**
- `Config` struct with `ServerConfig`, `StorageConfig`, `[]InstanceConfig`
- `InstanceConfig` has `*bool Enabled` (pointer = nil means "not set" → default true)
- `IntervalConfig` with High/Medium/Low `time.Duration` fields (koanf parses "10s" → 10s)
- `Load(path)` — koanf v2: file.Provider(YAML) + env.Provider("PGPULSE_") with transform
- `validate()` — applies all defaults, returns descriptive errors

**`internal/orchestrator` (5 production files):**
- `Orchestrator` — `New()`, `Start()` (connect + build + start all instances), `Stop()` (cancel + wait + close)
- `instanceRunner` — `connect()` (pgx.ParseConfig + 5s timeout + application_name), `buildCollectors()` (23 constructors across 3 interval groups), `start()`, `close()`
- `intervalGroup` — `run()` (immediate first tick + ticker loop), `collect()` (queryInstanceContext → collectors → batch Write); `icFunc icQueryFunc` field injectable for testing
- `LogStore` — MetricStore stub: Write() logs at debug, Query() returns nil; production placeholder until M2_02

**`cmd/pgpulse-server/main.go` — real implementation:**
- `-config` flag (default `pgpulse.yml`)
- Bootstrap logger → load config → reconfigure logger at configured level
- `signal.NotifyContext` for SIGINT/SIGTERM
- Orchestrator Start → block on ctx.Done → Stop → store.Close
- 10s graceful shutdown timeout context (HTTP server will use it in M2_03)

**Tests (16 new, all pass):**
- config: 7 tests (valid, defaults, missing file, invalid YAML, no instances, empty DSN, enabled=false)
- group: 4 tests (all success, partial failure, all fail, nil points) — no DB required via icFunc injection
- logstore: 4 tests (Write, Write_Empty, Query, Close)
- orchestrator: 1 test (TestNew — construction, no Start)

**Side fix:** `.gitignore` — `pgpulse-server` → `/pgpulse-server` (root-anchored). Plain name matched `cmd/pgpulse-server/` directory, silently preventing cmd/ from being tracked.

### What's Next

**M2_02** — TimescaleDB MetricStore implementation:
- `internal/storage/` — `TimescaleStore` implementing `MetricStore`
- Migrations: create metrics hypertable, retention policy
- Integration test with testcontainers-go (PostgreSQL + TimescaleDB)
- Wire `TimescaleStore` into `cmd/pgpulse-server/main.go` behind a flag or storage.dsn check

**M2_03** — REST API:
- `internal/api/` — chi router, `/metrics` query endpoint, `/health` endpoint
- Serve on `cfg.Server.Listen`

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
- **Single Claude Code session** (Sonnet 4.6) — no Agent Teams overhead
- **Hybrid workflow:** Claude Code creates files and runs bash directly
- **One chat per iteration** in Claude.ai for planning; Claude Code for implementation

### Known Environment Issues

| Issue | Status | Workaround |
|---|---|---|
| Claude Code bash EINVAL in Agent Teams subprocess mode | Unresolved | Use direct single-session Claude Code |
| LF/CRLF warnings on git add | Cosmetic | Add `.gitattributes` with `* text=auto eol=lf` |
| Docker Desktop unavailable | BIOS virtualization disabled | Integration tests CI-only |

---

## 7. IMPLEMENTATION PATTERNS (COMPLETE)

### Collector Pattern
```go
// Stateless collector (standard)
type XxxCollector struct { Base }
func NewXxxCollector(instanceID string, v version.PGVersion) *XxxCollector
func (c *XxxCollector) Collect(ctx, conn, ic) ([]MetricPoint, error) {
    qCtx, cancel := queryContext(ctx); defer cancel()
    // scan → buildMetrics(scanned)
}
func (c *XxxCollector) buildMetrics(rows []xxxRow) []MetricPoint { /* pure */ }

// Version-gated SQL
var myGate = version.Gate{Variants: []version.SQLVariant{
    {Range: version.VersionRange{MinMajor:14, MaxMajor:16}, SQL: `...`},
    {Range: version.VersionRange{MinMajor:17, MaxMajor:99}, SQL: `...`},
}}
```

### Orchestrator Pattern
```go
// All 23 collector constructors grouped in runner.go:buildCollectors()
high   (10s):  connections, cache, wait_events, lock_tree, long_transactions
medium (60s):  replication_*, statements_*, checkpoint, vacuum/cluster/analyze/index/basebackup/copy progress
low    (300s): server_info, database_sizes, settings, extensions, transactions, io_stats
```

### Config Pattern
```go
// Load config with env overrides
cfg, err := config.Load("pgpulse.yml")
// PGPULSE_SERVER_LISTEN=:9090 overrides server.listen
// PGPULSE_SERVER_LOG_LEVEL=debug overrides server.log_level
```

### Test Pattern (no-DB orchestrator tests)
```go
// Replace icFunc to avoid real DB in group tests
g := newIntervalGroup("test", 10*time.Second, collectors, nil, store, logger)
g.icFunc = func(_ context.Context, _ *pgx.Conn) (collector.InstanceContext, error) {
    return collector.InstanceContext{IsRecovery: false}, nil
}
g.collect(ctx) // testable without a database
```

### Metric Naming (complete list)
```
pgpulse.server.*             — uptime, is_in_recovery, is_in_backup, pg_version_num
pgpulse.connections.*        — active, idle, idle_in_transaction, waiting, total, max, utilization_ratio
pgpulse.cache.hit_ratio
pgpulse.transactions.*       — commit_ratio, deadlocks (label: db_name)
pgpulse.database.*           — size_bytes (label: db_name)
pgpulse.extensions.*         — pgss_installed, pgss_fill_pct, pgss_stats_reset_unix
pgpulse.replication.*        — lag.*, slot.*, active_replicas, replica.*, wal_receiver.*
pgpulse.progress.*           — vacuum.*, cluster.*, analyze.*, create_index.*, basebackup.*, copy.*
pgpulse.checkpoint.*         — timed, requested, write/sync_time_ms, buffers_written, restartpoints_*
pgpulse.bgwriter.*           — buffers_clean, maxwritten_clean, buffers_alloc, buffers_backend*
pgpulse.io.*                 — reads, read_time, writes, write_time, extends, hits, evictions, fsyncs*
pgpulse.statements.*         — max, fill_pct, track, track_io_timing, count, stats_reset_age_seconds
pgpulse.statements.top.*     — total_time_ms, io_time_ms, cpu_time_ms, calls, rows, avg_time_ms
pgpulse.wait_events.*        — count (labels: wait_event_type, wait_event), total_backends
pgpulse.locks.*              — blocker_count, blocked_count, max_chain_depth
pgpulse.long_transactions.*  — count, oldest_seconds (label: type=active|waiting)
```

---

## 8. HOW TO RESTORE THIS SAVE POINT

### Option A: Continue in Same Claude.ai Project
1. Open new chat in the PGPulse project
2. Upload this save point file
3. Say: "Restoring from save point. M2_01 (config+orchestrator) is done. Next is M2_02 (TimescaleDB MetricStore)."

### Option B: New Claude.ai Project / Different Tool
1. Clone: `git clone https://github.com/ios9000/PGPulse_01.git` (commit 98b06f1)
2. Read this file for complete context
3. Key interfaces: `internal/collector/collector.go`, `internal/config/config.go`, `internal/orchestrator/orchestrator.go`
4. `go test ./internal/config/ ./internal/orchestrator/` — 16 tests, all pass
