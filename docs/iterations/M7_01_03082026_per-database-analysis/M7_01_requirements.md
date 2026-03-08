# M7_01 — Per-Database Analysis: Requirements

**Iteration:** M7_01_03082026_per-database-analysis
**Date:** 2026-03-08
**Milestone:** M7 (Per-Database Analysis)

---

## Goal

Port `analiz_db.php` Q1–Q18 (18 queries) to Go. Implement a new `DBCollector`
interface and target-aware orchestration architecture that enables per-database
metric collection at scale. Add the Per-Database Analysis page to the frontend.

This is the last major PGAM query porting milestone — after M7, 70/76 PGAM queries
will be covered (6 are intentionally deferred or skipped).

---

## Context: Why This Is Architecturally Distinct

All 33 collectors built so far (M1–M6) connect to the `postgres` database and query
instance-level system views. Per-database analysis requires connections to *each
individual user database* — bloat estimation, index usage, vacuum health, sequences,
and TOAST sizes only exist within that database's context.

This requires a new parallel dispatch path, not a change to the existing `Collector`
interface, which is frozen.

---

## New Architecture: DBCollector + Target-Aware Orchestration

### Core Principle
Gathering global instance metrics and gathering per-database metrics are
fundamentally different domains. The Interface Segregation Principle applies:
two interfaces, two dispatch paths, clean separation of concerns.

This same architecture will later enable logical replication monitoring (deferred
Q41), which also requires per-database connections.

### New Interfaces (add to internal/collector/collector.go)

```go
// Queryer defines the minimal interface for executing SQL.
// Both *pgx.Conn and *pgxpool.Pool satisfy this interface.
// Enables mock injection in unit tests without spinning up a real database.
type Queryer interface {
    Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
    QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// DBCollector collects metrics for a single database.
// It is dispatched once per discovered database per collection cycle.
type DBCollector interface {
    Name() string
    Interval() time.Duration
    CollectDB(ctx context.Context, q Queryer, dbName string, ic InstanceContext) ([]MetricPoint, error)
}
```

### Orchestrator Changes (internal/orchestrator/runner.go)

The Runner grows per-database state alongside its existing per-instance state:

```go
type Runner struct {
    // existing fields...
    dbPools       map[string]*pgxpool.Pool // key: database name
    dbPoolSeen    map[string]int           // key: database name, value: cycles since last seen
    dbMu          sync.Mutex
    dbCollectors  []collector.DBCollector
    dbSemaphore   chan struct{}             // bounded fan-out
}
```

**DB Discovery cycle (runs every 5 minutes, same as DBCollector.Interval()):**

1. Query `pg_database` on the primary pool for all connectable, non-template databases
2. Apply `IncludeDatabases` and `ExcludeDatabases` regex filters from instance config
3. For each discovered database:
   - If pool exists in `dbPools`: mark as seen (reset `dbPoolSeen` counter)
   - If pool is new: construct DSN with database substituted, create `pgxpool.Pool` (max 2 conns)
4. TTL eviction: increment `dbPoolSeen` for all pools. Any pool unseen for ≥ 3 cycles → `pool.Close()` + delete from map
5. Fan-out: for each database × each DBCollector, acquire semaphore slot, launch goroutine calling `CollectDB(ctx, pool, dbName, ic)`, release slot on return
6. Collect all results, write MetricPoints, emit internal telemetry

**Bounded concurrency (semaphore pattern):**

```go
// Initialized as: make(chan struct{}, cfg.MaxConcurrentDBs)
// Default MaxConcurrentDBs = 5
for _, db := range discoveredDBs {
    for _, c := range r.dbCollectors {
        r.dbSemaphore <- struct{}{}  // acquire
        go func(dbName string, col collector.DBCollector) {
            defer func() { <-r.dbSemaphore }()  // release
            points, err := col.CollectDB(ctx, r.dbPools[dbName], dbName, ic)
            // ...
        }(db, c)
    }
}
```

**Internal telemetry (4 MetricPoints emitted after each per-DB cycle):**

```
pgpulse.agent.db.discovered{instance_id="..."}  — total DBs found in pg_database
pgpulse.agent.db.collected{instance_id="..."}   — DBs successfully scraped this cycle
pgpulse.agent.db.errors{instance_id="...", reason="timeout"}
pgpulse.agent.db.errors{instance_id="...", reason="permission_denied"}
pgpulse.agent.db.errors{instance_id="...", reason="other"}
```

---

## Database Discovery Policy

### Algorithm

```
1. SELECT datname FROM pg_database
   WHERE NOT datistemplate AND datallowconn
   ORDER BY datname

2. If IncludeDatabases is non-empty:
   retain only databases matching ANY include pattern (glob or regex)

3. If ExcludeDatabases is non-empty:
   discard databases matching ANY exclude pattern

4. Result = filtered list → initialize/evict pools
```

### Config Extension (internal/config/config.go)

Add to `InstanceConfig`:

```go
IncludeDatabases  []string `koanf:"include_databases"`  // optional glob/regex patterns
ExcludeDatabases  []string `koanf:"exclude_databases"`  // optional glob/regex patterns
MaxConcurrentDBs  int      `koanf:"max_concurrent_dbs"` // default: 5
```

Example config:
```yaml
instances:
  - id: "prod-primary"
    dsn: "host=10.0.0.1 port=5432 dbname=postgres user=pgpulse sslmode=disable"
    include_databases: ["prod_*", "billing"]
    exclude_databases: ["*_test", "*_staging"]
    max_concurrent_dbs: 5
```

### DSN Construction for Per-DB Pools

Replace `dbname=postgres` in the instance DSN with `dbname={discovered_db}`.
Parse DSN as key=value pairs (already used throughout the codebase for DSN handling).
Per-DB pool config: `MaxConns: 2` (small — these are analysis queries, not hot path).

