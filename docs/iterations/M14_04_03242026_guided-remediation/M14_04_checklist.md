# M14_04 — Developer Checklist

**Iteration:** M14_04
**Date:** 2026-03-24

---

## Step 1: Copy Docs to Iteration Folder

```bash
cd ~/Projects/PGPulse_01

# Create iteration folder
mkdir -p docs/iterations/M14_04_03242026_guided-remediation

# Copy all planning docs
cp /path/to/M14_04_requirements.md docs/iterations/M14_04_03242026_guided-remediation/
cp /path/to/M14_04_design.md docs/iterations/M14_04_03242026_guided-remediation/
cp /path/to/M14_04_team-prompt.md docs/iterations/M14_04_03242026_guided-remediation/
cp /path/to/M14_04_checklist.md docs/iterations/M14_04_03242026_guided-remediation/
cp /path/to/ADR-M14_04-Guided-Remediation-Playbooks.md docs/iterations/M14_04_03242026_guided-remediation/
```

## Step 2: Update CLAUDE.md

```bash
# Edit CLAUDE.md — update "Current State" section:
# - Iteration: M14_04
# - Last completed: M14_03 (Expansion, Calibration, Knowledge Integration)
# - Next: M14_04 (Guided Remediation Playbooks)
```

## Step 3: Update Project Knowledge

Upload to Claude.ai Project Knowledge:
- [ ] `M14_04_requirements.md`
- [ ] `M14_04_design.md`
- [ ] `ADR-M14_04-Guided-Remediation-Playbooks.md`
- [ ] Updated `CODEBASE_DIGEST.md` (from M14_03)

## Step 4: Commit Planning Docs

```bash
cd ~/Projects/PGPulse_01
git add docs/iterations/M14_04_03242026_guided-remediation/
git add CLAUDE.md
git commit -m "docs: M14_04 planning — guided remediation playbooks

- Requirements (W1-W12), design, team-prompt, checklist
- ADR: Guided Remediation Playbooks architecture
- Decisions D600-D609 locked
- Security corrections: transaction-scoped execution, multi-statement guard, interpreter scope"
```

## Step 5: Pre-Flight Greps (MANDATORY before spawning agents)

```bash
cd ~/Projects/PGPulse_01

# 5.1 — Hook constants in ontology (for seed playbook trigger_bindings)
grep -n "Hook" internal/rca/ontology.go

# 5.2 — hookToRuleID map (for adviser_rules bindings)
cat internal/remediation/hooks.go

# 5.3 — RBAC permission constants and roles
grep -n "Permission\|permission\|Role\|role" internal/auth/rbac.go | head -20

# 5.4 — InstanceConnProvider interface (for executor)
grep -A 5 "type InstanceConnProvider\|ConnFor" internal/api/connprovider.go

# 5.5 — ConnFor return type (is it *pgx.Conn or *pgxpool.Conn?)
grep -rn "func.*ConnFor" internal/orchestrator/orchestrator.go | head -5

# 5.6 — AlertHistoryStore interface (for feedback worker)
grep -A 10 "type AlertHistoryStore" internal/alert/store.go

# 5.7 — Existing migration numbering (confirm 018 is next)
ls -la internal/storage/migrations/ | tail -5

# 5.8 — Config struct location for new PlaybookConfig
grep -n "type.*Config struct" internal/config/config.go

# 5.9 — Main.go wiring pattern (how existing subsystems are initialized)
grep -n "rca\|remediation\|RCA\|Remediation\|NewEngine\|SetStore" cmd/pgpulse-server/main.go | head -20

# 5.10 — API server constructor (what dependencies it accepts)
grep -A 20 "func New\b" internal/api/server.go | head -25

# 5.11 — Route registration pattern
grep -n "Route\|r.Get\|r.Post\|r.Put\|r.Delete" internal/api/server.go | head -30

# 5.12 — Frontend router location
grep -rn "Route\|path.*=" web/src/App.tsx 2>/dev/null || grep -rn "Route\|path.*=" web/src/main.tsx 2>/dev/null || grep -rn "createBrowserRouter\|Routes" web/src/ | head -10

# 5.13 — Sidebar navigation structure
grep -n "Fleet\|Alerts\|Advisor\|RCA\|Settings\|Admin" web/src/components/layout/Sidebar.tsx | head -20

# 5.14 — AlertDetailPanel integration point
grep -n "ROOT CAUSE\|RCA\|remediation\|Investigate" web/src/components/alerts/AlertDetailPanel.tsx | head -10

# 5.15 — RCAIncidentDetail integration point
grep -n "Recommended\|RemediationHooks\|Actions" web/src/pages/RCAIncidentDetail.tsx | head -10

# 5.16 — AdvisorRow integration point
grep -n "RCABadge\|Acknowledge\|Button\|action" web/src/components/advisor/AdvisorRow.tsx | head -10

# 5.17 — Existing background worker pattern (for feedback_worker.go)
grep -n "func.*Start\|go func\|ticker\|background" internal/remediation/background.go | head -10

# 5.18 — pgx transaction API (Begin, Rollback, Exec, Query on tx)
grep -rn "conn.Begin\|tx.Exec\|tx.Query\|tx.Rollback" internal/ | head -10

# 5.19 — Existing seed pattern (how alert rules or remediation rules are seeded)
grep -n "func.*Seed\|SeedBuiltin\|UpsertBuiltin" internal/alert/seed.go internal/remediation/*.go 2>/dev/null | head -10

# 5.20 — Existing NullStore patterns (for playbook/nullstore.go)
cat internal/rca/nullstore.go | head -20

# 5.21 — Check if pg_stat_archiver is available (for WAL playbook seed SQL)
# This is a PG system view — should be available on all PG versions we support (14+)
grep -rn "pg_stat_archiver\|archive" internal/collector/ | head -5

# 5.22 — Frontend API wrapper pattern
grep -n "api.get\|api.post\|api.put\|api.delete\|fetchApi" web/src/lib/api.ts | head -10

# 5.23 — React Query hook patterns (for usePlaybooks.ts)
head -30 web/src/hooks/useRCA.ts

# 5.24 — Recommendation type location (for AdvisorRow integration)
grep -rn "interface Recommendation" web/src/types/
```

