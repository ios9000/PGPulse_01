# M11_01 Team Prompt — PGSS Snapshots, Diff Engine, Query Insights API + Bug Fixes

**Iteration:** M11_01
**Date:** 2026-03-16
**Agents:** 3 (Backend Specialist, API Specialist, Fix Specialist / QA)

---

## CONTEXT

PGPulse is a PostgreSQL monitoring platform. This iteration adds pg_stat_statements snapshot capture, a diff engine, query insights, and workload report data generation — the pg_profile paradigm applied to PGPulse. We also fix 4 known bugs.

**Key references in repo:**
- `internal/settings/` — established snapshot + diff + store pattern (mirror this)
- `internal/remediation/background.go` — BackgroundEvaluator pattern (mirror for SnapshotCapturer)
- `internal/collector/statements_top.go` — existing PGSS query pattern
- `internal/version/gate.go` — version-gated SQL selection
- `internal/storage/migrations/` — existing migrations (this is 015)

---

## DO NOT RE-DISCUSS

- **Storage schema:** Dedicated `pgss_snapshots` + `pgss_snapshot_entries` tables. NOT the metrics store.
- **Capture interval:** 30m default, configurable.
- **Diff in Go, not SQL:** Diff computation is pure Go for testability and version-awareness.
- **Version gating:** PG ≤12 uses `total_time`, PG 13+ uses `total_exec_time` + extra columns. Use `version.Gate`.
- **DB/user name resolution:** Resolved at capture time, stored as `database_name TEXT` and `user_name TEXT` in entries table.
- **Route order:** `/diff` and `/latest-diff` BEFORE `/{snapId}` in chi router.
- **COPY protocol:** Use `pgx.CopyFrom` for bulk insert when entry count > 100.
- **NullStore pattern:** Same as existing `NullRecommendationStore` — all methods return nil/empty.
- **Guard pattern:** `cfg.StatementSnapshots.Enabled && persistentStore != nil`

---

## TEAM STRUCTURE

### Agent 1 — Backend Specialist
**Owns:** `internal/statements/` package (all files)

Create the following files in order:

1. **`internal/statements/types.go`** — All type definitions: Snapshot, SnapshotEntry, DiffResult, DiffEntry, QueryInsight, QueryInsightPoint, WorkloadReport, ReportSummary, SnapshotListOptions, DiffOptions. See design §2.1 for complete field definitions. SnapshotEntry uses pointer types for PG 13+ fields (TotalPlanTime, WALRecords, WALFpi, WALBytes, MeanExecTime, MinExecTime, MaxExecTime, StddevExecTime). Add `DatabaseName string` and `UserName string` fields to SnapshotEntry.

2. **`internal/statements/store.go`** — SnapshotStore interface. Methods: WriteSnapshot, GetSnapshot, GetSnapshotEntries, ListSnapshots, GetLatestSnapshots, GetEntriesForQuery, CleanOld. See design §2.2.

3. **`internal/statements/nullstore.go`** — NullSnapshotStore implementing SnapshotStore with no-op methods. Return empty slices, nil errors.

4. **`internal/statements/pgstore.go`** — PGSnapshotStore implementing SnapshotStore using pgxpool.
   - `WriteSnapshot`: BEGIN tx → INSERT snapshot row (RETURNING id) → if len(entries) > 100 use `pgx.CopyFrom` else batch INSERT → COMMIT. Populate summary fields (total_statements, total_calls, total_exec_time) from entries.
   - `GetSnapshot`: SELECT from pgss_snapshots WHERE id = $1.
   - `GetSnapshotEntries`: SELECT from pgss_snapshot_entries WHERE snapshot_id = $1 with ORDER BY + LIMIT/OFFSET. Return total count.
   - `ListSnapshots`: SELECT from pgss_snapshots WHERE instance_id = $1 with optional time range, ORDER BY captured_at DESC, LIMIT/OFFSET. Return total count.
   - `GetLatestSnapshots`: SELECT from pgss_snapshots WHERE instance_id = $1 ORDER BY captured_at DESC LIMIT $2.
   - `GetEntriesForQuery`: JOIN pgss_snapshot_entries → pgss_snapshots WHERE queryid = $1 AND instance_id = $2 AND captured_at BETWEEN $3 AND $4. Return both entries and their parent snapshots (for stats_reset detection in insights).
   - `CleanOld`: DELETE FROM pgss_snapshots WHERE captured_at < $1 (CASCADE handles entries).

