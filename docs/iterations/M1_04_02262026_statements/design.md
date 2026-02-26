# M1_04 Design — pg_stat_statements Collectors

**Iteration:** M1_04
**Date:** 2026-02-26
**Based on:** requirements.md for M1_04

---

## 1. Files to Create

| File | Purpose | Test File |
|------|---------|-----------|
| `internal/collector/statements_config.go` | Q48+Q49: pgss settings, fill%, reset age | `statements_config_test.go` |
| `internal/collector/statements_top.go` | Q50+Q51: top-N by IO and CPU time | `statements_top_test.go` |

No new version gate entries needed — all SQL works identically on PG 14–17.

## 2. Shared Helper: pgss Availability Check

Add to `base.go` (or as a package-level function in a small helper):

```go
// pgssAvailable checks whether pg_stat_statements extension is installed.
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

Both statements collectors call this at the top of `Collect()`. If `false`, return `nil, nil`.

## 3. StatementsConfigCollector

### Struct

```go
type StatementsConfigCollector struct {
	Base
}

func NewStatementsConfigCollector(instanceID string, v version.PGVersion) *StatementsConfigCollector {
	return &StatementsConfigCollector{
		Base: newBase(instanceID, v, 60*time.Second),
	}
}

func (c *StatementsConfigCollector) Name() string { return "statements_config" }
```

### SQL

**Settings query** (single query, 3 rows):
```sql
SELECT name, setting
FROM pg_settings
WHERE name IN ('pg_stat_statements.max', 'pg_stat_statements.track', 'track_io_timing')
```

**Row count query:**
```sql
SELECT count(*) FROM pg_stat_statements
```

**Stats reset query:**
```sql
SELECT
    stats_reset,
    EXTRACT(EPOCH FROM now() - stats_reset)::float8 AS age_seconds
FROM pg_stat_statements_info
```

### Collect() Logic

```
1. pgssAvailable() → false? return nil, nil
2. Query pg_settings → parse into map[name]setting
3. Query count(*) → stmtCount
4. Parse max from settings map → compute fillPct = count/max*100
5. Query pg_stat_statements_info → statsReset, ageSeconds
6. Build metrics slice:
   - point("pgpulse.statements.max", float64(max), nil)
   - point("pgpulse.statements.track", 1, map{"value": trackValue})
   - point("pgpulse.statements.track_io_timing", 1 or 0, nil)
   - point("pgpulse.statements.count", float64(count), nil)
   - point("pgpulse.statements.fill_pct", fillPct, nil)
   - point("pgpulse.statements.stats_reset_age_seconds", ageSeconds, nil)
7. Return metrics, nil
```

**Edge cases:**
- `pg_stat_statements.max` setting missing → skip fill_pct metric (emit count only)
- `pg_stat_statements_info` returns NULL stats_reset → skip stats_reset_age_seconds metric
- Settings query returns fewer than 3 rows → emit only what's available

### track_io_timing Encoding

The `track_io_timing` setting value is `"on"` or `"off"`. Convert:
```go
var val float64
if setting == "on" {
    val = 1
}
// val stays 0 for "off"
```

## 4. StatementsTopCollector

### Struct

```go
type StatementsTopCollector struct {
	Base
	limit int // top-N, default 20
}

func NewStatementsTopCollector(instanceID string, v version.PGVersion) *StatementsTopCollector {
	return &StatementsTopCollector{
		Base:  newBase(instanceID, v, 60*time.Second),
		limit: 20,
	}
}

func (c *StatementsTopCollector) Name() string { return "statements_top" }
```

### SQL — Main Query

Single query retrieving top-N plus aggregated totals for computing "other":

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

The `$1` parameter is the limit (20).

### "Other" Bucket Computation

After scanning all top-N rows, compute "other" by subtraction:

```go
otherCalls := totalCalls - sumTopCalls
otherRows  := totalRows  - sumTopRows
otherTotal := totalTime   - sumTopTotal
otherIO    := totalIO     - sumTopIO
otherCPU   := totalCPU    - sumTopCPU
```

Emit one set of metrics with labels `{queryid: "other", dbid: "all", userid: "all"}`.
Only emit the "other" bucket if `otherCalls > 0`.

### Collect() Logic

```
1. pgssAvailable() → false? return nil, nil
2. Execute main query with limit parameter
3. Iterate rows:
   a. For each row, emit 6 metrics with labels {queryid, dbid, userid}
   b. Accumulate sums for "other" computation
   c. Capture totals from CROSS JOIN (same on every row)
