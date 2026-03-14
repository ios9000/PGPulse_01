# REM_01b — Design Document

**Iteration:** REM_01b — Remediation Frontend + Backend Gaps
**Date:** 2026-03-14
**Follows:** REM_01a (commits ab5336d, 1b12266)

---

## 1. Overview

REM_01b completes the remediation feature across three workstreams:

1. **Backend gaps** (API Agent): alert response enrichment, email templates, handler/store tests
2. **Frontend** (Frontend Agent): Advisor page, Diagnose button, alert row recommendations, sidebar nav
3. **Verification** (QA Agent): full regression, frontend build, integration validation

---

## 2. Backend Changes (API Agent)

### 2.1 Alert Response Enrichment — internal/api/alerts.go

> **PRE-FLIGHT FINDING:** The existing handlers return `[]alert.AlertEvent` directly
> inside a generic `Envelope{Data: events}` — there is NO named response wrapper struct.
> The Go `AlertEvent` struct (`internal/alert/alert.go:101`) has **no `ID` field**, even
> though the DB table `alert_history` has `id BIGSERIAL PRIMARY KEY`. The `eventColumns`
> const and `scanEvent()` function in `pgstore.go` do not SELECT or scan the `id` column.

**Required backend changes (in order):**

1. **Add `ID int64` to `AlertEvent`** in `internal/alert/alert.go`:
   ```go
   type AlertEvent struct {
       ID           int64             `json:"id"`
       // ... all existing fields unchanged ...
   }
   ```

2. **Update `eventColumns` and `scanEvent()`** in `internal/alert/pgstore.go`:
   - Prepend `id` to `eventColumns` const
   - Add `&ev.ID` as first scan target in `scanEvent()`

3. **Create a response wrapper** in `internal/api/alerts.go`:
   ```go
   type alertEventResponse struct {
       alert.AlertEvent
       Recommendations []remediation.Recommendation `json:"recommendations,omitempty"`
   }
   ```

4. **In `handleGetActiveAlerts` and `handleGetAlertHistory`:**
   - After fetching `[]alert.AlertEvent` from the history store
   - Build `[]alertEventResponse` by copying each event
   - For each event with `ID > 0`, call `s.remediationStore.ListByAlertEvent(ctx, event.ID)`
   - Attach results to the response wrapper
   - If `s.remediationStore` is nil, skip enrichment (recommendations field will be empty/omitted)
   - Return `[]alertEventResponse` in the `Envelope{Data: ...}`

The APIServer already has `remediationStore` wired from REM_01a's `SetRemediation()`. Use it directly.

### 2.2 Email Template — internal/alert/template.go

Add a recommendations section to the HTML email template. The `Dispatcher` already has access to `RemediationProvider` from REM_01a.

**Approach:**
- After calling `d.remediation.EvaluateForAlert()` in the dispatcher's fire path, pass the results to the template renderer
- Add a "Recommendations" section in the email HTML between the alert details and footer
- Each recommendation renders as:
  ```html
  <div style="margin: 8px 0; padding: 8px; border-left: 3px solid {color};">
    <strong>[{PRIORITY}]</strong> {Title}
    <p>{Description}</p>
    {optional: <a href="{DocURL}">Documentation →</a>}
  </div>
  ```
