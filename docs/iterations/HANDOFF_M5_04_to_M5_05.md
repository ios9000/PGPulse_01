# PGPulse — Iteration Handoff: M5_04 → M5_05

**Date:** 2026-03-03
**From:** M5_04 (Statements, Lock Tree, Progress Monitoring)
**To:** M5_05 (Alert Management UI)
**Latest commit:** b30873a (M5_04 code + tests)

---

## DO NOT RE-DISCUSS

These decisions are final. Do not revisit:

1. **React + TypeScript + Vite + Tailwind CSS + Apache ECharts** — frontend stack (D89, M5_01)
2. **4-role RBAC** — super_admin, roles_admin, dba, app_admin with permission groups (D90, M5_02)
3. **Dual-token JWT** — 15min access (memory) + 7d refresh (localStorage) (D91, M5_02)
4. **Polling via TanStack Query** — SSE deferred to future iteration (D95, M5_03)
5. **Server Detail: 8+ sections** — header, progress (conditional), key metrics, connections, cache hit, replication, wait events, statements, lock tree, long transactions, alerts (D96+D99, M5_03/M5_04)
6. **Time range: presets + custom** — HTML5 datetime-local, no calendar library (D97, M5_03)
7. **Hybrid API** — /metrics/current for snapshots, /metrics/history for time-series (D98, M5_03)
8. **InstanceConnProvider** — orchestrator implements ConnFor(), wired in main.go (housekeeping)
9. **SSoT for instance role** — orchestrator queries pg_is_in_recovery() once, passes via InstanceContext
10. **Statements: live query + historical expandable row** — sort whitelist, version-gated SQL (D100, M5_04)
11. **Lock tree: indented table with depth markers** — BFS in Go, root blockers highlighted (D101, M5_04)
12. **Progress: conditional section** — collapses when no active operations (D102, M5_04)

---

## What Exists Now

### Backend Architecture

```
cmd/pgpulse-server/main.go          — wires orchestrator, API, auth, storage, alerts, ConnProvider
internal/
  orchestrator/orchestrator.go       — runs collectors, implements ConnFor() for API live queries
  collector/                         — 20+ collector files, 33/76 PGAM queries ported
  storage/                           — MetricStore (TimescaleDB), queries.go (CurrentMetrics, HistoryMetrics)
  api/                               — chi router, JWT middleware, all endpoints below
  auth/                              — JWT, bcrypt, RBAC (4 roles, 5 permission groups)
  alert/                             — rule engine, state machine, email notifier
  config/                            — koanf YAML + env vars
  version/                           — PG version detection + SQL gate pattern
```

### REST API (all working)

| Method | Path | Notes |
|--------|------|-------|
| POST | /api/v1/auth/login | JWT token pair |
| POST | /api/v1/auth/refresh | New access token |
| POST | /api/v1/auth/register | user_management perm |
| GET | /api/v1/auth/me | Current user |
| PUT | /api/v1/auth/me/password | Change own password |
| GET | /api/v1/auth/users | user_management perm |
| PUT | /api/v1/auth/users/:id | Update user |
| GET | /api/v1/instances | ?include=metrics,alerts |
| GET | /api/v1/instances/:id | Instance detail |
| POST | /api/v1/instances | instance_management perm |
| GET | /api/v1/instances/:id/metrics/current | Latest snapshot |
| GET | /api/v1/instances/:id/metrics/history | Time-series (step: 1m/5m/15m/1h/1d) |
| GET | /api/v1/instances/:id/replication | Live query via ConnProvider |
| GET | /api/v1/instances/:id/activity/wait-events | Live query via ConnProvider |
| GET | /api/v1/instances/:id/activity/long-transactions | Live query via ConnProvider |
| GET | /api/v1/instances/:id/activity/statements | **NEW M5_04** — live top queries, sort/limit params |
| GET | /api/v1/instances/:id/activity/locks | **NEW M5_04** — blocking tree via BFS |
| GET | /api/v1/instances/:id/activity/progress | **NEW M5_04** — 6 version-gated progress views |
| GET | /api/v1/instances/:id/alerts | Instance-filtered alerts |
| GET | /api/v1/alerts | All active alerts |
| GET | /api/v1/alerts/rules | Alert rules |
| POST | /api/v1/alerts/rules | Create/update rule |
| GET | /api/v1/health | Health check |

### Alert Backend (M4 — already complete)

The alerting engine is fully built. M5_05 needs to build the UI on top of these existing pieces:

