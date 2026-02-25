# PGPulse — M1_01 Design: Instance Metrics Collector

**Milestone:** M1 — Core Collector
**Iteration:** M1_01
**Date:** 2026-02-25
**Produces:** 8 Go source files + 8 test files

---

## 1. Design Overview

M1_01 builds the first real collectors on top of the M0 scaffold. Each collector
is a standalone struct in its own file implementing the `Collector` interface.
One file = one collector = one struct. This keeps each collector independently
testable and allows selective enable/disable at runtime (e.g., an operator can
turn off `database_sizes` on a 200-database instance without touching other collectors).
A registry pattern ties them together for batch execution.

```
┌─────────────────────────────────────────────────────┐
│                  Registry                           │
│  CollectAll(ctx, conn) → []MetricPoint              │
│                                                     │
│  ┌──────────────┐ ┌──────────────┐ ┌─────────────┐  │
│  │ ServerInfo   │ │ Connections  │ │ Cache       │  │
│  │ Q2,Q3,Q9,Q10│ │ Q11,Q12,Q13 │ │ Q14         │  │
│  └──────────────┘ └──────────────┘ └─────────────┘  │
│  ┌──────────────┐ ┌──────────────┐ ┌─────────────┐  │
│  │ Transactions │ │ DB Sizes    │ │ Settings    │  │
│  │ Q15          │ │ Q16         │ │ Q17         │  │
│  └──────────────┘ └──────────────┘ └─────────────┘  │
│  ┌──────────────┐                                   │
│  │ Extensions   │                                   │
│  │ Q18, Q19     │                                   │
│  └──────────────┘                                   │
└─────────────────────────────────────────────────────┘
```

All collectors share:
- A `*pgx.Conn` passed in from the caller (no connection management here)
- An `instanceID string` set at construction time
- A `version.PGVersion` detected once and cached

---

## 2. Shared Patterns

### 2.1 Base Collector Struct

Every collector embeds a common base to avoid boilerplate:

```go
// internal/collector/base.go

package collector

import (
    "time"

    "github.com/ios9000/PGPulse_01/internal/version"
)

// Base provides common fields for all collectors.
type Base struct {
    instanceID string
    pgVersion  version.PGVersion
    interval   time.Duration
}

// newBase creates a Base with the given instance ID, PG version, and collection interval.
func newBase(instanceID string, v version.PGVersion, interval time.Duration) Base {
    return Base{
        instanceID: instanceID,
        pgVersion:  v,
        interval:   interval,
    }
}

// point creates a MetricPoint with common fields pre-filled.
func (b *Base) point(metric string, value float64, labels map[string]string) MetricPoint {
    return MetricPoint{
        InstanceID: b.instanceID,
        Metric:     metric,
        Value:      value,
        Labels:     labels,
        Timestamp:  time.Now(),
    }
}

// Interval returns the collection interval.
func (b *Base) Interval() time.Duration {
    return b.interval
}
```

### 2.2 Metric Naming Convention

All metrics use a dotted namespace: `pgpulse.<category>.<metric>`

```
pgpulse.server.start_time_unix
pgpulse.server.uptime_seconds
pgpulse.server.is_in_recovery
pgpulse.server.is_in_backup

pgpulse.connections.total
pgpulse.connections.by_state         (label: state=active|idle|idle_in_transaction|...)
pgpulse.connections.max
pgpulse.connections.superuser_reserved
pgpulse.connections.utilization_pct

pgpulse.cache.hit_ratio_pct

pgpulse.transactions.commit_ratio_pct   (label: database=<name>)
pgpulse.transactions.deadlocks          (label: database=<name>)

pgpulse.database.size_bytes             (label: database=<name>)

pgpulse.settings.track_io_timing
pgpulse.settings.shared_buffers_8kb
pgpulse.settings.max_locks_per_tx
pgpulse.settings.max_prepared_tx

pgpulse.extensions.pgss_installed
pgpulse.extensions.pgss_fill_pct
pgpulse.extensions.pgss_stats_reset_unix
```

### 2.3 Statement Timeout Convention

Every query must be wrapped with a statement timeout. Rather than SET per query
(which is session-level and could leak), use pgx's query-level timeout via
context:

