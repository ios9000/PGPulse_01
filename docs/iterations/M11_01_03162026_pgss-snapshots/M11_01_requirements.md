# M11_01 Requirements — PGSS Snapshots, Diff Engine, Query Insights API + Bug Fixes

**Iteration:** M11_01
**Date:** 2026-03-16
**Scope:** Backend foundation for Competitive Enrichment + 4 bug fixes

---

## Background

The competitive research synthesis identified pg_stat_statements snapshot diffs as the highest-impact, most tractable enrichment for PGPulse. The pg_profile paradigm — periodic snapshots of cumulative PGSS counters, diff between timepoints to show "what changed" — is the foundation for query insights, workload reports, and future query analysis features.

M11_01 delivers the full backend: snapshot capture, persistent storage, diff engine, query insights aggregation, workload report data generation, and all API endpoints. M11_02 (next iteration) will add the React frontend pages and HTML export.

---

## Functional Requirements

### FR-1: PGSS Snapshot Capture

- **FR-1.1:** Periodic capture of all rows from `pg_stat_statements` with metadata (instance_id, captured_at, pg_version, stats_reset timestamp, summary totals).
- **FR-1.2:** Configurable capture interval (default 30 minutes). Per-instance override possible.
- **FR-1.3:** Version-gated SQL: PG ≤12 uses `total_time`; PG 13+ uses `total_exec_time` + `total_plan_time` + WAL stats + min/max/mean/stddev_exec_time.
- **FR-1.4:** Capture records `stats_reset` from `pg_stat_statements_info` (PG 14+) to detect counter resets between snapshots.
- **FR-1.5:** Guard: only runs when `statement_snapshots.enabled = true` AND persistent storage is available.
- **FR-1.6:** Manual snapshot trigger via API endpoint.
- **FR-1.7:** Capture-on-startup option (configurable, default false).

### FR-2: Snapshot Storage

- **FR-2.1:** Migration 015 creates `pgss_snapshots` (metadata) and `pgss_snapshot_entries` (one row per query per snapshot) tables.
- **FR-2.2:** `pgss_snapshot_entries` stores: queryid, userid, dbid, query text, calls, total_exec_time, total_plan_time, rows, shared_blks_hit/read/dirtied/written, local_blks_hit/read, temp_blks_read/written, blk_read_time, blk_write_time, wal_records/fpi/bytes, mean/min/max/stddev_exec_time.
- **FR-2.3:** NULLable columns for version-dependent fields (PG ≤12 has no plan_time, WAL stats, or exec time stats).
- **FR-2.4:** Retention cleaner deletes snapshots older than `statement_snapshots.retention_days` (default 30).
- **FR-2.5:** NullSnapshotStore for live mode (no-op, safe).

### FR-3: Diff Engine

- **FR-3.1:** Given two snapshot IDs, compute per-query deltas for all numeric columns (calls, exec_time, plan_time, rows, blks, WAL, etc.).
- **FR-3.2:** Stats reset detection: if `stats_reset` timestamp differs between snapshots, flag the diff as unreliable and return a warning (don't skip — still compute, but mark).
- **FR-3.3:** Categorize queries: continuing (in both snapshots), new (only in "to"), evicted (only in "from").
- **FR-3.4:** Derive per-query: avg_exec_time_per_call, io_time_pct, cpu_time (exec − io), shared_hit_ratio.
- **FR-3.5:** Sort by configurable column (default: total_exec_time delta desc) with limit/offset pagination.
- **FR-3.6:** "Latest diff" convenience: automatically diff the two most recent snapshots for an instance.
- **FR-3.7:** Time-range diff: given a time range, pick the closest snapshots bracketing that range.

### FR-4: Query Insights (Per-Query History)

- **FR-4.1:** Given a queryid + instance_id + time range, return per-snapshot values across all snapshots in range.
- **FR-4.2:** Compute inter-snapshot deltas for time-series display (calls/interval, exec_time/interval, rows/interval).
- **FR-4.3:** Return query text, first_seen (earliest snapshot containing this queryid), database name, user name.

### FR-5: Workload Report Data

- **FR-5.1:** Given two snapshot IDs (or a time range), generate a structured report containing:
  - Summary: time range, total queries, total calls delta, total exec_time delta, stats_reset warning
  - Top-N queries by exec_time delta
  - Top-N queries by calls delta
  - Top-N queries by rows delta
  - Top-N queries by shared_blks_read delta (I/O heavy)
  - Top-N queries by mean_exec_time (slowest average)
  - New queries (appeared in "to" but not "from")
  - Evicted queries (in "from" but not "to")
- **FR-5.2:** Report is a Go data structure returned via API. Frontend rendering in M11_02.

### FR-6: API Endpoints

- `GET /api/v1/instances/{id}/snapshots` — list snapshots (paginated, time-range filter)
- `GET /api/v1/instances/{id}/snapshots/{snapId}` — get snapshot with entries (paginated, sortable)
- `GET /api/v1/instances/{id}/snapshots/diff` — diff between two snapshots (`?from=X&to=Y`, or `?from_time=&to_time=`)
- `GET /api/v1/instances/{id}/snapshots/latest-diff` — diff between last two snapshots
- `GET /api/v1/instances/{id}/query-insights/{queryid}` — per-query history across snapshots
- `GET /api/v1/instances/{id}/workload-report` — structured report data (`?from=X&to=Y` snapshot IDs)
- `POST /api/v1/instances/{id}/snapshots/capture` — manual snapshot trigger (instance_management permission)

### FR-7: Bug Fixes

- **FR-7.1:** Remove debug log line (`"remediation config"`) in `cmd/pgpulse-server/main.go`.
- **FR-7.2:** Fix `wastedibytes` float64→int64 scan error in `internal/collector/database.go` bloat sub-collector.
- **FR-7.3:** Add `pg.server.multixact_pct` metric emission to `ServerInfoCollector` (alert rules reference it but no collector emits it).
- **FR-7.4:** Fix `srsubstate` char(1) scan error in logical replication collector — use `*string` instead of `*byte` for char(1) column.

---

## Non-Functional Requirements

- **NFR-1:** Snapshot capture must not block the collection loop. Runs in its own goroutine with its own ticker (mirrors BackgroundEvaluator pattern).
- **NFR-2:** Snapshot capture SQL must use `statement_timeout = 30000` (30s) to avoid blocking on large PGSS.
- **NFR-3:** Entry storage should use COPY protocol (pgx CopyFrom) for bulk insert performance when entry count > 100.
- **NFR-4:** Diff computation happens in Go (not SQL) to keep the diff logic testable and version-aware.
- **NFR-5:** All new code has unit tests. Diff engine has table-driven tests covering: normal diff, stats_reset, new queries, evicted queries, PG 12 null columns.

---

## Configuration

```yaml
statement_snapshots:
  enabled: true
  interval: 30m          # capture interval
  retention_days: 30     # delete snapshots older than this
  capture_on_startup: false
  top_n: 50              # max entries to return in diff/report by default
```

---

## Out of Scope (M11_02)

- React "Query Insights" page with per-query trend charts
- React "Workload Report" page with sections + filters
- HTML export (server-rendered, downloadable)
- Sidebar navigation additions
- Query fingerprint normalization improvements
