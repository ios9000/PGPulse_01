# REM_01c — Remediation Rule Metric Key Audit

**Date:** 2026-03-14
**Source:** CODEBASE_DIGEST.md Section 3 (post-REM_01b, commit 6a3bc32)

---

## Summary

**13 of 25 rules reference incorrect metric keys.** The rules were written against
assumed key names; the actual collector output uses different naming.

Additionally, OS metrics have a **dual-prefix problem**: OSSQLCollector emits `os.*`
per the handoff, but CODEBASE_DIGEST shows them as `pg.os.*`. Rules must handle
whichever prefix is actually in the MetricStore.

---

## PG Rule Mismatches (9 rules)

| # | Rule ID | Rule References | Actual Key in Collectors | Fix |
|---|---------|----------------|--------------------------|-----|
| 1 | `rem_conn_high` | `pg.connections.active` | `pg.connections.total` (or use `pg.connections.utilization_pct` directly) | Use `pg.connections.utilization_pct` — already a percentage, no need to compute |
| 1 | `rem_conn_high` | `pg.connections.max_connections` | `pg.connections.max` | Rename |
| 2 | `rem_conn_exhausted` | `pg.connections.active` | same as above | Same fix |
| 2 | `rem_conn_exhausted` | `pg.connections.max_connections` | `pg.connections.max` | Rename |
| 4 | `rem_commit_ratio_low` | `pg.transactions.commit_ratio` | `pg.transactions.commit_ratio_pct` | Append `_pct` |
| 5 | `rem_repl_lag_bytes` | `pg.replication.replay_lag_bytes` | `pg.replication.lag.replay_bytes` | Fix path: `.lag.replay_bytes` |
| 6 | `rem_repl_lag_critical` | `pg.replication.replay_lag_bytes` | `pg.replication.lag.replay_bytes` | Same fix |
| 7 | `rem_repl_slot_inactive` | `pg.replication.slot_inactive` | `pg.replication.slot.active` (0 = inactive) | Change key + invert logic |
| 8 | `rem_long_txn_warn` | `pg.transactions.oldest_active_sec` | `pg.long_transactions.oldest_seconds` | Completely different key path |
| 9 | `rem_long_txn_crit` | `pg.transactions.oldest_active_sec` | `pg.long_transactions.oldest_seconds` | Same fix |
| 11 | `rem_pgss_fill` | `pg.statements.fill_pct` | `pg.extensions.pgss_fill_pct` | Different prefix: `pg.extensions.` |
| 12 | `rem_wraparound_warn` | `pg.server.wraparound_pct` | **DOES NOT EXIST** | No collector emits wraparound. Rule is dead code. |
| 13 | `rem_wraparound_crit` | `pg.server.wraparound_pct` | **DOES NOT EXIST** | Same — dead code |
| 16 | `rem_bloat_high` | `pg.db.bloat.ratio` | `pg.db.bloat.table_ratio` | Add `.table_` |
| 17 | `rem_bloat_extreme` | `pg.db.bloat.ratio` | `pg.db.bloat.table_ratio` | Same fix |

## OS Rule Dual-Prefix Issue (8 rules)

OSSQLCollector (default, SQL-based) emits with prefix shown in CODEBASE_DIGEST.
OSCollector (agent-based) emits with `os.*` prefix.

| # | Rule ID | Rule References | OSSQLCollector Key | OSCollector Key | Fix |
|---|---------|----------------|-------------------|-----------------|-----|
| 18 | `rem_cpu_high` | `os.cpu.user_pct` | `pg.os.cpu.user_pct` | `os.cpu.user_pct` | Check BOTH prefixes |
| 19 | `rem_cpu_iowait` | `os.cpu.iowait_pct` | `pg.os.cpu.iowait_pct` | `os.cpu.iowait_pct` | Check BOTH prefixes |
| 20 | `rem_mem_pressure` | `os.memory.available_kb` | `pg.os.memory.available_kb` | `os.memory.available_kb` | Check BOTH prefixes |
| 21 | `rem_mem_overcommit` | `os.memory.committed_as_kb` | `pg.os.memory.committed_as_kb` | `os.memory.committed_as_kb` | Check BOTH prefixes |
| 22 | `rem_load_high` | `os.load.1m` | `pg.os.load.1m` | `os.load.1m` | Check BOTH prefixes |
| 23 | `rem_disk_util` | `os.disk.util_pct` | `pg.os.disk.util_pct` | `os.disk.util_pct` | Check BOTH prefixes |
| 24 | `rem_disk_read_latency` | `os.disk.read_await_ms` | `pg.os.disk.read_await_ms` | `os.disk.read_await_ms` | Check BOTH prefixes |
| 25 | `rem_disk_write_latency` | `os.disk.write_await_ms` | `pg.os.disk.write_await_ms` | `os.disk.write_await_ms` | Check BOTH prefixes |

## Rules That Are Correct (4 rules)

| # | Rule ID | Key | Status |
|---|---------|-----|--------|
| 3 | `rem_cache_low` | `pg.cache.hit_ratio` | ✅ Correct |
| 10 | `rem_locks_blocking` | `pg.locks.blocked_count` | ✅ Correct |
| 14 | `rem_track_io` | `pg.settings.track_io_timing` | ✅ Correct |
| 15 | `rem_deadlocks` | `pg.transactions.deadlocks` | ✅ Correct |

## Wraparound — Missing Collector

Rules 12 and 13 (`rem_wraparound_warn`, `rem_wraparound_crit`) reference `pg.server.wraparound_pct`
which **no collector emits**. Options:
- A) Remove these rules (defer until a WraparoundCollector exists)
- B) Add a simple wraparound metric to ServerInfoCollector (query `age(datfrozenxid)` from pg_database)

## Recommended OS Prefix Fix

Add a helper function to OS rules:

```go
// getOS returns the first found value from either os.* or pg.os.* prefix
func getOS(snap MetricSnapshot, suffix string) (float64, bool) {
    if v, ok := snap.Get("os." + suffix); ok {
        return v, true
    }
    return snap.Get("pg.os." + suffix)
}
```

Then OS rules call `getOS(ctx.Snapshot, "cpu.user_pct")` instead of `ctx.Snapshot.Get("os.cpu.user_pct")`.
