# M5_05 — Alert Management UI: Requirements

**Iteration:** M5_05
**Date:** 2026-03-03
**Depends on:** M4 (alert engine, complete), M5_01–M5_04 (frontend scaffold + all server detail sections)

---

## Goal

Replace the two placeholder pages (AlertsPage.tsx, AlertRulesPage.tsx) with fully functional alert management UI. The backend is 100% complete — this iteration is frontend-only with zero backend changes.

---

## Decisions (Locked)

| ID | Decision | Rationale |
|----|----------|-----------|
| D103 | Modal dialog for rule create/edit | Keeps rule list visible; enough fields to warrant overlay |
| D104 | Show firing + resolved (last 24h) by default, with state filter | Empty-page problem when healthy; DBAs need recent timeline |
| D105 | Channels displayed as selectable names from YAML config | Channel config stays in YAML for now; rule form references by ID |
| D106 | Default rules editable but not deletable; "system" badge | DBAs tune thresholds per environment; prevents monitoring gaps |
| D107 | No bulk actions; single enable/disable toggle per rule | ~20-30 rules don't justify bulk UI; add later if needed |

---

## Functional Requirements

### FR-1: Active Alerts Page (AlertsPage.tsx)

**FR-1.1** Display all alerts from `GET /api/v1/alerts` in a sortable table.

**FR-1.2** Columns: severity badge (warning=amber, critical=red), rule name, instance name, metric, current value, threshold, state (firing/resolved), fired time, duration (live-updating for firing alerts).

**FR-1.3** Filter controls:
- Severity: All / Warning / Critical (toggle buttons or chips)
- State: Firing / Resolved / All (toggle buttons)
- Instance: dropdown of known instances (from `GET /api/v1/instances`)

**FR-1.4** Default filter: Firing + Resolved, all severities, all instances.

**FR-1.5** Auto-refresh every 30 seconds via TanStack Query `refetchInterval`.

**FR-1.6** Clicking an alert row navigates to the relevant instance's Server Detail page (`/servers/:instanceId`).

**FR-1.7** Empty state when no alerts: show a green "All clear" indicator with explanatory text.

**FR-1.8** Accessible to all authenticated roles (read-only view).

### FR-2: Alert Rules Page (AlertRulesPage.tsx)

**FR-2.1** Display all rules from `GET /api/v1/alerts/rules` in a table.

**FR-2.2** Columns: name, metric, condition (human-readable, e.g., "> 80%"), warning threshold, critical threshold, cooldown, channels (chip list), enabled toggle, system badge (for defaults), actions (edit button).

**FR-2.3** Enable/disable toggle per rule — calls `POST /api/v1/alerts/rules` with updated `enabled` field.

**FR-2.4** "Create Rule" button opens modal dialog.

**FR-2.5** Edit button on each row opens the same modal, pre-populated with rule data.

**FR-2.6** Delete button on user-created rules (not system defaults) — confirmation dialog before calling API.

**FR-2.7** Permission-gated: entire page requires `alert_management` permission. Wrap in `<PermissionGate permission="alert_management">`.

**FR-2.8** Auto-refresh every 60 seconds (rules change rarely).

### FR-3: Rule Form Modal

**FR-3.1** Fields:
- Name (text input, required)
- Description (textarea, optional)
- Metric (dropdown — populated from known metric names, or free text)
- Condition (select: >, <, >=, <=, ==, !=)
- Warning threshold (number input, optional)
- Critical threshold (number input, optional)
- Duration (number + unit select: seconds/minutes/hours — how long condition must hold)
- Cooldown (number + unit select — minimum time between re-fires)
- Channels (multi-select checkboxes from available channel names)
- Enabled (checkbox, default true)

**FR-3.2** At least one of warning_threshold or critical_threshold must be provided. Client-side validation.

**FR-3.3** Submit calls `POST /api/v1/alerts/rules`. On success: close modal, invalidate rules query to refresh list, show success toast/notification.

**FR-3.4** Error handling: display API errors inline in the modal (e.g., duplicate name, invalid metric).

**FR-3.5** System default rules: modal opens in edit mode but name field is read-only and delete is hidden.

### FR-4: Test Notification

**FR-4.1** "Test" button in the rule form modal (visible when editing an existing rule that has channels configured).

**FR-4.2** Calls `POST /api/v1/alerts/test` with the rule's channel configuration.

**FR-4.3** Shows success ("Test notification sent") or failure ("Failed to send: ...") inline in the modal.

### FR-5: Integration Points

**FR-5.1** Sidebar navigation: "Alerts" → AlertsPage, "Alert Rules" → AlertRulesPage (already configured, verify links work).

**FR-5.2** InstanceAlerts section on Server Detail page: verify the "View All Alerts" link navigates to AlertsPage filtered by that instance.

**FR-5.3** Fleet Overview: instance cards already show alert badge counts — no changes needed.

---

## Non-Functional Requirements

**NFR-1** No new npm dependencies. Use existing Tailwind, TanStack Query, Zustand, Lucide icons, ECharts.

**NFR-2** Dark-mode-first styling consistent with existing pages. Reuse shared components (DataTable, StatusBadge, Card, AlertBadge, Spinner).

**NFR-3** TypeScript strict mode — no `any` types.

**NFR-4** All new code passes `tsc --noEmit` and `eslint src/`.

**NFR-5** No backend changes. If API response shape doesn't match expectations, adapt the frontend, or flag for a future backend tweak.

---

## Out of Scope

- Notification channel management UI (stays in YAML config)
- Alert history beyond 24h / alert archival
- Alert rule templating or import/export
- Alert grouping or silencing/muting
- Bulk rule operations
- WebSocket/SSE real-time alert push (polling is sufficient)
