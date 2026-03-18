# Session: 2026-03-16 — M11_02 Query Insights UI + Workload Report + HTML Export

## Goal
Build the complete frontend for PGSS snapshot data: Query Insights page with per-query drill-down charts, Workload Report page with collapsible sections, HTML export endpoint for offline reports, and sidebar navigation.

## Agent Team
- **Agent 1 — Frontend Specialist:** 10 new files + 3 modified (React/TypeScript)
- **Agent 2 — Full-Stack Specialist:** 2 new files + 1 modified (Go HTML export)

## Duration
1 minute 33 seconds total.

## Commits

| Agent | Description | New Files | Modified Files |
|-------|-------------|-----------|----------------|
| Agent 1 | feat: Query Insights + Workload Report frontend | 10 new | 3 modified |
| Agent 2 | feat: HTML export endpoint + template | 2 new | 1 modified |

## New Frontend Files

| File | Purpose |
|------|---------|
| web/src/hooks/useSnapshots.ts | 6 React Query hooks (snapshots, diff, insights, report, capture) |
| web/src/pages/QueryInsights.tsx | Query insights page with sortable diff table + drill-down |
| web/src/pages/WorkloadReport.tsx | Workload report page with collapsible sections |
| web/src/components/snapshots/SnapshotSelector.tsx | From/To snapshot dropdown pair |
| web/src/components/snapshots/DiffTable.tsx | Sortable, paginated diff entries table |
| web/src/components/snapshots/QueryDetailPanel.tsx | 4 mini ECharts for per-query time-series |
| web/src/components/snapshots/StatsResetBanner.tsx | Amber warning for stats_reset |
| web/src/components/snapshots/QueryText.tsx | Truncated monospace with copy/expand |
| web/src/components/snapshots/ReportSummaryCard.tsx | Summary metrics grid |
| web/src/components/snapshots/ReportSection.tsx | Collapsible report section with count badge |

## New Backend Files

| File | Purpose |
|------|---------|
| internal/api/handler_report_html.go | HTML export handler with go:embed template |
| internal/api/templates/workload_report.html | Standalone HTML template with inline CSS |

## Modified Files

| File | Change |
|------|--------|
| web/src/types/models.ts | 9 new TypeScript interfaces added |
| web/src/App.tsx | 2 new routes (query-insights, workload-report) |
| web/src/components/layout/Sidebar.tsx | 2 instance-scoped nav links (BarChart3, FileText icons) |
| internal/api/server.go | /workload-report/html route registered |

## New Routes

| Route | Page |
|-------|------|
| /servers/:serverId/query-insights | QueryInsights |
| /servers/:serverId/workload-report | WorkloadReport |

## New API Endpoint

| Method | Path | Purpose |
|--------|------|---------|
| GET | /instances/{id}/workload-report/html | Downloadable HTML workload report |

## Build Status

- Frontend: build + typecheck + lint — pass
- Backend: build + vet + tests + lint — pass

## What's Next

M11 complete. Next milestone: M12 — Desktop App (Wails packaging).
