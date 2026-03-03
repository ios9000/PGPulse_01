# M5_03 Team Prompt — Live Data Integration

Wire real backend data into PGPulse frontend. Build Fleet Overview with live instance cards and Server Detail with charts, replication, wait events, long transactions, alerts.

Read CLAUDE.md for project context. Read docs/iterations/M5_03_03032026_live-data/design.md for specs.

**Agents cannot run shell commands on this platform.** Developer runs go build, go test, tsc, vite build, eslint manually. List all files at end.

Create a team of 3 specialists:

---

## BACKEND AGENT

Build 5 new API endpoints plus update instance list. All require auth with view_all permission.

### internal/api/connprovider.go (CREATE)
Define InstanceConnProvider interface: ConnFor(ctx, instanceID) (*pgx.Conn, error). Lets handlers query monitored instances directly.

### internal/storage/queries.go (UPDATE)
Add CurrentMetrics(ctx, instanceID): SQL DISTINCT ON (metric) ORDER BY metric, collected_at DESC. Returns map metric to value+labels.

Add HistoryMetrics(ctx, req): If step provided use time_bucket (TimescaleDB) or date_trunc fallback with AVG. No step means raw points capped 1000. Validate step: only 1m/5m/15m/1h/1d.

Add CurrentMetricValues(ctx, instanceID): simplified map for fleet enrichment.

### internal/api/metrics.go (UPDATE)
Current handler: GET /api/v1/instances/{id}/metrics/current. Calls storage.CurrentMetrics.

History handler: GET /api/v1/instances/{id}/metrics/history. Params: metric (repeatable), from, to, step. Validate from < to, step allowed, at least one metric.

### internal/api/replication.go (CREATE)
GET /api/v1/instances/{id}/replication. Use InstanceConnProvider. Check pg_is_in_recovery(). Primary: pg_stat_replication with pg_wal_lsn_diff for lag by phase, pg_replication_slots for slot health. Replica: pg_stat_wal_receiver. statement_timeout 5s.

Response types: ReplicationResponse with Role, Replicas (ReplicaInfo with LagInfo), Slots (SlotInfo), WALReceiver (WALReceiverInfo).

### internal/api/activity.go (CREATE)
WaitEvents handler: GET /instances/{id}/activity/wait-events. Query pg_stat_activity grouped by wait_event_type, wait_event, count. Plus total_backends. Exclude own PID.

LongTransactions handler: GET /instances/{id}/activity/long-transactions. Default threshold 5s, accept threshold_seconds param. Query pg_stat_activity where duration > threshold. Truncate query to 200 chars. Sort duration DESC.

### internal/api/instances.go (UPDATE)
Add ?include= query parameter. include=metrics embeds CurrentMetricValues. include=alerts embeds active alert counts. Fields omitted when not requested.

### internal/api/router.go (UPDATE)
Wire routes under /api/v1/instances/{id} with RequirePermission(PermViewAll). Inject InstanceConnProvider into replication and activity handlers.

### Tests
internal/api/metrics_test.go (UPDATE): test current, history, step validation (invalid=400)
internal/api/activity_test.go (CREATE): test wait-events, long-transactions, auth required
internal/api/replication_test.go (CREATE): test response structure, auth required

---

## FRONTEND AGENT

Build Fleet Overview and Server Detail with real data. React + TypeScript + Tailwind + ECharts.

### Stores and utilities

**web/src/stores/timeRangeStore.ts** (CREATE): Zustand store. Presets 15m/1h/6h/24h/7d, default 1h. setPreset stores key (from/to computed at query time). setCustomRange stores fixed dates with preset=custom.

**web/src/lib/echartsTheme.ts** (CREATE): Dark theme. slate-300 text, slate-600 axes, transparent bg. Palette: blue/emerald/amber/rose/violet/cyan-500. Tooltip: slate-800. Register in App.tsx via echarts.registerTheme('pgpulse-dark', theme).

**web/src/lib/formatters.ts** (CREATE): formatBytes, formatUptime (12d 4h 32m), formatDuration (5m 12s), formatPercent (99.2%), formatPGVersion (160004 to 16.4), formatTimestamp, thresholdColor (returns green/yellow/red).

### Hooks

**web/src/hooks/useInstances.ts** (UPDATE): Add include option building ?include=metrics,alerts. refetchInterval 30000.

**web/src/hooks/useMetrics.ts** (CREATE):
- useCurrentMetrics(instanceId): GET /instances/:id/metrics/current, refetch 10s
- useMetricsHistory(instanceId, metrics[], {step}): GET /instances/:id/metrics/history. Reads timeRangeStore. For presets compute from=now-duration, to=now at query time. refetch 10s for presets, disabled for custom.

**web/src/hooks/useReplication.ts** (CREATE): useReplication(instanceId), refetch 10s.

**web/src/hooks/useActivity.ts** (CREATE): useWaitEvents + useLongTransactions, refetch 10s.

**web/src/hooks/useAlerts.ts** (UPDATE): Add useInstanceAlerts(instanceId), refetch 30s.

### Shared components

