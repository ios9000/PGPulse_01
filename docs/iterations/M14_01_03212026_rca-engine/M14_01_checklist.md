# PGPulse M14_01 — Developer Checklist

**Iteration:** M14_01 — Causal Graph + Reliable Correlation Engine
**Date:** 2026-03-21

---

## Step 1 — Create iteration folder and copy docs

```bash
cd C:\Users\Archer\Projects\PGPulse_01

mkdir -p docs/iterations/M14_01_03212026_rca-engine
```

Copy downloaded files:

```bash
cp /path/to/downloaded/M14_requirements_v2.md docs/iterations/M14_01_03212026_rca-engine/requirements.md
cp /path/to/downloaded/M14_01_design.md docs/iterations/M14_01_03212026_rca-engine/design.md
cp /path/to/downloaded/M14_01_team-prompt.md docs/iterations/M14_01_03212026_rca-engine/team-prompt.md
cp /path/to/downloaded/M14_01_checklist.md docs/iterations/M14_01_03212026_rca-engine/checklist.md
```

---

## Step 2 — Update CLAUDE.md current iteration

Edit `CLAUDE.md` and update:

```
Current iteration: M14_01 — RCA Engine (Causal Graph + Reliable Correlation Engine)
```

---

## Step 3 — Update Project Knowledge (if needed)

Upload latest `docs/CODEBASE_DIGEST.md` to Project Knowledge if it wasn't re-uploaded after M12_02.

---

## Step 4 — Pre-flight grep

This is the most complex feature we've built. Run targeted greps to verify structural assumptions before spawning agents.

```bash
cd ~/Projects/PGPulse_01

# 1. Verify OnAlert hook exists on dispatcher (M12_02 addition)
grep -n "OnAlert" internal/alert/dispatcher.go
# Expected: func (d *AlertDispatcher) OnAlert(fn func(AlertEvent))

# 2. Verify MetricStore.Query interface
grep -n "Query(ctx" internal/collector/collector.go
# Expected: Query(ctx context.Context, query MetricQuery) ([]MetricPoint, error)

# 3. Verify MetricQuery struct has Start/End fields
grep -A5 "type MetricQuery struct" internal/collector/collector.go

# 4. Verify ML Detector exists and has detection method
grep -n "func.*Detect" internal/ml/detector.go

# 5. Verify latest migration number
ls -la migrations/*.sql | tail -3
# Expected: latest is 015_*

# 6. Verify api/server.go has Options or constructor pattern
grep -n "type.*Options\|func New" internal/api/server.go | head -5

# 7. Verify AlertEvent has needed fields for trigger
grep -A10 "type AlertEvent struct" internal/alert/alert.go
# Need: InstanceID, Metric (or RuleID+Metric), Value, Severity, FiredAt

# 8. Verify no existing internal/rca/ directory
ls internal/rca/ 2>/dev/null
# Expected: no such file or directory

# 9. Check key metric keys exist in collectors
grep -r "pg.replication.lag.replay_bytes" internal/collector/
grep -r "pg.checkpoint.requested_per_second" internal/collector/
grep -r "pg.connections.utilization_pct" internal/collector/
grep -r "os.disk.util_pct" internal/collector/
grep -r "pg.bgwriter.buffers_backend_per_second" internal/collector/

# 10. Verify config.go has extensible Config struct
grep -n "type Config struct" internal/config/config.go
```

If any grep reveals mismatches, document in corrections.md before spawning agents.

---

## Step 5 — Commit docs

```bash
cd C:\Users\Archer\Projects\PGPulse_01

git add docs/iterations/M14_01_03212026_rca-engine/
git add CLAUDE.md
git commit -m "docs(M14_01): requirements, design, team-prompt, checklist — RCA correlation engine"
```

---

## Step 6 — Spawn agents

```bash
cd C:\Users\Archer\Projects\PGPulse_01
claude --model claude-opus-4-6
```

Paste or point to:
```
Read docs/iterations/M14_01_03212026_rca-engine/team-prompt.md and execute.
```

**Note:** This is a 3-agent team (largest yet). Close all other CLI terminals before spawning to avoid OOM on memory-constrained machines.

---

## Step 7 — Watch-list of expected files

Monitor agent progress:

**New files (19):**
- [ ] `internal/rca/ontology.go`
- [ ] `internal/rca/graph.go`
- [ ] `internal/rca/chains.go`
- [ ] `internal/rca/engine.go`
- [ ] `internal/rca/anomaly.go`
- [ ] `internal/rca/statsource.go`
- [ ] `internal/rca/incident.go`
- [ ] `internal/rca/trigger.go`
- [ ] `internal/rca/config.go`
- [ ] `internal/rca/store.go`
- [ ] `internal/rca/pgstore.go`
- [ ] `internal/rca/nullstore.go`
- [ ] `internal/rca/engine_test.go`
- [ ] `internal/rca/graph_test.go`
- [ ] `internal/rca/chains_test.go`
- [ ] `internal/rca/anomaly_test.go`
- [ ] `internal/rca/pgstore_test.go`
- [ ] `internal/api/rca.go`
- [ ] `migrations/016_rca_incidents.sql`

**Modified files (6):**
- [ ] `cmd/pgpulse-server/main.go` (+~30 lines)
- [ ] `internal/api/server.go` (+~15 lines)
- [ ] `internal/config/config.go` (+~5 lines)
- [ ] `internal/config/load.go` (+~10 lines)
- [ ] `internal/storage/pgstore.go` (+~40 lines: MetricStatsProvider)
- [ ] `internal/storage/memory.go` (+~30 lines: MetricStatsProvider)

