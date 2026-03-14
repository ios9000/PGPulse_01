# REM_01b — Team Prompt

**Paste this into Claude Code to spawn the agent team.**

---

Complete the remediation feature for PGPulse: backend gaps from REM_01a + full frontend (Advisor page, Diagnose button, alert row recommendations).
Read CLAUDE.md for project context, then read docs/iterations/REM_01b_03142026_remediation-frontend/design.md for the full design.

Create a team of 3 specialists:

## API & SECURITY AGENT

You own the backend gap work deferred from REM_01a. Read the existing code carefully before modifying.

### Task 1: Alert Response Enrichment — internal/api/alerts.go

**FIRST:** Read alerts.go to understand:
- The response struct(s) used for active alerts and alert history
- Whether they use inline structs or named types
- How alert events are queried and returned

**THEN:** Enrich alert event responses with recommendations:
- For each alert event in the response, call `s.remediationStore.ListByAlertEvent(ctx, eventID)`
- Attach the results as a `recommendations` field (JSON: `"recommendations"`, omitempty)
- If `s.remediationStore` is nil, skip enrichment entirely
- The `remediationStore` field already exists on APIServer from REM_01a's `SetRemediation()`

**IMPORTANT:**
- Do NOT create a new response type if one already exists — extend the existing one
- If the existing code returns `[]AlertEvent` directly, you may need a wrapper struct
- Check both `handleGetActiveAlerts` and `handleGetAlertHistory` handlers

### Task 2: Email Template — internal/alert/template.go

**FIRST:** Read template.go to understand:
- How the HTML email is built (template strings? html/template?)
- What data is passed to the template
- What struct carries the notification context

**FIRST:** Read dispatcher.go to understand:
- Where `runRemediation()` (added in REM_01a) is called
- What it returns and where results are stored
- How the notification payload is assembled before calling Notifier.Send()

