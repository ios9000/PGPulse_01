# M14_04 — Session Log: Guided Remediation Playbooks

**Iteration:** M14_04
**Date:** 2026-03-24 → 2026-03-25
**Model:** Claude Opus 4.6 (1M context)
**Tool:** Claude Code
**Branch:** master

---

## Goals

Implement Guided Remediation Playbooks — a fourth operational layer transforming PGPulse from "what happened?" (alerting + RCA) into "what should I do now?" (step-by-step executable playbooks with inline SQL execution, tier-based safety, and branching logic).

Work items W1–W12 from requirements. Decisions D600–D609 locked.

---

## Timeline

| Time | Activity |
|------|----------|
| Session start | Pre-flight greps (checklist Step 5): 24 greps against codebase |
| +15min | **G1 CRITICAL finding**: 6/10 seed playbook hooks in design doc don't match actual ontology constants. 3 hooks missing, 3 renamed. |
| +20min | Corrections doc updated with all 20 grep findings (G1–G20). Committed: `ce31f74` |
| +25min | Team prompt received and parsed. 2 agents spawned in parallel worktrees. |
| +16min | **Frontend agent completes** — 23 files, all checks pass (typecheck, lint, build) |
| +26min | **Backend agent completes** — 30 files, all checks pass (build, vet, test, lint) |
| +5min | Both worktrees merged to master. Full build verification: all 7 checks pass. |

---

## Agent Activity

### Backend Agent (worktree-agent-a92cdb71)

**Duration:** ~26 minutes | **Tool uses:** 107 | **Tokens:** 191K

Implemented:
- Migration 018: 4 tables (`playbooks`, `playbook_steps`, `playbook_runs`, `playbook_run_steps`) with GIN index, partial index, UNIQUE constraints
- `internal/playbook/` package (22 files, 2,804 lines):
  - types.go, store.go (PlaybookStore interface with 20+ methods)
  - pgstore.go (746 lines — JSONB containment queries, seed upsert)
  - nullstore.go (no-op for live mode)
  - executor.go (transaction-scoped C1, injection guard C2, config row limit C8)
  - interpreter.go (scope "first" MVP per C3, template expansion)
  - resolver.go (5-level priority: hook > root_cause > metric > adviser_rule > manual)
  - feedback_worker.go (60s ticker, implicit alert resolution detection)
  - 10 seed playbook files (seed_wal.go through seed_query.go)
  - 3 test files (executor, interpreter, resolver)
- `internal/api/playbooks.go` (652 lines — 19 HTTP handlers)
- Modified: ontology.go (+3 hooks), hooks.go, config.go, load.go, server.go, main.go

### Frontend Agent (worktree-agent-a4f5280a)

**Duration:** ~16 minutes | **Tool uses:** 86 | **Tokens:** 128K

Implemented:
- `web/src/types/playbook.ts` — full type system
- `web/src/hooks/usePlaybooks.ts` — 20+ React Query hooks
- 11 components in `web/src/components/playbook/`:
  - TierBadge, VerdictBadge, ResultTable, RunProgressBar, BranchIndicator
  - PlaybookCard, PlaybookFilters, StepCard, StepBuilder, FeedbackModal, ResolverButton
- 5 pages:
  - PlaybookCatalog (`/playbooks`)
  - PlaybookDetail (`/playbooks/:id`)
  - PlaybookEditor (`/playbooks/:id/edit`)
  - PlaybookWizard (`/servers/:id/playbook-runs/:runId`) — key UX deliverable
  - PlaybookRunHistory (`/playbook-runs`)
- Modified: Sidebar.tsx, App.tsx, AlertDetailPanel.tsx, RCAIncidentDetail.tsx, AdvisorRow.tsx

---

## Security Corrections Applied

| # | Correction | Status |
|---|-----------|--------|
| C1 | Transaction-scoped execution (BEGIN + SET LOCAL + ROLLBACK) | Applied in executor.go |
| C2 | Multi-statement injection guard (semicolon check) | Applied in executor.go |
| C3 | Interpreter scope "first" only (any/all deferred) | Applied in interpreter.go |
| C4 | Feedback worker (background goroutine) | New file: feedback_worker.go |
| C5 | Concurrency guard (LockStepForExecution) | Applied in pgstore.go + API handler |
| C6 | Error state machine (failed → retry) | Applied in API handler + StepCard UI |
| C7 | Lightweight approval flow (pending_approval) | Applied in API handler + StepCard UI |
| C8 | Config-bound row limit | Applied in executor.go |
| C9 | array_agg ordering | Applied in pgstore.go |

