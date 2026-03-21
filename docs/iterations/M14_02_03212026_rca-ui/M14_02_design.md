# PGPulse M14_02 — RCA UI — Design Document

**Date:** 2026-03-21
**Iteration:** M14_02
**Parent:** M14_requirements_v2.md
**Depends on:** M14_01 (complete — RCA engine, 5 API endpoints, 20 chains)
**Scope:** RCA Incidents page, incident timeline visualization, causal graph view, alert detail integration, "Investigate" button, sidebar navigation

---

## 1. Integration Points

### 1.1 API Endpoints (from M14_01)

| Method | Path | Response Shape |
|--------|------|---------------|
| POST | `/instances/{id}/rca/analyze` | `{ data: Incident }` |
| GET | `/instances/{id}/rca/incidents` | `{ data: { incidents: Incident[], total: number } }` |
| GET | `/instances/{id}/rca/incidents/{incidentId}` | `{ data: Incident }` |
| GET | `/rca/incidents` | `{ data: { incidents: Incident[], total: number } }` |
| GET | `/rca/graph` | `{ data: { nodes: CausalNode[], edges: CausalEdge[], chain_ids: string[] } }` |

### 1.2 Incident JSON Shape (from M14_01 `incident.go`)

```typescript
interface Incident {
  id: number;
  instance_id: string;
  trigger_metric: string;
  trigger_value: number;
  trigger_time: string;        // ISO 8601
  trigger_kind: string;        // "alert" | "manual"
  analysis_window: { from: string; to: string };
  primary_chain?: CausalChainResult;
  alternative_chain?: CausalChainResult;
  timeline?: TimelineEvent[];
  summary: string;
  confidence: number;          // 0.0–1.0
  confidence_bucket: string;   // "high" | "medium" | "low"
  quality: QualityStatus;
  remediation_hooks?: string[];
  auto_triggered: boolean;
  chain_version: string;
  anomaly_mode: string;        // "ml" | "threshold"
  created_at: string;
}

interface CausalChainResult {
  chain_id: string;
  chain_name: string;
  score: number;
  root_cause_key: string;
  events: TimelineEvent[];
}

interface TimelineEvent {
  timestamp: string;
  node_id: string;
  node_name: string;
  metric_key: string;
  value: number;
  baseline_val: number;
  z_score: number;
  strength: number;
  layer: string;              // "db" | "os" | "workload" | "config"
  role: string;               // "root_cause" | "intermediate" | "symptom"
  evidence: string;           // "required" | "supporting"
  description: string;
  edge_desc: string;
}

interface QualityStatus {
  telemetry_completeness: number;
  anomaly_source_mode: string;
  scope_limitations: string[];
  unavailable_deps?: string[];
}
```

### 1.3 Existing Frontend Patterns

| Pattern | Example | Reuse |
|---------|---------|-------|
| Page layout | `Advisor.tsx` (120 lines) | Page structure, filter + list layout |
| Detail panel | `AlertDetailPanel.tsx` (249 lines) | Side-panel slide-in pattern |
| Data table | `DataTable.tsx` (111 lines) | Sortable table component |
| Status badge | `StatusBadge.tsx` (33 lines) | Color-coded severity display |
| Time range | `TimeRangeSelector.tsx` (85 lines) | Time filtering pattern |
| React Query hooks | `useRecommendations.ts` (106 lines) | Fetch + cache pattern |
| ECharts wrapper | `EChartWrapper.tsx` (33 lines) | Chart rendering |
| Sidebar nav | `Sidebar.tsx` (226 lines) | Expandable section pattern |
| Breadcrumb | `Breadcrumb.tsx` (53 lines) | Navigation context |

### 1.4 Existing Pages That Get Modified

| Page | Change |
|------|--------|
| `AlertsDashboard.tsx` | Add "Investigate" button on each alert row |
| `AlertDetailPanel.tsx` | Add inline RCA summary section with link to full timeline |
| `Sidebar.tsx` | Add "RCA Incidents" link under fleet-level nav |
| `App.tsx` | Add routes for RCA pages |

---

## 2. New Pages

### 2.1 RCA Incidents List (`/rca/incidents`)

Fleet-wide incident list. Similar to Advisor page layout.

