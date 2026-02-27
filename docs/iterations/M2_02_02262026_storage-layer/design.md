# M2_02 Design — Storage Layer & Migrations

**Iteration:** M2_02
**Date:** 2026-02-26

---

## 1. Migration Files (embedded via go:embed)

### Directory: `migrations/`

### File: `migrations/001_metrics.sql`

```sql
-- PGPulse metrics storage
CREATE TABLE IF NOT EXISTS metrics (
    time        TIMESTAMPTZ      NOT NULL,
    instance_id TEXT             NOT NULL,
    metric      TEXT             NOT NULL,
    value       DOUBLE PRECISION NOT NULL,
    labels      JSONB            NOT NULL DEFAULT '{}'
);

CREATE INDEX IF NOT EXISTS idx_metrics_instance_time
    ON metrics (instance_id, time DESC);

CREATE INDEX IF NOT EXISTS idx_metrics_metric_time
    ON metrics (metric, time DESC);

CREATE INDEX IF NOT EXISTS idx_metrics_instance_metric_time
    ON metrics (instance_id, metric, time DESC);
```

**Design notes:**
- No primary key — time-series data with potential duplicates (same metric collected twice in edge cases). The indexes provide query performance.
- JSONB labels for flexible key-value pairs. Indexed via GIN only if query patterns require it (deferred).
- `IF NOT EXISTS` for idempotency, even though the migration runner tracks state.
- Third compound index `(instance_id, metric, time DESC)` supports the most common query pattern: "give me metric X for instance Y over time range Z."

### File: `migrations/002_timescaledb.sql`

```sql
-- Optional: convert metrics to TimescaleDB hypertable.
-- Only applied when storage.use_timescaledb = true.
-- Requires TimescaleDB extension to be pre-installed.

CREATE EXTENSION IF NOT EXISTS timescaledb;

SELECT create_hypertable('metrics', 'time',
    if_not_exists => TRUE,
    migrate_data => TRUE
);
```

**Handling:** This migration is flagged as conditional. The migration runner applies it only when config says `use_timescaledb: true`. If the extension isn't available, the CREATE EXTENSION will fail and the migration is skipped with a warning.

---

## 2. Package: internal/storage/

### File: `internal/storage/migrate.go`

```go
package storage

import (
    "context"
    "embed"
    "fmt"
    "log/slog"
    "sort"
    "strings"

    "github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// Migrate runs all pending SQL migrations against the database.
// Creates schema_migrations table if it doesn't exist.
// Each migration runs in its own transaction.
// conditionalMigrations maps filename prefixes to skip conditions.
func Migrate(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger, opts MigrateOptions) error

type MigrateOptions struct {
    UseTimescaleDB bool // if false, skip 002_timescaledb.sql
}
```

**Note:** The `migrations/` directory with SQL files lives inside `internal/storage/` (not at project root) so the `go:embed` directive works from within the package. Full path: `internal/storage/migrations/001_metrics.sql`.

**Migrate() logic:**

```
1. Create schema_migrations table if not exists:
   CREATE TABLE IF NOT EXISTS schema_migrations (
       version TEXT PRIMARY KEY,
       applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
   );

2. Read all *.sql files from embedded FS, sort by name

3. For each migration file:
   a. Check if version (filename) exists in schema_migrations
   b. If already applied → skip
   c. If conditional (e.g., 002_timescaledb) and condition is false → skip, log info
   d. Begin transaction
   e. Execute SQL content
   f. INSERT INTO schema_migrations (version) VALUES ($1)
   g. Commit
   h. Log: "applied migration", version=filename
   
4. On any error: rollback transaction, return error with migration name
```

**Conditional migration detection:**
```go
func isConditional(filename string, opts MigrateOptions) bool {
    if strings.HasPrefix(filename, "002_timescaledb") && !opts.UseTimescaleDB {
        return true
    }
    return false
}
```

This is simple and explicit. No framework overhead.

### File: `internal/storage/pgstore.go`

```go
package storage

import (
    "context"
    "encoding/json"
    "fmt"
    "log/slog"
    "strings"
    "time"

    "github.com/jackc/pgx/v5"
    "github.com/jackc/pgx/v5/pgxpool"

    "github.com/ios9000/PGPulse_01/internal/collector"
)

// PGStore implements collector.MetricStore backed by PostgreSQL.
type PGStore struct {
    pool   *pgxpool.Pool
    logger *slog.Logger
}

// NewPGStore creates a PGStore from a connection pool.
// Does NOT run migrations — call Migrate() separately before creating PGStore.
func NewPGStore(pool *pgxpool.Pool, logger *slog.Logger) *PGStore {
    return &PGStore{pool: pool, logger: logger}
}

// Write inserts metric points using COPY protocol for performance.
func (s *PGStore) Write(ctx context.Context, points []collector.MetricPoint) error

// Query retrieves metric points matching the given criteria.
func (s *PGStore) Query(ctx context.Context, q collector.MetricQuery) ([]collector.MetricPoint, error)

// Close closes the connection pool.
func (s *PGStore) Close() error
```

### Write() — COPY Protocol

