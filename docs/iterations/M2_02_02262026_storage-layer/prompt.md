# M2_02 Prompt — Storage Layer & Migrations

> **Paste this into a single Claude Code session (not Agent Teams).**
> Agent creates files only — developer runs bash manually (Windows bash bug).

---

Build the PG-backed storage layer for PGPulse, replacing the LogStore placeholder.
Read `.claude/CLAUDE.md` for project context.
Read `docs/iterations/M2_02_.../design.md` for detailed specifications.

**⚠️ CRITICAL: You CANNOT run shell commands on this platform.**
Do NOT attempt `go build`, `go test`, `go get`, `git commit`, or any bash commands.
Create and edit files only. Developer will run all bash commands manually.

---

## Files to Create/Modify (in order)

### 1. Create `internal/storage/migrations/001_metrics.sql`

```sql
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

### 2. Create `internal/storage/migrations/002_timescaledb.sql`

```sql
CREATE EXTENSION IF NOT EXISTS timescaledb;

SELECT create_hypertable('metrics', 'time',
    if_not_exists => TRUE,
    migrate_data => TRUE
);
```

### 3. Create `internal/storage/migrate.go`

Implements embedded migration runner.

```go
package storage
```

Key elements:
- `//go:embed migrations/*.sql` directive to embed SQL files
- `var migrationFS embed.FS`
- `type MigrateOptions struct { UseTimescaleDB bool }`
- `func Migrate(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger, opts MigrateOptions) error`

**Migrate() logic:**
1. Create `schema_migrations` table: `CREATE TABLE IF NOT EXISTS schema_migrations (version TEXT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT now())`
2. Read all `migrations/*.sql` filenames from embedded FS, sort alphabetically
3. For each file:
   - Query `SELECT 1 FROM schema_migrations WHERE version = $1` — if exists, skip
   - Check `isConditional(filename, opts)` — if true, log skip and continue
   - Begin transaction
   - Read file content from embedded FS
   - Execute SQL content
   - `INSERT INTO schema_migrations (version) VALUES ($1)` with filename
   - Commit
   - Log: "applied migration", version=filename
4. On error: rollback, return `fmt.Errorf("migration %s: %w", filename, err)`

**isConditional():** returns true for "002_timescaledb.sql" when `opts.UseTimescaleDB == false`. All other migrations are unconditional.

**Imports:** context, embed, fmt, log/slog, sort, strings, path, `github.com/jackc/pgx/v5/pgxpool`

### 4. Create `internal/storage/pgstore.go`

Implements `collector.MetricStore`.

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
```

**PGStore struct:** holds `*pgxpool.Pool` and `*slog.Logger`

**NewPGStore(pool, logger) → *PGStore**

**Write(ctx, points) → error:**
- If empty, return nil
- 10s timeout context
- Build `[][]any` rows: each row is `{p.Timestamp, p.InstanceID, p.Metric, p.Value, labelsJSON}`
- Handle nil Labels: `if p.Labels == nil { p.Labels = map[string]string{} }` before marshal
- `pool.CopyFrom(ctx, pgx.Identifier{"metrics"}, columns, pgx.CopyFromRows(rows))`
- Log debug with count
- Return nil or wrapped error

**Query(ctx, q) → ([]MetricPoint, error):**
- 30s timeout context
- Build dynamic WHERE clauses with positional args ($1, $2...)
- Filters: instance_id =, metric LIKE prefix%, time >=, time <=, labels @> jsonb
- ORDER BY time DESC
- Optional LIMIT
- Scan rows into []MetricPoint (unmarshal JSONB labels)
- Return empty slice (not nil) when no results: `points := []collector.MetricPoint{}`

Extract query building into a helper for testability:
```go
// buildQuery constructs the SQL and args for a metric query.
// Exported for testing.
func buildQuery(q collector.MetricQuery) (string, []any)
```

**Close() → error:** calls pool.Close(), returns nil

### 5. Create `internal/storage/pool.go`

```go
package storage

