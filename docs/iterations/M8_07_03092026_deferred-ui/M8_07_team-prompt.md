# M8_07 Team Prompt — Deferred UI + Small Fixes

Read CLAUDE.md for full project context.
Read `docs/iterations/M8_07/M8_07_design.md` for detailed component specs.

This is primarily frontend work with two small backend touches. Use a
**2-specialist team** (Frontend Agent + QA Agent).

---

Create a team of 2 specialists:

## FRONTEND AGENT

### CRITICAL FIRST STEP: Route Verification

Before writing any UI code, verify that the M8_02 API routes are registered
in `internal/api/server.go`. Search for handler references from:
- `internal/api/plan_handlers.go` — `ListPlans`, `GetPlan`, `ListRegressions`, `ManualCapture`
- `internal/api/settings_handlers.go` — `SettingsHistory`, `SettingsDiff`, `SettingsLatest`, `PendingRestart`, `ManualSnapshot`

If the routes are NOT registered in server.go:
- Register GET routes in the viewer permission group
- Register POST routes (ManualCapture, ManualSnapshot) in the instance_management permission group
- This is the same pattern used by all other routes in server.go

Also examine the actual handler signatures and response shapes — the design doc's TypeScript
interfaces are best-guesses. Adjust the frontend types to match what the handlers actually return.

### Task 1: application_name Enrichment (Backend)

Find the long transactions query and response struct. Likely in one of:
- `internal/api/` (activity handler)
- `internal/collector/locks.go`
- Whatever handler serves `GET /api/v1/instances/{id}/activity/long-transactions`

Add `application_name` to:
1. The SQL SELECT list (from `pg_stat_activity`)
2. The Go struct (with `json:"application_name"` tag)
3. Verify the API response now includes the field

### Task 2: Administration.tsx Lint Fix

Open `web/src/pages/Administration.tsx`. Find where `useState` is called after
an early return (conditional hook). Move ALL `useState` and other hook calls
above any early returns. This is a mechanical fix — the component logic stays
the same, just hook ordering changes.

After fix: `npm run lint` should report **0 errors** (not 1).

### Task 3: Plan Capture History UI

Create `web/src/hooks/usePlanHistory.ts`:
- `usePlanHistory(instanceId, options?)` — fetches plan list, 30s refetch
- `usePlanRegressions(instanceId)` — fetches regressions, 60s refetch
- `useManualCapture(instanceId)` — POST mutation for manual capture

Create `web/src/components/PlanHistory.tsx`:
- Tabs: "All Plans" and "Regressions"
- All Plans tab:
  - Table: query fingerprint (truncated to ~60 chars), database, trigger type badge
    (duration=blue, scheduled=gray, manual=green, hash_diff=amber), total cost, captured timestamp
  - Click row → expand inline with plan tree (reuse `PlanNode.tsx` from M8_06)
  - Sort by captured_at (desc default)
- Regressions tab:
  - Table: fingerprint, database, old cost → new cost, change percentage, detected timestamp
  - Click row → expand with two PlanNode trees side by side (flex row)
- "Capture Now" button (top-right):
  - Check role — only show for users with instance_management permission
  - POST to manual capture API
  - Toast on success/error, auto-refresh list
- Empty state: "No query plans captured. Enable plan_capture in configuration."

Integrate into ServerDetail.tsx as a new "Plan History" tab.

### Task 4: Temporal Settings Snapshot UI

Create `web/src/hooks/useSettingsTimeline.ts`:
- `useSettingsSnapshots(instanceId)` — fetches snapshot list, 60s refetch
- `useSettingsDiffBetween(instanceId, fromId, toId)` — on-demand diff between two snapshots
- `usePendingRestart(instanceId)` — fetches pending restart settings, 60s refetch
- `useManualSnapshot(instanceId)` — POST mutation for manual snapshot

Create `web/src/components/SettingsTimeline.tsx`:
- Snapshot list with timestamps at the top
- Two dropdown selectors for picking snapshot A and snapshot B
- "Compare" button → calls diff API → renders diff below
- Diff view: accordion grouped by category (reuse pattern from SettingsDiff.tsx)
  - Each row: setting name, value at A, value at B, unit
  - Changed: amber highlight on differing cell
  - Added: green left border
  - Removed: red left border + strikethrough text
  - pending_restart: amber badge
- "Take Snapshot" button (instance_management permission only)
  - POST to manual snapshot API
  - Toast on success, refresh snapshot list
- "Pending Restart" quick-view button: filters to pending_restart=true from latest snapshot
- Empty state: "No settings snapshots captured yet."

Integrate into ServerDetail.tsx as "Settings Timeline" tab (next to existing "Settings Diff").

### Task 5: application_name Frontend Wiring

In `web/src/components/LongTransactionsTable.tsx`:
- The API response now includes `application_name` — pass it to the `SessionActions` component
- `SessionActions` already accepts `applicationName` prop and filters `pgpulse_*`

### Important Notes for Frontend Agent

- Read the actual Go handler code to determine exact API response shapes before writing TypeScript types
- Reuse existing components: `PlanNode.tsx` for plan trees, accordion pattern from `SettingsDiff.tsx`
- Use existing toast system (Toast.tsx + toastStore.ts from M8_06)
- Use existing auth context for permission checks
- No new Go files except the tiny application_name addition
- Do NOT modify `internal/ml/`, `internal/alert/`, or `internal/collector/` directories

---

## QA AGENT

### Verification Tasks

1. **Route check:** Confirm all plan_handlers and settings_handlers routes exist in server.go
2. **application_name:** Verify the long transactions API response includes the new field
   (inspect the actual Go struct, not just the handler)
3. **TypeScript type-check:** `cd web && npx tsc --noEmit` — zero errors
4. **Lint:** `cd web && npm run lint` — **zero errors** (Administration.tsx fix must eliminate the last one)
5. **Build:** `cd web && npm run build` — success
6. **Go build:** `go build ./cmd/pgpulse-server` — success
7. **Go tests:** `go test ./cmd/... ./internal/...` — all pass (application_name change should not break tests)

### Code Review Checks

- Verify Administration.tsx has ALL hooks above any conditional returns
- Verify PlanHistory reuses PlanNode.tsx (not a new plan tree implementation)
- Verify SettingsTimeline diff view uses accordion pattern consistent with SettingsDiff.tsx
- Verify "Capture Now" and "Take Snapshot" buttons are permission-gated
- Verify LongTransactionsTable passes application_name to SessionActions
- Verify no `any` types in new TypeScript code
- Verify all API calls include proper error handling (loading, error, empty states)

---

## Coordination

- Frontend Agent handles everything (including the tiny Go change for application_name)
- QA Agent reviews and runs checks as files land
- Merge only when QA confirms: lint = 0 errors, typecheck clean, build succeeds, Go build + tests pass
