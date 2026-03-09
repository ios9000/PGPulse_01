# M8_07 Design — Deferred UI + Small Fixes

**Iteration:** M8_07
**Date:** 2026-03-09
**Scope:** Primarily frontend; tiny backend touch for application_name + possible route registration

---

## Pre-Flight: Route Verification

Before building any UI, the agent MUST verify that the M8_02 handler routes are
actually registered in `internal/api/server.go`. Check for:

```go
// Plan capture routes — should be in a viewer or DBA permission group
r.Get("/instances/{id}/plans", ...)
r.Get("/instances/{id}/plans/{planId}", ...)
r.Get("/instances/{id}/plans/regressions", ...)
r.Post("/instances/{id}/plans/capture", ...)

// Settings snapshot routes
r.Get("/instances/{id}/settings/history", ...)
r.Get("/instances/{id}/settings/diff", ...)
r.Get("/instances/{id}/settings/latest", ...)
r.Get("/instances/{id}/settings/pending-restart", ...)
r.Post("/instances/{id}/settings/snapshot", ...)
```

If routes are missing: register them in `server.go` using the existing handler functions
from `plan_handlers.go` and `settings_handlers.go`. GET routes → viewer group. POST routes
→ DBA/instance_management group. This is the only acceptable backend change beyond the
application_name fix.

---

## 1. Plan Capture History UI

### New Files

**`web/src/components/PlanHistory.tsx`**

Main component for browsing captured query plans.

```tsx
interface CapturedPlan {
  id: string;
  instance_id: string;
  query_fingerprint: string;
  query_text: string;        // truncated in list, full in detail
  plan_json: object;         // EXPLAIN JSON
  plan_hash: string;
  trigger: 'duration' | 'scheduled' | 'manual' | 'hash_diff';
  database: string;
  total_cost: number;
  captured_at: string;       // ISO timestamp
}

interface PlanRegression {
  fingerprint: string;
  query_text: string;
  database: string;
  old_plan: CapturedPlan;
  new_plan: CapturedPlan;
  cost_change_pct: number;   // derived client-side
}
```

- Tabs or toggle: "All Plans" / "Regressions"
- All Plans tab:
  - Table: fingerprint (first 60 chars), database, trigger badge (coloured by type), cost, captured timestamp
  - Click row → expand inline with `PlanNode.tsx` tree (reuse from M8_06)
  - Sort by: captured_at (default desc), total_cost, trigger
  - Filter by: trigger type, database
- Regressions tab:
  - Table: fingerprint, database, old cost → new cost, change %, detected timestamp
  - Click row → expand with side-by-side plan tree comparison
  - For side-by-side: two `PlanNode` trees in a flex row, shared scroll
- "Capture Now" button (top-right, DBA+ role only):
  - POST to manual capture endpoint
  - Show toast on success/error
  - Auto-refresh plan list

**`web/src/hooks/usePlanHistory.ts`**

```tsx
function usePlanHistory(instanceId: string, options?: { trigger?: string; database?: string }) {
  // GET /api/v1/instances/{instanceId}/plans?trigger=...&database=...
  // 30s refetch
}

function usePlanRegressions(instanceId: string) {
  // GET /api/v1/instances/{instanceId}/plans/regressions
  // 60s refetch
}

function useManualCapture(instanceId: string) {
  // POST /api/v1/instances/{instanceId}/plans/capture
  // mutation hook
}
```

### Integration Point

New tab in ServerDetail: "Plan History" — lazy-loaded alongside existing tabs.

---

## 2. Temporal Settings Snapshot UI

### New Files

**`web/src/components/SettingsTimeline.tsx`**

```tsx
interface SettingsSnapshot {
  id: string;
  instance_id: string;
  captured_at: string;
  setting_count: number;
}

interface SettingChange {
  name: string;
  category: string;
  value_a: string;           // value at snapshot A
  value_b: string;           // value at snapshot B
  unit: string | null;
  change_type: 'changed' | 'added' | 'removed';
  pending_restart: boolean;
}
```

