# M8_02 Developer Checklist
## Auto-Capture Plans + Temporal Settings Diff + ML Anomaly Detection

**Date:** 2026-03-09
**Iteration folder:** `docs/iterations/M8_02_03092026_plan-capture-settings-ml/`

---

## Step 1 — Copy Docs to Iteration Folder

```bash
cd C:\Users\Archer\Projects\PGPulse_01

mkdir -p docs/iterations/M8_02_03092026_plan-capture-settings-ml

cp /path/to/downloads/M8_02_requirements.md docs/iterations/M8_02_03092026_plan-capture-settings-ml/
cp /path/to/downloads/M8_02_design.md       docs/iterations/M8_02_03092026_plan-capture-settings-ml/
cp /path/to/downloads/M8_02_team-prompt.md  docs/iterations/M8_02_03092026_plan-capture-settings-ml/
cp /path/to/downloads/M8_02_checklist.md    docs/iterations/M8_02_03092026_plan-capture-settings-ml/
```

Verify all four files are present:
```bash
ls docs/iterations/M8_02_03092026_plan-capture-settings-ml/
```

---

## Step 2 — Update CLAUDE.md Current Iteration

Edit `.claude/CLAUDE.md`, find the `## Current Iteration` section and set:

```markdown
## Current Iteration
M8_02 — Auto-Capture Plans + Temporal Settings Diff + ML Anomaly Detection
See: docs/iterations/M8_02_03092026_plan-capture-settings-ml/
```

---

## Step 3 — Commit Docs

```bash
git add docs/iterations/M8_02_03092026_plan-capture-settings-ml/
git add .claude/CLAUDE.md
git commit -m "docs: add M8_02 iteration docs (plan capture, settings diff, ML anomaly)"
git push
```

---

## Step 4 — Spawn Agents

```bash
cd C:\Users\Archer\Projects\PGPulse_01
claude --model claude-opus-4-6
```

Paste the full contents of `M8_02_team-prompt.md` into Claude Code.

---

## Step 5 — Watch-List of Expected Files

Confirm these files are created by the agents before proceeding to build:

**New files:**
- [ ] `internal/plans/capture.go`
- [ ] `internal/plans/store.go`
- [ ] `internal/plans/retention.go`
- [ ] `internal/plans/capture_test.go`
- [ ] `internal/plans/store_test.go`
- [ ] `internal/plans/capture_integration_test.go`
- [ ] `internal/settings/snapshot.go`
- [ ] `internal/settings/store.go`
- [ ] `internal/settings/diff.go`
- [ ] `internal/settings/diff_test.go`
- [ ] `internal/settings/store_test.go`
- [ ] `internal/ml/config.go`
- [ ] `internal/ml/baseline.go`
- [ ] `internal/ml/detector.go`
- [ ] `internal/ml/baseline_test.go`
- [ ] `internal/ml/detector_test.go`
- [ ] `internal/api/plans.go`
- [ ] `internal/api/settings.go`
- [ ] `internal/api/plans_test.go`
- [ ] `internal/api/settings_test.go`
- [ ] `migrations/007_plan_capture.sql`
- [ ] `migrations/008_settings_snapshots.sql`

**Modified files:**
- [ ] `internal/alert/rules.go` (ML default rules added)
- [ ] `cmd/pgpulse-server/main.go` (Detector bootstrap + new collectors wired)
- [ ] `configs/pgpulse.example.yml` (plan_capture, settings_snapshot, ml sections)
- [ ] `.claude/CLAUDE.md` (current iteration updated by Team Lead)

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
golangci-lint run ./internal/plans/... ./internal/settings/... ./internal/ml/... ./internal/api/...

# Security: verify no raw Sprintf in SQL (one allowed exception in runExplain — must have comment)
grep -rn "fmt.Sprintf.*SELECT\|fmt.Sprintf.*INSERT\|fmt.Sprintf.*UPDATE\|fmt.Sprintf.*DELETE" internal/
```

If `go build` fails → paste errors back to Claude Code. Agents fix. Re-run build.
If `go test` fails → paste failures back to Claude Code. Repeat until green.

---

## Step 7 — Commit Clean Build

```bash
git add .
git commit -m "feat(plans): add auto-capture query plans with duration/scheduled/manual triggers
feat(settings): add temporal pg_settings snapshots and diff API
feat(ml): add STL baseline anomaly detection with alert integration
test: add unit and integration tests for plans, settings, ml packages
chore: add migrations 007 (query_plans) and 008 (settings_snapshots)"
git push
```

---

## Step 8 — Wrap-Up

After successful build and commit, produce the following:

### 8a. Session Log
Create `docs/iterations/M8_02_03092026_plan-capture-settings-ml/session-log.md` covering:
- Agent activity summary (which agent created which files)
- Key decisions made during implementation
- Test results (count, pass/fail)
- Anything deferred or discovered as out of scope

### 8b. Handoff Document
Create `docs/iterations/HANDOFF_M8_02_to_M8_03.md` (self-contained) covering:
- What was just completed in M8_02
- Current state of `internal/ml/`, `internal/plans/`, `internal/settings/`
- Key interfaces (copy actual Go signatures from the committed code)
- Known issues or follow-up items
- Next task: M8_03 (forecast horizon / model persistence, TBD)

### 8c. Roadmap + Changelog
```bash
# Update docs/roadmap.md — mark M8_02 done with date
# Update docs/CHANGELOG.md — add M8_02 entry:
# ## M8_02 (2026-03-09)
# - Auto-capture query plans (duration threshold, manual API, scheduled top-N, plan hash diff)
# - Temporal pg_settings snapshots with diff API
# - STL-based ML anomaly detection with Z-score/IQR flagging and alert integration
```

### 8d. Final Push
```bash
git add docs/
git commit -m "docs: M8_02 session-log, handoff to M8_03, roadmap and changelog update"
git push
```