```go
func queryWithTimeout(ctx context.Context, conn *pgx.Conn, timeout time.Duration, sql string, args ...any) (pgx.Rows, error) {
    ctx, cancel := context.WithTimeout(ctx, timeout)
    defer cancel()
    return conn.Query(ctx, sql, args...)
}
```

Default timeout for all M1 collectors: **5 seconds** (matches PGAM live dashboard).

This helper goes in `base.go`.

---

## 3. File-by-File Design

### 3.1 internal/collector/base.go

**Purpose:** Shared base struct, helper methods, constants.

```go
package collector

import (
    "context"
    "time"

    "github.com/jackc/pgx/v5"
    "github.com/ios9000/PGPulse_01/internal/version"
)

const (
    defaultTimeout = 5 * time.Second
    metricPrefix   = "pgpulse"
)

type Base struct {
    instanceID string
    pgVersion  version.PGVersion
    interval   time.Duration
}

func newBase(instanceID string, v version.PGVersion, interval time.Duration) Base {
    return Base{instanceID: instanceID, pgVersion: v, interval: interval}
}

func (b *Base) point(metric string, value float64, labels map[string]string) MetricPoint {
    return MetricPoint{
        InstanceID: b.instanceID,
        Metric:     metricPrefix + "." + metric,
        Value:      value,
        Labels:     labels,
        Timestamp:  time.Now(),
    }
}

func (b *Base) Interval() time.Duration {
    return b.interval
}

// queryContext returns a child context with the default statement timeout.
func queryContext(ctx context.Context) (context.Context, context.CancelFunc) {
    return context.WithTimeout(ctx, defaultTimeout)
}
```

---

### 3.2 internal/collector/server_info.go

**Purpose:** PG start time, uptime, recovery state, backup state.
**PGAM Queries:** Q2, Q3, Q9, Q10

```go
package collector

// ServerInfoCollector collects PostgreSQL server identity and state metrics.
type ServerInfoCollector struct {
    Base
}

func NewServerInfoCollector(instanceID string, v version.PGVersion) *ServerInfoCollector {
    return &ServerInfoCollector{
        Base: newBase(instanceID, v, 60*time.Second),
    }
}

func (c *ServerInfoCollector) Name() string { return "server_info" }
```

**SQL Queries:**

```sql
-- Q2: Server start time
SELECT extract(epoch FROM pg_postmaster_start_time())::bigint AS start_epoch;

-- Q9: Recovery state
SELECT pg_is_in_recovery() AS is_recovery;

-- Q10: Backup state (PG 14 ONLY — removed in PG 15)
SELECT pg_is_in_backup() AS is_backup;
```

**Collect() logic:**

```
1. Query Q2 → emit "server.start_time_unix" = epoch
2. Compute uptime in Go: time.Now().Unix() - epoch → emit "server.uptime_seconds"
3. Query Q9 → emit "server.is_in_recovery" = 1.0 or 0.0
4. If pgVersion.Num < 150000:
     Use version.Gate to select Q10
     Query → emit "server.is_in_backup" = 1.0 or 0.0
   Else:
     emit "server.is_in_backup" = 0.0 (no backup tracking on PG 15+)
```

**Version Gate Definition:**

```go
var backupStateGate = version.Gate{
    Name: "backup_state",
    Variants: []version.SQLVariant{
        {
            MinVersion: version.PGVersion{Major: 14, Num: 140000},
            MaxVersion: version.PGVersion{Major: 14, Num: 149999},
            SQL:        "SELECT pg_is_in_backup()::int AS is_backup",
        },
        // PG 15+: no variant — Gate.Select() returns false, caller skips
    },
}
```

---

### 3.3 internal/collector/connections.go

**Purpose:** Connection counts by state, max, reserved, utilization.
**PGAM Queries:** Q11, Q12, Q13 (enhanced)

```go
package collector

// ConnectionsCollector collects connection count and utilization metrics.
type ConnectionsCollector struct {
    Base
}

func NewConnectionsCollector(instanceID string, v version.PGVersion) *ConnectionsCollector {
    return &ConnectionsCollector{
        Base: newBase(instanceID, v, 10*time.Second), // high frequency
    }
}

func (c *ConnectionsCollector) Name() string { return "connections" }
```

