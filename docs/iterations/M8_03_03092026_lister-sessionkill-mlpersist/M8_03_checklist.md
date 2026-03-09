# M8_03 Developer Checklist
## Instance Lister Fix + Session Kill API + ML Model Persistence

**Date:** 2026-03-09
**Iteration folder:** `docs/iterations/M8_03_03092026_lister-sessionkill-mlpersist/`

---

## Step 1 — Copy Docs to Iteration Folder

```bash
cd C:\Users\Archer\Projects\PGPulse_01

mkdir -p docs/iterations/M8_03_03092026_lister-sessionkill-mlpersist

cp /path/to/downloads/M8_03_requirements.md docs/iterations/M8_03_03092026_lister-sessionkill-mlpersist/
cp /path/to/downloads/M8_03_design.md       docs/iterations/M8_03_03092026_lister-sessionkill-mlpersist/
cp /path/to/downloads/M8_03_team-prompt.md  docs/iterations/M8_03_03092026_lister-sessionkill-mlpersist/
cp /path/to/downloads/M8_03_checklist.md    docs/iterations/M8_03_03092026_lister-sessionkill-mlpersist/
```

Verify:
```bash
ls docs/iterations/M8_03_03092026_lister-sessionkill-mlpersist/
```

Expected: `M8_03_requirements.md  M8_03_design.md  M8_03_team-prompt.md  M8_03_checklist.md`

---

## Step 2 — Update CLAUDE.md Current Iteration

Edit `.claude/CLAUDE.md`, find `## Current Iteration` and set:

```markdown
## Current Iteration
M8_03 — Instance Lister Fix + Session Kill API + ML Model Persistence
See: docs/iterations/M8_03_03092026_lister-sessionkill-mlpersist/
```

---

## Step 3 — Commit Docs

```bash
git add docs/iterations/M8_03_03092026_lister-sessionkill-mlpersist/
git add .claude/CLAUDE.md
git commit -m "docs: add M8_03 iteration docs (lister fix, session kill, ML persistence)"
git push
```

---

## Step 4 — Spawn Agents

```bash
cd C:\Users\Archer\Projects\PGPulse_01
claude --model claude-opus-4-6
```

Paste the full contents of `M8_03_team-prompt.md` into Claude Code.

---

## Step 5 — Watch-List of Expected Files

Confirm these files exist before running the build:

**New files:**
- [ ] `internal/ml/lister.go`
- [ ] `internal/ml/lister_test.go`
- [ ] `internal/ml/persistence.go`
- [ ] `internal/ml/persistence_test.go`
- [ ] `internal/api/session_actions.go`
- [ ] `internal/api/session_actions_test.go`
- [ ] `migrations/010_ml_baseline_snapshots.sql`

**Modified files:**
- [ ] `internal/ml/baseline.go` (`Snapshot()` + `LoadFromSnapshot()` added)
- [ ] `internal/ml/detector.go` (`persist` field, updated Bootstrap + Evaluate, updated NewDetector signature)
- [ ] `internal/api/server.go` (session cancel/terminate routes registered)
- [ ] `cmd/pgpulse-server/main.go` (`DBInstanceLister` + persistence wiring)
- [ ] `configs/pgpulse.example.yml` (`ml.persistence` section added)
- [ ] `.claude/CLAUDE.md` (current iteration updated by Team Lead)

**Verify no leftover `configInstanceLister`:**
```bash
grep -rn "configInstanceLister" cmd/ internal/
```
Expected: 0 results.

---

## Step 6 — Build Verification

```bash
cd C:\Users\Archer\Projects\PGPulse_01

# Frontend
cd web && npm run build && npm run typecheck && npm run lint
cd ..

# Backend
go mod tidy
go build ./cmd/pgpulse-server
go test ./cmd/... ./internal/...

# Lint
golangci-lint run

# Verify no configInstanceLister remains
grep -rn "configInstanceLister" cmd/ internal/

# Verify session kill comment present
grep -n "hardcoded function name" internal/api/session_actions.go
```

If `go build` fails → paste errors to Claude Code. If `go test` fails → paste failures. Repeat until green.

---

## Step 7 — Commit Clean Build

```bash
git add .
git commit -m "feat(ml): replace configInstanceLister with DBInstanceLister
feat(ml): add STLBaseline snapshot/restore and model persistence to storage DB
feat(api): add session cancel and terminate endpoints (admin only)
chore: migration 010 (ml_baseline_snapshots)"
git push
```

---

## Step 8 — Wrap-Up

### 8a. Session Log
Create `docs/iterations/M8_03_03092026_lister-sessionkill-mlpersist/M8_03_session-log.md`

### 8b. Handoff Document
Create `docs/iterations/HANDOFF_M8_03_to_M8_04.md`

Cover in handoff:
- What was completed in M8_03
- Current state of `internal/ml/` (lister, persistence, detector Bootstrap sequence)
- `NewDetector` signature (now takes 5 params including persist)
- Session kill API routes and safety guards
- Next task: M8_04 — forecast horizon (STL-based next-N prediction, alert integration, API endpoint)

### 8c. Update Roadmap + Changelog

```bash
# docs/roadmap.md — mark M8_03 done with today's date
# docs/CHANGELOG.md — add:
# ## M8_03 (2026-03-09)
# - DBInstanceLister: ML baseline now covers instances added via API after startup
# - Session cancel/terminate API (admin only, with superuser and own-PID guards)
# - ML model persistence: STLBaseline state saved to DB after each Evaluate cycle;
#   Bootstrap loads from DB first, falls back to TimescaleDB replay for stale/missing state
```

### 8d. Final Push

```bash
git add docs/
git commit -m "docs: M8_03 session-log, handoff to M8_04, roadmap and changelog"
git push
```
