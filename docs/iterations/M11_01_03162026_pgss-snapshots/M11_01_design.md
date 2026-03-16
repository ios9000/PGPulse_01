# M11_01 Design — PGSS Snapshots, Diff Engine, Query Insights API + Bug Fixes

**Iteration:** M11_01
**Date:** 2026-03-16
**Pattern reference:** `internal/settings/` (snapshot + diff + store)

---

## 1. Migration 015 — `pgss_snapshots` Tables

File: `internal/storage/migrations/015_pgss_snapshots.sql`

```sql
-- Snapshot metadata
CREATE TABLE IF NOT EXISTS pgss_snapshots (
    id              BIGSERIAL PRIMARY KEY,
    instance_id     TEXT NOT NULL,
    captured_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    pg_version      INT,
    stats_reset     TIMESTAMPTZ,
    total_statements INT NOT NULL DEFAULT 0,
    total_calls     BIGINT NOT NULL DEFAULT 0,
    total_exec_time DOUBLE PRECISION NOT NULL DEFAULT 0,
    CONSTRAINT uq_pgss_snapshot UNIQUE (instance_id, captured_at)
);

CREATE INDEX IF NOT EXISTS idx_pgss_snapshots_instance_time
    ON pgss_snapshots (instance_id, captured_at DESC);

-- One row per query per snapshot
CREATE TABLE IF NOT EXISTS pgss_snapshot_entries (
    snapshot_id       BIGINT NOT NULL REFERENCES pgss_snapshots(id) ON DELETE CASCADE,
    queryid           BIGINT NOT NULL,
    userid            OID,
    dbid              OID,
    query             TEXT,
    calls             BIGINT,
    total_exec_time   DOUBLE PRECISION,
    total_plan_time   DOUBLE PRECISION,     -- PG 13+ only, NULL on PG ≤12
    rows              BIGINT,
    shared_blks_hit   BIGINT,
    shared_blks_read  BIGINT,
    shared_blks_dirtied BIGINT,
    shared_blks_written BIGINT,
    local_blks_hit    BIGINT,
    local_blks_read   BIGINT,
    temp_blks_read    BIGINT,
    temp_blks_written BIGINT,
    blk_read_time     DOUBLE PRECISION,
    blk_write_time    DOUBLE PRECISION,
    wal_records       BIGINT,               -- PG 13+ only
    wal_fpi           BIGINT,               -- PG 13+ only
    wal_bytes         NUMERIC,              -- PG 13+ only
    mean_exec_time    DOUBLE PRECISION,     -- PG 13+ only
    min_exec_time     DOUBLE PRECISION,     -- PG 13+ only
    max_exec_time     DOUBLE PRECISION,     -- PG 13+ only
    stddev_exec_time  DOUBLE PRECISION,     -- PG 13+ only
    PRIMARY KEY (snapshot_id, queryid, dbid, userid)
);

CREATE INDEX IF NOT EXISTS idx_pgss_entries_queryid
    ON pgss_snapshot_entries (queryid);
```

**Note:** No TimescaleDB hypertable — these are snapshot tables, not time-series. Retention is managed by application-level cleanup (delete snapshots + CASCADE to entries).

---

## 2. Package: `internal/statements/`

### 2.1 Types (`types.go`)

