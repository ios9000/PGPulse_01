# M8_11 Developer Checklist — OS Metrics via PostgreSQL

---

## Step 1: Copy docs to iteration folder

```bash
cd C:\Users\Archer\Projects\PGPulse_01

# Create iteration folder
mkdir -p docs/iterations/M8_11_03102026_os-metrics-sql

# Copy the three deliverables (download from Claude.ai first)
cp M8_11_requirements.md docs/iterations/M8_11_03102026_os-metrics-sql/
cp M8_11_design.md docs/iterations/M8_11_03102026_os-metrics-sql/
cp M8_11_team-prompt.md docs/iterations/M8_11_03102026_os-metrics-sql/
cp M8_11_checklist.md docs/iterations/M8_11_03102026_os-metrics-sql/

# Copy codebase-digest-rules.md to rules directory
cp codebase-digest-rules.md .claude/rules/codebase-digest.md

# Copy updated strategy doc
cp PGPulse_Development_Strategy_v2.md docs/PGPulse_Development_Strategy_v2.md
```

## Step 2: Update CLAUDE.md current iteration

Edit `.claude/CLAUDE.md`:
```
## Current Iteration
M8_11 — OS Metrics via PostgreSQL (pg_read_file)
See: docs/iterations/M8_11_03102026_os-metrics-sql/
```

## Step 3: Update Project Knowledge (if needed)

In Claude.ai → Projects → PGPulse → Project Knowledge:
- Replace `PGPulse_Development_Strategy_v2.md` with updated version
- (CODEBASE_DIGEST.md will be uploaded at end of iteration — not yet)

## Step 4: Commit docs

```bash
git add docs/iterations/M8_11_03102026_os-metrics-sql/
git add .claude/rules/codebase-digest.md
git add .claude/CLAUDE.md
git add docs/PGPulse_Development_Strategy_v2.md
git commit -m "docs: M8_11 requirements, design, team-prompt + codebase-digest rules + strategy v2.4"
git push
```

## Step 5: Spawn agents

```bash
cd C:\Users\Archer\Projects\PGPulse_01
claude --model claude-opus-4-6
```

Paste contents of `M8_11_team-prompt.md` into Claude Code.

## Step 6: Watch-list of expected files

| File | Action | Agent |
|------|--------|-------|
| `internal/collector/os_sql.go` | Create (~350 lines) | Collector |
| `internal/collector/os_sql_test.go` | Create (~200 lines) | QA |
| `internal/collector/server_info.go` | Modify (+30 lines) | Collector |
| `internal/orchestrator/runner.go` | Modify (+15 lines) | Collector |
| `internal/config/config.go` | Modify (+10 lines) | Collector |
| `configs/pgpulse.example.yml` | Modify (+5 lines) | Collector |

## Step 7: Build verification

```bash
cd web && npm run build && npm run lint && npm run typecheck
cd .. && go build ./cmd/pgpulse-server && go test ./cmd/... ./internal/...
golangci-lint run
```

All must pass. If errors: paste into Claude Code for fixes, re-verify.

## Step 8: Commit clean build

```bash
git add .
git commit -m "feat(collector): add OSSQLCollector — OS metrics via pg_read_file('/proc/*')"
git push
```

## Step 9: Generate Codebase Digest (FIRST EVER)

In Claude Code:
```
Read the entire codebase and generate docs/CODEBASE_DIGEST.md
following the 7-section template in .claude/rules/codebase-digest.md
```

Review the output (sanity check — is the file inventory complete? are metric keys correct?).

```bash
git add docs/CODEBASE_DIGEST.md
git commit -m "docs: generate first CODEBASE_DIGEST.md"
git push
```

## Step 10: Wrap-up

- [ ] Return to Claude.ai planning chat
- [ ] Produce M8_11_session-log.md
- [ ] Update docs/roadmap.md (M8_11 ✅)
- [ ] Update CHANGELOG.md
- [ ] Create handoff document for next iteration
- [ ] Upload CODEBASE_DIGEST.md to Project Knowledge

```bash
git add docs/
git commit -m "docs: M8_11 session-log, roadmap, changelog"
git push
```

## Step 11: Deploy to demo VM (optional)

```bash
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o build/pgpulse-server ./cmd/pgpulse-server
scp build/pgpulse-server ml4dbs@185.159.111.139:/home/ml4dbs/pgpulse-demo/

# On VM:
sudo systemctl stop pgpulse
sudo cp /home/ml4dbs/pgpulse-demo/pgpulse-server /opt/pgpulse/bin/pgpulse-server
sudo chmod +x /opt/pgpulse/bin/pgpulse-server

# Grant pg_read_server_files on all monitored instances:
sudo -u postgres psql -p 5432 -c "GRANT pg_read_server_files TO pgpulse_monitor;"
sudo -u postgres psql -p 5433 -c "GRANT pg_read_server_files TO pgpulse_monitor;"
sudo -u postgres psql -p 5434 -c "GRANT pg_read_server_files TO pgpulse_monitor;"

sudo systemctl start pgpulse
sudo journalctl -u pgpulse -f  # verify no errors, OS metrics flowing
```