---

## PGAM Queries to Port (18 total → internal/collector/database.go)

All collected at 5-minute interval. All MetricPoints carry label `{database: dbName}`.

| PGAM # | Function | Description |
|--------|----------|-------------|
| Q1 | — | Recovery state — already in instance collector, skip |
| Q2 | collectLargeObjects | LO count, size, owner breakdown |
| Q3 | collectLargeObjectRefs | Tables with OID/lo columns |
| Q4 | collectFunctionStats | pg_stat_user_functions — calls, total/self time |
| Q5 | collectSequences | pg_sequences — schemaname, sequencename, last_value |
| Q6 | collectSchemaSizes | Schema sizes absolute + % of DB total |
| Q7 | collectUnloggedObjects | Tables/indexes with relpersistence='u' |
| Q8 | collectLargeObjectSizes | Objects > 1GB (uses pg_table_size) |
| Q9 | collectPartitions | Partitioned table hierarchy sizes |
| Q10 | collectToastSizes | TOAST table sizes per relation |
| Q11 | collectTableSizes | Table sizes > 1GB (pg_statio_user_tables) |
| Q12 | collectBloat | Table + index bloat estimate (complex CTE using pg_stats) |
| Q13 | collectCatalogSizes | System catalog table sizes |
| Q14 | collectTableCacheHit | Cache hit per table (pg_statio_user_tables) |
| Q15 | collectAutovacuumOptions | Tables with autovacuum_enabled=off |
| Q16 | collectVacuumNeed | Vacuum/analyze staleness (n_dead_tup, last_autovacuum) |
| Q17 | collectIndexUsage | Index scan count + hit ratio |
| Q18 | collectUnusedIndexes | Indexes with idx_scan=0 |

**Statement timeout:** 60 seconds for all per-DB queries (matches PGAM's `$statement_timeout_db`).
Set via `SET LOCAL statement_timeout = '60s'` at the start of each CollectDB call.

**Version gates required:**
- Q4 (pg_stat_user_functions): requires `track_functions != 'none'` — check pg_settings first, skip if disabled
- Q16 (vacuum need): `n_mod_since_analyze` column added in PG 9.4 — safe for our PG 14+ minimum

---

## Metric Naming Convention

All per-DB metrics are prefixed `db.` and labeled with `{database: dbName}`:

```
db.table.total_bytes{database="app_db", schema="public", table="orders"}
db.table.bloat_ratio{database="app_db", schema="public", table="orders"}
db.index.scan_count{database="app_db", schema="public", index="orders_pkey"}
db.index.unused{database="app_db", schema="public", index="orders_old_idx"}  = 1 if unused
db.vacuum.dead_tuples{database="app_db", schema="public", table="events"}
db.vacuum.last_autovacuum_age_seconds{database="app_db", ...}
db.schema.size_bytes{database="app_db", schema="public"}
db.schema.size_pct{database="app_db", schema="public"}
db.cache_hit.heap_pct{database="app_db", schema="public", table="orders"}
db.large_objects.count{database="app_db"}
db.large_objects.size_bytes{database="app_db"}
db.sequences.last_value{database="app_db", schema="public", sequence="order_id_seq"}
db.catalog.size_bytes{database="app_db", table="pg_attribute"}
db.unlogged.count{database="app_db"}
db.function.calls{database="app_db", schema="public", function="process_order"}
db.function.total_time_ms{database="app_db", schema="public", function="process_order"}
```

---

## Frontend: Per-Database Analysis Page

### New page: DatabaseAnalysisPage.tsx

Route: `/instances/:id/databases/:dbname`

Accessible from the existing Databases section on ServerDetail (database name is a link).

**Sections (collapsible, same pattern as ServerDetail):**

1. **Tables** — table sizes > 1MB, bloat ratio (yellow >2x, red >10x)
2. **Indexes** — usage stats, unused indexes highlighted red
3. **Vacuum Health** — dead tuples, last vacuum/analyze age, autovacuum disabled warning
4. **Schema Sizes** — bar chart by schema (ECharts horizontal bar)
5. **TOAST** — TOAST sizes per relation (shown only if non-trivial)
6. **Sequences** — list with last_value
7. **Functions** — call stats (shown only if track_functions enabled)
8. **Large Objects** — count + size warning (always yellow if any exist, matching PGAM)
9. **Unlogged Objects** — always red badge if any exist (matching PGAM)

### Update: FleetOverview or ServerDetail

The existing Databases section on ServerDetail shows per-DB stats (size, cache hit,
txid wraparound). Each database name should become a clickable link to the
`/instances/:id/databases/:dbname` page.

### New API endpoint

```
GET /api/v1/instances/:id/databases                    — list databases with summary metrics
GET /api/v1/instances/:id/databases/:dbname/metrics    — full per-DB analysis metrics
```

---

## Non-Functional Requirements

- Per-DB pool MaxConns = 2 (analysis queries, not hot path)
- TTL eviction: pool closed after 3 consecutive cycles without seeing the database
- Semaphore hard limit: `MaxConcurrentDBs` (default 5, configurable per instance)
- CollectDB errors: logged at WARN with `{instance_id, database, collector}` fields; never panic
- `Queryer` interface enables unit tests without real DB — all 18 functions must be testable with a mock Queryer
- Internal telemetry metrics treated identically to any other MetricPoint — stored in TimescaleDB, queryable via API

---

## Out of Scope

- Logical replication monitoring (Q41) — still deferred; this architecture enables it cleanly in M8+
- Query plan capture — M8
- Session kill from per-DB view — M8
- Large object orphan detection (lo.php) — low priority, defer