**SQL Queries:**

```sql
-- Q11 (enhanced): Connection count by state, excluding self
SELECT
    state,
    count(*) AS cnt
FROM pg_stat_activity
WHERE pid != pg_backend_pid()
  AND backend_type = 'client backend'
GROUP BY state;

-- Q12 + Q13: Max and reserved (single query)
SELECT
    current_setting('max_connections')::int AS max_conn,
    current_setting('superuser_reserved_connections')::int AS reserved;
```

**Collect() logic:**

```
1. Query state breakdown → for each row:
     emit "connections.by_state" value=cnt labels={state: <state>}
     accumulate total
2. emit "connections.total" = sum of all states
3. Query max + reserved
4. emit "connections.max" = max_conn
5. emit "connections.superuser_reserved" = reserved
6. Compute utilization = total / (max_conn - reserved) * 100
7. emit "connections.utilization_pct" = utilization
```

**Notes:**
- PGAM bug fixed: `WHERE pid != pg_backend_pid()` excludes PGPulse's own connection.
- PGAM only counted total. We enhance with per-state breakdown (active, idle,
  idle_in_transaction, idle_in_transaction_aborted, disabled, NULL).
- NULL state means backend hasn't reported state yet — treat as "unknown".

---

### 3.4 internal/collector/cache.go

**Purpose:** Global buffer cache hit ratio.
**PGAM Query:** Q14

```go
package collector

// CacheCollector collects global buffer cache hit ratio.
type CacheCollector struct {
    Base
}

func NewCacheCollector(instanceID string, v version.PGVersion) *CacheCollector {
    return &CacheCollector{
        Base: newBase(instanceID, v, 60*time.Second),
    }
}

func (c *CacheCollector) Name() string { return "cache" }
```

**SQL Query:**

```sql
-- Q14: Global cache hit ratio (with NULLIF fix)
SELECT
    COALESCE(
        sum(blks_hit) * 100.0 / NULLIF(sum(blks_hit) + sum(blks_read), 0),
        0
    ) AS hit_ratio
FROM pg_stat_database;
```

**Collect() logic:**

```
1. Query → single row, single value
2. emit "cache.hit_ratio_pct" = hit_ratio
```

**PGAM bug fixed:** Original used `sum(blks_hit)*100/sum(blks_hit+blks_read)` which
divides by zero on a freshly started instance with no reads. We add NULLIF + COALESCE.

---

### 3.5 internal/collector/transactions.go

**Purpose:** Commit/rollback ratio and deadlocks per database.
**PGAM Query:** Q15 (enhanced)

```go
package collector

// TransactionsCollector collects transaction commit ratio and deadlock counts.
type TransactionsCollector struct {
    Base
}

func NewTransactionsCollector(instanceID string, v version.PGVersion) *TransactionsCollector {
    return &TransactionsCollector{
        Base: newBase(instanceID, v, 60*time.Second),
    }
}

func (c *TransactionsCollector) Name() string { return "transactions" }
```

**SQL Query:**

```sql
-- Q15 (enhanced): Per-database commit ratio + deadlocks
SELECT
    d.datname,
    COALESCE(
        s.xact_commit * 100.0 / NULLIF(s.xact_commit + s.xact_rollback, 0),
        0
    ) AS commit_ratio,
    s.deadlocks
FROM pg_stat_database s
JOIN pg_database d ON d.oid = s.datid
WHERE d.datistemplate = false
  AND d.datname != 'postgres'
  AND s.xact_commit + s.xact_rollback > 0;
```

**Collect() logic:**

```
1. Query → for each row:
     emit "transactions.commit_ratio_pct" value=commit_ratio labels={database: datname}
     emit "transactions.deadlocks" value=deadlocks labels={database: datname}
```

**Enhancement over PGAM:** PGAM computed a single global ratio. We report per-database
so operators can see which database has poor commit ratios. We also add deadlock
counts which PGAM didn't surface directly.

**Design note:** We exclude `postgres` database (maintenance DB) and template DBs
to reduce noise. The `postgres` DB is used internally by PGPulse itself for
monitoring connections, so its stats would be self-referential.

---

### 3.6 internal/collector/database_sizes.go

**Purpose:** Size in bytes per non-template database.
**PGAM Query:** Q16

