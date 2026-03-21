# PGPulse M14_02 — Developer Checklist

**Iteration:** M14_02 — RCA UI
**Date:** 2026-03-21

---

## Step 1 — Create iteration folder and copy docs

```bash
cd C:\Users\Archer\Projects\PGPulse_01

mkdir -p docs/iterations/M14_02_03212026_rca-ui

cp /path/to/downloaded/M14_02_design.md docs/iterations/M14_02_03212026_rca-ui/design.md
cp /path/to/downloaded/M14_02_team-prompt.md docs/iterations/M14_02_03212026_rca-ui/team-prompt.md
cp /path/to/downloaded/M14_02_checklist.md docs/iterations/M14_02_03212026_rca-ui/checklist.md
```

---

## Step 2 — Update CLAUDE.md current iteration

```
Current iteration: M14_02 — RCA UI (incidents page, timeline visualization, alert integration)
```

---

## Step 3 — Commit docs

```bash
git add docs/iterations/M14_02_03212026_rca-ui/
git add CLAUDE.md
git commit -m "docs(M14_02): design, team-prompt, checklist — RCA UI"
```

---

## Step 4 — Spawn agents

```bash
cd C:\Users\Archer\Projects\PGPulse_01
claude --model claude-opus-4-6
```

Prompt:
```
Read docs/iterations/M14_02_03212026_rca-ui/team-prompt.md and execute.
```

---

## Step 5 — Watch-list

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

**Unchanged:**
- [ ] ALL Go files

---

## Step 6 — Build verification

```bash
cd C:\Users\Archer\Projects\PGPulse_01

# Frontend
cd web && npm run build && npm run typecheck && npm run lint && cd ..

# Backend (must be unchanged)
go build ./cmd/pgpulse-server
go test ./cmd/... ./internal/... -count=1
golangci-lint run ./cmd/... ./internal/...

# Verify no Go files changed
git diff --name-only | grep -v "^web/" | grep -v "^docs/"
# Expected: empty (or only .claude/CLAUDE.md)
```

---

## Step 7 — Commit clean build

```bash
git add -A
git commit -m "feat(rca-ui): M14_02 — RCA incidents page, timeline visualization, alert integration

- Add RCA Incidents list page (fleet-wide + per-instance)
- Add Incident Detail page with vertical timeline visualization
- Add causal graph reference page (ECharts force-directed)
- Add confidence badges, quality banners, remediation hooks display
- Add Investigate button on alert rows + inline RCA on alert detail
- Add RCA navigation to sidebar (fleet + per-server)
- React Query hooks for 5 RCA API endpoints
- TypeScript interfaces for all RCA types"
```

---

## Step 8 — Deploy to demo VM and verify

```bash
# Rebuild with new frontend
cd web && npm run build && cd ..

export GOOS=linux
export GOARCH=amd64
export CGO_ENABLED=0
go build -ldflags="-s -w" -o dist/pgpulse-server-linux-amd64 ./cmd/pgpulse-server
unset GOOS GOARCH CGO_ENABLED

scp dist/pgpulse-server-linux-amd64 ml4dbs@185.159.111.139:/home/ml4dbs/pgpulse-demo/
ssh ml4dbs@185.159.111.139 'sudo systemctl stop pgpulse && sudo cp /home/ml4dbs/pgpulse-demo/pgpulse-server-linux-amd64 /opt/pgpulse/bin/pgpulse-server && sudo chmod +x /opt/pgpulse/bin/pgpulse-server && sudo systemctl start pgpulse'
```

Visual check at `http://185.159.111.139:8989`:
- [ ] "RCA Incidents" appears in sidebar
- [ ] RCA Incidents page loads, shows existing incidents (IDs 1-5 from testing)
- [ ] Click an incident → detail page with timeline (or "No causal chain" for low-confidence ones)
- [ ] Causal Graph page renders the 20-chain knowledge graph
- [ ] Alert detail panel shows "Investigate" button

---

## Step 9 — Wrap-up

### 9a — Session log + handoff

Produce:
- `docs/iterations/M14_02_03212026_rca-ui/M14_02_session-log.md`
- `docs/iterations/M14_02_03212026_rca-ui/HANDOFF_M14_02_to_M14_03.md`

### 9b — Regenerate codebase digest

```bash
claude --model claude-opus-4-6
# Prompt: Regenerate docs/CODEBASE_DIGEST.md per codebase-digest.md rules.
```

### 9c — Commit and push

```bash
git add docs/iterations/M14_02_03212026_rca-ui/M14_02_session-log.md
git add docs/iterations/M14_02_03212026_rca-ui/HANDOFF_M14_02_to_M14_03.md
git add docs/CODEBASE_DIGEST.md
git commit -m "docs(M14_02): session-log, handoff to M14_03, codebase digest"
git push origin master
```

### 9d — Upload CODEBASE_DIGEST.md to Project Knowledge

---

## Summary

| # | Step | Status |
|---|------|--------|
| 1 | Copy docs to iteration folder | ☐ |
| 2 | Update CLAUDE.md | ☐ |
| 3 | Commit docs | ☐ |
| 4 | Spawn agents (2-agent team) | ☐ |
| 5 | Watch-list check | ☐ |
| 6 | Build verification | ☐ |
| 7 | Commit clean build | ☐ |
| 8 | Deploy + visual verify | ☐ |
| 9 | Wrap-up: session-log + handoff + digest + push | ☐ |
