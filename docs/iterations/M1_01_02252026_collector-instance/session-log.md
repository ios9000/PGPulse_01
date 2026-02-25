# Session: 2026-02-25 — M1_01 Instance Metrics Collector

## Goal

Port PGAM instance-level queries (Q2–Q3, Q9–Q19) into Go collectors implementing
the Collector interface from M0. Prove the architecture end-to-end:
SQL → MetricPoint → Registry. First real metric collection pipeline.

## Agent Team Configuration

- **Team Lead:** Opus 4.6
- **Specialists:** Collector Agent, QA Agent (2 agents — no API agent needed)
- **API & Security Agent:** Not active (no API/storage work in this iteration)
- **Mode:** Hybrid — agents created files, developer ran bash manually

## PGAM Queries Ported

| Query # | Description | Target Function | Agent | Status |
|---------|-------------|-----------------|-------|--------|
| Q1 | version() | version/version.go | — | ✅ Done in M0 |
| Q2 | pg_postmaster_start_time() | server_info.go | Collector | ✅ Ported |
| Q3 | uptime | server_info.go (computed in Go) | Collector | ✅ Ported |
| Q4–Q8 | OS metrics via COPY TO PROGRAM | — | — | ⏭️ Deferred to M6 |
| Q9 | pg_is_in_recovery() | server_info.go | Collector | ✅ Ported |
| Q10 | pg_is_in_backup() | server_info.go (version-gated PG14) | Collector | ✅ Ported |
| Q11 | connection count | connections.go | Collector | ✅ Ported + bug fix |
| Q12 | max_connections | connections.go | Collector | ✅ Ported |
| Q13 | superuser_reserved_connections | connections.go | Collector | ✅ Ported |
| Q14 | global cache hit ratio | cache.go | Collector | ✅ Ported + bug fix |
| Q15 | commit/rollback ratio | transactions.go | Collector | ✅ Ported + enhanced |
| Q16 | database sizes | database_sizes.go | Collector | ✅ Ported |
| Q17 | track_io_timing | settings.go | Collector | ✅ Ported + extended |
| Q18 | pg_stat_statements installed | extensions.go | Collector | ✅ Ported |
| Q19 | pgss fill percentage | extensions.go | Collector | ✅ Ported |

**Total: 12 queries ported out of 12 planned. Q1 done in M0, Q4–Q8 deferred to M6.**
**Running tally: 13/76 PGAM queries addressed (1 M0 + 12 M1_01).**

## PGAM Bugs Fixed During Port

| Query | Bug | Fix Applied |
|-------|-----|-------------|
| Q11 | Counts PGPulse's own connection | `WHERE pid != pg_backend_pid()` |
| Q14 | Division by zero on fresh instances | `NULLIF(sum(blks_hit) + sum(blks_read), 0)` + `COALESCE(..., 0)` |

## Enhancements Over PGAM

| Collector | PGAM Behavior | PGPulse Enhancement |
|-----------|---------------|---------------------|
| connections.go | Single total count | Per-state breakdown (active, idle, idle_in_transaction, etc.) |
| transactions.go | Single global commit ratio | Per-database commit ratio + deadlock counts |
| settings.go | Single SHOW track_io_timing | 4 key settings in one query via pg_settings IN-list |
| extensions.go | pgss presence only | Presence + fill % + stats_reset timestamp (PG ≥ 14) |

## Agent Activity Summary

### Collector Agent
- Created 9 files:
  - `internal/collector/base.go` — shared Base struct, point(), queryContext()
  - `internal/collector/server_info.go` — Q2, Q3, Q9, Q10 with version gate
  - `internal/collector/connections.go` — Q11–Q13 with state breakdown
  - `internal/collector/cache.go` — Q14 with NULLIF fix
  - `internal/collector/transactions.go` — Q15 enhanced with per-DB deadlocks
  - `internal/collector/database_sizes.go` — Q16
  - `internal/collector/settings.go` — Q17 extended with mapping table
  - `internal/collector/extensions.go` — Q18, Q19, Q19b (pgss_info)
  - `internal/collector/registry.go` — sequential CollectAll, partial-failure tolerant

