# M11_02 Team Prompt — Query Insights UI + Workload Report + HTML Export

**Iteration:** M11_02
**Date:** 2026-03-16
**Agents:** 2 (Frontend Specialist, Full-Stack Specialist)

---

## CONTEXT

PGPulse is a PostgreSQL monitoring platform built with Go (backend) and React/TypeScript/Vite/Tailwind/Apache ECharts (frontend). M11_01 added 7 API endpoints for PGSS snapshots, diffs, query insights, and workload reports. M11_02 adds the frontend pages and an HTML export endpoint.

**Key pattern references in repo:**
- `web/src/pages/SettingsDiff.tsx` (280 lines) — snapshot selection + diff display pattern
- `web/src/hooks/useSettingsTimeline.ts` (79 lines) — React Query hook pattern for snapshot data
- `web/src/components/charts/TimeSeriesChart.tsx` (156 lines) — ECharts time-series pattern
- `web/src/components/ui/DataTable.tsx` (111 lines) — sortable table pattern
- `web/src/components/layout/Sidebar.tsx` (197 lines) — navigation sidebar
- `web/src/types/models.ts` (617 lines) — existing type definitions
- `web/src/lib/formatters.ts` (72 lines) — number/byte/duration formatting
- `web/src/lib/api.ts` (107 lines) — API client with JWT refresh
- `web/src/App.tsx` (55 lines) — route definitions

**Existing API endpoints (from M11_01, already working):**
- `GET /api/v1/instances/{id}/snapshots` → `{ snapshots: [], total: number }`
- `GET /api/v1/instances/{id}/snapshots/{snapId}` → `{ snapshot: {}, entries: [], total_entries: number }`
- `GET /api/v1/instances/{id}/snapshots/diff?from=X&to=Y` → DiffResult
- `GET /api/v1/instances/{id}/snapshots/latest-diff` → DiffResult
- `GET /api/v1/instances/{id}/query-insights/{queryid}?from=&to=` → QueryInsight
- `GET /api/v1/instances/{id}/workload-report?from=X&to=Y` → WorkloadReport
- `POST /api/v1/instances/{id}/snapshots/capture` → Snapshot

---

## DO NOT RE-DISCUSS

- TypeScript types match the M11_01 Go struct JSON tags exactly — do NOT rename fields.
- ECharts is already set up (echarts-setup.ts, EChartWrapper.tsx) — reuse, don't install new chart libs.
- Tailwind CSS with dark mode support — follow existing component patterns.
- React Query (useQuery/useMutation) from `@tanstack/react-query` — follow existing hook patterns.
- API client is `web/src/lib/api.ts` with `api.get()`, `api.post()` — use this, don't use raw fetch.
- Sidebar links for instance-scoped pages follow the pattern in Sidebar.tsx where items like "EXPLAIN Plans" and "Settings" appear under the instance navigation.
- HTML export template is self-contained HTML with inline CSS — no CDN links, no Tailwind build, just handwritten CSS.
- The `go:embed` for the HTML template must be in the same package or accessible from the handler file.

---

## TEAM STRUCTURE

### Agent 1 — Frontend Specialist
**Owns:** All TypeScript/React files

**Work order:**

1. **`web/src/types/models.ts`** — ADD the following types to the EXISTING file (do NOT replace the file). Add at the end:
   - `PGSSSnapshot` — id, instance_id, captured_at, pg_version, stats_reset?, total_statements, total_calls, total_exec_time_ms
   - `PGSSSnapshotEntry` — snapshot_id, queryid, userid, dbid, query, database_name, user_name, calls, total_exec_time_ms, total_plan_time_ms?, rows, shared_blks_hit/read/dirtied/written, local_blks_hit/read, temp_blks_read/written, blk_read_time_ms, blk_write_time_ms, wal_records?, wal_fpi?, wal_bytes?, mean/min/max/stddev_exec_time_ms?
   - `DiffEntry` — queryid, userid, dbid, query, database_name, user_name, calls_delta, exec_time_delta_ms, plan_time_delta_ms?, rows_delta, shared_blks_read_delta, shared_blks_hit_delta, temp_blks_read_delta, temp_blks_written_delta, blk_read_time_delta_ms, blk_write_time_delta_ms, wal_bytes_delta?, avg_exec_time_per_call_ms, io_time_pct, cpu_time_delta_ms, shared_hit_ratio_pct
   - `DiffResult` — from_snapshot, to_snapshot, stats_reset_warning, duration, total_calls_delta, total_exec_time_delta_ms, entries: DiffEntry[], new_queries: DiffEntry[], evicted_queries: DiffEntry[], total_entries
   - `QueryInsightPoint` — captured_at, calls_delta, exec_time_delta_ms, rows_delta, avg_exec_time_ms, shared_hit_ratio_pct
   - `QueryInsight` — queryid, query, database_name, user_name, first_seen, points: QueryInsightPoint[]
   - `ReportSummary` — total_queries, total_calls_delta, total_exec_time_delta_ms, total_rows_delta, unique_queries, new_queries, evicted_queries
   - `WorkloadReport` — instance_id, from_time, to_time, duration, stats_reset_warning, summary: ReportSummary, top_by_exec_time/calls/rows/io_reads/avg_time: DiffEntry[], new_queries, evicted_queries: DiffEntry[]
   - `SnapshotListResponse` — snapshots: PGSSSnapshot[], total: number
   - `SnapshotDetailResponse` — snapshot: PGSSSnapshot, entries: PGSSSnapshotEntry[], total_entries: number

