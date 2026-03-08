# M5_05 — Alert Management UI: Technical Design

**Iteration:** M5_05
**Date:** 2026-03-03
**Scope:** Frontend-only — AlertsPage, AlertRulesPage, RuleFormModal, supporting hooks and types

---

## 1. File Plan

### New Files

| File | Purpose |
|------|---------|
| `web/src/pages/AlertsPage.tsx` | Replace placeholder — active alerts list with filters |
| `web/src/pages/AlertRulesPage.tsx` | Replace placeholder — rule CRUD with enable/disable |
| `web/src/components/alerts/RuleFormModal.tsx` | Modal dialog for create/edit alert rule |
| `web/src/components/alerts/AlertFilters.tsx` | Filter bar for alerts page (severity, state, instance) |
| `web/src/components/alerts/AlertRow.tsx` | Single alert row with severity badge, duration, click-to-navigate |
| `web/src/components/alerts/RuleRow.tsx` | Single rule row with enable toggle, system badge, action buttons |
| `web/src/components/alerts/DeleteConfirmModal.tsx` | Simple confirmation dialog for rule deletion |
| `web/src/hooks/useAlerts.ts` | TanStack Query hook for `GET /api/v1/alerts` |
| `web/src/hooks/useAlertRules.ts` | TanStack Query hooks for rules CRUD |

### Modified Files

| File | Change |
|------|--------|
| `web/src/types/models.ts` | Add `Alert`, `AlertRule`, `AlertChannel` types (if not already present) |
| `web/src/components/server/InstanceAlerts.tsx` | Add "View All" link to AlertsPage with instance filter |

### No Changes

Everything in `internal/`, `cmd/`, `migrations/` — backend is untouched.

---

## 2. Type Definitions

Add to `web/src/types/models.ts` (verify these match the Go structs from M4):

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
  fired_at: string;          // ISO 8601
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
  duration: number;          // seconds
  cooldown: number;          // seconds
  labels: Record<string, string>;
  channels: string[];        // channel IDs
  enabled: boolean;
  description: string;
  is_system: boolean;        // true for default rules, false for user-created
}

export type AlertSeverityFilter = 'all' | 'warning' | 'critical';
export type AlertStateFilter = 'all' | 'firing' | 'resolved';
```

> **Note:** If the backend doesn't return `is_system`, the frontend can infer it from a known list of default rule IDs, or we flag this for a minor backend addition in a future iteration.

---

## 3. TanStack Query Hooks

### useAlerts.ts

```typescript
import { useQuery } from '@tanstack/react-query';
import { apiFetch } from '../lib/apiClient';
import type { Alert, AlertSeverityFilter, AlertStateFilter } from '../types/models';

interface UseAlertsOptions {
  severity?: AlertSeverityFilter;
  state?: AlertStateFilter;
  instanceId?: string;
}

export function useAlerts(options: UseAlertsOptions = {}) {
  const params = new URLSearchParams();
  if (options.severity && options.severity !== 'all') params.set('severity', options.severity);
  if (options.state && options.state !== 'all') params.set('state', options.state);
  if (options.instanceId) params.set('instance_id', options.instanceId);

  const queryString = params.toString();
  const url = `/api/v1/alerts${queryString ? `?${queryString}` : ''}`;

  return useQuery<Alert[]>({
    queryKey: ['alerts', options],
    queryFn: () => apiFetch(url),
    refetchInterval: 30_000,
  });
}
```

### useAlertRules.ts

```typescript
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { apiFetch } from '../lib/apiClient';
import type { AlertRule } from '../types/models';

export function useAlertRules() {
  return useQuery<AlertRule[]>({
    queryKey: ['alertRules'],
    queryFn: () => apiFetch('/api/v1/alerts/rules'),
    refetchInterval: 60_000,
  });
}

