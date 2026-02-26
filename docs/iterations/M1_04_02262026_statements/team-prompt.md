# M1_04 Team Prompt — pg_stat_statements Collectors

**Iteration:** M1_04
**Date:** 2026-02-26
**Recommended mode:** Single Claude Code session (Sonnet). Scope is 2 collectors + 2 test files + 1 small helper. No version gates. Agent Teams overhead not justified.

---

## Prompt for Claude Code

```
Read CLAUDE.md for project context, then read docs/iterations/M1_04_.../design.md
for the full specification of this iteration.

Implement the pg_stat_statements collectors for PGPulse. Here is the complete spec:

### Context

- Module: github.com/ios9000/PGPulse_01
- Package: internal/collector
- Existing patterns: see base.go (Base struct, point(), queryContext()), and any
  existing collector (e.g., io_stats.go, checkpoint.go) for the established pattern.
- Interfaces: collector.go (Collector, MetricPoint, InstanceContext)
- This iteration adds 2 collectors + a shared helper function.

### Task 1: Add pgssAvailable() helper to base.go

Add a package-level function to base.go:

```go
// pgssAvailable checks whether the pg_stat_statements extension is installed.
func pgssAvailable(ctx context.Context, conn *pgx.Conn) (bool, error) {
    qctx, cancel := queryContext(ctx)
    defer cancel()
    var exists bool
    err := conn.QueryRow(qctx,
        `SELECT EXISTS(SELECT 1 FROM pg_extension WHERE extname = 'pg_stat_statements')`,
    ).Scan(&exists)
    if err != nil {
        return false, fmt.Errorf("check pgss availability: %w", err)
    }
    return exists, nil
}
```

### Task 2: Create internal/collector/statements_config.go

StatementsConfigCollector — collects pgss health metrics.

Name: "statements_config"
Interval: 60s

Collect() flow:
1. Call pgssAvailable(). If false → return nil, nil.
2. Query pg_settings for pg_stat_statements.max, pg_stat_statements.track, track_io_timing.
3. Query SELECT count(*) FROM pg_stat_statements.
4. Compute fill_pct = count / max * 100.
5. Query pg_stat_statements_info for stats_reset and EXTRACT(EPOCH FROM now() - stats_reset).
6. Emit metrics:
   - pgpulse.statements.max (gauge, float64 of the max setting)
   - pgpulse.statements.track (value=1, label {value: "all"|"top"|"none"})
   - pgpulse.statements.track_io_timing (1 if "on", 0 if "off")
   - pgpulse.statements.count (the count)
   - pgpulse.statements.fill_pct (derived)
   - pgpulse.statements.stats_reset_age_seconds (from info view)

Edge cases:
- If stats_reset is NULL, skip the stats_reset_age_seconds metric.
- Encode track_io_timing: "on" → 1, anything else → 0.
- If max is 0 or missing, skip fill_pct to avoid division by zero.

### Task 3: Create internal/collector/statements_top.go

StatementsTopCollector — top-N queries by total execution time.

Name: "statements_top"
Interval: 60s
Field: limit int (default 20)

SQL (single query):
```sql
WITH ranked AS (
    SELECT
        queryid::text                                        AS queryid,
        dbid::text                                           AS dbid,
        userid::text                                         AS userid,
        calls,
        rows,
        total_exec_time                                      AS total_time_ms,
        blk_read_time + blk_write_time                       AS io_time_ms,
        total_exec_time - blk_read_time - blk_write_time     AS cpu_time_ms,
        total_exec_time / calls                              AS avg_time_ms,
        ROW_NUMBER() OVER (ORDER BY total_exec_time DESC)    AS rn
    FROM pg_stat_statements
    WHERE calls > 0
),
totals AS (
    SELECT
        sum(calls)::float8          AS total_calls,
        sum(rows)::float8           AS total_rows,
        sum(total_time_ms)::float8  AS total_time,
        sum(io_time_ms)::float8     AS total_io,
        sum(cpu_time_ms)::float8    AS total_cpu
    FROM ranked
)
SELECT
    r.queryid,
    r.dbid,
    r.userid,
    r.calls::float8,
    r.rows::float8,
    r.total_time_ms,
    r.io_time_ms,
    r.cpu_time_ms,
    r.avg_time_ms,
    t.total_calls,
    t.total_rows,
    t.total_time,
    t.total_io,
    t.total_cpu