```go
package statements

import "time"

// Snapshot is the metadata for one PGSS capture
type Snapshot struct {
    ID              int64      `json:"id"`
    InstanceID      string     `json:"instance_id"`
    CapturedAt      time.Time  `json:"captured_at"`
    PGVersion       int        `json:"pg_version"`
    StatsReset      *time.Time `json:"stats_reset,omitempty"`
    TotalStatements int        `json:"total_statements"`
    TotalCalls      int64      `json:"total_calls"`
    TotalExecTime   float64    `json:"total_exec_time_ms"`
}

// SnapshotEntry is one query's counters at a point in time
type SnapshotEntry struct {
    SnapshotID      int64    `json:"snapshot_id"`
    QueryID         int64    `json:"queryid"`
    UserID          uint32   `json:"userid"`
    DbID            uint32   `json:"dbid"`
    Query           string   `json:"query"`
    Calls           int64    `json:"calls"`
    TotalExecTime   float64  `json:"total_exec_time_ms"`
    TotalPlanTime   *float64 `json:"total_plan_time_ms,omitempty"` // PG 13+
    Rows            int64    `json:"rows"`
    SharedBlksHit   int64    `json:"shared_blks_hit"`
    SharedBlksRead  int64    `json:"shared_blks_read"`
    SharedBlksDirtied int64  `json:"shared_blks_dirtied"`
    SharedBlksWritten int64  `json:"shared_blks_written"`
    LocalBlksHit    int64    `json:"local_blks_hit"`
    LocalBlksRead   int64    `json:"local_blks_read"`
    TempBlksRead    int64    `json:"temp_blks_read"`
    TempBlksWritten int64    `json:"temp_blks_written"`
    BlkReadTime     float64  `json:"blk_read_time_ms"`
    BlkWriteTime    float64  `json:"blk_write_time_ms"`
    WALRecords      *int64   `json:"wal_records,omitempty"`       // PG 13+
    WALFpi          *int64   `json:"wal_fpi,omitempty"`           // PG 13+
    WALBytes        *float64 `json:"wal_bytes,omitempty"`         // PG 13+
    MeanExecTime    *float64 `json:"mean_exec_time_ms,omitempty"` // PG 13+
    MinExecTime     *float64 `json:"min_exec_time_ms,omitempty"`  // PG 13+
    MaxExecTime     *float64 `json:"max_exec_time_ms,omitempty"`  // PG 13+
    StddevExecTime  *float64 `json:"stddev_exec_time_ms,omitempty"` // PG 13+
}

// DiffResult is the output of comparing two snapshots
type DiffResult struct {
    FromSnapshot    Snapshot     `json:"from_snapshot"`
    ToSnapshot      Snapshot     `json:"to_snapshot"`
    StatsResetWarning bool      `json:"stats_reset_warning"`
    Duration        time.Duration `json:"duration"`
    TotalCallsDelta int64       `json:"total_calls_delta"`
    TotalExecTimeDelta float64  `json:"total_exec_time_delta_ms"`
    Entries         []DiffEntry  `json:"entries"`
    NewQueries      []DiffEntry  `json:"new_queries"`
    EvictedQueries  []DiffEntry  `json:"evicted_queries"`
    TotalEntries    int          `json:"total_entries"` // for pagination
}

// DiffEntry is per-query delta between two snapshots
type DiffEntry struct {
    QueryID          int64   `json:"queryid"`
    UserID           uint32  `json:"userid"`
    DbID             uint32  `json:"dbid"`
    Query            string  `json:"query"`
    DatabaseName     string  `json:"database_name,omitempty"`
    UserName         string  `json:"user_name,omitempty"`
    CallsDelta       int64   `json:"calls_delta"`
    ExecTimeDelta    float64 `json:"exec_time_delta_ms"`
    PlanTimeDelta    *float64 `json:"plan_time_delta_ms,omitempty"`
    RowsDelta        int64   `json:"rows_delta"`
    SharedBlksReadDelta  int64   `json:"shared_blks_read_delta"`
    SharedBlksHitDelta   int64   `json:"shared_blks_hit_delta"`
    TempBlksReadDelta    int64   `json:"temp_blks_read_delta"`
    TempBlksWrittenDelta int64   `json:"temp_blks_written_delta"`
    BlkReadTimeDelta     float64 `json:"blk_read_time_delta_ms"`
    BlkWriteTimeDelta    float64 `json:"blk_write_time_delta_ms"`
    WALBytesDelta        *float64 `json:"wal_bytes_delta,omitempty"`
    // Derived fields
    AvgExecTimePerCall float64 `json:"avg_exec_time_per_call_ms"`
    IOTimePct          float64 `json:"io_time_pct"`
    CPUTimeDelta       float64 `json:"cpu_time_delta_ms"`
    SharedHitRatio     float64 `json:"shared_hit_ratio_pct"`
}

// QueryInsight is the time-series view for a single queryid
type QueryInsight struct {
    QueryID      int64              `json:"queryid"`
    Query        string             `json:"query"`
    DatabaseName string             `json:"database_name"`
    UserName     string             `json:"user_name"`
    FirstSeen    time.Time          `json:"first_seen"`
    Points       []QueryInsightPoint `json:"points"`
}

// QueryInsightPoint is one interval's delta for a query
type QueryInsightPoint struct {
    CapturedAt    time.Time `json:"captured_at"`
    CallsDelta    int64     `json:"calls_delta"`
    ExecTimeDelta float64   `json:"exec_time_delta_ms"`
    RowsDelta     int64     `json:"rows_delta"`
    AvgExecTime   float64   `json:"avg_exec_time_ms"`
    SharedHitRatio float64  `json:"shared_hit_ratio_pct"`
}

// WorkloadReport is the full structured report
type WorkloadReport struct {
    InstanceID     string    `json:"instance_id"`
    FromTime       time.Time `json:"from_time"`
    ToTime         time.Time `json:"to_time"`
    Duration       string    `json:"duration"`
    StatsResetWarning bool   `json:"stats_reset_warning"`
    Summary        ReportSummary    `json:"summary"`
    TopByExecTime  []DiffEntry      `json:"top_by_exec_time"`
    TopByCalls     []DiffEntry      `json:"top_by_calls"`
    TopByRows      []DiffEntry      `json:"top_by_rows"`
    TopByIOReads   []DiffEntry      `json:"top_by_io_reads"`
    TopByAvgTime   []DiffEntry      `json:"top_by_avg_time"`
    NewQueries     []DiffEntry      `json:"new_queries"`
    EvictedQueries []DiffEntry      `json:"evicted_queries"`
}

type ReportSummary struct {
    TotalQueries     int     `json:"total_queries"`
    TotalCallsDelta  int64   `json:"total_calls_delta"`
    TotalExecTimeDelta float64 `json:"total_exec_time_delta_ms"`
    TotalRowsDelta   int64   `json:"total_rows_delta"`
    UniqueQueries    int     `json:"unique_queries"`
    NewQueries       int     `json:"new_queries"`
    EvictedQueries   int     `json:"evicted_queries"`
}

// ListOptions for paginated queries
type SnapshotListOptions struct {
    Limit  int
    Offset int
    From   *time.Time
    To     *time.Time
}

type DiffOptions struct {
    SortBy string // total_exec_time, calls, rows, shared_blks_read, avg_exec_time
    Limit  int
    Offset int
}
```