**Alert Rule Engine** (`internal/alert/`):
- `evaluator.go` — threshold comparison (>, <, >=, <=, ==, !=), state machine (OK → WARNING → CRITICAL → OK), hysteresis (N consecutive violations before firing), cooldown window
- `rules.go` — 20 default alert rules (14 from PGAM + 6 new), stored in config/DB
- `dispatcher.go` — routes alerts to notification channels, retry with exponential backoff
- `notifier/email.go` — SMTP email sender (implemented and working)

**Alert API Endpoints** (already registered, need frontend):
- `GET /api/v1/alerts` — list all active/firing alerts, supports filters
- `GET /api/v1/alerts/rules` — list all configured alert rules
- `POST /api/v1/alerts/rules` — create or update an alert rule
- `POST /api/v1/alerts/test` — send a test notification to verify channel config

**Alert Rule Structure** (from M4 design):
```go
type AlertRule struct {
    ID              string            `json:"id"`
    Name            string            `json:"name"`
    Metric          string            `json:"metric"`
    Condition       string            `json:"condition"`       // ">", "<", ">=", "<=", "==", "!="
    WarningThreshold  *float64        `json:"warning_threshold"`
    CriticalThreshold *float64        `json:"critical_threshold"`
    Duration        time.Duration     `json:"duration"`        // how long condition must hold
    Cooldown        time.Duration     `json:"cooldown"`        // min time between alerts
    Labels          map[string]string `json:"labels"`          // label filters
    Channels        []string          `json:"channels"`        // notification channel IDs
    Enabled         bool              `json:"enabled"`
    Description     string            `json:"description"`
}
```

**Active Alert Structure:**
```go
type Alert struct {
    ID           string            `json:"id"`
    RuleID       string            `json:"rule_id"`
    RuleName     string            `json:"rule_name"`
    InstanceID   string            `json:"instance_id"`
    Severity     string            `json:"severity"`     // "warning", "critical"
    Metric       string            `json:"metric"`
    Value        float64           `json:"value"`
    Threshold    float64           `json:"threshold"`
    Labels       map[string]string `json:"labels"`
    State        string            `json:"state"`        // "firing", "resolved"
    FiredAt      time.Time         `json:"fired_at"`
    ResolvedAt   *time.Time        `json:"resolved_at"`
    Message      string            `json:"message"`
}
```

### Frontend Architecture

```
web/src/
  pages/
    FleetOverviewPage.tsx           — real instance cards, 30s auto-refresh
    ServerDetailPage.tsx            — 11 sections (header, progress, metrics, connections, cache, replication, wait events, statements, lock tree, long txns, alerts)
    LoginPage.tsx                   — JWT auth flow
    UsersPage.tsx                   — user management (permission-gated)
    AlertsPage.tsx                  — *** PLACEHOLDER — M5_05 target ***
    AlertRulesPage.tsx              — *** PLACEHOLDER — M5_05 target ***
  components/
    shared/                         — MetricCard, StatusBadge, DataTable, Card, Spinner, AlertBadge, TimeRangeSelector
    charts/                         — TimeSeriesChart, ConnectionGauge, WaitEventsChart, EChartWrapper
    fleet/                          — InstanceCard
    server/                         — HeaderCard, KeyMetricsRow, ReplicationSection, WaitEventsSection, LongTransactionsTable, InstanceAlerts, StatementsSection, StatementsConfigBar, StatementRow, LockTreeSection, LockTreeRow, ProgressSection, ProgressCard
    auth/                           — ProtectedRoute, PermissionGate
    layout/                         — Sidebar, Navbar
  stores/                           — authStore (Zustand), timeRangeStore (Zustand)
  hooks/                            — useInstances, useCurrentMetrics, useMetricsHistory, useReplication, useWaitEvents, useLongTransactions, useInstanceAlerts, useStatements, useLockTree, useProgress
  lib/                              — apiClient, formatters, echartsTheme
  types/                            — models.ts (all API response types)
```

### Key Frontend Patterns

```typescript
// TanStack Query hooks pattern (all hooks follow this)
export function useStatements(instanceId: string, sort: StatementSortField = 'total_time', limit = 25) {
  return useQuery<StatementsResponse>({
    queryKey: ['statements', instanceId, sort, limit],
    queryFn: () => apiFetch(`/api/v1/instances/${instanceId}/activity/statements?sort=${sort}&limit=${limit}`),
    refetchInterval: 10_000,
  });
}

// Permission gating pattern (used in UsersPage, AlertRulesPage will need it)
<PermissionGate permission="alert_management">
  <AlertRulesPage />
</PermissionGate>
```

---

## What Was Just Completed

