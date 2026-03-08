# PGPulse — M5_04 Requirements: Statements, Lock Tree, Progress Monitoring

**Iteration:** M5_04
**Date:** 2026-03-03
**Depends on:** M5_03 (Live Data Integration), M1_03 (Progress), M1_04 (Statements), M1_05 (Locks)
**Status:** Planning

---

## 1. Overview

M5_04 adds three DBA-critical views to the PGPulse web UI — all three were deferred as "Tier 3" from M5_03. They share a common backend pattern (ConnProvider live queries) but each has distinct frontend requirements.

### Features

| # | Feature | Backend | Frontend | Priority |
|---|---------|---------|----------|----------|
| F1 | pg_stat_statements Top Queries | New live-query endpoint + existing stored metrics | Sortable table + expandable row with historical ECharts | P0 |
| F2 | Lock Tree Visualization | New live-query endpoint (recursive blocking chain) | Indented table with depth markers | P0 |
| F3 | Progress Monitoring | New live-query endpoint (6 operation types) | Conditional section with progress bars | P0 |

### Key Decisions (locked)

| ID | Decision | Source |
|----|----------|--------|
| D99 | All 3 features in M5_04 | M5_04 planning |
| D100 | Statements: live query primary + historical drill-down via expandable row with inline ECharts | M5_04 planning |
| D101 | Lock tree: indented table with depth markers, root blockers highlighted | M5_04 planning |
| D102 | Progress: conditional section on Server Detail, collapses when no active operations | M5_04 planning |
| D103 | 3 new ConnProvider live-query endpoints | M5_04 planning |

---

## 2. Feature Requirements

### F1: pg_stat_statements Top Queries

#### 2.1.1 Backend — Live Query Endpoint

**Endpoint:** `GET /api/v1/instances/:id/activity/statements`

**Query Parameters:**
- `sort` — `io_time` | `cpu_time` | `calls` | `rows` | `total_time` (default: `total_time`)
- `limit` — integer, 1–100 (default: 25)

**Response:** Array of statement objects with:
- `queryid` (int64 — pg_stat_statements.queryid)
- `query_text` (string, truncated to 500 chars)
- `dbname` (string)
- `username` (string)
- `calls` (int64)
- `total_exec_time_ms` (float64) — PG ≥ 13: `total_exec_time`; PG ≤ 12: `total_time`
- `mean_exec_time_ms` (float64)
- `rows` (int64)
- `blk_read_time_ms` (float64)
- `blk_write_time_ms` (float64)
- `io_time_ms` (float64) — `blk_read_time + blk_write_time`
- `cpu_time_ms` (float64) — `total_exec_time - blk_read_time - blk_write_time`
- `shared_blks_hit` (int64)
- `shared_blks_read` (int64)
- `hit_ratio` (float64) — `blks_hit / (blks_hit + blks_read)`, 0 if no reads
- `pct_of_total_time` (float64) — percentage of grand total exec time

**Version gate:** PG ≤ 12 uses `total_time`; PG ≥ 13 uses `total_exec_time`. Column selection in SQL must be version-gated.

**pgss config sub-response** (or separate lightweight endpoint):
- `pg_stat_statements.max` setting
- `pg_stat_statements.track` setting
- `track_io_timing` setting
- Current statement count and fill percentage
- `stats_reset` timestamp and age (PG ≥ 14 only, from `pg_stat_statements_info`)

**Auth:** Requires valid JWT. Any role (dba, app_admin, super_admin, roles_admin) can read.

**Error cases:**
- pg_stat_statements extension not installed → 404 with message `"pg_stat_statements extension not available"`
- Instance unreachable → 502 with timeout message
- statement_timeout (5s) exceeded → 504

#### 2.1.2 Frontend — Statements View

**Location:** New section on Server Detail page, between Wait Events and Long Transactions. Also accessible as a dedicated page/tab for more room.

**pgss Config Bar (top of section):**
- Compact horizontal bar showing: fill % (with warning color at ≥95%), max setting, track setting, IO timing on/off, stats reset age
- Badges/pills for each config item

**Statements Table:**
- Columns: #, Query (truncated, monospace), Database, User, Total Time, Mean Time, Calls, Rows, IO Time, CPU Time, Hit Ratio
- Default sort: Total Time descending
- Clickable column headers to re-sort (triggers new API call with `sort` param)
- Query text truncated to ~80 chars in table, with `...` indicator
- Row click expands to show full query text + historical charts

**Expandable Row (drill-down):**
- Full query text in a code block (syntax-highlighted if feasible, monospace otherwise)
- 2–3 inline ECharts time-series charts:
  - Execution time over time (from `/metrics/history` filtered by queryid label)
  - Calls per interval over time
  - IO time vs CPU time over time (stacked or dual-axis)
