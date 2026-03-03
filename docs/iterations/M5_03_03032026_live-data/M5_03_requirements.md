# M5_03 Requirements — Live Data Integration: Fleet Overview + Server Detail

**Iteration:** M5_03
**Date:** 2026-03-03
**Depends on:** M5_02 (auth + RBAC), M2 (storage + API), M4 (alerting)
**Milestone:** M5 — Web UI

---

## Goal

Replace all mock data in the PGPulse frontend with real backend data. Build the Fleet Overview page (instance cards with live metrics) and the Server Detail page (the core monitoring view — charts, replication, wait events, long transactions, alerts). This is where PGPulse transitions from "working scaffold" to "functional monitoring tool."

---

## Scope

### In Scope

1. **Fleet Overview page rewrite** — real instance data, live metrics per card, auto-refresh
2. **Server Detail page build** — 8 sections with real data:
   - Header card (version, uptime, role, status)
   - Key metrics row (4 MetricCards: connections, cache hit, active txns, replication lag)
   - Connections time-series chart (ECharts)
   - Cache hit time-series chart (ECharts)
   - Replication section (lag table + slots table)
   - Wait events bar chart
   - Long transactions table
   - Instance alerts list
3. **Time range selector** — preset buttons (15m, 1h, 6h, 24h, 7d) + custom date picker
4. **Backend API additions** — 5 new/updated endpoints to serve frontend data needs
5. **ECharts dark theme** — consistent chart styling across all visualizations
6. **TanStack Query hooks** — data fetching layer with auto-refresh

### Out of Scope (Deferred)

- pg_stat_statements top queries view — M5_04
- Lock tree visualization — M5_04
- Progress monitoring (VACUUM, CREATE INDEX) — M5_04
- Per-database drill-down — M5_04
- SSE/WebSocket real-time streaming — future iteration
- Instance CRUD from frontend (add/remove servers) — future iteration

---

## User Stories

**US-01: DBA views fleet health at a glance**
As a DBA, I open PGPulse and immediately see all monitored instances with their current status, connection utilization, cache hit ratio, replication lag, and active alert counts — so I can identify which servers need attention without clicking into each one.

**US-02: DBA investigates a specific instance**
As a DBA, I click on an instance card and see a detailed view with real-time metrics, historical charts, replication state, wait events, and long transactions — the equivalent of PGAM's analiz2.php but with time-series history and a modern interface.

**US-03: DBA adjusts time range for forensic analysis**
As a DBA investigating a past incident, I select a custom time range (e.g., yesterday 2:00 PM to 3:00 PM) and all charts on the Server Detail page update to show that window — so I can correlate metric changes across a specific timeframe.

**US-04: DBA spots long-running transactions**
As a DBA, I see a table of long-running transactions sorted by duration with color-coded severity — so I can quickly identify transactions that may need to be terminated.

**US-05: DBA monitors replication health**
As a DBA managing a primary with replicas, I see replication lag broken down by phase (pending, write, flush, replay) with slot health indicators — so I can diagnose replication delays.

---

## Acceptance Criteria

### Fleet Overview
1. Page loads real instances from GET /api/v1/instances?include=metrics,alerts
2. Each instance card shows: name, host:port, PG version, role, status, connection%, cache hit%, replication lag, alert badges
3. Cards auto-refresh every 30 seconds without flicker
4. Clicking a card navigates to /servers/:id
5. Responsive grid: 1 column mobile, 2 tablet, 3-4 desktop
6. Loading skeleton shown while data fetches
7. Empty state shown when no instances configured

### Server Detail
8. Header card shows real PG version, uptime (human-readable), role badge, status dot
9. Four MetricCards show real values with color-coded thresholds
10. Connections chart renders time-series with active/idle/total series + max_connections line
11. Cache hit chart renders time-series with 95% reference line
12. Time range selector controls all charts — preset buttons + custom date picker
13. Changing time range updates all charts simultaneously
14. Replication section shows lag-by-phase table for primaries, WAL receiver for replicas
15. Replication slots table shows slot health with inactive slots highlighted red
16. Wait events bar chart shows current distribution grouped by type
17. Long transactions table shows PID, user, database, state, duration, query (truncated)
18. Long transactions color-coded: yellow > 1 min, red > 5 min
19. Instance alerts section shows active alerts for this instance
20. All sections show loading spinners while fetching
21. All sections show appropriate empty states

### Backend
22. GET /api/v1/instances?include=metrics,alerts returns enriched instance list
23. GET /api/v1/instances/:id/metrics/current returns latest metric snapshot
24. GET /api/v1/instances/:id/metrics/history returns time-series with step aggregation
25. GET /api/v1/instances/:id/replication returns structured replication data
26. GET /api/v1/instances/:id/activity/wait-events returns wait event distribution
27. GET /api/v1/instances/:id/activity/long-transactions returns long transactions
28. GET /api/v1/instances/:id/alerts returns filtered alerts for instance
29. All new endpoints require auth and respect RBAC
30. go build, go vet, go test pass
31. tsc --noEmit, eslint, vite build pass

---

## Non-Functional Requirements

- Fleet Overview initial load < 2s for 20 instances
- Server Detail initial load < 3s (all sections)
- Chart rendering < 500ms after data arrives
- No visible flicker on auto-refresh
- Custom date picker uses native HTML5 datetime-local (no new library)
