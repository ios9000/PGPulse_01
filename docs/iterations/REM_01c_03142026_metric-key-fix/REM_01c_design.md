# REM_01c — Design Document

**Iteration:** REM_01c — Remediation Rule Metric Key Fix (bugfix)
**Date:** 2026-03-14
**Follows:** REM_01b (commits fcf45b4, 6a3bc32)

---

## 1. PG Rule Key Corrections

### Exact Replacements in internal/remediation/rules_pg.go

| Rule ID | Old Key | New Key | Notes |
|---------|---------|---------|-------|
| `rem_conn_high` | `pg.connections.active` | `pg.connections.utilization_pct` | Use pre-computed percentage directly |
| `rem_conn_high` | `pg.connections.max_connections` | *(remove — no longer needed)* | utilization_pct eliminates division |
| `rem_conn_exhausted` | `pg.connections.active` | `pg.connections.utilization_pct` | Same — use pre-computed pct |
| `rem_conn_exhausted` | `pg.connections.max_connections` | *(remove)* | Same |
| `rem_commit_ratio_low` | `pg.transactions.commit_ratio` | `pg.transactions.commit_ratio_pct` | Append `_pct` |
| `rem_repl_lag_bytes` | `pg.replication.replay_lag_bytes` | `pg.replication.lag.replay_bytes` | Fix path hierarchy |
| `rem_repl_lag_critical` | `pg.replication.replay_lag_bytes` | `pg.replication.lag.replay_bytes` | Same |
| `rem_repl_slot_inactive` | `pg.replication.slot_inactive` | `pg.replication.slot.active` | Invert logic: active=0 means inactive |
| `rem_long_txn_warn` | `pg.transactions.oldest_active_sec` | `pg.long_transactions.oldest_seconds` | Completely different key path |
| `rem_long_txn_crit` | `pg.transactions.oldest_active_sec` | `pg.long_transactions.oldest_seconds` | Same |
| `rem_pgss_fill` | `pg.statements.fill_pct` | `pg.extensions.pgss_fill_pct` | Different prefix |
| `rem_wraparound_warn` | `pg.server.wraparound_pct` | `pg.server.wraparound_pct` | Key unchanged — add to collector |
| `rem_wraparound_crit` | `pg.server.wraparound_pct` | `pg.server.wraparound_pct` | Same |
| `rem_bloat_high` | `pg.db.bloat.ratio` | `pg.db.bloat.table_ratio` | Add `.table_` |
| `rem_bloat_extreme` | `pg.db.bloat.ratio` | `pg.db.bloat.table_ratio` | Same |

### Connection Rules — Logic Simplification

The connection rules currently compute percentage from active/max. Since `pg.connections.utilization_pct` already exists as a pre-computed gauge, simplify:

**Before (rem_conn_high):**
```go
// Alert-triggered mode
if ctx.MetricKey == "pg.connections.active" {
    maxConn, ok := ctx.Snapshot.Get("pg.connections.max_connections")
    if !ok || maxConn == 0 { return nil }
    pct := (ctx.Value / maxConn) * 100
    if pct > 80 && pct < 99 { ... }
}
// Diagnose mode
active, ok1 := ctx.Snapshot.Get("pg.connections.active")
maxConn, ok2 := ctx.Snapshot.Get("pg.connections.max_connections")
...
```

**After (rem_conn_high):**
```go
// Alert-triggered mode
if ctx.MetricKey == "pg.connections.utilization_pct" {
    if ctx.Value > 80 && ctx.Value < 99 {
        return &RuleResult{
            Title:       "Consider connection pooling",
            Description: fmt.Sprintf(
                "Connection utilization at %.0f%%. "+
                    "Consider adding PgBouncer or increasing max_connections. "+
                    "Review application connection pool settings for idle connections.",
                ctx.Value),
            DocURL: "https://www.pgbouncer.org/",
        }
    }
    return nil
}
// Diagnose mode
pct, ok := ctx.Snapshot.Get("pg.connections.utilization_pct")
if !ok { return nil }
if pct > 80 && pct < 99 { ... }
```

### Replication Slot Rule — Logic Inversion

The collector emits `pg.replication.slot.active` where 1=active, 0=inactive.
The rule needs to detect inactive slots.

**Before:**
```go
if ctx.MetricKey == "pg.replication.slot_inactive" {
    if ctx.Value > 0 { ... }
}
```

**After:**
```go
if ctx.MetricKey == "pg.replication.slot.active" {
    if ctx.Value == 0 { // 0 means inactive
        return &RuleResult{ ... }
    }
    return nil
}
// Diagnose mode: check all slot.active metrics — any with value 0?
// Note: slot metrics have labels (slot_name, slot_type, active)
// In snapshot mode, we only get the latest value per key.
// Since labels make each slot a unique key entry, this won't work
// for Diagnose. Mark Diagnose as unsupported for this rule.
```

**IMPORTANT:** Slot metrics have per-slot labels, which means the snapshot
will have multiple entries keyed differently. For Diagnose mode, this rule
should gracefully skip (return nil) since we can't iterate labeled metrics
in the simple snapshot map. The alert-triggered path works because the
evaluator fires per-metric-point.

---

## 2. OS Rule Dual-Prefix Support

### Helper Function — internal/remediation/rules_os.go

Add at the top of the file:

```go
// getOS looks up an OS metric in the snapshot, checking both the agent prefix
// (os.*) and the SQL collector prefix (pg.os.*).
func getOS(snap MetricSnapshot, suffix string) (float64, bool) {
    if v, ok := snap.Get("os." + suffix); ok {
        return v, true
    }
    return snap.Get("pg.os." + suffix)
}
```

### Apply to All 8 OS Rules

