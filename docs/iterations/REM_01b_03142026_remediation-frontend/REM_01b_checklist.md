# REM_01b — Developer Checklist

**Iteration:** REM_01b — Remediation Frontend + Backend Gaps
**Date:** 2026-03-14

---

## 1. Copy docs to iteration folder

```bash
cd C:\Users\Archer\Projects\PGPulse_01

mkdir -p docs/iterations/REM_01b_03142026_remediation-frontend

cp /path/to/downloads/REM_01b_requirements.md docs/iterations/REM_01b_03142026_remediation-frontend/requirements.md
cp /path/to/downloads/REM_01b_design.md docs/iterations/REM_01b_03142026_remediation-frontend/design.md
cp /path/to/downloads/REM_01b_team-prompt.md docs/iterations/REM_01b_03142026_remediation-frontend/team-prompt.md
cp /path/to/downloads/REM_01b_checklist.md docs/iterations/REM_01b_03142026_remediation-frontend/checklist.md
```

## 2. Update CLAUDE.md current iteration

Edit `.claude/CLAUDE.md` and set:
```
## Current Iteration
REM_01b — Remediation Frontend + Backend Gaps
See: docs/iterations/REM_01b_03142026_remediation-frontend/
```

## 3. Upload updated CODEBASE_DIGEST.md to Project Knowledge

Upload the REM_01a-updated `docs/CODEBASE_DIGEST.md` if not already done.

## 4. Pre-flight issue resolution

Before spawning agents, manually check these files and note findings:

- [ ] `internal/api/alerts.go` — What struct is returned for active alerts and alert history? Named type or inline? What is the alert event ID field name and type?
- [ ] `internal/alert/dispatcher.go` — Where is `runRemediation()` called? What does it store? How does the notification payload flow from fire() → notify()?
- [ ] `internal/alert/template.go` — How is the HTML email built? What data struct is passed to the renderer?
- [ ] `web/src/components/alerts/AlertRow.tsx` — Does it already have expandable rows? What's the expand/collapse pattern?
- [ ] `web/src/components/server/HeaderCard.tsx` — What props does it accept? What layout structure?
- [ ] `web/src/components/layout/Sidebar.tsx` — How are nav items defined (array? inline JSX?)
- [ ] `web/src/components/server/InstanceAlerts.tsx` — Does it show expandable individual alerts or just a summary?
- [ ] Verify `Lightbulb` exists in lucide-react: `cd web && npx ts-node -e "import { Lightbulb } from 'lucide-react'; console.log('OK')"` (or just check node_modules/lucide-react/dist/esm/icons/)

If any findings conflict with the design doc, update the design doc before spawning.

## 5. Commit docs

```bash
cd C:\Users\Archer\Projects\PGPulse_01

git add docs/iterations/REM_01b_03142026_remediation-frontend/
git add .claude/CLAUDE.md
git commit -m "docs(REM_01b): requirements, design, team-prompt, checklist for remediation frontend"
```

## 6. Spawn agents

```bash
cd C:\Users\Archer\Projects\PGPulse_01
claude --model claude-opus-4-6
```

Paste contents of `docs/iterations/REM_01b_03142026_remediation-frontend/team-prompt.md`.

**OOM prevention:** Close Chrome/Claude.ai tabs before spawning. The OOM in REM_01a was caused by browser memory pressure during agent team execution.

## 7. Watch-list of expected files

### New files (agent should create these):
- [ ] `web/src/pages/Advisor.tsx`
- [ ] `web/src/components/advisor/AdvisorFilters.tsx`
- [ ] `web/src/components/advisor/AdvisorRow.tsx`
- [ ] `web/src/components/advisor/PriorityBadge.tsx`
- [ ] `web/src/components/server/DiagnosePanel.tsx`
- [ ] `web/src/hooks/useRecommendations.ts`
- [ ] `internal/api/remediation_test.go`
- [ ] `internal/remediation/pgstore_test.go`