### 2.2 Store Interface + PGStore (`store.go`, `pgstore.go`)

```go
// store.go
type SnapshotStore interface {
    WriteSnapshot(ctx context.Context, snap Snapshot, entries []SnapshotEntry) (int64, error)
    GetSnapshot(ctx context.Context, id int64) (*Snapshot, error)
    GetSnapshotEntries(ctx context.Context, snapshotID int64, limit, offset int) ([]SnapshotEntry, int, error)
    ListSnapshots(ctx context.Context, instanceID string, opts SnapshotListOptions) ([]Snapshot, int, error)
    GetLatestSnapshots(ctx context.Context, instanceID string, n int) ([]Snapshot, error)
    GetEntriesForQuery(ctx context.Context, instanceID string, queryID int64, from, to time.Time) ([]SnapshotEntry, []Snapshot, error)
    CleanOld(ctx context.Context, olderThan time.Time) error
}
```

**PGStore implementation notes:**
- `WriteSnapshot`: INSERT snapshot row, then `pgx.CopyFrom` for entries when len > 100, else batch INSERT.
- `GetEntriesForQuery`: JOIN pgss_snapshot_entries → pgss_snapshots WHERE queryid = $1 AND instance_id = $2 AND captured_at BETWEEN $3 AND $4, ORDER BY captured_at ASC. This powers the query insights timeline.
- `CleanOld`: DELETE FROM pgss_snapshots WHERE captured_at < $1 (CASCADE handles entries).

