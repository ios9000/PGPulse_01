# M8_06 Team Prompt — UI Catch-Up + Forecast Extension

Read CLAUDE.md for full project context.
Read `docs/iterations/M8_06/M8_06_design.md` for detailed component specs.

This is **frontend-only work** — all backend APIs already exist. Use a **2-specialist team**
(not the full 4). No Go code changes, no new migrations.

---

Create a team of 2 specialists:

## FRONTEND AGENT

### Task 1: Session Kill UI

Create `web/src/components/ConfirmModal.tsx`:
- Generic reusable confirmation modal with Tailwind styling
- Props: open, title, message, confirmLabel, confirmVariant ('warning' | 'danger'),
  onConfirm, onCancel, loading
- Backdrop overlay, centered card, confirm button colour matches variant
  (amber for warning, red for danger)
- Spinner on confirm button while loading=true

Create `web/src/components/SessionActions.tsx`:
- Props: instanceId (string), pid (number), applicationName (string)
- Read user role from auth context — if viewer role, render nothing
- If applicationName starts with 'pgpulse_', render nothing (don't kill our own connections)
- Render Cancel button (amber/warning) and Terminate button (red/danger)
- Each button opens ConfirmModal with appropriate messaging:
  - Cancel: "Cancel the running query for PID {pid}?"
  - Terminate: "Terminate the entire session for PID {pid}? This will disconnect the client."
- On confirm: POST to `/api/v1/instances/{instanceId}/sessions/{pid}/cancel` or `/terminate`
- On success: show success toast, call an `onRefresh` callback prop
- On 403: show "Insufficient permissions" toast
- On 404: show "Session no longer active" toast
- On error: show generic error toast

Integrate into ServerDetail.tsx: add SessionActions as action column in the activity table.

### Task 2: Settings Diff UI

Create `web/src/components/SettingsDiff.tsx`:
- Fetches `GET /api/v1/instances/{instanceId}/settings/diff` on mount
- Groups results by `category` field into collapsible accordion sections
- Each section header: category name + count of changed settings in that category
- Table per section: Name | Current Value | Default Value | Unit | Context
- Rows where `pending_restart === true` get an amber "Restart Required" badge
- "Export CSV" button at the top — client-side download from in-memory data
  (Blob + createObjectURL + hidden anchor click)
- Empty state when array is empty: "All settings match defaults"

Integrate into ServerDetail.tsx: add "Settings Diff" as a new tab in the existing
tab navigation. Lazy-load — only fetch when the tab becomes active.

### Task 3: Query Plan Viewer UI

Create `web/src/components/PlanNode.tsx`:
- Recursive component for rendering EXPLAIN JSON plan tree nodes
- Props: node (the plan node object), depth (number, starts at 0)
- Display: Node Type (bold), Startup Cost → Total Cost, Actual Total Time,
  Plan Rows vs Actual Rows, Width
- Indentation via left padding or margin based on depth
- Highlight rules:
  - Actual Total Time > 100ms → amber background (bg-amber-50, border-l-4 border-l-amber-500)
  - Row estimate error ratio > 10x (either direction) → red left border
  - Both conditions → red background (bg-red-50)
- Recursively render child nodes from the `Plans` array

Create `web/src/components/QueryPlanViewer.tsx`:
- Props: instanceId (string), queryId (string)
- Fetches `GET /api/v1/instances/{instanceId}/statements/{queryId}/plan`
- Loading state: spinner
- Error state: "Plan unavailable — the statement may no longer exist"
- Renders PlanNode tree from the JSON response
- "Show Raw JSON" / "Hide Raw JSON" toggle button
- When toggled on: render raw EXPLAIN JSON in a `<pre>` block with monospace font

Integrate into ServerDetail.tsx: in the top queries table, add a clickable expand
icon on each row. Clicking toggles an expanded row below that renders QueryPlanViewer.

### Task 4: Forecast Overlay Extension

Create a helper hook in ServerDetail.tsx (or a new file `web/src/hooks/useForecastChart.ts`):

```typescript
function useForecastChart(instanceId: string, metric: string) {
  const forecast = useForecast(instanceId, metric);
  const extraSeries = useMemo(() => {
    if (!forecast?.points?.length) return undefined;
    return buildForecastSeries(forecast.points);
  }, [forecast]);
  const xAxisMax = useMemo(() => {
    if (!forecast?.points?.length) return undefined;
    return forecast.points[forecast.points.length - 1].timestamp;
  }, [forecast]);
  const nowMark = useMemo(() =>
    forecast ? getNowMarkLine(Date.now()) : undefined, [forecast]);
  return { extraSeries, xAxisMax, nowMarkLine: nowMark };
}
```

Refactor the existing connections_active forecast code to use this hook.
Then add the same hook call for each additional chart:
- `cache_hit_ratio`
- `transactions_per_sec`
- `replication_lag_bytes`
- `sessions_active`

Pass the returned `extraSeries`, `xAxisMax`, `nowMarkLine` to each chart's
`<TimeSeriesChart>` component. If forecast returns null, the chart renders
normally without any overlay.

### Important Notes for Frontend Agent

- Use only Tailwind core utility classes — no custom Tailwind config changes
- Import React hooks at top: `import { useState, useMemo, useEffect, useCallback } from "react"`
- Use the existing toast/notification system (find it in the codebase, do not create a new one)
- Use the existing auth context for role checks (find it in the codebase)
- Use the existing API fetch pattern (find how other components call the API — likely with
  Bearer token from auth context)
- Do NOT modify any Go files
- Do NOT modify `Administration.tsx` (known pre-existing lint error, not our scope)

---

## QA AGENT

### Verification Tasks

1. **TypeScript type-check:** Run `cd web && npx tsc --noEmit` and verify zero errors
2. **Lint:** Run `cd web && npm run lint` and verify zero new errors
   (the pre-existing `Administration.tsx` error is allowed)
3. **Build:** Run `cd web && npm run build` and verify it succeeds
4. **Go embed build:** Run `cd .. && go build ./cmd/pgpulse-server` to confirm
   the new frontend files are picked up by go:embed
5. **Go tests:** Run `go test ./cmd/... ./internal/...` — all existing tests must still pass

### Code Review Checks

- Verify SessionActions hides buttons for viewer role AND for pgpulse_* application names
- Verify ConfirmModal is generic and reusable (no session-specific logic inside it)
- Verify SettingsDiff CSV export includes all columns and handles commas in values
  (should quote fields containing commas)
- Verify PlanNode handles missing optional fields gracefully (Actual Total Time may be
  absent if EXPLAIN was run without ANALYZE)
- Verify forecast extension uses the helper hook pattern (no copy-paste of 15 lines × 4 charts)
- Verify no `localStorage` or `sessionStorage` usage
- Verify all new components have proper TypeScript types (no `any` types)
- Verify error states are handled for all API calls (loading, error, empty)

### If Build Fails

- Fix TypeScript errors in new components
- Fix lint errors in new components
- Do NOT fix the pre-existing `Administration.tsx` lint error

---

## Coordination

- Frontend Agent and QA Agent can work in parallel:
  - Frontend Agent creates components
  - QA Agent reviews and runs checks as files land
- Dependencies: QA cannot run final build verification until all Frontend Agent files are committed
- Merge only when QA confirms: typecheck clean, lint clean (minus known exception),
  build succeeds, Go build succeeds, Go tests pass
