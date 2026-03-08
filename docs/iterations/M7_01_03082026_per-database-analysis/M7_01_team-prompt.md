# M7_01 — Per-Database Analysis: Team Prompt

**Iteration:** M7_01_03082026_per-database-analysis
**Date:** 2026-03-08
**Paste this into Claude Code after updating CLAUDE.md "Current Iteration" section.**

---

## Context

We are implementing M7: Per-Database Analysis for PGPulse. This ports
`analiz_db.php` Q2–Q18 (18 queries) and introduces a new `DBCollector` interface
with target-aware orchestration.

Read the full design at `docs/iterations/M7_01_03082026_per-database-analysis/design.md`
before writing any code. All interfaces, struct fields, SQL queries, and patterns
are specified there.

## Critical Rules

- DO NOT modify the existing `Collector` interface — only ADD new interfaces
- DO NOT modify any existing collector files (instance.go, replication.go, etc.)
- DO NOT modify internal/auth/ or internal/api/auth.go
- All per-DB SQL uses `SET LOCAL statement_timeout = '60s'` at the start of CollectDB
- Partial success is required — one failing sub-collector must not block the rest
- DBRunner is a new file (db_runner.go), not changes to runner.go (except wiring)
- go test scope: `./cmd/... ./internal/...` — NEVER `./...`

---

## Create a team of 3 specialists:

---

### SPECIALIST 1 — INTERFACES + DB RUNNER + COLLECTOR

**Your scope:**
- `internal/collector/collector.go` (append only — add Queryer + DBCollector)
- `internal/collector/database.go` (new file)
- `internal/orchestrator/db_runner.go` (new file)
- `internal/orchestrator/runner.go` (wire DBRunner only — no other changes)
- `internal/config/config.go` (add 3 fields only)

**Task 1: Extend internal/collector/collector.go**

Append after the existing `AlertEvaluator` interface — do not touch any existing code:

```go
// Queryer defines the minimal SQL execution interface.
// Both *pgx.Conn and *pgxpool.Pool satisfy this interface natively.
// Use Queryer in collectors to enable mock injection in unit tests.
type Queryer interface {
    Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
    QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// DBCollector collects metrics for a single database.
// Dispatched once per discovered database per cycle by the orchestrator.
type DBCollector interface {
    Name() string
    Interval() time.Duration
    CollectDB(ctx context.Context, q Queryer, dbName string, ic InstanceContext) ([]MetricPoint, error)
}
```

**Task 2: Add 3 fields to internal/config/config.go**

Append to InstanceConfig struct only — do not rename or remove anything:
```go
IncludeDatabases []string `koanf:"include_databases"`
ExcludeDatabases []string `koanf:"exclude_databases"`
MaxConcurrentDBs int      `koanf:"max_concurrent_dbs"`
```

**Task 3: Create internal/orchestrator/db_runner.go**

Full implementation from the design doc. Key components:
- `DBRunner` struct with: instanceID, baseDSN, cfg, primaryPool, collectors, pools map, poolLastSeen map, semaphore chan, logger, metricStore, evalFn
- `NewDBRunner(...)` constructor
- `Run(ctx, ic)` — discovery → eviction → fan-out → telemetry
- `discoverDatabases(ctx)` — query pg_database, apply filters
- `applyFilters(databases)` — include/exclude glob matching via `path/filepath.Match`
- `evictStalePools(seen)` — increment counters, close pools at 3 missed cycles
- `getOrCreatePool(ctx, dbName)` — create pgxpool with MaxConns=2
- `substituteDBName(dsn, newDB)` — handle both key=value and URL DSN formats (see design doc)
- `Close()` — close all pools on shutdown
- `classifyDBError(err)` — returns "timeout", "permission_denied", or "other"

Constants:
```go
const (
    dbPoolMaxConns      = 2
    dbPoolTTLCycles     = 3
    defaultMaxConcurrent = 5
)
```

