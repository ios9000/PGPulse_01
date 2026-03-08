# M7_01 — Per-Database Analysis: Design

**Iteration:** M7_01_03082026_per-database-analysis
**Date:** 2026-03-08

---

## New Files

```
internal/collector/collector.go      — add Queryer + DBCollector interfaces (extend, do not replace)
internal/collector/database.go       — DatabaseCollector implementing DBCollector (18 functions)
internal/orchestrator/db_runner.go   — per-DB pool map, discovery, TTL eviction, semaphore fan-out
internal/api/databases.go            — GET /instances/:id/databases, GET /instances/:id/databases/:dbname/metrics
web/src/pages/DatabaseAnalysisPage.tsx
web/src/hooks/useDatabaseMetrics.ts
web/src/types/models.ts              — add DBAnalysisMetrics type
```

Modified files:
```
internal/collector/collector.go      — add 2 interfaces (Queryer, DBCollector)
internal/config/config.go            — add 3 fields to InstanceConfig
internal/orchestrator/runner.go      — wire db_runner into existing Runner lifecycle
internal/api/router.go               — register 2 new endpoints
web/src/pages/ServerDetail.tsx       — make database names clickable links
```

---

## Interface Additions (internal/collector/collector.go)

Append after the existing `AlertEvaluator` interface — do not touch any existing types:

```go
// Queryer defines the minimal SQL execution interface.
// Both *pgx.Conn and *pgxpool.Pool satisfy this interface natively.
// Using Queryer instead of *pgx.Conn or *pgxpool.Pool enables mock injection
// in unit tests without spinning up a real PostgreSQL instance.
type Queryer interface {
    Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
    QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// DBCollector collects metrics for a single database.
// Dispatched once per discovered database per collection cycle by the orchestrator.
// Contrast with Collector, which operates at the instance level.
type DBCollector interface {
    Name() string
    Interval() time.Duration
    CollectDB(ctx context.Context, q Queryer, dbName string, ic InstanceContext) ([]MetricPoint, error)
}
```

---

## Config Changes (internal/config/config.go)

Add to InstanceConfig (do not rename or remove existing fields):

```go
// Per-database analysis settings (M7)
IncludeDatabases []string `koanf:"include_databases"` // glob patterns; empty = include all
ExcludeDatabases []string `koanf:"exclude_databases"` // glob patterns; empty = exclude none
MaxConcurrentDBs int      `koanf:"max_concurrent_dbs"` // default 5 if zero
```

---

## DB Runner (internal/orchestrator/db_runner.go)

New file — keeps per-DB concerns out of runner.go:

