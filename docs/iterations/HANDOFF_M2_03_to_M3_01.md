# PGPulse — Iteration Handoff: M2_03 → M3_01

> **Purpose:** Upload this file when starting the next Claude.ai chat.
> Contains EVERYTHING needed to begin M3_01 without re-discovery.
> **Created:** 2026-02-27 (end of M2_03)

---

## DO NOT RE-DISCUSS

These decisions are final. The new chat must not revisit them:

1. **Stack**: Go 1.24.0, pgx v5.8.0, chi v5.2.5, koanf, slog, testcontainers-go
2. **Architecture**: Single binary, version-adaptive SQL via Gate pattern, Collector interface with InstanceContext
3. **Granularity**: One file = one collector = one struct implementing Collector
4. **Module ownership**: Collector Agent owns internal/collector/* and internal/version/*
5. **Agent Teams bash bug**: Claude Code cannot run bash on Windows. Agents create files only. Developer runs go build/test/commit manually.
6. **Go module path**: `github.com/ios9000/PGPulse_01`
7. **Project path**: `C:\Users\Archer\Projects\PGPulse_01`
8. **PG version support**: 14, 15, 16, 17 (18 optional)
9. **Monitoring user**: pg_monitor role, never superuser
10. **OS metrics**: via Go agent (M6), NEVER via COPY TO PROGRAM
11. **Metric naming**: `pgpulse.<category>.<metric>` with labels as map[string]string
12. **Statement timeout**: 5s for live dashboard collectors via context.WithTimeout
13. **golangci-lint**: v2.10.1. Config requires `version: "2"` field.
14. **Docker Desktop**: not available on developer workstation. Integration tests run in CI only.
15. **InstanceContext SSoT**: Orchestrator queries `pg_is_in_recovery()` once per cycle, passes `InstanceContext{IsRecovery: bool}` to all collectors.
16. **Collector registration**: Explicit in runner.go buildCollectors(), NOT via init() auto-registration.
17. **Orchestrator architecture**: Goroutine per interval group (high=10s, medium=60s, low=300s). Collectors within a group run sequentially sharing one pgx.Conn.
18. **Server inventory**: YAML config only for M2. Database-backed inventory deferred to M3+.
19. **Storage approach**: Plain PG first, TimescaleDB-ready. Schema works on plain PG; hypertable creation is conditional.
20. **Migration runner**: Embedded SQL files via go:embed + simple runner. No external migration framework.
21. **Connection resilience**: Skip cycle on error, no auto-reconnect.
22. **LogStore fallback**: When storage.dsn is empty, use LogStore (log-only mode). PGStore when DSN configured.
23. **Claude Code**: v2.1.59. Single Sonnet session for focused tasks; Agent Teams for multi-file iterations.
24. **pgss availability check**: `pgssAvailable()` helper in base.go.
25. **Checkpoint version gate**: PG ≤ 16 → pg_stat_bgwriter. PG ≥ 17 → pg_stat_checkpointer CROSS JOIN pg_stat_bgwriter.
26. **Lock tree**: pg_blocking_pids() + Go BFS graph. Summary metrics only; full tree deferred to API.
27. **Storage: CopyFrom for writes**: Batch insert via COPY protocol. 10s write timeout, 30s query timeout.
28. **Storage: buildQuery()**: Pure function for dynamic WHERE construction. Supports instance_id, metric prefix, time range, labels @> jsonb, LIMIT.
29. **Storage pool**: pgxpool MaxConns=5, application_name="pgpulse_storage". For PGPulse's own DB only.
30. **Conditional migrations**: 002_timescaledb.sql skipped when use_timescaledb=false. MigrateOptions struct controls this.
31. **REST API response format**: JSON envelope `{"data":..., "meta":...}`. Errors: `{"error":{"code":"...","message":"..."}}`.
32. **CSV export**: GET /api/v1/instances/{id}/metrics?format=csv OR Accept: text/csv. Labels as JSON string.
33. **Auth stub**: authStubMiddleware sets user="anonymous" in context. UserFromContext() helper used by handlers. M3 replaces middleware only.
34. **Graceful shutdown order**: HTTP server → Orchestrator → Store.
35. **Pinger interface**: Abstracts pgxpool.Pool.Ping() for health endpoint testability.
36. **Design docs rule**: Always write full paths (e.g. docs/iterations/M2_03_02272026_rest-api/design.md), never use "..." or abbreviations.

---

## What Exists After M2_03

### Repository Structure

```
PGPulse_01/
├── .claude/
│   ├── CLAUDE.md
│   ├── settings.json
│   └── rules/
│       ├── code-style.md
│       ├── architecture.md
│       ├── security.md
│       └── postgresql.md
│
├── cmd/
│   ├── pgpulse-server/
│   │   └── main.go               ← Config → Storage → Orchestrator → HTTP server → graceful shutdown
│   └── pgpulse-agent/
│       └── main.go               ← OS agent placeholder
│
├── configs/
│   └── pgpulse.example.yml
│
├── internal/
│   ├── api/                       ← [NEW in M2_03]
│   │   ├── server.go              ← APIServer struct, Pinger interface, New(), Routes()
│   │   ├── response.go            ← Envelope, ErrorResponse, writeJSON, writeError
│   │   ├── middleware.go          ← requestID, logger, recoverer, CORS, authStub, UserFromContext
│   │   ├── health.go              ← GET /api/v1/health
│   │   ├── instances.go           ← GET /api/v1/instances, GET /api/v1/instances/{id}
│   │   ├── metrics.go             ← GET /api/v1/instances/{id}/metrics (JSON + CSV)
│   │   ├── helpers_test.go        ← mockStore, mockPinger, newTestServer
│   │   ├── health_test.go         ← 5 tests
│   │   ├── instances_test.go      ← 5 tests
│   │   ├── metrics_test.go        ← 10 tests
│   │   └── middleware_test.go     ← 4 tests
│   │
│   ├── collector/
│   │   ├── collector.go           ← Interfaces: Collector, MetricStore, MetricPoint, MetricQuery, InstanceContext
│   │   ├── base.go                ← Base struct, point(), queryContext(), pgssAvailable()
│   │   ├── registry.go            ← RegisterCollector(), CollectAll()
│   │   ├── server_info.go         ← Q2,Q3,Q9,Q10
│   │   ├── connections.go         ← Q11-Q13
│   │   ├── cache.go               ← Q14
│   │   ├── transactions.go        ← Q15
│   │   ├── database_sizes.go      ← Q16
│   │   ├── settings.go            ← Q17
│   │   ├── extensions.go          ← Q18-Q19
│   │   ├── replication_status.go  ← Q20,Q21
│   │   ├── replication_lag.go     ← Q37,Q38
│   │   ├── replication_slots.go   ← Q40
│   │   ├── progress_vacuum.go     ← Q42
│   │   ├── progress_maintenance.go ← Q43,Q45
│   │   ├── progress_operations.go ← Q44,Q46,Q47
│   │   ├── checkpoint.go          ← Checkpoint/BGWriter (stateful)
│   │   ├── io_stats.go            ← pg_stat_io (PG 16+)
│   │   ├── statements_config.go   ← Q48,Q49
│   │   ├── statements_top.go      ← Q50,Q51
│   │   ├── wait_events.go         ← Q53/Q54
│   │   ├── lock_tree.go           ← Q55
│   │   ├── long_transactions.go   ← Q56/Q57
│   │   └── [20 test files]
│   │
│   ├── config/
│   │   ├── config.go              ← Config, ServerConfig (with timeouts, CORS), StorageConfig, InstanceConfig (with Description)
│   │   ├── load.go                ← Load() via koanf, validate() with defaults
│   │   └── config_test.go         ← 7 tests
│   │
│   ├── orchestrator/
│   │   ├── orchestrator.go        ← New(), Start(), Stop()
│   │   ├── runner.go              ← instanceRunner: connect, buildCollectors, start, close
│   │   ├── group.go               ← intervalGroup: run(), collect(), queryInstanceContext()
│   │   ├── logstore.go            ← LogStore placeholder (MetricStore that logs)
│   │   ├── group_test.go          ← 4 tests
│   │   ├── logstore_test.go       ← 4 tests
│   │   └── orchestrator_test.go   ← 1 test
│   │
│   ├── storage/
│   │   ├── migrations/
│   │   │   ├── 001_metrics.sql    ← Metrics table + 3 indexes
│   │   │   └── 002_timescaledb.sql ← Conditional hypertable
│   │   ├── migrate.go             ← Migrate(), go:embed, schema_migrations bootstrap
│   │   ├── pgstore.go             ← PGStore: Write (CopyFrom), Query (dynamic WHERE), Close, Pool()
│   │   ├── pool.go                ← NewPool() (pgxpool, 5 max conns)
│   │   ├── migrate_test.go        ← 5 tests
│   │   ├── pgstore_test.go        ← 9 tests
│   │   └── pool_test.go           ← 1 test
│   │
│   └── version/
│       ├── version.go
│       └── gate.go
│
├── docs/
│   └── iterations/
│       ├── M1_01 through M1_05 folders
│       ├── M2_01_02272026_config-orchestrator/
│       ├── M2_02_02272026_storage-layer/
│       └── M2_03_02272026_rest-api/
│
├── .golangci.yml
├── go.mod
├── go.sum
└── Makefile
```

### Key Interfaces

#### Collector (internal/collector/collector.go — unchanged)

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

type MetricStore interface {
    Write(ctx context.Context, points []MetricPoint) error
    Query(ctx context.Context, query MetricQuery) ([]MetricPoint, error)
    Close() error
}

type MetricQuery struct {
    InstanceID string
    Metric     string
    Labels     map[string]string
    Start      time.Time
    End        time.Time
    Limit      int
}
```

#### API Server (internal/api/server.go — new in M2_03)

```go
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

#### Config (internal/config/config.go — updated in M2_03)

```go
type ServerConfig struct {
    Address         string        `koanf:"address"`          // default ":8080"
    CORSEnabled     bool          `koanf:"cors_enabled"`     // default false
    ReadTimeout     time.Duration `koanf:"read_timeout"`     // default 30s
    WriteTimeout    time.Duration `koanf:"write_timeout"`    // default 60s
    ShutdownTimeout time.Duration `koanf:"shutdown_timeout"` // default 10s
}

type InstanceConfig struct {
    ID                 string   `koanf:"id"`
    DSN                string   `koanf:"dsn"`
    Enabled            *bool    `koanf:"enabled"`
    Description        string   `koanf:"description"`       // NEW in M2_03
    CollectorsEnabled  []string `koanf:"collectors_enabled"`
}

type StorageConfig struct {
    DSN            string `koanf:"dsn"`
    UseTimescaleDB bool   `koanf:"use_timescaledb"`
    RetentionDays  int    `koanf:"retention_days"` // default 30
}
```

### REST API Endpoints (new in M2_03)

| Method | Path | Purpose | Auth |
|--------|------|---------|------|
| GET | `/api/v1/health` | Liveness + storage ping + uptime + version | None (public) |
| GET | `/api/v1/instances` | List instances from config | Stub (anonymous) |
| GET | `/api/v1/instances/{id}` | Single instance detail | Stub (anonymous) |
| GET | `/api/v1/instances/{id}/metrics` | Query stored metrics (JSON or CSV) | Stub (anonymous) |

Metrics query params: `?metric=`, `?start=`, `?end=`, `?limit=`, `?format=json|csv`

### Middleware Stack (applied in order)

1. RequestID — UUID in X-Request-ID header + context
2. Logger — method, path, status, duration via slog
3. Recoverer — catch panics → 500
4. CORS — only when cors_enabled=true
5. AuthStub — sets user="anonymous" in context

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

## Build & Test Status After M2_03

| Check | Result |
|-------|--------|
| `go build ./...` | ✅ Pass |
| `go vet ./...` | ✅ Pass |
| `golangci-lint run` | ✅ 0 issues |
| API tests (24) | ✅ All pass |
| Config tests (7) | ✅ All pass |
| Storage tests (15) | ✅ All pass |
| Orchestrator tests (9) | ✅ All pass |
| Collector tests (all prior) | ✅ All pass |

---

## Next Task: M3_01 — Authentication & RBAC

### Goal

Add JWT-based authentication and role-based access control. After M3_01, all mutation endpoints require a valid token, viewers get read-only access, admins get full access.

### What to Build

| Package/File | Purpose |
|-------------|---------|
| `internal/auth/jwt.go` | JWT token generation (access + refresh) and validation |
| `internal/auth/password.go` | bcrypt hashing and comparison |
| `internal/auth/rbac.go` | Role definitions (admin, viewer), permission checks |
| `internal/auth/middleware.go` | chi middleware: extract Bearer token, validate JWT, set user in context |
| `internal/api/auth.go` | POST /api/v1/auth/login, POST /api/v1/auth/refresh, GET /api/v1/auth/me |
| `internal/storage/migrations/003_auth.sql` | users table (id, username, password_hash, role, created_at, updated_at) |
| Update `internal/api/server.go` | Replace authStubMiddleware with real JWT middleware; protect routes |
| Update `internal/api/middleware.go` | Remove authStub, import real auth middleware |
| Update `cmd/pgpulse-server/main.go` | Wire auth service |
| Test files | jwt_test, password_test, rbac_test, middleware_test, auth_handler_test |

### Key Design Questions for M3_01 Planning

1. **User storage** — users table in PGPulse's own DB (same pool as metrics), or separate?
2. **Initial admin user** — seed via migration, config file, or CLI command?
3. **Token structure** — claims: user_id, username, role, exp, iat. Signing key from config?
4. **Refresh token** — stateful (stored in DB) or stateless (longer-lived JWT)?
5. **Rate limiting** — per-IP on /auth/login. In-memory or Redis-backed?
6. **CSRF** — needed now (no browser client yet) or defer to M5?
7. **Health endpoint** — stays public (no auth), or require auth?

### PGAM Reference

PGAM had zero authentication (`_auth.php` was empty). This is entirely new functionality.

### Session Type

Single Claude Code session — focused scope (auth is self-contained, minimal cross-module changes).

---

## Known Issues

1. **Docker Desktop unavailable** — integration tests CI-only
2. **No auto-reconnect** — orchestrator skips cycle on connection error
3. **No retention cleanup** — metrics accumulate indefinitely
4. **Storage DB failure mid-run** — Write() errors logged, metrics lost for that cycle
5. **DSN parsing for Host/Port** — instances.go parses postgres:// URIs via net/url; key-value DSN format returns empty host/port. Acceptable for M2; M3+ DB inventory will have explicit fields.

---

## Milestone Progress

| Milestone | Iteration | Scope | Status |
|-----------|-----------|-------|--------|
| M1 | M1_01–M1_05 | Collectors (instance, replication, progress, statements, locks) | ✅ Done |
| M2 | M2_01 | Config + Orchestrator | ✅ Done |
| M2 | M2_02 | Storage Layer + Migrations | ✅ Done |
| M2 | M2_03 | REST API + Wiring | ✅ Done |
| **M2** | | **Milestone complete** | **✅ Done** |
| **M3** | **M3_01** | **Auth & Security** | **🔲 Next** |

---

## Environment Reference

| Tool | Version | Notes |
|------|---------|-------|
| Go | 1.24.0 windows/amd64 | |
| golangci-lint | 2.10.1 | v2 config |
| Claude Code | 2.1.59 | Bash broken on Windows |
| Docker Desktop | Not installed | Integration tests → CI only |
| Git | 2.52.0 | |
| chi | v5.2.5 | Added in M2_03 |
| koanf | v2 | |
| pgx | v5.8.0 | Includes pgxpool |