export function useSaveAlertRule() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (rule: Partial<AlertRule>) =>
      apiFetch('/api/v1/alerts/rules', {
        method: 'POST',
        body: JSON.stringify(rule),
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['alertRules'] });
    },
  });
}

export function useDeleteAlertRule() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (ruleId: string) =>
      apiFetch(`/api/v1/alerts/rules/${ruleId}`, { method: 'DELETE' }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['alertRules'] });
    },
  });
}

export function useTestNotification() {
  return useMutation({
    mutationFn: (channels: string[]) =>
      apiFetch('/api/v1/alerts/test', {
        method: 'POST',
        body: JSON.stringify({ channels }),
      }),
  });
}
```

> **API compatibility note:** If `DELETE /api/v1/alerts/rules/:id` doesn't exist yet, the delete mutation should use `POST /api/v1/alerts/rules` with an `enabled: false` + soft-delete flag, or we surface this as a backend gap for a quick addition. Check the actual endpoint list in the handoff — the current API has `POST /api/v1/alerts/rules` for create/update. Deletion may need the `POST` with a `deleted: true` field, or may be unsupported. Adapt accordingly.

---

## 4. Component Design

### AlertsPage.tsx

Layout:
```
┌─────────────────────────────────────────────────┐
│  Active Alerts                          [count] │
├─────────────────────────────────────────────────┤
│  [All | Warning | Critical]  [Firing | Resolved | All]  [Instance ▼]  │
├─────────────────────────────────────────────────┤
│  Sev │ Rule Name      │ Instance │ Metric │ Value │ Threshold │ State   │ Fired      │ Duration │
│  🔴  │ High Wraparound│ prod-01  │ wrap%  │ 52.3  │ 50        │ Firing  │ 10:32 AM   │ 2h 15m   │
│  🟡  │ Cache Hit Low  │ prod-02  │ cache% │ 87.4  │ 90        │ Firing  │ 11:00 AM   │ 1h 47m   │
│  🟢  │ Long Txn       │ prod-01  │ txn_s  │ 12.0  │ 60        │ Resolved│ 09:15 AM   │ 45m      │
└─────────────────────────────────────────────────┘

Empty state (no alerts):
┌─────────────────────────────────────────────────┐
│            ✓ All clear                          │
│      No active alerts across all instances       │
└─────────────────────────────────────────────────┘
```

State management:
- Filters stored in component state (useState), not Zustand — page-local, no persistence needed
- URL search params optional enhancement: `?severity=critical&state=firing&instance=prod-01` for linkability from InstanceAlerts "View All"

Duration column:
- For firing alerts: compute `Date.now() - fired_at`, update every 30s with refetch
- For resolved alerts: compute `resolved_at - fired_at`, static

Row click handler:
- `navigate(`/servers/${alert.instance_id}`)`

### AlertRulesPage.tsx

Layout:
```
┌────────────────────────────────────────────────────────────────────┐
│  Alert Rules                                    [+ Create Rule]   │
├────────────────────────────────────────────────────────────────────┤
│  Name             │ Metric    │ Condition  │ Warn  │ Crit │ Cooldown │ Channels    │ On/Off │ Actions │
│  ⚙ Wraparound    │ wrap%     │ >          │ 20%   │ 50%  │ 10m      │ email,slack │ [●]    │ [✎]    │
│  ⚙ Cache Hit     │ cache%    │ <          │ 90%   │ —    │ 5m       │ email       │ [●]    │ [✎]    │
│    My Custom Rule │ disk_free │ <          │ 20GB  │ 10GB │ 30m      │ slack       │ [○]    │ [✎][🗑]│
└────────────────────────────────────────────────────────────────────┘

⚙ = system badge (default rule — editable, not deletable)
[●] = enabled toggle  [○] = disabled toggle
[✎] = edit button  [🗑] = delete button (user-created only)
```

Permission gate:
```tsx
// In router or page wrapper
<ProtectedRoute>
  <PermissionGate permission="alert_management" fallback={<Navigate to="/" />}>
    <AlertRulesPage />
  </PermissionGate>
