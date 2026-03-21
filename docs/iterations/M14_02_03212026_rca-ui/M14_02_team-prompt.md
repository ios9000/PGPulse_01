# PGPulse M14_02 — Team Prompt

**Iteration:** M14_02 — RCA UI
**Date:** 2026-03-21
**Agent team size:** 2 (Frontend + QA)
**Team lead model:** Opus

---

## Context

Read these files before starting:
- `docs/iterations/M14_02_03212026_rca-ui/design.md` — full UI design, component specs, integration points
- `docs/iterations/M14_01_03212026_rca-engine/requirements.md` — RCA requirements with amendments
- `CLAUDE.md` — project conventions
- `docs/CODEBASE_DIGEST.md` — current file inventory, frontend component map

M14_01 built the RCA backend engine with 5 API endpoints. M14_02 adds the frontend: incident list pages, timeline visualization, causal graph view, alert detail integration, and sidebar navigation.

**This is a pure frontend iteration. Zero Go files are modified.**

---

## DO NOT RE-DISCUSS

| Decision | Locked Value |
|----------|-------------|
| All D400–D408 | Locked from M14_requirements_v2.md |
| Backend | Complete in M14_01 — do NOT modify any `internal/` or `cmd/` files |
| API endpoints | 5 endpoints working: POST analyze, GET incidents (instance + fleet), GET incident detail, GET graph |
| Timeline layout | Vertical event chain (root cause at top → symptom at bottom) |
| Node colors by layer | DB = blue (`text-blue-400`), OS = green (`text-green-400`), Workload = purple (`text-purple-400`), Config = orange (`text-orange-400`) |
| Confidence badge colors | High = green, Medium = yellow, Low = red |
| Summary text | Displayed as-is from API (engine produces qualified language) |
| ECharts | Use existing setup for causal graph view |
| React Query | Follow `useRecommendations.ts` / `useSnapshots.ts` pattern |
| Styling | Tailwind CSS utility classes only |
| Build scope | `./cmd/... ./internal/...` for Go, `npm run build/typecheck/lint` for frontend |
| Git branch | `master` |

---

## Team Structure

### Agent 1 — Frontend

**Owns:** All new files in `web/src/components/rca/`, `web/src/hooks/useRCA.ts`, `web/src/pages/RCA*.tsx`, `web/src/types/rca.ts`. Modifications to `App.tsx`, `Sidebar.tsx`, `AlertDetailPanel.tsx`, `AlertRow.tsx`.

**Does NOT touch:** Any file outside `web/src/`. No Go files. No config files. No migrations.

**Tasks (in order):**

#### Task 1 — TypeScript types (`web/src/types/rca.ts`)

Define all RCA interfaces matching the API response shapes:

```typescript
export interface Incident {
  id: number;
  instance_id: string;
  trigger_metric: string;
  trigger_value: number;
  trigger_time: string;
  trigger_kind: string;
  analysis_window: { from: string; to: string };
  primary_chain?: CausalChainResult;
  alternative_chain?: CausalChainResult;
  timeline?: TimelineEvent[];
  summary: string;
  confidence: number;
  confidence_bucket: string;
  quality: QualityStatus;
  remediation_hooks?: string[];
  auto_triggered: boolean;
  chain_version: string;
  anomaly_mode: string;
  created_at: string;
}

export interface CausalChainResult { ... }
export interface TimelineEvent { ... }
export interface QualityStatus { ... }
export interface CausalNode { ... }
export interface CausalEdge { ... }
export interface CausalGraph { ... }
```

See design doc Section 1.2 for the full shapes. Match the JSON field names exactly as the Go backend serializes them (snake_case).

#### Task 2 — React Query hooks (`web/src/hooks/useRCA.ts`)

Follow the pattern in `useRecommendations.ts` and `useSnapshots.ts`:

```typescript
// Fetch fleet-wide incidents
export function useRCAIncidents(params: { limit?: number; offset?: number }) { ... }

// Fetch per-instance incidents
export function useInstanceRCAIncidents(instanceId: string, params: { limit?: number; offset?: number }) { ... }

// Fetch single incident detail
export function useRCAIncident(incidentId: number) { ... }

// Fetch causal graph
export function useRCAGraph() { ... }

// Trigger on-demand analysis (mutation)
export function useRCAAnalyze() { ... }
```

