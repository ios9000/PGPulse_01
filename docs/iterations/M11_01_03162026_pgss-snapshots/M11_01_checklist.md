# M11_01 Checklist — PGSS Snapshots, Diff Engine, Query Insights API + Bug Fixes

**Iteration:** M11_01
**Date:** 2026-03-16

---

## Pre-Flight

- [ ] Verify CODEBASE_DIGEST.md is uploaded to Project Knowledge (M10_01 version)
- [ ] Create iteration folder: `docs/iterations/M11_01_03162026_pgss-snapshots/`
- [ ] Copy requirements.md, design.md, team-prompt.md, checklist.md to iteration folder
- [ ] Update `.claude/CLAUDE.md` current iteration to M11_01
- [ ] Commit docs: `git add docs/iterations/M11_01_*/ && git commit -m "docs: M11_01 planning — PGSS snapshots"`
- [ ] Pre-flight grep: verify `SnapshotStore` name not already used elsewhere in codebase
- [ ] Pre-flight grep: verify migration 015 doesn't already exist
- [ ] Pre-flight grep: find exact location of `wastedibytes`/`wastedIBytes` scan in database.go
- [ ] Pre-flight grep: find exact location of `srsubstate` scan
- [ ] Pre-flight grep: find exact `"remediation config"` debug log line in main.go

---

## Agent Spawn

- [ ] Close other CLI terminals (OOM risk with Agent Teams)
- [ ] `cd ~/Projects/PGPulse_01`
- [ ] `claude --model claude-opus-4-6`
- [ ] Paste team-prompt.md content

---

## Agent 1 — Backend Specialist (statements package)

- [ ] `internal/statements/types.go` created with all types
- [ ] `internal/statements/store.go` created with SnapshotStore interface
- [ ] `internal/statements/nullstore.go` created (compiles, satisfies interface)
- [ ] `internal/statements/pgstore.go` created with COPY bulk insert
- [ ] `internal/statements/pgstore_test.go` passes
- [ ] `internal/statements/diff.go` created with ComputeDiff
- [ ] `internal/statements/diff_test.go` passes — covers: normal diff, stats_reset, new queries, evicted, null columns, div-by-zero
- [ ] `internal/statements/insights.go` created with BuildQueryInsight
- [ ] `internal/statements/insights_test.go` passes
- [ ] `internal/statements/report.go` created with GenerateReport
- [ ] `internal/statements/report_test.go` passes
- [ ] `internal/statements/capture.go` created with Start/Stop/CaptureInstance
- [ ] `internal/statements/capture_test.go` passes
- [ ] Version-gated SQL uses `version.Gate` (not hardcoded if/else)

---

## Agent 2 — API Specialist

- [ ] `internal/storage/migrations/015_pgss_snapshots.sql` created
- [ ] Migration includes `database_name TEXT` and `user_name TEXT` in entries table
- [ ] Migration includes both indexes (snapshot_instance_time, entries_queryid)
- [ ] `internal/config/config.go` — StatementSnapshotsConfig added with defaults
- [ ] `internal/api/handler_snapshots.go` — 7 handlers created
- [ ] `/diff` and `/latest-diff` registered BEFORE `/{snapId}` in router
- [ ] `internal/api/server.go` — routes registered, setter methods added
- [ ] `cmd/pgpulse-server/main.go` — wiring under guard, debug log removed
- [ ] `internal/api/handler_snapshots_test.go` passes
- [ ] Manual capture endpoint requires `instance_management` permission

---

## Agent 3 — Fix Specialist + QA

- [ ] Debug log removed from main.go
- [ ] `wastedibytes` float64 scan fix applied in database.go
- [ ] `pg.server.multixact_pct` metric added to server_info.go
- [ ] `srsubstate` char(1) scan fix applied
- [ ] Each fix verified with targeted test or manual build check

---

## Build Verification

- [ ] `cd web && npm run build` — PASS
- [ ] `npm run lint` — PASS
- [ ] `npm run typecheck` — PASS
- [ ] `cd .. && go build ./cmd/pgpulse-server` — PASS
- [ ] `go test ./cmd/... ./internal/... -count=1` — PASS
- [ ] `golangci-lint run ./cmd/... ./internal/...` — PASS

---

## Post-Build

- [ ] All agents committed their changes
- [ ] Verify migration 015 is in embedded FS (check `internal/storage/migrations/` has it)
- [ ] Verify NullSnapshotStore satisfies interface (compile check)
- [ ] Verify config parsing with `statement_snapshots:` section in sample YAML
- [ ] Count new API endpoints: should be 7 new (total ~54)
- [ ] Count new files: should be ~16 new

---

## Wrap-Up

- [ ] Regenerate `docs/CODEBASE_DIGEST.md` via Claude Code
- [ ] Write `M11_01_session-log.md`
- [ ] Write `HANDOFF_M11_01_to_M11_02.md`
- [ ] Commit all docs
- [ ] Upload new CODEBASE_DIGEST.md to Project Knowledge
- [ ] Cross-compile Linux binary: `export GOOS=linux && export GOARCH=amd64 && export CGO_ENABLED=0 && go build -ldflags="-s -w" -o dist/pgpulse-server-linux-amd64 ./cmd/pgpulse-server && unset GOOS GOARCH CGO_ENABLED`
- [ ] Deploy to demo VM and verify:
  - [ ] Migration 015 runs on startup
  - [ ] First snapshot captured (if capture_on_startup=true)
  - [ ] `GET /api/v1/instances/{id}/snapshots` returns snapshots after interval
  - [ ] `GET /api/v1/instances/{id}/snapshots/latest-diff` returns diff after 2+ snapshots
  - [ ] Bug fixes verified on live data

---

## Watch-List (Expected Files)

```
NEW:
  internal/statements/types.go
  internal/statements/store.go
  internal/statements/nullstore.go
  internal/statements/pgstore.go
  internal/statements/pgstore_test.go
  internal/statements/diff.go
  internal/statements/diff_test.go
  internal/statements/insights.go
  internal/statements/insights_test.go
  internal/statements/report.go
  internal/statements/report_test.go
  internal/statements/capture.go
  internal/statements/capture_test.go
  internal/storage/migrations/015_pgss_snapshots.sql
  internal/api/handler_snapshots.go
  internal/api/handler_snapshots_test.go

MODIFIED:
  cmd/pgpulse-server/main.go
  internal/config/config.go
  internal/api/server.go
  internal/collector/database.go
  internal/collector/server_info.go
  internal/collector/database.go (or logical_replication.go for srsubstate)
```