**Task 4: Wire DBRunner into internal/orchestrator/runner.go**

Add `dbRunner *DBRunner` field to Runner struct.
After primary pool is established in the Runner's init/start:
```go
r.dbRunner = NewDBRunner(
    cfg.ID, cfg.DSN, cfg, r.pool,
    []collector.DBCollector{collector.NewDatabaseCollector()},
    store, eval, logger,
)
```
Add a 5-minute ticker in the Runner's main loop:
```go
dbTicker := time.NewTicker(5 * time.Minute)
// in select:
case <-dbTicker.C:
    go r.dbRunner.Run(ctx, ic)
```
In Runner shutdown: `r.dbRunner.Close()`

**Task 5: Create internal/collector/database.go**

Implement `DatabaseCollector` with 16 collection functions for Q2–Q18.

`CollectDB` pattern:
1. `SET LOCAL statement_timeout = '60s'` via query
2. Call each sub-function via appendPoints helper
3. Return all collected points + first error (partial success)

Sub-functions to implement (all take ctx, Queryer, dbName, return []MetricPoint, error):

`collectLargeObjects` — Q2:
```sql
SELECT count(*) AS lo_count,
       pg_size_pretty(pg_table_size('pg_catalog.pg_largeobject')) AS lo_size
FROM pg_largeobject_metadata
```
MetricPoints: `db.large_objects.count{database}`, `db.large_objects.size_bytes{database}`

`collectFunctionStats` — Q4:
Check `SHOW track_functions` first. If 'none', skip (return empty, no error).
```sql
SELECT schemaname, funcname, calls, total_time, self_time
FROM pg_stat_user_functions
ORDER BY total_time DESC LIMIT 50
```
MetricPoints: `db.function.calls{database, schema, function}`, `db.function.total_time_ms{...}`

`collectSequences` — Q5:
```sql
SELECT schemaname, sequencename, last_value
FROM pg_sequences
ORDER BY schemaname, sequencename
```
MetricPoints: `db.sequence.last_value{database, schema, sequence}`

`collectSchemaSizes` — Q6:
```sql
SELECT n.nspname AS schema,
       SUM(pg_relation_size(c.oid)) AS schema_bytes
FROM pg_class c
JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE n.nspname NOT IN ('pg_catalog','information_schema','pg_toast')
GROUP BY n.nspname
ORDER BY schema_bytes DESC
```
MetricPoints: `db.schema.size_bytes{database, schema}`

`collectUnloggedObjects` — Q7:
```sql
SELECT n.nspname, c.relname, c.relkind
FROM pg_class c
JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE c.relpersistence = 'u'
  AND c.relkind IN ('r','i')
  AND n.nspname NOT IN ('pg_catalog','information_schema')
```
MetricPoints: `db.unlogged.count{database}` = total count

`collectTableSizes` — Q11:
```sql
SELECT n.nspname, c.relname,
       pg_total_relation_size(c.oid) AS total_bytes,
       pg_relation_size(c.oid) AS table_bytes
FROM pg_statio_user_tables s
JOIN pg_class c ON c.relname = s.relname
JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE pg_total_relation_size(c.oid) > 1073741824
ORDER BY total_bytes DESC
```
MetricPoints: `db.table.total_bytes{database, schema, table}`, `db.table.table_bytes{...}`

`collectBloat` — Q12: Use the full CTE from the design doc.
MetricPoints: `db.table.bloat_ratio{database, schema, table}`, `db.table.wasted_bytes{...}`,
`db.index.bloat_ratio{database, schema, index}`, `db.index.wasted_bytes{...}`

`collectCatalogSizes` — Q13:
```sql
SELECT table_name,
       pg_relation_size(quote_ident(table_schema)||'.'||quote_ident(table_name)) AS size_bytes
FROM information_schema.tables
WHERE table_schema = 'pg_catalog'
ORDER BY size_bytes DESC NULLS LAST
LIMIT 20
```
MetricPoints: `db.catalog.size_bytes{database, table}`