```go
package orchestrator

import (
    "context"
    "path/filepath" // for glob matching
    "sync"
    "time"

    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/ios9000/PGPulse_01/internal/collector"
    "github.com/ios9000/PGPulse_01/internal/config"
)

const (
    dbPoolMaxConns     = 2
    dbPoolTTLCycles    = 3
    defaultMaxConcurrent = 5
)

// DBRunner manages per-database connection pools and dispatches DBCollectors.
type DBRunner struct {
    instanceID   string
    baseDSN      string               // instance DSN (connects to postgres DB)
    cfg          config.InstanceConfig
    primaryPool  *pgxpool.Pool        // borrowed from Runner — used only for discovery query
    collectors   []collector.DBCollector

    mu           sync.Mutex
    pools        map[string]*pgxpool.Pool // key: dbname
    poolLastSeen map[string]int           // key: dbname, value: cycles since last seen

    semaphore    chan struct{}
    logger       *slog.Logger
    metricStore  collector.MetricStore
    evalFn       collector.AlertEvaluator
}

func NewDBRunner(
    instanceID, baseDSN string,
    cfg config.InstanceConfig,
    primaryPool *pgxpool.Pool,
    collectors []collector.DBCollector,
    store collector.MetricStore,
    eval collector.AlertEvaluator,
    logger *slog.Logger,
) *DBRunner {
    maxConc := cfg.MaxConcurrentDBs
    if maxConc <= 0 {
        maxConc = defaultMaxConcurrent
    }
    return &DBRunner{
        instanceID:   instanceID,
        baseDSN:      baseDSN,
        cfg:          cfg,
        primaryPool:  primaryPool,
        collectors:   collectors,
        pools:        make(map[string]*pgxpool.Pool),
        poolLastSeen: make(map[string]int),
        semaphore:    make(chan struct{}, maxConc),
        logger:       logger,
        metricStore:  store,
        evalFn:       eval,
    }
}

// Run executes one per-DB collection cycle. Called by Runner.
func (r *DBRunner) Run(ctx context.Context, ic collector.InstanceContext) {
    databases, err := r.discoverDatabases(ctx)
    if err != nil {
        r.logger.Warn("db discovery failed", "instance", r.instanceID, "err", err)
        return
    }

    r.evictStalePools(databases)

    discovered := int64(len(databases))
    var collected, errTimeout, errPerm, errOther int64

    var wg sync.WaitGroup
    var mu sync.Mutex

    for _, dbName := range databases {
        pool, err := r.getOrCreatePool(ctx, dbName)
        if err != nil {
            r.logger.Warn("failed to create db pool", "db", dbName, "err", err)
            errOther++
            continue
        }

        for _, col := range r.collectors {
            wg.Add(1)
            r.semaphore <- struct{}{} // acquire slot
            go func(db string, c collector.DBCollector, p *pgxpool.Pool) {
                defer wg.Done()
                defer func() { <-r.semaphore }() // release slot

                points, err := c.CollectDB(ctx, p, db, ic)
                if err != nil {
                    r.logger.Warn("db collector error",
                        "instance", r.instanceID, "db", db,
                        "collector", c.Name(), "err", err)
                    mu.Lock()
                    // classify error
                    switch classifyDBError(err) {
                    case "timeout":
                        errTimeout++
                    case "permission_denied":
                        errPerm++
                    default:
                        errOther++
                    }
                    mu.Unlock()
                    return
                }
                if len(points) > 0 {
                    if werr := r.metricStore.Write(ctx, points); werr != nil {
                        r.logger.Warn("metric write error", "err", werr)
                    }
                }
                mu.Lock()
                collected++
                mu.Unlock()
            }(dbName, col, pool)
        }
    }
    wg.Wait()

    // Emit internal telemetry
    now := time.Now()
    telemetry := []collector.MetricPoint{
        {InstanceID: r.instanceID, Metric: "pgpulse.agent.db.discovered", Value: float64(discovered), Timestamp: now},
        {InstanceID: r.instanceID, Metric: "pgpulse.agent.db.collected", Value: float64(collected), Timestamp: now},
        {InstanceID: r.instanceID, Metric: "pgpulse.agent.db.errors",
            Value: float64(errTimeout), Labels: map[string]string{"reason": "timeout"}, Timestamp: now},
        {InstanceID: r.instanceID, Metric: "pgpulse.agent.db.errors",
            Value: float64(errPerm), Labels: map[string]string{"reason": "permission_denied"}, Timestamp: now},
        {InstanceID: r.instanceID, Metric: "pgpulse.agent.db.errors",
            Value: float64(errOther), Labels: map[string]string{"reason": "other"}, Timestamp: now},
    }
    _ = r.metricStore.Write(ctx, telemetry)
}

// discoverDatabases queries pg_database and applies include/exclude filters.
func (r *DBRunner) discoverDatabases(ctx context.Context) ([]string, error) {
    rows, err := r.primaryPool.Query(ctx,
        `SELECT datname FROM pg_database
         WHERE NOT datistemplate AND datallowconn
         ORDER BY datname`)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var all []string
    for rows.Next() {
        var name string
        if err := rows.Scan(&name); err != nil {
            return nil, err
        }
        all = append(all, name)
    }
    if err := rows.Err(); err != nil {
        return nil, err
    }

    return r.applyFilters(all), nil
}

// applyFilters applies include/exclude glob patterns from config.
func (r *DBRunner) applyFilters(databases []string) []string {
    var result []string
    for _, db := range databases {
        if len(r.cfg.IncludeDatabases) > 0 && !matchesAny(db, r.cfg.IncludeDatabases) {
            continue
        }
        if len(r.cfg.ExcludeDatabases) > 0 && matchesAny(db, r.cfg.ExcludeDatabases) {
            continue
        }
        result = append(result, db)
    }
    return result
}

func matchesAny(name string, patterns []string) bool {
    for _, p := range patterns {
        if ok, _ := filepath.Match(p, name); ok {
            return true
        }
    }
    return false
}

// evictStalePools increments lastSeen counters and closes pools not seen in dbPoolTTLCycles.
func (r *DBRunner) evictStalePools(seen []string) {
    seenSet := make(map[string]bool, len(seen))
    for _, db := range seen {
        seenSet[db] = true
    }

    r.mu.Lock()
    defer r.mu.Unlock()

    for db := range r.pools {
        if seenSet[db] {
            r.poolLastSeen[db] = 0
        } else {
            r.poolLastSeen[db]++
            if r.poolLastSeen[db] >= dbPoolTTLCycles {
                r.logger.Info("evicting stale db pool", "db", db)
                r.pools[db].Close()
                delete(r.pools, db)
                delete(r.poolLastSeen, db)
            }
        }
    }
}

// getOrCreatePool returns existing pool or creates a new one for the given database.
func (r *DBRunner) getOrCreatePool(ctx context.Context, dbName string) (*pgxpool.Pool, error) {
    r.mu.Lock()
    defer r.mu.Unlock()

    if p, ok := r.pools[dbName]; ok {
        return p, nil
    }

    dsn := substituteDBName(r.baseDSN, dbName)
    cfg, err := pgxpool.ParseConfig(dsn)
    if err != nil {
        return nil, err
    }
    cfg.MaxConns = dbPoolMaxConns

    pool, err := pgxpool.NewWithConfig(ctx, cfg)
    if err != nil {
        return nil, err
    }

    r.pools[dbName] = pool
    r.poolLastSeen[dbName] = 0
    return pool, nil
}

// substituteDBName replaces the dbname value in a DSN string.
// Handles both key=value DSN format and URL format.
func substituteDBName(dsn, dbName string) string {
    // key=value format: replace dbname=xxx
    // Implementation: split on spaces, find dbname= token, replace value
    // ... (see implementation notes below)
}

// Close closes all per-DB pools. Called when the instance runner shuts down.
func (r *DBRunner) Close() {
    r.mu.Lock()
    defer r.mu.Unlock()
    for db, p := range r.pools {
        p.Close()
        delete(r.pools, db)
    }
}
```

