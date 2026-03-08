# PGPulse — Iteration Handoff: M5_03 → M5_04

**Date:** 2026-03-03
**From:** M5_03 (Live Data Integration) + Housekeeping
**To:** M5_04 (Statements, Lock Tree, Progress Monitoring)
**Latest commits:** 7d97fc4 (M5_03 code), 3c739dc (M5_03 docs), 12933e8 (housekeeping: ConnProvider wired + lint fix), plus strategy/save-point doc corrections

---

## DO NOT RE-DISCUSS

These decisions are final. Do not revisit:

1. **React + TypeScript + Vite + Tailwind CSS + Apache ECharts** — frontend stack (D89, M5_01)
2. **4-role RBAC** — super_admin, roles_admin, dba, app_admin with permission groups (D90, M5_02)
3. **Dual-token JWT** — 15min access (memory) + 7d refresh (localStorage) (D91, M5_02)
4. **Polling via TanStack Query** — SSE deferred to future iteration (D95, M5_03)
5. **Server Detail: 8 sections** — header, key metrics, connections, cache hit, replication, wait events, long transactions, alerts (D96, M5_03)
6. **Time range: presets + custom** — HTML5 datetime-local, no calendar library (D97, M5_03)
7. **Hybrid API** — /metrics/current for snapshots, /metrics/history for time-series (D98, M5_03)
8. **InstanceConnProvider** — orchestrator implements ConnFor(), wired in main.go (housekeeping, 12933e8)
9. **SSoT for instance role** — orchestrator queries pg_is_in_recovery() once, passes via InstanceContext

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
| GET | /api/v1/instances/:id/replication | **Live query via ConnProvider** |
| GET | /api/v1/instances/:id/activity/wait-events | **Live query via ConnProvider** |
| GET | /api/v1/instances/:id/activity/long-transactions | **Live query via ConnProvider** |
| GET | /api/v1/instances/:id/alerts | Instance-filtered alerts |
| GET | /api/v1/alerts | All active alerts |
| GET | /api/v1/alerts/rules | Alert rules |
| POST | /api/v1/alerts/rules | Create/update rule |
| GET | /api/v1/health | Health check |

### Frontend Architecture

```
web/src/
  pages/
    FleetOverviewPage.tsx           — real instance cards, 30s auto-refresh
    ServerDetailPage.tsx            — 8 sections, 10s refresh, time range selector
    LoginPage.tsx                   — JWT auth flow
    UsersPage.tsx                   — user management (permission-gated)
    AlertsPage.tsx                  — placeholder
    AlertRulesPage.tsx              — placeholder (permission-gated)
  components/
    shared/                         — MetricCard, StatusBadge, DataTable, Card, Spinner, AlertBadge, TimeRangeSelector
    charts/                         — TimeSeriesChart, ConnectionGauge, WaitEventsChart, EChartWrapper
    fleet/                          — InstanceCard
    server/                         — HeaderCard, KeyMetricsRow, ReplicationSection, WaitEventsSection, LongTransactionsTable, InstanceAlerts
    auth/                           — ProtectedRoute, PermissionGate
    layout/                         — Sidebar, Navbar
  stores/                           — authStore (Zustand), timeRangeStore (Zustand)
  hooks/                            — useInstances, useCurrentMetrics, useMetricsHistory, useReplication, useWaitEvents, useLongTransactions, useInstanceAlerts
  lib/                              — apiClient, formatters, echartsTheme
  types/                            — models.ts (all API response types)
```

### Key Frontend Patterns

```typescript
// TanStack Query hooks pattern
export function useCurrentMetrics(instanceId: string) {
  return useQuery({
    queryKey: ['metrics', 'current', instanceId],
    queryFn: () => apiFetch(`/api/v1/instances/${instanceId}/metrics/current`),
    refetchInterval: 10_000,
  });
}

// Time range integration
export function useMetricsHistory(instanceId: string, metrics: string[], options?: { step?: string }) {
  const { range } = useTimeRangeStore();
  const { from, to } = computeRange(range);
  return useQuery({
    queryKey: ['metrics', 'history', instanceId, metrics, from.toISOString(), to.toISOString()],
    queryFn: () => apiFetch(`/api/v1/instances/${instanceId}/metrics/history?...`),
    refetchInterval: range.preset !== 'custom' ? 10_000 : false,
  });
}
```