5. **`internal/statements/diff.go`** — Pure logic, no DB dependencies.
   - `ComputeDiff(from, to Snapshot, fromEntries, toEntries []SnapshotEntry, opts DiffOptions) *DiffResult`
   - Build map `entryKey{queryid,dbid,userid} → SnapshotEntry` from "from" entries
   - Iterate "to" entries: if key in map → compute delta, else → new_queries
   - Remaining in map → evicted_queries
   - StatsResetWarning: `from.StatsReset != to.StatsReset` (compare safely with nil checks)
   - Derived fields per entry: AvgExecTimePerCall, IOTimePct, CPUTimeDelta, SharedHitRatio (guard all div-by-zero)
   - Sort by opts.SortBy (default "total_exec_time") descending
   - Apply limit/offset for pagination; set TotalEntries before pagination

6. **`internal/statements/insights.go`** — Pure logic.
   - `BuildQueryInsight(instanceID string, queryID int64, entries []SnapshotEntry, snapshots []Snapshot, ...) *QueryInsight`
   - Computes inter-snapshot deltas: point[i].calls - point[i-1].calls, etc.
   - If delta is negative (stats_reset), use point[i] values as delta
   - Returns QueryInsight with Points array sorted by captured_at ASC

7. **`internal/statements/report.go`** — Depends on diff types.
   - `GenerateReport(diff *DiffResult, topN int) *WorkloadReport`
   - Copy diff entries, sort by each section's primary metric, take topN per section
   - Summary aggregated from full entry set

8. **`internal/statements/capture.go`** — SnapshotCapturer.
   - Mirror `remediation/background.go` pattern: Start(ctx)/Stop() with ticker goroutine
   - `CaptureInstance(ctx, instanceID)`: get pool → detect version → check pgss exists → execute version-gated SQL → build Snapshot+Entries → resolve DB/user names → write to store
   - Version-gated SQL: use `version.Gate` with two SQL variants (PG ≤12 vs 13+). See design §2.4.
   - DB name resolution: `SELECT oid, datname FROM pg_database`; User name resolution: `SELECT oid, rolname FROM pg_roles`
   - `statement_timeout = 30000` on the snapshot query
   - Log errors per instance but don't stop the ticker

9. **Tests:** `pgstore_test.go`, `diff_test.go`, `insights_test.go`, `report_test.go`, `capture_test.go`. Diff tests MUST be table-driven covering: normal diff, stats_reset detected, new queries only, evicted queries only, PG 12 null columns, div-by-zero guards, empty snapshots.

### Agent 2 — API Specialist
**Owns:** API handlers, config, migration, main.go wiring

**Wait for Agent 1 to complete types.go and store.go before starting handlers.**

1. **`internal/storage/migrations/015_pgss_snapshots.sql`** — Create both tables + indexes. Include `database_name TEXT` and `user_name TEXT` columns in entries table. See design §1 for full DDL.

2. **`internal/config/config.go`** — Add `StatementSnapshotsConfig` struct + field on Config. Add defaults in `Load()`: Interval=30m, RetentionDays=30, TopN=50. See design §4.

3. **`internal/api/handler_snapshots.go`** — 7 handlers:
   - `handleListSnapshots` — parse limit/offset/from/to query params, call store.ListSnapshots, writeJSON
   - `handleGetSnapshot` — parse snapId from URL, call store.GetSnapshot + GetSnapshotEntries with pagination
   - `handleSnapshotDiff` — parse from/to (snapshot IDs or time-based), load both snapshots + entries, call diff.ComputeDiff, writeJSON
   - `handleLatestDiff` — call store.GetLatestSnapshots(2), load entries, compute diff
   - `handleQueryInsights` — parse queryid from URL + time range, call store.GetEntriesForQuery, call insights.BuildQueryInsight, writeJSON
   - `handleWorkloadReport` — same as diff but wrap in report.GenerateReport, writeJSON
   - `handleManualSnapshotCapture` — call capturer.CaptureInstance, return snapshot metadata
   
   API server needs `snapshotStore SnapshotStore` and `capturer *SnapshotCapturer` fields. Add `SetSnapshotStore()` and `SetSnapshotCapturer()` setter methods (same pattern as existing SetXxx methods).

4. **`internal/api/server.go`** — Add routes. CRITICAL: register `/diff` and `/latest-diff` BEFORE `/{snapId}`:
   ```go
   r.Route("/instances/{id}/snapshots", func(r chi.Router) {
       r.Get("/", h.handleListSnapshots)
       r.Get("/diff", h.handleSnapshotDiff)
       r.Get("/latest-diff", h.handleLatestDiff)
       r.Post("/capture", requirePerm(h.handleManualSnapshotCapture, "instance_management"))
       r.Get("/{snapId}", h.handleGetSnapshot)
   })
   r.Get("/instances/{id}/query-insights/{queryid}", h.handleQueryInsights)
   r.Get("/instances/{id}/workload-report", h.handleWorkloadReport)
   ```