## Step 6: Write Corrections Doc

```bash
# Based on grep findings, create M14_04_corrections.md
# Record: Hook constant values, ConnFor return type, migration number,
# router location, AlertHistoryStore methods, seed patterns, etc.

cp M14_04_corrections.md docs/iterations/M14_04_03242026_guided-remediation/
git add docs/iterations/M14_04_03242026_guided-remediation/M14_04_corrections.md
git commit -m "docs: M14_04 pre-flight corrections"
```

## Step 7: Spawn Agents

```bash
cd ~/Projects/PGPulse_01

# Close all other CLI terminals to avoid OOM
# (M14_04 is large — 48+ new files expected)
claude --model claude-opus-4-6
```

Paste team-prompt content. Monitor for:
- [ ] Agents read all pre-read docs
- [ ] Backend agent starts with migration + types + executor
- [ ] Frontend agent starts with types + component skeletons
- [ ] No agent relitigates security corrections
- [ ] Executor uses BEGIN + SET LOCAL + ROLLBACK (NOT session-level SET)

## Step 8: Watch List — Expected Files

### Backend (25 new files)
- [ ] `internal/playbook/types.go`
- [ ] `internal/playbook/store.go`
- [ ] `internal/playbook/pgstore.go`
- [ ] `internal/playbook/nullstore.go`
- [ ] `internal/playbook/executor.go`
- [ ] `internal/playbook/executor_test.go`
- [ ] `internal/playbook/resolver.go`
- [ ] `internal/playbook/resolver_test.go`
- [ ] `internal/playbook/interpreter.go`
- [ ] `internal/playbook/interpreter_test.go`
- [ ] `internal/playbook/feedback_worker.go`
- [ ] `internal/playbook/seed.go`
- [ ] `internal/playbook/seed_wal.go`
- [ ] `internal/playbook/seed_replication.go`
- [ ] `internal/playbook/seed_connections.go`
- [ ] `internal/playbook/seed_locks.go`
- [ ] `internal/playbook/seed_longtx.go`
- [ ] `internal/playbook/seed_checkpoint.go`
- [ ] `internal/playbook/seed_disk.go`
- [ ] `internal/playbook/seed_vacuum.go`
- [ ] `internal/playbook/seed_wraparound.go`
- [ ] `internal/playbook/seed_query.go`
- [ ] `internal/storage/migrations/018_playbooks.sql`
- [ ] `internal/api/playbooks.go`
- [ ] `internal/api/playbooks_test.go`