### QA Agent
- Created 9 test files:
  - `internal/collector/testutil_test.go` — setupPG(), setupPGWithStatements(), metric helpers
  - `internal/collector/server_info_test.go` — PG 14 + PG 17 version gate tests
  - `internal/collector/connections_test.go` — self-exclusion test with dual connections
  - `internal/collector/cache_test.go` — hit ratio bounds check
  - `internal/collector/transactions_test.go` — per-DB labels verification
  - `internal/collector/database_sizes_test.go` — size > 0 verification
  - `internal/collector/settings_test.go` — bool conversion + 4 settings present
  - `internal/collector/extensions_test.go` — with-pgss and without-pgss paths
  - `internal/collector/registry_test.go` — mock-based, no Docker required

## Architecture Decisions Made

| Decision | Rationale |
|----------|-----------|
| One file = one collector = one struct | Selective enable/disable at runtime; independent test files |
| Gate struct uses VersionRange (not raw int min/max) | Matches M0 gate.go implementation; design.md showed simplified form |
| Uptime computed in Go, not SQL | Avoids extra round-trip; more precise with time.Now() |
| pg_settings IN-list instead of multiple SHOW | Single round-trip; extensible by adding names to list |
| setupPGWithStatements uses Cmd: ["-c", "shared_preload_libraries=pg_stat_statements"] | Correct way to preload pgss in testcontainers postgres Docker image |
| registry_test.go has no build tag | Uses mock collectors only — no Docker dependency for unit tests |
| Checkpoint/bgwriter stats deferred to M1_03 | PG 17 bgwriter/checkpointer split is high complexity; isolate it |

## Files Created

```
internal/collector/base.go
internal/collector/server_info.go
internal/collector/connections.go
internal/collector/cache.go
internal/collector/transactions.go
internal/collector/database_sizes.go
internal/collector/settings.go
internal/collector/extensions.go
internal/collector/registry.go
internal/collector/testutil_test.go
internal/collector/server_info_test.go
internal/collector/connections_test.go
internal/collector/cache_test.go
internal/collector/transactions_test.go
internal/collector/database_sizes_test.go
internal/collector/settings_test.go
internal/collector/extensions_test.go
internal/collector/registry_test.go
```

**Total: 18 files (9 production + 9 test)**

## Build & Test Results

Developer ran manually (hybrid workflow):

```
go get github.com/testcontainers/testcontainers-go
go get github.com/testcontainers/testcontainers-go/modules/postgres
go get github.com/stretchr/testify
go mod tidy
go build ./...
go vet ./...
golangci-lint run
go test -v ./internal/collector/...              # unit tests (registry mocks)
go test -tags integration -v ./internal/collector/...  # integration (Docker)
```

Results: [TO BE FILLED BY DEVELOPER AFTER RUNNING]

## Known Issues

1. **Claude Code bash still broken on Windows** — hybrid workflow used successfully
2. **Gate struct mismatch** — design.md showed `{MinVersion: 140000, MaxVersion: 149999}` but M0's gate.go uses `VersionRange{MinMajor, MinMinor, MaxMajor, MaxMinor}`. Collector agent used the correct M0 struct. Design.md should be updated.
3. **Docker Desktop required** — integration tests need Docker running. CI (GitHub Actions) has Docker. Local dev on Windows needs Docker Desktop or WSL2.

## Not Done / Next Iteration

- [ ] Port queries 20–41 (replication: physical + logical, slots, WAL receiver) → **M1_02**
- [ ] Port checkpoint/bgwriter stats with PG 17 split → **M1_03**
- [ ] Port pg_stat_statements analysis (IO + CPU sorted) → **M1_04**
- [ ] Port locks, wait events, long transactions → **M1_05**
- [ ] Update roadmap.md — mark M1_01 complete
- [ ] Update CHANGELOG.md — add M1_01 features
- [ ] Commit session-log + docs: `git commit -m "docs: add M1_01 session-log"`
