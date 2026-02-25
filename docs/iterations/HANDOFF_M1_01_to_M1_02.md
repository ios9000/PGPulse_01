# PGPulse — Iteration Handoff: M1_01 → M1_02

> **Purpose:** Upload this file when starting the next Claude.ai chat.
> Contains EVERYTHING needed to begin M1_02 without re-discovery.
> **Created:** 2026-02-25 (end of M1_01)

---

## DO NOT RE-DISCUSS

These decisions are final. The new chat must not revisit them:

1. **Stack**: Go 1.24.0, pgx v5.8.0, chi v5, koanf, slog, testcontainers-go
2. **Architecture**: Single binary, version-adaptive SQL via Gate pattern, Collector interface
3. **Granularity**: One file = one collector = one struct implementing Collector. No merging.
4. **Module ownership**: Collector Agent owns internal/collector/* and internal/version/*
5. **Agent Teams bash bug**: Claude Code cannot run bash on Windows. Agents create files only. Developer runs go build/test/commit manually.
6. **Go module path**: `github.com/ios9000/PGPulse_01`
7. **Project path**: `C:\Users\Archer\Projects\PGPulse_01`
8. **PG version support**: 14, 15, 16, 17 (18 optional)
9. **Monitoring user**: pg_monitor role, never superuser
10. **OS metrics**: via Go agent (M6), NEVER via COPY TO PROGRAM
11. **Metric naming**: `pgpulse.<category>.<metric>` with labels as map[string]string
12. **Statement timeout**: 5s for live dashboard collectors via context.WithTimeout
13. **golangci-lint**: upgraded to v2.10.1 (v1 doesn't support Go 1.24). Config requires `version: "2"` field. `gosimple` linter removed (merged into `staticcheck`).
14. **Docker Desktop**: not available on developer workstation. Integration tests run in CI only for now. Unit tests with mocks work locally.
15. **Checkpoint/bgwriter stats**: deferred to M1_03 (PG 17 splits pg_stat_bgwriter → pg_stat_checkpointer)
16. **pg_stat_io**: deferred to M1_03 (PG 16+ only)

---

## What Exists After M1_01

### Repository Structure (collector-related)

```
internal/
├── collector/
│   ├── collector.go          ← interfaces (MetricPoint, Collector, MetricStore, AlertEvaluator) [M0]
│   ├── base.go               ← shared Base struct, point(), queryContext(), constants [M1_01]
│   ├── server_info.go        ← Q2,Q3,Q9,Q10: start time, uptime, recovery, backup [M1_01]
│   ├── connections.go        ← Q11-Q13: per-state counts, max, reserved, utilization [M1_01]
│   ├── cache.go              ← Q14: global cache hit ratio [M1_01]
│   ├── transactions.go       ← Q15: per-DB commit ratio + deadlocks [M1_01]
│   ├── database_sizes.go     ← Q16: per-DB size bytes [M1_01]
│   ├── settings.go           ← Q17: track_io_timing, shared_buffers, etc. [M1_01]
│   ├── extensions.go         ← Q18-Q19: pgss presence, fill%, stats_reset [M1_01]
│   ├── registry.go           ← RegisterCollector(), CollectAll() with partial-failure [M1_01]
│   ├── testutil_test.go      ← setupPG(), setupPGWithStatements(), metric helpers [M1_01]
│   ├── server_info_test.go   ← PG14 + PG17 version gate tests [M1_01]
│   ├── connections_test.go   ← self-exclusion, utilization tests [M1_01]
│   ├── cache_test.go         ← hit ratio bounds [M1_01]
│   ├── transactions_test.go  ← per-DB labels [M1_01]
│   ├── database_sizes_test.go ← size > 0 [M1_01]
│   ├── settings_test.go      ← bool conversion [M1_01]
│   ├── extensions_test.go    ← with/without pgss paths [M1_01]
│   └── registry_test.go      ← mock-based, no Docker [M1_01]
│
└── version/
    ├── version.go             ← PGVersion, Detect(), AtLeast() [M0]
    └── gate.go                ← Gate, SQLVariant, VersionRange, Select() [M0]
```

### Key Interface: Collector (from collector.go — DO NOT MODIFY)

```go
type MetricPoint struct {
    InstanceID string
    Metric     string
    Value      float64
    Labels     map[string]string
    Timestamp  time.Time
}

type Collector interface {
    Name() string
    Collect(ctx context.Context, conn *pgx.Conn) ([]MetricPoint, error)
    Interval() time.Duration
}
```

### Key Interface: Version Gate (from gate.go — DO NOT MODIFY)

```go
type VersionRange struct {
    MinMajor int
    MinMinor int
    MaxMajor int
    MaxMinor int
}

type SQLVariant struct {
    Range VersionRange
    SQL   string
}

type Gate struct {
    Name     string
    Variants []SQLVariant
}

func (g Gate) Select(v PGVersion) (string, bool)
```

**Important:** Design docs showed `{MinVersion: 140000, MaxVersion: 149999}` but the actual M0 code uses `VersionRange{MinMajor, MinMinor, MaxMajor, MaxMinor}`. Use the struct-based form.

### Base Struct Pattern (from base.go)

```go
type Base struct {
    instanceID string
    pgVersion  version.PGVersion
    interval   time.Duration
}

func newBase(instanceID string, v version.PGVersion, interval time.Duration) Base
func (b *Base) point(metric string, value float64, labels map[string]string) MetricPoint
func (b *Base) Interval() time.Duration
func queryContext(ctx context.Context) (context.Context, context.CancelFunc) // 5s timeout
```

All collectors embed Base and use `b.point()` which auto-prefixes "pgpulse." and fills InstanceID + Timestamp.

---

## Build & Test Status After M1_01

| Check | Result |
|-------|--------|
| `go build ./...` | ✅ Pass |
| `go vet ./...` | ✅ Pass |
| `golangci-lint run` (v2.10.1) | ✅ 0 issues |
| Unit tests (registry mocks) | ✅ 5/5 pass |
| Integration tests (testcontainers) | ❌ Docker Desktop not available on Windows — CI only |

---

## Query Porting Progress

| Source | Queries | Status |
|--------|---------|--------|
| analiz2.php Q1–Q3, Q9–Q19 | 13 | ✅ Done (M0 + M1_01) |
| analiz2.php Q4–Q8 | 5 | ⏭️ Deferred to M6 (OS agent) |
| **analiz2.php Q20–Q41** | **22** | **🔲 M1_02 — THIS ITERATION** |
| analiz2.php Q42–Q47 | 6 | 🔲 M1_03 |
| analiz2.php Q48–Q52 | 5 | 🔲 M1_04 |
| analiz2.php Q53–Q58 | 6 | 🔲 M1_05 |
| analiz_db.php Q1–Q18 | 18 | 🔲 Later milestone |
| **Total** | **76** | **13 done, 6 deferred, 57 remaining** |

---

## Next Task: M1_02 — Replication Collector

### Goal

Port PGAM queries Q20–Q41 into replication collectors. These cover:
- Physical replication: active replicas, lag by phase (pending/write/flush/replay)
- Replication slots: name, type, active status, WAL retention size
- WAL receiver status (on standby servers)
- Logical replication: subscription sync state

### PGAM Queries Q20–Q41 (from PGAM_FEATURE_AUDIT.md in Project Knowledge)

**Q20 — Active physical replicas**
```sql
SELECT
    r.pid, r.usename, r.application_name, r.client_addr,
    r.state, r.sync_state
FROM pg_stat_replication r
JOIN pg_replication_slots s ON r.pid = s.active_pid
WHERE s.slot_type = 'physical' AND s.active = true;
```

**Q21 — WAL receiver (replica side)**
```sql
SELECT
    status, sender_host, sender_port,
    received_lsn, latest_end_lsn
FROM pg_stat_wal_receiver;
```

**Q36 — Replication lag (PG < 10 — OUT OF SCOPE, our min is PG 14)**
```sql
-- Uses pg_xlog_location_diff — not needed
```

**Q37 — Replication lag by phase (PG ≥ 10)**
```sql
SELECT
    application_name,
    client_addr,
    state,
    pg_wal_lsn_diff(pg_current_wal_lsn(), sent_lsn) AS pending_bytes,
    pg_wal_lsn_diff(sent_lsn, write_lsn) AS write_bytes,
    pg_wal_lsn_diff(write_lsn, flush_lsn) AS flush_bytes,
    pg_wal_lsn_diff(flush_lsn, replay_lsn) AS replay_bytes,
    pg_wal_lsn_diff(pg_current_wal_lsn(), replay_lsn) AS total_bytes
FROM pg_stat_replication;
```

**Q38 — Replication lag (time-based, PG ≥ 10)**
```sql
SELECT
    application_name,
    client_addr,
    write_lag,
    flush_lag,
    replay_lag
FROM pg_stat_replication;
```

**Q39 — Replication slots (PG < 10 — OUT OF SCOPE)**
```sql
-- Uses pg_xlog_location_diff — not needed
```

**Q40 — Replication slots (PG ≥ 10)**
```sql
SELECT
    slot_name,
    slot_type,
    active,
    pg_wal_lsn_diff(pg_current_wal_lsn(), restart_lsn) AS retained_bytes
FROM pg_replication_slots;
```
Note: Additional columns by version:
- PG 15+: `two_phase` column exists
- PG 16+: `conflicting` column exists

**Q41 — Logical replication sync state**
```sql
SELECT
    s.subname,
    r.srsubstate,
    r.srrelid::regclass AS relname
FROM pg_subscription_rel r
JOIN pg_subscription s ON s.oid = r.srsubid
WHERE r.srsubstate <> 'r';
```
Note: Requires connecting to each database that has subscriptions.

### Version Gates Required for M1_02

| Gate | PG 14 | PG 15 | PG 16 | PG 17 |
|------|-------|-------|-------|-------|
| Replication slot `two_phase` | ❌ | ✅ column exists | ✅ | ✅ |
| Replication slot `conflicting` | ❌ | ❌ | ✅ column exists | ✅ |
| WAL functions (pg_wal_lsn_diff) | ✅ (PG ≥ 10) | ✅ | ✅ | ✅ |

Since our minimum is PG 14, the WAL function names are stable (`pg_wal_lsn_diff`, `pg_current_wal_lsn`). The gates are only for the slot metadata columns.

### Complexity Assessment

This is **medium-high** complexity because:
1. Replication lag query joins pg_stat_replication with multiple LSN diff calculations
2. Slot query needs version-conditional column selection (PG 15+, PG 16+)
3. Logical replication query requires per-database connection (different from all M1_01 collectors that use a single connection)
4. Some queries only make sense on primary (replication lag) vs replica (WAL receiver)

### Decision Needed: Per-Database Logical Replication

Q41 connects to each database to check `pg_subscription_rel`. This breaks the current Collector pattern (single connection). Options:

- **Option A:** Skip logical replication sync in M1_02, defer to M1_03 or later when we have multi-connection support
- **Option B:** Accept a `[]string` of database names as constructor arg, create per-DB connections inside Collect()
- **Option C:** Create a separate `PerDatabaseCollector` interface that receives a connection factory

This should be discussed in the M1_02 planning chat.

### Suggested Collector Files for M1_02

| File | Queries | Interval | Notes |
|------|---------|----------|-------|
| `replication_lag.go` | Q37, Q38 | 10s | Primary only; byte + time lag per replica |
| `replication_slots.go` | Q40 | 60s | Version-gated columns (PG 15+, 16+) |
| `replication_status.go` | Q20, Q21 | 60s | Active replicas (primary) + WAL receiver (replica) |
| `replication_logical.go` | Q41 | 300s | Per-DB subscription sync — needs design discussion |

---

## Known Issues Affecting M1_02

1. **Docker Desktop unavailable** — integration tests are CI-only. Unit tests with mocks work locally.
2. **Gate struct uses VersionRange** — not raw int min/max. See "Important" note in interfaces section above.
3. **Primary vs replica detection** — server_info.go already collects `pgpulse.server.is_in_recovery`. Replication collectors should check this to decide which queries to run (lag queries are primary-only, WAL receiver is replica-only).
4. **Per-database connections** — current Collector interface takes a single `*pgx.Conn`. Q41 (logical replication) needs connections to multiple databases. Requires design discussion.

---

## Workflow for M1_02

```
1. New Claude.ai chat (this handoff uploaded):
   - Discuss replication query design
   - Decide on logical replication approach (per-DB connection)
   - Produce requirements.md, design.md, team-prompt.md

2. Developer: copy to docs/iterations/M1_02_.../

3. Claude Code: paste team-prompt.md → agents create files

4. Developer: go mod tidy → go build → go vet → golangci-lint run → fix cycle

5. Developer: go test ./internal/collector/... (unit tests)

6. Claude.ai: create session-log.md

7. Developer: git commit + push
```

---

## Environment Reference

| Tool | Version | Notes |
|------|---------|-------|
| Go | 1.24.0 windows/amd64 | Upgraded from 1.23.6 during M1_01 |
| golangci-lint | 2.10.1 (built with go1.26.0) | v2 config: requires `version: "2"`, no `gosimple` |
| Claude Code | 2.1.53 | Bash broken on Windows — file creation only |
| testcontainers-go | 0.40.0 | Requires Docker Desktop on Windows |
| Docker Desktop | Not installed | Integration tests → CI only |
| Git | 2.52.0 | |
| Node.js | 22.14.0 | For Claude Code |