</ProtectedRoute>
```

### RuleFormModal.tsx

Two modes: **create** (empty form) and **edit** (pre-populated).

```
┌──────────────────────────────────────────┐
│  Create Alert Rule              [✕]      │
├──────────────────────────────────────────┤
│                                          │
│  Name:        [________________________] │
│  Description: [________________________] │
│               [________________________] │
│                                          │
│  Metric:      [________ ▼]              │
│  Condition:   [> ▼]                      │
│                                          │
│  Warning Threshold:  [________]          │
│  Critical Threshold: [________]          │
│                                          │
│  Duration:    [___] [minutes ▼]          │
│  Cooldown:    [___] [minutes ▼]          │
│                                          │
│  Channels:    ☑ email  ☐ slack  ☐ webhook│
│                                          │
│  ☑ Enabled                               │
│                                          │
│  [Test Notification]                     │
│                                          │
│  ┌─ Error ──────────────────────────┐    │
│  │ (API errors displayed here)      │    │
│  └──────────────────────────────────┘    │
│                                          │
│           [Cancel]     [Save Rule]       │
└──────────────────────────────────────────┘
```

Props:
```typescript
interface RuleFormModalProps {
  open: boolean;
  onClose: () => void;
  rule?: AlertRule;          // undefined = create mode, defined = edit mode
  availableChannels: string[]; // from config or a future API
}
```

Form state: use `useState` with a `formData` object mirroring `AlertRule` fields. On submit, call `useSaveAlertRule().mutate(formData)`.

Duration/cooldown conversion: the form shows user-friendly units (minutes, hours), but the API expects seconds. Convert on submit: `durationSeconds = value * multiplier`.

Validation:
- Name required, non-empty
- At least one threshold (warning or critical) must be set
- Duration > 0
- Cooldown >= 0

System rule handling:
- When `rule.is_system === true`: name field is read-only, delete button hidden
- All other fields remain editable (thresholds, cooldown, channels, enabled)

### AlertFilters.tsx

Compact filter bar using toggle buttons (styled like the existing time range presets):

```typescript
interface AlertFiltersProps {
  severity: AlertSeverityFilter;
  onSeverityChange: (s: AlertSeverityFilter) => void;
  state: AlertStateFilter;
  onStateChange: (s: AlertStateFilter) => void;
  instanceId: string;             // '' = all
  onInstanceChange: (id: string) => void;
  instances: Array<{ id: string; name: string }>;
}
```

### DeleteConfirmModal.tsx

Minimal confirmation dialog:
```
┌──────────────────────────────────────┐
│  Delete Rule                  [✕]    │
├──────────────────────────────────────┤
│                                      │
│  Are you sure you want to delete     │
│  "My Custom Rule"?                   │
│                                      │
│  This action cannot be undone.       │
│                                      │
│        [Cancel]    [Delete]          │
└──────────────────────────────────────┘
```

---

## 5. Available Channels Discovery

The M4 alert engine reads notification channel config from YAML:

```yaml
alerts:
  channels:
    - id: email-dba
      type: email
      config:
        to: dba-team@company.com
    - id: slack-ops
      type: slack
      config:
        webhook_url: https://hooks.slack.com/...
