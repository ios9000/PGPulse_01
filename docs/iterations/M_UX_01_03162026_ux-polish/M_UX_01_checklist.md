# M_UX_01 Checklist — Alert Detail + Duration Fix + UX Polish

**Iteration:** M_UX_01
**Date:** 2026-03-16

---

## Pre-Flight

- [ ] Create iteration folder: `docs/iterations/M_UX_01_03162026_ux-polish/`
- [ ] Copy team-prompt.md and checklist.md
- [ ] Update CLAUDE.md current iteration
- [ ] Pre-flight: `grep -oP "'[a-z]+\.[a-z._]+'" internal/alert/rules.go | sort -u` — get all metric keys for description registry
- [ ] Pre-flight: check AlertRow.tsx current click handler
- [ ] Pre-flight: check AlertEvent JSON response shape from /api/v1/alerts
- [ ] Pre-flight: check alert rule Description fields: `grep -A2 'Description:' internal/alert/rules.go | head -40`

---

## Agent Spawn

- [ ] `cd ~/Projects/PGPulse_01`
- [ ] `claude --model claude-opus-4-6`
- [ ] Paste team-prompt.md

---

## Agent 1 — Frontend

### Metric Descriptions
- [ ] `web/src/lib/metricDescriptions.ts` created
- [ ] Covers ALL metric keys from alert rules
- [ ] Each has: name, description, significance
- [ ] Relevant entries have pageLink

### Alert Detail Panel
- [ ] `web/src/components/alerts/AlertDetailPanel.tsx` created
- [ ] Shows: severity, metric name, value, threshold, instance
- [ ] Shows: metric description + significance from registry
- [ ] Shows: linked recommendations (if any)
- [ ] Shows: relevant page links
- [ ] Has close button, dark mode support

### Alert Row/Dashboard
- [ ] AlertRow click opens detail panel (not server redirect)
- [ ] AlertRow shows rule name (not empty)
- [ ] AlertsDashboard wires selectedAlert state + panel

### Duration Fix
- [ ] formatDurationHuman added to formatters.ts
- [ ] WorkloadReport period uses the new formatter
- [ ] Any other raw Go duration strings are formatted

### CSS Polish
- [ ] AlertsDashboard: max-w-7xl, consistent spacing
- [ ] AlertRow: left border by severity, better padding
- [ ] Advisor page: max-w-7xl, spacing
- [ ] AdvisorRow: consistent with AlertRow
- [ ] QueryInsights: empty state polish

---

## Agent 2 — Backend

- [ ] Alert rule descriptions in rules.go are DBA-actionable (not empty/generic)
- [ ] API responses include full AlertEvent data (recommendations, rule_name, etc.)
- [ ] Duration formatting checked on API side

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

- [ ] Deploy to demo VM
- [ ] Verify: click alert → detail panel opens (not server redirect)
- [ ] Verify: metric description shows in panel
- [ ] Verify: duration shows "4h 25m" not "4h24m36.79747s"
- [ ] Verify: report sections have consistent styling
- [ ] Verify: dark mode works on all changed components

---

## Wrap-Up

- [ ] Commit all changes
- [ ] Update HANDOFF document with UX fixes
- [ ] Push to master
- [ ] Regenerate CODEBASE_DIGEST.md
