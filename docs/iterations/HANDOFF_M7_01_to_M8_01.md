# PGPulse — Iteration Handoff: M7_01 → M8_01
**Date:** 2026-03-08
**From:** M7_01 — Per-Database Analysis
**To:** M8_01 — ML Baseline / Anomaly Detection

---

## DO NOT RE-DISCUSS

These decisions are final. Do not revisit:

- DBCollector interface is separate from Collector — parallel dispatch path, never merge them
- Queryer abstraction is the correct hook for all future per-DB collectors (including logical replication)
- DBRunner owns the pool map — never pass *pgxpool.Pool directly to a DBCollector
- Logical replication (Q41) is deferred but the DBCollector interface is the right home for it
- RBAC: 4 roles (super_admin, roles_admin, dba, app_admin) — locked, no changes
- Collector interface signature is frozen: `Collect(ctx, *pgx.Conn, InstanceContext)` — no new parameters
- TimescaleDB is conditional — migrations fall back gracefully when extension absent
- No COPY TO PROGRAM ever — OS metrics via Go agent only
- golangci-lint v2 config format — do not downgrade or change linter config

---

## What Exists Now

### New interfaces (internal/collector/collector.go, appended in M7_01)

```go
type Queryer interface {
    Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
    QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type DBCollector interface {
    Name() string
    Interval() time.Duration
    CollectDB(ctx context.Context, q Queryer, dbName string, ic InstanceContext) ([]MetricPoint, error)
}
```

### New config fields (internal/config/config.go, added to InstanceConfig)

```go
IncludeDatabases []string `koanf:"include_databases"`
ExcludeDatabases []string `koanf:"exclude_databases"`
MaxConcurrentDBs int      `koanf:"max_concurrent_dbs"` // default 5
```

### New files created in M7_01

| File | Description |
|------|-------------|
| internal/orchestrator/db_runner.go | DBRunner: pool map, TTL eviction (3 cycles), semaphore fan-out, glob filters, DSN substitution, 5 telemetry points/cycle |
| internal/collector/database.go | DatabaseCollector: 16 sub-functions covering analiz_db.php Q2–Q18 |
| internal/api/databases.go | handleListDatabases + handleGetDatabaseMetrics |
| web/src/hooks/useDatabaseMetrics.ts | useDatabaseList + useDatabaseMetrics |
| web/src/pages/DatabaseDetail.tsx | Full per-DB analysis page |

### Modified files in M7_01

| File | Change |
|------|--------|
| internal/collector/collector.go | Queryer + DBCollector interfaces appended |
| internal/config/config.go | 3 fields added to InstanceConfig |
| internal/orchestrator/runner.go | dbRunner field + 5-min ticker + Close() cleanup |
| internal/api/server.go | 2 new routes registered |
| web/src/types/models.ts | 9 new types added |

---

## What Was Just Completed

M7_01 ported all 17 analiz_db.php sub-collectors (Q2–Q18, Q1 skipped as duplicate):
bloat estimation CTE, vacuum need analysis, index usage, unused indexes, schema sizes,
TOAST sizes, partition hierarchy, large objects, sequences, functions, catalog sizes,
autovacuum options, table sizes, table cache hit ratios, unlogged objects.

New DBRunner manages dynamic per-database connection pools with TTL eviction and
semaphore-bounded fan-out. Discovery uses pg_database + include/exclude glob filters.
5 internal telemetry MetricPoints emitted per cycle (discovered, collected, 3 error buckets).

Frontend: DatabaseDetail page accessible via clickable database names on ServerDetail.
Sections: Tables, Vacuum Health, Indexes (unused + usage), Schema Sizes (ECharts bar),
Large Objects, Unlogged Objects, Sequences, Functions.

Build: go build ✅ · go vet ✅ · go test ./cmd/... ./internal/... (14 packages) ✅ · golangci-lint 0 issues ✅ · npx tsc --noEmit 0 errors ✅

---

## PGAM Query Porting Status

| Source | Queries | Status |
|--------|---------|--------|
| analiz2.php Q1–Q19 | Instance metrics | ✅ M1 |
| analiz2.php Q20–Q41 | Replication | ✅ M2 (Q41 deferred) |
| analiz2.php Q42–Q47 | Progress | ✅ M2 |
| analiz2.php Q48–Q52 | Statements | ✅ M3 |
| analiz2.php Q53–Q58 | Locks/wait events | ✅ M4 |
| OS/cluster Q4–Q8, Q22–Q35 | COPY TO PROGRAM → Go | ✅ M6 |
| analiz_db.php Q2–Q18 | Per-DB analysis | ✅ M7 |
| Q41 | Logical replication | 🔲 Deferred — needs DBCollector |
| Q36, Q39 | PG <10 xlog functions | ⏭ Skipped (below minimum PG 14) |
| Q52 | Normalized totals | ⏭ Covered by Q50+Q51 combined |

**~69/76 ported. Remaining: Q41 (deferred), Q36/Q39/Q52 (skipped).**

---

## Known Issues

- DatabaseDetail.tsx was named differently from what requirements.md called DatabaseAnalysisPage.tsx — the agent used DatabaseDetail.tsx. Functionally equivalent, route works. No action needed unless you prefer to rename.
- Large object orphan detection (lo.php) not implemented — low priority, no plan yet.
- Logical replication (Q41) deferred — DBCollector interface is the correct hook when ready.

---

## Build & Test Commands (unchanged)

```bash
cd web && npm run build && npm run lint && npm run typecheck
cd .. && go build ./... && go vet ./...
go test -race ./cmd/... ./internal/... && golangci-lint run
```

**NEVER use `go test ./...`** — scans web/node_modules/ and fails.

---

## Next Task: M8_01

M8 is ML Phase 1. Scope to discuss and design in Claude.ai before spawning agents:

**Core goal:** Anomaly detection on metric time-series using statistical baselines.
No external ML service — pure Go using gonum.

**Candidate features for M8_01:**
1. Baseline computation — rolling mean + stddev per metric per instance (7-day window)
2. Z-score anomaly detection — flag values > N stddev from baseline
3. STL decomposition — seasonal/trend/residual decomposition for metrics with daily cycles (connections, cache hit, query time)
4. Anomaly alert integration — emit alert when anomaly detected (uses existing AlertEvaluator)
5. Forecast: disk space exhaustion — linear regression on disk usage trend → days until full

**Open questions to resolve in M8_01 design session:**
- Which metrics get anomaly detection first? (connections, cache hit, bloat ratio, query time are strongest candidates)
- STL decomposition period: 24h (hourly data) or 7-day (daily patterns)?
- Sensitivity: fixed Z-score threshold or per-metric configurable?
- Where does the ML state live? In-memory (lost on restart) or persisted in TimescaleDB?
- Should anomaly results surface as a new MetricPoint type or as a new API endpoint?
- Frontend: anomaly markers on existing ECharts time-series, or separate anomaly feed?

**New package:** `internal/ml/` — gonum-based, owned by ML Agent (unlocked at M8).

**Deferred to later M8 sub-iterations:**
- Logical replication (Q41) — can be M8_01 or M8_02 depending on complexity discussion
- Query plan capture (EXPLAIN ANALYZE)
- Workload forecasting beyond disk (connection saturation, query performance degradation)
