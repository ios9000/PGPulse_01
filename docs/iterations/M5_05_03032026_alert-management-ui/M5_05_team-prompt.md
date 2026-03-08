# M5_05 — Alert Management UI: Team Prompt

**Paste this into Claude Code to begin implementation.**

---

Build the Alert Management UI for PGPulse. This is frontend-only — NO backend changes.
Read `docs/iterations/M5_05_03032026_alert-management-ui/design.md` for the full technical design.
Read `.claude/CLAUDE.md` for project context.

The alert backend is complete (M4). Two placeholder pages need real implementations.

Create a team of 2 specialists:

---

## FRONTEND AGENT

You own all files in `web/src/`. Your task is to build the alert management UI.

### Step 1: Types and Hooks

**Modify `web/src/types/models.ts`** — add these types (verify they don't already exist):
```typescript
export interface Alert {
  id: string;
  rule_id: string;
  rule_name: string;
  instance_id: string;
  severity: 'warning' | 'critical';
  metric: string;
  value: number;
  threshold: number;
  labels: Record<string, string>;
  state: 'firing' | 'resolved';
  fired_at: string;
  resolved_at: string | null;
  message: string;
}

export interface AlertRule {
  id: string;
  name: string;
  metric: string;
  condition: '>' | '<' | '>=' | '<=' | '==' | '!=';
  warning_threshold: number | null;
  critical_threshold: number | null;
  duration: number;
  cooldown: number;
  labels: Record<string, string>;
  channels: string[];
  enabled: boolean;
  description: string;
  is_system: boolean;
}

export type AlertSeverityFilter = 'all' | 'warning' | 'critical';
export type AlertStateFilter = 'all' | 'firing' | 'resolved';
```

**Create `web/src/hooks/useAlerts.ts`:**
- `useAlerts(options: { severity?, state?, instanceId? })` — GET /api/v1/alerts with query params, refetchInterval 30s
- Follow the exact same pattern as existing hooks (useStatements, useLockTree, etc.)

**Create `web/src/hooks/useAlertRules.ts`:**
- `useAlertRules()` — GET /api/v1/alerts/rules, refetchInterval 60s
- `useSaveAlertRule()` — POST /api/v1/alerts/rules mutation, invalidates alertRules query on success
- `useTestNotification()` — POST /api/v1/alerts/test mutation

### Step 2: Alert Components

**Create `web/src/components/alerts/AlertFilters.tsx`:**
- Toggle buttons for severity (All/Warning/Critical) and state (Firing/Resolved/All)
- Instance dropdown (populated from useInstances hook)
- Style like the existing TimeRangeSelector preset buttons

**Create `web/src/components/alerts/AlertRow.tsx`:**
- Single table row: severity badge, rule name, instance, metric, value, threshold, state, fired time, duration
- Duration: for firing alerts show live duration (now - fired_at), for resolved show (resolved_at - fired_at)
- Click navigates to `/servers/${instanceId}`
- Use AlertBadge or StatusBadge for severity indicator

**Create `web/src/components/alerts/RuleRow.tsx`:**
- Single table row: name (with ⚙ system badge if is_system), metric, condition, thresholds, cooldown, channels as chips, enable/disable toggle, edit/delete buttons
- Toggle calls useSaveAlertRule with updated enabled field
- Delete button only shows for non-system rules
- Edit button opens RuleFormModal

**Create `web/src/components/alerts/RuleFormModal.tsx`:**
- Modal overlay (fixed inset-0, bg-black/50, centered card bg-gray-900)
- Props: open, onClose, rule? (undefined=create, defined=edit)
- Form fields: name, description, metric, condition (select), warning_threshold, critical_threshold, duration (number + unit), cooldown (number + unit), channels (checkboxes), enabled
- Available channels: extract from all existing rules' channels arrays (union set)
- Duration/cooldown: display in user-friendly units (minutes/hours), convert to seconds on submit
- Validation: name required, at least one threshold required
- System rules: name field read-only
- "Test Notification" button (visible when editing existing rule with channels)
- Error display: inline error message area below the form
- Submit: call useSaveAlertRule mutation, on success close + show brief success state

**Create `web/src/components/alerts/DeleteConfirmModal.tsx`:**
- Simple modal: "Are you sure you want to delete [rule name]?" with Cancel and Delete buttons
- Delete button calls the API (POST with delete flag or a dedicated endpoint — check what the backend supports)

### Step 3: Pages

**Replace `web/src/pages/AlertsPage.tsx`** (currently a placeholder):
- Page title: "Active Alerts" with count badge
- AlertFilters component at top
- Read `instance_id` from URL search params for initial filter (for linking from InstanceAlerts)
- Table of AlertRow components
- Empty state: green checkmark icon + "All clear — no active alerts" when filtered list is empty
- Loading state: Spinner component
- Wrap entire content in a Card

**Replace `web/src/pages/AlertRulesPage.tsx`** (currently a placeholder):
- Wrap in `<PermissionGate permission="alert_management">`
- Page title: "Alert Rules" with "+ Create Rule" button
- Table of RuleRow components
- "Create Rule" button opens RuleFormModal in create mode
- Each row's edit button opens RuleFormModal in edit mode
- Each non-system row's delete button opens DeleteConfirmModal
- Loading/empty states

### Step 4: Integration

**Modify `web/src/components/server/InstanceAlerts.tsx`:**
- Add a "View all alerts →" link at the bottom that navigates to `/alerts?instance_id=${instanceId}`
- Use React Router's Link component

### Styling Rules

- Dark-mode-first: bg-gray-900, bg-gray-800 for cards, text-gray-100 for primary text
- Severity colors: critical=red-500, warning=amber-500, resolved/ok=green-500
- Tables: same styling as StatementsSection and LockTreeSection
- Modal: bg-gray-900 border border-gray-700 rounded-lg shadow-xl
- Form inputs: bg-gray-800 border-gray-700 text-gray-100 rounded focus:ring-blue-500
- Toggle switch: custom Tailwind — bg-gray-600 when off, bg-blue-600 when on
- Filter buttons: match TimeRangeSelector toggle pattern
- No new CSS files — Tailwind utility classes only
- No new npm dependencies

---

## QA & REVIEW AGENT

Your job is to verify the frontend code quality after the Frontend Agent completes each step.

### Checks After Each Step

1. **Type safety:** Run `npx tsc --noEmit` from `web/` directory — must pass with zero errors
2. **Lint:** Run `npx eslint src/` from `web/` directory — must pass with zero errors
3. **Build:** Run `npx vite build` from `web/` directory — must succeed
4. **Import verification:** Check that all new files are properly imported where used (no orphan files)
5. **Consistency check:**
   - All hooks follow the same pattern as existing hooks (useStatements, useLockTree)
   - All components follow the same dark-mode styling as existing components
   - No `any` types used
   - No hardcoded colors — use Tailwind classes
   - Modal has proper escape-key and click-outside-to-close handling
   - All user-facing strings are clear and professional

### Backend Compatibility Check

Read the existing API handler files to verify:
- The response shapes from `GET /api/v1/alerts` and `GET /api/v1/alerts/rules` match the TypeScript types
- Query parameters (severity, state, instance_id) are supported by the backend
- If any mismatch is found: document it clearly and flag for the developer. Do NOT modify backend code.
- Check if `DELETE /api/v1/alerts/rules/:id` exists. If not, document how the frontend should handle rule deletion.

### Final Verification

After all steps complete:
1. `npx tsc --noEmit` — pass
2. `npx eslint src/` — pass
3. `npx vite build` — pass (check bundle size, note any increase)
4. `go build ./cmd/... ./internal/...` — pass (should be unchanged)
5. `go vet ./...` — pass
6. `go test ./...` — pass (no regressions)
7. `golangci-lint run` — pass

List all files created/modified with line counts.

---

## Coordination Notes

- Frontend Agent can start immediately — all backend endpoints exist
- QA Agent verifies after each step, not just at the end
- If the API response shape doesn't match the TypeScript types, QA Agent documents the gap — do NOT change backend
- Team Lead: merge only when all QA checks pass
- No bash limitations — agents can run build/test/lint directly (Claude Code v2.1.63, Windows bash works)