```go
func (s *PGStore) Write(ctx context.Context, points []collector.MetricPoint) error {
    if len(points) == 0 {
        return nil
    }

    writeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
    defer cancel()

    // Build rows for CopyFrom.
    rows := make([][]any, len(points))
    for i, p := range points {
        labelsJSON, err := json.Marshal(p.Labels)
        if err != nil {
            return fmt.Errorf("marshal labels for %s: %w", p.Metric, err)
        }
        rows[i] = []any{p.Timestamp, p.InstanceID, p.Metric, p.Value, labelsJSON}
    }

    copied, err := s.pool.CopyFrom(
        writeCtx,
        pgx.Identifier{"metrics"},
        []string{"time", "instance_id", "metric", "value", "labels"},
        pgx.CopyFromRows(rows),
    )
    if err != nil {
        return fmt.Errorf("copy metrics: %w", err)
    }

    s.logger.Debug("wrote metrics", "count", copied)
    return nil
}
```

**Why CopyFrom:**
- INSERT with VALUES for N rows: N round-trips (or $1... expansion for batch)
- CopyFrom: single COPY command, binary protocol, fastest bulk insert in pgx
- For a typical cycle: ~50–200 points per write — CopyFrom handles this efficiently

### Query() — Dynamic WHERE Builder

```go
func (s *PGStore) Query(ctx context.Context, q collector.MetricQuery) ([]collector.MetricPoint, error) {
    queryCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
    defer cancel()

    // Build dynamic query.
    var conditions []string
    var args []any
    argN := 1

    if q.InstanceID != "" {
        conditions = append(conditions, fmt.Sprintf("instance_id = $%d", argN))
        args = append(args, q.InstanceID)
        argN++
    }
    if q.Metric != "" {
        // Prefix match: pgpulse.connections.% matches all connection metrics
        conditions = append(conditions, fmt.Sprintf("metric LIKE $%d", argN))
        args = append(args, q.Metric+"%")
        argN++
    }
    if !q.Start.IsZero() {
        conditions = append(conditions, fmt.Sprintf("time >= $%d", argN))
        args = append(args, q.Start)
        argN++
    }
    if !q.End.IsZero() {
        conditions = append(conditions, fmt.Sprintf("time <= $%d", argN))
        args = append(args, q.End)
        argN++
    }
    // Labels filter: JSONB containment operator @>
    if len(q.Labels) > 0 {
        labelsJSON, err := json.Marshal(q.Labels)
        if err != nil {
            return nil, fmt.Errorf("marshal label filter: %w", err)
        }
        conditions = append(conditions, fmt.Sprintf("labels @> $%d::jsonb", argN))
        args = append(args, labelsJSON)
        argN++
    }

    sql := "SELECT time, instance_id, metric, value, labels FROM metrics"
    if len(conditions) > 0 {
        sql += " WHERE " + strings.Join(conditions, " AND ")
    }
    sql += " ORDER BY time DESC"

    if q.Limit > 0 {
        sql += fmt.Sprintf(" LIMIT $%d", argN)
        args = append(args, q.Limit)
    }

    rows, err := s.pool.Query(queryCtx, sql, args...)
    if err != nil {
        return nil, fmt.Errorf("query metrics: %w", err)
    }
    defer rows.Close()

    var points []collector.MetricPoint
    for rows.Next() {
        var p collector.MetricPoint
        var labelsJSON []byte
        if err := rows.Scan(&p.Timestamp, &p.InstanceID, &p.Metric, &p.Value, &labelsJSON); err != nil {
            return nil, fmt.Errorf("scan metric row: %w", err)
        }
        if err := json.Unmarshal(labelsJSON, &p.Labels); err != nil {
            return nil, fmt.Errorf("unmarshal labels: %w", err)
        }
        points = append(points, p)
    }
    if err := rows.Err(); err != nil {
        return nil, fmt.Errorf("iterate metrics: %w", err)
    }

    return points, nil
}
```

**Security note:** All dynamic parameters use `$N` positional args — no string interpolation of user input into SQL. The Metric field uses LIKE with a suffix `%` appended in Go, but the metric value itself is parameterized.

### Close()

```go
func (s *PGStore) Close() error {
    s.pool.Close()
    return nil
}
```

---

## 3. Pool Creation Helper

### File: `internal/storage/pool.go`

```go
package storage

import (
    "context"
    "fmt"
    "time"

    "github.com/jackc/pgx/v5/pgxpool"
)

// NewPool creates a pgxpool.Pool for PGPulse's own database.
// This is NOT for monitored instances — only for metric storage.
func NewPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
    config, err := pgxpool.ParseConfig(dsn)
    if err != nil {
        return nil, fmt.Errorf("parse storage DSN: %w", err)
    }

    config.MaxConns = 5
    config.MinConns = 1
    config.MaxConnLifetime = 30 * time.Minute
    config.MaxConnIdleTime = 5 * time.Minute
    config.ConnConfig.ConnectTimeout = 10 * time.Second
    config.ConnConfig.RuntimeParams["application_name"] = "pgpulse_storage"

    pool, err := pgxpool.NewWithConfig(ctx, config)
    if err != nil {
        return nil, fmt.Errorf("create storage pool: %w", err)
    }

    // Verify connectivity.
    if err := pool.Ping(ctx); err != nil {
        pool.Close()
        return nil, fmt.Errorf("ping storage DB: %w", err)
    }

    return pool, nil
}
```

