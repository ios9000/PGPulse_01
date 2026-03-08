# PGPulse — M5_04 Team Prompt: Statements, Lock Tree, Progress Monitoring

**Iteration:** M5_04
**Date:** 2026-03-03
**Paste this into Claude Code after updating CLAUDE.md**

---

## Team Prompt

```
Build three new live-query features for the PGPulse web UI: pg_stat_statements top queries, lock tree visualization, and progress monitoring.

Read CLAUDE.md for full context. Read docs/iterations/M5_04_*/design.md for detailed specifications.

All three features follow the same backend pattern established in M5_03: ConnProvider live-query → JSON response → React component with TanStack Query polling.

Create a team of 3 specialists:

API AGENT:
Create three new live-query handlers in internal/api/:

1. handler_statements.go — GET /api/v1/instances/:id/activity/statements
   - Query params: sort (total_time|io_time|cpu_time|calls|rows), limit (1-100, default 25)
   - Sort column selected via Go switch on whitelist — NEVER interpolate user input into SQL
   - $1 parameter for LIMIT only
   - Version-gated: PG ≤ 12 uses total_time; PG ≥ 13 uses total_exec_time
   - Check pg_stat_statements extension exists first; return 404 with code EXTENSION_NOT_FOUND if missing
   - Include pgss config in response: max, track, io_timing, current_count, fill_pct, stats_reset (PG ≥ 14)
   - Response shape: { config: StatementsConfig, statements: StatementEntry[] }

2. handler_locks.go — GET /api/v1/instances/:id/activity/locks
   - Query pg_stat_activity with pg_blocking_pids() for all active sessions
   - Build blocking tree in Go using BFS:
     a. Map pid → blocking_pids (who blocks me)
     b. Reverse map pid → blocked_pids (who I block)
     c. Find root blockers (appear as blocker but have empty blocking_pids)
     d. BFS from roots, assign depth
     e. Only include PIDs involved in blocking chains
   - Response shape: { summary: { root_blockers, total_blocked, max_depth }, locks: LockEntry[] }
   - Each LockEntry: pid, depth, usename, datname, state, wait_event_type, wait_event, duration_seconds, query (200 chars), blocked_by_count, blocking_count, is_root, parent_pid

3. handler_progress.go — GET /api/v1/instances/:id/activity/progress
   - Run 6 version-gated queries against pg_stat_progress_* views:
     - vacuum (always), cluster (PG ≥ 12), create_index (PG ≥ 12)
     - analyze (PG ≥ 13), basebackup (PG ≥ 13), copy (PG ≥ 14)
   - Merge all results into single array with operation_type discriminator
   - Calculate progress_pct where possible (blks_scanned/blks_total etc.)
   - Response shape: { operations: ProgressOperation[] }
   - Return empty array when no operations running

All three handlers:
- Use ConnProvider.ConnFor() to borrow pgx.Conn
- SET statement_timeout = '5000' on borrowed connection
- Require JWT auth (existing middleware handles this)
- Return 502 on connection failure, 504 on timeout
- All SQL parameterized — no string concatenation

Register all 3 routes in router.go under the existing instances group:
  r.Get("/instances/{id}/activity/statements", h.GetStatements)
  r.Get("/instances/{id}/activity/locks", h.GetLockTree)
  r.Get("/instances/{id}/activity/progress", h.GetProgress)

FRONTEND AGENT:
Build three new React sections and integrate them into ServerDetailPage.

1. Types — add to web/src/types/models.ts:
   - StatementsConfig, StatementEntry, StatementsResponse, StatementSortField
   - LockTreeSummary, LockEntry, LockTreeResponse
   - ProgressOperation, ProgressResponse
   (See design.md section 3.1 for complete type definitions)

2. Hooks — create in web/src/hooks/:
   - useStatements.ts: TanStack Query, 10s refetch, params: instanceId, sort, limit
   - useLockTree.ts: TanStack Query, 10s refetch, param: instanceId
   - useProgress.ts: TanStack Query, 5s refetch, param: instanceId

3. StatementsSection — web/src/components/server/:
   - StatementsConfigBar.tsx: horizontal row of pill badges for pgss config
     - Fill % (yellow ≥80%, red ≥95%), Max, Track, IO Timing (yellow if off), Reset Age (yellow ≥1d)
   - StatementsSection.tsx: main section with config bar + sortable table
     - Columns: #, Query, DB, User, Total Time, Mean, Calls, Rows, IO Time, CPU Time, Hit%
     - Clickable column headers trigger re-sort (React state → refetch with new sort param)
     - Active sort column highlighted with ▼ indicator
   - StatementRow.tsx: table row, clickable to expand
     - Collapsed: single row with truncated query (~80 chars, monospace)
     - Expanded: panel below row with:
       - Full query text in code block (max-height 200px, scroll)
       - 2-3 ECharts using existing TimeSeriesChart/EChartWrapper:
         - Execution time over time (line chart)
         - Calls per interval (bar chart)
         - IO vs CPU time (stacked area)
       - Historical data from useMetricsHistory hook filtered by queryid label
       - Handle sparse/empty historical data gracefully
     - Click again or close button to collapse
   - Handle extension-not-found error: show info message about enabling pg_stat_statements

4. LockTreeSection — web/src/components/server/:
   - LockTreeSection.tsx: summary line + indented table
     - Summary: "⚠ N root blockers affecting M processes" (orange/red) or "✓ No blocking locks" (green)
     - Table columns: PID, User, Database, State, Wait Event, Duration, Blocking, Query
   - LockTreeRow.tsx: row with depth-based left padding (depth × 24px)
     - Root blockers (depth=0): left border border-l-4 border-red-500, subtle bg
     - Duration color: normal <1min, yellow 1-5min, red >5min
     - Blocking count badge, red if > 0
     - Query truncated, tooltip on hover

5. ProgressSection — web/src/components/server/:
   - ProgressSection.tsx: CONDITIONAL — returns null when operations.length === 0
     - No empty state, no placeholder, section simply does not render
     - When active: renders "Active Operations" card with ProgressCard children
   - ProgressCard.tsx: compact card per operation
     - Operation type pill: VACUUM=blue, ANALYZE=green, CREATE INDEX=purple, CLUSTER=orange, BASEBACKUP=cyan, COPY=yellow
     - Target: database / relname
     - Phase label + PID + duration
     - Progress bar: colored fill in slate-700 track, striped animation when active
     - Indeterminate/pulse when progress_pct is null

6. Integration — modify ServerDetailPage.tsx:
   Section order (top to bottom):
   - HeaderCard (existing)
   - ProgressSection (NEW — conditional)
   - KeyMetricsRow (existing)
   - Connections section (existing)
   - Cache Hit section (existing)
   - ReplicationSection (existing)
   - WaitEventsSection (existing)
   - StatementsSection (NEW)
   - LockTreeSection (NEW)
   - LongTransactionsTable (existing)
   - InstanceAlerts (existing)

All components use Tailwind CSS dark-mode-first styling consistent with existing components. Use existing shared components (Card, DataTable, StatusBadge, MetricCard, Spinner) where appropriate.

QA AGENT:
Write backend tests for all three new handlers:

1. internal/api/handler_statements_test.go:
   - TestGetStatements_SortValidation: verify sort whitelist (total_time, io_time, cpu_time, calls, rows accepted; anything else defaults to total_time)
   - TestGetStatements_LimitClamping: verify limit clamped to 1-100, defaults to 25
   - TestGetStatements_ExtensionNotFound: verify 404 response when pg_stat_statements not installed
   - TestGetStatements_VersionGate: verify PG 12 uses total_time, PG 13+ uses total_exec_time (can mock version detection)

2. internal/api/handler_locks_test.go:
   - TestBuildLockTree_Empty: no blocking → empty response
   - TestBuildLockTree_SingleRoot: one blocker, two blocked → depth 0 + depth 1
   - TestBuildLockTree_MultiRoot: two independent blocking chains → two depth-0 roots
   - TestBuildLockTree_DeepChain: A blocks B blocks C → depths 0, 1, 2
   - TestLockTreeSummary: verify root_blockers, total_blocked, max_depth counts

3. internal/api/handler_progress_test.go:
   - TestGetProgress_VersionGating: PG 14 runs all 6 queries, PG 12 runs only 3 (vacuum, cluster, index)
   - TestGetProgress_EmptyResults: no active operations → empty array
   - TestGetProgress_ProgressPctCalculation: verify progress_pct math

Test the tree-building logic as a pure function (extract BuildLockTree function for testability).
Run golangci-lint on all new code.
Verify no SQL string concatenation — search for fmt.Sprintf patterns near SQL.
Run full test suite: go test -race ./...
Run frontend checks: cd web && npx tsc --noEmit && npx eslint src/ && npx vite build

COORDINATION:
- API Agent and Frontend Agent can start in parallel — Frontend uses type definitions immediately, hooks point to endpoints that API Agent creates
- QA Agent starts writing test structure immediately, fills assertions as API code lands
- Tree-building logic in handler_locks.go should be extracted as a testable pure function: BuildLockTree(entries []RawLockEntry) (LockTreeResponse)
- Team Lead merges in order: API Agent first (defines response contracts), Frontend Agent second (renders them), QA Agent last (validates everything)
- Final verification: go build → go vet → golangci-lint → go test → tsc → eslint → vite build — ALL must pass before commit
```

---

## Pre-Flight Checklist

Before pasting the team prompt into Claude Code:

1. [ ] Copy M5_04_requirements.md, M5_04_design.md, M5_04_team-prompt.md to `docs/iterations/M5_04_{date}_statements-locks-progress/`
2. [ ] Update CLAUDE.md "Current Iteration" section:
   ```
   ## Current Iteration
   M5_04 — Statements, Lock Tree, Progress Monitoring
   See: docs/iterations/M5_04_{date}_statements-locks-progress/
   ```
3. [ ] Verify build is clean before starting:
   ```bash
   go build ./cmd/... ./internal/...
   go vet ./...
   golangci-lint run
   cd web && npx tsc --noEmit && npx eslint src/ && npx vite build
   ```
4. [ ] Paste the team prompt block above into Claude Code