5. **`cmd/pgpulse-server/main.go`** — Wire SnapshotCapturer + snapshotStore under guard `cfg.StatementSnapshots.Enabled && persistentStore != nil`. See design §5. Also remove the debug log line (`"remediation config"` — see Bug Fix 6.1).

6. **Tests:** `handler_snapshots_test.go` — test each handler with mock store. At minimum: list snapshots, get snapshot, diff (normal + empty), latest-diff (fewer than 2 snapshots → 404), manual capture.

### Agent 3 — Fix Specialist + QA
**Owns:** 4 bug fixes + integration testing + build verification

1. **Bug: Remove debug log** — In `cmd/pgpulse-server/main.go`, find and remove the `logger.Info("remediation config", ...)` line (or `logger.Debug`). Search for `"remediation config"` string.

2. **Bug: `wastedibytes` float64→int64** — In `internal/collector/database.go`, find the bloat sub-collector. The `wastedibytes` value comes from a SQL query returning `numeric`/`bigint` that pgx may scan as float64. Change the scan variable from `int64` to `float64`, then convert: `int64(wastedIBytes)`. Search for `wastedibytes` or `wasted_bytes` or `wastedIBytes`.

3. **Bug: `pg.server.multixact_pct` missing** — In `internal/collector/server_info.go`, add a query to compute multixact wrap percentage:
   ```sql
   SELECT COALESCE(max(mxid_age(datminmxid))::float8 / (2147483648)::float8 * 100, 0) FROM pg_database
   ```
   Emit as `point("server.multixact_pct", pct)`. Mirror the existing `server.txid_wraparound_pct` pattern.

4. **Bug: `srsubstate` char scan** — Search codebase for `srsubstate`. It's a `char(1)` column from `pg_subscription_rel`. Change the scan target from `*byte` or `byte` to `*string` or `string`. Adjust any downstream usage.

5. **Build verification:**
   ```bash
   cd web && npm run build && npm run lint && npm run typecheck
   cd .. && go build ./cmd/pgpulse-server
   go test ./cmd/... ./internal/... -count=1
   golangci-lint run ./cmd/... ./internal/...
   ```

6. **Integration check:** After build passes, verify:
   - Migration 015 applies cleanly (check `internal/storage/migrations/` embed directive includes the new file)
   - Config parsing works with new `statement_snapshots:` section
   - NullSnapshotStore compiles and all interface methods satisfied

---

## BUILD VERIFICATION COMMAND (all agents must pass)

```bash
cd web && npm run build && npm run lint && npm run typecheck && cd .. && go build ./cmd/pgpulse-server && go test ./cmd/... ./internal/... -count=1 && golangci-lint run ./cmd/... ./internal/...
```

---

## EXPECTED OUTPUT FILES

### New Files (Agent 1)
- `internal/statements/types.go`
- `internal/statements/store.go`
- `internal/statements/nullstore.go`
- `internal/statements/pgstore.go`
- `internal/statements/pgstore_test.go`
- `internal/statements/diff.go`
- `internal/statements/diff_test.go`
- `internal/statements/insights.go`
- `internal/statements/insights_test.go`
- `internal/statements/report.go`
- `internal/statements/report_test.go`
- `internal/statements/capture.go`
- `internal/statements/capture_test.go`

### New Files (Agent 2)
- `internal/storage/migrations/015_pgss_snapshots.sql`
- `internal/api/handler_snapshots.go`
- `internal/api/handler_snapshots_test.go`

### Modified Files (Agent 2)
- `internal/config/config.go`
- `internal/api/server.go`
- `cmd/pgpulse-server/main.go`

### Modified Files (Agent 3)
- `cmd/pgpulse-server/main.go` (debug log removal — coordinate with Agent 2)
- `internal/collector/database.go` (wastedibytes fix)
- `internal/collector/server_info.go` (multixact_pct addition)
- `internal/collector/database.go` or relevant file (srsubstate fix)

---

## COORDINATION NOTES

- **Agent 2 depends on Agent 1** for `types.go` and `store.go`. Agent 2 can start with migration + config immediately, but must wait for types before writing handlers.
- **Agent 3 is independent** — bug fixes don't depend on new code. Can start immediately.
- **main.go conflict:** Both Agent 2 (wiring) and Agent 3 (debug log removal) modify main.go. Agent 3 should complete the debug log fix first, then Agent 2 adds wiring. If conflict occurs, Agent 2 resolves.
- **Commit order:** Agent 3 commits fixes first → Agent 1 commits statements package → Agent 2 commits API + wiring last.
