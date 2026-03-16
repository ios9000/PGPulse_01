# M11_02 Design — Query Insights UI + Workload Report + HTML Export

**Iteration:** M11_02
**Date:** 2026-03-16
**Pattern references:** `web/src/pages/SettingsDiff.tsx` (snapshot selection), `web/src/hooks/useSettingsTimeline.ts` (snapshot hooks), `web/src/components/charts/TimeSeriesChart.tsx` (ECharts)

---

## 1. TypeScript Types (`web/src/types/models.ts`)

Add to existing models.ts file:

```typescript
// PGSS Snapshot types
export interface PGSSSnapshot {
  id: number;
  instance_id: string;
  captured_at: string;
  pg_version: number;
  stats_reset?: string;
  total_statements: number;
  total_calls: number;
  total_exec_time_ms: number;
}

export interface PGSSSnapshotEntry {
  snapshot_id: number;
  queryid: number;
  userid: number;
  dbid: number;
  query: string;
  database_name: string;
  user_name: string;
  calls: number;
  total_exec_time_ms: number;
  total_plan_time_ms?: number;
  rows: number;
  shared_blks_hit: number;
  shared_blks_read: number;
  shared_blks_dirtied: number;
  shared_blks_written: number;
  local_blks_hit: number;
  local_blks_read: number;
  temp_blks_read: number;
  temp_blks_written: number;
  blk_read_time_ms: number;
  blk_write_time_ms: number;
  wal_records?: number;
  wal_fpi?: number;
  wal_bytes?: number;
  mean_exec_time_ms?: number;
  min_exec_time_ms?: number;
  max_exec_time_ms?: number;
  stddev_exec_time_ms?: number;
}

export interface DiffEntry {
  queryid: number;
  userid: number;
  dbid: number;
  query: string;
  database_name: string;
  user_name: string;
  calls_delta: number;
  exec_time_delta_ms: number;
  plan_time_delta_ms?: number;
  rows_delta: number;
  shared_blks_read_delta: number;
  shared_blks_hit_delta: number;
  temp_blks_read_delta: number;
  temp_blks_written_delta: number;
  blk_read_time_delta_ms: number;
  blk_write_time_delta_ms: number;
  wal_bytes_delta?: number;
  avg_exec_time_per_call_ms: number;
  io_time_pct: number;
  cpu_time_delta_ms: number;
  shared_hit_ratio_pct: number;
}

export interface DiffResult {
  from_snapshot: PGSSSnapshot;
  to_snapshot: PGSSSnapshot;
  stats_reset_warning: boolean;
  duration: string;
  total_calls_delta: number;
  total_exec_time_delta_ms: number;
  entries: DiffEntry[];
  new_queries: DiffEntry[];
  evicted_queries: DiffEntry[];
  total_entries: number;
}

export interface QueryInsightPoint {
  captured_at: string;
  calls_delta: number;
  exec_time_delta_ms: number;
  rows_delta: number;
  avg_exec_time_ms: number;
  shared_hit_ratio_pct: number;
}

export interface QueryInsight {
  queryid: number;
  query: string;
  database_name: string;
  user_name: string;
  first_seen: string;
  points: QueryInsightPoint[];
}

export interface ReportSummary {
  total_queries: number;
  total_calls_delta: number;
  total_exec_time_delta_ms: number;
  total_rows_delta: number;
  unique_queries: number;
  new_queries: number;
  evicted_queries: number;
}

export interface WorkloadReport {
  instance_id: string;
  from_time: string;
  to_time: string;
  duration: string;
  stats_reset_warning: boolean;
  summary: ReportSummary;
  top_by_exec_time: DiffEntry[];
  top_by_calls: DiffEntry[];
  top_by_rows: DiffEntry[];
  top_by_io_reads: DiffEntry[];
  top_by_avg_time: DiffEntry[];
  new_queries: DiffEntry[];
  evicted_queries: DiffEntry[];
}

export interface SnapshotListResponse {
  snapshots: PGSSSnapshot[];
  total: number;
}

export interface SnapshotDetailResponse {
  snapshot: PGSSSnapshot;
  entries: PGSSSnapshotEntry[];
  total_entries: number;
}
```

---

## 2. React Query Hooks (`web/src/hooks/useSnapshots.ts`)

Single file with all snapshot-related hooks. Pattern: mirror `useSettingsTimeline.ts`.

```typescript
// useSnapshots.ts

// useSnapshots(instanceId, { limit, offset, from, to })
//   → GET /instances/{id}/snapshots?limit=...&offset=...&from=...&to=...
//   → returns { data: SnapshotListResponse, isLoading, error }

// useLatestDiff(instanceId, enabled?)
//   → GET /instances/{id}/snapshots/latest-diff
//   → refetchInterval: 30_000 (30s)
//   → returns { data: DiffResult, isLoading, error }

// useSnapshotDiff(instanceId, fromId, toId)
//   → GET /instances/{id}/snapshots/diff?from={fromId}&to={toId}
//   → enabled only when both fromId and toId are set
//   → returns { data: DiffResult, isLoading, error }

// useQueryInsights(instanceId, queryId, from?, to?)
//   → GET /instances/{id}/query-insights/{queryId}?from=...&to=...
//   → enabled only when queryId is set
//   → returns { data: QueryInsight, isLoading, error }

// useWorkloadReport(instanceId, fromId?, toId?)
//   → GET /instances/{id}/workload-report?from={fromId}&to={toId}
//   → enabled only when both IDs set
//   → returns { data: WorkloadReport, isLoading, error }

// useManualCapture(instanceId)
//   → POST /instances/{id}/snapshots/capture
//   → useMutation, invalidates snapshot queries on success
//   → returns { mutate, isLoading }
```