func NewPool(ctx context.Context, dsn string) (*pgxpool.Pool, error)
```

- Parse DSN via `pgxpool.ParseConfig(dsn)`
- Set: MaxConns=5, MinConns=1, MaxConnLifetime=30m, MaxConnIdleTime=5m
- Set: ConnectTimeout=10s, application_name="pgpulse_storage"
- Create pool, ping to verify, return

### 6. Update `cmd/pgpulse-server/main.go`

**Add storage initialization** between config loading and orchestrator creation:

```go
import "github.com/ios9000/PGPulse_01/internal/storage"
```

Logic (insert before orchestrator.New):
```go
var store collector.MetricStore

if cfg.Storage.DSN != "" {
    pool, err := storage.NewPool(ctx, cfg.Storage.DSN)
    if err != nil {
        logger.Error("failed to connect to storage DB", "error", err)
        os.Exit(1)
    }
    // defer pool.Close() — but PGStore.Close() handles this

    if err := storage.Migrate(ctx, pool, logger, storage.MigrateOptions{
        UseTimescaleDB: cfg.Storage.UseTimescaleDB,
    }); err != nil {
        logger.Error("failed to run migrations", "error", err)
        os.Exit(1)
    }

    pgStore := storage.NewPGStore(pool, logger)
    store = pgStore
    logger.Info("storage initialized with PostgreSQL")
} else {
    store = orchestrator.NewLogStore(logger)
    logger.Info("no storage DSN configured, using log-only mode")
}
```

Also add `pgStore.Close()` in the shutdown sequence (after orchestrator.Stop(), before exit).

### 7. Create test files

#### `internal/storage/migrate_test.go`

| Test | What to verify |
|------|---------------|
| TestMigrateFS_ContainsFiles | migrationFS has 001_metrics.sql and 002_timescaledb.sql |
| TestMigrateFS_FilesAreSorted | Files read in correct order |
| TestIsConditional_TimescaleDisabled | 002_timescaledb + UseTimescaleDB=false → true |
| TestIsConditional_TimescaleEnabled | 002_timescaledb + UseTimescaleDB=true → false |
| TestIsConditional_RegularMigration | 001_metrics → always false |

Note: `isConditional` needs to be exported as `IsConditional` for testing, or tests in same package.

#### `internal/storage/pgstore_test.go`

| Test | What to verify |
|------|---------------|
| TestBuildQuery_Empty | No filters → "SELECT ... FROM metrics ORDER BY time DESC", no args |
| TestBuildQuery_InstanceOnly | instance_id filter → WHERE instance_id = $1 |
| TestBuildQuery_MetricPrefix | metric filter → WHERE metric LIKE $1 (with % appended) |
| TestBuildQuery_TimeRange | Start + End → WHERE time >= $1 AND time <= $2 |
| TestBuildQuery_WithLabels | Labels filter → WHERE labels @> $1::jsonb |
| TestBuildQuery_WithLimit | Limit set → LIMIT $N at end |
| TestBuildQuery_AllFilters | All fields set → verify correct arg count and order |
| TestPGStore_Write_EmptySlice | Empty input → nil error, no panic |
| TestPGStore_Write_NilLabels | Point with nil Labels → marshals as {} |

#### `internal/storage/pool_test.go`

| Test | What to verify |
|------|---------------|
| TestNewPool_InvalidDSN | Garbage DSN → returns error |

---

## Checklist Before Finishing

- [ ] `internal/storage/migrations/` directory contains both .sql files
- [ ] `//go:embed migrations/*.sql` is in migrate.go (not pgstore.go)
- [ ] `embed` is imported in migrate.go
- [ ] PGStore implements all 3 methods of collector.MetricStore (Write, Query, Close)
- [ ] buildQuery() is testable independently
- [ ] main.go imports storage package
- [ ] main.go falls back to LogStore when DSN is empty
- [ ] No string concatenation of user input in SQL (all $N params)
- [ ] CopyFrom uses correct column order matching the CREATE TABLE
- [ ] No bash commands attempted

## Output

List all files created/modified so the developer can run:
```bash
go mod tidy
go build ./...
go vet ./...
golangci-lint run
go test -v ./internal/storage/
```