- Priority colors: info=#3B82F6 (blue), suggestion=#EAB308 (yellow), action_required=#EF4444 (red)
- If no recommendations, omit the entire section (don't show an empty "Recommendations" header)

**Integration with Dispatcher (PRE-FLIGHT VERIFIED):**

> **Actual code flow in `dispatcher.go`:**
> 1. `processEvent()` checks cooldown, then loops channels calling `sendWithRetry(notifier, event)` (lines 101-108)
> 2. `runRemediation()` is called **AFTER** notifications are sent (line 112)
> 3. `runRemediation()` calls `d.remediation.EvaluateForAlert()` but **discards results** — only logs the count
> 4. `Notifier.Send(ctx context.Context, event AlertEvent)` takes only `AlertEvent` — no recommendations payload
>
> **Conclusion:** Recommendations currently cannot reach the email template.

**Required changes to `dispatcher.go`:**
1. **Reorder:** Move `runRemediation()` call **before** the notification loop in `processEvent()`
2. **Capture results:** Change `runRemediation()` to return `[]RemediationResult` instead of void
3. **Do NOT change `Notifier.Send()` interface** — that would break all notifier implementations

**Required changes to template rendering:**
1. Add a `Recommendations []RemediationResult` field to `templateData` struct in `template.go`
2. Change `RenderHTMLTemplate` and `RenderTextTemplate` signatures to accept recommendations:
   ```go
   func RenderHTMLTemplate(event AlertEvent, dashboardURL string, recs []RemediationResult) (string, error)
   func RenderTextTemplate(event AlertEvent, dashboardURL string, recs []RemediationResult) (string, error)
   ```
3. Update callers: `notifier/email.go` `Send()` must pass recommendations through
4. Since `Notifier.Send()` only receives `AlertEvent`, the email notifier needs another way to get recs.
   **Best approach:** Add a `Recommendations []RemediationResult` field to `AlertEvent` struct
   (transient, not persisted — json tag `json:"recommendations,omitempty"`)
   and populate it in `processEvent()` before calling `sendWithRetry()`.
   This avoids changing the `Notifier` interface.

### 2.3 Handler Tests — internal/api/remediation_test.go

HTTP handler tests using `httptest.NewRecorder` following the pattern in existing test files:

| Test | Endpoint | Validates |
|------|----------|-----------|
| TestHandleListRecommendations | GET /instances/{id}/recommendations | Pagination, filters, empty result |
| TestHandleDiagnose | POST /instances/{id}/diagnose | Returns recommendations from engine |
| TestHandleListAllRecommendations | GET /recommendations | Fleet-wide listing |
| TestHandleAcknowledgeRecommendation | PUT /recommendations/{id}/acknowledge | 200 on success, 404 on missing |
| TestHandleListRemediationRules | GET /recommendations/rules | Returns all 25 rules |

Use mock/in-memory implementations where possible. If existing API tests use a test helper or fixture pattern, follow it.

### 2.4 Store Integration Tests — internal/remediation/pgstore_test.go

Database integration tests (build-tag guarded with `//go:build integration` if the project uses that pattern, or use testcontainers if available):

| Test | Validates |
|------|-----------|
| TestPGStore_WriteAndListByInstance | Write 3 recs, list by instance, verify count |
| TestPGStore_ListAll_Filters | Write recs with different priorities/categories, filter each |
| TestPGStore_ListByAlertEvent | Write recs with alert_event_id, verify filter |
| TestPGStore_Acknowledge | Write rec, acknowledge, verify timestamp set |
| TestPGStore_CleanOld | Write old + new recs, clean, verify only old removed |

If testcontainers is not available in the test suite, use build-tag guarded tests that require a live database connection string via env var.

---

## 3. Frontend Changes (Frontend Agent)

### 3.1 TypeScript Types — web/src/types/models.ts

Add to the existing models.ts file:

```typescript
// Remediation types
export type RecommendationPriority = 'info' | 'suggestion' | 'action_required';
export type RecommendationCategory = 'performance' | 'capacity' | 'configuration' | 'replication' | 'maintenance';

export interface Recommendation {
  id: number;
  rule_id: string;
  instance_id: string;
  alert_event_id?: number;
  metric_key: string;
  metric_value: number;
  priority: RecommendationPriority;
  category: RecommendationCategory;
  title: string;
  description: string;
  doc_url?: string;
  created_at: string;
  acknowledged_at?: string;
  acknowledged_by?: string;
}

export interface DiagnoseResponse {
  recommendations: Recommendation[];
  metrics_evaluated: number;
  rules_evaluated: number;
}

export interface RemediationRule {
  id: string;
  priority: RecommendationPriority;
  category: RecommendationCategory;
}
```

Also extend the existing `AlertEvent` interface in `models.ts` (line 162) with two new fields:

```typescript
export interface AlertEvent {
  id: number              // NEW — maps to alert_history.id BIGSERIAL
  rule_id: string
  rule_name: string
  // ... all other existing fields unchanged ...
  recommendations?: Recommendation[]  // NEW — populated by alert enrichment API
}
```

### 3.2 API Hooks — web/src/hooks/useRecommendations.ts (~90 lines)

Follow the `useAlerts.ts` and `useAlertRules.ts` pattern:

```typescript
// useRecommendations.ts

import { useState, useEffect, useCallback } from 'react';
import api from '../lib/api';
import { Recommendation, DiagnoseResponse, RemediationRule } from '../types/models';

interface ListParams {
  instanceId?: string;
  priority?: string;
  category?: string;
  acknowledged?: boolean | null;
  limit?: number;
  offset?: number;
}

// Fleet-wide recommendation listing (for Advisor page)
export function useRecommendations(params: ListParams) {
  // GET /api/v1/recommendations with query params
  // Returns { recommendations, total, loading, error, refetch }
}

// Instance-specific recommendations
export function useInstanceRecommendations(instanceId: string, params?: ListParams) {
  // GET /api/v1/instances/{id}/recommendations
  // Returns { recommendations, total, loading, error, refetch }
}

// Diagnose (on-demand)
export function useDiagnose(instanceId: string) {
  // Returns { diagnose: () => Promise<DiagnoseResponse>, loading, result, error }
  // POST /api/v1/instances/{id}/diagnose — triggered manually, not on mount
}

// Acknowledge a recommendation
export function useAcknowledge() {
  // Returns { acknowledge: (id: number) => Promise<void>, loading }
  // PUT /api/v1/recommendations/{id}/acknowledge
}

// List compiled-in rules (for reference/display)
export function useRemediationRules() {
  // GET /api/v1/recommendations/rules
  // Returns { rules, loading, error }
}
```

### 3.3 Advisor Page — web/src/pages/Advisor.tsx (~200 lines)

Full-page data table following the `AlertsDashboard.tsx` pattern.

**Layout:**
```
┌─────────────────────────────────────────────────────────────┐
│  PageHeader: "Advisor"                                       │
├─────────────────────────────────────────────────────────────┤
│  Filter Bar:                                                 │
│  [Priority ▼] [Category ▼] [Status ▼] [Instance ▼]          │
├─────────────────────────────────────────────────────────────┤
│  Data Table:                                                 │
│  Priority │ Instance │ Category │ Title │ Created │ Status   │
│  ─────────┼──────────┼──────────┼───────┼─────────┼────────  │
│  🔴 act.. │ prod-01  │ capacity │ Conn..│ 2m ago  │ New      │
│  ▼ (expanded row)                                            │
│    Description text here. Consider adding PgBouncer...       │
│    📎 Documentation →          [Acknowledge]                 │
│  🟡 sugg..│ prod-02  │ perf     │ Cache.│ 15m ago │ Acked    │
│  🔵 info  │ prod-01  │ config   │ Enabl.│ 1h ago  │ Acked    │
├─────────────────────────────────────────────────────────────┤
│  Pagination: < 1 2 3 ... 10 >                               │
└─────────────────────────────────────────────────────────────┘
```

**Columns:**
- **Priority**: Color-coded badge (action_required=red, suggestion=amber, info=blue)
- **Instance**: Instance name/ID, clickable → navigates to ServerDetail
- **Category**: Capitalized category name
- **Title**: Recommendation title (truncated to ~60 chars in table, full on expand)
- **Created**: Relative time (e.g., "2m ago", "1h ago")
- **Status**: "New" or "Acknowledged" with timestamp

**Row Expansion:**
- Click row to expand/collapse
- Shows full description text
- If doc_url present: link "Documentation →" opening in new tab
- "Acknowledge" button (visible only if not yet acknowledged, requires alert_management permission)
- After acknowledge: row updates status to "Acknowledged" without full refetch

**Filter Bar** (follow AlertFilters.tsx pattern):
- Priority dropdown: All / Info / Suggestion / Action Required
- Category dropdown: All / Performance / Capacity / Configuration / Replication / Maintenance
- Status dropdown: All / New / Acknowledged
- Instance dropdown: populated from /api/v1/instances (reuse useInstances hook)

**Empty state**: "No recommendations found. All monitored instances are healthy."

### 3.4 Advisor Filter Component — web/src/components/advisor/AdvisorFilters.tsx (~100 lines)

Follows `AlertFilters.tsx` pattern. Four dropdowns in a horizontal row.

### 3.5 Advisor Row Component — web/src/components/advisor/AdvisorRow.tsx (~80 lines)

Follows `AlertRow.tsx` pattern. Expandable row with priority badge, details, and acknowledge button.

### 3.6 Priority Badge Component — web/src/components/advisor/PriorityBadge.tsx (~25 lines)

Reusable badge component:
- `action_required` → red background, white text, "Action Required"
- `suggestion` → amber/yellow background, dark text, "Suggestion"
- `info` → blue background, white text, "Info"

Uses Tailwind classes matching the existing `StatusBadge.tsx` and `AlertBadge.tsx` patterns.

### 3.7 Diagnose Button — Modify web/src/components/server/HeaderCard.tsx

> **PRE-FLIGHT FINDING:** `HeaderCard` props are `{ instanceName, host, port, currentMetrics }`.
> It does **not** receive `instanceId`. The layout is a single flex container with StatusBadge,
> h1 title, host:port, version/role badges, and uptime.

**Two options for wiring the Diagnose button:**

**Option A (preferred): Add `instanceId` prop to HeaderCard.**
Add `instanceId: string` to `HeaderCardProps`. The parent `ServerDetail.tsx` already has the ID.
This keeps the button visually in the header where it belongs.

**Option B: Keep Diagnose logic in ServerDetail.tsx.**
Render the button outside HeaderCard, in the parent. Less clean visually.

**Go with Option A.** Add `instanceId` to props, then add the button:

```tsx
// In HeaderCard.tsx, add instanceId to props:
interface HeaderCardProps {
  instanceId: string    // NEW
  instanceName: string
  host: string
  port: number
  currentMetrics: CurrentMetricsResult | undefined
}

// Add button inside the existing flex container (after uptime span):
<button
  onClick={handleDiagnose}
  disabled={diagnosing}
  className="px-3 py-1.5 text-sm bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-50"
>
  {diagnosing ? <Spinner size="sm" /> : <Lightbulb size={16} />}
  Diagnose
</button>
```

**Diagnose result display:**
- On click: calls `useDiagnose(instanceId).diagnose()`
- While loading: button shows spinner
- On success: results shown in a slide-down panel below HeaderCard (DiagnosePanel component)
- Panel shows: summary line ("Found N recommendations from M rules evaluated") + list of recommendations
- Each recommendation: PriorityBadge + title + description
- Panel dismissible with X button
- If zero recommendations: "All clear — no issues detected."

### 3.8 Diagnose Results Panel — web/src/components/server/DiagnosePanel.tsx (~80 lines)

New component rendered conditionally below HeaderCard in ServerDetail.tsx:

```tsx
// In ServerDetail.tsx:
{diagnoseResults && (
  <DiagnosePanel
    results={diagnoseResults}
    onClose={() => setDiagnoseResults(null)}
  />
)}
```

### 3.9 Alert Row Recommendations — Modify web/src/components/alerts/AlertRow.tsx

> **PRE-FLIGHT FINDING:** `AlertRow.tsx` currently has **NO expand/collapse**. It is a plain
> `<tr>` that navigates to `/servers/{instance_id}` on click. There is no expanded content area,
> no chevron icon, no local state for expansion.
>
> `InstanceAlerts.tsx` similarly shows individual alert cards (div-based, not table rows)
> with **no expand/collapse pattern**. Each card shows severity badge, rule_name, metric line,
> and timestamp as a flat layout.

**Required changes to `AlertRow.tsx`:**

1. **Add expand/collapse state:** `const [expanded, setExpanded] = useState(false)`
2. **Split click behavior:** Click on the row toggles expansion. Add a separate link/button
   to navigate to the server page (e.g., clickable instance_id cell, or a "View Server" link
   in the expanded area).
3. **Add expanded content row:** After the main `<tr>`, conditionally render a second `<tr>`
   with `colSpan` spanning all columns, containing:
   - Alert details (metric, value, threshold — already in main row but can show more context)
   - Recommendations section (if `alert.recommendations` is present and non-empty)
4. **Add chevron icon:** First cell gets a ChevronRight/ChevronDown toggle indicator.

```tsx
export function AlertRow({ alert }: AlertRowProps) {
  const [expanded, setExpanded] = useState(false)
  const navigate = useNavigate()
  const isResolved = !!alert.resolved_at

  return (
    <>
      <tr
        onClick={() => setExpanded(!expanded)}
        className="cursor-pointer border-b border-pgp-border transition-colors hover:bg-pgp-bg-hover"
      >
        {/* existing cells... */}
        {/* Add chevron in first cell */}
      </tr>
      {expanded && (
        <tr className="border-b border-pgp-border bg-pgp-bg-secondary">
          <td colSpan={8} className="px-8 py-4">
            {/* Link to server */}
            <button onClick={() => navigate(`/servers/${alert.instance_id}`)}
              className="text-sm text-blue-400 hover:text-blue-300 mb-3">
              View Server →
            </button>
            {/* Recommendations */}
            {alert.recommendations && alert.recommendations.length > 0 && (
              <div className="mt-3 border-t border-pgp-border pt-3">
                <h4 className="text-sm font-medium text-pgp-text-muted mb-2">Recommendations</h4>
                {alert.recommendations.map(rec => (
                  <div key={rec.id} className="flex items-start gap-2 mb-2">
                    <PriorityBadge priority={rec.priority} />
                    <div>
                      <p className="text-sm font-medium text-pgp-text-primary">{rec.title}</p>
                      <p className="text-xs text-pgp-text-muted">{rec.description}</p>
                      {rec.doc_url && (
                        <a href={rec.doc_url} target="_blank" rel="noopener noreferrer"
                          className="text-xs text-blue-400 hover:underline">
                          Documentation →
                        </a>
                      )}
                    </div>
                  </div>
                ))}
              </div>
            )}
          </td>
        </tr>
      )}
    </>
  )
}
```

**`InstanceAlerts.tsx`:** No changes needed. It shows a summary card list (not a table),
and recommendations don't fit the compact card layout. Users can click "View all alerts →"
to reach the Alerts page where expandable rows show recommendations.

### 3.10 Sidebar Navigation — Modify web/src/components/layout/Sidebar.tsx

Add "Advisor" nav item between Alerts and Admin:

```tsx
// In Sidebar.tsx navItems array, add between Alerts and Settings Diff:
// NOTE: The actual field names are `label` (not `name`) and `icon`, matching NavItem interface.
{ label: 'Advisor', icon: Lightbulb, path: '/advisor' },
```

Import `Lightbulb` from `lucide-react`.

### 3.11 Route Registration — Modify web/src/App.tsx

Add the Advisor route:

```tsx
import Advisor from './pages/Advisor';

// In the Routes:
<Route path="/advisor" element={<Advisor />} />
```

---

## 4. File Inventory

### New Files (8)

| File | Lines (est.) | Owner |
|------|-------------|-------|
| `web/src/pages/Advisor.tsx` | ~200 | Frontend Agent |
| `web/src/components/advisor/AdvisorFilters.tsx` | ~100 | Frontend Agent |
| `web/src/components/advisor/AdvisorRow.tsx` | ~80 | Frontend Agent |
| `web/src/components/advisor/PriorityBadge.tsx` | ~25 | Frontend Agent |
| `web/src/components/server/DiagnosePanel.tsx` | ~80 | Frontend Agent |
| `web/src/hooks/useRecommendations.ts` | ~90 | Frontend Agent |
| `internal/api/remediation_test.go` | ~250 | QA Agent |
| `internal/remediation/pgstore_test.go` | ~200 | QA Agent |

### Modified Files (12)

| File | Change | Owner |
|------|--------|-------|
| `web/src/types/models.ts` | Add Recommendation, DiagnoseResponse, RemediationRule types; extend AlertEvent with `id` + `recommendations` | Frontend Agent |
| `web/src/App.tsx` | Add /advisor route | Frontend Agent |
| `web/src/components/layout/Sidebar.tsx` | Add Advisor nav item with Lightbulb icon (field: `label`, not `name`) | Frontend Agent |
| `web/src/components/server/HeaderCard.tsx` | Add `instanceId` prop + Diagnose button | Frontend Agent |
| `web/src/pages/ServerDetail.tsx` | Wire DiagnosePanel below HeaderCard; pass `instanceId` to HeaderCard | Frontend Agent |
| `web/src/components/alerts/AlertRow.tsx` | Add expand/collapse state + expanded row with recommendations | Frontend Agent |
| `internal/alert/alert.go` | Add `ID int64` and `Recommendations []RemediationResult` (transient) to AlertEvent | API Agent |
| `internal/alert/pgstore.go` | Add `id` to `eventColumns` + `scanEvent()` | API Agent |
| `internal/api/alerts.go` | Create `alertEventResponse` wrapper; enrich with recommendations from remediationStore | API Agent |
| `internal/alert/template.go` | Add `Recommendations` field to `templateData`; add recommendations section to HTML/text templates; update render function signatures | API Agent |
| `internal/alert/dispatcher.go` | Reorder: run remediation BEFORE notifications; capture results; attach to event | API Agent |
| `internal/alert/notifier/email.go` | Update `RenderHTMLTemplate`/`RenderTextTemplate` calls to pass `event.Recommendations` | API Agent |

---

## 5. Dependencies Between Agents

```
API Agent (backend gaps)          Frontend Agent              QA Agent
─────────────────────            ─────────────────           ──────────
1. alerts.go enrichment ─────┐   1. types/models.ts          (waits for
2. template.go update    │   2. hooks                     both agents)
3. dispatcher.go tweak   │   3. PriorityBadge
                         │   4. AdvisorFilters             1. remediation_test.go
                         │   5. AdvisorRow                 2. pgstore_test.go
                         └──→ 6. Advisor.tsx               3. frontend build check
                              7. Sidebar.tsx               4. full regression
                              8. HeaderCard.tsx
                              9. DiagnosePanel.tsx
                              10. AlertRow.tsx
                              11. App.tsx
                              12. ServerDetail.tsx
```

**Parallelism:** API Agent and Frontend Agent work independently. The Frontend Agent can build the Advisor page, Diagnose button, and all components using the existing API endpoints (which already work from REM_01a). The alert row recommendations depend on the API Agent's alerts.go enrichment, but the component code can be written optimistically since the type is known.

QA Agent starts after both land, runs full regression.

---

## 6. Live Mode Behavior

| Component | Live Mode | Persistent Mode |
|-----------|-----------|-----------------|
| Advisor page | Shows empty state (NullStore returns no data) | Full fleet-wide listing |
| Diagnose button | Works (queries MemoryStore) | Works (queries PG store) |
| Alert row recommendations | Empty (no enrichment from NullStore) | Shows recommendations |
| Acknowledge button | No-op (NullStore) | Persists |

---

## 7. Pre-Flight Issue Checklist — VERIFIED 2026-03-14

All items verified and design doc updated with findings.

| # | Check | Finding | Design Doc Updated? |
|---|-------|---------|---------------------|
| 1 | `alerts.go` response struct | No named type — returns `[]AlertEvent` in `Envelope{Data}`. `AlertEvent` has **no ID field** despite DB having `id BIGSERIAL`. `scanEvent()` and `eventColumns` skip the `id` column. | YES — §2.1 rewritten with 4-step plan to add ID |
| 2 | `dispatcher.go` remediation flow | `runRemediation()` runs AFTER notifications (line 112), results are discarded (only logged). `Notifier.Send()` takes only `AlertEvent` — no recs payload. | YES — §2.2 rewritten with reorder + transient field approach |
| 3 | `AlertRow.tsx` expand pattern | NO expand/collapse. Plain `<tr>` that navigates on click. No local state, no chevron, no expanded area. | YES — §3.9 rewritten with full expand/collapse implementation |
| 4 | `HeaderCard.tsx` props | `{instanceName, host, port, currentMetrics}` — NO `instanceId`. Flex layout. | YES — §3.7 updated: add `instanceId` prop (Option A) |
| 5 | `Sidebar.tsx` nav items | Typed array `navItems: NavItem[]` with `{label, icon, path, permission?}`. Field is `label` not `name`. | YES — §3.10 fixed field name to `label` |
| 6 | `Lightbulb` in lucide-react | CONFIRMED: `lightbulb.js` exists in `dist/esm/icons/` | No change needed |
| 7 | `InstanceAlerts.tsx` | Shows individual alert cards (div-based), NO expand/collapse. Summary cards with badge + metric line. | YES — §3.9 notes no changes needed to InstanceAlerts |

---

## 8. Test Strategy

### Backend Tests (QA Agent)

**internal/api/remediation_test.go** (~250 lines):
- Set up test APIServer with mock stores
- Test all 5 remediation endpoints
- Test pagination params
- Test filter params
- Test 404 on acknowledge non-existent
- Test auth required on acknowledge

**internal/remediation/pgstore_test.go** (~200 lines):
- If testcontainers available: full integration tests
- If not: build-tag guarded or env-var gated
- Test Write + List round-trip
- Test filter combinations
- Test acknowledge flow
- Test CleanOld retention

### Frontend Verification (QA Agent)

```bash
cd web
npm run build      # production build succeeds
npm run typecheck  # no TypeScript errors
npm run lint       # no lint errors
```

### Full Regression

```bash
go build ./cmd/... ./internal/...
go test ./cmd/... ./internal/... -count=1
golangci-lint run ./cmd/... ./internal/...
```