### M5_04 — Statements, Lock Tree, Progress Monitoring
- 3 new ConnProvider live-query endpoints (statements, locks, progress)
- StatementsSection: sortable table with expandable row drill-down (historical ECharts)
- LockTreeSection: indented table with BFS-built blocking tree, root blocker highlighting
- ProgressSection: conditional rendering with color-coded operation cards and progress bars
- 20 tests covering sort validation, tree building, version gating
- 19 files changed, 2,181 insertions
- Commit: b30873a

---

## Known Issues

| Issue | Impact | Notes |
|-------|--------|-------|
| ECharts chunk 347KB gzipped | Performance | Deferred optimization |
| Historical drill-down may show sparse data | UX | Only queries consistently in top-N have full history |
| AlertsPage.tsx is placeholder | No alert management from UI | M5_05 target |
| AlertRulesPage.tsx is placeholder | No rule CRUD from UI | M5_05 target |
| Roadmap + CHANGELOG need M5_04 entry | Docs | Include in M5_05 commit |

---

## Next Task: M5_05

### Scope: Alert Management UI

Build the frontend for the alert system. The backend is already complete (M4). Two placeholder pages need to be replaced with real implementations.

**1. Active Alerts Page (AlertsPage.tsx)**
- List of all currently firing and recently resolved alerts
- Filter by: severity (warning/critical), state (firing/resolved), instance
- Each alert shows: severity badge, rule name, instance, metric, current value vs threshold, fired time, duration
- Auto-refresh (10s or 30s)
- Clicking an alert could navigate to the relevant instance's Server Detail page
- Data source: `GET /api/v1/alerts` (existing endpoint)

**2. Alert Rules Page (AlertRulesPage.tsx)**
- List all configured alert rules with enable/disable toggle
- Each rule shows: name, metric, condition, warning/critical thresholds, cooldown, channels, enabled state
- Create new rule: form/modal with fields matching AlertRule structure
- Edit existing rule: same form, pre-populated
- Delete rule (with confirmation)
- Permission-gated: requires `alert_management` permission (super_admin and dba roles)
- Data source: `GET /api/v1/alerts/rules` and `POST /api/v1/alerts/rules` (existing endpoints)

**3. Test Notification**
- Button on the rules page or within a rule form to send a test notification
- Calls `POST /api/v1/alerts/test`
- Shows success/failure feedback

**4. Integration with Existing UI**
- Sidebar already has "Alerts" and "Alert Rules" navigation items pointing to the placeholder pages
- InstanceAlerts section on Server Detail already shows per-instance alerts — verify it links to the full Alerts page
- AlertBadge component already exists in shared/ — reuse for severity indicators

### Design Questions for M5_05

1. **Alert rule form**: Modal dialog or inline form on the rules page? Modal keeps the list visible; inline is simpler.

2. **Alert history**: Show only currently firing alerts, or include recently resolved (last 24h)? The API likely supports a state filter.

3. **Notification channels**: The rule form needs a "channels" field. Are notification channels (email addresses, Slack webhooks) configured elsewhere, or does the rule form need channel configuration too? From M4, the notifier config is in YAML — the UI may just show channel names as selectable options.

4. **Default rules display**: The 20 default rules from M4 should appear pre-populated. Should they be editable/deletable, or locked as system defaults?

5. **Bulk actions**: Enable/disable multiple rules at once, or one at a time?

---

## Environment

| Tool | Version | Notes |
|------|---------|-------|
| Go | 1.24.0 | |
| Node.js | 22.14.0 | |
| Claude Code | 2.1.63 | Bash works on Windows |
| golangci-lint | v2.10.1 | 0 issues currently |
| Git | 2.52.0 | |

### Build Status

All checks passing:
- `go build ./cmd/... ./internal/...` — pass
- `go vet ./...` — pass
- `go test ./...` — pass (20 new tests from M5_04)
- `golangci-lint run` — 0 issues
- `tsc --noEmit` — pass
- `eslint src/` — pass
- `vite build` — pass

---

## Workflow Reminder

1. Claude.ai: discuss M5_05 design questions → produce requirements.md, design.md, team-prompt.md
2. Copy docs to `docs/iterations/M5_05_YYYYMMDD_alert-management-ui/`
3. Update CLAUDE.md current iteration section
4. Paste team-prompt into Claude Code
5. Verify: go build → go vet → golangci-lint → go test → tsc → eslint → vite build
6. Claude.ai: produce session-log.md
7. Update roadmap.md + CHANGELOG.md (include M5_04 entry too)
8. Commit and push
