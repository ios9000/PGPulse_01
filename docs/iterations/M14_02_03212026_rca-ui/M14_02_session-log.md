# PGPulse M14_02 — Session Log

**Date:** 2026-03-21
**Iteration:** M14_02 — RCA UI (incidents page, timeline visualization, alert integration)
**Duration:** ~1 session
**Tool:** Claude Code (Opus 4.6, 1M context) — Agent Teams (2 agents)
**Commit:** 9439d3c

---

## Goal

Add the frontend for the RCA engine built in M14_01. Incident list pages (fleet-wide and per-instance), incident detail with vertical timeline visualization, causal graph reference page, and alert integration ("Investigate" button + inline RCA summaries).

This is a pure frontend iteration — zero Go files modified.

---

## Agent Team

| Agent | Role | Files Created | Files Modified |
|-------|------|---------------|----------------|
| Agent 1 — Frontend | Types, hooks, 10 components, 3 pages, route/sidebar/alert integration | 15 | 5 |
| Agent 2 — QA (Team Lead) | Build verification (npm build, typecheck, lint, Go build/test) | 0 | 0 |

**Execution:** Agent 1 ran (~7 min). QA verification ran after completion. Total wall-clock: ~10 minutes.

---

## What Was Built

### New Files (15 files, 1,083 lines)

| File | Lines | Purpose |
|------|-------|---------|
| `web/src/types/rca.ts` | 80 | TypeScript interfaces: RCAIncident, RCACausalChainResult, RCATimelineEvent, RCAQualityStatus, RCACausalNode, RCACausalEdge, RCACausalGraph |
| `web/src/hooks/useRCA.ts` | 89 | 5 React Query hooks: useRCAIncidents, useInstanceRCAIncidents, useRCAIncident, useRCAGraph, useRCAAnalyze (mutation) |
| `web/src/components/rca/ConfidenceBadge.tsx` | 23 | Colored pill badge (green=high, yellow=medium, red=low) with score percentage |
| `web/src/components/rca/QualityBanner.tsx` | 46 | Dismissible info banner: telemetry completeness %, anomaly source mode, scope limitations |
| `web/src/components/rca/ChainSummaryCard.tsx` | 26 | Prominent callout card with qualified summary text and confidence badge |
| `web/src/components/rca/RemediationHooks.tsx` | 36 | "Recommended Actions" list with formatted hook labels |
| `web/src/components/rca/TimelineNode.tsx` | 90 | Timeline event node: role indicator (root_cause/intermediate/symptom), layer badge (DB/OS/Workload/Config), metric key, value vs baseline, strength bar, Z-score |
| `web/src/components/rca/TimelineEdge.tsx` | 22 | Dashed connector between timeline nodes with causal description text |
| `web/src/components/rca/IncidentTimeline.tsx` | 54 | Full vertical timeline: sorts events chronologically, alternates nodes and edges |
| `web/src/components/rca/IncidentRow.tsx` | 49 | Table row for incident list: timestamp, instance, trigger metric, summary, confidence badge, auto/manual badge |
| `web/src/components/rca/IncidentFilters.tsx` | 87 | Filter bar: instance dropdown, confidence bucket selector, trigger kind (auto/manual) |
| `web/src/components/rca/CausalGraphView.tsx` | 125 | ECharts force-directed graph: nodes colored by layer, edges with description tooltips |
| `web/src/pages/RCAIncidents.tsx` | 154 | Incident list page — works for both fleet-wide and per-instance, with pagination and client-side filtering |
| `web/src/pages/RCAIncidentDetail.tsx` | 185 | Incident detail: header card, summary banner, quality banner, timeline visualization, alternative chain (collapsible), remediation hooks, metadata footer |
| `web/src/pages/RCACausalGraph.tsx` | 17 | Causal knowledge graph reference page wrapping CausalGraphView |

### Modified Files (5)

