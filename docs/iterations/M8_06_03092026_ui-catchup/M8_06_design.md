# M8_06 Design — UI Catch-Up + Forecast Extension

**Iteration:** M8_06
**Date:** 2026-03-09
**Scope:** Frontend-only (React/TypeScript/Tailwind CSS)

---

## Architecture Notes

All four features are frontend additions. No Go code changes, no new migrations,
no new API endpoints. The team-prompt should spawn a **Frontend Agent** and a
**QA Agent** — two specialists, not the full four.

---

## 1. Session Kill UI

### New Files

**`web/src/components/SessionActions.tsx`**

```tsx
// Props
interface SessionActionsProps {
  instanceId: string;
  pid: number;
  applicationName: string;  // hide buttons for pgpulse_* connections
}
```

- Renders Cancel and Terminate buttons
- Reads user role from auth context (`useAuth()` hook or similar)
- If role === 'viewer' OR applicationName starts with 'pgpulse_', render nothing
- Cancel button: `className="text-amber-600 hover:bg-amber-50"` (warning style)
- Terminate button: `className="text-red-600 hover:bg-red-50"` (danger style)

**`web/src/components/ConfirmModal.tsx`** (generic, reusable)

```tsx
interface ConfirmModalProps {
  open: boolean;
  title: string;
  message: string;
  confirmLabel: string;       // "Cancel Query" / "Terminate Session"
  confirmVariant: 'warning' | 'danger';
  onConfirm: () => void;
  onCancel: () => void;
  loading?: boolean;
}
```

- Tailwind modal with backdrop overlay
- Confirm button colour matches variant (amber for warning, red for danger)
- Shows spinner on confirm button while `loading=true`

### Integration Point

In `ServerDetail.tsx` (or wherever the activity table is rendered), add
`<SessionActions>` as the last column of each activity row. Wire the API calls:

```typescript
const cancelSession = async (pid: number) => {
  await fetch(`/api/v1/instances/${instanceId}/sessions/${pid}/cancel`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${token}` },
  });
};
```

After success: show toast via existing toast system, then refetch activity data.

### Error Handling

| HTTP Status | UI Response |
|-------------|-------------|
| 200 | Success toast: "Query cancelled for PID {pid}" / "Session {pid} terminated" |
| 403 | Error toast: "Insufficient permissions" |
| 404 | Error toast: "Session no longer active" |
| 500 | Error toast: "Failed — server error" |

---

## 2. Settings Diff UI

### New Files

**`web/src/components/SettingsDiff.tsx`**

```tsx
interface SettingDiff {
  name: string;
  setting: string;
  boot_val: string;
  unit: string | null;
  category: string;
  context: string;
  pending_restart: boolean;
}
```

- Fetches `GET /api/v1/instances/{id}/settings/diff` on mount
- Groups results by `category` using a `Map<string, SettingDiff[]>`
- Renders collapsible accordion sections (one per category)
- Each section header: category name + count badge
- Table inside each section: Name | Current | Default | Unit | Context | Status
- `pending_restart` rows get an amber "Restart Required" badge
- Empty state: informational card "All settings match defaults"

**CSV Export:**

```typescript
const exportCsv = () => {
  const header = 'name,current,default,unit,category,context,pending_restart\n';
  const rows = diffs.map(d =>
    `${d.name},${d.setting},${d.boot_val},${d.unit ?? ''},${d.category},${d.context},${d.pending_restart}`
  ).join('\n');
  const blob = new Blob([header + rows], { type: 'text/csv' });
  const url = URL.createObjectURL(blob);
  // trigger download via hidden <a> element
};
```

### Integration Point

Add a "Settings Diff" tab in ServerDetail's tab navigation (alongside existing
tabs like Overview, Activity, etc.). Lazy-load the component — only fetch data
when the tab is active.

---

## 3. Query Plan Viewer UI

### New Files

**`web/src/components/QueryPlanViewer.tsx`**

Main component. Fetches plan, manages raw JSON toggle state.

```tsx
interface QueryPlanViewerProps {
  instanceId: string;
  queryId: string;
}
```

- Fetches `GET /api/v1/instances/{id}/statements/{queryId}/plan`
- Loading spinner while fetching
- Error state: "Plan unavailable — the statement may no longer exist"
- Toggle button: "Show Raw JSON" / "Hide Raw JSON"

**`web/src/components/PlanNode.tsx`**

Recursive tree node component.

```tsx
interface PlanNodeProps {
  node: ExplainNode;
  depth: number;
}

