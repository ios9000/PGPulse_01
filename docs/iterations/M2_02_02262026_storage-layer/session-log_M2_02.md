# Session: 2026-02-27 — M2_02 Storage Layer & Migrations

## Goal

Replace LogStore placeholder with a real PG-backed MetricStore. After M2_02, collected metrics persist to PGPulse's own PostgreSQL database and are queryable by time range, instance, and metric name. Migrations run automatically on startup.

## Session Type

Single Claude Code session (Sonnet) — not Agent Teams. Scope was ~555 lines across 9 files with clear sequential dependencies.

## Planning (Claude.ai)

- Reviewed handoff from M2_01 (config + orchestrator complete)
- Confirmed 5 design points: schema_migrations bootstrap, conditional migration via MigrateOptions, labels @> jsonb containment, CopyFrom column order, test strategy without Docker
- Produced: requirements.md, design.md, prompt.md
- Design reviewed before implementation — two minor nits noted (buildQuery export level, nil labels mutation), neither blocking

## Files Created

| File | Lines (est.) | Purpose |
|------|-------------|---------|
| `internal/storage/migrations/001_metrics.sql` | ~15 | Metrics table + 3 indexes |
| `internal/storage/migrations/002_timescaledb.sql` | ~10 | Conditional hypertable creation |
| `internal/storage/migrate.go` | ~110 | Embedded migration runner (go:embed + schema_migrations) |
| `internal/storage/pgstore.go` | ~140 | PGStore: Write (CopyFrom), Query (dynamic WHERE), Close |
| `internal/storage/pool.go` | ~40 | NewPool() helper (pgxpool, 5 max conns) |
| `internal/storage/migrate_test.go` | ~80 | 5 tests: embedded FS, sort order, conditional logic |
| `internal/storage/pgstore_test.go` | ~120 | 9 tests: buildQuery variants, empty write, nil labels |
| `internal/storage/pool_test.go` | ~20 | 1 test: invalid DSN |

## Files Modified

| File | Change |
|------|--------|
| `cmd/pgpulse-server/main.go` | +~20 lines: conditional PGStore/LogStore init, migration call, shutdown |

## Build & Test Results

| Check | Result |
|-------|--------|
| `go mod tidy` | ✅ Clean |
| `go build ./...` | ✅ Pass |
| `go vet ./...` | ✅ Pass |
| `golangci-lint run` | ✅ 0 issues |
| `go test -v ./internal/storage/` | ✅ 15/15 pass |

### Test Breakdown

| Test | Status |
|------|--------|
| TestMigrateFS_ContainsFiles | ✅ |
| TestMigrateFS_FilesAreSorted | ✅ |
| TestIsConditional_TimescaleDisabled | ✅ |
| TestIsConditional_TimescaleEnabled | ✅ |
| TestIsConditional_RegularMigration | ✅ |
| TestBuildQuery_Empty | ✅ |
| TestBuildQuery_InstanceOnly | ✅ |
| TestBuildQuery_MetricPrefix | ✅ |
| TestBuildQuery_TimeRange | ✅ |
| TestBuildQuery_WithLabels | ✅ |
| TestBuildQuery_WithLimit | ✅ |
| TestBuildQuery_AllFilters | ✅ |
| TestPGStore_Write_EmptySlice | ✅ |
| TestPGStore_Write_NilLabels | ✅ |
| TestNewPool_InvalidDSN | ✅ |

## Key Design Decisions

| Decision | Rationale |
|----------|-----------|
| go:embed inside `internal/storage/` package | Keeps migration SQL co-located with runner, no cross-package embed |
| schema_migrations bootstrapped by Migrate() | Self-contained — no chicken-and-egg problem |
| CopyFrom for batch writes | Single COPY command vs N INSERTs — fastest pgx bulk path |
| buildQuery() as pure function | Testable without DB connection, 7 query construction tests |
| Conditional migration via MigrateOptions | Simple explicit check, no framework, no naming conventions |
| pgxpool MaxConns=5 | PGPulse's own storage DB, not monitored instances |
| LogStore fallback when DSN empty | Zero-config mode preserved — monitoring works without storage DB |

## Architecture State After M2_02

```
main.go
  → config.Load(path)
  → if storage.dsn:
      storage.NewPool() → pool
      storage.Migrate(pool, opts) → schema ready
      storage.NewPGStore(pool) → store
    else:
      orchestrator.NewLogStore() → store
  → orchestrator.New(cfg, store, logger)
  → orch.Start(ctx)
      → per instance: connect → buildCollectors → 3 interval groups
      → each group: collect → store.Write(points)  ← NOW PERSISTS TO PG
  → shutdown: orch.Stop() → pgStore.Close()
```

## Deviations from Design

None. Implementation followed design.md as specified.

## Not Done / Next Iteration

- [ ] **M2_03: REST API + Wiring** — expose stored metrics via HTTP endpoints
- [ ] Retention cleanup (DELETE WHERE time < now() - retention)
- [ ] Integration tests with real PG (CI-only, needs Docker)
- [ ] Health check endpoint

## Milestone Progress

| Iteration | Scope | Status |
|-----------|-------|--------|
| M2_01 | Config + Orchestrator | ✅ Done |
| M2_02 | Storage Layer + Migrations | ✅ Done |
| M2_03 | REST API + Wiring | 🔲 Next |
