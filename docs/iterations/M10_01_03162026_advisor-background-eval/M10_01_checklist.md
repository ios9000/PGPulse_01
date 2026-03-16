# M10_01 — Checklist

**Iteration:** M10_01
**Milestone:** M10 — Advisor Auto-Population
**Date:** 2026-03-16
**Module:** advisor-background-eval

---

## Pre-Flight

### Step 1: Create iteration folder and copy docs

```bash
cd ~/Projects/PGPulse_01
mkdir -p docs/iterations/M10_01_03162026_advisor-background-eval
cp M10_01_requirements.md docs/iterations/M10_01_03162026_advisor-background-eval/M10_01_requirements.md
cp M10_01_design.md docs/iterations/M10_01_03162026_advisor-background-eval/M10_01_design.md
cp M10_01_team-prompt.md docs/iterations/M10_01_03162026_advisor-background-eval/M10_01_team-prompt.md
cp M10_01_checklist.md docs/iterations/M10_01_03162026_advisor-background-eval/M10_01_checklist.md
```

### Step 2: Update CLAUDE.md current iteration

Edit `.claude/CLAUDE.md` — update the "Current Iteration" section:

```
## Current Iteration
M10_01 — Advisor Auto-Population (background eval + create alert rule)
See: docs/iterations/M10_01_03162026_advisor-background-eval/
```

### Step 3: Commit planning docs

```bash
cd ~/Projects/PGPulse_01
git add docs/iterations/M10_01_03162026_advisor-background-eval/
git add .claude/CLAUDE.md
git commit -m "docs: M10_01 planning — advisor background eval + create alert rule"
git push
```

---

## Implementation

### Step 4: Spawn agents

```bash
cd ~/Projects/PGPulse_01
claude --model claude-opus-4-6
```

Paste the contents of `docs/iterations/M10_01_03162026_advisor-background-eval/M10_01_team-prompt.md` into Claude Code.

### Step 5: Watch list — expected files

**New files:**
- `internal/remediation/background.go` — BackgroundEvaluator worker
- `internal/remediation/background_test.go` — Worker tests
- `migrations/014_remediation_status.sql` — Schema update (if needed)

**Modified files (Backend Agent):**
- `internal/remediation/store.go` — ResolveStale method on interface
- `internal/remediation/pgstore.go` — ResolveStale + upsert + status filter
- `internal/remediation/nullstore.go` — ResolveStale no-op
- `internal/config/config.go` — RemediationConfig struct
- `internal/config/load.go` — Defaults
- `internal/config/config_test.go` — New config tests
- `internal/api/remediation.go` — Status filter query param
- `cmd/pgpulse-server/main.go` — BackgroundEvaluator wiring

**Modified files (Frontend Agent):**
- `web/src/types/models.ts` — Recommendation type update
- `web/src/pages/Advisor.tsx` — Auto-refresh, last evaluated
- `web/src/components/advisor/AdvisorRow.tsx` — "Create Alert Rule" button
- `web/src/components/advisor/AdvisorFilters.tsx` — Status filter
- `web/src/components/layout/Sidebar.tsx` — Badge count
- `web/src/hooks/useRecommendations.ts` — refetchInterval, status param

### Step 6: Build verification

```bash
cd ~/Projects/PGPulse_01/web
npm run build && npm run typecheck && npm run lint

cd ~/Projects/PGPulse_01
go build ./cmd/... ./internal/...
go test ./cmd/... ./internal/... -count=1
golangci-lint run ./cmd/... ./internal/...
```

### Step 7: Commit clean build

```bash
cd ~/Projects/PGPulse_01
git add -A
git commit -m "feat: M10_01 — advisor auto-population with background eval and create alert rule"
git push
```

---

## Deploy & Verify

### Step 8: Update demo config

Before deploying, the demo VM config needs `remediation.enabled: true`. SSH in and add the config:

```bash
ssh ml4dbs@185.159.111.139
sudo nano /opt/pgpulse/etc/pgpulse.yaml
```

Add to the config:

```yaml
remediation:
  enabled: true
  background_interval: 5m
  retention_days: 30
```