### substituteDBName implementation

```go
func substituteDBName(dsn, newDB string) string {
    // Handle URL format: postgres://user:pass@host/dbname?options
    if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
        u, err := url.Parse(dsn)
        if err == nil {
            u.Path = "/" + newDB
            return u.String()
        }
    }
    // Handle key=value format
    parts := strings.Fields(dsn)
    found := false
    for i, p := range parts {
        if strings.HasPrefix(p, "dbname=") {
            parts[i] = "dbname=" + newDB
            found = true
            break
        }
    }
    if !found {
        parts = append(parts, "dbname="+newDB)
    }
    return strings.Join(parts, " ")
}
```

### Integration into runner.go

In `internal/orchestrator/runner.go`, add to the Runner struct:
```go
dbRunner *DBRunner
```

In the Runner's start/init function, after the primary pool is established:
```go
r.dbRunner = NewDBRunner(
    cfg.ID, cfg.DSN, cfg, r.pool,
    []collector.DBCollector{collector.NewDatabaseCollector()},
    store, eval, logger,
)
```

In the Runner's main collection loop, add a separate ticker at 5-minute interval:
```go
dbTicker := time.NewTicker(5 * time.Minute)
// ... in select:
case <-dbTicker.C:
    r.dbRunner.Run(ctx, ic)
```

On Runner shutdown: `r.dbRunner.Close()`

---

## Database Collector (internal/collector/database.go)