### Key Backend Patterns

```go
// InstanceConnProvider — live queries to monitored instances
type InstanceConnProvider interface {
    ConnFor(ctx context.Context, instanceID string) (*pgx.Conn, error)
}
// Orchestrator implements this. Opens fresh pgx.Conn using instance DSN.
// Handler borrows conn → queries → releases → writes response.
// statement_timeout = 5s on borrowed connections.

// Collector interface
type Collector interface {
    Name() string
    Collect(ctx context.Context, conn *pgx.Conn) ([]MetricPoint, error)
    Interval() time.Duration
}
```

### Collectors Relevant to M5_04

These collectors already exist and produce the data M5_04 needs to display:

**pg_stat_statements (M1_04):**
- `internal/collector/statements_config.go` — pgss settings, fill %, reset age
- `internal/collector/statements_top.go` — top queries by IO time and CPU time

Metrics emitted:
- `pgpulse.statements.top.io_time_ms` with labels `{queryid, query_text_truncated, dbname, username}`
- `pgpulse.statements.top.cpu_time_ms` with same labels
- `pgpulse.statements.top.calls`, `.rows`, `.mean_time_ms`
- `pgpulse.statements.fill_pct`, `.count`, `.max`, `.stats_reset_age_seconds`

**Locks (M1_05):**
- `internal/collector/locks_wait_events.go` — wait event distribution
- `internal/collector/locks_tree.go` — summary metrics: blocking_pids, blocked_pids, max_depth, total_blocked
- `internal/collector/locks_long_transactions.go` — long transaction counts

Full lock tree (per-PID blocking chain with query text) is NOT in the collector — it was deferred to the API layer (decision from M1_05: collectors emit summary metrics, API runs the recursive CTE directly for the tree view). The recursive CTE SQL exists in the design docs but has not been implemented as an API endpoint yet.

**Progress monitoring (M1_03):**
- `internal/collector/progress.go` — 6 progress views: VACUUM, CLUSTER/VACUUM FULL, CREATE INDEX, ANALYZE, BASEBACKUP, COPY

Metrics emitted per active operation:
- `pgpulse.progress.vacuum.phase`, `.heap_blks_scanned`, `.heap_blks_total`, etc.
- Similar for each progress type
- Returns empty slice when no operations are running (transient data)

---

## What Was Just Completed

### M5_03 — Live Data Integration
- Fleet Overview: real instance cards with metrics + alerts, 30s auto-refresh
- Server Detail: 8 sections with real data, 10s refresh, time range selector
- 6 new API endpoints, 36 files, ~2,530 lines
- Commits: 7d97fc4, 3c739dc

### Housekeeping (post-M5_03)
- **ConnProvider wired**: orchestrator.ConnFor() implemented, main.go calls SetConnProvider(). Replication, wait-events, long-transactions endpoints now return real data. Commit 12933e8.
- **static.go errcheck fixed**: `_ = f.Close()`. golangci-lint now reports 0 issues.
- **Strategy doc + save point template updated**: all Svelte references → React + TypeScript + ECharts. Re-uploaded to Project Knowledge.
- Roadmap and CHANGELOG still need M5_03 entry (text prepared but not yet committed — include in M5_04 commit or commit separately).

---

## Known Issues

| Issue | Impact | Notes |
|-------|--------|-------|
| ECharts chunk 347KB gzipped | Performance | Deferred optimization |
| Lock tree recursive CTE not in API | No lock tree visualization | Needs new endpoint for M5_04 |
| Progress data is transient | Progress sections may often be empty | Normal — only shows during active operations |
| Alerts page is placeholder | No alert management from UI yet | Low priority for M5 |

---

## Next Task: M5_04

### Scope

Three features deferred from M5_03 as "Tier 3":

