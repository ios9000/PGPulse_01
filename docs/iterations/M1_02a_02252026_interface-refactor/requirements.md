# M1_02a — Requirements: InstanceContext Interface Refactor

**Iteration:** M1_02a (pre-refactor for M1_02b Replication Collectors)
**Type:** Refactor — zero behavior change (except ServerInfo recovery source)
**Estimated effort:** Single Claude Code session, ~30 minutes
**Date:** 2026-02-26

---

## Motivation

The upcoming replication collectors (M1_02b) require per-scrape-cycle awareness
of whether the PostgreSQL instance is a primary or a replica. Replication lag
queries only make sense on a primary; WAL receiver queries only on a replica.

Rather than having each collector independently query `pg_is_in_recovery()`,
we adopt a **Single Source of Truth (SSoT)** pattern: the orchestrator queries
it once per cycle and propagates the result to all collectors via a new
`InstanceContext` struct passed to the `Collect()` method.

This refactor must land as a clean commit before M1_02b begins, so that new
replication collectors start from the updated interface.

## Requirements

### R1: Add InstanceContext struct to collector.go
- New struct `InstanceContext` with a single field: `IsRecovery bool`
- Placed in `internal/collector/collector.go` alongside existing interfaces
- Doc comment explaining SSoT purpose and future extensibility

### R2: Update Collector interface signature
- `Collect(ctx context.Context, conn *pgx.Conn, ic InstanceContext) ([]MetricPoint, error)`
- This is a breaking change — all implementors must be updated

### R3: Update all 7 M1_01 collector implementations
- `server_info.go`, `connections.go`, `cache.go`, `transactions.go`,
  `database_sizes.go`, `settings.go`, `extensions.go`
- Mechanical: add `_ InstanceContext` parameter (unused in these collectors)
- Exception: `server_info.go` — see R4

### R4: ServerInfoCollector reads IsRecovery from InstanceContext
- Remove the `pg_is_in_recovery()` query from ServerInfoCollector
- Instead, read `ic.IsRecovery` and emit the `pgpulse.server.is_in_recovery`
  metric point from that value
- This eliminates one redundant query per cycle
- The `pg_is_in_backup()` query (PG < 15) remains unchanged — it is not
  covered by InstanceContext

### R5: Update registry.go
- `CollectAll()` must accept and pass `InstanceContext` to each collector
- The orchestrator (future Scraper) will be responsible for constructing
  `InstanceContext` from a single `SELECT pg_is_in_recovery()` call
- For now, `CollectAll()` receives it as a parameter

### R6: Update all test files
- All `*_test.go` files that call `Collect()` must pass `InstanceContext{}`
- `server_info_test.go` needs updated assertions: the collector no longer
  queries `pg_is_in_recovery()` itself, so mock expectations may change
- `registry_test.go` mock collectors must match new signature
- `testutil_test.go` helpers updated if they invoke `Collect()`

### R7: Validation gates
- `go build ./...` passes
- `go vet ./...` passes
- `golangci-lint run` passes (v2.10.1 config)
- `go test ./internal/collector/...` passes (unit tests)
- `go test ./internal/version/...` passes (unchanged but verify no breakage)

## Non-Requirements (explicitly out of scope)

- No new collectors
- No new metrics
- No changes to `internal/version/` (Gate, PGVersion unchanged)
- No Scraper/Orchestrator implementation (M2 scope)
- No additional fields in InstanceContext (IsRecovery only for now)
- No changes to `go.mod` dependencies