Use the existing `api` client from `web/src/lib/api.ts`. All GET hooks use `useQuery` with appropriate cache keys. The analyze mutation uses `useMutation`.

#### Task 3 — Small components

Create these small components in `web/src/components/rca/`:

**`ConfidenceBadge.tsx` (~25 lines)**
- Props: `bucket: string`, `score: number`
- Renders: pill badge with color (high=green-500, medium=yellow-500, low=red-500) and label + score percentage
- Pattern: follow `StatusBadge.tsx` / `PriorityBadge.tsx`

**`QualityBanner.tsx` (~50 lines)**
- Props: `quality: QualityStatus`, `onDismiss?: () => void`
- Renders: info banner with telemetry completeness %, anomaly source mode, scope limitations list
- Dismissible via optional close button
- Style: subtle blue/gray info bar, not alarming

**`ChainSummaryCard.tsx` (~40 lines)**
- Props: `summary: string`, `confidence: number`, `bucket: string`
- Renders: prominent callout card with the qualified summary text and confidence badge
- Style: left border colored by confidence bucket

**`RemediationHooks.tsx` (~35 lines)**
- Props: `hooks: string[]`
- Renders: "Recommended Actions" section with list of hook IDs formatted as readable labels
- If empty/null, render nothing

#### Task 4 — Timeline components (core)

**`TimelineNode.tsx` (~80 lines)**
- Props: `event: TimelineEvent`, `isFirst: boolean`, `isLast: boolean`
- Renders:
  - Circle indicator: red filled for `root_cause`, yellow hollow for `intermediate`, blue target for `symptom`
  - Layer badge: small rounded tag "DB" / "OS" / "Workload" / "Config" with layer color
  - Node name (font-semibold)
  - Metric key in monospace + value + baseline comparison: "98.2% (baseline: 35%)"
  - Strength bar: thin horizontal bar 0–100%, color gradient from green to red
  - Z-score if > 0: "Z: 4.1"
  - Evidence type: tiny "Required" / "Supporting" label in muted text
- Vertical connector line to next node (except for last)

**`TimelineEdge.tsx` (~40 lines)**
- Props: `description: string`
- Renders: horizontal dashed line with causal description text centered
- Style: muted text, small arrow indicator

**`IncidentTimeline.tsx` (~120 lines)**
- Props: `events: TimelineEvent[]`, `primaryChain?: CausalChainResult`
- Sorts events by timestamp ascending (root cause first, symptom last)
- Renders alternating `TimelineNode` and `TimelineEdge` components
- If `events` is null or empty, show EmptyState: "No causal chain identified"
- If `primaryChain` provided, use its events; otherwise use top-level `events`

This is the showcase component. Spend time making it look good.

#### Task 5 — List components

**`IncidentFilters.tsx` (~60 lines)**
- Props: `onFilter: (filters) => void`, `instances?: Instance[]`
- Renders: instance dropdown, confidence bucket selector (all/high/medium/low), auto/manual toggle
- Pattern: follow `AlertFilters.tsx`

**`IncidentRow.tsx` (~50 lines)**
- Props: `incident: Incident`, `onClick: () => void`
- Renders table row: timestamp (formatted), instance name, trigger metric (truncated), summary (truncated to ~60 chars), confidence badge, auto/manual badge
- Clickable → navigates to detail
- Pattern: follow `AlertRow.tsx` / `AdvisorRow.tsx`

#### Task 6 — Causal graph visualization

**`CausalGraphView.tsx` (~120 lines)**
- Props: none (fetches data via `useRCAGraph()`)
- Uses ECharts graph chart type with force-directed layout
- Nodes: circles colored by layer, labeled with node name
- Edges: arrows with description as tooltip
- Click node: tooltip shows metric keys
- Click edge: tooltip shows lag range, evidence requirement, confidence
- Keep it functional, not over-designed — this is a reference page

#### Task 7 — Pages

**`RCAIncidents.tsx` (~120 lines)**
- Reusable for both fleet-wide (`/rca/incidents`) and per-instance (`/servers/:serverId/rca/incidents`)
- If `serverId` param present, use `useInstanceRCAIncidents`, else `useRCAIncidents`
- PageHeader: "RCA Incidents" (or "RCA Incidents — {instanceName}")
- IncidentFilters + DataTable with IncidentRow
- Pagination controls
- Empty state: "No incidents recorded yet"

