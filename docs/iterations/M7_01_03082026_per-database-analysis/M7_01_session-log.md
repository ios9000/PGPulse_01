# Session Log — M7_01 — Per-Database Analysis
**Date:** 2026-03-08
**Duration:** ~13 minutes
**Status:** ✅ Complete

---

## Goal
Port analiz_db.php Q2–Q18 (18 queries) to Go. Introduce DBCollector interface
and target-aware orchestration with dynamic per-database connection pools.

---

## Agent Team Configuration
- Team Lead: Opus 4.6
- Specialist 1: Interfaces + DB Runner + Collector
- Specialist 2: API Endpoints
- Specialist 3: Frontend
- Duration: 12m 58s

---

## PGAM Queries Ported

| PGAM # | Function | Target | Agent |
|--------|----------|--------|-------|
| Q2 | collectLargeObjects | database.go | Spec 1 |
| Q3 | collectLargeObjectRefs | database.go | Spec 1 |
| Q4 | collectFunctionStats | database.go | Spec 1 |
| Q5 | collectSequences | database.go | Spec 1 |
| Q6 | collectSchemaSizes | database.go | Spec 1 |
| Q7 | collectUnloggedObjects | database.go | Spec 1 |
| Q8 | collectLargeObjectSizes | database.go | Spec 1 |
| Q9 | collectPartitions | database.go | Spec 1 |
| Q10 | collectToastSizes | database.go | Spec 1 |
| Q11 | collectTableSizes | database.go | Spec 1 |
| Q12 | collectBloat | database.go | Spec 1 |
| Q13 | collectCatalogSizes | database.go | Spec 1 |
| Q14 | collectTableCacheHit | database.go | Spec 1 |
| Q15 | collectAutovacuumOptions | database.go | Spec 1 |
| Q16 | collectVacuumNeed | database.go | Spec 1 |
| Q17 | collectIndexUsage | database.go | Spec 1 |
| Q18 | collectUnusedIndexes | database.go | Spec 1 |

Q1 (recovery state) — skipped, already covered by instance collector.
Total this iteration: 17 functions covering Q2–Q18. Cumulative: ~69/76 PGAM queries ported.

---

## Agent Activity Summary

### Specialist 1 — Interfaces + DB Runner + Collector
- `internal/collector/collector.go` — Queryer + DBCollector interfaces appended (existing interfaces untouched)
- `internal/config/config.go` — IncludeDatabases, ExcludeDatabases, MaxConcurrentDBs added to InstanceConfig
- `internal/orchestrator/db_runner.go` — NEW: DBRunner with dynamic pool map, TTL eviction (3 cycles), semaphore fan-out, glob include/exclude filtering, DSN substitution (URL + key=value formats), error classification (timeout/permission_denied/other), 5 telemetry MetricPoints per cycle
- `internal/orchestrator/runner.go` — dbRunner field wired, 5-minute ticker goroutine, cleanup on Close()
- `internal/collector/database.go` — NEW: DatabaseCollector with 16 sub-collector functions

### Specialist 2 — API Endpoints
- `internal/api/databases.go` — NEW: handleListDatabases + handleGetDatabaseMetrics
- `internal/api/server.go` — Both routes registered in auth-enabled and auth-disabled sections

### Specialist 3 — Frontend
- `web/src/types/models.ts` — 9 new types: DatabaseSummary, TableMetric, IndexMetric, VacuumMetric, SchemaMetric, SequenceMetric, FunctionMetric, CatalogMetric, DatabaseMetrics
- `web/src/hooks/useDatabaseMetrics.ts` — useDatabaseList + useDatabaseMetrics hooks
- `web/src/pages/DatabaseDetail.tsx` — Full analysis page: Tables, Vacuum Health, Indexes (unused + usage), Schema Sizes, Large Objects, Unlogged Objects, Sequences, Functions sections with color coding and badges

---

## Architecture Decisions Made

| # | Decision | Rationale |
|---|----------|-----------|
| D-M7-01 | New DBCollector interface (Option C) | ISP: instance and per-DB metrics are different domains; Collector interface frozen |
| D-M7-02 | Queryer abstraction | Decouples collectors from pgx; enables mock injection in unit tests |
| D-M7-03 | Dynamic pool map with TTL eviction (3 cycles) | Prevents leaks when databases dropped or excluded by config |
| D-M7-04 | Semaphore fan-out (MaxConcurrentDBs=5 default) | Storm protection: prevents overwhelming PG on instances with many databases |
| D-M7-05 | Partial success in CollectDB | Bloat CTE failure on huge DB must not block vacuum metrics |
| D-M7-06 | Internal telemetry MetricPoints | Orchestrator health observable from same TimescaleDB store |
| D-M7-07 | Hybrid discovery: pg_database + include/exclude glob | Zero-config by default; override levers in pgpulse.yml when needed |
| D-M7-08 | 5-minute interval | Bloat estimation CTE is expensive; don't hammer DB |
| D-M7-09 | Per-DB pool MaxConns=2 | Analysis queries, not hot path; prevent pool explosion |

---

## Build Results
- go build ./... — ✅ pass
- go vet ./... — ✅ pass
- go test ./cmd/... ./internal/... — ✅ all 14 packages pass
- golangci-lint run — ✅ 0 issues
- npx tsc --noEmit — ✅ 0 errors

---

## PGAM Query Porting Status (cumulative)
- analiz2.php Q1–Q57: ✅ complete (M1–M6)
- analiz_db.php Q2–Q18: ✅ complete (M7_01)
- Skipped/deferred: Q36, Q39 (PG <10), Q41 (logical replication, deferred to M8+), Q52 (covered by Q50+Q51)
- Estimated total ported: ~69/76

---

## Not Done / Next Iteration
- M8: Logical replication monitoring (Q41) — DBCollector architecture is the correct hook
- M8: Query plan capture (EXPLAIN/EXPLAIN ANALYZE)
- M8: Session kill from per-DB view
- M8: ML baseline (anomaly detection, STL decomposition)
- Large object orphan detection (lo.php) — low priority, deferred
