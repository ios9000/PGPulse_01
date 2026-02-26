# Session: 2026-02-26 — M1_02 Replication Collectors

## Goal
Port PGAM replication queries (Q20, Q21, Q37, Q38, Q40) into Go collectors, preceded by an interface refactor to add InstanceContext for role-aware behavior.

Split into two sub-iterations:
- **M1_02a:** Interface refactor — add InstanceContext to Collector.Collect() signature
- **M1_02b:** Replication collectors — three new collector files

## Planning Decisions (Claude.ai — Brain)

### InstanceContext SSoT Architecture (Major Decision)

The replication collectors need to know whether the instance is a primary or replica. Three options were evaluated for how to propagate this:

| Option | Approach | Verdict |
|--------|----------|---------|
| Constructor flag (`isPrimary bool`) | Set at construction time | ❌ Doesn't handle failover — role can change at runtime |
| Detect at collect time | Each collector queries `pg_is_in_recovery()` | ❌ Redundant N queries per cycle |
| **SSoT via InstanceContext** | **Orchestrator queries once, passes to all collectors** | **✅ Chosen** |

Developer proposed a Hybrid SSoT approach with three implementation sub-options:
- Option 1: Extend Collector interface with `InstanceContext` struct parameter
- Option 2: Use `context.WithValue()` (hidden dependency)
- Option 3: Shared Scraper state object (tight coupling)

**Decision: Option 1 (explicit interface parameter).** Rationale: type-safe, transparent, extensible for future per-cycle state (e.g., current LSN).

### Version in Base vs InstanceContext

Key semantic distinction established:
- **PG version → stays in Base** — structural, immutable for connection lifetime. If PG restarts with different version, collector instances are rebuilt.
- **IsRecovery → goes in InstanceContext** — dynamic, can flip between cycles (Patroni failover takes seconds).

This avoided a full refactor of all M1_01 collectors while keeping the architecture clean.

### M1_02a / M1_02b Split

Decision to create a separate refactor iteration before the feature work:
- M1_02a: Mechanical signature change across 18 files (~50 lines), single Claude Code session
- M1_02b: New replication collectors, Agent Teams with 2 specialists

Rationale: Clean git history, agents start from stable updated interface, no merge conflicts.

### Logical Replication Q41 Deferred

Q41 (`pg_subscription_rel`) requires per-database connections, breaking the single-`*pgx.Conn` Collector interface. Deferred to when PerDatabaseCollector interface is designed (alongside `analiz_db.php` Q1–Q18).

### Replication Slot Version Gates

Three SQL variants for slots query:
- PG 14: base columns only
- PG 15+: adds `two_phase`
- PG 16+: adds `conflicting`

Design: all variants SELECT exactly 6 columns (missing columns as empty string literals), enabling uniform `Scan()` across versions. Extra columns become labels (non-empty only).

## Agent Activity Summary

### M1_02a — Single Claude Code Session

| Action | Result |
|--------|--------|
| Added `InstanceContext` struct to `collector.go` | ✅ |
| Updated `Collector` interface signature | ✅ |
| Updated 6 collectors with `_ InstanceContext` | ✅ (connections, cache, transactions, database_sizes, settings, extensions) |
| Updated `server_info.go` — reads `ic.IsRecovery` instead of querying | ✅ |
| Updated `registry.go` — `CollectAll()` passes InstanceContext | ✅ |
| Updated 9 test files | ✅ |
| **Build/vet/lint/test** | **✅ All pass** |
| **Commit** | `c50dbe1` — pushed to master |

### M1_02b — Agent Teams (2 Specialists)

**Collector Agent created:**
- `internal/collector/replication_lag.go` — Q37 + Q38 combined, primary only
- `internal/collector/replication_slots.go` — Q40, version-gated (PG 14/15/16+)
- `internal/collector/replication_status.go` — Q20 (primary) + Q21 (replica)

**QA Agent created:**
- `internal/collector/replication_lag_test.go` — 2 pass, 1 integration skip
- `internal/collector/replication_slots_test.go` — 5 pass, 1 integration skip
- `internal/collector/replication_status_test.go` — 1 pass, 2 integration skip

**Validation:**
| Check | Result |
|-------|--------|
| `go build ./...` | ✅ |
| `go vet ./...` | ✅ |
| `golangci-lint run` | ✅ 0 issues |
| `go test ./internal/collector/...` | ✅ 8 pass, 3 skip (integration) |

**Commit:** M1_02b pushed to master.

## PGAM Queries Ported in This Session

| Query # | Description | Target File | Agent |
|---------|-------------|-------------|-------|
| Q37 | Replication lag by phase (bytes) | replication_lag.go | Collector |
| Q38 | Replication lag (time-based) | replication_lag.go | Collector |
| Q40 | Replication slots + WAL retention | replication_slots.go | Collector |
| Q20 | Active physical replicas | replication_status.go | Collector |
| Q21 | WAL receiver status | replication_status.go | Collector |

**Running total: 18/76 queries ported** (13 from M1_01 + 5 from M1_02b)

## Discovery: RegisterCollector Pattern Mismatch

**Important finding:** The strategy doc, CLAUDE.md, and all design docs reference an `init()` + `RegisterCollector()` auto-registration pattern. However, the actual M1_01 codebase uses **explicit registration in main.go**. `RegisterCollector()` exists in `registry.go` but M1_01 collectors don't use `init()` auto-registration.

The M1_02b agent correctly detected this and omitted `init()` blocks from the new collectors.

**Action required:** Future planning docs must reference explicit registration, not auto-registration. Corrected in the handoff and save point.

## Metrics Added

### replication_lag.go (8 metrics per replica)
- `pgpulse.replication.lag.pending_bytes`
- `pgpulse.replication.lag.write_bytes`
- `pgpulse.replication.lag.flush_bytes`
- `pgpulse.replication.lag.replay_bytes`
- `pgpulse.replication.lag.total_bytes`
- `pgpulse.replication.lag.write_seconds`
- `pgpulse.replication.lag.flush_seconds`
- `pgpulse.replication.lag.replay_seconds`

### replication_slots.go (2 metrics per slot)
- `pgpulse.replication.slot.retained_bytes`
- `pgpulse.replication.slot.active`

### replication_status.go (primary: 2 metrics, replica: 2 metrics)
- `pgpulse.replication.active_replicas`
- `pgpulse.replication.replica.connected`
- `pgpulse.replication.wal_receiver.connected`
- `pgpulse.replication.wal_receiver.lag_bytes`

## Not Done / Next Iteration

- [ ] Port Q42–Q47 (progress monitoring) → M1_03
- [ ] Checkpoint/bgwriter stats (PG 17 splits pg_stat_bgwriter) → M1_03
- [ ] pg_stat_io (PG 16+ only) → M1_03
- [ ] Q41 logical replication sync → deferred to PerDatabaseCollector design
- [ ] Version tests for internal/version/ — still no test files
