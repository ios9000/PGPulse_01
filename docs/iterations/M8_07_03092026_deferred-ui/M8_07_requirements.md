# M8_07 Requirements — Deferred UI + Small Fixes

**Iteration:** M8_07
**Date:** 2026-03-09
**Scope:** Primarily frontend; one tiny backend change (application_name); one lint fix

---

## Goal

Ship frontend for two M8_02 backend features that have no UI, fix two small
quality issues, and close out the remaining M8 deferred items before starting M9.

---

## Feature 1: Plan Capture History UI

**Backend (already complete, M8_02):**
- `internal/api/plan_handlers.go` — `ListPlans`, `GetPlan`, `ListRegressions`, `ManualCapture`
- `internal/plans/store.go` — `PGPlanStore` with `SavePlan`, `ListPlans`, `GetPlan`, `ListRegressions`, `LatestPlanHash`
- `migrations/008_plan_capture.sql` — `query_plans` table

**⚠️ Route verification needed:** The M8_02 session log shows handlers were created but does
not explicitly confirm routes were registered in `server.go`. The Frontend Agent's first
step must be to verify routes exist. If missing, register them (this is a 5-line backend
change, acceptable for this iteration).

**Expected API shape (verify from actual handler code):**
- `GET /api/v1/instances/{id}/plans` — list captured plans (paginated/filtered)
- `GET /api/v1/instances/{id}/plans/{planId}` — get single plan detail
- `GET /api/v1/instances/{id}/plans/regressions` — list plan regressions (plan hash changed)
- `POST /api/v1/instances/{id}/plans/capture` — trigger manual plan capture

**Frontend requirements:**

1. New `PlanHistory.tsx` page or section in ServerDetail
2. Plans table: query fingerprint (truncated), captured timestamp, trigger type
   (duration/scheduled/manual/hash_diff), database name, cost estimate
3. Click a row → expand to show full EXPLAIN tree (reuse `PlanNode.tsx` from M8_06)
4. Regressions tab/filter: show only plans where hash changed (same fingerprint, new plan)
   - Side-by-side or before/after comparison for regression pairs
5. "Capture Now" button — calls manual capture endpoint, refreshes list
6. Empty state when plan capture is disabled or no plans collected yet

---

## Feature 2: Temporal Settings Snapshot UI

**Backend (already complete, M8_02):**
- `internal/api/settings_handlers.go` — `SettingsHistory`, `SettingsDiff`, `SettingsLatest`, `PendingRestart`, `ManualSnapshot`
- `internal/settings/store.go` — `PGSnapshotStore` with `SaveSnapshot`, `GetSnapshot`, `ListSnapshots`, `LatestSnapshot`
- `internal/settings/diff.go` — `DiffSnapshots` (Go-side): changed/added/removed/pending_restart
- `migrations/009_settings_snapshots.sql` — `settings_snapshots` table

**⚠️ Same route verification caveat as above.**

**Expected API shape (verify from actual handler code):**
- `GET /api/v1/instances/{id}/settings/history` — list snapshots (timestamp, count)
- `GET /api/v1/instances/{id}/settings/diff?from={snapA}&to={snapB}` — diff two snapshots
- `GET /api/v1/instances/{id}/settings/latest` — latest snapshot values
- `GET /api/v1/instances/{id}/settings/pending-restart` — settings requiring restart
- `POST /api/v1/instances/{id}/settings/snapshot` — trigger manual snapshot

**Frontend requirements:**

1. New `SettingsTimeline.tsx` component (distinct from M8_06's `SettingsDiff.tsx` which shows current vs defaults)
2. Snapshot timeline: list of snapshots with timestamps, "Take Snapshot" button
3. Diff view: select two snapshots → show changed settings
   - Group by category (reuse accordion pattern from SettingsDiff.tsx)
   - Each row: setting name, value at snapshot A, value at snapshot B, unit
   - Changed values highlighted
   - Added settings in green, removed in red
   - `pending_restart` badge where applicable
4. "Pending Restart" quick view — single-click to see settings needing restart
5. Mount as a new tab in ServerDetail: "Settings Timeline" alongside existing "Settings Diff"

**Key distinction:** M8_06's `SettingsDiff` shows current values vs boot defaults (single instance,
one point in time). This feature shows the same instance at time A vs time B (temporal comparison).

---

## Fix 3: Session Kill `application_name` Enrichment

**Backend change (tiny):**
- Add `application_name` to the long transactions API response
- This is likely a one-field addition to the SQL query in the long transactions collector
  and the corresponding Go struct + JSON serialization

**Frontend change:**
- `SessionActions.tsx` already has the `applicationName` prop and filters `pgpulse_*`
- `LongTransactionsTable.tsx` needs to pass the new `application_name` field from the API
  response to `SessionActions`
- Currently the guard doesn't fire because the field is absent

---

## Fix 4: Administration.tsx Lint Error

**Root cause:** `useState` called conditionally after an early return in the component.

**Fix:** Move all `useState` calls above the early return. This is a mechanical refactor —
hooks must be called in the same order on every render per React rules.

**Expected result:** `npm run lint` reports 0 errors (currently reports 1).

---

## Out of Scope

- Logical replication monitoring (M8_08)
- New alert rules
- ML model changes
- Any other backend features
