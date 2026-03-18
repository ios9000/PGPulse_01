# Session: 2026-03-16 — M11_01 PGSS Snapshots, Diff Engine, Query Insights API + Bug Fixes

## Goal
Build the complete backend foundation for Competitive Enrichment: pg_stat_statements snapshot capture, persistent storage, diff engine, query insights aggregation, workload report data generation, 7 new API endpoints, and fix 4 known bugs.

## Agent Team
- **Agent 1 — Backend Specialist:** `internal/statements/` package (12 files)
- **Agent 2 — API Specialist:** Migration, handlers, config, main.go wiring (4 new + 3 modified)
- **Agent 3 — Fix Specialist + QA:** 4 bug fixes (3 modified files)

## Duration
5 minutes 36 seconds total.

## Commits

| Hash | Author | Description | Files |
|------|--------|-------------|-------|
| 2272cd0 | Agent 3 | fix: 4 bug fixes (debug log, wastedibytes, multixact_pct, srsubstate) | 3 modified, +17 -4 |
| 354679b | Agent 1 | feat: statements package (types, store, pgstore, diff, insights, report, capture + tests) | 12 new, +1,960 |
| 8b9d5ac | Agent 2 | feat: API handlers + migration 015 + config + wiring | 4 new + 3 modified, +1,084 -15 |

## New Package: `internal/statements/`

| File | Purpose |
|------|---------|
| types.go | Snapshot, SnapshotEntry, DiffResult, DiffEntry, QueryInsight, WorkloadReport, etc. |
| store.go | SnapshotStore interface (7 methods) |
| pgstore.go | PGSnapshotStore — PostgreSQL implementation with CopyFrom bulk insert |
| nullstore.go | NullSnapshotStore — no-op for live mode |
| diff.go | ComputeDiff — pure Go diff engine with derived fields |
| insights.go | BuildQueryInsight — inter-snapshot delta computation |
| report.go | GenerateReport — structured workload report from diff |
| capture.go | SnapshotCapturer — periodic capture with Start/Stop, version-gated SQL |
| *_test.go | 4 test files covering all above |

## New API Endpoints (7, total now ~54)

| Method | Path | Handler |
|--------|------|---------|
| GET | /instances/{id}/snapshots | handleListSnapshots |
| GET | /instances/{id}/snapshots/{snapId} | handleGetSnapshot |
| GET | /instances/{id}/snapshots/diff | handleSnapshotDiff |
| GET | /instances/{id}/snapshots/latest-diff | handleLatestDiff |
| GET | /instances/{id}/query-insights/{queryid} | handleQueryInsights |
| GET | /instances/{id}/workload-report | handleWorkloadReport |
| POST | /instances/{id}/snapshots/capture | handleManualSnapshotCapture |

## Bug Fixes

1. **Debug log removed** — `"remediation config"` line in main.go
2. **wastedibytes** — NullInt64 → NullFloat64 scan in database.go bloat sub-collector
3. **pg.server.multixact_pct** — Added to ServerInfoCollector (mirrors txid_wraparound_pct)
4. **srsubstate char(1)** — Added `::text` cast in 2 SQL queries for logical replication

## Configuration Added

```yaml
statement_snapshots:
  enabled: true
  interval: 30m
  retention_days: 30
  capture_on_startup: false
  top_n: 50
```

## Migration

015_pgss_snapshots.sql — creates `pgss_snapshots` + `pgss_snapshot_entries` tables with indexes.

## Build Status

- go build: clean
- go test: 16 packages pass (20+ new statement tests)
- golangci-lint: 0 issues
- Frontend build/lint/typecheck: clean (no frontend changes this iteration)

## Decisions Made by Agents

- Agent 1 used NullFloat64 for wastedibytes (safer than raw float64 for nullable numerics)
- Agent 3 used `::text` cast for srsubstate rather than changing Go scan type — cleaner fix, keeps SQL explicit

## What's Next

M11_02: Frontend — Query Insights page, Workload Report page, HTML export, sidebar navigation.
