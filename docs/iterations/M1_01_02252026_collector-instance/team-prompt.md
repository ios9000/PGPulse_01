# PGPulse — M1_01 Team Prompt: Instance Metrics Collector

> **Paste this entire file into Claude Code.**
> **Platform:** Windows — agents CANNOT run bash. All shell commands run by developer manually.

---

Read CLAUDE.md for project context. Read docs/iterations/M1_01/design.md for
detailed implementation specifications including exact SQL, struct definitions,
and version gate logic.

Reference docs/legacy/PGAM_FEATURE_AUDIT.md (in Project Knowledge) for the
original PHP queries being ported.

## Goal

Build the first real metric collectors for PGPulse. Port PGAM queries 2–19
(skipping Q4–Q8 OS metrics) into Go collectors that implement the Collector
interface defined in internal/collector/collector.go (created in M0).

This proves the architecture end-to-end: SQL → MetricPoint → Registry.

## Constraints

- Agents CANNOT run shell commands on this platform (Windows bash bug)
- Agents create and edit .go files ONLY
- Developer will run go build, go test, golangci-lint, and git commit manually
- At the end, list ALL files created so developer can run the build

## Team: 2 Specialists

### SPECIALIST 1 — COLLECTOR AGENT

You own: internal/collector/*
You reference: internal/version/* (read-only, created in M0), design.md

Create these files in order:

#### 1. internal/collector/base.go

Shared base struct and helpers. All collectors embed this.

```go
package collector

type Base struct {
    instanceID string
    pgVersion  version.PGVersion
    interval   time.Duration
}
```

Provide:
- `newBase(instanceID, version, interval) Base`
- `(b *Base) point(metric string, value float64, labels map[string]string) MetricPoint`
  — prefixes metric with "pgpulse.", fills InstanceID and Timestamp automatically
- `(b *Base) Interval() time.Duration`
- `queryContext(ctx) (context.Context, CancelFunc)` — returns child context with 5s timeout

Import time, context, and the project's version and collector packages.

#### 2. internal/collector/server_info.go

ServerInfoCollector — Q2, Q3, Q9, Q10. Interval: 60s.

Queries:
```sql
-- Q2: start time
SELECT extract(epoch FROM pg_postmaster_start_time())::bigint AS start_epoch

-- Q9: recovery state
SELECT pg_is_in_recovery() AS is_recovery

-- Q10: backup state (PG 14 ONLY)
SELECT pg_is_in_backup()::int AS is_backup
```

Collect() logic:
1. Query start_epoch → emit "server.start_time_unix"
2. Compute uptime in Go: time.Now().Unix() - start_epoch → emit "server.uptime_seconds"
3. Query is_recovery → emit "server.is_in_recovery" (1.0 or 0.0)
4. Version gate for backup:
   - If pgVersion.Num < 150000: query pg_is_in_backup() → emit "server.is_in_backup"
   - Else: emit "server.is_in_backup" = 0.0 (function removed in PG 15)

Use the Gate pattern from internal/version/gate.go for the backup query:
```go
var backupStateGate = version.Gate{
    Name: "backup_state",
    Variants: []version.SQLVariant{
        {MinVersion: 140000, MaxVersion: 149999, SQL: "SELECT pg_is_in_backup()::int AS is_backup"},
    },
}
```

All queries use pgx conn.QueryRow with the timeout context from queryContext().

#### 3. internal/collector/connections.go

ConnectionsCollector — Q11, Q12, Q13 (enhanced). Interval: 10s.

Queries:
```sql
-- Q11 enhanced: by state, excluding self
SELECT COALESCE(state, 'unknown') AS state, count(*) AS cnt
FROM pg_stat_activity
WHERE pid != pg_backend_pid()
  AND backend_type = 'client backend'
GROUP BY 1

-- Q12 + Q13: max and reserved
SELECT
    current_setting('max_connections')::int AS max_conn,
    current_setting('superuser_reserved_connections')::int AS reserved
```

Collect() logic:
1. Query state breakdown → for each row: emit "connections.by_state" with label {state: <state>}
2. Sum all rows → emit "connections.total"
3. Query max + reserved → emit "connections.max" and "connections.superuser_reserved"
4. Compute utilization = total / (max - reserved) × 100 → emit "connections.utilization_pct"

PGAM bug fix: WHERE pid != pg_backend_pid() excludes PGPulse's own connection.

#### 4. internal/collector/cache.go

CacheCollector — Q14. Interval: 60s.

Query:
```sql
SELECT COALESCE(
    sum(blks_hit) * 100.0 / NULLIF(sum(blks_hit) + sum(blks_read), 0),
    0
) AS hit_ratio
FROM pg_stat_database
```

Emit: "cache.hit_ratio_pct"

PGAM bug fix: NULLIF + COALESCE guard against division by zero on fresh instances.

#### 5. internal/collector/transactions.go

TransactionsCollector — Q15 (enhanced). Interval: 60s.

Query:
```sql
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
  AND s.xact_commit + s.xact_rollback > 0
```

Collect() logic: for each row emit:
- "transactions.commit_ratio_pct" with label {database: datname}
- "transactions.deadlocks" with label {database: datname}

Enhancement over PGAM: per-database breakdown + deadlock counts.

#### 6. internal/collector/database_sizes.go

DatabaseSizesCollector — Q16. Interval: 300s.

Query:
```sql
SELECT datname, pg_database_size(datname) AS size_bytes
FROM pg_database
WHERE datistemplate = false
ORDER BY size_bytes DESC
```

Emit: "database.size_bytes" with label {database: datname} for each row.

#### 7. internal/collector/settings.go

SettingsCollector — Q17 (extended). Interval: 300s.

Query:
```sql
SELECT name, setting
FROM pg_settings
WHERE name IN (
    'track_io_timing',
    'shared_buffers',
    'max_locks_per_transaction',
    'max_prepared_transactions'
)
```

Use a mapping table to convert values:
- "track_io_timing": "on" → 1.0, "off" → 0.0
- All others: parse setting as float64

Emit: "settings.track_io_timing", "settings.shared_buffers_8kb",
"settings.max_locks_per_tx", "settings.max_prepared_tx"

Design the mapping as a Go map so adding new settings requires only one line.

#### 8. internal/collector/extensions.go

ExtensionsCollector — Q18, Q19. Interval: 300s.

Queries:
```sql
-- Q18: pg_stat_statements installed?
SELECT count(*) FROM pg_extension WHERE extname = 'pg_stat_statements'

-- Q19: fill percentage (only if Q18 > 0)
SELECT count(*) * 100.0 / current_setting('pg_stat_statements.max')::float AS fill_pct
FROM pg_stat_statements

-- Q19b: stats reset (PG >= 14 with pgss installed)
SELECT extract(epoch FROM stats_reset)::bigint AS reset_epoch
FROM pg_stat_statements_info
```

Collect() logic:
1. Query Q18 → emit "extensions.pgss_installed" (1.0 or 0.0)
2. If installed:
   - Query Q19 → emit "extensions.pgss_fill_pct"
   - Query Q19b → emit "extensions.pgss_stats_reset_unix"
3. If Q19 or Q19b fail, log warning and continue (don't abort collector)

#### 9. internal/collector/registry.go

Registry — orchestrates all collectors.

```go
type Registry struct {
    collectors []Collector
}
```

Provide:
- `NewRegistry() *Registry`
- `(r *Registry) Register(c Collector)`
- `(r *Registry) CollectAll(ctx context.Context, conn *pgx.Conn) []MetricPoint`

CollectAll() logic:
1. Iterate collectors sequentially (NOT parallel — single connection)
2. For each: create 5s timeout context, call Collect(), measure duration
3. On error: log with slog.Error (collector name, error, duration) and CONTINUE
4. On success: log with slog.Debug (collector name, point count, duration)
5. Append successful points to results
6. Return combined results

Critical: one collector failing must NOT abort the batch.

#### Rules for Collector Agent

- ALL SQL uses pgx parameterized queries — ZERO string concatenation
- ALL queries go through queryContext() for the 5s timeout
- ALL metric names prefixed with "pgpulse." via base.point()
- Wrap errors with fmt.Errorf("<function>: %w", err)
- Every exported type and function gets a GoDoc comment
- Do NOT import or reference internal/api/, internal/auth/, or internal/storage/
- Do NOT create any test files — that's the QA Agent's job

---

### SPECIALIST 2 — QA AGENT

You own: all *_test.go files in internal/collector/
You reference: internal/collector/*.go (read after Collector Agent creates them),
internal/version/* (from M0)

Create these files:

#### 1. internal/collector/testutil_test.go

Shared test infrastructure. Build tag: `//go:build integration`

Provide:
- `setupPG(t *testing.T, pgVersion string) *pgx.Conn` — starts a testcontainers
  PostgreSQL container, returns a connected pgx.Conn, registers cleanup
- `setupPGWithStatements(t *testing.T, pgVersion string) *pgx.Conn` — same but
  with pg_stat_statements enabled (shared_preload_libraries or CREATE EXTENSION)
- `metricNames(points []MetricPoint) []string` — extracts metric names
- `findMetric(points []MetricPoint, name string) *MetricPoint` — finds first match
- `findMetricWithLabel(points []MetricPoint, name, labelKey, labelValue string) *MetricPoint`

Use testcontainers-go/modules/postgres for container setup.

#### 2. internal/collector/server_info_test.go

Build tag: `//go:build integration`

Tests:
- `TestServerInfoCollector_PG17`: verify all 4 metrics present, start_time > 2020,
  uptime > 0, is_in_backup = 0.0 (function doesn't exist on PG 17)
- `TestServerInfoCollector_PG14`: verify pg_is_in_backup() executes without error,
  returns 0.0 (no backup running)
- `TestServerInfoCollector_Name`: verify Name() returns "server_info"
- `TestServerInfoCollector_Interval`: verify Interval() returns 60s

#### 3. internal/collector/connections_test.go

Build tag: `//go:build integration`

Tests:
- `TestConnectionsCollector_PG17`: verify metrics present (by_state, total, max,
  superuser_reserved, utilization_pct), total > 0, max > 0
- `TestConnectionsCollector_ExcludesSelf`: verify total count does NOT include
  the test connection itself (connect twice, check count equals 1 not 2... or
  verify count is at least plausible — the key is that pg_backend_pid() is excluded)
- `TestConnectionsCollector_Utilization`: verify utilization_pct is between 0 and 100

#### 4. internal/collector/cache_test.go

Build tag: `//go:build integration`

Tests:
- `TestCacheCollector_PG17`: verify hit_ratio_pct metric present, value between 0 and 100
- `TestCacheCollector_Name`: verify Name() returns "cache"

#### 5. internal/collector/transactions_test.go

Build tag: `//go:build integration`

Tests:
- `TestTransactionsCollector_PG17`: verify commit_ratio_pct and deadlocks metrics
  present with database labels
- `TestTransactionsCollector_Name`: verify Name() returns "transactions"

#### 6. internal/collector/database_sizes_test.go

Build tag: `//go:build integration`

Tests:
- `TestDatabaseSizesCollector_PG17`: verify size_bytes metrics present with database
  labels, values > 0
- `TestDatabaseSizesCollector_Name`: verify Name() returns "database_sizes"

#### 7. internal/collector/settings_test.go

Build tag: `//go:build integration`

Tests:
- `TestSettingsCollector_PG17`: verify all 4 settings metrics present
  (track_io_timing, shared_buffers_8kb, max_locks_per_tx, max_prepared_tx)
- `TestSettingsCollector_BoolConversion`: verify track_io_timing is 1.0 or 0.0
- `TestSettingsCollector_Name`: verify Name() returns "settings"

#### 8. internal/collector/extensions_test.go

Build tag: `//go:build integration`

Tests:
- `TestExtensionsCollector_WithPGSS`: use setupPGWithStatements, verify
  pgss_installed = 1.0, pgss_fill_pct between 0 and 100,
  pgss_stats_reset_unix present
- `TestExtensionsCollector_WithoutPGSS`: use setupPG (no pgss), verify
  pgss_installed = 0.0, no fill_pct or stats_reset metrics emitted
- `TestExtensionsCollector_Name`: verify Name() returns "extensions"

#### 9. internal/collector/registry_test.go

This file does NOT need testcontainers. No build tag needed.

Tests:
- `TestRegistry_CollectAll_Success`: register 2 mock collectors that return
  known MetricPoints, verify CollectAll returns all points combined
- `TestRegistry_CollectAll_PartialFailure`: register one failing mock + one
  passing mock, verify only the passing mock's points are returned
- `TestRegistry_CollectAll_Empty`: empty registry returns empty slice, no panic
- `TestRegistry_CollectAll_ContextCanceled`: pass a pre-canceled context, verify
  collectors receive it (they may error, but should not hang)

For mock collectors in registry tests:

```go
type mockCollector struct {
    name     string
    points   []MetricPoint
    err      error
    interval time.Duration
}

func (m *mockCollector) Name() string                                                    { return m.name }
func (m *mockCollector) Collect(ctx context.Context, conn *pgx.Conn) ([]MetricPoint, error) { return m.points, m.err }
func (m *mockCollector) Interval() time.Duration                                         { return m.interval }
```

#### Rules for QA Agent

- Every test file for collectors that hit the database gets `//go:build integration`
- registry_test.go uses mocks — no build tag, no testcontainers
- Use testify/require for fatal assertions, testify/assert for non-fatal
- Verify EVERY SQL query in each collector:
  - No string concatenation (no fmt.Sprintf with SQL)
  - Uses pgx conn.QueryRow or conn.Query (never conn.Exec for SELECT)
  - Has context timeout via queryContext()
- Verify every exported type/function has a GoDoc comment
- Do NOT modify any non-test files
- Do NOT create files outside internal/collector/

---

## Coordination

Dependencies:
1. Collector Agent starts immediately — creates all production files
2. QA Agent can start test stubs (testutil, mock types, test function signatures)
   immediately, then fills in assertions once Collector Agent's files are committed

Shared contract: both agents use the Collector interface and MetricPoint struct
from internal/collector/collector.go (created in M0 — do not modify).

## Completion Checklist

When both agents are done, list ALL files created:

```
internal/collector/base.go
internal/collector/server_info.go
internal/collector/connections.go
internal/collector/cache.go
internal/collector/transactions.go
internal/collector/database_sizes.go
internal/collector/settings.go
internal/collector/extensions.go
internal/collector/registry.go
internal/collector/testutil_test.go
internal/collector/server_info_test.go
internal/collector/connections_test.go
internal/collector/cache_test.go
internal/collector/transactions_test.go
internal/collector/database_sizes_test.go
internal/collector/settings_test.go
internal/collector/extensions_test.go
internal/collector/registry_test.go
```

Developer will then run manually:
```
go mod tidy
go build ./...
go vet ./...
golangci-lint run
go test -tags integration ./internal/collector/...
```

If build errors occur, developer will paste them back for fixes.