`collectTableCacheHit` — Q14:
```sql
SELECT schemaname, relname,
       CASE WHEN heap_blks_read + heap_blks_hit = 0 THEN 0
            ELSE ROUND(heap_blks_hit::numeric / (heap_blks_read + heap_blks_hit) * 100, 2)
       END AS cache_hit_pct
FROM pg_statio_user_tables
ORDER BY heap_blks_read DESC
LIMIT 50
```
MetricPoints: `db.cache_hit.heap_pct{database, schema, table}`

`collectAutovacuumOptions` — Q15:
```sql
SELECT n.nspname, c.relname
FROM pg_class c
JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE c.reloptions::text LIKE '%autovacuum_enabled=false%'
   OR c.reloptions::text LIKE '%autovacuum_enabled=off%'
```
MetricPoints: `db.autovacuum.disabled_count{database}` = total count

`collectVacuumNeed` — Q16: Use the SQL from the design doc.
MetricPoints: `db.vacuum.dead_tuples{database, schema, table}`,
`db.vacuum.dead_pct{...}`, `db.vacuum.autovacuum_age_sec{...}`,
`db.vacuum.autoanalyze_age_sec{...}`

`collectIndexUsage` — Q17:
```sql
SELECT n.nspname, t.relname AS table_name, i.relname AS index_name,
       s.idx_scan, s.idx_tup_read, s.idx_tup_fetch,
       si.idx_blks_read, si.idx_blks_hit
FROM pg_stat_user_indexes s
JOIN pg_statio_user_indexes si ON s.indexrelid = si.indexrelid
JOIN pg_class i ON i.oid = s.indexrelid
JOIN pg_class t ON t.oid = s.relid
JOIN pg_namespace n ON n.oid = t.relnamespace
ORDER BY s.idx_scan ASC
LIMIT 100
```
MetricPoints: `db.index.scan_count{database, schema, table, index}`,
`db.index.tup_read{...}`, `db.index.cache_hit_pct{...}`

`collectUnusedIndexes` — Q18:
```sql
SELECT n.nspname, t.relname, i.relname AS index_name,
       pg_relation_size(s.indexrelid) AS index_bytes
FROM pg_stat_user_indexes s
JOIN pg_class i ON i.oid = s.indexrelid
JOIN pg_class t ON t.oid = s.relid
JOIN pg_namespace n ON n.oid = t.relnamespace
WHERE s.idx_scan = 0
  AND NOT EXISTS (SELECT 1 FROM pg_constraint c WHERE c.conindid = s.indexrelid)
ORDER BY index_bytes DESC
```
MetricPoints: `db.index.unused{database, schema, table, index}` = 1,
`db.index.unused_bytes{...}` = index_bytes

`collectToastSizes` — Q10:
```sql
SELECT n.nspname, c.relname,
       pg_relation_size(c.reltoastrelid) AS toast_bytes
FROM pg_class c
JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE c.relkind = 'r' AND c.reltoastrelid <> 0
  AND pg_relation_size(c.reltoastrelid) > 1048576
ORDER BY toast_bytes DESC
LIMIT 50
```
MetricPoints: `db.toast.size_bytes{database, schema, table}`

`collectPartitions` — Q9:
```sql
SELECT n.nspname, c.relname,
       pg_total_relation_size(c.oid) AS total_bytes,
       pg_indexes_size(c.oid) AS index_bytes,
       (SELECT count(*) FROM pg_inherits WHERE inhparent = c.oid) AS partition_count
FROM pg_class c
JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE c.relkind = 'p'
ORDER BY total_bytes DESC
LIMIT 50
```
MetricPoints: `db.partition.total_bytes{database, schema, table}`,
`db.partition.count{...}`