4. Compute "other" bucket by subtraction
5. If otherCalls > 0, emit 6 "other" metrics
6. Return metrics, nil
```

**Edge cases:**
- pgss has zero rows → query returns nothing → return empty slice, nil
- Fewer than 20 rows in pgss → return only what exists, no "other" bucket
- Negative cpu_time_ms (theoretically possible if IO timing is off and blk times are 0) → clamp to 0

### Metric Labels

```go
labels := map[string]string{
    "queryid": row.queryID,
    "dbid":    row.dbID,
    "userid":  row.userID,
}
```

Using string representations of the numeric IDs. The API layer can resolve `dbid` → database name and `userid` → role name later via `pg_database` and `pg_roles`.

## 5. Registration

Both collectors registered explicitly in `main.go` (per decision #16 from handoff):

```go
// In main.go or wherever collectors are registered:
collectors = append(collectors,
    collector.NewStatementsConfigCollector(instanceID, pgVer),
    collector.NewStatementsTopCollector(instanceID, pgVer),
)
```

## 6. Test Design

### statements_config_test.go

| Test | Scenario | Expectation |
|------|----------|-------------|
| TestStatementsConfigCollector_PgssNotInstalled | Mock: pgss availability returns false | nil, nil |
| TestStatementsConfigCollector_Normal | Mock: settings + count + info returned | 6 metrics emitted |
| TestStatementsConfigCollector_NullStatsReset | Mock: stats_reset is NULL | 5 metrics (skip reset_age) |
| TestStatementsConfigCollector_TrackIoTimingOff | Mock: track_io_timing = "off" | metric value = 0 |
| TestStatementsConfigCollector_TrackIoTimingOn | Mock: track_io_timing = "on" | metric value = 1 |

### statements_top_test.go

| Test | Scenario | Expectation |
|------|----------|-------------|
| TestStatementsTopCollector_PgssNotInstalled | Mock: pgss availability returns false | nil, nil |
| TestStatementsTopCollector_EmptyPgss | Mock: query returns 0 rows | empty slice, nil |
| TestStatementsTopCollector_NormalTopN | Mock: 25 queries, limit 20 | 20×6 metrics + "other" bucket (6) = 126 |
| TestStatementsTopCollector_FewerThanLimit | Mock: 5 queries, limit 20 | 5×6 metrics, no "other" |
| TestStatementsTopCollector_OtherBucket | Mock: verify other = totals - sum(top) | Arithmetic check |
| TestStatementsTopCollector_NegativeCpuTime | Mock: cpu_time < 0 | Clamped to 0 |

### Mock Pattern

Follow existing testutil_test.go patterns. Mock `pgx.Conn` via the `pgxmock` approach used in other collector tests (or the pgx Rows interface mock pattern established in M1_01).

## 7. Metric Summary

### StatementsConfigCollector — 6 metrics

| Metric | Example Value |
|--------|--------------|
| pgpulse.statements.max | 5000 |
| pgpulse.statements.track | 1 (label: value=top) |
| pgpulse.statements.track_io_timing | 1 |
| pgpulse.statements.count | 342 |
| pgpulse.statements.fill_pct | 6.84 |
| pgpulse.statements.stats_reset_age_seconds | 86400.5 |

### StatementsTopCollector — up to 126 metrics (21 × 6)

| Metric | Example Value | Labels |
|--------|--------------|--------|
| pgpulse.statements.top.total_time_ms | 15432.7 | queryid=123, dbid=16384, userid=10 |
| pgpulse.statements.top.io_time_ms | 8921.3 | queryid=123, dbid=16384, userid=10 |
| pgpulse.statements.top.cpu_time_ms | 6511.4 | queryid=123, dbid=16384, userid=10 |
| pgpulse.statements.top.calls | 1500 | queryid=123, dbid=16384, userid=10 |
| pgpulse.statements.top.rows | 45000 | queryid=123, dbid=16384, userid=10 |
| pgpulse.statements.top.avg_time_ms | 10.29 | queryid=123, dbid=16384, userid=10 |

## 8. PGAM Comparison

| PGAM | PGPulse M1_04 | Change |
|------|---------------|--------|
| Q48: 2 separate queries, no pgss check | 3 queries with pgss availability guard | Safer |
| Q49: Only PG ≥ 14 | Always available (PG 14+ minimum) | Simpler |
| Q50: Separate IO sort query | Combined into single query | More efficient |
| Q51: Separate CPU sort query | Combined into single query | More efficient |
| Q52: Regex normalization in PHP | Deferred — queryid handles normalization | Cleaner |
| Q50/Q51: 0.5% threshold, variable result size | Fixed LIMIT 20, predictable cardinality | More predictable |
| Q50/Q51: Possible division by zero | WHERE calls > 0 guard | Bug fix |
| No pgss availability check | pgssAvailable() returns nil, nil | Bug fix |