**web/src/components/shared/TimeRangeSelector.tsx** (CREATE): Preset buttons [15m][1h][6h][24h][7d][Custom]. Active: blue-500/20 bg + blue-500 border. Custom mode: two datetime-local inputs + Apply button. Reads/writes timeRangeStore.

**web/src/components/shared/AlertBadge.tsx** (CREATE): Pill badge. Amber warnings, red criticals, hidden if zero.

**web/src/components/charts/TimeSeriesChart.tsx** (CREATE): Generic ECharts line/area. Props: series[] (name, data, color, type, dashed), referenceLine (value, label, color), yAxisLabel, yAxisFormat, yAxisMin, yAxisMax, loading, height(default 300). Uses pgpulse-dark theme. ResizeObserver for responsive. Loading overlay with spinner. Empty state message.

**web/src/components/charts/ConnectionGauge.tsx** (CREATE): ECharts semicircular gauge. Green 0-70%, amber 70-90%, red 90-100%. Shows current/max center text. Props: value, max, optional size.

**web/src/components/charts/WaitEventsChart.tsx** (CREATE): Horizontal bars by wait_event_type. Lock=rose-500, IO=blue-500, LWLock=amber-500, Client=slate-400, Activity=emerald-500.

### Fleet Overview

**web/src/components/fleet/InstanceCard.tsx** (CREATE): Clickable card navigating to /servers/:id. Status dot: green (metric<60s old), yellow (>60s), red (>300s/no metrics). Shows: name bold, host:port subtle, PG version badge, role badge (PRIMARY blue / REPLICA green), connection util%, cache hit%, repl lag (replicas), alert badge. Dark card with hover.

**web/src/pages/FleetOverviewPage.tsx** (REWRITE): Remove all mock data. useInstances({include:['metrics','alerts']}). Grid of InstanceCard. Loading skeleton. Empty state. Responsive: grid-cols-1 md:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4.

### Server Detail

**web/src/components/server/HeaderCard.tsx** (CREATE): Full-width. Instance name large, host:port subtle, PG version badge, role badge, uptime formatted, status dot, collected_at.

**web/src/components/server/KeyMetricsRow.tsx** (CREATE): 4 MetricCards using existing component. Connections: active/max + util%. Cache Hit: ratio% color-coded (green>95, yellow 90-95, red<90). Active Txns: count + waiting. Repl Lag: formatted bytes or N/A Primary.

**web/src/components/server/ReplicationSection.tsx** (CREATE): useReplication. Primary: lag table (replica, state, pending/write/flush/replay bytes + time) + slots table (name, type, active green/red, WAL retained, inactive rows red). Replica: WAL receiver card (host, port, status).

**web/src/components/server/WaitEventsSection.tsx** (CREATE): Card with WaitEventsChart. useWaitEvents. Empty: No active wait events.

**web/src/components/server/LongTransactionsTable.tsx** (CREATE): DataTable. Columns: PID, User, Database, State, Duration (formatted), Query (truncated 80 chars, expand click). useLongTransactions. Yellow bg >60s, red >300s. Empty: No long-running transactions.

**web/src/components/server/InstanceAlerts.tsx** (CREATE): useInstanceAlerts. Alert list: severity badge, rule name, metric+value vs threshold, duration since fired. Empty: green check + No active alerts.

**web/src/pages/ServerDetailPage.tsx** (REWRITE): useParams for id. Layout: Breadcrumb (Fleet > name), HeaderCard, KeyMetricsRow, TimeRangeSelector, 2-col grid (ConnectionsChart | CacheHitChart), ReplicationSection full width, 2-col grid (WaitEventsSection | LongTransactionsTable), InstanceAlerts full width. ConnectionsChart: useMetricsHistory connections.active/idle/total, area for active, max_connections reference line. CacheHitChart: cache.hit_ratio, 95% reference line.

**web/src/App.tsx** (UPDATE): Route /servers/:id to ServerDetailPage in ProtectedRoute. Register ECharts theme on mount.

**web/src/components/layout/Sidebar.tsx** (UPDATE): Fleet Overview highlights on /fleet.

---

## QA AND REVIEW AGENT

### Backend
- All endpoints tested: 200 valid, 401 no auth, 400 bad params
- History step validation rejects invalid values
- No SQL string concatenation — all parameterized queries
- Connections released properly from InstanceConnProvider
- go build ./cmd/pgpulse-server, go vet ./..., go test ./cmd/... ./internal/...

### Frontend
- tsc --noEmit, eslint, vite build all pass
- No mock data in Fleet or Server Detail
- All hooks use apiFetch for auth headers
- refetchInterval correct (10s live, 30s fleet/alerts)
- Presets compute from/to at query time
- Custom ranges disable auto-refresh
- ECharts theme registered before chart render
- Responsive Tailwind breakpoints correct

### Integration
- API response shapes match TypeScript interfaces
- Metric names match collector output
- ISO 8601 time format consistent
- Empty states render for all sections

---

## COORDINATION

Backend starts first on storage queries and handlers.
Frontend starts immediately on stores, formatters, theme, chart components (no API dependency).
Frontend wires hooks once Backend confirms response shapes.
QA writes stubs, fills assertions as code lands.
Merge: storage queries then API handlers then frontend stores/hooks then components then pages then tests.
List all files at end for developer build.