```go
package collector

// DatabaseSizesCollector collects the size of each database.
type DatabaseSizesCollector struct {
    Base
}

func NewDatabaseSizesCollector(instanceID string, v version.PGVersion) *DatabaseSizesCollector {
    return &DatabaseSizesCollector{
        Base: newBase(instanceID, v, 300*time.Second), // 5 min — sizes change slowly
    }
}

func (c *DatabaseSizesCollector) Name() string { return "database_sizes" }
```

**SQL Query:**

```sql
-- Q16: Database sizes
SELECT
    datname,
    pg_database_size(datname) AS size_bytes
FROM pg_database
WHERE datistemplate = false
ORDER BY size_bytes DESC;
```

**Collect() logic:**

```
1. Query → for each row:
     emit "database.size_bytes" value=size_bytes labels={database: datname}
```

**Note:** pg_database_size() requires CONNECT privilege on the database, which
pg_monitor has by default.

---

### 3.7 internal/collector/settings.go

**Purpose:** Key runtime settings as metrics.
**PGAM Query:** Q17 (extended)

```go
package collector

// SettingsCollector collects key PostgreSQL runtime settings.
type SettingsCollector struct {
    Base
}

func NewSettingsCollector(instanceID string, v version.PGVersion) *SettingsCollector {
    return &SettingsCollector{
        Base: newBase(instanceID, v, 300*time.Second), // settings rarely change
    }
}

func (c *SettingsCollector) Name() string { return "settings" }
```

**SQL Query:**

```sql
-- Q17 (extended): Key settings
SELECT name, setting, unit
FROM pg_settings
WHERE name IN (
    'track_io_timing',
    'shared_buffers',
    'max_locks_per_transaction',
    'max_prepared_transactions'
);
```

**Collect() logic:**

```
1. Query → for each row:
     Convert value based on setting type:
       - 'track_io_timing': "on" → 1.0, "off" → 0.0
       - 'shared_buffers': numeric (already in 8KB pages)
       - 'max_locks_per_transaction': numeric
       - 'max_prepared_transactions': numeric
     emit "settings.<name>" value=converted labels=nil
```

**Design:** Using an IN-list query instead of multiple SHOW commands. This is more
efficient (single round-trip) and easier to extend — just add names to the list.

**Mapping table (in code):**

```go
var settingsMap = map[string]struct {
    metric   string
    boolType bool // true = on/off → 1.0/0.0; false = parse as float
}{
    "track_io_timing":            {metric: "settings.track_io_timing", boolType: true},
    "shared_buffers":             {metric: "settings.shared_buffers_8kb", boolType: false},
    "max_locks_per_transaction":  {metric: "settings.max_locks_per_tx", boolType: false},
    "max_prepared_transactions":  {metric: "settings.max_prepared_tx", boolType: false},
}
```

---

### 3.8 internal/collector/extensions.go

**Purpose:** pg_stat_statements presence, fill %, and stats reset info.
**PGAM Queries:** Q18, Q19

```go
package collector

// ExtensionsCollector checks for key PostgreSQL extensions and their state.
type ExtensionsCollector struct {
    Base
}

func NewExtensionsCollector(instanceID string, v version.PGVersion) *ExtensionsCollector {
    return &ExtensionsCollector{
        Base: newBase(instanceID, v, 300*time.Second),
    }
}

func (c *ExtensionsCollector) Name() string { return "extensions" }
```

**SQL Queries:**

```sql
-- Q18: pg_stat_statements installed?
SELECT count(*) AS installed
FROM pg_extension
WHERE extname = 'pg_stat_statements';

-- Q19: pg_stat_statements fill percentage (only if installed)
SELECT
    count(*) * 100.0 / current_setting('pg_stat_statements.max')::float AS fill_pct
FROM pg_stat_statements;

-- Q19b: stats reset info (PG ≥ 14 only, if pgss installed)
SELECT
    extract(epoch FROM stats_reset)::bigint AS reset_epoch
FROM pg_stat_statements_info;
```

**Collect() logic:**