```

**Option A (simplest, recommended):** Add a `GET /api/v1/alerts/channels` endpoint that returns channel IDs and types (no secrets). This is a minor backend addition — a single handler reading from config.

**Option B (no backend change):** Hardcode channel names in the frontend config or extract them from existing rules (union of all `channels` arrays from the rules response). Less clean but zero backend work.

**Recommendation:** Go with Option B for M5_05 (extract from rules response). If the rules list is empty or channels are missing, show a "No channels configured" message. Flag Option A for a future iteration.

```typescript
// Extract available channels from rules
function extractChannels(rules: AlertRule[]): string[] {
  const channelSet = new Set<string>();
  for (const rule of rules) {
    for (const ch of rule.channels) {
      channelSet.add(ch);
    }
  }
  return Array.from(channelSet).sort();
}
```

---

## 6. Styling Patterns

Reuse existing component patterns:

| Need | Existing Component | Usage |
|------|--------------------|-------|
| Severity indicator | `AlertBadge` or `StatusBadge` | Warning = amber, Critical = red, Resolved = green |
| Data table | `DataTable` (if generic) or inline `<table>` with Tailwind | Same dark-mode table styling as StatementsSection |
| Cards/containers | `Card` | Wrap page sections |
| Loading states | `Spinner` | While fetching alerts/rules |
| Toggle switch | Tailwind toggle (custom) | Enable/disable rule |
| Modal overlay | New pattern — use Tailwind `fixed inset-0 bg-black/50` with centered card | Consistent with dark theme |
| Form inputs | Tailwind `bg-gray-800 border-gray-700 text-gray-100` | Match existing dark input styling |
| Filter toggles | Match `TimeRangeSelector` preset button pattern | Consistent filter UX |

Modal pattern (reusable):
```tsx
function Modal({ open, onClose, title, children }: ModalProps) {
  if (!open) return null;
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50" onClick={onClose}>
      <div className="bg-gray-900 border border-gray-700 rounded-lg shadow-xl w-full max-w-lg mx-4 max-h-[90vh] overflow-y-auto"
           onClick={e => e.stopPropagation()}>
        <div className="flex items-center justify-between px-6 py-4 border-b border-gray-700">
          <h2 className="text-lg font-semibold text-gray-100">{title}</h2>
          <button onClick={onClose} className="text-gray-400 hover:text-gray-200">✕</button>
        </div>
        <div className="px-6 py-4">{children}</div>
      </div>
    </div>
  );
}
```

---

## 7. Navigation & Integration

### Sidebar (already configured)
- "Alerts" → `/alerts` → AlertsPage
- "Alert Rules" → `/alerts/rules` → AlertRulesPage (permission-gated)

### InstanceAlerts → AlertsPage link
In `InstanceAlerts.tsx`, add a "View All" link:
```tsx
<Link to={`/alerts?instance_id=${instanceId}`} className="text-blue-400 hover:text-blue-300 text-sm">
  View all alerts →
</Link>
```

AlertsPage reads `instance_id` from URL search params to set initial filter:
```typescript
const [searchParams] = useSearchParams();
const initialInstanceId = searchParams.get('instance_id') || '';
```

---

## 8. Error Handling

| Scenario | Handling |
|----------|----------|
| API fetch fails (network) | TanStack Query shows stale data + error indicator |
| Rule save fails (400) | Display error message inline in modal |
| Rule save fails (403) | Display "Insufficient permissions" — shouldn't happen due to PermissionGate but handle defensively |
| Test notification fails | Display failure message with reason in modal |
| No alerts data | Show "All clear" empty state |
| No rules data | Show empty table with "No alert rules configured" message |

---

## 9. Implementation Order

1. **Types** — Add `Alert`, `AlertRule` types to `models.ts`
2. **Hooks** — `useAlerts.ts`, `useAlertRules.ts`
3. **AlertsPage** — Replace placeholder with full implementation (AlertFilters, AlertRow)
4. **AlertRulesPage** — Replace placeholder with rule list (RuleRow, enable toggle)
5. **RuleFormModal** — Create/edit form with validation
6. **DeleteConfirmModal** — Simple confirmation dialog
7. **Integration** — InstanceAlerts "View All" link, URL param handling
8. **Polish** — Empty states, loading states, error states, responsive layout

---

## 10. Testing Expectations

Since this is frontend-only, testing is via:
- `tsc --noEmit` — type safety
- `eslint src/` — lint rules
- `vite build` — successful bundle
- Manual verification of all user flows

No new Go tests needed. Existing backend tests remain green.