**`RCAIncidentDetail.tsx` (~180 lines)**
- Fetches incident via `useRCAIncident(incidentId)`
- Header card: instance, trigger, time, confidence badge, auto/manual
- ChainSummaryCard with the summary text
- QualityBanner (if quality data present)
- IncidentTimeline (main visualization)
- Alternative chain section (collapsed by default, expandable)
- RemediationHooks section
- Analysis metadata footer (collapsible)

**`RCACausalGraph.tsx` (~40 lines)**
- PageHeader: "RCA Causal Knowledge Graph"
- CausalGraphView component
- Brief description text explaining the graph

#### Task 8 — Route + navigation integration

**Modify `web/src/App.tsx`:**
- Import new pages
- Add routes:
  ```tsx
  <Route path="/rca/incidents" element={<RCAIncidents />} />
  <Route path="/rca/incidents/:incidentId" element={<RCAIncidentDetail />} />
  <Route path="/rca/graph" element={<RCACausalGraph />} />
  <Route path="/servers/:serverId/rca/incidents" element={<RCAIncidents />} />
  ```

**Modify `web/src/components/layout/Sidebar.tsx`:**
- Add "RCA Incidents" link in the fleet-level navigation section (use `GitBranch` or `Search` icon from lucide-react)
- Add "RCA Incidents" link under each server's expandable section (alongside Query Insights and Workload Report)

#### Task 9 — Alert integration

**Modify `web/src/components/alerts/AlertDetailPanel.tsx`:**
- Add a new section at the bottom: "Root Cause Analysis"
- If RCA incidents exist for this alert's metric + instance (query via `useInstanceRCAIncidents` filtered by metric), show inline summary + link
- If no incidents exist, show "Investigate" button that triggers `useRCAAnalyze()` mutation with the alert's metric and timestamp
- After analysis completes, navigate to the incident detail page

**Modify `web/src/components/alerts/AlertRow.tsx`** (or equivalent alert list component):
- Add a small "Investigate" icon button (🔍 or lucide `Search` icon) on each alert row
- Click: triggers analyze mutation → navigates to result

---

### Agent 2 — QA

**Owns:** Build verification only.

**Tasks (in order):**

1. `cd web && npm run build` — must succeed
2. `cd web && npm run typecheck` — must succeed
3. `cd web && npm run lint` — 0 errors (pre-existing warnings OK)
4. `go build ./cmd/pgpulse-server` — must succeed (backend unchanged)
5. `go test ./cmd/... ./internal/... -count=1` — all pass (backend unchanged)
6. `golangci-lint run ./cmd/... ./internal/...` — 0 issues
7. Verify zero Go files changed: `git diff --name-only | grep -v "^web/"` should be empty (or only docs)
8. Commit:

```bash
git add -A
git commit -m "feat(rca-ui): M14_02 — RCA incidents page, timeline visualization, alert integration

- Add RCA Incidents list page (fleet-wide + per-instance)
- Add Incident Detail page with vertical timeline visualization
- Add causal graph reference page (ECharts force-directed)
- Add confidence badges, quality banners, remediation hooks display
- Add 'Investigate' button on alert rows + inline RCA summary on alert detail
- Add RCA navigation to sidebar (fleet + per-server)
- React Query hooks for 5 RCA API endpoints
- TypeScript interfaces for all RCA types"
```

---

## Build Commands Reference

```bash
# Frontend (primary concern for M14_02)
cd web && npm run build && npm run typecheck && npm run lint && cd ..

# Backend (must be unchanged)
go build ./cmd/pgpulse-server
go test ./cmd/... ./internal/... -count=1
golangci-lint run ./cmd/... ./internal/...
```

---

## Watch-List

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
- [ ] `web/src/App.tsx`
- [ ] `web/src/components/layout/Sidebar.tsx`
- [ ] `web/src/components/alerts/AlertDetailPanel.tsx`
- [ ] `web/src/components/alerts/AlertRow.tsx`

**Unchanged (verify):**
- [ ] ALL `internal/**` Go files
- [ ] ALL `cmd/**` Go files
- [ ] `go.mod` / `go.sum`
