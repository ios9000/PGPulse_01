# M8_03_session-log.md
## Instance Lister Fix + Session Kill API + ML Model Persistence

**Date:** 2026-03-09
**Iteration:** M8_03
**Milestone:** M8 — ML Phase 1

---

## Goal

Three correctness/completeness gaps from M8_02 closed:
1. ML Bootstrap ignoring instances added via API after startup (`configInstanceLister` fix)
2. Session cancel/terminate API reintroduced cleanly with proper route wiring
3. STLBaseline fitted state persisted to storage DB — fast restart, no continuity loss

---

## Agent Activity

### Collector Agent
Created:
- `internal/ml/lister.go` — `DBInstanceLister` querying `instances WHERE enabled = true`
- `internal/ml/persistence.go` — `PersistenceStore` interface + `DBPersistenceStore` (JSONB upsert on `(instance_id, metric_key)`)

Modified:
- `internal/ml/baseline.go` — added `BaselineSnapshot` struct, `Snapshot()` (exports live ring residuals in chronological order), `LoadFromSnapshot()`
- `internal/ml/detector.go` — 5th `persist PersistenceStore` param on `NewDetector`; two-phase Bootstrap (snapshot load → TimescaleDB replay fallback); `Evaluate` persists all baselines after each cycle

### API Agent
Created:
- `internal/api/session_actions.go` — `handleSessionCancel` + `handleSessionTerminate` with own-PID guard, superuser guard, audit log via slog
- `internal/storage/migrations/010_ml_baseline_snapshots.sql` — `ml_baseline_snapshots` table with unique on `(instance_id, metric_key)`

Modified:
- `internal/api/server.go` — session routes registered in `PermInstanceManagement` group (both auth-enabled and auth-disabled paths)
- `internal/config/config.go` — `MLPersistenceConfig` added to `MLConfig`

### Team Lead (main.go wiring)
- `configInstanceLister` removed entirely
- `ml.NewDBInstanceLister(storagePool)` wired in its place
- `ml.NewDBPersistenceStore(storagePool)` initialized when `cfg.ML.Persistence.Enabled`
- `ml.NewDetector` updated to 5-arg call with persist store

---

## Test Results

All 16 packages pass. No new test count reported by Team Lead — tests for
lister, persistence, and session actions were written by QA Agent and included
in the passing suite.

`go build ./...` — ✅ clean
`go vet ./...` — ✅ clean
`golangci-lint run` — ✅ 0 issues (one pre-existing lint issue in `web/src/pages/Administration.tsx` confirmed pre-existing, not introduced by M8_03)

---

## Commits

2 commits pushed to origin/master:
1. `feat(ml/api)` — main M8_03 implementation (12 files, 437 insertions)
2. `chore` — removed accidentally committed agent worktree (`.claude/worktrees/agent-a87dfd96`), added `.claude/worktrees/` to `.gitignore`

---

## Post-Commit Fix: Agent Worktree in Git

**What happened:** `git add .` picked up `.claude/worktrees/agent-a87dfd96` — a nested git repo created by Agent Teams for the isolated worktree. It was committed as a git submodule reference (mode `160000`).

**Fix applied:**
```bash
git rm --cached .claude/worktrees/agent-a87dfd96
echo ".claude/worktrees/" >> .gitignore
git commit -m "chore: remove accidentally committed agent worktree, ignore .claude/worktrees/"
```

**Prevention:** `.claude/worktrees/` is now in `.gitignore`. Will not recur.

---

## Architecture Decisions

| Decision | Rationale |
|----------|-----------|
| `DBInstanceLister` queries on each Bootstrap call (no cache) | Bootstrap runs once at startup; single `SELECT id FROM instances` is negligible overhead |
| Persist after every `Evaluate` cycle (~60s) | State JSONB is small; continuity after restart matters more than write overhead |
| Export only live residuals in `Snapshot()` | Ring buffer has pre-allocated stale slots; exporting the full slice would restore incorrect residual distribution |
| `persist` param may be nil | Existing tests require no changes; passing nil disables persistence cleanly |
| Session routes in `PermInstanceManagement` group | Matches permission model of other instance mutation endpoints; admin only |

---

## Not Done / Next Iteration (M8_04)

- [ ] Forecast horizon — predict next N points from STL baseline (deferred from M8_02 by design)
- [ ] Forecast alert integration — fire alert when forecast crosses threshold (e.g. disk < 7 days)
- [ ] Forecast API endpoint — expose predictions for UI charting
- [ ] Session kill UI — frontend for the new cancel/terminate API
- [ ] Settings diff + plan capture UI — frontend deferred from M8_02