Replace all `ctx.Snapshot.Get("os.cpu.user_pct")` calls with `getOS(ctx.Snapshot, "cpu.user_pct")`.

Also update alert-triggered `ctx.MetricKey` checks to match BOTH prefixes:

```go
// Before
if ctx.MetricKey == "os.cpu.iowait_pct" { ... }

// After
if ctx.MetricKey == "os.cpu.iowait_pct" || ctx.MetricKey == "pg.os.cpu.iowait_pct" { ... }
```

### Full OS Rule Key Mapping

| Rule | Snapshot Lookup | Alert MetricKey Match |
|------|----------------|----------------------|
| `rem_cpu_high` | `getOS(snap, "cpu.user_pct")` + `getOS(snap, "cpu.system_pct")` | `os.cpu.user_pct` OR `pg.os.cpu.user_pct` |
| `rem_cpu_iowait` | `getOS(snap, "cpu.iowait_pct")` | `os.cpu.iowait_pct` OR `pg.os.cpu.iowait_pct` |
| `rem_mem_pressure` | `getOS(snap, "memory.available_kb")` + `getOS(snap, "memory.total_kb")` | `os.memory.available_kb` OR `pg.os.memory.available_kb` |
| `rem_mem_overcommit` | `getOS(snap, "memory.committed_as_kb")` + `getOS(snap, "memory.commit_limit_kb")` | `os.memory.committed_as_kb` OR `pg.os.memory.committed_as_kb` |
| `rem_load_high` | `getOS(snap, "load.1m")` | `os.load.1m` OR `pg.os.load.1m` |
| `rem_disk_util` | `getOS(snap, "disk.util_pct")` | `os.disk.util_pct` OR `pg.os.disk.util_pct` |
| `rem_disk_read_latency` | `getOS(snap, "disk.read_await_ms")` | `os.disk.read_await_ms` OR `pg.os.disk.read_await_ms` |
| `rem_disk_write_latency` | `getOS(snap, "disk.write_await_ms")` | `os.disk.write_await_ms` OR `pg.os.disk.write_await_ms` |

---

## 3. Add Wraparound Metric to ServerInfoCollector

### Modify: internal/collector/server_info.go

Add one metric to the ServerInfoCollector's Collect method:

```go
// After existing server info queries, add:
var wraparoundPct float64
err := conn.QueryRow(ctx,
    "SELECT COALESCE(max(age(datfrozenxid))::float / 2147483647 * 100, 0) FROM pg_database WHERE datallowconn",
).Scan(&wraparoundPct)
if err != nil {
    slog.Warn("server_info: failed to collect wraparound", "error", err)
} else {
    points = append(points, collector.MetricPoint{
        InstanceID: ic.InstanceID,  // however the collector gets instanceID
        Metric:     b.point("server.wraparound_pct"),
        Value:      wraparoundPct,
        Timestamp:  time.Now(),
    })
}
```

**NOTE:** Read server_info.go first to understand:
- How it gets the instance ID (from InstanceContext? From a field?)
- How it constructs MetricPoints (does it use `b.point()` or manual prefix?)
- Where in the Collect method to add this query

The SQL is simple and safe:
- `age(datfrozenxid)` returns the transaction age
- `2147483647` is the max XID before wraparound
- Result is a percentage (0-100)
- `datallowconn` filters out template databases

---

## 4. Test Updates

### internal/remediation/rules_test.go

All table-driven test cases that reference the old metric keys must be updated.

**Key changes in test snapshots:**

| Old Snapshot Key | New Snapshot Key |
|-----------------|-----------------|
| `"pg.connections.active": 90` | `"pg.connections.utilization_pct": 85` |
| `"pg.connections.max_connections": 100` | *(remove from snapshot)* |
| `"pg.transactions.commit_ratio": 0.85` | `"pg.transactions.commit_ratio_pct": 85` |
| `"pg.replication.replay_lag_bytes": 50e6` | `"pg.replication.lag.replay_bytes": 50e6` |
| `"pg.replication.slot_inactive": 1` | `"pg.replication.slot.active": 0` |
| `"pg.transactions.oldest_active_sec": 120` | `"pg.long_transactions.oldest_seconds": 120` |
| `"pg.statements.fill_pct": 96` | `"pg.extensions.pgss_fill_pct": 96` |
| `"pg.db.bloat.ratio": 3.0` | `"pg.db.bloat.table_ratio": 3.0` |
| `"os.cpu.user_pct": 50` | Keep (agent prefix), OR add `"pg.os.cpu.user_pct": 50` variant |

**Add new test cases:**
- Test `getOS()` helper with both prefixes
- Test `pg.server.wraparound_pct` rules with correct key
- Test OS rules fire with `pg.os.*` prefix (OSSQLCollector path)

### internal/collector/server_info_test.go

Add a test case for the wraparound metric:
- Verify `pg.server.wraparound_pct` is emitted
- Verify value is between 0 and 100

---

## 5. File Inventory

### Modified Files (4)

| File | Change | Owner |
|------|--------|-------|
| `internal/remediation/rules_pg.go` | Fix 9 metric key references + simplify connection rules | Agent |
| `internal/remediation/rules_os.go` | Add `getOS()` helper + update all 8 rules for dual prefix | Agent |
| `internal/collector/server_info.go` | Add `pg.server.wraparound_pct` metric | Agent |
| `internal/remediation/rules_test.go` | Update all affected test snapshots + add new test cases | Agent |

### Possibly Modified (1)

| File | Change | Owner |
|------|--------|-------|
| `internal/collector/server_info_test.go` | Add wraparound metric test | Agent |

### Estimated Scope

~150 lines changed across 4-5 files. No new files, no new dependencies, no migration.