---

## Pre-Flight Findings (G1–G20)

### Critical

**G1: Hook constant mismatches** — 6/10 seed playbook hooks in design doc used hypothetical names that don't exist in `internal/rca/ontology.go`.

Resolution:
- Added 3 new constants: `HookWALArchive`, `HookReplicationLag`, `HookDiskCapacity`
- Corrected 3 mappings: `HookLockTimeout` → `HookLockInvestigation`, `HookLongTransaction` → `HookKillLongTx`, `HookWraparound` → `HookWraparoundVacuum`
- Updated `internal/remediation/hooks.go` HookToRuleID map

### Confirmed (no surprises)

G2: RBAC matches design. G3: ConnFor returns `*pgx.Conn`. G4: AlertHistoryStore.ListUnresolved available. G5: Migration 018 is next. G6–G9: Config, wiring, API constructor, routes follow patterns. G10–G12: Frontend router, sidebar, integration points confirmed. G13–G16: Background worker, pgx tx, seed, NullStore patterns confirmed. G17: pg_stat_archiver available PG14+. G18–G20: Frontend API/hook/type patterns confirmed.

---

## Files Created/Modified

### New Files (48)

**Backend (25):**
```
internal/storage/migrations/018_playbooks.sql
internal/playbook/types.go
internal/playbook/store.go
internal/playbook/pgstore.go
internal/playbook/nullstore.go
internal/playbook/executor.go
internal/playbook/executor_test.go
internal/playbook/interpreter.go
internal/playbook/interpreter_test.go
internal/playbook/resolver.go
internal/playbook/resolver_test.go
internal/playbook/feedback_worker.go
internal/playbook/seed.go
internal/playbook/seed_wal.go
internal/playbook/seed_replication.go
internal/playbook/seed_connections.go
internal/playbook/seed_locks.go
internal/playbook/seed_longtx.go
internal/playbook/seed_checkpoint.go
internal/playbook/seed_disk.go
internal/playbook/seed_vacuum.go
internal/playbook/seed_wraparound.go
internal/playbook/seed_query.go
internal/api/playbooks.go
```

**Frontend (23):**
```
web/src/types/playbook.ts
web/src/hooks/usePlaybooks.ts
web/src/pages/PlaybookCatalog.tsx
web/src/pages/PlaybookDetail.tsx
web/src/pages/PlaybookEditor.tsx
web/src/pages/PlaybookWizard.tsx
web/src/pages/PlaybookRunHistory.tsx
web/src/components/playbook/TierBadge.tsx
web/src/components/playbook/VerdictBadge.tsx
web/src/components/playbook/ResultTable.tsx
web/src/components/playbook/RunProgressBar.tsx
web/src/components/playbook/BranchIndicator.tsx
web/src/components/playbook/PlaybookCard.tsx
web/src/components/playbook/PlaybookFilters.tsx
web/src/components/playbook/StepCard.tsx
web/src/components/playbook/StepBuilder.tsx
web/src/components/playbook/FeedbackModal.tsx
web/src/components/playbook/ResolverButton.tsx
```

### Modified Files (11)
```
internal/rca/ontology.go          — +3 hook constants
internal/remediation/hooks.go     — +3 HookToRuleID entries
internal/config/config.go         — +PlaybooksConfig struct
internal/config/load.go           — +playbook defaults
internal/api/server.go            — +playbook routes, setter, fields
cmd/pgpulse-server/main.go        — +playbook wiring (store, executor, seed, feedback worker)
web/src/App.tsx                    — +5 playbook routes
web/src/components/layout/Sidebar.tsx       — +Playbooks nav item
web/src/components/alerts/AlertDetailPanel.tsx — +ResolverButton
web/src/pages/RCAIncidentDetail.tsx          — +Guided Remediation card
web/src/components/advisor/AdvisorRow.tsx     — +Remediate button
```

---

## Build Verification

| Check | Result |
|-------|--------|
| `npm run typecheck` | PASS |
| `npm run lint` | PASS (1 pre-existing warning) |
| `npm run build` | PASS |
| `go build ./cmd/pgpulse-server` | PASS |
| `go vet ./cmd/... ./internal/...` | PASS |
| `go test -count=1 ./cmd/... ./internal/...` | PASS (18 packages) |
| `golangci-lint run ./cmd/... ./internal/...` | PASS (0 issues) |