```
1. Query Q18 → installed = count > 0
2. emit "extensions.pgss_installed" = 1.0 or 0.0
3. If installed:
     Query Q19 → emit "extensions.pgss_fill_pct" = fill_pct
     If pgVersion.AtLeast(14, 0):
       Query Q19b → emit "extensions.pgss_stats_reset_unix" = reset_epoch
```

**Error handling:** Q19 will fail if pg_stat_statements.max is not set or pgss has
no data. Wrap in error check — if Q19 fails, log warning and skip (don't abort
the whole collector).

---

### 3.9 internal/collector/registry.go

**Purpose:** Register collectors, execute all, handle partial failures.

```go
package collector

// Registry manages a set of collectors and runs them in batch.
type Registry struct {
    collectors []Collector
}

// NewRegistry creates an empty collector registry.
func NewRegistry() *Registry {
    return &Registry{}
}

// Register adds a collector to the registry.
func (r *Registry) Register(c Collector) {
    r.collectors = append(r.collectors, c)
}

// CollectAll runs every registered collector and returns all metrics.
// Individual collector failures are logged but do not abort the batch.
func (r *Registry) CollectAll(ctx context.Context, conn *pgx.Conn) []MetricPoint
```

**CollectAll() logic:**

```
1. Create results slice
2. For each collector:
     a. Create child context with 5s timeout
     b. start := time.Now()
     c. points, err := collector.Collect(ctx, conn)
     d. duration := time.Since(start)
     e. If err != nil:
          slog.Error("collector failed", "name", collector.Name(),
                     "error", err, "duration", duration)
          continue  // do NOT abort
     f. slog.Debug("collector completed", "name", collector.Name(),
                   "points", len(points), "duration", duration)
     g. Append points to results
3. Return results
```

**Key design choice:** Sequential execution within CollectAll(). We do NOT
parallelize collectors in M1 because:
- All queries go to the same PG instance over one connection
- PG processes them sequentially anyway
- Simpler error handling and debugging
- Parallel scheduling comes in M2 with the collection scheduler

---

## 4. Test Strategy

### 4.1 Test Helper: testcontainers Setup

Create a shared test helper to avoid boilerplate:

```go
// internal/collector/testutil_test.go

package collector_test

import (
    "context"
    "testing"

    "github.com/jackc/pgx/v5"
    "github.com/testcontainers/testcontainers-go"
    "github.com/testcontainers/testcontainers-go/modules/postgres"
)

// setupPG starts a PostgreSQL container and returns a connected *pgx.Conn.
// The container and connection are cleaned up when the test ends.
func setupPG(t *testing.T, pgVersion string) *pgx.Conn {
    t.Helper()
    ctx := context.Background()

    container, err := postgres.Run(ctx,
        "docker.io/postgres:"+pgVersion,
        postgres.WithDatabase("testdb"),
        postgres.WithUsername("testuser"),
        postgres.WithPassword("testpass"),
    )
    if err != nil {
        t.Fatalf("failed to start PG %s container: %v", pgVersion, err)
    }
    t.Cleanup(func() { container.Terminate(ctx) })

    connStr, err := container.ConnectionString(ctx, "sslmode=disable")
    if err != nil {
        t.Fatalf("failed to get connection string: %v", err)
    }

    conn, err := pgx.Connect(ctx, connStr)
    if err != nil {
        t.Fatalf("failed to connect to PG %s: %v", pgVersion, err)
    }
    t.Cleanup(func() { conn.Close(ctx) })

    return conn
}

// setupPGWithStatements starts PG with pg_stat_statements enabled.
func setupPGWithStatements(t *testing.T, pgVersion string) *pgx.Conn {
    t.Helper()
    ctx := context.Background()

    container, err := postgres.Run(ctx,
        "docker.io/postgres:"+pgVersion,
        postgres.WithDatabase("testdb"),
        postgres.WithUsername("testuser"),
        postgres.WithPassword("testpass"),
        postgres.WithInitScripts(), // could add init SQL here
        testcontainers.WithEnv(map[string]string{
            "POSTGRES_INITDB_ARGS": "--auth=trust",
        }),
        // Preload pg_stat_statements
        postgres.WithConfigFile(""), // We'll use ALTER SYSTEM instead
    )
    if err != nil {
        t.Fatalf("failed to start PG %s: %v", pgVersion, err)
    }
    t.Cleanup(func() { container.Terminate(ctx) })

    connStr, err := container.ConnectionString(ctx, "sslmode=disable")
    if err != nil {
        t.Fatalf("failed to get connection string: %v", err)
    }

    conn, err := pgx.Connect(ctx, connStr)
    if err != nil {
        t.Fatalf("failed to connect: %v", err)
    }
    t.Cleanup(func() { conn.Close(ctx) })

    // Enable pg_stat_statements (requires shared_preload_libraries,
    // which needs a restart — for testcontainers we may need to
    // configure this at container start. Alternative: test without it
    // and handle the "extension not available" path.)
    _, _ = conn.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS pg_stat_statements")

    return conn
}
```