**Unchanged (verify):**
- [ ] `internal/alert/dispatcher.go`
- [ ] `internal/ml/detector.go`
- [ ] `web/src/*` — zero changes

---

## Step 8 — Build verification

Run after agents complete. All must pass.

```bash
cd C:\Users\Archer\Projects\PGPulse_01

# Frontend (must be unchanged)
cd web && npm run build && npm run typecheck && npm run lint && cd ..

# Standard server build
go build ./cmd/pgpulse-server

# Desktop build
go build -tags desktop ./cmd/pgpulse-server

# Tests
go test ./cmd/... ./internal/... -count=1

# Lint
golangci-lint run ./cmd/... ./internal/...

# Symbol check — standard binary has no Wails
go tool nm pgpulse-server.exe 2>/dev/null | grep -ci wails
# Expected: 0

# Verify new RCA package compiles independently
go build ./internal/rca/...

# Verify migration file exists
ls migrations/016_rca_incidents.sql
```

### Optional: Deploy to demo VM and test

```bash
# Cross-compile
export GOOS=linux && export GOARCH=amd64 && export CGO_ENABLED=0
go build -ldflags="-s -w" -o dist/pgpulse-server-linux-amd64 ./cmd/pgpulse-server
unset GOOS GOARCH CGO_ENABLED

# Deploy
scp dist/pgpulse-server-linux-amd64 ml4dbs@185.159.111.139:/home/ml4dbs/pgpulse-demo/
ssh ml4dbs@185.159.111.139 'sudo systemctl stop pgpulse && sudo cp /home/ml4dbs/pgpulse-demo/pgpulse-server-linux-amd64 /opt/pgpulse/bin/pgpulse-server && sudo chmod +x /opt/pgpulse/bin/pgpulse-server && sudo systemctl start pgpulse'

# Test RCA endpoint
curl -s http://185.159.111.139:8989/api/v1/rca/graph | python3 -m json.tool | head -20
# Expected: JSON with nodes and edges

# Trigger manual RCA
curl -s -X POST http://185.159.111.139:8989/api/v1/instances/primary/rca/analyze \
  -H "Content-Type: application/json" \
  -d '{"metric":"pg.replication.lag.replay_bytes","timestamp":"2026-03-21T12:00:00Z","window_minutes":30}'
# Expected: JSON incident (may have low confidence if no anomalies in the window)
```

---

## Step 9 — Commit clean build

```bash
cd C:\Users\Archer\Projects\PGPulse_01

git add -A
git status

git commit -m "feat(rca): M14_01 — causal graph + reliable correlation engine

- Create internal/rca/ package: 16 Tier A causal chains, 4 Tier B stubs
- Shared RCA↔Adviser ontology with stable identifiers
- Correlation engine with required-evidence pruning and bounded traversal
- Dual anomaly source: ML Z-scores or threshold fallback with fuzzy window
- Incident timeline with confidence scoring and quality markers
- Qualified summary language (never presents causality as certainty)
- Auto-trigger on CRITICAL alerts via OnAlert hook
- 5 new API endpoints for RCA analysis and incident retrieval
- PostgreSQL incident storage with normalized summary columns
- Migration 016: rca_incidents table with JSONB timeline
- MetricStatsProvider for batch stats optimization in threshold mode"
```

---

## Step 10 — Wrap-up

### 10a — Regenerate codebase digest

```bash
cd C:\Users\Archer\Projects\PGPulse_01
claude --model claude-opus-4-6
```

Prompt:
```
Regenerate docs/CODEBASE_DIGEST.md per codebase-digest.md rules.
```

### 10b — Session log

Produce `docs/iterations/M14_01_03212026_rca-engine/M14_01_session-log.md` with:
- Agent execution time (3 agents)
- Files created/modified
- Decisions made by agents (deviations from design)
- Issues encountered (especially: circular imports, metric key mismatches, test failures)
- Build verification results

### 10c — Handoff document

Produce `docs/iterations/M14_01_03212026_rca-engine/HANDOFF_M14_01_to_M14_02.md` with:
- What M14_01 built
- RCA engine interfaces for the UI (API response shapes)
- What M14_02 needs: RCA Incidents page, timeline visualization, alert detail integration
- Known issues

### 10d — Commit wrap-up docs

```bash
cd C:\Users\Archer\Projects\PGPulse_01

git add docs/iterations/M14_01_03212026_rca-engine/M14_01_session-log.md
git add docs/iterations/M14_01_03212026_rca-engine/HANDOFF_M14_01_to_M14_02.md
git add docs/CODEBASE_DIGEST.md
git commit -m "docs(M14_01): session-log, handoff to M14_02, codebase digest update"
```

### 10e — Upload CODEBASE_DIGEST.md to Project Knowledge

Re-upload `docs/CODEBASE_DIGEST.md` to Claude.ai Project Knowledge.

### 10f — Push

```bash
git push origin master
```

---

## Summary of all 10 steps

| # | Step | Status |
|---|------|--------|
| 1 | Copy docs to iteration folder | ☐ |
| 2 | Update CLAUDE.md current iteration | ☐ |
| 3 | Update Project Knowledge (if needed) | ☐ |
| 4 | Pre-flight grep (10 checks) | ☐ |
| 5 | Commit docs | ☐ |
| 6 | Spawn agents (3-agent team) | ☐ |
| 7 | Watch-list check | ☐ |
| 8 | Build verification | ☐ |
| 9 | Commit clean build | ☐ |
| 10 | Wrap-up: session-log + handoff + digest + push | ☐ |