---

## 3. Query Insights Page

### 3.1 Page (`web/src/pages/QueryInsights.tsx`)

**Layout:**
```
┌──────────────────────────────────────────────────────────┐
│  Query Insights                    [Snapshot Selector ▾]  │
│                                    [Capture Now]          │
├──────────────────────────────────────────────────────────┤
│  ⚠ Stats Reset Warning (conditional)                     │
├──────────────────────────────────────────────────────────┤
│  Summary: 142 queries │ 1.2M calls │ 45.3s exec time     │
├──────────────────────────────────────────────────────────┤
│  ┌─ Top Queries ─────────────────────────────────────┐   │
│  │ # │ Query          │ Calls Δ │ Time Δ │ Avg │ IO% │   │
│  │ 1 │ SELECT * FR... │ 45,231  │ 12.3s  │ 0.3 │ 15% │   │
│  │ 2 │ INSERT INTO... │ 32,100  │  8.7s  │ 0.3 │  5% │   │
│  │   ▼ (expanded detail with time-series charts)      │   │
│  └────────────────────────────────────────────────────┘   │
│  ┌─ New Queries (3) ──── [collapse] ─────────────────┐   │
│  └────────────────────────────────────────────────────┘   │
│  ┌─ Evicted Queries (1) ── [collapse] ───────────────┐   │
│  └────────────────────────────────────────────────────┘   │
└──────────────────────────────────────────────────────────┘
```

**State management:**
- `selectedFromSnap` / `selectedToSnap` — snapshot IDs (default: null = use latest-diff)
- `expandedQueryId` — which query row is expanded (null = none)
- `sortBy` / `sortDir` — table sort state
- `page` / `pageSize` — pagination

**Data flow:**
- On mount: `useSnapshots(instanceId)` to populate snapshot selector
- Default: `useLatestDiff(instanceId)` for the table
- When user picks from/to: `useSnapshotDiff(instanceId, from, to)`
- When row expanded: `useQueryInsights(instanceId, queryId)` for time-series

### 3.2 Components

**`web/src/components/snapshots/SnapshotSelector.tsx`** (~80 lines)
- Dropdown with snapshot list (captured_at formatted)
- "From" and "To" selectors
- "Latest" option (default)
- Used by both Query Insights and Workload Report pages

**`web/src/components/snapshots/DiffTable.tsx`** (~200 lines)
- Sortable table of DiffEntry[]
- Columns: #, Query (truncated), DB, Calls Δ, Exec Time Δ, Avg Time, Rows Δ, I/O %, Hit Ratio
- Click row → expand with QueryDetailPanel
- Pagination controls
- formatDuration, formatNumber from existing `lib/formatters.ts`

**`web/src/components/snapshots/QueryDetailPanel.tsx`** (~150 lines)
- Expanded view for a single query
- Full query text (monospace, copy button)
- 4 ECharts mini-charts: calls/interval, exec_time/interval, avg_exec_time, shared_hit_ratio
- Uses `useQueryInsights` hook
- Uses existing `EChartWrapper` component

**`web/src/components/snapshots/StatsResetBanner.tsx`** (~20 lines)
- Yellow warning banner: "Stats were reset between these snapshots. Delta values may be inaccurate."
- Conditionally rendered when `diff.stats_reset_warning === true`

**`web/src/components/snapshots/QueryText.tsx`** (~40 lines)
- Monospace display with truncation
- Expand/collapse toggle
- Copy to clipboard button

### 3.3 Empty State

When `useSnapshots` returns 0 snapshots or `useLatestDiff` returns 404:
- Show EmptyState component with message: "Statement snapshots are being collected. First diff will be available after two capture cycles (~60 minutes)."
- Show "Capture Now" button if user has instance_management permission.

---

## 4. Workload Report Page

### 4.1 Page (`web/src/pages/WorkloadReport.tsx`)