```go
package collector

import (
    "context"
    "fmt"
    "time"
)

const dbStatementTimeout = "60s"

// DatabaseCollector implements DBCollector for per-database analysis.
// Ports analiz_db.php Q2–Q18.
type DatabaseCollector struct{}

func NewDatabaseCollector() *DatabaseCollector { return &DatabaseCollector{} }

func (c *DatabaseCollector) Name() string                { return "database" }
func (c *DatabaseCollector) Interval() time.Duration     { return 5 * time.Minute }

func (c *DatabaseCollector) CollectDB(ctx context.Context, q Queryer, dbName string, ic InstanceContext) ([]MetricPoint, error) {
    // Set statement timeout for this session
    if _, err := q.Query(ctx, "SET LOCAL statement_timeout = '"+dbStatementTimeout+"'", nil); err != nil {
        return nil, fmt.Errorf("set timeout: %w", err)
    }

    var points []MetricPoint
    var errs []error

    appendPoints := func(fn func(context.Context, Queryer, string) ([]MetricPoint, error)) {
        pts, err := fn(ctx, q, dbName)
        if err != nil {
            errs = append(errs, err)
            return
        }
        points = append(points, pts...)
    }

    appendPoints(collectLargeObjects)
    appendPoints(collectFunctionStats)
    appendPoints(collectSequences)
    appendPoints(collectSchemaSizes)
    appendPoints(collectUnloggedObjects)
    appendPoints(collectTableSizes)
    appendPoints(collectBloat)
    appendPoints(collectCatalogSizes)
    appendPoints(collectTableCacheHit)
    appendPoints(collectAutovacuumOptions)
    appendPoints(collectVacuumNeed)
    appendPoints(collectIndexUsage)
    appendPoints(collectUnusedIndexes)
    appendPoints(collectToastSizes)
    appendPoints(collectPartitions)
    appendPoints(collectLargeObjectSizes)

    // Partial success: return points collected so far even if some functions errored.
    // Individual function errors are non-fatal — log them in CollectDB's caller.
    if len(errs) > 0 {
        // Return combined error for caller to log, but also return the points we have
        return points, fmt.Errorf("%d sub-collectors failed: %v", len(errs), errs[0])
    }
    return points, nil
}
```

### Key SQL for collectBloat (most complex — Q12)

The PGAM bloat estimation CTE using pg_stats is the most complex query. Implement it faithfully:

```go
func collectBloat(ctx context.Context, q Queryer, dbName string) ([]MetricPoint, error) {
    const sql = `
    WITH constants AS (
        SELECT current_setting('block_size')::numeric AS bs, 23 AS hdr, 4 AS ma
    ),
    bloat_info AS (
        SELECT
            schemaname, tablename, reltuples, relpages, otta,
            ROUND((CASE WHEN otta=0 THEN 0.0 ELSE sml.relpages::float/otta END)::numeric,1) AS tbloat,
            GREATEST(relpages::bigint - otta, 0) AS wastedpages,
            GREATEST(bs*(relpages::bigint-otta),0) AS wastedbytes,
            iname, ituples, ipages, iotta,
            ROUND((CASE WHEN iotta=0 OR ipages=0 THEN 0.0 ELSE ipages::float/iotta END)::numeric,1) AS ibloat,
            GREATEST(ipages::bigint-iotta,0) AS wastedipages,
            GREATEST(bs*(ipages::bigint-iotta),0) AS wastedibytes
        FROM (
            SELECT
                schemaname, tablename, cc.reltuples, cc.relpages,
                CEIL((cc.reltuples*((datahdr+ma-
                    (CASE WHEN datahdr%ma=0 THEN ma ELSE datahdr%ma END))+nullhdr2+4))/(bs-20::float)) AS otta,
                c2.relname AS iname, c2.reltuples AS ituples, c2.relpages AS ipages,
                CEIL((c2.reltuples*(datahdr-12))/(bs-20::float)) AS iotta
            FROM (
                SELECT
                    ma, bs, schemaname, tablename,
                    (datawidth+(hdr+ma-(CASE WHEN hdr%ma=0 THEN ma ELSE hdr%ma END)))::numeric AS datahdr,
                    (maxfracsum*(nullhdr+ma-(CASE WHEN nullhdr%ma=0 THEN ma ELSE nullhdr%ma END))) AS nullhdr2
                FROM (
                    SELECT
                        schemaname, tablename, hdr, ma, bs,
                        SUM((1-stanullfrac)*stawidth) AS datawidth,
                        MAX(stanullfrac) AS maxfracsum,
                        hdr+(1+(COUNT(CASE WHEN stanullfrac>0 THEN 1 END)*8)/bitlength) AS nullhdr
                    FROM pg_stats CROSS JOIN constants
                    LEFT JOIN (SELECT 8 AS bitlength) bl ON true
                    GROUP BY 1,2,3,4,5
                ) AS foo
            ) AS rs
            JOIN pg_class cc ON cc.relname=rs.tablename
            JOIN pg_namespace nn ON cc.relnamespace=nn.oid AND nn.nspname=rs.schemaname AND nn.nspname<>'information_schema'
            LEFT JOIN pg_index i ON indrelid=cc.oid
            LEFT JOIN pg_class c2 ON c2.oid=i.indexrelid
        ) AS sml
    )
    SELECT schemaname, tablename,
           wastedbytes, tbloat,
           iname, wastedibytes, ibloat
    FROM bloat_info
    WHERE tbloat > 1.5 OR ibloat > 1.5
    ORDER BY wastedbytes DESC NULLS LAST
    LIMIT 100`

    // ... execute and convert to MetricPoints
}
```