2. **`web/src/hooks/useSnapshots.ts`** — NEW file (~120 lines). Create these hooks:
   - `useSnapshots(instanceId: string, opts?: { limit?: number; offset?: number; from?: string; to?: string })` — `useQuery` wrapping `api.get(`/instances/${instanceId}/snapshots`, { params })`. Query key: `['snapshots', instanceId, opts]`.
   - `useLatestDiff(instanceId: string, enabled = true)` — `useQuery` wrapping `api.get(`/instances/${instanceId}/snapshots/latest-diff`)`. Query key: `['latest-diff', instanceId]`. Set `refetchInterval: 30_000`. Set `enabled` param.
   - `useSnapshotDiff(instanceId: string, fromId?: number, toId?: number)` — `useQuery` wrapping `api.get(`/instances/${instanceId}/snapshots/diff`, { params: { from: fromId, to: toId } })`. Enabled only when `fromId && toId`. Query key: `['snapshot-diff', instanceId, fromId, toId]`.
   - `useQueryInsights(instanceId: string, queryId?: number, from?: string, to?: string)` — `useQuery` wrapping `api.get(`/instances/${instanceId}/query-insights/${queryId}`, { params: { from, to } })`. Enabled only when `queryId` is set. Query key: `['query-insights', instanceId, queryId, from, to]`.
   - `useWorkloadReport(instanceId: string, fromId?: number, toId?: number)` — `useQuery` wrapping `api.get(`/instances/${instanceId}/workload-report`, { params: { from: fromId, to: toId } })`. Enabled only when `fromId && toId`. Query key: `['workload-report', instanceId, fromId, toId]`.
   - `useManualCapture(instanceId: string)` — `useMutation` wrapping `api.post(`/instances/${instanceId}/snapshots/capture`)`. On success: invalidate `['snapshots', instanceId]` and `['latest-diff', instanceId]`.

3. **`web/src/components/snapshots/SnapshotSelector.tsx`** — NEW file (~80 lines).
   - Props: `instanceId: string`, `snapshots: PGSSSnapshot[]`, `fromId: number | null`, `toId: number | null`, `onChange: (from: number | null, to: number | null) => void`, `loading?: boolean`
   - Two select dropdowns: "From" and "To"
   - Options: snapshot list formatted as "Mar 16, 2026 8:27 PM (142 statements)"
   - First option in each: "Latest" (sends null, triggers useLatestDiff)
   - Tail-wind styled to match existing filter bars (see AlertFilters.tsx pattern)

4. **`web/src/components/snapshots/StatsResetBanner.tsx`** — NEW file (~20 lines).
   - Props: `visible: boolean`
   - Yellow/amber banner: "⚠ Statistics were reset between these snapshots. Delta values may be inaccurate."
   - Tailwind: `bg-amber-50 dark:bg-amber-900/20 border-amber-200 dark:border-amber-800 text-amber-800 dark:text-amber-200`

5. **`web/src/components/snapshots/QueryText.tsx`** — NEW file (~40 lines).
   - Props: `query: string`, `maxLength?: number` (default 120)
   - Truncated display with "Show more" / "Show less" toggle
   - Copy to clipboard button (navigator.clipboard.writeText)
   - Monospace font: `font-mono text-xs`