**Layout:**
```
┌──────────────────────────────────────────────────────────┐
│  Workload Report                   [Snapshot Selector ▾]  │
│                                    [Export HTML ⬇]        │
├──────────────────────────────────────────────────────────┤
│  ⚠ Stats Reset Warning (conditional)                     │
├──────────────────────────────────────────────────────────┤
│  ┌─ Summary ─────────────────────────────────────────┐   │
│  │ Period: 30m │ Calls: 1.2M │ Time: 45.3s │        │   │
│  │ Queries: 142 │ New: 3 │ Evicted: 1               │   │
│  └────────────────────────────────────────────────────┘   │
│  ┌─ Top by Execution Time (10) ──────────────────────┐   │
│  │ (DiffTable, collapsed by default, sorted by time) │   │
│  └────────────────────────────────────────────────────┘   │
│  ┌─ Top by Call Count (10) ──────────────────────────┐   │
│  │ ...                                                │   │
│  └────────────────────────────────────────────────────┘   │
│  ... (Top by Rows, Top by I/O Reads, Top by Avg Time)    │
│  ┌─ New Queries ─────────────────────────────────────┐   │
│  └────────────────────────────────────────────────────┘   │
│  ┌─ Evicted Queries ────────────────────────────────┐    │
│  └────────────────────────────────────────────────────┘   │
└──────────────────────────────────────────────────────────┘
```

**Components:**

**`web/src/components/snapshots/ReportSummaryCard.tsx`** (~60 lines)
- Grid of summary metrics from WorkloadReport.summary
- Uses MetricCard pattern

**`web/src/components/snapshots/ReportSection.tsx`** (~80 lines)
- Collapsible section with title + count badge
- Contains a DiffTable (reused from Query Insights)
- Default: first section expanded, rest collapsed

**Export HTML button:**
- Constructs URL: `/api/v1/instances/{id}/workload-report/html?from={fromId}&to={toId}`
- Opens in new tab or triggers download
- Permission: viewer+ (read-only export)

**Print CSS:**
- Add `@media print` rules to the page
- Hide sidebar, topbar, buttons
- Expand all collapsible sections
- Page breaks between sections

---

## 5. HTML Export Endpoint (Go)

### 5.1 Handler (`internal/api/handler_report_html.go`)

```go
func (s *APIServer) handleWorkloadReportHTML(w http.ResponseWriter, r *http.Request) {
    // Parse instance ID, from/to params (same as handleWorkloadReport)
    // Load snapshots + entries, compute diff, generate report
    // Render Go html/template
    // Set Content-Type: text/html
    // If ?inline=true → no Content-Disposition
    // Else → Content-Disposition: attachment; filename="workload-report-{instance}-{date}.html"
}
```

### 5.2 Template (`internal/api/templates/workload_report.html`)

Embedded via `go:embed`. Standalone HTML with inline CSS (Tailwind-like utility classes, manually written — NOT full Tailwind). Sections:

- Header: "PGPulse Workload Report", instance name, time range, generation timestamp
- Summary table
- 5 "Top by X" tables (exec_time, calls, rows, io_reads, avg_time)
- New queries table
- Evicted queries table
- Footer: "Generated by PGPulse"

Design: clean, professional, print-friendly. Light background, dark text. Tables with alternating row colors.

### 5.3 Route

In `server.go`, add within the instances route group:
```go
r.Get("/instances/{id}/workload-report/html", h.handleWorkloadReportHTML)
```

---

## 6. Sidebar Navigation

In `web/src/components/layout/Sidebar.tsx`:

Add two new links in the instance-scoped navigation section (near "EXPLAIN Plans", "Settings Diff"):

```
📊 Query Insights    → /servers/{id}/query-insights
📋 Workload Report   → /servers/{id}/workload-report
```

These should be visible when the user is on any `/servers/:serverId/*` page. Mirror the existing pattern for "EXPLAIN Plans" link.

---

## 7. Routes (`web/src/App.tsx`)

Add two new routes:
```tsx
<Route path="/servers/:serverId/query-insights" element={<QueryInsights />} />
<Route path="/servers/:serverId/workload-report" element={<WorkloadReport />} />
```

---

## 8. File Inventory

### New Files

| File | Est. Lines | Purpose |
|------|------------|---------|
| `web/src/hooks/useSnapshots.ts` | ~120 | All snapshot/diff/insights/report hooks |
| `web/src/pages/QueryInsights.tsx` | ~200 | Query insights page |
| `web/src/pages/WorkloadReport.tsx` | ~180 | Workload report page |
| `web/src/components/snapshots/SnapshotSelector.tsx` | ~80 | Shared snapshot range picker |
| `web/src/components/snapshots/DiffTable.tsx` | ~200 | Sortable diff entries table |
| `web/src/components/snapshots/QueryDetailPanel.tsx` | ~150 | Expanded query detail with charts |
| `web/src/components/snapshots/StatsResetBanner.tsx` | ~20 | Warning banner |
| `web/src/components/snapshots/QueryText.tsx` | ~40 | Query display with copy |
| `web/src/components/snapshots/ReportSummaryCard.tsx` | ~60 | Report summary metrics |
| `web/src/components/snapshots/ReportSection.tsx` | ~80 | Collapsible report section |
| `internal/api/handler_report_html.go` | ~120 | HTML export handler |
| `internal/api/templates/workload_report.html` | ~250 | HTML template |

**Total new:** ~12 files, ~1,500 estimated lines

### Modified Files

| File | Change |
|------|--------|
| `web/src/types/models.ts` | Add PGSS/diff/insight/report types |
| `web/src/App.tsx` | Add 2 routes |
| `web/src/components/layout/Sidebar.tsx` | Add 2 nav links |
| `internal/api/server.go` | Add HTML export route |

**Total modified:** ~4 files