**Header:** "RCA Incidents" with total count
**Filters:** Instance selector, confidence bucket (high/medium/low/all), auto vs manual trigger
**Table columns:** Timestamp, Instance, Trigger Metric, Summary (truncated), Confidence badge, Chain ID, Auto/Manual badge
**Row click:** Navigate to `/rca/incidents/{id}`
**Refresh:** 30s polling via React Query

### 2.2 Instance RCA Incidents (`/servers/:serverId/rca/incidents`)

Per-instance incident list. Same layout as fleet-wide but filtered to one instance. Accessible from sidebar under each server.

### 2.3 Incident Detail (`/rca/incidents/:incidentId`)

The showcase page. Displays the full RCA analysis result.

**Sections:**

1. **Header card** — Instance name, trigger metric, trigger time, confidence badge (high=green, medium=yellow, low=red), auto/manual badge, chain version

2. **Summary banner** — The qualified summary text: "Likely caused by..." Styled as a prominent callout with appropriate tone (not alarming, informative)

3. **Quality banner** (if `quality_banner_enabled`) — Shows telemetry completeness %, anomaly source mode (ML/threshold), scope limitations. Dismissible.

4. **Timeline visualization** — The core component. Vertical event chain:

```
   ● Root Cause (earliest)
   │  [DB] Bulk workload detected
   │  pg.statements.top.avg_time_ms: 450ms (4.2x baseline)
   │  Z-score: 3.8 | Strength: 0.92
   │
   ├──── "WAL spike causes checkpoint within 1-3 min" ────▶
   │
   ● Intermediate
   │  [DB] Checkpoint storm
   │  pg.checkpoint.requested_per_second: 12.4 (8x baseline)
   │  Z-score: 4.1 | Strength: 0.95
   │
   ├──── "Checkpoint storm saturates disk I/O" ────▶
   │
   ● Intermediate
   │  [OS] Disk I/O saturation
   │  os.disk.util_pct: 98.2% (baseline: 35%)
   │  Z-score: 5.2 | Strength: 0.98
   │
   ├──── "Disk I/O saturation causes replication lag" ────▶
   │
   ◉ Symptom (trigger)
      [DB] Replication lag spike
      pg.replication.lag.replay_bytes: 35MB
      TRIGGER EVENT
```

Each node is color-coded by layer: DB=blue, OS=green, Workload=purple, Config=orange.
Each node shows: role badge, node name, metric key + value, baseline comparison, Z-score, strength bar.
Edges show the causal description text and time lag.

5. **Alternative chain** (if present) — Collapsed section "Alternative explanation (score: 0.65)" expandable to show a second timeline.

6. **Remediation hooks** — If the incident has remediation hook IDs, show a "Recommended Actions" section linking to adviser rules. "Based on this analysis, consider: [Tune checkpoint_completion_target]"

7. **Analysis metadata** — Collapsible footer: analysis window, chain version, anomaly source mode, created_at.

### 2.4 Causal Graph View (`/rca/graph`)

Optional but valuable for understanding the knowledge base. Shows the full causal graph as an interactive node-edge diagram.

**Implementation:** Use ECharts graph type (force-directed or custom layout). Nodes are metric categories, edges are causal links. Click a node to see its metric keys. Click an edge to see lag/evidence/confidence.

This is a lower-priority "reference" page, not the primary user flow. Keep it simple.

---

## 3. New Components

### 3.1 File Layout

```
web/src/components/rca/
├── IncidentRow.tsx           // Single incident in list table
├── IncidentTimeline.tsx      // Vertical timeline visualization (core component)
├── TimelineNode.tsx          // Single node in timeline (role badge, metrics, strength)
├── TimelineEdge.tsx          // Causal edge between nodes (description, lag)
├── ConfidenceBadge.tsx       // Color-coded confidence indicator (high/med/low)
├── QualityBanner.tsx         // Telemetry completeness + scope limitations
├── ChainSummaryCard.tsx      // Summary banner with qualified language
├── RemediationHooks.tsx      // Recommended actions from RCA findings
├── IncidentFilters.tsx       // Filter controls for incident list
├── CausalGraphView.tsx       // Interactive graph visualization (ECharts)

web/src/hooks/
├── useRCA.ts                 // React Query hooks for all RCA endpoints

web/src/pages/
├── RCAIncidents.tsx          // Fleet-wide incident list page
├── InstanceRCAIncidents.tsx  // Per-instance incident list (optional — can reuse RCAIncidents with filter)
├── RCAIncidentDetail.tsx     // Full incident detail with timeline
├── RCACausalGraph.tsx        // Causal graph reference page

web/src/types/
├── rca.ts                    // TypeScript interfaces for RCA types
```