### Key SQL for collectVacuumNeed (Q16)

```go
const vacuumSQL = `
SELECT schemaname, relname,
       n_dead_tup, n_live_tup,
       ROUND(n_dead_tup::numeric / GREATEST(n_live_tup + n_dead_tup, 1) * 100, 2) AS dead_pct,
       last_autovacuum, last_vacuum, last_autoanalyze, last_analyze,
       n_mod_since_analyze,
       EXTRACT(EPOCH FROM (now() - last_autovacuum))::bigint AS autovacuum_age_sec,
       EXTRACT(EPOCH FROM (now() - last_autoanalyze))::bigint AS autoanalyze_age_sec
FROM pg_stat_user_tables
ORDER BY n_dead_tup DESC
LIMIT 50`
```

---

## API Design (internal/api/databases.go)

```go
// GET /api/v1/instances/:id/databases
// Returns list of discovered databases with summary metrics
type databaseSummary struct {
    Name           string  `json:"name"`
    SizeBytes      int64   `json:"size_bytes"`
    TableCount     int64   `json:"table_count"`
    BloatRatio     float64 `json:"bloat_ratio"`      // worst table bloat in this DB
    DeadTuples     int64   `json:"dead_tuples"`       // sum across all tables
    UnusedIndexes  int64   `json:"unused_indexes"`
    AgentCollected bool    `json:"agent_collected"`   // was per-DB data collected?
    LastCollected  string  `json:"last_collected"`    // ISO timestamp
}

// GET /api/v1/instances/:id/databases/:dbname/metrics
// Returns full per-DB analysis metrics (latest collection)
type databaseMetrics struct {
    DatabaseName string          `json:"database_name"`
    CollectedAt  string          `json:"collected_at"`
    Tables       []tableMetric   `json:"tables"`
    Indexes      []indexMetric   `json:"indexes"`
    Schemas      []schemaMetric  `json:"schemas"`
    Vacuum       []vacuumMetric  `json:"vacuum"`
    Bloat        []bloatMetric   `json:"bloat"`
    Sequences    []sequenceMetric `json:"sequences"`
    LargeObjects *largeObjMetric `json:"large_objects"`
    Unlogged     []unloggedMetric `json:"unlogged"`
    Functions    []functionMetric `json:"functions"`
    Catalogs     []catalogMetric  `json:"catalogs"`
}
```

Both endpoints require JWT auth (any authenticated user, PermViewAll).
Must be registered in `internal/api/router.go` under the auth middleware group.

---

## Frontend Design

### DatabaseAnalysisPage.tsx

```
Route: /instances/:instanceId/databases/:dbname

┌─ Per-Database Analysis: app_db ──────────────────────────────────┐
│  Instance: Production Primary   Last collected: 2 min ago  [↻]   │
│                                                                    │
│  ▾ Tables (top by size)                                           │
│    Schema | Table | Size | Bloat | Rows | Toast                  │
│    public | orders | 42GB | 2.3x  | 12M  | 1.2GB                 │
│    ...                                                             │
│                                                                    │
│  ▾ Vacuum Health                                                   │
│    Schema | Table | Dead Tuples | Dead% | Last Autovacuum | ...   │
│    🔴 public | events | 4,200,000 | 42% | 3 days ago | ...       │
│                                                                    │
│  ▾ Index Usage                                                     │
│  ▾ Schema Sizes  [ECharts horizontal bar]                          │
│  ▾ Large Objects  [⚠ yellow if any]                               │
│  ▾ Unlogged Objects  [🔴 red if any]                              │
│  ▾ Sequences                                                       │
│  ▾ Functions  [hidden if track_functions=none]                    │
└────────────────────────────────────────────────────────────────────┘
```