interface ExplainNode {
  'Node Type': string;
  'Startup Cost': number;
  'Total Cost': number;
  'Actual Total Time'?: number;
  'Plan Rows': number;
  'Actual Rows'?: number;
  'Plan Width': number;
  Plans?: ExplainNode[];
  // ... other EXPLAIN fields
}
```

- Indented tree with connecting lines (left border or tree-line CSS)
- Each node card shows: Node Type (bold), cost range, actual time, row comparison
- **Highlight rules:**
  - `Actual Total Time > 100` → amber background (`bg-amber-50 border-l-amber-500`)
  - `Actual Rows / Plan Rows > 10` (or Plan Rows / Actual Rows > 10) → red left border
  - Both conditions → red background (`bg-red-50`)

### Integration Point

In the top queries table (ServerDetail), add a clickable icon/button on each row.
Clicking opens the plan viewer. Two viable patterns:

- **Option A: Expandable row** — plan renders inline below the query row
- **Option B: Slide-out panel** — plan renders in a right-side drawer

Recommend **Option A** (expandable row) for simplicity — no new layout components needed.
Just conditionally render `<QueryPlanViewer>` below the selected row.

---

## 4. Forecast Overlay Extension

### No New Files

Reuse existing:
- `web/src/hooks/useForecast.ts`
- `web/src/components/ForecastBand.ts`
- `web/src/components/charts/TimeSeriesChart.tsx` (already has the props)

### Changes in ServerDetail.tsx

For each additional chart, replicate the pattern already used for `connections_active`:

```tsx
// Example for cache_hit_ratio
const cacheHitForecast = useForecast(instanceId, 'cache_hit_ratio');

const cacheHitExtraSeries = useMemo(() => {
  if (!cacheHitForecast?.points?.length) return undefined;
  return buildForecastSeries(cacheHitForecast.points);
}, [cacheHitForecast]);

const cacheHitXAxisMax = useMemo(() => {
  if (!cacheHitForecast?.points?.length) return undefined;
  const last = cacheHitForecast.points[cacheHitForecast.points.length - 1];
  return last.timestamp;
}, [cacheHitForecast]);

const nowMarkLine = useMemo(() => getNowMarkLine(Date.now()), []);

// In JSX:
<TimeSeriesChart
  // ... existing props
  extraSeries={cacheHitExtraSeries}
  xAxisMax={cacheHitXAxisMax}
  nowMarkLine={cacheHitForecast ? nowMarkLine : undefined}
/>
```

Repeat for: `transactions_per_sec`, `replication_lag_bytes`, `sessions_active`.

### Potential Refactor

If ServerDetail gets too verbose with five identical forecast blocks, extract a
helper hook:

```tsx
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

This keeps ServerDetail clean and makes future chart additions trivial.

---

## File Summary

| Action | File |
|--------|------|
| CREATE | `web/src/components/SessionActions.tsx` |
| CREATE | `web/src/components/ConfirmModal.tsx` |
| CREATE | `web/src/components/SettingsDiff.tsx` |
| CREATE | `web/src/components/QueryPlanViewer.tsx` |
| CREATE | `web/src/components/PlanNode.tsx` |
| MODIFY | `web/src/pages/ServerDetail.tsx` — integrate all four features |

---

## Build Verification

```bash
cd web && npm run build && npm run typecheck && npm run lint
cd .. && go build ./cmd/pgpulse-server && go test ./cmd/... ./internal/...
```

No Go changes expected, but run the full stack to confirm `go:embed` still picks
up the new frontend build.