6. **`web/src/components/snapshots/DiffTable.tsx`** — NEW file (~200 lines).
   - Props: `entries: DiffEntry[]`, `onRowClick?: (queryid: number) => void`, `expandedQueryId?: number | null`, `instanceId: string`, `compact?: boolean` (for report sections)
   - Columns: #, Query, Database, Calls Δ, Exec Time Δ, Avg Time, Rows Δ, I/O %, Hit Ratio %
   - Sortable headers (click to sort, toggle asc/desc). Default sort: exec_time_delta_ms desc.
   - Click row → calls onRowClick. If expandedQueryId matches → render QueryDetailPanel inline below the row.
   - Format numbers with existing formatters (formatDuration for times, toLocaleString for counts).
   - Use Tailwind table classes matching existing tables in the app.
   - Pagination: show page controls when entries.length > 20.

7. **`web/src/components/snapshots/QueryDetailPanel.tsx`** — NEW file (~150 lines).
   - Props: `instanceId: string`, `queryId: number`, `query: string`, `databaseName: string`, `userName: string`
   - Calls `useQueryInsights(instanceId, queryId)` internally
   - Shows: full query text (QueryText component), first_seen date, database, user
   - 4 mini ECharts: calls/interval, exec_time/interval, avg_exec_time, shared_hit_ratio
   - Chart pattern: use `EChartWrapper` with line chart options. X-axis: captured_at timestamps. Y-axis: metric value.
   - Loading state: Spinner while insights load
   - Each chart ~100px tall, 2x2 grid layout

8. **`web/src/components/snapshots/ReportSummaryCard.tsx`** — NEW file (~60 lines).
   - Props: `summary: ReportSummary`, `fromTime: string`, `toTime: string`, `duration: string`
   - Grid of metric cards: Total Calls, Total Exec Time, Unique Queries, New Queries, Evicted Queries, Time Range
   - Reuse MetricCard component or similar styling

9. **`web/src/components/snapshots/ReportSection.tsx`** — NEW file (~80 lines).
   - Props: `title: string`, `entries: DiffEntry[]`, `instanceId: string`, `defaultExpanded?: boolean`
   - Collapsible section with chevron toggle
   - Count badge next to title
   - Contains DiffTable in compact mode (no expand on click, no QueryDetailPanel)

10. **`web/src/pages/QueryInsights.tsx`** — NEW file (~200 lines).
    - Uses `useParams()` to get `serverId`
    - State: `fromSnap`, `toSnap` (snapshot IDs), `expandedQueryId`, `sortBy`, `sortDir`
    - Loads: `useSnapshots(serverId)` for selector, `useLatestDiff(serverId)` or `useSnapshotDiff(serverId, from, to)` for table
    - Layout: PageHeader → SnapshotSelector + CaptureNow button → StatsResetBanner → Summary line → DiffTable → New Queries section → Evicted Queries section
    - Capture Now button: gated by `useAuth()` permissions check for `instance_management`, calls `useManualCapture`
    - Empty state when no snapshots: "Statement snapshots are being collected. First comparison will be available after two capture cycles (~60 minutes)."

11. **`web/src/pages/WorkloadReport.tsx`** — NEW file (~180 lines).
    - Uses `useParams()` to get `serverId`
    - State: `fromSnap`, `toSnap` (snapshot IDs)
    - Loads: `useSnapshots(serverId)` for selector, `useWorkloadReport(serverId, from, to)` for data.
    - When from/to not selected: use the two most recent snapshots from useSnapshots (auto-select).
    - Layout: PageHeader → SnapshotSelector + Export HTML button → StatsResetBanner → ReportSummaryCard → 5 ReportSections → New Queries → Evicted Queries
    - Export HTML button: `<a>` tag with href `/api/v1/instances/${serverId}/workload-report/html?from=${fromSnap}&to=${toSnap}` and `target="_blank"`
    - Add print CSS in the page or index.css: `@media print { .sidebar, .topbar, .no-print { display: none; } .report-section { break-inside: avoid; } }`

12. **`web/src/App.tsx`** — MODIFY: add 2 routes inside the existing ProtectedRoute wrapper:
    ```tsx
    <Route path="/servers/:serverId/query-insights" element={<QueryInsights />} />
    <Route path="/servers/:serverId/workload-report" element={<WorkloadReport />} />
    ```
    Import the new page components.

13. **`web/src/components/layout/Sidebar.tsx`** — MODIFY: add 2 links in the instance-scoped section.
    Look for where "EXPLAIN Plans" or "Settings" or similar instance-scoped links are rendered. Add:
    - "Query Insights" → `/servers/${serverId}/query-insights`
    - "Workload Report" → `/servers/${serverId}/workload-report`
    Use an appropriate icon (e.g., BarChart3 and FileText from lucide-react if available, or simple text).

### Agent 2 — Full-Stack Specialist
**Owns:** Go HTML export endpoint + integration verification