### 3.2 Component Specifications

**`IncidentTimeline.tsx` (~200 lines)** — The most important component.

Props: `events: TimelineEvent[]`, `primaryChain?: CausalChainResult`

Renders a vertical timeline with:
- Nodes connected by dashed lines
- Each node is a `TimelineNode` component
- Edges between nodes are `TimelineEdge` components
- Events are sorted by timestamp (earliest at top = root cause, latest at bottom = symptom)
- The trigger event (symptom) gets a special "TRIGGER" badge

**`TimelineNode.tsx` (~80 lines)**

Props: `event: TimelineEvent`

Renders:
- Circle indicator: filled for root_cause (red), hollow for intermediate (yellow), target for symptom (blue)
- Layer badge: small tag showing "DB" / "OS" / "Workload" with layer color
- Node name (bold)
- Metric key + value, with baseline comparison: "98.2% (baseline: 35%)"
- Strength bar: horizontal bar 0–100%, color from green to red
- Z-score if available: "Z: 4.1"
- Evidence type: small "Required" / "Supporting" label

**`ConfidenceBadge.tsx` (~25 lines)**

Props: `bucket: "high" | "medium" | "low"`, `score: number`

Colors: high=green, medium=yellow, low=red. Shows bucket label + score percentage.

**`QualityBanner.tsx` (~50 lines)**

Props: `quality: QualityStatus`

Renders a dismissible info banner showing: telemetry completeness %, anomaly source (ML/threshold), scope limitations list. Styled as a subtle informational bar, not alarming.

**`useRCA.ts` (~80 lines)**

React Query hooks:
- `useRCAIncidents(instanceId?, limit, offset)` — fetches incident list
- `useRCAIncident(incidentId)` — fetches single incident
- `useRCAGraph()` — fetches causal graph
- `useRCAAnalyze()` — mutation for on-demand analysis

---

## 4. Alert Detail Integration

### 4.1 "Investigate" Button on Alert Rows

In `AlertRow.tsx` or `AlertsDashboard.tsx`: add a small "Investigate" button/icon next to each alert. Click triggers `POST /instances/{id}/rca/analyze` with the alert's metric and timestamp, then navigates to the resulting incident detail page.

### 4.2 Inline RCA Summary on AlertDetailPanel

In `AlertDetailPanel.tsx`: add a new section below existing content. If the alert has associated RCA incidents (query incidents where `trigger_metric` matches and `trigger_time` is close), show:

```
┌─────────────────────────────────────┐
│ 🔍 Root Cause Analysis              │
│                                     │
│ Likely caused by WAL generation     │
│ spike following bulk workload.      │
│ Confidence: ●●●○ Medium (0.62)     │
│                                     │
│ [View Full Timeline →]              │
└─────────────────────────────────────┘
```

If no RCA incident exists for this alert, show "Investigate" button that triggers on-demand analysis.

---

## 5. Sidebar Navigation

Add to `Sidebar.tsx`:

**Fleet-level (top section):**
- Existing: Fleet Overview, Alerts ▸, Advisor, Settings Diff, Administration
- **New:** "RCA Incidents" with `Search` or `GitBranch` icon from lucide-react

**Per-server (under each server):**
- Existing: Query Insights, Workload Report
- **New:** "RCA Incidents" — links to `/servers/:serverId/rca/incidents`

---

## 6. Routes

Add to `App.tsx`:

```tsx
<Route path="/rca/incidents" element={<RCAIncidents />} />
<Route path="/rca/incidents/:incidentId" element={<RCAIncidentDetail />} />
<Route path="/rca/graph" element={<RCACausalGraph />} />
<Route path="/servers/:serverId/rca/incidents" element={<RCAIncidents />} />
```

The per-server route reuses the same `RCAIncidents` component but passes `serverId` as a filter.

---

## 7. Agent Team: 2 Agents

### Agent 1 — Frontend

**Owns:** All new files in `web/src/components/rca/`, `web/src/hooks/useRCA.ts`, `web/src/pages/RCA*.tsx`, `web/src/types/rca.ts`

**Modifies:** `web/src/App.tsx` (routes), `web/src/components/layout/Sidebar.tsx` (nav links), `web/src/components/alerts/AlertDetailPanel.tsx` (inline RCA), `web/src/components/alerts/AlertRow.tsx` (investigate button)

