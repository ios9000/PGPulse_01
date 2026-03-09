# M8_06 Requirements — UI Catch-Up + Forecast Extension

**Iteration:** M8_06
**Date:** 2026-03-09
**Scope:** Frontend-only — all backend APIs already exist and are tested

---

## Goal

Complete three deferred UI features and extend the forecast overlay to all
remaining metric charts. This closes out the M8 milestone.

---

## Feature 1: Session Kill UI

**Backend (already complete, M8_03):**
- `POST /api/v1/instances/{id}/sessions/{pid}/cancel` → `pg_cancel_backend`
- `POST /api/v1/instances/{id}/sessions/{pid}/terminate` → `pg_terminate_backend`
- Both return 200 on success, 403 for viewer role, 404/500 on failure

**Frontend requirements:**

1. Activity table rows in ServerDetail gain two action buttons:
   - **Cancel** (yellow/warning) — cancels the current query
   - **Terminate** (red/danger) — kills the entire backend process
2. Buttons hidden or disabled for viewer role (check user role from auth context)
3. Clicking either button opens a **confirmation modal**:
   - Cancel: "Cancel the running query for PID {pid}?"
   - Terminate: "Terminate the entire session for PID {pid}? This will disconnect the client."
4. On confirm → call API → show success/error toast → auto-refresh the activity table
5. On 403 → show "Insufficient permissions" toast (should not happen if buttons are hidden,
   but handle defensively)
6. Buttons should not appear for PGPulse's own connections (`application_name LIKE 'pgpulse_%'`)

---

## Feature 2: Settings Diff UI

**Backend (already complete, M8_01):**
- `GET /api/v1/instances/{id}/settings/diff` → returns array of changed settings,
  each with: `name`, `setting` (current), `boot_val` (default), `unit`, `category`,
  `context`, `pending_restart` (bool)

**Frontend requirements:**

1. New `SettingsDiff.tsx` component
2. Grouped by `category` in collapsible accordion sections
3. Each row: setting name, current value, default value, unit, context
4. `pending_restart = true` → badge "Restart Required" in amber
5. "Export CSV" button — client-side download from in-memory data
6. Mounted as a new tab/section in ServerDetail (label: "Settings Diff")
7. Empty state: "All settings match defaults" when diff array is empty

---

## Feature 3: Query Plan Viewer UI

**Backend (already complete, M8_02):**
- `GET /api/v1/instances/{id}/statements/{queryid}/plan` → returns EXPLAIN (FORMAT JSON) output

**Frontend requirements:**

1. New `QueryPlanViewer.tsx` component
2. Recursive tree rendering of the EXPLAIN JSON plan nodes
3. Each node shows: Node Type, Startup Cost → Total Cost, Actual Time,
   Rows (planned vs actual), Width
4. **Cost highlighting:** nodes with `actual_time > 100ms` or
   `actual_rows / plan_rows > 10` (row estimate error) get a red/amber background
5. "Show Raw JSON" toggle to display the raw EXPLAIN output in a `<pre>` block
6. Accessible from the top queries table in ServerDetail — clicking a query row
   opens the plan viewer (in a slide-out panel or expanded row)
7. Loading state while plan is being fetched; error state if EXPLAIN fails
   (e.g., prepared statement no longer exists)

---

## Feature 4: Forecast Overlay — Remaining Charts

**Pattern (already working on `connections_active` from M8_05):**
- `useForecast(instanceId, metricKey)` hook
- `buildForecastSeries(points)` + `getNowMarkLine(nowMs)` from `ForecastBand.ts`
- `TimeSeriesChart` props: `extraSeries`, `xAxisMax`, `nowMarkLine`

**Extend to these charts:**

| Chart | Metric key |
|-------|------------|
| Cache hit ratio | `cache_hit_ratio` |
| Transactions/sec | `transactions_per_sec` |
| Replication lag | `replication_lag_bytes` |
| Active sessions | `sessions_active` |

Requirements:
1. Same visual pattern as `connections_active` — confidence band + dashed centre line + "Now" divider
2. Each chart calls `useForecast` independently (different metric keys)
3. If forecast returns null (not bootstrapped / no baseline), chart renders normally without overlay
4. No new API endpoints needed — same `/forecast` endpoint with different `metric` param

---

## Out of Scope

- Backend changes (all APIs exist)
- New API endpoints
- `Administration.tsx` lint error
- Session kill for logical replication workers
- Settings diff for non-default configuration files
