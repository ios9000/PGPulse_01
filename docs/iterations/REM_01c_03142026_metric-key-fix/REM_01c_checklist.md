# REM_01c — Developer Checklist

**Iteration:** REM_01c — Remediation Rule Metric Key Fix (bugfix)
**Date:** 2026-03-14

---

## 1. Copy docs to iteration folder

```bash
cd C:\Users\Archer\Projects\PGPulse_01

mkdir -p docs/iterations/REM_01c_03142026_metric-key-fix

cp /path/to/downloads/REM_01c_requirements.md docs/iterations/REM_01c_03142026_metric-key-fix/requirements.md
cp /path/to/downloads/REM_01c_design.md docs/iterations/REM_01c_03142026_metric-key-fix/design.md
cp /path/to/downloads/REM_01c_team-prompt.md docs/iterations/REM_01c_03142026_metric-key-fix/team-prompt.md
cp /path/to/downloads/REM_01c_checklist.md docs/iterations/REM_01c_03142026_metric-key-fix/checklist.md
cp /path/to/downloads/REM_01c_metric_key_audit.md docs/iterations/REM_01c_03142026_metric-key-fix/audit.md
```

## 2. Update CLAUDE.md current iteration

Edit `.claude/CLAUDE.md` and set:
```
## Current Iteration
REM_01c — Remediation Rule Metric Key Fix (bugfix)
See: docs/iterations/REM_01c_03142026_metric-key-fix/
```

## 3. Commit docs

```bash
cd C:\Users\Archer\Projects\PGPulse_01

git add docs/iterations/REM_01c_03142026_metric-key-fix/
git add .claude/CLAUDE.md
git commit -m "docs(REM_01c): bugfix design for remediation metric key mismatches"
```

## 4. Spawn single agent

```bash
cd C:\Users\Archer\Projects\PGPulse_01
claude --model claude-opus-4-6
```

Paste contents of `docs/iterations/REM_01c_03142026_metric-key-fix/team-prompt.md`.

**No team spawn needed** — this is a single-agent bugfix session.

## 5. Watch-list of expected changes

### Modified files (agent should change these):
- [ ] `internal/remediation/rules_pg.go` — 9 metric key fixes + connection rule simplification
- [ ] `internal/remediation/rules_os.go` — `getOS()` + `isOSMetric()` helpers + 8 rule updates
- [ ] `internal/collector/server_info.go` — add `pg.server.wraparound_pct` metric
- [ ] `internal/remediation/rules_test.go` — update all affected test snapshots + new tests

### Possibly modified:
- [ ] `internal/collector/server_info_test.go` — wraparound metric test

### NOT modified (no changes expected):
- `internal/remediation/engine.go` — no engine changes
- `internal/remediation/store.go` — no store changes
- `internal/api/remediation.go` — no API changes
- Any frontend files — no frontend changes

## 6. Build verification

```bash
cd C:\Users\Archer\Projects\PGPulse_01

go build ./cmd/... ./internal/...
go test ./cmd/... ./internal/... -count=1
golangci-lint run ./cmd/... ./internal/...
cd web && npm run build && npm run typecheck && npm run lint
```

All must pass with zero errors.

## 7. Commit clean build

```bash
git add -A
git status
# Review — should see 4-5 modified files, 0 new files
git commit -m "fix(remediation): correct 13 metric key mismatches, add wraparound metric, dual OS prefix support"
```

## 8. Wrap-up

### 8a. Generate updated CODEBASE_DIGEST.md

In Claude Code:
```
Regenerate docs/CODEBASE_DIGEST.md per .claude/rules/codebase-digest.md
```

### 8b. Commit digest

```bash
git add docs/CODEBASE_DIGEST.md
git commit -m "docs: regenerate CODEBASE_DIGEST.md after REM_01c"
```

### 8c. Upload CODEBASE_DIGEST.md to Project Knowledge

Upload the new `docs/CODEBASE_DIGEST.md` to Claude.ai Project Knowledge.

### 8d. Quick session-log

```bash
cat > docs/iterations/REM_01c_03142026_metric-key-fix/session-log.md << 'EOF'
# Session Log: REM_01c — Metric Key Fix

**Date:** 2026-03-14
**Type:** Bugfix
**Agent:** Single agent (Opus 4.6)

## Changes
- Fixed 9 PG rule metric key mismatches in rules_pg.go
- Added getOS()/isOSMetric() helpers for dual OS prefix support in rules_os.go
- Updated all 8 OS rules to check both os.* and pg.os.* prefixes
- Added pg.server.wraparound_pct metric to ServerInfoCollector
- Updated all affected test cases in rules_test.go

## Result
All 25 rules now reference correct metric keys matching CODEBASE_DIGEST Section 3.
EOF

git add docs/iterations/REM_01c_03142026_metric-key-fix/session-log.md
git commit -m "docs: session-log for REM_01c"
```

### 8e. Push

```bash
git push origin main
```

### 8f. Deploy to demo VM and test Diagnose

```bash
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o dist/pgpulse-server-linux-amd64 ./cmd/pgpulse-server
scp dist/pgpulse-server-linux-amd64 ml4dbs@185.159.111.139:/home/ml4dbs/pgpulse-demo/
ssh ml4dbs@185.159.111.139 'sudo systemctl stop pgpulse && sudo cp /home/ml4dbs/pgpulse-demo/pgpulse-server-linux-amd64 /opt/pgpulse/bin/pgpulse-server && sudo chmod +x /opt/pgpulse/bin/pgpulse-server && sudo systemctl start pgpulse'
```

Then test:
1. Run chaos script: `sudo -u postgres PGPASSWORD=pgpulse_monitor_demo psql -h localhost -p 5434 -U pgpulse_monitor -d demo_app -c "SELECT pg_sleep(600);" &`
2. Wait 70 seconds
3. Click Diagnose on Staging (Chaos Target) — should now show "Long-running transactions detected"
