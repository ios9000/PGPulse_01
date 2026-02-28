# PGPulse — Save Point

**Save Point:** M2 — Storage & API (complete)
**Date:** 2026-02-27
**Commit:** [update with actual commit hash after push]
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
PGPulse is a real-time PostgreSQL monitoring tool that collects metrics from PG 14–18 instances (connections, cache hit ratio, replication lag, locks, wait events, pg_stat_statements, vacuum progress, bloat), stores them in PostgreSQL (TimescaleDB-ready), serves them via a REST API with JSON and CSV export, and will include alerting and ML-based anomaly detection. It's designed as a single Go binary with an embedded web UI, targeting PostgreSQL DBAs who need a lightweight, PG-specialized alternative to heavyweight platforms like PMM or SolarWinds.

### Origin Story
Rewrite of PGAM — a legacy PHP PostgreSQL Activity Monitor used internally at VTB Bank. PGAM had 76 SQL queries across 2 PHP files (analiz2.php + analiz_db.php), zero authentication, SQL injection vulnerabilities via raw GET params, and relied on COPY TO PROGRAM for OS metrics (requiring superuser). PGPulse is a clean-room rewrite in Go that preserves the SQL monitoring knowledge while fixing every architectural and security flaw.

---

## 2. ARCHITECTURE SNAPSHOT

### Tech Stack
| Component | Choice | Version | Why |
|---|---|---|---|
| Language | Go | 1.24.0 | Upgraded from 1.23.6; pgx v5.8.0 requires ≥ 1.24 |
| PG Driver | jackc/pgx v5 | 5.8.0 | Best Go PG driver, parameterized queries, named args, CopyFrom |
| HTTP Router | go-chi/chi v5 | 5.2.5 | Lightweight, middleware-friendly |
| Storage | PostgreSQL + TimescaleDB | — | PG-native; hypertable creation conditional on config flag |
| Config | koanf v2 | 2.3.2 | YAML + env var overrides; mapstructure for duration parsing |
| Frontend | Svelte + Tailwind | — | Embedded via go:embed (M5) |
| Logging | log/slog | stdlib | Structured logging |
| Testing | testcontainers-go | 0.40.0 | Real PG instances in CI tests |
| ML (Phase 1) | gonum | — | Pure Go statistics (M8) |
| CI | GitHub Actions | — | Lint + test + build |
| Linter | golangci-lint | v2.10.1 | Go 1.24 required v2 config format |