1. **`internal/api/templates/workload_report.html`** — NEW file (~250 lines).
   Go `html/template` with inline CSS. Structure:
   ```html
   <!DOCTYPE html>
   <html>
   <head>
       <meta charset="utf-8">
       <title>PGPulse Workload Report — {{.InstanceID}}</title>
       <style>
           /* Clean professional CSS: */
           body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif; margin: 2rem; color: #1a1a1a; }
           h1, h2 { color: #1e3a5f; }
           table { width: 100%; border-collapse: collapse; margin: 1rem 0; }
           th { background: #f0f4f8; text-align: left; padding: 8px 12px; border-bottom: 2px solid #d0d7de; }
           td { padding: 8px 12px; border-bottom: 1px solid #e8ecf0; }
           tr:nth-child(even) { background: #f8fafc; }
           .summary { display: grid; grid-template-columns: repeat(auto-fit, minmax(150px, 1fr)); gap: 1rem; margin: 1rem 0; }
           .summary-card { background: #f0f4f8; padding: 1rem; border-radius: 8px; }
           .summary-card .label { font-size: 0.85rem; color: #666; }
           .summary-card .value { font-size: 1.5rem; font-weight: 600; }
           .query-text { font-family: monospace; font-size: 0.8rem; max-width: 600px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
           .warning { background: #fef3cd; border: 1px solid #ffc107; padding: 0.75rem; border-radius: 4px; margin: 1rem 0; }
           .footer { margin-top: 2rem; padding-top: 1rem; border-top: 1px solid #e8ecf0; color: #999; font-size: 0.8rem; }
           @media print { body { margin: 0.5cm; } }
       </style>
   </head>
   <body>
       <h1>PGPulse Workload Report</h1>
       <p>Instance: <strong>{{.InstanceID}}</strong></p>
       <p>Period: {{.FromTime}} — {{.ToTime}} ({{.Duration}})</p>
       {{if .StatsResetWarning}}<div class="warning">⚠ Statistics were reset during this period.</div>{{end}}
       
       <div class="summary">
           <div class="summary-card"><div class="label">Total Calls</div><div class="value">{{.Summary.TotalCallsDelta | formatNumber}}</div></div>
           <div class="summary-card"><div class="label">Total Exec Time</div><div class="value">{{.Summary.TotalExecTimeDelta | formatDuration}}</div></div>
           <div class="summary-card"><div class="label">Unique Queries</div><div class="value">{{.Summary.UniqueQueries}}</div></div>
           <div class="summary-card"><div class="label">New Queries</div><div class="value">{{.Summary.NewQueries}}</div></div>
           <div class="summary-card"><div class="label">Evicted Queries</div><div class="value">{{.Summary.EvictedQueries}}</div></div>
       </div>
       
       {{template "section" dict "Title" "Top by Execution Time" "Entries" .TopByExecTime}}
       {{template "section" dict "Title" "Top by Calls" "Entries" .TopByCalls}}
       {{template "section" dict "Title" "Top by Rows" "Entries" .TopByRows}}
       {{template "section" dict "Title" "Top by I/O Reads" "Entries" .TopByIOReads}}
       {{template "section" dict "Title" "Top by Average Time" "Entries" .TopByAvgTime}}
       
       {{if .NewQueries}}
       {{template "section" dict "Title" "New Queries" "Entries" .NewQueries}}
       {{end}}
       {{if .EvictedQueries}}
       {{template "section" dict "Title" "Evicted Queries" "Entries" .EvictedQueries}}
       {{end}}
       
       <div class="footer">Generated by PGPulse at {{.GeneratedAt}}</div>
   </body>
   </html>
   
   {{define "section"}}
   <h2>{{.Title}} ({{len .Entries}})</h2>
   <table>
       <tr><th>#</th><th>Query</th><th>Database</th><th>Calls Δ</th><th>Exec Time Δ</th><th>Avg Time</th><th>Rows Δ</th><th>I/O %</th><th>Hit Ratio</th></tr>
       {{range $i, $e := .Entries}}
       <tr>
           <td>{{add $i 1}}</td>
           <td class="query-text" title="{{$e.Query}}">{{truncate $e.Query 100}}</td>
           <td>{{$e.DatabaseName}}</td>
           <td>{{$e.CallsDelta | formatNumber}}</td>
           <td>{{$e.ExecTimeDelta | formatDuration}}</td>
           <td>{{$e.AvgExecTimePerCall | formatDuration}}</td>
           <td>{{$e.RowsDelta | formatNumber}}</td>
           <td>{{printf "%.1f" $e.IOTimePct}}%</td>
           <td>{{printf "%.1f" $e.SharedHitRatio}}%</td>
       </tr>
       {{end}}
   </table>
   {{end}}
   ```
   
   **IMPORTANT:** The template uses custom functions: `formatNumber`, `formatDuration`, `truncate`, `add`, `dict`. These must be registered in the Go handler via `template.FuncMap`. The `dict` function is a common helper that creates a map from key/value pairs — if it's too complex, simplify the template to pass section data directly without `dict`.
   
   Alternative simpler approach: don't use `{{template}}` with `dict`. Instead, render each section inline with direct field access. This avoids needing the `dict` function.

