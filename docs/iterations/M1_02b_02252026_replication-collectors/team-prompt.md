# M1_02b — Agent Teams Prompt: Replication Collectors

> **Mode:** Agent Teams — 2 specialists
> **Model:** claude-opus-4-6
> **Estimated tokens:** ~400K input, ~200K output
> **Estimated time:** ~30 minutes
> **Prerequisite:** M1_02a refactor committed (InstanceContext in Collector interface)

---

## Prompt (paste into Claude Code)

```
Read CLAUDE.md for project context, then read
docs/iterations/M1_02b/design.md for the detailed implementation plan.

Build replication monitoring collectors for PGPulse.
These port PGAM queries Q20, Q21, Q37, Q38, Q40 into Go collectors
that use the InstanceContext.IsRecovery field (added in M1_02a)
to determine primary/replica role.

Create a team of 2 specialists:

COLLECTOR AGENT:
Create three new files in internal/collector/:

1. replication_lag.go
   - ReplicationLagCollector struct embedding Base
   - Constructor: NewReplicationLagCollector(instanceID string, v version.PGVersion)
   - Name() returns "replication_lag"
   - Interval: 10 seconds
   - Collect(): if ic.IsRecovery is true, return nil, nil (primary only)
   - Single SQL query combining Q37 (byte lag) and Q38 (time lag):
     SELECT from pg_stat_replication with:
     - pg_wal_lsn_diff for pending/write/flush/replay/total bytes
     - EXTRACT(EPOCH FROM write_lag/flush_lag/replay_lag) for seconds
     - COALESCE all nullable columns to 0 or empty string
   - Emit 8 metrics per replica with labels: app_name, client_addr, state
   - Register via init() with RegisterCollector

2. replication_slots.go
   - ReplicationSlotsCollector struct embedding Base + sqlGate version.Gate
   - Constructor: NewReplicationSlotsCollector(instanceID string, v version.PGVersion)
   - Name() returns "replication_slots"
   - Interval: 60 seconds
   - Collect(): runs on BOTH primary and replica (no IsRecovery check)
   - THREE version-gated SQL variants:
     PG 14: slot_name, slot_type, active, retained_bytes, '' as two_phase, '' as conflicting
     PG 15: same but two_phase::text instead of empty string
     PG 16+: same but both two_phase::text and conflicting::text
   - All variants SELECT exactly 6 columns for uniform Scan()
   - retained_bytes via pg_wal_lsn_diff(pg_current_wal_lsn(), restart_lsn), COALESCE to 0
   - Emit 2 metrics per slot: retained_bytes + active (1/0)
   - Labels: slot_name, slot_type, active; conditionally add two_phase/conflicting if non-empty
   - Register via init()

3. replication_status.go
   - ReplicationStatusCollector struct embedding Base
   - Constructor: NewReplicationStatusCollector(instanceID string, v version.PGVersion)
   - Name() returns "replication_status"
   - Interval: 60 seconds
   - Collect(): delegates to collectPrimary() or collectReplica() based on ic.IsRecovery
   - collectPrimary() (Q20): query pg_stat_replication JOIN pg_replication_slots
     WHERE slot_type='physical' AND active=true
     Emit: replication.active_replicas (count) + replication.replica.connected (1 per replica)
     Labels: app_name, client_addr, state, sync_state
   - collectReplica() (Q21): query pg_stat_wal_receiver
     Emit: wal_receiver.connected (1 if status='streaming', 0 otherwise)
     + wal_receiver.lag_bytes (pg_wal_lsn_diff(latest_end_lsn, received_lsn))
     Labels: sender_host, sender_port
     If no WAL receiver row: emit connected=0 with nil labels
   - Register via init()

All collectors:
- Use queryContext(ctx) for 5s statement timeout (from base.go)
- Use c.point() for metric creation (auto-prefixes "pgpulse.")
- Use fmt.Errorf("collector_name: %w", err) for error wrapping
- Handle empty result sets gracefully (return empty slice, not error)
- SQL as const/var string constants — NEVER use fmt.Sprintf for SQL

QA AGENT:
Create three test files in internal/collector/:

1. replication_lag_test.go
   - TestReplicationLag_SkipsOnReplica: pass InstanceContext{IsRecovery: true},
     assert returns nil, nil
   - TestReplicationLag_NameAndInterval: verify Name()="replication_lag", Interval()=10s
   - Add //go:build integration test stub for future Docker-based testing:
     TestReplicationLag_Integration_PG16 (empty body with t.Skip if no Docker)

2. replication_slots_test.go
   - TestReplicationSlots_GateSelectPG14: verify Gate selects PG14 variant
   - TestReplicationSlots_GateSelectPG15: verify Gate selects PG15 variant
   - TestReplicationSlots_GateSelectPG16: verify Gate selects PG16+ variant
   - TestReplicationSlots_GateSelectPG17: verify Gate selects PG16+ variant (same SQL)
   - TestReplicationSlots_NameAndInterval: verify Name()="replication_slots", Interval()=60s
   - Add //go:build integration test stub

3. replication_status_test.go
   - TestReplicationStatus_NameAndInterval: verify Name()="replication_status", Interval()=60s
   - TestReplicationStatus_RoleRouting: verify Collect delegates correctly
     based on InstanceContext.IsRecovery (this tests the branching logic)
   - Add //go:build integration test stubs for primary and replica modes

For all test files:
- Use the same package: package collector
- Follow table-driven test patterns from existing *_test.go files
- Version gate tests: create version.PGVersion with desired Major,
  call Gate.Select(), assert correct SQL or ok==true/false
- Unit tests must NOT require Docker or testcontainers
- Import "testing", "github.com/ios9000/PGPulse_01/internal/version"

COORDINATION:
- Collector Agent creates all three .go files first
- QA Agent creates all three _test.go files
- QA Agent verifies the Gate variable is exported (or package-level)
  so tests can access it
- Both agents can work in parallel since tests reference the
  structs/functions created by Collector Agent

CANNOT RUN BASH:
You cannot run shell commands on this platform (Windows bash bug).
Create files only. When both agents are done, list ALL created files
so the developer can run:

go build ./...
go vet ./...
golangci-lint run
go test ./internal/collector/...
```