**1. pg_stat_statements Top Queries View**
- Most-requested DBA feature
- Complex sortable table: query text, total time, calls, mean time, IO time, CPU time, rows
- Sorting by IO time, CPU time, calls, rows
- Query text with expand/collapse (queries are long)
- pgss config section: fill %, track setting, IO timing status, reset age
- Data source: existing statements_top collector metrics in storage + possibly a new live-query endpoint for richer data

**2. Lock Tree Visualization**
- Recursive blocking chain: who blocks whom
- Tree rendering (indented or actual tree diagram)
- Shows: PID, user, database, lock mode, query text, duration, blocked count
- Root blockers highlighted
- Data source: NEW API endpoint needed — runs recursive CTE via ConnProvider (design from M1_05 decision: option (a) — API-layer query, not collector)

Lock tree recursive CTE (from PGAM Q55, adapted):
```sql
WITH RECURSIVE lock_tree AS (
    SELECT
        pid,
        pg_blocking_pids(pid) AS blocking_pids,
        0 AS depth
    FROM pg_stat_activity
    WHERE cardinality(pg_blocking_pids(pid)) > 0
    
    UNION ALL
    
    SELECT
        sa.pid,
        pg_blocking_pids(sa.pid),
        lt.depth + 1
    FROM pg_stat_activity sa
    JOIN lock_tree lt ON sa.pid = ANY(lt.blocking_pids)
)
SELECT lt.pid, lt.depth,
       sa.usename, sa.datname, sa.state,
       sa.wait_event_type, sa.wait_event,
       EXTRACT(EPOCH FROM (now() - sa.xact_start)) AS duration_seconds,
       LEFT(sa.query, 200) AS query,
       cardinality(pg_blocking_pids(lt.pid)) AS blocked_by_count
FROM lock_tree lt
JOIN pg_stat_activity sa ON sa.pid = lt.pid
ORDER BY lt.depth, lt.pid
```

Note: PGPulse M1_05 used pg_blocking_pids() + Go BFS graph instead of recursive CTE. For the API endpoint, either approach works — the Go BFS is already in the collector code and could be reused.

**3. Progress Monitoring**
- VACUUM, CREATE INDEX, ANALYZE progress bars
- Transient data — only shows during active operations
- Data source: existing progress collector metrics in storage + possibly live-query for real-time progress percentage
- May need a new API endpoint: GET /api/v1/instances/:id/activity/progress

### Design Questions for M5_04

1. **Statements data source**: Use stored metrics from statements_top collector, or add a new live-query endpoint that hits pg_stat_statements directly for richer data (more columns, real-time)?

2. **Lock tree rendering**: Indented table rows (simple, CSS-only) vs actual tree/graph visualization (D3 or custom SVG)? The indented table is what PGAM did and is sufficient for most DBA workflows.

3. **Progress monitoring**: Show progress bars inline on Server Detail page, or as a separate section/tab? Progress data is transient — the section will often be empty.

4. **New API endpoints needed**:
   - GET /api/v1/instances/:id/statements — top queries (live or stored?)
   - GET /api/v1/instances/:id/activity/locks — full lock tree (live via ConnProvider)
   - GET /api/v1/instances/:id/activity/progress — active operations (live via ConnProvider)

5. **Scope split**: All three features in M5_04, or split into M5_04 (statements) + M5_05 (locks + progress)?

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
- `go test ./...` — pass
- `golangci-lint run` — 0 issues
- `tsc --noEmit` — pass
- `eslint src/` — pass
- `vite build` — pass

---

## Workflow Reminder

1. Claude.ai: discuss design questions → produce requirements.md, design.md, team-prompt.md
2. Copy docs to `docs/iterations/M5_04_YYYYMMDD_statements-locks-progress/`
3. Update CLAUDE.md current iteration section
4. Paste team-prompt into Claude Code
5. Verify: go build → go vet → golangci-lint → go test → tsc → eslint → vite build
6. Claude.ai: produce session-log.md
7. Update roadmap.md + CHANGELOG.md
8. Commit and push
