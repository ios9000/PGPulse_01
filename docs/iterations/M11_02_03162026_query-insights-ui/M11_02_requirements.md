# M11_02 Requirements — Query Insights UI + Workload Report + HTML Export

**Iteration:** M11_02
**Date:** 2026-03-16
**Scope:** Frontend pages for PGSS snapshot data + server-rendered HTML export

---

## Background

M11_01 delivered the full backend: PGSS snapshot capture, diff engine, query insights aggregation, workload report data, and 7 API endpoints. M11_02 adds the frontend to make this data visible and actionable, plus an HTML export endpoint for offline workload reports.

---

## Functional Requirements

### FR-1: TypeScript Types

- **FR-1.1:** Add types matching all M11_01 API response structures: Snapshot, SnapshotEntry, DiffResult, DiffEntry, QueryInsight, QueryInsightPoint, WorkloadReport, ReportSummary.

### FR-2: React Query Hooks

- **FR-2.1:** `useSnapshots(instanceId, opts)` — GET /snapshots (paginated, time-range)
- **FR-2.2:** `useSnapshotDiff(instanceId, from, to)` — GET /snapshots/diff
- **FR-2.3:** `useLatestDiff(instanceId)` — GET /snapshots/latest-diff, 30s refresh
- **FR-2.4:** `useQueryInsights(instanceId, queryId, from, to)` — GET /query-insights/{queryid}
- **FR-2.5:** `useWorkloadReport(instanceId, from, to)` — GET /workload-report
- **FR-2.6:** `useManualCapture(instanceId)` — POST /snapshots/capture (mutation)

### FR-3: Query Insights Page

- **FR-3.1:** Route: `/servers/:serverId/query-insights`
- **FR-3.2:** Top queries table from latest-diff data, sortable by: exec_time_delta, calls_delta, rows_delta, avg_exec_time, io_pct, shared_hit_ratio
- **FR-3.3:** Click a row → expands or navigates to per-query detail with ECharts time-series showing calls/interval, exec_time/interval, avg_exec_time, shared_hit_ratio over time
- **FR-3.4:** Snapshot range selector: dropdown/picker to choose from/to snapshots, or "Latest" default
- **FR-3.5:** Stats reset warning banner when DiffResult.stats_reset_warning is true
- **FR-3.6:** Query text display: monospace, truncated with expand, copy button
- **FR-3.7:** New/evicted query sections (collapsible) below the main table
- **FR-3.8:** "Capture Now" button (permission-gated to instance_management) calls manual capture mutation
- **FR-3.9:** Empty state when no snapshots exist yet ("Snapshots are being collected. First data available in ~30 minutes.")

### FR-4: Workload Report Page

- **FR-4.1:** Route: `/servers/:serverId/workload-report`
- **FR-4.2:** Snapshot range selector (same component as Query Insights)
- **FR-4.3:** Summary card: total calls delta, total exec_time delta, unique queries, new/evicted counts, time range, duration
- **FR-4.4:** Sections: Top by Exec Time, Top by Calls, Top by Rows, Top by I/O Reads, Top by Avg Time — each collapsible with sortable table
- **FR-4.5:** New Queries and Evicted Queries sections
- **FR-4.6:** "Export HTML" button → downloads server-rendered HTML report
- **FR-4.7:** Print-friendly layout via CSS @media print

### FR-5: HTML Export

- **FR-5.1:** New Go endpoint: `GET /instances/{id}/workload-report/html`
- **FR-5.2:** Query params: `?from={snapId}&to={snapId}` or `?from_time=&to_time=`
- **FR-5.3:** Server-rendered using Go `html/template`
- **FR-5.4:** Standalone HTML: inline CSS, no external dependencies
- **FR-5.5:** Content-Disposition: `attachment; filename="workload-report-{instance}-{date}.html"`
- **FR-5.6:** Optional `?inline=true` for in-browser rendering (no download header)
- **FR-5.7:** Same sections as the React page

### FR-6: Navigation

- **FR-6.1:** Add "Query Insights" link to sidebar under instance navigation
- **FR-6.2:** Add "Workload Report" link to sidebar
- **FR-6.3:** Both links visible when navigating an instance (alongside Explain, Settings Diff)
- **FR-6.4:** Add routes to App.tsx

---

## Non-Functional Requirements

- **NFR-1:** Query Insights table must handle 500+ queries without jank (virtualization not required but pagination yes)
- **NFR-2:** ECharts time-series must reuse the existing TimeSeriesChart/EChartWrapper pattern
- **NFR-3:** All new components follow existing Tailwind patterns (dark mode support)
- **NFR-4:** HTML export template must render correctly in all major browsers (Chrome, Firefox, Safari, Edge)

---

## Out of Scope

- Query fingerprint normalization improvements
- Syntax highlighting for query text (use monospace + basic formatting)
- Compare two arbitrary workload reports
- Inline EXPLAIN from query insights (future enhancement)