### Step 9: Cross-compile and deploy

```bash
cd ~/Projects/PGPulse_01
export GOOS=linux
export GOARCH=amd64
export CGO_ENABLED=0
go build -ldflags="-s -w" -o dist/pgpulse-server-linux-amd64 ./cmd/pgpulse-server
unset GOOS GOARCH CGO_ENABLED

scp dist/pgpulse-server-linux-amd64 ml4dbs@185.159.111.139:/home/ml4dbs/pgpulse-demo/
ssh ml4dbs@185.159.111.139 'sudo systemctl stop pgpulse && sudo cp /home/ml4dbs/pgpulse-demo/pgpulse-server-linux-amd64 /opt/pgpulse/bin/pgpulse-server && sudo chmod +x /opt/pgpulse/bin/pgpulse-server && sudo systemctl start pgpulse'
```

### Step 10: Verify on demo

Check logs for background evaluator startup:

```bash
ssh ml4dbs@185.159.111.139 'sudo journalctl -u pgpulse --no-pager -n 30'
```

Look for: `"remediation background evaluator started" interval=5m`

Open browser: `http://185.159.111.139:8989` (admin / pgpulse_admin)

**Verify checklist:**
- [ ] Check logs: background evaluator started with correct interval
- [ ] Wait 5 minutes for first evaluation cycle
- [ ] Check logs: `"background evaluation complete" instances=3 recommendations=N`
- [ ] Navigate to Advisor page → recommendations auto-populated (no manual Diagnose needed)
- [ ] Advisor shows "Last evaluated: X minutes ago"
- [ ] Status filter works: Active / Resolved / Acknowledged
- [ ] Sidebar "Advisor" shows badge with active count
- [ ] Click "Create Alert Rule" on a recommendation → RuleFormModal opens pre-filled
- [ ] Verify pre-filled values: metric key, threshold, severity match recommendation
- [ ] Edit and save the rule → navigates to Alert Rules page, new rule visible
- [ ] Start a chaos script → wait for next eval cycle → new recommendation appears
- [ ] Acknowledge a recommendation → status changes, filtered correctly
- [ ] Run Diagnose manually on a server → still works as before (no regression)

---

## Post-Implementation

### Step 11: Regenerate Codebase Digest

In Claude Code:

```
Read the entire codebase and regenerate docs/CODEBASE_DIGEST.md following the 7-section template in .claude/rules/codebase-digest.md
```

### Step 12: Wrap-up docs

Back in Claude.ai, produce:
- `M10_01_session-log.md`
- Handoff document for M11

```bash
cd ~/Projects/PGPulse_01
cp M10_01_session-log.md docs/iterations/M10_01_03162026_advisor-background-eval/M10_01_session-log.md
git add docs/iterations/M10_01_03162026_advisor-background-eval/M10_01_session-log.md
git add docs/CODEBASE_DIGEST.md
git add docs/roadmap.md
git add CHANGELOG.md
git commit -m "docs: M10_01 session-log, digest, roadmap, changelog"
git push
```

### Step 13: Upload updated CODEBASE_DIGEST.md to Project Knowledge

Upload `docs/CODEBASE_DIGEST.md` to Claude.ai Project Knowledge (replace existing).

---

## Updated Roadmap After M10_01

| Milestone | Scope | Status |
|-----------|-------|--------|
| ~~MW_01~~ | Windows executable + live mode | ✅ Done |
| ~~MW_01b~~ | Bugfixes (5 bugs) | ✅ Done |
| ~~MN_01~~ | Metric naming standardization | ✅ Done |
| ~~REM_01~~ | Rule-based remediation (3 sub-iterations) | ✅ Done |
| ~~M9~~ | Alert & Advisor Polish (metric keys + UI nav + cosmetic fixes) | ✅ Done |
| ~~M10~~ | Advisor Auto-Population (background eval + create alert rule) | ✅ Done (pending) |
| M11 | Competitive Enrichment (query insights, pganalyze-style) | 🔲 Next |
| M12 | Desktop App (Wails packaging) | 🔲 |
| M13 | Prometheus Exporter | 🔲 |