---

## Developer Post-Session Checklist

After Agent Teams finish:

```bash
cd ~/Projects/PGPulse_01

# 1. Verify new files exist
ls internal/collector/replication_*.go

# 2. Build
go build ./...

# 3. Vet
go vet ./...

# 4. Lint
golangci-lint run

# 5. Unit tests
go test -v ./internal/collector/...

# 6. If all pass — commit
git add internal/collector/replication_*.go
git commit -m "feat(collector): add replication collectors (Q20, Q21, Q37, Q38, Q40)

- ReplicationLagCollector: byte + time lag per replica (primary only)
- ReplicationSlotsCollector: WAL retention per slot, version-gated (PG 14/15/16+)
- ReplicationStatusCollector: active replicas (primary) / WAL receiver (replica)
- All collectors use InstanceContext.IsRecovery for role-aware behavior
- Unit tests for role routing, version gates, name/interval
- Integration test stubs for future Docker-based CI

PGAM queries ported: Q20, Q21, Q37, Q38, Q40 (5 queries)
Running total: 18/76 queries ported (13 M1_01 + 5 M1_02b)"

git push origin main
```

## Success Criteria

| Check | Expected |
|-------|----------|
| `go build ./...` | ✅ No errors |
| `go vet ./...` | ✅ No warnings |
| `golangci-lint run` | ✅ 0 issues |
| `go test ./internal/collector/...` | ✅ All pass (unit tests) |
| 6 new files created | 3 collectors + 3 test files |
| No existing files modified | ✅ Clean addition |
| No fmt.Sprintf in SQL | ✅ All SQL as string constants |
| COALESCE on all nullable columns | ✅ No nil scan panics |
| Version gate covers PG 14–17+ | ✅ 3 variants in slots gate |