### 4.2 Test File Structure

Each collector gets a test file with:
- At minimum one test per supported PG version (14, 17)
- Table-driven tests where multiple cases exist
- Build tag `//go:build integration` for tests requiring Docker

```go
// internal/collector/server_info_test.go

//go:build integration

package collector_test

func TestServerInfoCollector_PG17(t *testing.T) {
    conn := setupPG(t, "17")
    v, _ := version.Detect(context.Background(), conn)
    c := collector.NewServerInfoCollector("test-instance", v)

    points, err := c.Collect(context.Background(), conn)
    require.NoError(t, err)
    require.NotEmpty(t, points)

    // Verify expected metrics are present
    metrics := metricNames(points)
    assert.Contains(t, metrics, "pgpulse.server.start_time_unix")
    assert.Contains(t, metrics, "pgpulse.server.uptime_seconds")
    assert.Contains(t, metrics, "pgpulse.server.is_in_recovery")
    assert.Contains(t, metrics, "pgpulse.server.is_in_backup")

    // Verify start_time is reasonable (after 2020)
    startTime := findMetric(points, "pgpulse.server.start_time_unix")
    assert.Greater(t, startTime.Value, float64(1577836800)) // 2020-01-01

    // Verify uptime is positive
    uptime := findMetric(points, "pgpulse.server.uptime_seconds")
    assert.Greater(t, uptime.Value, 0.0)

    // PG 17: backup should be 0 (function doesn't exist)
    backup := findMetric(points, "pgpulse.server.is_in_backup")
    assert.Equal(t, 0.0, backup.Value)
}

func TestServerInfoCollector_PG14(t *testing.T) {
    conn := setupPG(t, "14")
    v, _ := version.Detect(context.Background(), conn)
    c := collector.NewServerInfoCollector("test-instance", v)

    points, err := c.Collect(context.Background(), conn)
    require.NoError(t, err)

    // PG 14: pg_is_in_backup() should execute without error
    backup := findMetric(points, "pgpulse.server.is_in_backup")
    assert.NotNil(t, backup)
    // Value will be 0.0 (no backup running) but the query should not error
}
```

### 4.3 Test Helpers

```go
// internal/collector/testutil_test.go (additional helpers)

// metricNames extracts metric names from a slice of MetricPoints.
func metricNames(points []MetricPoint) []string {
    names := make([]string, len(points))
    for i, p := range points {
        names[i] = p.Metric
    }
    return names
}

// findMetric finds the first MetricPoint with the given name.
func findMetric(points []MetricPoint, name string) *MetricPoint {
    for _, p := range points {
        if p.Metric == name {
            return &p
        }
    }
    return nil
}

// findMetricWithLabel finds a MetricPoint matching name and label key=value.
func findMetricWithLabel(points []MetricPoint, name, labelKey, labelValue string) *MetricPoint {
    for _, p := range points {
        if p.Metric == name && p.Labels[labelKey] == labelValue {
            return &p
        }
    }
    return nil
}
```

### 4.4 Registry Test

```go
// internal/collector/registry_test.go

func TestRegistry_CollectAll_PartialFailure(t *testing.T) {
    // Create a collector that always fails
    failing := &mockCollector{
        name:    "failing",
        err:     errors.New("simulated failure"),
    }

    // Create a collector that succeeds
    passing := &mockCollector{
        name:   "passing",
        points: []MetricPoint{{Metric: "test.metric", Value: 1.0}},
    }

    reg := collector.NewRegistry()
    reg.Register(failing)
    reg.Register(passing)

    points := reg.CollectAll(context.Background(), nil) // conn not used by mocks
    assert.Len(t, points, 1) // only the passing collector's results
    assert.Equal(t, "test.metric", points[0].Metric)
}
```

