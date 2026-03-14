# REM_01a — Developer Checklist

**Iteration:** REM_01a — Rule-Based Remediation Engine (Backend)
**Date:** 2026-03-13

---

## 1. Copy docs to iteration folder

```bash
cd C:\Users\Archer\Projects\PGPulse_01

mkdir -p docs/iterations/REM_01a_03132026_remediation-engine

cp /path/to/downloads/REM_01a_requirements.md docs/iterations/REM_01a_03132026_remediation-engine/requirements.md
cp /path/to/downloads/REM_01a_design.md docs/iterations/REM_01a_03132026_remediation-engine/design.md
cp /path/to/downloads/REM_01a_team-prompt.md docs/iterations/REM_01a_03132026_remediation-engine/team-prompt.md
cp /path/to/downloads/REM_01a_checklist.md docs/iterations/REM_01a_03132026_remediation-engine/checklist.md
```

## 2. Update CLAUDE.md current iteration

Edit `.claude/CLAUDE.md` and set:
```
## Current Iteration
REM_01a — Rule-Based Remediation Engine (Backend)
See: docs/iterations/REM_01a_03132026_remediation-engine/
```

## 3. Upload updated CODEBASE_DIGEST.md to Project Knowledge

⚠️ The handoff noted CODEBASE_DIGEST.md needs re-upload after MN_01 (metric keys changed).
Upload the latest `docs/CODEBASE_DIGEST.md` to Project Knowledge before spawning agents.

## 4. Pre-flight issue resolution

Before spawning agents, manually verify these in the codebase:

- [ ] Check `internal/alert/dispatcher.go` — find the `fire()` method, note its signature and where alert events are written to history
- [ ] Check `internal/alert/pgstore.go` — verify the alert_history table name and ID column type (BIGINT? SERIAL? UUID?)
- [ ] Check `internal/storage/migrations/` — verify no existing 013 migration
- [ ] Check `internal/collector/collector.go` — verify `MetricQuery` has Start/End fields
- [ ] Check `internal/api/server.go` — note the setter pattern (SetLiveMode, SetAuthMode) to follow

If any of these reveal issues (wrong column type, missing field, etc.), fix the design.md before spawning agents.

## 5. Commit docs

```bash
cd C:\Users\Archer\Projects\PGPulse_01

git add docs/iterations/REM_01a_03132026_remediation-engine/
git add .claude/CLAUDE.md
git commit -m "docs(REM_01a): requirements, design, team-prompt, checklist for remediation engine"
```

## 6. Spawn agents

```bash
cd C:\Users\Archer\Projects\PGPulse_01
claude --model claude-opus-4-6
```

Paste contents of `docs/iterations/REM_01a_03132026_remediation-engine/team-prompt.md`.

## 7. Watch-list of expected files

### New files (agent should create these):
- [ ] `internal/remediation/rule.go`
- [ ] `internal/remediation/engine.go`
- [ ] `internal/remediation/rules_pg.go`
- [ ] `internal/remediation/rules_os.go`
- [ ] `internal/remediation/store.go`
- [ ] `internal/remediation/pgstore.go`
- [ ] `internal/remediation/nullstore.go`
- [ ] `internal/remediation/adapter.go`
- [ ] `internal/remediation/metricsource.go`
- [ ] `internal/api/remediation.go`
- [ ] `internal/alert/remediation.go`
- [ ] `internal/storage/migrations/013_remediation.sql`

### Test files:
- [ ] `internal/remediation/engine_test.go`
- [ ] `internal/remediation/rules_test.go`
- [ ] `internal/remediation/pgstore_test.go`
- [ ] `internal/api/remediation_test.go`

### Modified files:
- [ ] `internal/alert/dispatcher.go` — RemediationProvider field + setter + call in fire()
- [ ] `internal/api/server.go` — remediation dependencies + routes
- [ ] `internal/api/alerts.go` — embed recommendations in alert responses
- [ ] `cmd/pgpulse-server/main.go` — wire remediation engine/store/adapter
- [ ] `internal/alert/template.go` — recommendations in email notifications

## 8. Build verification

```bash
cd C:\Users\Archer\Projects\PGPulse_01

cd web && npm run build && npm run lint && npm run typecheck
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
# Review changes — should see ~16 new files, ~5 modified files
git commit -m "feat(remediation): REM_01a — rule-based remediation engine with 25 rules, API, and alert integration"
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
git commit -m "docs: regenerate CODEBASE_DIGEST.md after REM_01a"
```

### 10c. Upload CODEBASE_DIGEST.md to Project Knowledge

Upload the new `docs/CODEBASE_DIGEST.md` to Claude.ai Project Knowledge.

### 10d. Create session-log

Return to Claude.ai and create `REM_01a_session-log.md` with:
- Goals achieved
- Agents used (API & Security, QA)
- Files created/modified
- Decisions made during implementation
- Test results
- Known issues
- Handoff notes for REM_01b

### 10e. Update roadmap + changelog

```bash
# Edit docs/roadmap.md — mark REM_01a complete
# Edit CHANGELOG.md — add REM_01a entry
git add docs/roadmap.md CHANGELOG.md docs/iterations/REM_01a_03132026_remediation-engine/session-log.md
git commit -m "docs: session-log, roadmap, changelog for REM_01a"
```

### 10f. Push

```bash
git push origin main
```

### 10g. Prepare REM_01b handoff

REM_01b scope (frontend + Advisor page):
- Frontend Agent: Advisor page route, recommendation display components, Diagnose button
- Frontend Agent: Alert detail panel with embedded recommendations
- QA Agent: Frontend build verification, component tests
- API endpoints are already in place from REM_01a
