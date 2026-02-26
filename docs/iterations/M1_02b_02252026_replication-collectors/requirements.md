# M1_02b — Requirements: Replication Collectors

**Iteration:** M1_02b (Replication monitoring — PGAM Q20, Q21, Q37, Q38, Q40)
**Type:** Feature — new collectors
**Depends on:** M1_02a (InstanceContext refactor) completed and committed
**Estimated effort:** Agent Teams with 2 specialists, ~45 minutes
**Date:** 2026-02-26

---

## Goal

Port PGAM replication queries into three new Go collectors that provide:
- Real-time replication lag monitoring (byte-based and time-based) on primaries
- Replication slot health and WAL retention tracking on both roles
- Active replica enumeration on primaries, WAL receiver status on replicas

All collectors use `InstanceContext.IsRecovery` to determine role-appropriate
behavior without redundant `pg_is_in_recovery()` queries.

---

## Requirements

### R1: ReplicationLagCollector (replication_lag.go)

**PGAM source:** Q37 (byte lag), Q38 (time lag)
**Role:** Primary only — returns `nil, nil` when `ic.IsRecovery == true`
**Interval:** 10 seconds (high frequency — lag is critical)
**Connection:** Uses existing single `*pgx.Conn` (no new connections)

**Metrics emitted (per replica):**

| Metric | Type | Labels |
|--------|------|--------|
| `pgpulse.replication.lag.pending_bytes` | float64 | app_name, client_addr, state |
| `pgpulse.replication.lag.write_bytes` | float64 | app_name, client_addr, state |
| `pgpulse.replication.lag.flush_bytes` | float64 | app_name, client_addr, state |
| `pgpulse.replication.lag.replay_bytes` | float64 | app_name, client_addr, state |
| `pgpulse.replication.lag.total_bytes` | float64 | app_name, client_addr, state |
| `pgpulse.replication.lag.write_seconds` | float64 | app_name, client_addr |
| `pgpulse.replication.lag.flush_seconds` | float64 | app_name, client_addr |
| `pgpulse.replication.lag.replay_seconds` | float64 | app_name, client_addr |

**Notes:**
- Time lag columns (`write_lag`, `flush_lag`, `replay_lag`) are `interval` type.
  Convert to `float64` seconds using `EXTRACT(EPOCH FROM ...)`.
- If no replicas are connected, return empty slice (not an error).
- All lag columns can be NULL if replica hasn't started streaming — emit 0 or skip.

### R2: ReplicationSlotsCollector (replication_slots.go)

**PGAM source:** Q40
**Role:** Both primary and replica (slots can exist on replicas)
**Interval:** 60 seconds
**Version gates required:**

| PG Version | Columns Available |
|------------|-------------------|
| 14 | slot_name, slot_type, active, retained_bytes |
| 15+ | + two_phase |
| 16+ | + conflicting |

**Metrics emitted (per slot):**

| Metric | Type | Labels |
|--------|------|--------|
| `pgpulse.replication.slot.retained_bytes` | float64 | slot_name, slot_type, active |
| `pgpulse.replication.slot.active` | float64 (1/0) | slot_name, slot_type |

**Conditional labels (version-gated):**
- PG 15+: add `two_phase` label ("true"/"false")
- PG 16+: add `conflicting` label ("true"/"false")

**Notes:**
- Use `version.Gate` with 3 SQL variants (PG 14, PG 15, PG 16+)
- `retained_bytes` can be NULL if `restart_lsn` is NULL (slot never activated) — emit 0
- If no replication slots exist, return empty slice (not an error)

### R3: ReplicationStatusCollector (replication_status.go)

**PGAM source:** Q20 (primary), Q21 (replica)
**Role:** Role-dependent — runs different queries based on `ic.IsRecovery`
**Interval:** 60 seconds

**Primary-side metrics (Q20 — active replicas):**

| Metric | Type | Labels |
|--------|------|--------|
| `pgpulse.replication.active_replicas` | float64 | — (aggregate count) |
| `pgpulse.replication.replica.connected` | float64 (1) | app_name, client_addr, state, sync_state |

**Replica-side metrics (Q21 — WAL receiver):**

| Metric | Type | Labels |
|--------|------|--------|
| `pgpulse.replication.wal_receiver.connected` | float64 (1/0) | sender_host, sender_port |
| `pgpulse.replication.wal_receiver.lag_bytes` | float64 | sender_host, sender_port |

**Notes:**
- Primary: `active_replicas` = count of rows from `pg_stat_replication`
  (where slot_type='physical' and active=true). Also emit one point per
  replica with connection details as labels.
- Replica: `wal_receiver.connected` = 1 if `pg_stat_wal_receiver` returns a row
  with status='streaming', 0 otherwise. `lag_bytes` = difference between
  `latest_end_lsn` and `received_lsn` (if `received_lsn < latest_end_lsn`).
- If `pg_stat_wal_receiver` is empty (replica not connected to primary),
  emit connected=0 with no other metrics.

### R4: Collector Registration

- All three collectors must call `RegisterCollector()` in `init()` functions
  (same pattern as M1_01 collectors)
- Constructor functions: `NewReplicationLagCollector(instanceID, pgVersion)`,
  `NewReplicationSlotsCollector(instanceID, pgVersion)`,
  `NewReplicationStatusCollector(instanceID, pgVersion)`
- All embed `Base` struct

### R5: Unit Tests

Each collector needs tests covering:
- Primary mode (`InstanceContext{IsRecovery: false}`)
- Replica mode (`InstanceContext{IsRecovery: true}`)
- Empty result sets (no replicas, no slots, no WAL receiver)
- Version gate selection (slots collector: PG 14 vs 15 vs 16)

Since Docker Desktop is unavailable, tests should use the existing mock/helper
patterns from M1_01 (`testutil_test.go`). If `testutil_test.go` provides
`setupPG()` with testcontainers, add build-tag-guarded integration tests
(`//go:build integration`) that will run in CI.

### R6: Validation Gates

Same as all M1 iterations:
- `go build ./...` passes
- `go vet ./...` passes
- `golangci-lint run` passes
- `go test ./internal/collector/...` passes
- No SQL string concatenation anywhere (parameterized queries only)

---

## Explicitly Deferred

| Item | Reason | When |
|------|--------|------|
| Q41 — Logical replication sync | Requires per-DB connections (PerDatabaseCollector interface) | M1_03+ |
| Q36, Q39 — PG < 10 replication | Below minimum supported version (PG 14) | Never |
| Alert rules for replication | M4 scope (alert engine) | M4 |
| Replication dashboard UI | M5 scope (frontend) | M5 |