`collectLargeObjectSizes` — Q8:
```sql
SELECT n.nspname, c.relname,
       pg_table_size(c.oid) AS size_bytes,
       obj_description(c.oid, 'pg_class') AS description
FROM pg_class c
LEFT JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE pg_table_size(c.oid) > 1073741824
  AND c.relkind IN ('r','i','t')
ORDER BY size_bytes DESC
LIMIT 20
```
MetricPoints: `db.object.size_bytes{database, schema, object}`

---

### SPECIALIST 2 — API ENDPOINTS

**Your scope:**
- `internal/api/databases.go` (new file)
- `internal/api/router.go` (add 2 routes)

**Task 1: Create internal/api/databases.go**

Two handlers:

`handleListDatabases` — GET /api/v1/instances/:id/databases

Query the MetricStore for the latest values of these metrics for the instance:
- `db.large_objects.count` → count per database
- `db.vacuum.dead_tuples` → sum per database
- `db.index.unused` → count per database
- `db.table.bloat_ratio` → max per database

Return JSON array of `databaseSummary`:
```go
type databaseSummary struct {
    Name           string  `json:"name"`
    LargeObjCount  int64   `json:"large_object_count"`
    DeadTuples     int64   `json:"dead_tuples"`
    UnusedIndexes  int64   `json:"unused_indexes"`
    MaxBloatRatio  float64 `json:"max_bloat_ratio"`
    LastCollected  string  `json:"last_collected,omitempty"`
}
```

`handleGetDatabaseMetrics` — GET /api/v1/instances/:id/databases/:dbname/metrics

Query MetricStore for all `db.*` metrics for this instance + dbname label.
Return structured `databaseMetrics` response grouping by metric prefix.

Both handlers: require JWT auth (RequireAuth middleware), any authenticated user (PermViewAll).
Return 404 if instance not found. Return empty response (not 404) if no per-DB data collected yet.

**Task 2: Register routes in internal/api/router.go**

Add under the existing auth-protected group:
```go
r.Get("/instances/{id}/databases", s.handleListDatabases)
r.Get("/instances/{id}/databases/{dbname}/metrics", s.handleGetDatabaseMetrics)
```

---

### SPECIALIST 3 — FRONTEND

**Your scope:** `web/src/` only

**Task 1: Add types to web/src/types/models.ts**

```typescript
export interface DatabaseSummary {
  name: string;
  large_object_count: number;
  dead_tuples: number;
  unused_indexes: number;
  max_bloat_ratio: number;
  last_collected?: string;
}

export interface TableMetric {
  schema: string; table: string;
  total_bytes: number; table_bytes: number;
  bloat_ratio?: number; wasted_bytes?: number;
}

export interface IndexMetric {
  schema: string; table: string; index: string;
  scan_count: number; cache_hit_pct?: number;
  unused?: boolean; unused_bytes?: number;
  bloat_ratio?: number;
}

export interface VacuumMetric {
  schema: string; table: string;
  dead_tuples: number; dead_pct: number;
  autovacuum_age_sec?: number; autoanalyze_age_sec?: number;
}

export interface SchemaMetric {
  schema: string; size_bytes: number;
}

export interface DatabaseMetrics {
  database_name: string;
  collected_at: string;
  tables: TableMetric[];
  indexes: IndexMetric[];
  vacuum: VacuumMetric[];
  schemas: SchemaMetric[];
  large_object_count: number;
  large_object_size_bytes: number;
  unused_index_count: number;
  unlogged_count: number;
}
```

**Task 2: Create web/src/hooks/useDatabaseMetrics.ts**

```typescript
export function useDatabaseList(instanceId: string) {
  return useQuery<DatabaseSummary[]>({
    queryKey: ['instances', instanceId, 'databases'],
    queryFn: async () => {
      const data = await apiClient.get<{ databases: DatabaseSummary[] }>(
        `/instances/${instanceId}/databases`
      );
      return data.databases;
    },
    refetchInterval: 5 * 60 * 1000,
  });
}

export function useDatabaseMetrics(instanceId: string, dbName: string) {
  return useQuery<DatabaseMetrics>({
    queryKey: ['instances', instanceId, 'databases', dbName, 'metrics'],
    queryFn: () => apiClient.get<DatabaseMetrics>(
      `/instances/${instanceId}/databases/${dbName}/metrics`
    ),
    refetchInterval: 5 * 60 * 1000,
    enabled: !!dbName,
  });
}
```