**NullSnapshotStore** (`nullstore.go`): All methods return nil/empty — safe for live mode.

### 2.3 Snapshot Capturer (`capture.go`)

```go
// Mirrors settings/snapshot.go pattern
type SnapshotCapturer struct {
    store       SnapshotStore
    connFor     func(instanceID string) (*pgxpool.Pool, error)  // from orchestrator
    lister      InstanceLister                                   // list active instances
    interval    time.Duration
    onStartup   bool
    logger      *slog.Logger
    stopCh      chan struct{}
    doneCh      chan struct{}
}

func NewSnapshotCapturer(store, connFor, lister, interval, onStartup, logger) *SnapshotCapturer
func (c *SnapshotCapturer) Start(ctx context.Context)
func (c *SnapshotCapturer) Stop()
func (c *SnapshotCapturer) CaptureInstance(ctx context.Context, instanceID string) (*Snapshot, error)
```

**CaptureInstance flow:**
1. Get pool via `connFor(instanceID)`
2. Detect PG version (reuse `version.Detect()`)
3. Check `pg_stat_statements` extension exists (reuse existing check pattern)
4. Read `stats_reset` from `pg_stat_statements_info` (PG 14+, else NULL)
5. Execute version-gated SELECT from `pg_stat_statements` (see §2.4)
6. Build Snapshot + []SnapshotEntry
7. Write to store

### 2.4 Version-Gated SQL

**PG ≤12 query:**
```sql
SET statement_timeout = 30000;
SELECT
    s.queryid, s.userid, s.dbid,
    left(s.query, 8192) AS query,
    s.calls, s.total_time AS total_exec_time,
    s.rows,
    s.shared_blks_hit, s.shared_blks_read,
    s.shared_blks_dirtied, s.shared_blks_written,
    s.local_blks_hit, s.local_blks_read,
    s.temp_blks_read, s.temp_blks_written,
    s.blk_read_time, s.blk_write_time,
    -- NULLs for PG 13+ fields
    NULL::double precision AS total_plan_time,
    NULL::bigint AS wal_records, NULL::bigint AS wal_fpi, NULL::numeric AS wal_bytes,
    NULL::double precision AS mean_exec_time, NULL::double precision AS min_exec_time,
    NULL::double precision AS max_exec_time, NULL::double precision AS stddev_exec_time
FROM pg_stat_statements s
WHERE s.queryid IS NOT NULL
```

**PG 13+ query:**
```sql
SET statement_timeout = 30000;
SELECT
    s.queryid, s.userid, s.dbid,
    left(s.query, 8192) AS query,
    s.calls, s.total_exec_time, s.total_plan_time,
    s.rows,
    s.shared_blks_hit, s.shared_blks_read,
    s.shared_blks_dirtied, s.shared_blks_written,
    s.local_blks_hit, s.local_blks_read,
    s.temp_blks_read, s.temp_blks_written,
    s.blk_read_time, s.blk_write_time,
    s.wal_records, s.wal_fpi, s.wal_bytes,
    s.mean_exec_time, s.min_exec_time,
    s.max_exec_time, s.stddev_exec_time
FROM pg_stat_statements s
WHERE s.queryid IS NOT NULL
```

**Use `version.Gate`** with `SQLVariant` for PG 12 vs 13+ selection.

### 2.5 Diff Engine (`diff.go`)