### Frontend (23 new files)
- [ ] `web/src/types/playbook.ts`
- [ ] `web/src/hooks/usePlaybooks.ts`
- [ ] `web/src/pages/PlaybookCatalog.tsx`
- [ ] `web/src/pages/PlaybookDetail.tsx`
- [ ] `web/src/pages/PlaybookEditor.tsx`
- [ ] `web/src/pages/PlaybookWizard.tsx`
- [ ] `web/src/pages/PlaybookRunHistory.tsx`
- [ ] `web/src/components/playbook/PlaybookCard.tsx`
- [ ] `web/src/components/playbook/PlaybookFilters.tsx`
- [ ] `web/src/components/playbook/StepBuilder.tsx`
- [ ] `web/src/components/playbook/StepCard.tsx`
- [ ] `web/src/components/playbook/TierBadge.tsx`
- [ ] `web/src/components/playbook/ResultTable.tsx`
- [ ] `web/src/components/playbook/VerdictBadge.tsx`
- [ ] `web/src/components/playbook/BranchIndicator.tsx`
- [ ] `web/src/components/playbook/RunProgressBar.tsx`
- [ ] `web/src/components/playbook/FeedbackModal.tsx`
- [ ] `web/src/components/playbook/ResolverButton.tsx`

## Step 9: Build Verification

```bash
cd ~/Projects/PGPulse_01
cd web && npm run build && npm run lint && npm run typecheck && cd .. && go build ./cmd/pgpulse-server && go test ./cmd/... ./internal/... -count=1 && golangci-lint run ./cmd/... ./internal/...
```

Common failure points:
- **pgx transaction API:** If `conn.Begin()` returns `pgx.Tx`, verify `tx.Query()` signature matches
- **JSONB scanning:** `TriggerBindings`, `InterpretationSpec`, `BranchRule` need custom scan/value implementations or pgx JSON helpers
- **NullStore methods:** Every `PlaybookStore` interface method must have a no-op in `nullstore.go`
- **Frontend route conflicts:** New `/playbooks` route must not conflict with existing routes

## Step 10: Commit Clean Build

```bash
git add -A
git commit -m "feat(M14_04): guided remediation playbooks

Playbook Engine:
- Database-stored playbooks with version management and draft/stable lifecycle
- Four-tier execution safety: diagnostic (READ ONLY), remediate (confirm),
  dangerous (RBAC approval), external (manual instructions)
- Transaction-scoped SQL execution (BEGIN + SET LOCAL + ROLLBACK)
- Multi-statement injection guard
- Playbook Resolver with 5-level priority ranking
- Static declarative result interpretation with any/all/first scope
- Bounded conditional branching (no loops)
- Persistent PlaybookRun state with browser resume capability
- Implicit feedback worker (alert auto-resolve detection)

Seed Playbooks (Core 10):
- WAL archive failure, replication lag, connection saturation
- Lock contention, long transactions, checkpoint storm
- Disk full, autovacuum failing, wraparound risk, heavy query

Frontend:
- Playbook catalog with filters and search
- Step-by-step wizard with tier-aware execution
- Inline result tables with green/yellow/red interpretation
- Branch visualization and progress tracking
- Playbook editor with step builder
- Integration: Alert → Playbook, RCA → Playbook, Adviser → Playbook
- Run history with feedback collection

Migration 018: playbooks, playbook_steps, playbook_runs, playbook_run_steps"
```

## Step 11: Deploy to Demo VM