2. **`internal/api/handler_report_html.go`** — NEW file (~120 lines).
   - Embed the template: `//go:embed templates/workload_report.html`
   - Parse template with FuncMap: formatNumber, formatDuration, truncate, add
   - Handler logic:
     1. Parse instance ID from URL
     2. Parse `from` and `to` query params (snapshot IDs or time range — same logic as existing handleWorkloadReport)
     3. If no from/to: get latest 2 snapshots
     4. Load snapshots + entries from store
     5. Compute diff, generate report (reuse existing statements.ComputeDiff + statements.GenerateReport)
     6. Add GeneratedAt timestamp
     7. Execute template
     8. If `?inline=true`: Content-Type text/html, no disposition
     9. Else: Content-Type text/html + Content-Disposition attachment with filename `workload-report-{instanceID}-{date}.html`
   - Template FuncMap:
     ```go
     funcMap := template.FuncMap{
         "formatNumber":   func(n int64) string { return humanize.Comma(n) },  // or manual formatting
         "formatDuration": func(ms float64) string { /* format as "1.23s" or "456ms" */ },
         "truncate":       func(s string, n int) string { if len(s) > n { return s[:n] + "..." }; return s },
         "add":            func(a, b int) int { return a + b },
     }
     ```
     NOTE: If `humanize` package is not available, write a simple formatNumber manually. Do NOT add new dependencies.

3. **`internal/api/server.go`** — MODIFY: add route for HTML export.
   Add inside the instances route group, near the existing workload-report route:
   ```go
   r.Get("/instances/{id}/workload-report/html", h.handleWorkloadReportHTML)
   ```

4. **Build verification:**
   ```bash
   cd web && npm run build && npm run lint && npm run typecheck
   cd .. && go build ./cmd/pgpulse-server
   go test ./cmd/... ./internal/... -count=1
   golangci-lint run ./cmd/... ./internal/...
   ```

---

## BUILD VERIFICATION COMMAND (all agents must pass)

```bash
cd web && npm run build && npm run lint && npm run typecheck && cd .. && go build ./cmd/pgpulse-server && go test ./cmd/... ./internal/... -count=1 && golangci-lint run ./cmd/... ./internal/...
```

---

## EXPECTED OUTPUT FILES

### New Files (Agent 1)
- `web/src/hooks/useSnapshots.ts`
- `web/src/pages/QueryInsights.tsx`
- `web/src/pages/WorkloadReport.tsx`
- `web/src/components/snapshots/SnapshotSelector.tsx`
- `web/src/components/snapshots/DiffTable.tsx`
- `web/src/components/snapshots/QueryDetailPanel.tsx`
- `web/src/components/snapshots/StatsResetBanner.tsx`
- `web/src/components/snapshots/QueryText.tsx`
- `web/src/components/snapshots/ReportSummaryCard.tsx`
- `web/src/components/snapshots/ReportSection.tsx`

### New Files (Agent 2)
- `internal/api/handler_report_html.go`
- `internal/api/templates/workload_report.html`

### Modified Files (Agent 1)
- `web/src/types/models.ts` (ADD types, don't replace)
- `web/src/App.tsx` (ADD 2 routes)
- `web/src/components/layout/Sidebar.tsx` (ADD 2 nav links)

### Modified Files (Agent 2)
- `internal/api/server.go` (ADD 1 route)

---

## COORDINATION NOTES

- **Agent 1 and Agent 2 are independent.** Agent 1 works entirely in `web/src/`, Agent 2 works entirely in `internal/api/`. No file conflicts.
- **Both agents modify `internal/api/server.go`** — but Agent 1 doesn't touch Go files. Only Agent 2 modifies server.go.
- **Commit order:** Agent 1 commits frontend → Agent 2 commits HTML export → final build verification.
- **Template embed path:** The `go:embed templates/workload_report.html` directive requires the template file to be in `internal/api/templates/` relative to the Go file that embeds it. Verify this path.