```go
func ComputeDiff(from, to Snapshot, fromEntries, toEntries []SnapshotEntry, opts DiffOptions) *DiffResult

func computeEntryDiff(from, to SnapshotEntry) DiffEntry  // single query delta
func derivedFields(d *DiffEntry)                          // avg_exec_time, io_pct, cpu_time, hit_ratio
func sortEntries(entries []DiffEntry, sortBy string)       // sort by requested column
```

**Diff algorithm:**
1. Build map: `key(queryid, dbid, userid) → SnapshotEntry` for "from" entries
2. Iterate "to" entries:
   - If key exists in "from" map → compute delta, add to `entries`, remove from map
   - If key not in "from" map → add to `new_queries`
3. Remaining keys in "from" map → `evicted_queries`
4. Check `from.StatsReset != to.StatsReset` → set `StatsResetWarning`
5. Compute derived fields for all entries
6. Sort by `opts.SortBy` desc
7. Apply limit/offset pagination

**Derived fields:**
- `AvgExecTimePerCall = ExecTimeDelta / CallsDelta` (guard div-by-zero)
- `IOTimePct = (BlkReadTimeDelta + BlkWriteTimeDelta) / ExecTimeDelta * 100`
- `CPUTimeDelta = ExecTimeDelta - BlkReadTimeDelta - BlkWriteTimeDelta`
- `SharedHitRatio = SharedBlksHitDelta / (SharedBlksHitDelta + SharedBlksReadDelta) * 100`

### 2.6 Query Insights (`insights.go`)

```go
func BuildQueryInsight(
    instanceID string,
    queryID int64,
    entries []SnapshotEntry,
    snapshots []Snapshot,
    dbNames map[uint32]string,
    userNames map[uint32]string,
) *QueryInsight
```

- Takes entries for a single queryid from store (sorted by captured_at)
- Computes inter-snapshot deltas (entry[i] − entry[i-1] for cumulative counters)
- Handles stats_reset: if counters decrease, use the "to" value as the delta (fresh start)
- Returns QueryInsight with Points array for time-series rendering

### 2.7 Workload Report (`report.go`)

```go
func GenerateReport(diff *DiffResult, topN int) *WorkloadReport
```

- Takes a DiffResult and reshapes it into report sections
- Each section sorted by its primary metric (exec_time, calls, rows, blks_read, avg_time)
- Top-N applied per section (configurable, default 50)
- Summary computed from full entry set before pagination

### 2.8 DB/User Name Resolution

The diff and insights endpoints need human-readable database and user names, not just OIDs. The capturer should resolve them at capture time.

**Add to SnapshotEntry:** `DatabaseName string`, `UserName string` (populated during capture via pg_database and pg_roles lookups).

Alternative: store dbid/userid only, resolve at query time. **Decision: resolve at capture time and store as columns.** Rationale: DB/user can be dropped between capture and query time; capture-time resolution preserves accuracy. Add `database_name TEXT` and `user_name TEXT` columns to `pgss_snapshot_entries`.

---

## 3. API Handlers (`internal/api/handler_snapshots.go`)

### 3.1 Endpoint Summary

| Method | Path | Handler | Auth | Description |
|--------|------|---------|------|-------------|
| GET | /instances/{id}/snapshots | handleListSnapshots | viewer+ | List snapshots with pagination + time filter |
| GET | /instances/{id}/snapshots/{snapId} | handleGetSnapshot | viewer+ | Snapshot detail with paginated entries |
| GET | /instances/{id}/snapshots/diff | handleSnapshotDiff | viewer+ | Diff between two snapshots |
| GET | /instances/{id}/snapshots/latest-diff | handleLatestDiff | viewer+ | Diff between last two snapshots |
| GET | /instances/{id}/query-insights/{queryid} | handleQueryInsights | viewer+ | Per-query time-series |
| GET | /instances/{id}/workload-report | handleWorkloadReport | viewer+ | Full structured report |
| POST | /instances/{id}/snapshots/capture | handleManualSnapshotCapture | instance_management | Trigger manual capture |