### useDatabaseMetrics.ts

```typescript
export function useDatabaseList(instanceId: string) {
  return useQuery({
    queryKey: ['instances', instanceId, 'databases'],
    queryFn: () => apiClient.get(`/instances/${instanceId}/databases`),
    refetchInterval: 5 * 60 * 1000, // 5 min, matching collection interval
  });
}

export function useDatabaseMetrics(instanceId: string, dbName: string) {
  return useQuery({
    queryKey: ['instances', instanceId, 'databases', dbName, 'metrics'],
    queryFn: () => apiClient.get(`/instances/${instanceId}/databases/${dbName}/metrics`),
    refetchInterval: 5 * 60 * 1000,
  });
}
```

---

## Test Coverage Required

### Unit tests (mock Queryer — no real DB)

`internal/collector/database_test.go`:
- `TestDatabaseCollector_Name` — returns "database"
- `TestDatabaseCollector_Interval` — returns 5 * time.Minute
- `TestCollectVacuumNeed_MockQuery` — feed mock rows, assert MetricPoints
- `TestCollectBloat_MockQuery` — feed mock rows, assert table + index bloat metrics
- `TestCollectIndexUsage_MockQuery`
- `TestCollectUnusedIndexes_MockQuery`
- `TestCollectSchemaSizes_MockQuery`
- `TestCollectVacuumNeed_PartialErrors` — one function errors, rest succeed, points returned

`internal/orchestrator/db_runner_test.go`:
- `TestDBRunner_ApplyFilters_IncludeOnly` — include=["prod_*"], verify only matching DBs pass
- `TestDBRunner_ApplyFilters_ExcludeOnly` — exclude=["*_test"], verify filtered
- `TestDBRunner_ApplyFilters_Combined` — both filters applied in order
- `TestDBRunner_SubstituteDBName_KeyValue` — DSN substitution for key=value format
- `TestDBRunner_SubstituteDBName_URL` — DSN substitution for URL format
- `TestDBRunner_EvictStalePools` — pool evicted after 3 cycles of not being seen

### Mock Queryer for tests

```go
// internal/collector/testutil_test.go (or mock_queryer_test.go)
type mockQueryer struct {
    rows [][]any
    err  error
}

func (m *mockQueryer) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
    // return mock rows
}

func (m *mockQueryer) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
    // return mock single row
}
```

---

## Architecture Decisions

| # | Decision | Rationale |
|---|----------|-----------|
| D-M7-01 | Option C: new DBCollector interface, parallel dispatch | ISP: global and per-DB metrics are different domains; frozen Collector interface stays frozen |
| D-M7-02 | Queryer abstraction over *pgxpool.Pool | Enables mock injection; both pgx.Conn and pgxpool.Pool satisfy it natively |
| D-M7-03 | Dynamic pool map with TTL eviction (3 cycles) | Prevents connection leaks when databases are dropped or excluded by config changes |
| D-M7-04 | Semaphore fan-out (MaxConcurrentDBs=5 default) | Storm protection: prevents overwhelming PG with N*M concurrent connections |
| D-M7-05 | Partial success in CollectDB | One failing sub-collector (e.g., bloat on huge DB) should not block vacuum metrics |
| D-M7-06 | Internal telemetry MetricPoints | Orchestrator health is observable from the same TimescaleDB store as all other metrics |
| D-M7-07 | Hybrid discovery: system catalog + include/exclude glob | Works zero-config; DevOps has override levers when needed |
| D-M7-08 | 5-minute interval | Bloat estimation CTE is expensive; don't hammer DB; matches PGAM's manual refresh model |
| D-M7-09 | Per-DB pool MaxConns=2 | Analysis queries, not hot path; prevent pool explosion on instances with many databases |
| D-M7-10 | Logical replication (Q41) still deferred | This architecture enables it cleanly — DBCollector interface is the right hook |
