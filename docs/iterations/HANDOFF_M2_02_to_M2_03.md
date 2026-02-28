# PGPulse — Iteration Handoff: M2_02 → M2_03

> **Purpose:** Upload this file when starting the next Claude.ai chat.
> Contains EVERYTHING needed to begin M2_03 without re-discovery.
> **Created:** 2026-02-27 (end of M2_02)

---

## DO NOT RE-DISCUSS

These decisions are final. The new chat must not revisit them:

1. **Stack**: Go 1.24.0, pgx v5.8.0, chi v5, koanf, slog, testcontainers-go
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

---

## What Exists After M2_02

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
│   │   └── main.go               ← Config → Storage (PGStore|LogStore) → Orchestrator → signal handling
│   └── pgpulse-agent/
│       └── main.go               ← OS agent placeholder
│
├── configs/
│   └── pgpulse.example.yml
│
├── internal/
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
│   │   ├── config.go              ← Config, ServerConfig, StorageConfig, InstanceConfig structs
│   │   ├── load.go                ← Load() via koanf, validate()
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
│   ├── storage/                   ← [NEW in M2_02]
│   │   ├── migrations/
│   │   │   ├── 001_metrics.sql    ← Metrics table + 3 indexes
│   │   │   └── 002_timescaledb.sql ← Conditional hypertable
│   │   ├── migrate.go             ← Migrate(), go:embed, schema_migrations bootstrap
│   │   ├── pgstore.go             ← PGStore: Write (CopyFrom), Query (dynamic WHERE), Close
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
│       ├── M2_01_.../
│       └── M2_02_.../
│
├── .golangci.yml
├── go.mod
├── go.sum
└── Makefile
```

### Key Interfaces (collector.go — unchanged)

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

### Storage Interfaces (new in M2_02)

```go
// internal/storage/pgstore.go
type PGStore struct {
    pool   *pgxpool.Pool
    logger *slog.Logger
}

func NewPGStore(pool *pgxpool.Pool, logger *slog.Logger) *PGStore
func (s *PGStore) Write(ctx context.Context, points []collector.MetricPoint) error  // CopyFrom, 10s timeout
func (s *PGStore) Query(ctx context.Context, q collector.MetricQuery) ([]collector.MetricPoint, error) // dynamic WHERE, 30s timeout
func (s *PGStore) Close() error

// internal/storage/migrate.go
type MigrateOptions struct { UseTimescaleDB bool }
func Migrate(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger, opts MigrateOptions) error

// internal/storage/pool.go
func NewPool(ctx context.Context, dsn string) (*pgxpool.Pool, error)

// internal/storage/pgstore.go (pure function, testable without DB)
func buildQuery(q collector.MetricQuery) (string, []any)
```

### Config Structs (unchanged from M2_01)

```go
type Config struct {
    Server    ServerConfig     `koanf:"server"`
    Storage   StorageConfig    `koanf:"storage"`
    Instances []InstanceConfig `koanf:"instances"`
}

type StorageConfig struct {
    DSN            string `koanf:"dsn"`
    UseTimescaleDB bool   `koanf:"use_timescaledb"`
    RetentionDays  int    `koanf:"retention_days"` // default 30
}
```

### main.go Flow (after M2_02)

```
main.go
  → config.Load(path) → Config
  → if cfg.Storage.DSN != "":
      storage.NewPool(ctx, dsn) → pool
      storage.Migrate(ctx, pool, logger, MigrateOptions{...}) → schema ready
      storage.NewPGStore(pool, logger) → store
    else:
      orchestrator.NewLogStore(logger) → store
  → orchestrator.New(cfg, store, logger) → Orchestrator
  → orch.Start(ctx)
      → per instance: connect → buildCollectors → 3 interval groups
      → each group: collect → store.Write(points)  ← PERSISTS TO PG (or logs)
  → signal.Notify(SIGINT, SIGTERM)
  → orch.Stop() → pgStore.Close()
```

---

## Build & Test Status After M2_02

| Check | Result |
|-------|--------|
| `go build ./...` | ✅ Pass |
| `go vet ./...` | ✅ Pass |
| `golangci-lint run` | ✅ 0 issues |
| Storage tests (15) | ✅ All pass |
| Config tests (7) | ✅ All pass |
| Orchestrator tests (9) | ✅ All pass |
| Collector tests (all prior) | ✅ All pass |

---

## Next Task: M2_03 — REST API & Wiring

### Goal

Expose collected metrics via a REST API. After M2_03, an HTTP client can query stored metrics, list monitored instances, and check server health.

### What to Build

| Package/File | Purpose |
|-------------|---------|
| `internal/api/router.go` | chi router setup, middleware (logging, request ID, recovery) |
| `internal/api/health.go` | GET /api/v1/health — liveness + DB connectivity |
| `internal/api/instances.go` | GET /api/v1/instances — list from config; GET /api/v1/instances/:id — detail |
| `internal/api/metrics.go` | GET /api/v1/instances/:id/metrics — query stored metrics with time range |
| Update `cmd/pgpulse-server/main.go` | Wire chi router, start HTTP server alongside orchestrator |
| Test files | router_test, health_test, instances_test, metrics_test |

### Key Design Questions for M2_03 Planning

1. **HTTP server lifecycle** — Start HTTP server in a goroutine, graceful shutdown alongside orchestrator?
2. **Instance list source** — Serve from config ([]InstanceConfig) or query orchestrator for live status?
3. **Metrics query params** — URL query params for time range? `?start=...&end=...&metric=...&limit=...`
4. **Response format** — JSON with envelope `{"data": [...], "meta": {...}}` or flat array?
5. **Auth** — Deferred to M3, but should we stub middleware now?
6. **CORS** — Needed for future frontend dev mode?

### Session Type

Likely single Claude Code session — scope is focused on HTTP handlers + wiring.

---

## Known Issues

1. **Docker Desktop unavailable** — integration tests CI-only
2. **No auto-reconnect** — orchestrator skips cycle on connection error
3. **No retention cleanup** — metrics accumulate indefinitely
4. **Storage DB failure mid-run** — Write() errors logged, metrics lost for that cycle

---

## Environment Reference

| Tool | Version | Notes |
|------|---------|-------|
| Go | 1.24.0 windows/amd64 | |
| golangci-lint | 2.10.1 | v2 config |
| Claude Code | 2.1.59 | Bash broken on Windows |
| Docker Desktop | Not installed | Integration tests → CI only |
| Git | 2.52.0 | |
| koanf | v2 | |
| pgx | v5.8.0 | Includes pgxpool |

---

## Milestone Progress

| Iteration | Scope | Status |
|-----------|-------|--------|
| M2_01 | Config + Orchestrator | ✅ Done |
| M2_02 | Storage Layer + Migrations | ✅ Done |
| **M2_03** | **REST API + Wiring** | **🔲 Next** |