- Top section: snapshot timeline list with timestamps + "Take Snapshot" button
- Two snapshot selectors (dropdowns or timeline markers): "Compare" button
- Diff view (below selectors):
  - Accordion grouped by `category` (reuse pattern from `SettingsDiff.tsx`)
  - Each row: name, value at A, value at B, unit, change_type badge
  - Changed values: amber highlight on the differing cell
  - Added settings: green left border
  - Removed settings: red left border + strikethrough
  - `pending_restart` → amber badge (same as M8_06)
- "Pending Restart" quick-view button: shows only settings where `pending_restart = true`
  from the latest snapshot
- Empty state: "No settings snapshots captured yet. Enable settings_snapshot in config
  or click Take Snapshot."

**`web/src/hooks/useSettingsTimeline.ts`**

```tsx
function useSettingsSnapshots(instanceId: string) {
  // GET /api/v1/instances/{instanceId}/settings/history
  // 60s refetch
}

function useSettingsDiffBetween(instanceId: string, fromId: string, toId: string) {
  // GET /api/v1/instances/{instanceId}/settings/diff?from={fromId}&to={toId}
  // on-demand (not polling)
}

function usePendingRestart(instanceId: string) {
  // GET /api/v1/instances/{instanceId}/settings/pending-restart
  // 60s refetch
}

function useManualSnapshot(instanceId: string) {
  // POST /api/v1/instances/{instanceId}/settings/snapshot
  // mutation hook
}
```

### Integration Point

New tab in ServerDetail: "Settings Timeline" — placed next to existing "Settings Diff" tab.

---

## 3. application_name Enrichment

### Backend Change (Go)

Find the long transactions query (likely in `internal/collector/locks.go` or the activity
endpoint handler). The SQL probably queries `pg_stat_activity WHERE state = 'active' AND
xact_start < ...`. Add `application_name` to the SELECT list and to the Go struct.

Before:
```go
type LongTransaction struct {
    PID       int       `json:"pid"`
    // ... other fields
}
```

After:
```go
type LongTransaction struct {
    PID             int       `json:"pid"`
    ApplicationName string    `json:"application_name"`
    // ... other fields
}
```

### Frontend Change

In `LongTransactionsTable.tsx`, pass the new field to `SessionActions`:

```tsx
<SessionActions
  instanceId={instanceId}
  pid={tx.pid}
  applicationName={tx.application_name}  // was hardcoded or empty before
  onRefresh={refetch}
/>
```

No changes to `SessionActions.tsx` itself — it already filters `pgpulse_*`.

---

## 4. Administration.tsx Lint Fix

Find the early return that precedes `useState` calls. Move all hooks above it.

Typical pattern:
```tsx
// BEFORE (broken)
function Administration() {
  if (!hasPermission('admin')) return <NotAllowed />;
  const [tab, setTab] = useState('instances');  // ❌ conditional hook
  // ...
}

// AFTER (fixed)
function Administration() {
  const [tab, setTab] = useState('instances');  // ✅ always called
  if (!hasPermission('admin')) return <NotAllowed />;
  // ...
}
```

---

## File Summary

| Action | File |
|--------|------|
| CREATE | `web/src/components/PlanHistory.tsx` |
| CREATE | `web/src/hooks/usePlanHistory.ts` |
| CREATE | `web/src/components/SettingsTimeline.tsx` |
| CREATE | `web/src/hooks/useSettingsTimeline.ts` |
| MODIFY | `web/src/pages/ServerDetail.tsx` — add two new tabs |
| MODIFY | `web/src/pages/Administration.tsx` — lint fix |
| MODIFY | `web/src/components/LongTransactionsTable.tsx` — pass application_name |
| VERIFY/MODIFY | `internal/api/server.go` — ensure plan + settings routes registered |
| MODIFY | Long transactions struct/query — add application_name field |

---

## Build Verification

```bash
cd web && npm run build && npm run typecheck && npm run lint
cd .. && go build ./cmd/pgpulse-server && go test ./cmd/... ./internal/...
```

Expected: **0 lint errors** (the Administration.tsx fix eliminates the last one).