---

## Statistics

| Metric | Value |
|--------|-------|
| Backend Go lines added | ~3,538 (playbook pkg + API + migration) |
| Frontend TS/TSX lines added | ~2,687 |
| Total lines added | ~6,225 |
| New files | 48 |
| Modified files | 11 |
| API endpoints | 19 |
| Frontend routes | 5 |
| Seed playbooks | 10 |
| Database tables | 4 |
| Tests | 3 test files (executor, interpreter, resolver) |

---

## API Endpoints Added (19)

| Method | Path | Handler |
|--------|------|---------|
| GET | /api/v1/playbooks | handleListPlaybooks |
| GET | /api/v1/playbooks/resolve | handleResolvePlaybook |
| POST | /api/v1/playbooks | handleCreatePlaybook |
| GET | /api/v1/playbooks/{id} | handleGetPlaybook |
| PUT | /api/v1/playbooks/{id} | handleUpdatePlaybook |
| DELETE | /api/v1/playbooks/{id} | handleDeletePlaybook |
| POST | /api/v1/playbooks/{id}/promote | handlePromotePlaybook |
| POST | /api/v1/playbooks/{id}/deprecate | handleDeprecatePlaybook |
| POST | /api/v1/instances/{id}/playbooks/{playbookId}/run | handleStartRun |
| GET | /api/v1/instances/{id}/playbook-runs | handleListInstanceRuns |
| GET | /api/v1/playbook-runs | handleListAllRuns |
| GET | /api/v1/playbook-runs/{runId} | handleGetRun |
| POST | /api/v1/playbook-runs/{runId}/steps/{order}/execute | handleExecuteStep |
| POST | /api/v1/playbook-runs/{runId}/steps/{order}/confirm | handleConfirmStep |
| POST | /api/v1/playbook-runs/{runId}/steps/{order}/approve | handleApproveStep |
| POST | /api/v1/playbook-runs/{runId}/steps/{order}/skip | handleSkipStep |
| POST | /api/v1/playbook-runs/{runId}/steps/{order}/retry | handleRetryStep |
| POST | /api/v1/playbook-runs/{runId}/abandon | handleAbandonRun |
| POST | /api/v1/playbook-runs/{runId}/feedback | handleFeedback |

---

## Seed Playbooks (Core 10)

| # | Slug | Steps | Hook Constant Used |
|---|------|-------|--------------------|
| 1 | wal-archive-failure | 5 | HookWALArchive (NEW) |
| 2 | replication-lag | 4 | HookReplicationLag (NEW) |
| 3 | connection-saturation | 5 | HookConnectionPooling |
| 4 | lock-contention | 5 | HookLockInvestigation |
| 5 | long-transactions | 4 | HookKillLongTx |
| 6 | checkpoint-storm | 4 | HookCheckpointTuning |
| 7 | disk-full | 5 | HookDiskCapacity (NEW) |
| 8 | autovacuum-failing | 5 | HookVacuumTuning |
| 9 | wraparound-risk | 4 | HookWraparoundVacuum |
| 10 | heavy-query | 4 | HookQueryOptimization |

---

## Decisions Made This Session

| Decision | Rationale |
|----------|-----------|
| Add 3 new hook constants to ontology.go | Design doc referenced hooks that didn't exist (G1 finding) |
| Parallel worktree agents (backend + frontend) | No file overlap between agents; 42min total vs ~70min sequential |
| Copy-merge worktrees (not git merge) | Both worktrees had uncommitted changes; direct file copy simpler |

---

## Known Issues / Deferred

- **Interpreter scope "any"/"all"**: Struct field exists but evaluation falls back to "first" with warning log. Deferred per C3.
- **Playbook editor**: Uses textarea for SQL (not CodeMirror). Full editor deferred.
- **Approval notifications**: No notification to DBAs when approval requested. Deferred per D609.
- **Approval queue page**: No global pending approvals view. Deferred per D609.
- **Parameterized inputs**: No interactive PID/OID injection into step SQL. Deferred.
- **Integration tests**: Unit tests only; no testcontainers tests for playbook execution against real PG.

---

## Next Steps

1. Commit all changes with M14_04 commit message (checklist Step 10)
2. Deploy to demo VM (checklist Step 11)
3. Acceptance criteria verification (checklist Step 12)
4. Regenerate CODEBASE_DIGEST.md
5. Update roadmap + CHANGELOG
6. Create HANDOFF_M14_04_to_M15.md