---

## 5. Version Gate Detail

### 5.1 Full M1 Version Gate Inventory

Every version-conditional query across all M1 sub-iterations:

| Gate | PG 14 | PG 15 | PG 16 | PG 17 | Sub-Iteration |
|------|-------|-------|-------|-------|---------------|
| `pg_is_in_backup()` | ✅ exists | ❌ removed | ❌ removed | ❌ removed | **M1_01** |
| `pg_stat_statements_info` | ✅ PG ≥ 14 | same | same | same | **M1_01** |
| `pg_stat_bgwriter` (checkpoint cols) | ✅ all-in-one | ✅ all-in-one | ✅ all-in-one | ❌ split → `pg_stat_checkpointer` | M1_03 |
| `pg_stat_io` | ❌ doesn't exist | ❌ doesn't exist | ✅ new view | ✅ exists | M1_03 |
| Replication slot `two_phase` col | ❌ | ✅ PG 15+ | same | same | M1_02 |
| Replication slot `conflicting` col | ❌ | ❌ | ✅ PG 16+ | same | M1_02 |
| `pg_stat_wal` | ✅ PG ≥ 14 | same | same | same | M1_03 |
| `total_time` vs `total_exec_time` | `total_exec_time` (PG ≥ 13) | same | same | same | M1_04 (moot — min PG 14) |

### 5.2 M1_01 Gates (This Iteration)

Only two gates, both low complexity:

| Gate Name | PG 14 | PG 15+ |
|-----------|-------|--------|
| `backup_state` | `SELECT pg_is_in_backup()::int` | No query (skip, emit 0.0) |
| `pgss_info` | `SELECT ... FROM pg_stat_statements_info` | Same (exists on all PG ≥ 14) |

The `pgss_info` gate is technically always-true given our minimum is PG 14,
but it's conditional on pg_stat_statements being *installed* — so the real
gate is "is pgss loaded?" not "is PG new enough?".

### 5.3 M1 Sub-Iteration Breakdown (Revised)

The hardest version gate work (bgwriter/checkpointer split, pg_stat_io) is
concentrated in M1_03 where it can be the sole focus.

| Iteration | Scope | Version Gates | Complexity |
|-----------|-------|---------------|------------|
| **M1_01** | Server info, connections, cache, transactions, sizes, settings, extensions | backup (PG14), pgss_info (PG≥14) | Low — prove the architecture |
| **M1_02** | Replication: physical + logical, slots, WAL receiver | slot columns (PG15+, PG16+) | Medium-High |
| **M1_03** | Checkpoint, bgwriter, WAL generation, pg_stat_io | **bgwriter/checkpointer split (PG17)**, pg_stat_io (PG16+) | High — most gate work |
| **M1_04** | pg_stat_statements analysis (IO + CPU sorted) | Minimal (PG 14 is our floor) | Medium |
| **M1_05** | Locks, wait events, long transactions | Minimal gates | Medium (recursive CTE complexity) |

### 5.4 How the Gate Pattern Works (from M0)

```go
// Defined in collector
var backupGate = version.Gate{
    Name: "backup_state",
    Variants: []version.SQLVariant{
        {
            MinVersion: 140000,
            MaxVersion: 149999,
            SQL:        "SELECT pg_is_in_backup()::int AS is_backup",
        },
    },
}

// Used in Collect()
if sql, ok := backupGate.Select(c.pgVersion); ok {
    var isBackup int
    err := conn.QueryRow(ctx, sql).Scan(&isBackup)
    // ...
    points = append(points, c.point("server.is_in_backup", float64(isBackup), nil))
} else {
    // PG 15+: function doesn't exist, emit 0
    points = append(points, c.point("server.is_in_backup", 0.0, nil))
}
```

### 5.5 Future Gates (documented in 5.1 above, not in M1_01)