### 3.2 Query Parameters

**`handleListSnapshots`:**
- `?limit=20&offset=0` — pagination
- `?from=2026-03-15T00:00:00Z&to=2026-03-16T00:00:00Z` — time range

**`handleGetSnapshot`:**
- `?limit=50&offset=0` — paginate entries
- `?sort=total_exec_time` — sort entries

**`handleSnapshotDiff`:**
- `?from={snapId}&to={snapId}` — by snapshot IDs
- `?from_time=...&to_time=...` — by time range (finds closest snapshots)
- `?sort=total_exec_time&limit=50&offset=0`

**`handleQueryInsights`:**
- `?from=...&to=...` — time range (default: last 24h)

**`handleWorkloadReport`:**
- `?from={snapId}&to={snapId}` — by snapshot IDs
- `?from_time=...&to_time=...` — by time range
- `?top_n=50` — entries per section

### 3.3 Route Registration

In `server.go`, add within the instances group:

```go
r.Route("/instances/{id}/snapshots", func(r chi.Router) {
    r.Get("/", h.handleListSnapshots)
    r.Get("/diff", h.handleSnapshotDiff)
    r.Get("/latest-diff", h.handleLatestDiff)
    r.Post("/capture", h.handleManualSnapshotCapture)  // instance_management
    r.Get("/{snapId}", h.handleGetSnapshot)
})
r.Get("/instances/{id}/query-insights/{queryid}", h.handleQueryInsights)
r.Get("/instances/{id}/workload-report", h.handleWorkloadReport)
```

**Important:** `/diff` and `/latest-diff` routes must be registered BEFORE `/{snapId}` to avoid chi treating "diff" as a snapId.

---

## 4. Configuration

Add to `internal/config/config.go`:

```go
type StatementSnapshotsConfig struct {
    Enabled         bool          `koanf:"enabled"`
    Interval        time.Duration `koanf:"interval"`
    RetentionDays   int           `koanf:"retention_days"`
    CaptureOnStartup bool         `koanf:"capture_on_startup"`
    TopN            int           `koanf:"top_n"`
}
```

Default values in `Load()`:
```go
cfg.StatementSnapshots.Interval = 30 * time.Minute
cfg.StatementSnapshots.RetentionDays = 30
cfg.StatementSnapshots.TopN = 50
```

Add `StatementSnapshots StatementSnapshotsConfig` field to the top-level `Config` struct.

---

## 5. Wiring in `main.go`

Guard: `cfg.StatementSnapshots.Enabled && persistentStore != nil`

```go
if cfg.StatementSnapshots.Enabled && persistentStore != nil {
    snapshotStore := statements.NewPGSnapshotStore(pgPool)
    capturer := statements.NewSnapshotCapturer(
        snapshotStore,
        orchestrator.ConnFor,
        ml.NewDBInstanceLister(pgPool),
        cfg.StatementSnapshots.Interval,
        cfg.StatementSnapshots.CaptureOnStartup,
        logger,
    )
    capturer.Start(ctx)
    defer capturer.Stop()
    
    // Pass to API server
    apiServer.SetSnapshotStore(snapshotStore)
    apiServer.SetSnapshotCapturer(capturer)
}
```

---

## 6. Bug Fixes

### 6.1 Remove Debug Log (`main.go`)

Remove the line: `logger.Info("remediation config", ...)` (added during M10_01 troubleshooting).

### 6.2 `wastedibytes` Float→Int Fix (`internal/collector/database.go`)

In the bloat sub-collector, the `wastedibytes` column from the bloat query returns `numeric` which pgx scans as float64. Change the scan target from `int64` to `float64`, then cast to int64 after scan:

```go
var wastedIBytes float64  // was int64
// ... scan ...
points = append(points, point("db.bloat.wasted_bytes", int64(wastedIBytes), ...))
```