| File | Change |
|------|--------|
| `web/src/App.tsx` | +7 lines: 4 new routes (fleet incidents, causal graph, server incidents, server incident detail) |
| `web/src/components/layout/Sidebar.tsx` | +4 lines: Search icon import, "RCA Incidents" in main nav + per-server sub-items |
| `web/src/components/alerts/AlertDetailPanel.tsx` | +63 lines: "Root Cause Analysis" section with matching incident display or "Investigate Root Cause" button |
| `web/src/components/alerts/AlertRow.tsx` | +29 lines: Investigate icon button (Search) with analyze mutation + navigation |
| `web/src/pages/AlertsDashboard.tsx` | +1 line: Empty header column for investigate button |

### New Routes

| Route | Page | Purpose |
|-------|------|---------|
| `/rca/incidents` | RCAIncidents | Fleet-wide incident list |
| `/rca/graph` | RCACausalGraph | Causal knowledge graph reference |
| `/servers/:serverId/rca/incidents` | RCAIncidents | Per-instance incident list |
| `/servers/:serverId/rca/incidents/:incidentId` | RCAIncidentDetail | Full incident detail with timeline |

### React Query Hooks

| Hook | Type | Endpoint | Refresh |
|------|------|----------|---------|
| `useRCAIncidents` | query | GET `/rca/incidents` | 30s |
| `useInstanceRCAIncidents` | query | GET `/instances/{id}/rca/incidents` | 30s |
| `useRCAIncident` | query | GET `/instances/{id}/rca/incidents/{incidentId}` | — |
| `useRCAGraph` | query | GET `/rca/graph` | 5min stale |
| `useRCAAnalyze` | mutation | POST `/instances/{id}/rca/analyze` | invalidates rca-incidents |

---

## Key Design Decisions Implemented

| Decision | Implementation |
|----------|----------------|
| Timeline layout | Vertical event chain — root cause at top, symptom at bottom |
| Node colors by layer | DB = blue-400, OS = green-400, Workload = purple-400, Config = orange-400 |
| Confidence badge colors | High = green, Medium = yellow, Low = red |
| Summary text | Displayed as-is from API (engine produces qualified language) |
| Alert integration | Inline RCA summary in AlertDetailPanel + "Investigate" button on AlertRow |
| Navigation | RCA Incidents in main sidebar nav + per-server sub-items |
| Graph visualization | ECharts force-directed layout with node/edge tooltips |
| Zero backend changes | Pure frontend — no Go files modified |

---

## Verification Results

| Check | Result |
|-------|--------|
| `npm run build` | OK (pre-existing chunk size warning) |
| `npm run typecheck` | OK (0 errors) |
| `npm run lint` | 0 errors (1 pre-existing warning) |
| `go build ./cmd/pgpulse-server` | OK |
| `go test ./cmd/... ./internal/...` | All PASS |
| `golangci-lint run` | 0 issues |
| No Go files changed | Confirmed |

---

## Notable Observations

1. **Graph API returns PascalCase:** The Go CausalGraph struct has no json tags, so fields serialize as PascalCase (Nodes, Edges, ChainIDs, FromNode, etc.). TypeScript types match this convention for graph types while incident types use snake_case.

2. **Dual-mode incidents page:** RCAIncidents.tsx serves both fleet-wide (`/rca/incidents`) and per-instance (`/servers/:serverId/rca/incidents`) routes by checking the `serverId` URL param and switching between `useRCAIncidents` and `useInstanceRCAIncidents`.

3. **Alert integration pattern:** AlertDetailPanel queries recent incidents for the alert's instance and filters client-side by trigger_metric. This avoids a dedicated API endpoint for "incidents matching this alert" while still providing relevant context.

4. **Incident detail routing:** Uses `/servers/:serverId/rca/incidents/:incidentId` (not `/rca/incidents/:incidentId`) because the API requires instanceId. IncidentRow navigates with the instance context already embedded in the URL.

---

## What's Next

M14 backend and frontend are complete. Potential follow-ups:
- M14_03: Activate Tier B chains (query regression, new query, config state, network)
- Statement snapshot diff integration for chains 12/13
- Incident review workflow (review_status, reviewed_by fields already in schema)
- RCA retention cleanup worker (schema supports it, worker not yet implemented)

---

## Stats

- **Total lines added:** 1,184 (20 files)
- **New frontend code:** 1,083 lines (15 new files)
- **Modified files:** 5 existing files (~104 lines added)
- **New routes:** 4
- **New React Query hooks:** 5
- **New components:** 10
- **New pages:** 3