**THEN:** Add a "Recommendations" section to the email:
- Insert after alert details, before footer
- Each recommendation: colored left border (info=#3B82F6, suggestion=#EAB308, action_required=#EF4444), bold priority label, title, description
- If doc_url present, add a "Documentation →" link
- If no recommendations, omit the section entirely (no empty header)

**Integration flow:**
- The dispatcher already calls `runRemediation()` — results need to flow into the notification payload
- This may require:
  1. Extending the notification context/payload struct to include `[]RemediationResult`
  2. Modifying the `notify()` / `dispatch()` path to pass recommendations along
  3. Updating the template rendering to conditionally include the recommendations section
- Follow the existing code patterns exactly — do not restructure the notification flow

### Task 3: Dispatcher Notification Payload — internal/alert/dispatcher.go

**Only if needed by Task 2:**
- Extend the notification struct/payload to carry `[]RemediationResult`
- Ensure `runRemediation()` results are available when building the notification
- Keep changes minimal — this is a data passthrough, not a refactor

### CRITICAL RULES FOR THIS AGENT:
- Read EVERY file you plan to modify BEFORE making changes
- Follow existing patterns exactly (struct naming, error handling, response format)
- Do NOT import internal/remediation from internal/alert (use the RemediationResult type already in internal/alert/remediation.go)
- All SQL must use parameterized queries
- Test scope: `go test ./cmd/... ./internal/...`

---

## FRONTEND AGENT

You own all frontend work. The 5 API endpoints from REM_01a already work — you can start immediately.

### Task 1: TypeScript Types — web/src/types/models.ts

Add these types to the EXISTING models.ts file (do not create a new file):

```typescript
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

Also find the existing alert event type (AlertEvent or AlertHistoryItem) and add:
```typescript
recommendations?: Recommendation[];
```

### Task 2: API Hooks — web/src/hooks/useRecommendations.ts (~90 lines)

**FIRST:** Read `web/src/hooks/useAlerts.ts` and `web/src/hooks/useAlertRules.ts` to understand the hook pattern (useState + useEffect + useCallback with api client).

Create hooks:
- `useRecommendations(params)` — GET /api/v1/recommendations (fleet-wide, paginated, filterable)
- `useInstanceRecommendations(instanceId, params)` — GET /api/v1/instances/{id}/recommendations
- `useDiagnose(instanceId)` — POST /api/v1/instances/{id}/diagnose (manual trigger, not auto-fetch)
- `useAcknowledge()` — PUT /api/v1/recommendations/{id}/acknowledge
- `useRemediationRules()` — GET /api/v1/recommendations/rules

The `useDiagnose` hook should NOT auto-fetch on mount. It should return a `diagnose()` function that triggers the API call on demand and returns the result.

### Task 3: PriorityBadge Component — web/src/components/advisor/PriorityBadge.tsx (~25 lines)

**FIRST:** Read `web/src/components/shared/AlertBadge.tsx` and `web/src/components/ui/StatusBadge.tsx` for the badge pattern.

Create a reusable priority badge:
- `action_required` → `bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-200` → "Action Required"
- `suggestion` → `bg-amber-100 text-amber-800 dark:bg-amber-900 dark:text-amber-200` → "Suggestion"
- `info` → `bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200` → "Info"
- Small rounded badge, same size/style as AlertBadge

### Task 4: AdvisorFilters Component — web/src/components/advisor/AdvisorFilters.tsx (~100 lines)

**FIRST:** Read `web/src/components/alerts/AlertFilters.tsx` for the filter pattern.

Four dropdown filters in a horizontal row:
- Priority: All / Info / Suggestion / Action Required
- Category: All / Performance / Capacity / Configuration / Replication / Maintenance
- Status: All / New / Acknowledged
- Instance: All / [populated from useInstances hook]

Each dropdown calls an `onChange` prop to update the parent's filter state.

### Task 5: AdvisorRow Component — web/src/components/advisor/AdvisorRow.tsx (~80 lines)

**FIRST:** Read `web/src/components/alerts/AlertRow.tsx` for the expandable row pattern.

Expandable table row:
- Collapsed: Priority badge | Instance (link to /servers/{id}) | Category | Title (truncated) | Created (relative time) | Status
- Expanded: Full description | Doc URL link (if present) | Acknowledge button (if not yet acknowledged)
- Acknowledge button calls `useAcknowledge().acknowledge(rec.id)` and updates local state

### Task 6: Advisor Page — web/src/pages/Advisor.tsx (~200 lines)

**FIRST:** Read `web/src/pages/AlertsDashboard.tsx` for the page layout pattern.

Full-page layout:
- PageHeader: "Advisor"
- AdvisorFilters component
- Data table with AdvisorRow components
- Pagination controls (Previous / Next buttons + page indicator)
- Empty state: "No recommendations found. All monitored instances are healthy."
- Uses `useRecommendations()` hook with filter state as params
- Loading state: Spinner component

### Task 7: DiagnosePanel Component — web/src/components/server/DiagnosePanel.tsx (~80 lines)

Slide-down results panel:
- Props: `results: DiagnoseResponse`, `onClose: () => void`
- Header: "Diagnosis Results" + close button (X)
- Summary line: "Found {N} recommendations ({M} rules evaluated)"
- List of recommendations: PriorityBadge + title + description + optional doc URL link
- If zero recommendations: "All clear — no issues detected."
- Styled with Tailwind: rounded border, subtle background, padding

### Task 8: Diagnose Button — Modify web/src/components/server/HeaderCard.tsx

**FIRST:** Read HeaderCard.tsx to understand its current layout and props.

Add a "Diagnose" button:
- Position: alongside existing header content (next to instance name/info)
- Icon: Lightbulb from lucide-react
- On click: calls `useDiagnose(instanceId).diagnose()`
- Loading state: spinner instead of icon
- Passes result up to parent via callback prop `onDiagnoseResult`

The HeaderCard may need a new prop: `onDiagnose?: () => void` and `diagnosing?: boolean`
OR the diagnose state can be managed in ServerDetail.tsx and passed down.

**Read ServerDetail.tsx** to decide which approach is cleaner.

### Task 9: Wire DiagnosePanel in ServerDetail.tsx

**FIRST:** Read `web/src/pages/ServerDetail.tsx` to understand the component structure.

- Add state: `const [diagnoseResults, setDiagnoseResults] = useState<DiagnoseResponse | null>(null)`
- Wire `useDiagnose(instanceId)` hook
- Pass diagnose trigger to HeaderCard
- Render `DiagnosePanel` conditionally below HeaderCard when results exist

### Task 10: Alert Row Recommendations — Modify web/src/components/alerts/AlertRow.tsx

**FIRST:** Read AlertRow.tsx to understand the expand/collapse pattern.

In the expanded section, after existing alert details:
- Check if `alert.recommendations` exists and has items
- If yes, render a "Recommendations" sub-section with each recommendation showing:
  - PriorityBadge + title + description + optional doc URL link
- If no recommendations, show nothing (no empty state noise)

Also check `web/src/components/server/InstanceAlerts.tsx`:
- If it renders individual expandable alert rows, apply the same recommendation display
- If it's just a summary/count, leave it alone

### Task 11: Sidebar Navigation — Modify web/src/components/layout/Sidebar.tsx

**FIRST:** Read Sidebar.tsx to understand how nav items are defined.

Add "Advisor" entry:
- Position: between Alerts and Admin
- Icon: `Lightbulb` from `lucide-react`
- Path: `/advisor`
- Label: "Advisor"

### Task 12: Route — Modify web/src/App.tsx

Add the Advisor route:
```tsx
import Advisor from './pages/Advisor';
// In Routes:
<Route path="/advisor" element={<Advisor />} />
```

### CRITICAL RULES FOR THIS AGENT:
- Read EVERY file you plan to modify BEFORE making changes
- Follow existing component patterns exactly (Tailwind classes, hook patterns, prop conventions)
- All new components go in the correct directory (advisor/, server/, or hooks/)
- Import PriorityBadge from components/advisor/ in all files that need it
- Use relative time formatting from existing formatters.ts if available
- Verify `Lightbulb` icon exists in lucide-react before importing
- Do NOT add new npm dependencies
- Run `npm run build && npm run typecheck && npm run lint` before committing

---

## QA & REVIEW AGENT

You own verification and deferred test files. Wait for both API and Frontend agents to finish their work before running the full test suite.

### Task 1: Backend Handler Tests — internal/api/remediation_test.go (~250 lines)

**FIRST:** Read existing API test files (e.g., `internal/api/alerts_test.go` or similar) to understand the test pattern: how APIServer is set up for tests, how mocks are used, how HTTP requests are constructed.

Write tests:
- `TestHandleListRecommendations` — GET with pagination, GET with filters, GET empty result
- `TestHandleDiagnose` — POST returns recommendations, POST on instance with no metrics
- `TestHandleListAllRecommendations` — GET fleet-wide, GET with filters
- `TestHandleAcknowledgeRecommendation` — PUT success (200), PUT non-existent (404)
- `TestHandleListRemediationRules` — GET returns 25 rules

Use in-memory mocks or NullStore where appropriate. If the project has test helper functions for setting up the API server, use them.

### Task 2: Store Integration Tests — internal/remediation/pgstore_test.go (~200 lines)

**FIRST:** Check if the project uses testcontainers-go for database tests. Look at `internal/storage/pgstore_test.go` for the pattern.

If testcontainers available:
- TestPGStore_WriteAndList — write recommendations, list by instance
- TestPGStore_Filters — priority, category, acknowledged filters
- TestPGStore_AlertEventLink — write with alert_event_id, list by event
- TestPGStore_Acknowledge — acknowledge and verify timestamp
- TestPGStore_CleanOld — retention cleanup

If testcontainers NOT available:
- Use `//go:build integration` tag
- Gate on `PGPULSE_TEST_DSN` environment variable
- Document in test file how to run

### Task 3: Frontend Build Verification

```bash
cd web
npm run build
npm run typecheck
npm run lint
```

All must pass. If there are TypeScript errors, identify the file and line, then fix or coordinate with Frontend Agent.

### Task 4: Backend Build & Test Regression

```bash
cd ..
go build ./cmd/... ./internal/...
go test ./cmd/... ./internal/... -count=1
golangci-lint run ./cmd/... ./internal/...
```

All must pass with zero errors. Any regressions must be fixed before merging.

### Task 5: Integration Validation

Verify these integration points work:
- `internal/remediation` does NOT import `internal/alert` in any cycle-causing way (only imports alert.RemediationResult)
- `internal/alert` does NOT import `internal/remediation`
- All 5 remediation API endpoints respond correctly
- NullStore path works (remediation-related code doesn't panic when store is nil/null)

### CRITICAL RULES FOR THIS AGENT:
- Do NOT start full regression until both API and Frontend agents confirm their work compiles
- If you find a test failure, check if it's pre-existing or caused by REM_01b changes
- Use `t.Parallel()` where safe
- Test scope: `go test ./cmd/... ./internal/...` (not `./...`)
- Frontend: `cd web && npm run build && npm run typecheck && npm run lint`

---

## COORDINATION NOTES

- **API Agent** and **Frontend Agent** work in PARALLEL — no dependency between them for initial work
- Frontend Agent can use the existing REM_01a endpoints immediately (they already work)
- The only dependency: AlertRow recommendations display depends on API Agent enriching alert responses — but Frontend Agent can write the component code optimistically since the type shape is known
- **QA Agent** starts after both agents complete, then runs full verification
- Final merge only when ALL builds pass: Go build + test + lint AND npm build + typecheck + lint
- After completion: regenerate docs/CODEBASE_DIGEST.md per .claude/rules/codebase-digest.md