```bash
# Cross-compile
export GOOS=linux
export GOARCH=amd64
export CGO_ENABLED=0
go build -ldflags="-s -w" -o dist/pgpulse-server-linux-amd64 ./cmd/pgpulse-server
unset GOOS GOARCH CGO_ENABLED

# Deploy
scp dist/pgpulse-server-linux-amd64 ml4dbs@185.159.111.139:/home/ml4dbs/pgpulse-demo/
ssh ml4dbs@185.159.111.139 'sudo systemctl stop pgpulse && sudo cp /home/ml4dbs/pgpulse-demo/pgpulse-server-linux-amd64 /opt/pgpulse/bin/pgpulse-server && sudo chmod +x /opt/pgpulse/bin/pgpulse-server && sudo systemctl start pgpulse'

# Add playbook config to pgpulse.yml (if not already present)
ssh ml4dbs@185.159.111.139 'grep -q "^playbooks:" /opt/pgpulse/configs/pgpulse.yml || echo "
playbooks:
  enabled: true
  default_statement_timeout: 5
  default_lock_timeout: 5
  result_row_limit: 100
  run_retention_days: 90
  implicit_feedback_window: 5m" | sudo tee -a /opt/pgpulse/configs/pgpulse.yml'

# Restart to pick up config + run migration 018
ssh ml4dbs@185.159.111.139 'sudo systemctl restart pgpulse'

# Verify seed playbooks loaded
TOKEN=$(curl -s http://185.159.111.139:8989/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"pgpulse_admin"}' | python3 -c "import json,sys; print(json.load(sys.stdin)['access_token'])")

curl -s -H "Authorization: Bearer $TOKEN" http://185.159.111.139:8989/api/v1/playbooks | python3 -m json.tool | head -30
```

## Step 12: Acceptance Criteria Verification

- [ ] AC1: 10 seed playbooks in catalog
- [ ] AC2: WAL playbook Step 1 auto-runs pg_stat_archiver
- [ ] AC3: Tier 1 blocks write operations (READ ONLY)
- [ ] AC4: Tier 2 shows confirmation modal
- [ ] AC5: Tier 3 shows approval requirement for viewer role
- [ ] AC6: Resume after tab close works
- [ ] AC7: Alert detail → "Run Playbook" for connection alert
- [ ] AC8: RCA incident → Resolver returns checkpoint playbook
- [ ] AC9: Adviser → "Remediate" button appears
- [ ] AC10: Playbook editor create + promote works
- [ ] AC11: Editing stable playbook resets to draft
- [ ] AC12: Branch logic skips steps correctly
- [ ] AC13: statement_timeout kills long queries
- [ ] AC14: Implicit feedback auto-detects resolution
- [ ] AC15: Full build passes

## Step 13: Wrap-Up

### 13.1 Session Log

Create `docs/iterations/M14_04_03242026_guided-remediation/M14_04_session-log.md`

### 13.2 Regenerate CODEBASE_DIGEST

```bash
# In Claude Code:
# "Regenerate docs/CODEBASE_DIGEST.md per .claude/rules/codebase-digest.md"
```

Upload updated digest to Project Knowledge.

### 13.3 Update Roadmap + Changelog

```bash
# docs/roadmap.md — mark M14_04 complete, M14 milestone complete
# CHANGELOG.md — add M14_04 entry
```

### 13.4 Handoff Document

Create `HANDOFF_M14_04_to_M15.md`:
- Full M14 retrospective (M14_01 → M14_04)
- What Guided Remediation delivers
- Integration points for M15 (forecast → playbook via Resolver)
- Known issues and future vectors from ADR

### 13.5 Final Commit

```bash
git add docs/
git add CHANGELOG.md
git commit -m "docs: M14_04 wrap-up — session log, digest, handoff to M15"
git push origin master
```

---

## Quick Reference: File Locations

| Document | Path |
|----------|------|
| Requirements | `docs/iterations/M14_04_03242026_guided-remediation/M14_04_requirements.md` |
| Design | `docs/iterations/M14_04_03242026_guided-remediation/M14_04_design.md` |
| Team Prompt | `docs/iterations/M14_04_03242026_guided-remediation/M14_04_team-prompt.md` |
| Checklist | `docs/iterations/M14_04_03242026_guided-remediation/M14_04_checklist.md` |
| Corrections | `docs/iterations/M14_04_03242026_guided-remediation/M14_04_corrections.md` |
| ADR | `docs/iterations/M14_04_03242026_guided-remediation/ADR-M14_04-Guided-Remediation-Playbooks.md` |
| Session Log | `docs/iterations/M14_04_03242026_guided-remediation/M14_04_session-log.md` |
| Handoff | `docs/iterations/M14_04_03242026_guided-remediation/HANDOFF_M14_04_to_M15.md` |
