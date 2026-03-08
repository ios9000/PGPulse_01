# Session: 2026-03-03 — M5_04 Statements, Lock Tree, Progress Monitoring

## Goal
Add three Tier 3 features deferred from M5_03 to the Server Detail page: pg_stat_statements top queries view with historical drill-down, lock tree visualization with indented depth markers, and conditional progress monitoring section.

## Agent Team Configuration
- Team Lead: Opus 4.6
- Specialists: API Agent, Frontend Agent, QA Agent
- Duration: ~1m 32s bake time
- Commit: b30873a

## Decisions Made

| ID | Decision | Rationale |
|----|----------|-----------|
| D99 | All 3 features in M5_04 | Shared backend pattern (ConnProvider), manageable scope |
| D100 | Statements: live query primary + historical expandable row with ECharts | DBAs want current snapshot; drill-down uses existing stored metrics |
| D101 | Lock tree: indented table with depth markers | Matches PGAM UX, more readable than D3/SVG for tabular lock data |
| D102 | Progress: conditional section, collapses when idle | Avoids empty-state clutter; high visibility when ops are active |
| D103 | 3 new ConnProvider live-query endpoints | Consistent with M5_03 pattern (replication, wait-events, long-txns) |

## New API Endpoints

| Method | Path | Purpose |
|--------|------|---------|
| GET | /api/v1/instances/:id/activity/statements | Top queries, sort whitelist, version-gated |
| GET | /api/v1/instances/:id/activity/locks | Full blocking tree via BFS |
| GET | /api/v1/instances/:id/activity/progress | 6 version-gated progress views |

## PGAM Queries Covered

| PGAM # | Description | Implementation | Agent |
|--------|-------------|----------------|-------|
| #48-49 | pgss settings + stats reset | handler_statements.go config query | API |
| #50 | Query stats by IO time | handler_statements.go sort=io_time | API |
| #51 | Query stats by CPU time | handler_statements.go sort=cpu_time | API |
| #55 | Lock tree (blocking tree) | handler_locks.go BFS + BuildLockTree() | API |
| #42-47 | Progress monitoring (6 types) | handler_progress.go version-gated | API |

Running PGAM coverage: 33 (prior) + 7 (statements/locks/progress API equivalents) = ~40/76

Note: The collector-level porting count remains 33/76. M5_04 added API-layer equivalents for queries that were previously collector-only or not yet surfaced. The actual collector files from M1_03–M1_05 already handle the data collection; M5_04 added the live-query API endpoints and frontend views.

## Files Created/Modified

### Backend (5 files)
- `internal/api/handler_statements.go` — NEW: sort whitelist, version gate, pgss config, extension check
- `internal/api/handler_locks.go` — NEW: exported BuildLockTree(), BFS algorithm, summary computation
- `internal/api/handler_progress.go` — NEW: 6 version-gated queries, merged response
- `internal/api/server.go` — MODIFIED: 3 new routes in both auth groups

### Frontend (12 files)
- `web/src/hooks/useStatements.ts` — NEW: TanStack Query, 10s refetch
- `web/src/hooks/useLockTree.ts` — NEW: TanStack Query, 10s refetch
- `web/src/hooks/useProgress.ts` — NEW: TanStack Query, 5s refetch
- `web/src/components/server/StatementsSection.tsx` — NEW: sortable table + config bar
- `web/src/components/server/StatementsConfigBar.tsx` — NEW: pgss config pill badges
- `web/src/components/server/StatementRow.tsx` — NEW: expandable row with ECharts drill-down
- `web/src/components/server/LockTreeSection.tsx` — NEW: summary + indented table
- `web/src/components/server/LockTreeRow.tsx` — NEW: depth-based padding, root highlighting
- `web/src/components/server/ProgressSection.tsx` — NEW: conditional render
- `web/src/components/server/ProgressCard.tsx` — NEW: operation cards with progress bars
- `web/src/pages/ServerDetailPage.tsx` — MODIFIED: new section order
- `web/src/types/models.ts` — MODIFIED: 9 new types

### Tests (3 files, 20 tests)
- `internal/api/handler_statements_test.go` — 7 tests
- `internal/api/handler_locks_test.go` — 10 tests (BuildLockTree pure function)
- `internal/api/handler_progress_test.go` — 2 tests (error paths, version gating concepts covered in unit structure)

## Build Status

All passing:
- go build — pass
- go vet — pass
- golangci-lint — 0 issues
- go test — 20/20 pass
- tsc --noEmit — pass
- eslint — pass
- vite build — pass

## Stats
- 19 files changed
- 2,181 lines inserted
- Commit: b30873a
- Push: master → origin/master

## Not Done / Next Iteration
- [ ] Alert management UI (rules CRUD, active alerts, test notification) → M5_05
- [ ] Historical drill-down charts may show sparse data for queries not consistently in top-N — acceptable for now
- [ ] AlertsPage.tsx and AlertRulesPage.tsx remain placeholders → M5_05
- [ ] Roadmap.md and CHANGELOG.md need M5_04 entry