### Modified files:
- [ ] `web/src/types/models.ts` — Recommendation types + AlertEvent extension
- [ ] `web/src/App.tsx` — /advisor route
- [ ] `web/src/components/layout/Sidebar.tsx` — Advisor nav item
- [ ] `web/src/components/server/HeaderCard.tsx` — Diagnose button
- [ ] `web/src/pages/ServerDetail.tsx` — DiagnosePanel wiring
- [ ] `web/src/components/alerts/AlertRow.tsx` — Inline recommendations
- [ ] `internal/api/alerts.go` — Recommendation enrichment
- [ ] `internal/alert/template.go` — Email recommendations section
- [ ] `internal/alert/dispatcher.go` — Notification payload extension (if needed)

## 8. Build verification

```bash
cd C:\Users\Archer\Projects\PGPulse_01

cd web && npm run build && npm run typecheck && npm run lint
cd ..
go build ./cmd/... ./internal/...
go test ./cmd/... ./internal/... -count=1
golangci-lint run ./cmd/... ./internal/...
```

All must pass with zero errors.

## 9. Commit clean build

```bash
git add -A
git status
# Review changes — should see ~8 new files, ~9 modified files
git commit -m "feat(remediation): REM_01b — Advisor page, Diagnose button, alert recommendations, backend gaps"
```

## 10. Wrap-up

### 10a. Generate updated CODEBASE_DIGEST.md

In Claude Code:
```
Regenerate docs/CODEBASE_DIGEST.md per .claude/rules/codebase-digest.md
```

### 10b. Commit digest

```bash
git add docs/CODEBASE_DIGEST.md
git commit -m "docs: regenerate CODEBASE_DIGEST.md after REM_01b"
```

### 10c. Upload CODEBASE_DIGEST.md to Project Knowledge

Upload the new `docs/CODEBASE_DIGEST.md` to Claude.ai Project Knowledge.

### 10d. Create session-log

Return to Claude.ai and create `REM_01b_session-log.md` with:
- Goals achieved
- Agents used (API & Security, Frontend, QA)
- Files created/modified
- Decisions made during implementation
- Test results
- Known issues
- What's next (Prometheus exporter? Workload snapshot reports?)

### 10e. Update roadmap + changelog + handoff

```bash
# Edit docs/roadmap.md — mark REM_01 (both a+b) complete
# Edit CHANGELOG.md — add REM_01b entry
git add docs/roadmap.md CHANGELOG.md docs/iterations/REM_01b_03142026_remediation-frontend/session-log.md
git commit -m "docs: session-log, roadmap, changelog for REM_01b"
```

### 10f. Create handoff document

```bash
# Create docs/iterations/HANDOFF_REM_01_to_next.md
# Include: all REM_01 decisions, new interfaces, new API endpoints, new frontend routes
git add docs/iterations/HANDOFF_REM_01_to_next.md
git commit -m "docs: handoff from REM_01 to next iteration"
```

### 10g. Push

```bash
git push origin main
```

### 10h. Deploy to demo VM (optional)

```bash
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o dist/pgpulse-server-linux-amd64 ./cmd/pgpulse-server
scp dist/pgpulse-server-linux-amd64 ml4dbs@185.159.111.139:/home/ml4dbs/pgpulse-demo/
ssh ml4dbs@185.159.111.139 'sudo systemctl stop pgpulse && sudo cp /home/ml4dbs/pgpulse-demo/pgpulse-server-linux-amd64 /opt/pgpulse/bin/pgpulse-server && sudo chmod +x /opt/pgpulse/bin/pgpulse-server && sudo systemctl start pgpulse'

# Run REM_01a migration if not already done
ssh ml4dbs@185.159.111.139 'psql -U pgpulse_monitor -d pgpulse_storage -f /opt/pgpulse/migrations/013_remediation.sql'
```