### Architecture Diagram
```
┌──────────────────────────────────────────────────────────────┐
│                    PGPulse Server (Go binary)                 │
│                                                               │
│  ┌─────────────────────────────────────────────────────────┐ │
│  │  HTTP Server (chi v5, :8080)              [M2_03]       │ │
│  │  ┌──────────┐ ┌──────────┐ ┌───────────────────────┐   │ │
│  │  │ /health  │ │/instances│ │/instances/{id}/metrics│   │ │
│  │  └──────────┘ └──────────┘ └───────────────────────┘   │ │
│  │  Middleware: RequestID → Logger → Recoverer → CORS →   │ │
│  │              AuthStub (→ JWT in M3)                      │ │
│  └──────────────────────────────┬──────────────────────────┘ │
│                                  │ Query                      │
│  ┌───────────────────────────────▼─────────────────────────┐ │
│  │  Storage (PGStore | LogStore)                 [M2_02]   │ │
│  │  Write: CopyFrom (10s timeout)                          │ │
│  │  Query: dynamic WHERE + buildQuery() (30s timeout)      │ │
│  │  Migrations: 001_metrics + 002_timescaledb (conditional)│ │
│  └──────────────────────────────▲──────────────────────────┘ │
│                                  │ Write                      │
│  ┌───────────────────────────────┴─────────────────────────┐ │
│  │  Orchestrator                                 [M2_01]   │ │
│  │  ┌──────────────┐  ┌──────────────┐                     │ │
│  │  │instanceRunner│  │instanceRunner│  (per instance)     │ │
│  │  │ ┌──────────┐ │  │ ┌──────────┐ │                     │ │
│  │  │ │high (10s)│ │  │ │med (60s) │ │                     │ │
│  │  │ │ group    │ │  │ │ group    │ │                     │ │
│  │  │ ├──────────┤ │  │ ├──────────┤ │                     │ │
│  │  │ │low(300s) │ │  │ │low(300s) │ │                     │ │
│  │  │ │ group    │ │  │ │ group    │ │                     │ │
│  │  │ └──────────┘ │  │ └──────────┘ │                     │ │
│  │  └──────────────┘  └──────────────┘                     │ │
│  └─────────────────────────────────────────────────────────┘ │
│            ↓                                                  │
│  ┌─────────────────────────────────────────────────────────┐ │
│  │  Collectors (20 files, 23 constructors)        [M1]     │ │
│  │  connections, cache, wait_events, lock_tree,             │ │
│  │  long_txns, replication_*, checkpoint,                   │ │
│  │  statements_*, progress_*, server_info, ...              │ │
│  └───────┬─────────────────────────────────────────────────┘ │
│          ↓                                                    │
│  ┌───────▼─────────┐                                         │
│  │  Version Gate   │                                         │
│  │  PG 14–18 SQL  │                                         │
│  └─────────────────┘                                         │
└──────────────────────────────────────────────────────────────┘
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
| D11 | Registry pattern for collectors | CollectAll() with partial-failure semantics; registered explicitly in runner.go | 2026-02-25 |
| D12 | 5s statement_timeout for live collectors | Via context.WithTimeout in queryContext() helper, not SQL SET | 2026-02-25 |
| D13 | InstanceContext SSoT for per-cycle state | Orchestrator queries pg_is_in_recovery() once per cycle, passes to all collectors | 2026-02-25 |
| D14 | Version in Base, IsRecovery in InstanceContext | Version = structural (immutable). Recovery state = dynamic (changes on failover) | 2026-02-25 |
| D15 | Defer logical replication Q41 | Requires per-DB connections, breaks single-conn Collector interface | 2026-02-25 |
| D16 | golangci-lint v2 config format | v1 doesn't support Go 1.24. Config requires `version: "2"` field | 2026-02-25 |
| D17 | Docker Desktop not available | BIOS virtualization disabled. Integration tests CI-only | 2026-02-25 |
| D18 | Stateful checkpoint collector | Checkpoint/bgwriter counters are cumulative. Need deltas for per-second rates | 2026-02-26 |
| D19 | -1 sentinel for version-unavailable columns | PG 14-16 lacks restartpoints; PG 17 lacks buffers_backend. Use -1 (not 0, not NULL) | 2026-02-26 |
| D20 | completionPct() shared helper | Six progress collectors all need safe division. One helper in progress_vacuum.go | 2026-02-26 |
| D21 | Multiple collectors per file for related operations | Group by similarity: progress_maintenance.go, progress_operations.go | 2026-02-26 |
| D22 | pg_stat_io deferred to M1_03b | PG 16+ only, high cardinality, needs granularity design | 2026-02-26 |
| D23 | pgssAvailable() shared helper in base.go | Both statements collectors need the same EXISTS check | 2026-02-26 |
| D24 | buildMetrics()/buildTopMetrics() extracted as pure methods | Enables unit testing without DB | 2026-02-26 |
| D25 | Top-N unified query (Q50+Q51 combined) | Single CTE ranks by total_exec_time, derives IO and CPU from same scan | 2026-02-26 |
| D26 | "Other" bucket uses unclamped sums | other = totals - actual_sum preserves accuracy | 2026-02-26 |
| D27 | *float64 for nullable stats_reset scan | pgx v5 sets *float64 to nil for NULL. Metric not emitted when nil | 2026-02-26 |
| D28 | Single Claude Code session for focused tasks | Scope small enough; Agent Teams overhead not justified | 2026-02-26 |
| D29 | pg_blocking_pids() over recursive pg_locks CTE | Simpler, PG-native since 9.6 | 2026-02-26 |
| D30 | Lock graph traversal in Go (BFS) not SQL | Pure function = directly unit-testable without DB | 2026-02-26 |
| D31 | Deadlock: BlockerCount=0, BlockedCount>0 | All deadlock participants in both sets → no roots. Signals deadlock correctly | 2026-02-26 |
| D32 | LongTransactions always emits 4 points | Zero-fill missing types: consistent schema regardless of activity | 2026-02-26 |
| D33 | Q56+Q57 merged into single parameterized query | CASE WHEN eliminates redundant round-trip | 2026-02-26 |
| D34 | Q58 (lock details) deferred | Q53–Q57 provide operational monitoring. Per-lock-detail is analytic | 2026-02-26 |
| D35 | koanf v2 for config loading | YAML + env var override pattern; mapstructure handles time.Duration | 2026-02-26 |
| D36 | DSN-based instance config | Single connection string per instance (pgx.ParseConfig) | 2026-02-26 |
| D37 | *bool for Enabled field | Pointer distinguishes "not set" (nil → default true) from "explicitly false" | 2026-02-26 |
| D38 | Orchestrator stores cancel func, not context | Avoids leaking ctx across method boundary | 2026-02-26 |
| D39 | icFunc injectable in intervalGroup | Enables unit testing collect() without real DB | 2026-02-26 |
| D40 | .gitignore binary entries root-anchored | `/pgpulse-server` not `pgpulse-server` | 2026-02-26 |
| D41 | LogStore stub for dev mode | Write() logs at debug level; Query() returns nil | 2026-02-26 |
| D42 | CopyFrom for storage writes | Batch insert via COPY protocol. 10s write timeout | 2026-02-27 |
| D43 | buildQuery() pure function | Dynamic WHERE construction. Testable without DB | 2026-02-27 |
| D44 | pgxpool MaxConns=5 for storage | application_name="pgpulse_storage". For PGPulse's own DB only | 2026-02-27 |
| D45 | Conditional TimescaleDB migrations | 002_timescaledb.sql skipped when use_timescaledb=false | 2026-02-27 |
| D46 | Embedded SQL migrations via go:embed | Simple runner with schema_migrations table. No external framework | 2026-02-27 |
| D47 | JSON envelope for API responses | `{"data":..., "meta":...}` for success; `{"error":{"code","message"}}` for errors | 2026-02-27 |
| D48 | Pinger interface for health checks | Abstracts pgxpool.Pool.Ping() for testability | 2026-02-27 |
| D49 | Auth stub middleware | Sets user="anonymous" in context. M3 replaces middleware only, handlers unchanged | 2026-02-27 |
| D50 | Graceful shutdown order | HTTP server → Orchestrator → Store. Drain requests before stopping collection | 2026-02-27 |
| D51 | CSV export via Accept header or query param | ?format=csv OR Accept: text/csv. Labels serialized as JSON in CSV | 2026-02-27 |
| D52 | Full paths in design docs | Never use "..." or abbreviations in file paths | 2026-02-27 |

---

## 3. CODEBASE STATE

### File Tree (after M2_03)
```
.claude/CLAUDE.md
.claude/rules/code-style.md
.claude/rules/architecture.md
.claude/rules/security.md
.claude/rules/postgresql.md
.github/workflows/ci.yml
.gitignore
.golangci.yml
Makefile
README.md
go.mod
go.sum

