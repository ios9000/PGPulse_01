# MN_01 — Metric Naming Standardization: Developer Checklist

**Date:** 2026-03-13
**Iteration:** MN_01

---

## Pre-Flight (Before Spawning Agents)

### 1. Copy docs to iteration folder

```bash
cd C:\Users\Archer\Projects\PGPulse_01

mkdir -p docs/iterations/MN_01_03132026_metric-naming

cp <download-path>/MN_01_requirements.md docs/iterations/MN_01_03132026_metric-naming/MN_01_requirements.md
cp <download-path>/MN_01_design.md docs/iterations/MN_01_03132026_metric-naming/MN_01_design.md
cp <download-path>/MN_01_team-prompt.md docs/iterations/MN_01_03132026_metric-naming/MN_01_team-prompt.md
cp <download-path>/MN_01_checklist.md docs/iterations/MN_01_03132026_metric-naming/MN_01_checklist.md
```

### 2. Update CLAUDE.md current iteration

Edit `.claude/CLAUDE.md`, set:

```
## Current Iteration
MN_01 — Metric Naming Standardization
See: docs/iterations/MN_01_03132026_metric-naming/
```

### 3. Update Project Knowledge if needed

- [ ] Re-upload `CODEBASE_DIGEST.md` (if not done after MW_01b — cache.hit_ratio_pct change)
- [ ] Confirm `PGPulse_Competitive_Research_Synthesis.md` is in Project Knowledge

### 4. Commit docs

```bash
git add docs/iterations/MN_01_03132026_metric-naming/
git add .claude/CLAUDE.md
git commit -m "docs(iteration): add MN_01 metric naming standardization design"
```

### 5. Pre-flight issue review

Before spawning agents, manually verify:

- [ ] `Base.point()` location — confirm the file path (likely `internal/collector/base.go`)
- [ ] Current highest migration number — run `ls migrations/ | sort | tail -1`
- [ ] `config.sample.yaml` ML section — confirm metric keys are present
- [ ] OSSQLCollector — confirm it uses `Base.point()` (grep for point() calls in os_sql.go)

---

## Agent Execution

### 6. Spawn agents

```bash
cd C:\Users\Archer\Projects\PGPulse_01
claude --model claude-opus-4-6
```

Paste contents of `MN_01_team-prompt.md`.

### 7. Watch-list of expected file changes

| Agent | Files to Watch |
|-------|---------------|
| Collector | `internal/collector/base.go`, `internal/collector/os_sql.go`, `internal/collector/os.go`, `internal/agent/osmetrics*.go` |
| API | `internal/alert/seed.go`, `internal/ml/*.go`, `internal/api/*.go`, `migrations/NNN_*.sql`, `config.sample.yaml` |
| Frontend | `web/src/lib/constants.ts`, `web/src/components/**/*.tsx`, `web/src/hooks/*.ts` |
| QA | Grep results, build output |

### 8. Build verification (after agents finish)

```bash
cd web && npm run build && npm run typecheck && npm run lint
cd .. && go build ./cmd/pgpulse-server ./cmd/pgpulse-agent
go test ./cmd/... ./internal/... -count=1
golangci-lint run ./cmd/... ./internal/...
```

### 9. Manual spot-check

```bash
# Verify no old prefixes remain
grep -rn '"pgpulse\.' internal/ cmd/ | grep -v '\.md' | grep -v 'migration'
grep -rn 'pgpulse\.' web/src/
grep -rn 'diskstat' internal/ cmd/ web/src/
# Verify cluster keys are UNCHANGED
grep -n '"cluster\.' internal/collector/cluster.go
```

### 10. Commit clean build

```bash
git add -A
git commit -m "feat: metric naming standardization — pgpulse.* → pg.*, os.diskstat.* → os.disk.*

Renames ~120 PG metric keys from pgpulse.* to pg.* prefix.
Fixes OS SQL path prefix inconsistency (pgpulse.os.* → os.*).
Renames os.diskstat.* hierarchy to os.disk.*.
Cluster metrics (cluster.*) unchanged per D200.
Includes TimescaleDB migration for existing data.
Affects: collectors, API, frontend, alert rules, ML config."
```

---

## Wrap-Up

### 11. Generate session-log

Ask Claude.ai to create `MN_01_session-log.md` covering:
- Goal achieved / not achieved
- Agent activity summary
- Key decisions made during implementation
- Files created/modified
- Test results
- Any issues discovered

### 12. Generate updated CODEBASE_DIGEST.md

In Claude Code:
```
Read the entire codebase and regenerate docs/CODEBASE_DIGEST.md
following the 7-section template in .claude/rules/codebase-digest.md.
Focus on Section 3 (Metric Keys) — all keys must show the new naming.
```

### 13. Create handoff document

Ask Claude.ai to create `HANDOFF_MN_01_to_next.md`.

### 14. Final commit and push

```bash
git add docs/iterations/MN_01_03132026_metric-naming/MN_01_session-log.md
git add docs/CODEBASE_DIGEST.md
git commit -m "docs: MN_01 session-log + updated codebase digest"
git push origin main
```

### 15. Update Project Knowledge

- [ ] Re-upload `CODEBASE_DIGEST.md` to Project Knowledge (Section 3 changed completely)

### 16. Deploy to demo VM (optional)

```bash
# Build Linux binary
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o dist/pgpulse-server-linux-amd64 ./cmd/pgpulse-server

# Deploy
scp dist/pgpulse-server-linux-amd64 ml4dbs@185.159.111.139:/home/ml4dbs/pgpulse-demo/
ssh ml4dbs@185.159.111.139 'sudo systemctl stop pgpulse && sudo cp /home/ml4dbs/pgpulse-demo/pgpulse-server-linux-amd64 /opt/pgpulse/bin/pgpulse-server && sudo chmod +x /opt/pgpulse/bin/pgpulse-server && sudo systemctl start pgpulse'

# Run migration on demo DB
ssh ml4dbs@185.159.111.139 'psql -U pgpulse_monitor -d pgpulse_storage -f /opt/pgpulse/migrations/NNN_metric_naming_standardization.sql'
```

### 17. Update roadmap

Add entry to `docs/roadmap.md` and `CHANGELOG.md`.