**Task 3: Create web/src/pages/DatabaseAnalysisPage.tsx**

Route: `/instances/:instanceId/databases/:dbname`

Header: "Database Analysis: {dbname}" + "Instance: {instance name}" + "Last collected: X min ago" + manual refresh button.

Sections (all collapsible, same pattern as ServerDetail):

**Tables section** — TableMetric array sorted by total_bytes DESC
Table columns: Schema | Table | Size | Bloat | Index Size | Rows (if available)
Bloat column: plain number if <2, yellow badge if 2–10x, red badge if >10x

**Vacuum Health section** — VacuumMetric array sorted by dead_tuples DESC
Columns: Schema | Table | Dead Tuples | Dead% | Last Autovacuum | Last Analyze
Row coloring: dead_pct > 20% → yellow, > 40% → red
autovacuum_age_sec > 86400 (1 day) → yellow, > 259200 (3 days) → red

**Indexes section** — two sub-tables:
- Unused indexes (index.unused = 1): Schema | Table | Index | Size — always red badge count in section header
- Index usage stats: Schema | Table | Index | Scans | Cache Hit%

**Schema Sizes section** — ECharts horizontal bar chart
x-axis: bytes (format as GB/MB), y-axis: schema names, sorted by size DESC

**Large Objects section**
If count > 0: yellow warning card "⚠ {count} large objects, total {size}"
If count = 0: small green "No large objects"

**Unlogged Objects section**
If count > 0: red warning card "🔴 {count} unlogged tables/indexes — data lost on crash"
If count = 0: small green "No unlogged objects"

**Sequences section** — table: Schema | Sequence | Last Value

**Functions section** (hidden if no function data)
Columns: Schema | Function | Calls | Avg Time (ms)

**No data state**: if `collected_at` is null or > 10 min ago without data:
Show placeholder: "Per-database analysis not yet collected. Data refreshes every 5 minutes."

**Task 4: Update web/src/pages/ServerDetail.tsx**

In the existing Databases section, make each database name a clickable link:
```tsx
// Change plain text database name to:
<Link to={`/instances/${instanceId}/databases/${db.datname}`}>
  {db.datname}
</Link>
```

**Task 5: Add route in router (App.tsx or wherever routes are defined)**

```tsx
<Route path="/instances/:instanceId/databases/:dbname" element={<DatabaseAnalysisPage />} />
```

---

## Coordination and Dependencies

```
Specialist 1 (Interfaces + Runner + Collector)  — start immediately
Specialist 2 (API)  — depends on Spec 1 interfaces (Queryer, DBCollector)
                      Wait for Spec 1 to commit collector.go additions, then proceed
Specialist 3 (Frontend)  — independent of Spec 1 and 2, start immediately
```

Team Lead: merge Spec 1 first, then Spec 2, then Spec 3.

## Build Verification

```bash
cd web && npm run build && npm run lint && npm run typecheck
cd .. && go build ./... && go vet ./...
go test ./cmd/... ./internal/... && golangci-lint run
```

All five commands must pass with zero errors before committing.

## Commit Messages

```
feat(collector): add Queryer and DBCollector interfaces
feat(collector): add per-database analysis collector (Q2-Q18)
feat(orchestrator): add DBRunner with pool map, TTL eviction, semaphore fan-out
feat(config): add include/exclude_databases and max_concurrent_dbs config
feat(api): add per-database analysis endpoints
feat(ui): add DatabaseAnalysisPage with tables, vacuum, indexes, schema sections
```