### 6.3 Add `pg.server.multixact_pct` (`internal/collector/server_info.go`)

The alert rules reference `pg.server.multixact_pct` but no collector emits it. Add to `ServerInfoCollector.Collect()`:

```go
// Query: SELECT max(mxid_age(datminmxid))::float / (2^31)::float * 100 FROM pg_database
// Emit: point("server.multixact_pct", pct)
```

This mirrors the existing `server.txid_wraparound_pct` pattern already in `ServerInfoCollector`.

### 6.4 `srsubstate` Char Scan Fix (`internal/collector/` — logical replication or database.go)

The `srsubstate` column is `char(1)`. pgx doesn't scan `char(1)` into `*byte`. Change scan target to `*string`:

```go
var srsubstate *string  // was *byte
// ... scan ...
```

---

## 7. File Inventory (New + Modified)

### New Files

| File | Est. Lines | Purpose |
|------|------------|---------|
| `internal/statements/types.go` | ~150 | All type definitions |
| `internal/statements/store.go` | ~30 | SnapshotStore interface |
| `internal/statements/pgstore.go` | ~300 | PostgreSQL implementation (COPY bulk insert) |
| `internal/statements/pgstore_test.go` | ~250 | Store tests |
| `internal/statements/nullstore.go` | ~40 | No-op store for live mode |
| `internal/statements/capture.go` | ~200 | SnapshotCapturer (periodic + manual) |
| `internal/statements/capture_test.go` | ~200 | Capturer tests |
| `internal/statements/diff.go` | ~180 | ComputeDiff + derived field calculation |
| `internal/statements/diff_test.go` | ~300 | Table-driven diff tests |
| `internal/statements/insights.go` | ~80 | BuildQueryInsight |
| `internal/statements/insights_test.go` | ~120 | Insights tests |
| `internal/statements/report.go` | ~100 | GenerateReport |
| `internal/statements/report_test.go` | ~100 | Report tests |
| `internal/api/handler_snapshots.go` | ~350 | 7 API endpoint handlers |
| `internal/api/handler_snapshots_test.go` | ~400 | Handler tests |
| `internal/storage/migrations/015_pgss_snapshots.sql` | ~50 | Migration |

**Total new:** ~16 files, ~2,850 estimated lines

### Modified Files

| File | Change |
|------|--------|
| `cmd/pgpulse-server/main.go` | Remove debug log + wire SnapshotCapturer + snapshotStore |
| `internal/config/config.go` | Add StatementSnapshotsConfig struct + field |
| `internal/api/server.go` | Add snapshot routes, SetSnapshotStore/SetSnapshotCapturer methods |
| `internal/collector/database.go` | Fix wastedibytes float64 scan |
| `internal/collector/server_info.go` | Add multixact_pct metric |
| `internal/collector/database.go` or relevant file | Fix srsubstate char(1) scan |

**Total modified:** ~6 files

---

## 8. Dependency Graph

```
statements/types.go        ← used by everything
statements/store.go        ← interface, depends on types
statements/pgstore.go      ← depends on store interface + types
statements/nullstore.go    ← depends on store interface
statements/capture.go      ← depends on store + pgxpool + version
statements/diff.go         ← depends on types only (pure logic)
statements/insights.go     ← depends on types only (pure logic)
statements/report.go       ← depends on types + diff
api/handler_snapshots.go   ← depends on statements/* + store interface
config/config.go           ← standalone change
main.go                    ← depends on config + statements + api
migration                  ← standalone SQL file
```

**Agent work order:**
1. types.go + store.go + nullstore.go (no deps, unblocks everything)
2. pgstore.go + migration (depends on 1)
3. diff.go + insights.go + report.go (depends on 1, pure logic)
4. capture.go (depends on 1 + 2)
5. handler_snapshots.go (depends on all above)
6. config.go + main.go wiring (depends on 5)
7. Bug fixes (independent, can be parallel)
