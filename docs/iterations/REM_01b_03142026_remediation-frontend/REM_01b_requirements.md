# REM_01b — Remediation Frontend + Backend Gaps

**Iteration:** REM_01b
**Date:** 2026-03-14
**Scope:** Advisor page, alert detail recommendations, Diagnose button, deferred backend work
**Follows:** REM_01a (commits ab5336d, 1b12266)

---

## Goal

Complete the remediation feature by building the frontend Advisor page, integrating
recommendations into the alert detail view, adding the Diagnose button, and finishing
backend gaps deferred from REM_01a (alert response enrichment, email templates, tests).

## Decisions (Locked)

| ID | Decision | Choice |
|----|----------|--------|
| D309 | Agent strategy | 3 agents: API (backend gaps) + Frontend (all frontend) + QA (tests) |
| D310 | Advisor page layout | Full-page data table with filters (AlertsDashboard pattern) |
| D311 | Diagnose button location | ServerDetail page header (next to instance name) |
| D312 | Advisor page navigation | New top-level nav item in Sidebar.tsx (lightbulb icon) |
| D313 | Alert detail recommendations | Inline in alert row (expand alert → see recommendations) |
| D314 | Advisor page route | /advisor (top-level route) |

## Functional Requirements

### FR-1: Advisor Page (/advisor)
- Fleet-wide recommendation listing via GET /api/v1/recommendations
- Data table with sortable columns: Priority, Instance, Category, Title, Created, Status
- Filter bar: priority (info/suggestion/action_required), category (5 values), acknowledged (yes/no/all)
- Pagination (100 per page)
- Priority badges: color-coded (info=blue, suggestion=yellow, action_required=red)
- Row expansion: click row → show full description + doc URL link + acknowledge button
- Empty state when no recommendations exist

### FR-2: Diagnose Button
- Button in ServerDetail HeaderCard component, next to instance name
- Calls POST /api/v1/instances/{id}/diagnose
- Shows loading spinner during evaluation
- Results displayed in a modal or expandable section below the button
- Each result shows: priority badge, title, description, doc URL
- Works in both live and persistent modes

### FR-3: Alert Row Recommendation Enrichment
- Expand an alert row → see attached recommendations inline below alert details
- Recommendations fetched from enriched alert API response (alert_event_id → recommendations)
- Each recommendation: priority badge, title, description
- If no recommendations for an alert, show nothing (no empty state noise)

### FR-4: Sidebar Navigation
- New top-level nav item "Advisor" in Sidebar.tsx
- Position: between Alerts and Admin
- Icon: lightbulb (from lucide-react)
- Badge: count of unacknowledged action_required recommendations (optional — can defer)

### FR-5: Backend — Alert Response Enrichment (deferred from REM_01a)
- Modify alerts.go: embed recommendations[] in alert history and active alert responses
- Query remediationStore.ListByAlertEvent(alertEventID) for each event
- In live mode (NullStore), skip enrichment (empty array)

### FR-6: Backend — Email Template (deferred from REM_01a)
- Modify template.go: add "Recommendations" section to alert email
- Show priority badge (text), title, description for each recommendation
- If no recommendations, omit the section entirely

### FR-7: Backend Tests (deferred from REM_01a)
- internal/api/remediation_test.go — HTTP handler tests for all 5 endpoints
- internal/remediation/pgstore_test.go — integration tests (build-tag guarded if needed)

## Non-Functional Requirements

- Frontend components follow existing Tailwind CSS patterns
- All new TypeScript types added to models.ts
- New hooks follow useAlerts.ts / useAlertRules.ts pattern
- No new npm dependencies (lucide-react already available)
- Frontend build must pass: npm run build, typecheck, lint
- Backend build must pass: go build, go test, golangci-lint
