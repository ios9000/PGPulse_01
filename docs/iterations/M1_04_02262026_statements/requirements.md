# M1_04 Requirements — pg_stat_statements Collectors

**Iteration:** M1_04
**Date:** 2026-02-26
**PGAM Queries:** Q48, Q49, Q50, Q51 (Q52 deferred to M2/API)

---

## Objective

Implement two collectors that monitor pg_stat_statements health and capture top-N query performance metrics by I/O and CPU time. This completes the pg_stat_statements portion of the M1 milestone (instance-level collectors).

## Scope

### In Scope

1. **StatementsConfigCollector** (`statements_config.go`)
   - PGAM Q48: pg_stat_statements settings (max, track, track_io_timing) + fill percentage
   - PGAM Q49: stats reset age from pg_stat_statements_info

2. **StatementsTopCollector** (`statements_top.go`)
   - PGAM Q50: top queries by I/O time (blk_read_time + blk_write_time)
   - PGAM Q51: top queries by CPU time (total_exec_time - IO time)
   - Combined into a single query sorted by total_exec_time DESC, LIMIT 20
   - Includes "other" bucket aggregating remaining queries

3. **Shared pgss availability check**
   - Helper function usable by both collectors
   - Returns `nil, nil` when pg_stat_statements is not installed

4. **Unit tests** for both collectors

### Out of Scope

- PGAM Q52 (normalized text report) — deferred to M2/API layer. The per-queryid metrics from StatementsTopCollector already capture the underlying data; the formatted report is a presentation concern.
- Query text in metric labels — queryid is the identifier, query text is a lookup dimension for the API layer.
- PG < 14 code paths — our minimum is PG 14, so `total_exec_time` and `pg_stat_statements_info` are always available.

## Functional Requirements

### FR-1: pgss Availability Check

| ID | Requirement |
|----|-------------|
| FR-1.1 | Both collectors MUST check pg_stat_statements extension presence before querying |
| FR-1.2 | If pgss is not installed, collectors MUST return `nil, nil` (not an error) |
| FR-1.3 | Check uses `SELECT EXISTS(SELECT 1 FROM pg_extension WHERE extname = 'pg_stat_statements')` |
| FR-1.4 | Check runs once per Collect() call (acceptable overhead at 60s intervals) |

### FR-2: StatementsConfigCollector (Q48 + Q49)

| ID | Requirement |
|----|-------------|
| FR-2.1 | Query pg_settings for: `pg_stat_statements.max`, `pg_stat_statements.track`, `track_io_timing` |
| FR-2.2 | Query `SELECT count(*) FROM pg_stat_statements` for current row count |
| FR-2.3 | Compute fill percentage: `count / max * 100` |
| FR-2.4 | Query `pg_stat_statements_info` for `stats_reset` timestamp |
| FR-2.5 | Compute stats reset age in seconds: `EXTRACT(EPOCH FROM now() - stats_reset)` |
| FR-2.6 | Collector interval: 60 seconds |
| FR-2.7 | Collector name: `"statements_config"` |

**Metrics emitted:**

| Metric | Type | Labels | Source |
|--------|------|--------|--------|
| `pgpulse.statements.max` | gauge | — | pg_settings |
| `pgpulse.statements.track` | gauge (encoded) | `{value: "all"\|"top"\|"none"}` | pg_settings |
| `pgpulse.statements.track_io_timing` | gauge | — | pg_settings (1=on, 0=off) |
| `pgpulse.statements.count` | gauge | — | count(*) from pgss |
| `pgpulse.statements.fill_pct` | gauge | — | derived |
| `pgpulse.statements.stats_reset_age_seconds` | gauge | — | pg_stat_statements_info |

Note on `track`: The setting value is a string. Emit as a metric point with value `1` and the actual value in a label. This follows the pattern used by settings.go.

### FR-3: StatementsTopCollector (Q50 + Q51)

| ID | Requirement |
|----|-------------|
| FR-3.1 | Query pg_stat_statements for top 20 queries by `total_exec_time DESC` |
| FR-3.2 | For each query, compute: total_time, io_time, cpu_time, calls, rows, avg_time |
| FR-3.3 | Use `queryid` and `dbid` as metric labels (NOT query text) |
| FR-3.4 | Compute "other" bucket: aggregate of all queries outside top 20 |
| FR-3.5 | Filter: `WHERE calls > 0` to exclude never-executed entries |
| FR-3.6 | Collector interval: 60 seconds |
| FR-3.7 | Collector name: `"statements_top"` |
| FR-3.8 | If pgss has zero rows, return empty slice (not error) |

**Metrics emitted per queryid (top 20 + "other"):**

| Metric | Type | Labels | Notes |
|--------|------|--------|-------|
| `pgpulse.statements.top.total_time_ms` | gauge | `{queryid, dbid, userid}` | total_exec_time for this queryid |
| `pgpulse.statements.top.io_time_ms` | gauge | `{queryid, dbid, userid}` | blk_read_time + blk_write_time |
| `pgpulse.statements.top.cpu_time_ms` | gauge | `{queryid, dbid, userid}` | total_exec_time - io_time |
| `pgpulse.statements.top.calls` | gauge | `{queryid, dbid, userid}` | call count |
| `pgpulse.statements.top.rows` | gauge | `{queryid, dbid, userid}` | rows returned/affected |
| `pgpulse.statements.top.avg_time_ms` | gauge | `{queryid, dbid, userid}` | total_exec_time / calls |

For the "other" bucket, labels are `{queryid: "other", dbid: "all", userid: "all"}`.

## Non-Functional Requirements

| ID | Requirement |
|----|-------------|
| NFR-1 | All SQL via pgx parameterized queries — no string concatenation |
| NFR-2 | statement_timeout enforced via queryContext() (5s) |
| NFR-3 | application_name set on connection: `pgpulse_statements_config` / `pgpulse_statements_top` |
| NFR-4 | No version gates needed (PG 14+ only), but structure code to allow future gates |
| NFR-5 | Unit tests cover: pgss present, pgss absent, zero rows, normal data |
| NFR-6 | golangci-lint clean with v2.10.1 config |

## PGAM Bugs / Improvements

| PGAM Issue | PGPulse Fix |
|------------|------------|
| Q48: No pgss availability check — crashes if extension missing | Check before querying, return nil |
| Q50/Q51: Two separate queries hitting pgss twice | Single query computing both IO and CPU |
| Q52: PHP regex normalization of query text | Unnecessary — PG 14+ queryid handles this natively |
| Q50/Q51: Uses 0.5% threshold (variable result set size) | Fixed LIMIT 20 — predictable cardinality |
| Q50/Q51: No handling of division by zero (calls=0) | WHERE calls > 0 filter |
