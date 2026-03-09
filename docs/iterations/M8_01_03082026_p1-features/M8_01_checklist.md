# M8_01 Developer Checklist
**Iteration:** M8_01 — P1 Features: Session Kill + On-Demand Query Plans + Cross-Instance Settings Diff
**Date:** 2026-03-08

Complete every step in order before moving to the next.

---

## Step 1 — Copy docs to iteration folder

```bash
mkdir -p docs/iterations/M8_01_03082026_p1-features
cp M8_01_requirements.md docs/iterations/M8_01_03082026_p1-features/requirements.md
cp M8_01_design.md docs/iterations/M8_01_03082026_p1-features/design.md
cp M8_01_team-prompt.md docs/iterations/M8_01_03082026_p1-features/team-prompt.md
cp M8_01_checklist.md docs/iterations/M8_01_03082026_p1-features/checklist.md
```

---

## Step 2 — Update CLAUDE.md current iteration section

Open `.claude/CLAUDE.md` and update the `## Current Iteration` section:

```
## Current Iteration
M8_01 — P1 Features (Session Kill, Query Plans, Settings Diff)
See: docs/iterations/M8_01_03082026_p1-features/
```

---

## Step 3 — Update Project Knowledge in Claude.ai (if needed)

Check whether any of these have changed since last upload. If yes, re-upload:

- [ ] `PGPulse_Development_Strategy_v2.md`
- [ ] `PGAM_FEATURE_AUDIT.md`
- [ ] `pgpulse_architecture.docx`

---

## Step 4 — Commit docs before spawning agents

```bash
cd C:\Users\Archer\Projects\PGPulse_01
git add docs/iterations/M8_01_03082026_p1-features/
git add .claude/CLAUDE.md
git commit -m "docs: add M8_01 iteration docs and update CLAUDE.md"
git push
```

---

## Step 5 — Spawn agent team in Claude Code

```bash
cd C:\Users\Archer\Projects\PGPulse_01
claude --model claude-opus-4-6
```

Paste the contents of `docs/iterations/M8_01_03082026_p1-features/team-prompt.md`.

---

## Step 6 — Watch-list: expected files agents will produce

### New files
- [ ] `internal/api/sessions.go`
- [ ] `internal/api/plans.go`
- [ ] `internal/api/settings_diff.go`
- [ ] `migrations/006_session_audit_log.sql`
- [ ] `internal/api/sessions_test.go`
- [ ] `internal/api/plans_test.go`
- [ ] `internal/api/settings_diff_test.go`
- [ ] `web/src/components/SessionKillButtons.tsx`
- [ ] `web/src/pages/QueryPlanViewer.tsx`
- [ ] `web/src/pages/SettingsDiff.tsx`

### Modified files
- [ ] `internal/api/server.go` — 4 new routes
- [ ] `web/src/pages/ServerDetail.tsx` — kill buttons + explain link
- [ ] `web/src/App.tsx` — 2 new routes
- [ ] `web/src/types/models.ts` — 8 new types
- [ ] `web/src/components/Navigation.tsx` — Settings Diff link

---

## Step 7 — Build verification

Run in this exact order. Fix any errors before proceeding to step 8.

```bash
cd C:\Users\Archer\Projects\PGPulse_01

# Frontend
cd web
npm run build
npm run lint
npm run typecheck
cd ..

# Backend
go build ./cmd/pgpulse-server
go vet ./cmd/... ./internal/...
go test ./cmd/... ./internal/...
golangci-lint run
```

**NEVER run** `go test ./...` — hits web/node_modules/ and fails.

If anything fails → paste errors back into Claude Code → agents fix → re-run from top of this step.

---

## Step 8 — Commit clean build

```bash
git add .
git commit -m "feat(api): session kill, on-demand explain, settings diff (M8_01)"
git push
```

---

## Step 9 — Wrap-up (back in Claude.ai)

- [ ] Share results with Claude.ai: files created, test summary, any agent decisions
- [ ] Ask Claude.ai to produce `session-log.md` → save to `docs/iterations/M8_01_03082026_p1-features/session-log.md`
- [ ] Update `docs/roadmap.md` — mark M8_01 done, update dates
- [ ] Update `CHANGELOG.md` — add session kill, EXPLAIN, settings diff entries
- [ ] Create `HANDOFF_M8_01_to_M8_02.md`

```bash
git add docs/
git commit -m "docs: M8_01 session-log, changelog, handoff to M8_02"
git push
```
