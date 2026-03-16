# M11_02 Checklist — Query Insights UI + Workload Report + HTML Export

**Iteration:** M11_02
**Date:** 2026-03-16

---

## Pre-Flight

- [ ] Verify M11_01 build is clean and deployed
- [ ] Verify at least 1 snapshot exists on demo VM (`curl .../snapshots | jq .total`)
- [ ] Create iteration folder: `docs/iterations/M11_02_03162026_query-insights-ui/`
- [ ] Copy requirements.md, design.md, team-prompt.md, checklist.md to iteration folder
- [ ] Update `.claude/CLAUDE.md` current iteration to M11_02
- [ ] Commit docs
- [ ] Pre-flight: check Sidebar.tsx for exact location of instance-scoped nav links
- [ ] Pre-flight: check App.tsx for exact route registration pattern
- [ ] Pre-flight: check if lucide-react icons are available (for sidebar icons)
- [ ] Pre-flight: check existing api.ts for how query params are passed
- [ ] Pre-flight: check if `go:embed` for templates dir needs special handling

---

## Agent Spawn

- [ ] `cd ~/Projects/PGPulse_01`
- [ ] `claude --model claude-opus-4-6`
- [ ] Paste team-prompt.md content

---

## Agent 1 — Frontend Specialist

### Types + Hooks
- [ ] Types added to `web/src/types/models.ts` (appended, not replaced)
- [ ] `web/src/hooks/useSnapshots.ts` created with 6 hooks
- [ ] useLatestDiff has 30s refetchInterval
- [ ] useManualCapture invalidates snapshot queries on success

### Components (web/src/components/snapshots/)
- [ ] `SnapshotSelector.tsx` created — from/to dropdowns with "Latest" option
- [ ] `StatsResetBanner.tsx` created — conditional amber warning
- [ ] `QueryText.tsx` created — truncated monospace with copy button
- [ ] `DiffTable.tsx` created — sortable, clickable rows, pagination
- [ ] `QueryDetailPanel.tsx` created — 4 mini ECharts, query detail
- [ ] `ReportSummaryCard.tsx` created — summary metrics grid
- [ ] `ReportSection.tsx` created — collapsible with count badge

### Pages
- [ ] `web/src/pages/QueryInsights.tsx` created
- [ ] QueryInsights: snapshot selector works
- [ ] QueryInsights: latest-diff loads on mount
- [ ] QueryInsights: row expand shows QueryDetailPanel
- [ ] QueryInsights: empty state when no snapshots
- [ ] QueryInsights: Capture Now button (permission-gated)
- [ ] `web/src/pages/WorkloadReport.tsx` created
- [ ] WorkloadReport: snapshot selector works
- [ ] WorkloadReport: report loads with summary + sections
- [ ] WorkloadReport: Export HTML button links to backend endpoint
- [ ] WorkloadReport: sections are collapsible

### Navigation
- [ ] `web/src/App.tsx` — 2 routes added
- [ ] `web/src/components/layout/Sidebar.tsx` — 2 links added
- [ ] Links visible when on instance pages
- [ ] Links navigate correctly

---

## Agent 2 — Full-Stack Specialist

- [ ] `internal/api/templates/workload_report.html` created
- [ ] Template is standalone HTML with inline CSS
- [ ] Template renders all sections (summary, 5 top-by, new, evicted)
- [ ] `internal/api/handler_report_html.go` created
- [ ] Handler uses `go:embed` for template
- [ ] Handler registers template FuncMap (formatNumber, formatDuration, truncate, add)
- [ ] Content-Disposition header set for download (unless ?inline=true)
- [ ] `internal/api/server.go` — HTML export route added

---

## Build Verification

- [ ] `cd web && npm run build` — PASS
- [ ] `npm run lint` — PASS
- [ ] `npm run typecheck` — PASS
- [ ] `cd .. && go build ./cmd/pgpulse-server` — PASS
- [ ] `go test ./cmd/... ./internal/... -count=1` — PASS
- [ ] `golangci-lint run ./cmd/... ./internal/...` — PASS

---

## Post-Build

- [ ] All agents committed
- [ ] Verify `go:embed` for template works (template included in binary)
- [ ] Count new files: should be ~12 new
- [ ] Count new API endpoints: 1 new (HTML export), total ~55

---

## Wrap-Up

- [ ] Regenerate `docs/CODEBASE_DIGEST.md`
- [ ] Write `M11_02_session-log.md`
- [ ] Write `HANDOFF_M11_02_to_M12.md`
- [ ] Update `docs/roadmap.md` and `CHANGELOG.md`
- [ ] Commit all docs
- [ ] Upload new CODEBASE_DIGEST.md to Project Knowledge
- [ ] Cross-compile and deploy to demo VM
- [ ] Verify on demo VM:
  - [ ] Query Insights page loads at `/servers/production-primary/query-insights`
  - [ ] Latest diff table shows queries (need 2+ snapshots)
  - [ ] Workload Report page loads
  - [ ] Export HTML downloads a file
  - [ ] Sidebar links visible and functional

---

## Watch-List (Expected Files)

```
NEW:
  web/src/hooks/useSnapshots.ts
  web/src/pages/QueryInsights.tsx
  web/src/pages/WorkloadReport.tsx
  web/src/components/snapshots/SnapshotSelector.tsx
  web/src/components/snapshots/DiffTable.tsx
  web/src/components/snapshots/QueryDetailPanel.tsx
  web/src/components/snapshots/StatsResetBanner.tsx
  web/src/components/snapshots/QueryText.tsx
  web/src/components/snapshots/ReportSummaryCard.tsx
  web/src/components/snapshots/ReportSection.tsx
  internal/api/handler_report_html.go
  internal/api/templates/workload_report.html

MODIFIED:
  web/src/types/models.ts
  web/src/App.tsx
  web/src/components/layout/Sidebar.tsx
  internal/api/server.go
```