FROM ranked r
CROSS JOIN totals t
WHERE r.rn <= $1
ORDER BY r.rn
```

Collect() flow:
1. Call pgssAvailable(). If false → return nil, nil.
2. Execute query with c.limit as $1 parameter.
3. For each row:
   a. Emit 6 metrics with labels {queryid, dbid, userid}:
      - pgpulse.statements.top.total_time_ms
      - pgpulse.statements.top.io_time_ms
      - pgpulse.statements.top.cpu_time_ms
      - pgpulse.statements.top.calls
      - pgpulse.statements.top.rows
      - pgpulse.statements.top.avg_time_ms
   b. Accumulate sums for "other" computation.
   c. Capture totals (same on every row from CROSS JOIN).
4. Compute "other" bucket: totals - sum(top-N).
5. If otherCalls > 0, emit 6 "other" metrics with labels {queryid: "other", dbid: "all", userid: "all"}.
6. Clamp cpu_time_ms to 0 if negative.

Edge cases:
- Zero rows in pgss → return empty slice, nil.
- Fewer than limit rows → no "other" bucket.
- Negative cpu_time after subtraction → clamp to 0.

### Task 4: Create internal/collector/statements_config_test.go

Tests using the established mock pattern from testutil_test.go:

- TestStatementsConfigCollector_PgssNotInstalled → returns nil, nil
- TestStatementsConfigCollector_Normal → 6 metrics with correct values
- TestStatementsConfigCollector_NullStatsReset → 5 metrics (skips reset_age)
- TestStatementsConfigCollector_TrackIoTimingOn → value = 1
- TestStatementsConfigCollector_TrackIoTimingOff → value = 0

### Task 5: Create internal/collector/statements_top_test.go

Tests:

- TestStatementsTopCollector_PgssNotInstalled → returns nil, nil
- TestStatementsTopCollector_EmptyPgss → empty slice, nil
- TestStatementsTopCollector_NormalTopN → correct metric count (N*6 + 6 for other)
- TestStatementsTopCollector_FewerThanLimit → no "other" bucket
- TestStatementsTopCollector_NegativeCpuTime → clamped to 0
- TestStatementsTopCollector_OtherBucketArithmetic → verify other = totals - sum(top)

### Rules

- All SQL via pgx parameterized queries (no string concatenation).
- Use queryContext() for 5s timeout on every query.
- Follow Base struct pattern from base.go.
- Follow existing test patterns from testutil_test.go.
- Ensure golangci-lint v2.10.1 clean.
- Do NOT register collectors in main.go — developer will do this manually.
- Do NOT modify collector.go interfaces.

### ⚠️ Platform Note
You CANNOT run bash commands (go build, go test, etc.) on this platform.
Create all files only. The developer will run build and test commands manually.

List all files created/modified when done so the developer can run:
  go build ./...
  go vet ./...
  golangci-lint run
  go test ./internal/collector/... -v -run Statements
```

---

## Expected Output Files

| File | Action | Description |
|------|--------|-------------|
| `internal/collector/base.go` | Modified | Add pgssAvailable() function |
| `internal/collector/statements_config.go` | New | StatementsConfigCollector |
| `internal/collector/statements_top.go` | New | StatementsTopCollector |
| `internal/collector/statements_config_test.go` | New | 5 unit tests |
| `internal/collector/statements_top_test.go` | New | 6 unit tests |

## Post-Agent Developer Steps

```bash
cd C:\Users\Archer\Projects\PGPulse_01

# Build
go mod tidy
go build ./...
go vet ./...

# Lint
golangci-lint run

# Test (statements collectors only)
go test ./internal/collector/... -v -run Statements

# Full test suite
go test ./internal/collector/... -v

# If all green:
git add internal/collector/
git commit -m "feat(collector): add pg_stat_statements collectors (Q48-Q51)"
git push
```