- Uses the existing `useMetricsHistory` hook with queryid label filter
- Time range controlled by the global TimeRangeSelector
- Collapse on second click or via close button

**Empty state:** If pg_stat_statements is not installed, show an informational message explaining the extension is required and how to enable it.

### F2: Lock Tree Visualization

#### 2.2.1 Backend — Lock Tree Endpoint

**Endpoint:** `GET /api/v1/instances/:id/activity/locks`

**Response:** Array of lock entries representing the blocking tree:
- `pid` (int)
- `depth` (int) — 0 = root blocker, 1+ = blocked processes
- `usename` (string)
- `datname` (string)
- `state` (string) — active, idle in transaction, etc.
- `wait_event_type` (string, nullable)
- `wait_event` (string, nullable)
- `lock_mode` (string, nullable) — e.g., "AccessExclusiveLock"
- `duration_seconds` (float64) — seconds since xact_start
- `query` (string) — truncated to 200 chars
- `blocked_by_count` (int) — number of PIDs blocking this process
- `blocking_count` (int) — number of PIDs this process is blocking

**Implementation approach:** Reuse the Go BFS graph logic from `internal/collector/locks_tree.go` (M1_05). The collector already builds the blocking graph using `pg_blocking_pids()`. For the API endpoint, run the same query but return the full per-PID detail instead of summary metrics. The handler borrows a conn via ConnProvider, runs the query, builds the tree in Go, and returns JSON sorted by depth then PID.

**SQL (core query for tree building):**
```sql
SELECT
    sa.pid,
    sa.usename,
    sa.datname,
    sa.state,
    sa.wait_event_type,
    sa.wait_event,
    EXTRACT(EPOCH FROM (now() - sa.xact_start))::float8 AS duration_seconds,
    LEFT(sa.query, 200) AS query,
    pg_blocking_pids(sa.pid) AS blocking_pids
FROM pg_stat_activity sa
WHERE sa.pid != pg_backend_pid()
  AND sa.state IS NOT NULL
```

Go code then builds the tree: find PIDs with non-empty `blocking_pids` (these are blocked), trace up to root blockers, assign depth via BFS from roots down.

**Auth:** Requires valid JWT. Any role can read.

**Empty state:** When no blocking locks exist, return empty array `[]`.

#### 2.2.2 Frontend — Lock Tree Section

**Location:** New section on Server Detail page, between Wait Events and Long Transactions (or after Long Transactions — placement TBD in design doc).

**Indented Table:**
- Columns: PID, User, Database, State, Wait Event, Lock Mode, Duration, Blocking, Query
- Depth indicated by left-padding (e.g., 24px per depth level) and tree connector characters (`├──`, `└──`)
- Root blockers (depth=0) highlighted with red/orange left border or background tint
- Duration formatted as human-readable: "2m 34s", "15s", "1h 12m"
- Query text truncated with tooltip or expand on click
- "Blocking" column shows count of PIDs this process blocks (for root blockers, this is the impact indicator)

**Root Blocker Summary:**
- Above the table, a compact summary: "3 root blockers affecting 12 processes" (or similar)
- Red/orange alert styling when any blocking exists

**Empty state:** "No blocking locks detected" with a green checkmark icon.

**Auto-refresh:** 10s interval via TanStack Query, matching other Server Detail sections.

### F3: Progress Monitoring

#### 2.3.1 Backend — Progress Endpoint

**Endpoint:** `GET /api/v1/instances/:id/activity/progress`

**Response:** Array of active operations:
- `operation_type` — `vacuum` | `analyze` | `create_index` | `cluster` | `basebackup` | `copy`
- `pid` (int)
- `datname` (string)
- `relname` (string, nullable) — table name for vacuum/analyze/index/cluster
- `phase` (string) — current phase name
- `progress_pct` (float64, nullable) — calculated percentage where possible
- `started_at` (string, nullable) — operation start time if available
- `duration_seconds` (float64) — time since operation started
- Additional fields per operation type (see design doc for complete list)

**SQL:** 6 queries against progress views, all version-gated:
- `pg_stat_progress_vacuum` (PG ≥ 9.6)
- `pg_stat_progress_cluster` (PG ≥ 12)
- `pg_stat_progress_create_index` (PG ≥ 12)
- `pg_stat_progress_analyze` (PG ≥ 13)
- `pg_stat_progress_basebackup` (PG ≥ 13)
- `pg_stat_progress_copy` (PG ≥ 14)

Handler runs all applicable queries (based on PG version), merges results into a single array.