cmd/pgpulse-server/main.go        ← Config → Storage → Orchestrator → HTTP server → graceful shutdown
cmd/pgpulse-agent/main.go         ← OS agent placeholder

configs/pgpulse.example.yml

internal/api/                      ← [NEW M2_03]
  server.go                        APIServer struct, Pinger interface, New(), Routes()
  response.go                      Envelope, ErrorResponse, writeJSON, writeError
  middleware.go                    requestID, logger, recoverer, CORS, authStub, UserFromContext
  health.go                        GET /api/v1/health
  instances.go                     GET /api/v1/instances, GET /api/v1/instances/{id}
  metrics.go                       GET /api/v1/instances/{id}/metrics (JSON + CSV)
  helpers_test.go                  mockStore, mockPinger, newTestServer
  health_test.go                   5 tests
  instances_test.go                5 tests
  metrics_test.go                  10 tests
  middleware_test.go               4 tests

internal/collector/
  collector.go                     Interfaces: MetricPoint, InstanceContext, Collector, MetricStore, MetricQuery
  base.go                          Base struct, point(), queryContext(), pgssAvailable()
  registry.go / registry_test.go
  server_info.go / _test.go        Q2,Q3,Q9,Q10
  connections.go / _test.go        Q11-Q13
  cache.go / _test.go              Q14
  transactions.go / _test.go       Q15
  database_sizes.go / _test.go     Q16
  settings.go / _test.go           Q17
  extensions.go / _test.go         Q18-Q19
  replication_status.go / _test.go Q20,Q21
  replication_lag.go / _test.go    Q37,Q38
  replication_slots.go / _test.go  Q40
  progress_vacuum.go / _test.go    Q42
  progress_maintenance.go / _test.go Q43,Q45
  progress_operations.go / _test.go Q44,Q46,Q47
  checkpoint.go / _test.go         Stateful, version-gated PG≤16/≥17
  io_stats.go / _test.go           pg_stat_io, PG 16+
  statements_config.go / _test.go  Q48,Q49
  statements_top.go / _test.go     Q50,Q51
  wait_events.go / _test.go        Q53/Q54
  lock_tree.go / _test.go          Q55 — blocking tree, BFS graph
  long_transactions.go / _test.go  Q56/Q57 merged
  testutil_test.go                 Integration test helpers (//go:build integration)

internal/config/                   ← [NEW M2_01, UPDATED M2_03]
  config.go                        Config, ServerConfig (timeouts, CORS), StorageConfig, InstanceConfig (Description)
  load.go                          Load(), transformEnvKey(), validate() with defaults
  config_test.go                   7 tests

internal/orchestrator/             ← [NEW M2_01]
  orchestrator.go                  New(), Start(), Stop()
  runner.go                        instanceRunner: connect, buildCollectors, start, close
  group.go                         intervalGroup: run(), collect(), queryInstanceContext(), icFunc injection
  logstore.go                      LogStore stub (Write logs, Query returns nil)
  group_test.go                    4 tests
  logstore_test.go                 4 tests
  orchestrator_test.go             1 test

internal/storage/                  ← [NEW M2_02, UPDATED M2_03]
  migrations/
    001_metrics.sql                Metrics table + 3 indexes
    002_timescaledb.sql            Conditional hypertable
  migrate.go                       Migrate(), go:embed, schema_migrations bootstrap
  pgstore.go                       PGStore: Write (CopyFrom), Query (dynamic WHERE), Close, Pool()
  pool.go                          NewPool() (pgxpool, 5 max conns)
  migrate_test.go                  5 tests
  pgstore_test.go                  9 tests
  pool_test.go                     1 test

internal/version/
  gate.go                          Gate, SQLVariant, VersionRange, Select()
  version.go                       PGVersion, Detect(), AtLeast()

docs/iterations/
  M1_01 through M1_05 folders
  M2_01_02272026_config-orchestrator/
  M2_02_02272026_storage-layer/
  M2_03_02272026_rest-api/
  HANDOFF_M2_03_to_M3_01.md

docs/save-points/
  SAVEPOINT_M0_20260225.md
  SAVEPOINT_M1_20260225.md
  SAVEPOINT_M1_20260226.md
  SAVEPOINT_M1_20260226b.md
  SAVEPOINT_M1_20260226c.md
  SAVEPOINT_M2_20260226.md
  SAVEPOINT_M2_20260227.md         ← THIS FILE
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
// internal/api/server.go  [NEW M2_03]

type Pinger interface {
    Ping(ctx context.Context) error
}

type APIServer struct {
    store      collector.MetricStore
    instances  []config.InstanceConfig
    serverCfg  config.ServerConfig
    logger     *slog.Logger
    startTime  time.Time
    pool       Pinger // nil when using LogStore
}

func New(cfg config.Config, store collector.MetricStore, pool Pinger, logger *slog.Logger) *APIServer
func (s *APIServer) Routes() http.Handler
```

```go
// internal/config/config.go  [UPDATED M2_03]

type Config struct {
    Server    ServerConfig     `koanf:"server"`
    Storage   StorageConfig    `koanf:"storage"`
    Instances []InstanceConfig `koanf:"instances"`
}

type ServerConfig struct {
    Address         string        `koanf:"address"`          // default ":8080"
    CORSEnabled     bool          `koanf:"cors_enabled"`     // default false
    ReadTimeout     time.Duration `koanf:"read_timeout"`     // default 30s
    WriteTimeout    time.Duration `koanf:"write_timeout"`    // default 60s
    ShutdownTimeout time.Duration `koanf:"shutdown_timeout"` // default 10s
}

type StorageConfig struct {
    DSN            string `koanf:"dsn"`
    UseTimescaleDB bool   `koanf:"use_timescaledb"`
    RetentionDays  int    `koanf:"retention_days"` // default 30
}

type InstanceConfig struct {
    ID                string   `koanf:"id"`
    DSN               string   `koanf:"dsn"`
    Enabled           *bool    `koanf:"enabled"`     // nil → default true
    Description       string   `koanf:"description"` // NEW M2_03
    CollectorsEnabled []string `koanf:"collectors_enabled"`
}
```

```go
// internal/storage/pgstore.go  [NEW M2_02, UPDATED M2_03]

type PGStore struct { pool *pgxpool.Pool; logger *slog.Logger }

func NewPGStore(pool *pgxpool.Pool, logger *slog.Logger) *PGStore
func (s *PGStore) Write(ctx context.Context, points []collector.MetricPoint) error  // CopyFrom, 10s timeout
func (s *PGStore) Query(ctx context.Context, q collector.MetricQuery) ([]collector.MetricPoint, error)  // dynamic WHERE, 30s timeout
func (s *PGStore) Close() error
func (s *PGStore) Pool() *pgxpool.Pool  // NEW M2_03: for health checks

func buildQuery(q collector.MetricQuery) (string, []any)  // pure function, testable

// internal/storage/migrate.go
type MigrateOptions struct { UseTimescaleDB bool }
func Migrate(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger, opts MigrateOptions) error

// internal/storage/pool.go
func NewPool(ctx context.Context, dsn string) (*pgxpool.Pool, error)  // MaxConns=5
```

```go
// internal/orchestrator/orchestrator.go

func New(cfg config.Config, store collector.MetricStore, logger *slog.Logger) *Orchestrator
func (o *Orchestrator) Start(ctx context.Context) error  // error if 0 instances connect
func (o *Orchestrator) Stop()                            // cancel + wg.Wait + close conns
```

```go
// internal/version/gate.go

type VersionRange struct { MinMajor, MinMinor, MaxMajor, MaxMinor int }
type SQLVariant struct { Range VersionRange; SQL string }
type Gate struct { Name string; Variants []SQLVariant }
func (g Gate) Select(v PGVersion) (string, bool)
```

### REST API Endpoints

| Method | Path | Purpose | Auth |
|--------|------|---------|------|
| GET | `/api/v1/health` | Liveness + storage ping + uptime + version | None (public) |
| GET | `/api/v1/instances` | List instances from config | Stub (anonymous) |
| GET | `/api/v1/instances/{id}` | Single instance detail | Stub (anonymous) |
| GET | `/api/v1/instances/{id}/metrics` | Query stored metrics (?metric, ?start, ?end, ?limit, ?format=json|csv) | Stub (anonymous) |

### Dependencies (go.mod key entries)
```
go 1.24.0
github.com/jackc/pgx/v5 v5.8.0
github.com/go-chi/chi/v5 v5.2.5
github.com/knadh/koanf/v2 v2.3.2
github.com/knadh/koanf/parsers/yaml v1.1.0
github.com/knadh/koanf/providers/file v1.2.1
github.com/knadh/koanf/providers/env v1.1.0
github.com/go-viper/mapstructure/v2 v2.4.0
```

### main.go Flow (after M2_03)

```
main.go
  → config.Load(path) → Config
  → if cfg.Storage.DSN != "":
      storage.NewPool(ctx, dsn) → pool
      storage.Migrate(ctx, pool, logger, MigrateOptions{...}) → schema ready
      storage.NewPGStore(pool, logger) → store
    else:
      orchestrator.NewLogStore(logger) → store
  → api.New(cfg, store, pool, logger) → apiServer
  → &http.Server{Addr, Handler: apiServer.Routes(), ReadTimeout, WriteTimeout}
  → go httpServer.ListenAndServe()       ← HTTP in background goroutine
  → orchestrator.New(cfg, store, logger) → orch
  → orch.Start(ctx)
  → signal.Notify(SIGINT, SIGTERM)
  → httpServer.Shutdown(shutdownCtx)     ← Drain HTTP first
  → orch.Stop()                          ← Stop collectors second
  → store.Close()                        ← Close pool last
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
| M1 | Core Collectors | ✅ Done | 2026-02-26 |
| M2 | Storage & API | ✅ Done | 2026-02-27 |
| M3 | Auth & Security | 🔲 Next | — |
| M4 | Alerting | 🔲 Not started | — |
| M5 | Web UI (MVP) | 🔲 Not started | — |
| M6 | Agent Mode | 🔲 Not started | — |
| M7–M10 | P1 Features / ML / Reports / Polish | 🔲 Not started | — |

### M2 Sub-Iteration History

| Sub-Iteration | Scope | Status |
|---|---|---|
| M2_01 | Config loader + Orchestrator + LogStore stub | ✅ Done |
| M2_02 | PGStore (CopyFrom + dynamic Query) + Migrations (001_metrics + 002_timescaledb) | ✅ Done |
| M2_03 | REST API (chi v5, health/instances/metrics, JSON+CSV, middleware stack) | ✅ Done |

### What Was Just Completed (M2 — full milestone)

**M2_01 — Config & Orchestrator:**
- Config loader (koanf v2, YAML + env overrides, validate with defaults)
- Orchestrator (goroutine per interval group, InstanceContext SSoT, 23 collectors wired)
- LogStore fallback for dev mode
- main.go with signal handling and graceful shutdown

**M2_02 — Storage Layer:**
- PGStore with CopyFrom writes (10s timeout) and dynamic WHERE queries (30s timeout)
- Migration runner with go:embed SQL files and schema_migrations bootstrap table
- 001_metrics.sql (metrics table + 3 indexes) and 002_timescaledb.sql (conditional hypertable)
- pgxpool with MaxConns=5, application_name="pgpulse_storage"
- buildQuery() pure function for testable query construction

**M2_03 — REST API:**
- chi v5 router with 4 endpoints (health, list instances, get instance, query metrics)
- Middleware stack: RequestID, Logger, Recoverer, CORS (configurable), AuthStub
- JSON envelope responses + CSV export via ?format=csv or Accept: text/csv
- Pinger interface for testable health checks (abstracts pgxpool.Pool.Ping)
- Graceful shutdown: HTTP → Orchestrator → Store
- 24 API unit tests

### What's Next

**M3_01 — Authentication & RBAC:**
- JWT token generation and validation
- bcrypt password hashing
- RBAC: admin (full access) and viewer (read-only)
- Auth middleware replacing current authStub
- POST /api/v1/auth/login, POST /api/v1/auth/refresh, GET /api/v1/auth/me
- users table migration (003_auth.sql)
- Rate limiting on auth endpoints

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
- **Single Claude Code session** (Sonnet 4.6) — no Agent Teams overhead for focused tasks
- **Hybrid workflow:** Claude Code creates files; developer runs go build/test/commit manually
- **One chat per iteration** in Claude.ai for planning; Claude Code for implementation

### Known Environment Issues

| Issue | Status | Workaround |
|---|---|---|
| Claude Code bash EINVAL in Agent Teams subprocess mode | Unresolved | Use direct single-session Claude Code |
| LF/CRLF warnings on git add | Cosmetic | `.gitattributes` with `* text=auto eol=lf` |
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

### Storage Pattern
```go
// Write via CopyFrom (batch insert)
store.Write(ctx, points) // 10s timeout, COPY protocol

// Query via dynamic WHERE
store.Query(ctx, MetricQuery{InstanceID: "x", Metric: "pgpulse.connections.", Start: t1, End: t2, Limit: 100})
// buildQuery() constructs: SELECT ... FROM metrics WHERE instance_id=$1 AND metric LIKE $2||'%' AND ...
```

### API Pattern
```go
// All handlers are methods on *APIServer — testable with httptest
s := api.New(cfg, mockStore, mockPinger, logger)
router := s.Routes()
rec := httptest.NewRecorder()
router.ServeHTTP(rec, req)
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
3. Say: "Restoring from save point. M2 is complete. Next is M3_01 (Auth & RBAC)."

### Option B: New Claude.ai Project / Different Tool
1. Clone: `git clone https://github.com/ios9000/PGPulse_01.git`
2. Read this file for complete context
3. Key interfaces: `internal/collector/collector.go`, `internal/api/server.go`, `internal/config/config.go`, `internal/storage/pgstore.go`
4. `go test ./...` — all tests pass

### Option C: Complete Disaster Recovery
If the repo is lost:
1. This save point contains all interfaces, patterns, and architectural decisions
2. PGAM SQL queries are in PGAM_FEATURE_AUDIT.md (76 queries documented)
3. Rebuild order: version/ → collector/ → config/ → orchestrator/ → storage/ → api/
4. All design decisions documented in section 2 (D1–D52)