**Tasks (in order):**
1. Create `web/src/types/rca.ts` — TypeScript interfaces
2. Create `web/src/hooks/useRCA.ts` — React Query hooks for 5 endpoints
3. Create `web/src/components/rca/ConfidenceBadge.tsx`
4. Create `web/src/components/rca/QualityBanner.tsx`
5. Create `web/src/components/rca/TimelineNode.tsx`
6. Create `web/src/components/rca/TimelineEdge.tsx`
7. Create `web/src/components/rca/IncidentTimeline.tsx` — core timeline visualization
8. Create `web/src/components/rca/ChainSummaryCard.tsx`
9. Create `web/src/components/rca/RemediationHooks.tsx`
10. Create `web/src/components/rca/IncidentFilters.tsx`
11. Create `web/src/components/rca/IncidentRow.tsx`
12. Create `web/src/components/rca/CausalGraphView.tsx` — ECharts graph
13. Create `web/src/pages/RCAIncidents.tsx` — fleet + per-instance list
14. Create `web/src/pages/RCAIncidentDetail.tsx` — full detail with timeline
15. Create `web/src/pages/RCACausalGraph.tsx` — graph reference page
16. Modify `web/src/App.tsx` — add 4 routes
17. Modify `web/src/components/layout/Sidebar.tsx` — add RCA nav links
18. Modify `web/src/components/alerts/AlertDetailPanel.tsx` — inline RCA summary
19. Modify `web/src/components/alerts/AlertRow.tsx` — "Investigate" button

### Agent 2 — QA

**Owns:** Build verification, lint, typecheck

**Tasks:**
1. `cd web && npm run build && npm run typecheck && npm run lint`
2. `go build ./cmd/pgpulse-server` (backend unchanged)
3. `go test ./cmd/... ./internal/... -count=1` (all backend tests still pass)
4. `golangci-lint run ./cmd/... ./internal/...`
5. Verify zero Go file changes (pure frontend iteration)
6. Commit

---

## 8. DO NOT RE-DISCUSS

| Decision | Status |
|----------|--------|
| All D400–D408 | Locked |
| RCA backend | Complete (M14_01) — do not modify `internal/rca/*` |
| API endpoints | 5 endpoints exist and work — no changes |
| Timeline is vertical | Locked — vertical event chain, not horizontal |
| Node colors by layer | DB=blue, OS=green, Workload=purple, Config=orange |
| Confidence colors | High=green, Medium=yellow, Low=red |
| Summary language | Qualified — engine produces it, frontend just displays |
| ECharts for graph view | Use existing ECharts setup, graph chart type |
| React Query | Follow existing hook patterns (useRecommendations.ts as template) |
| Tailwind CSS | All styling via Tailwind utility classes |
| No Go changes | Zero backend changes in M14_02 |

---

## 9. Watch-List

**New files (~15):**
- [ ] `web/src/types/rca.ts`
- [ ] `web/src/hooks/useRCA.ts`
- [ ] `web/src/components/rca/ConfidenceBadge.tsx`
- [ ] `web/src/components/rca/QualityBanner.tsx`
- [ ] `web/src/components/rca/TimelineNode.tsx`
- [ ] `web/src/components/rca/TimelineEdge.tsx`
- [ ] `web/src/components/rca/IncidentTimeline.tsx`
- [ ] `web/src/components/rca/ChainSummaryCard.tsx`
- [ ] `web/src/components/rca/RemediationHooks.tsx`
- [ ] `web/src/components/rca/IncidentFilters.tsx`
- [ ] `web/src/components/rca/IncidentRow.tsx`
- [ ] `web/src/components/rca/CausalGraphView.tsx`
- [ ] `web/src/pages/RCAIncidents.tsx`
- [ ] `web/src/pages/RCAIncidentDetail.tsx`
- [ ] `web/src/pages/RCACausalGraph.tsx`

**Modified files (~4):**
- [ ] `web/src/App.tsx` (add routes)
- [ ] `web/src/components/layout/Sidebar.tsx` (add RCA nav)
- [ ] `web/src/components/alerts/AlertDetailPanel.tsx` (inline RCA)
- [ ] `web/src/components/alerts/AlertRow.tsx` (investigate button)

**Unchanged (verify):**
- [ ] ALL `internal/**` Go files — zero changes
- [ ] `cmd/**` — zero changes