---

## 4. Wiring in main.go

Update `cmd/pgpulse-server/main.go` to create PGStore when DSN is configured:

```go
// After loading config, before creating orchestrator:

var store collector.MetricStore

if cfg.Storage.DSN != "" {
    // Create pool
    pool, err := storage.NewPool(ctx, cfg.Storage.DSN)
    if err != nil {
        logger.Error("failed to connect to storage DB", "error", err)
        os.Exit(1)
    }
    defer pool.Close()

    // Run migrations
    if err := storage.Migrate(ctx, pool, logger, storage.MigrateOptions{
        UseTimescaleDB: cfg.Storage.UseTimescaleDB,
    }); err != nil {
        logger.Error("failed to run migrations", "error", err)
        os.Exit(1)
    }

    store = storage.NewPGStore(pool, logger)
    logger.Info("storage initialized", "dsn", "***", "timescaledb", cfg.Storage.UseTimescaleDB)
} else {
    store = orchestrator.NewLogStore(logger)
    logger.Info("no storage DSN configured, using log-only mode")
}

// Pass store to orchestrator as before
orch := orchestrator.New(cfg, store, logger)
```

**Note:** The storage DSN is masked in logs ("***") for security.

---

## 5. Test Strategy

### File: `internal/storage/migrate_test.go`

| Test | Description |
|------|-------------|
| TestMigrate_ParsesEmbeddedFiles | Verify embedded FS contains expected migration files |
| TestMigrate_SortsFilesByName | Verify ordering: 001 before 002 |
| TestIsConditional_TimescaleDB | 002_timescaledb skipped when UseTimescaleDB=false |
| TestIsConditional_Regular | 001_metrics never conditional |

Note: Full Migrate() integration test requires real PG → CI only. Unit tests verify the file parsing and conditional logic.

### File: `internal/storage/pgstore_test.go`

| Test | Description |
|------|-------------|
| TestPGStore_Write_EmptySlice | Write with empty slice → no error, no DB call |
| TestPGStore_Write_MarshalLabels | Verify labels serialize to JSON correctly |
| TestPGStore_Query_BuildsSQL | Verify SQL construction with various MetricQuery fields |
| TestPGStore_Query_EmptyQuery | No filters → SELECT all with ORDER BY |
| TestPGStore_Query_WithLabels | Labels filter uses @> containment |
| TestPGStore_Query_WithLimit | Limit appended as last parameter |
| TestPGStore_Close | Calls pool.Close() |

For SQL construction tests: we can test the query builder logic by extracting it into a helper function that returns (sql string, args []any) — this is testable without a DB connection.

### File: `internal/storage/pool_test.go`

| Test | Description |
|------|-------------|
| TestNewPool_InvalidDSN | Invalid DSN string → error |

Full pool connectivity test requires real PG → CI only.

---

## 6. File Layout

```
internal/storage/
├── migrations/
│   ├── 001_metrics.sql
│   └── 002_timescaledb.sql
├── migrate.go          # Migrate(), go:embed, schema_migrations
├── pgstore.go          # PGStore: Write (CopyFrom), Query, Close
├── pool.go             # NewPool() helper
├── migrate_test.go
├── pgstore_test.go
└── pool_test.go
```

### File Size Estimates

| File | Lines (est.) |
|------|-------------|
| `migrations/001_metrics.sql` | ~15 |
| `migrations/002_timescaledb.sql` | ~10 |
| `internal/storage/migrate.go` | ~110 |
| `internal/storage/pgstore.go` | ~140 |
| `internal/storage/pool.go` | ~40 |
| `cmd/pgpulse-server/main.go` (update) | +20 |
| `internal/storage/migrate_test.go` | ~80 |
| `internal/storage/pgstore_test.go` | ~120 |
| `internal/storage/pool_test.go` | ~20 |
| **Total** | **~555** |

---

## 7. Dependencies

pgxpool is part of pgx v5 — already in go.mod. No new dependencies needed.

Verify: `github.com/jackc/pgx/v5/pgxpool` should resolve without `go get`.

---

## 8. Edge Cases

- **Empty Write:** Write([]) → return nil immediately, no DB call
- **Nil Labels:** If MetricPoint.Labels is nil, marshal as `{}` (the JSONB default)
- **Concurrent Writes:** Multiple interval groups call Write() simultaneously → pgxpool handles connection multiplexing
- **Large batches:** A low-frequency group collecting all slow collectors might produce 100+ points. CopyFrom handles this efficiently.
- **Query with no results:** Return empty slice (not nil) for clarity
- **Storage DB down at startup:** main.go exits with error — can't run without storage if DSN configured
- **Storage DB goes down mid-run:** Write() returns error → orchestrator logs it → metrics lost for that cycle. Acceptable for M2.