| Gate | PG < 10 | PG ≥ 10 | Milestone |
|------|---------|---------|-----------|
| WAL functions | `pg_xlog_location_diff` | `pg_wal_lsn_diff` | M1_02 |
| Replication LSN | `pg_current_xlog_insert_location()` | `pg_current_wal_insert_lsn()` | M1_02 |
| Statement time | `total_time` | `total_exec_time` | M1_04 |
| pgss info | N/A | `pg_stat_statements_info` (PG ≥ 14) | M1_01 (extensions.go) |

---

## 6. Error Handling Strategy

### Per-Query Errors

```go
// Pattern: wrap and return, let registry decide
row := conn.QueryRow(ctx, sql)
var value float64
if err := row.Scan(&value); err != nil {
    return nil, fmt.Errorf("collectCacheHit: %w", err)
}
```

### Per-Collector Errors

The registry logs the error and continues to the next collector:

```go
// In Registry.CollectAll()
points, err := c.Collect(ctx, conn)
if err != nil {
    slog.Error("collector failed",
        "collector", c.Name(),
        "error", err,
        "duration", time.Since(start),
    )
    continue // do not abort
}
```

### Connection Errors

If the pgx connection is dead, every collector will fail. The registry will
log N errors and return an empty slice. The caller (scheduler, in M2) is
responsible for reconnection logic.

---

## 7. Files Created in This Iteration

| File | Agent | Lines (est.) |
|------|-------|-------------|
| `internal/collector/base.go` | Collector | ~50 |
| `internal/collector/server_info.go` | Collector | ~80 |
| `internal/collector/connections.go` | Collector | ~90 |
| `internal/collector/cache.go` | Collector | ~40 |
| `internal/collector/transactions.go` | Collector | ~60 |
| `internal/collector/database_sizes.go` | Collector | ~50 |
| `internal/collector/settings.go` | Collector | ~70 |
| `internal/collector/extensions.go` | Collector | ~90 |
| `internal/collector/registry.go` | Collector | ~60 |
| `internal/collector/testutil_test.go` | QA | ~80 |
| `internal/collector/server_info_test.go` | QA | ~70 |
| `internal/collector/connections_test.go` | QA | ~60 |
| `internal/collector/cache_test.go` | QA | ~40 |
| `internal/collector/transactions_test.go` | QA | ~50 |
| `internal/collector/database_sizes_test.go` | QA | ~40 |
| `internal/collector/settings_test.go` | QA | ~50 |
| `internal/collector/extensions_test.go` | QA | ~60 |
| `internal/collector/registry_test.go` | QA | ~50 |
| **Total** | | **~1090** |

---

## 8. Agent Assignment Summary

**Team size: 2 agents + Team Lead** (no API/Security agent needed for M1_01).

### Collector Agent

Creates all production `.go` files:
- base.go, server_info.go, connections.go, cache.go, transactions.go,
  database_sizes.go, settings.go, extensions.go, registry.go

References: PGAM_FEATURE_AUDIT.md Q2–Q19, this design.md, internal/version/* from M0.

Rules: parameterized SQL only, version gates via Gate pattern, 5s timeout,
application_name = 'pgpulse_collector'.

### QA Agent

Creates all `_test.go` files + test helpers:
- testutil_test.go (shared setup), one test file per collector, registry_test.go

Rules: use testcontainers-go, test PG 14 + PG 17 minimum, build tag `integration`,
verify no string concatenation in SQL, check all exported functions have GoDoc.

**Additional responsibility:** Independently verify each SQL query against the
PG 14 and PG 17 system catalog documentation. PGAM queries originate from a
PG 9-era codebase and may reference removed columns or use outdated syntax.
Flag any query that doesn't match current PG catalog definitions.

### API & Security Agent

**Not active in M1_01.** No API, storage, or auth work in this iteration.
Activates in M2.

---

## 9. Open Questions (Resolved)

| Question | Resolution |
|----------|-----------|
| Should we use one file (instance.go) or split into modules? | **Split** — 7 focused files are easier to test and own |
| Should CollectAll() run collectors in parallel? | **No** — sequential over one connection; parallel scheduling in M2 |
| Should we exclude `postgres` DB from transaction stats? | **Yes** — it's the monitoring DB, stats would be self-referential |
| How to handle pg_stat_statements not preloaded in tests? | **Handle gracefully** — test the "not installed" path too |
| Use SHOW vs pg_settings? | **pg_settings** — single query for multiple settings, more metadata |