**Auth:** Requires valid JWT. Any role can read.

**Empty state:** Empty array `[]` when no operations are running.

#### 2.3.2 Frontend — Progress Section

**Location:** Conditional section on Server Detail page. Appears between Header/Key Metrics and Connections when active operations exist. Collapses to nothing when idle (no empty-state placeholder — the section simply doesn't render).

**Per-Operation Card:**
- Operation type badge (color-coded: VACUUM=blue, ANALYZE=green, CREATE INDEX=purple, etc.)
- Target: database.schema.table (or just database for basebackup)
- Phase label
- Progress bar (filled to progress_pct, striped/animated to indicate active)
- Duration: "Running for 2m 34s"
- PID shown in smaller text

**Layout:** Stack vertically, one card per operation. Rare to have more than 2–3 simultaneous operations.

**Auto-refresh:** 5s interval (faster than other sections, since progress changes rapidly).

**Conditional rendering logic:**
```
if (progressData.length === 0) return null; // section not rendered
```

---

## 3. New API Endpoints Summary

| Method | Path | Data Source | Auto-refresh |
|--------|------|-------------|-------------|
| GET | `/api/v1/instances/:id/activity/statements` | Live query via ConnProvider | 10s |
| GET | `/api/v1/instances/:id/activity/locks` | Live query via ConnProvider | 10s |
| GET | `/api/v1/instances/:id/activity/progress` | Live query via ConnProvider | 5s |

All three:
- Require valid JWT (any role)
- Use ConnProvider to borrow a pgx.Conn with `statement_timeout = 5s`
- Return JSON arrays
- Return empty arrays when no data (not 404)
- Return 502 on connection failure, 504 on timeout

---

## 4. Files to Create/Modify

### Backend (new files)
- `internal/api/handler_statements.go` — statements live-query handler
- `internal/api/handler_locks.go` — lock tree live-query handler
- `internal/api/handler_progress.go` — progress live-query handler

### Backend (modified files)
- `internal/api/router.go` — register 3 new routes
- `internal/version/gate.go` — add SQL variants if not already present

### Frontend (new files)
- `web/src/components/server/StatementsSection.tsx` — main statements section with table
- `web/src/components/server/StatementRow.tsx` — expandable row with historical charts
- `web/src/components/server/StatementsConfigBar.tsx` — pgss config display
- `web/src/components/server/LockTreeSection.tsx` — lock tree indented table
- `web/src/components/server/LockTreeRow.tsx` — individual lock tree row with depth indent
- `web/src/components/server/ProgressSection.tsx` — conditional progress section
- `web/src/components/server/ProgressCard.tsx` — per-operation progress card
- `web/src/hooks/useStatements.ts` — TanStack Query hook for statements endpoint
- `web/src/hooks/useLockTree.ts` — TanStack Query hook for locks endpoint
- `web/src/hooks/useProgress.ts` — TanStack Query hook for progress endpoint

### Frontend (modified files)
- `web/src/pages/ServerDetailPage.tsx` — add 3 new sections
- `web/src/types/models.ts` — add Statement, LockEntry, ProgressOperation types

---

## 5. Acceptance Criteria

### Statements
- [ ] Top queries table renders with correct data from live endpoint
- [ ] Sort by any of: total_time, io_time, cpu_time, calls, rows — triggers server-side sort
- [ ] Click row expands to show full query text + historical charts
- [ ] Historical charts show data from existing stored metrics for that queryid
- [ ] pgss config bar displays fill %, max, track, IO timing, reset age
- [ ] Graceful handling when pg_stat_statements is not installed
- [ ] Version-gated SQL works for PG 14–17 (total_time vs total_exec_time)

### Lock Tree
- [ ] Lock tree displays all blocking chains with correct depth hierarchy
- [ ] Root blockers visually highlighted
- [ ] Summary line above table shows blocker/blocked counts
- [ ] Empty state when no blocking locks
- [ ] Tree connector characters (├──, └──) render correctly
- [ ] 10s auto-refresh

### Progress
- [ ] Progress section appears only when active operations exist
- [ ] Section does not render when no operations are running
- [ ] Progress bars show correct percentage and phase
- [ ] All 6 operation types render correctly
- [ ] 5s auto-refresh for rapid progress updates
- [ ] Duration shows elapsed time in human-readable format

### General
- [ ] All 3 new endpoints return correct JSON
- [ ] All endpoints use ConnProvider with statement_timeout=5s
- [ ] All endpoints require JWT auth
- [ ] go build, go vet, golangci-lint pass
- [ ] tsc --noEmit, eslint, vite build pass
- [ ] No SQL string concatenation in any new handler
