# REM_01c Session Log
## Remediation Metric Key Fix

**Date:** 2026-03-14
**Iteration:** REM_01c
**Parent:** REM_01 (Remediation)
**Commit:** bff2791

---

## Goal

Fix 13 broken remediation rules whose metric keys didn't match actual collector output.
Single-agent bugfix — no new packages, no new files, no migrations.

---

## Changes

### internal/remediation/rules_pg.go
- `rem_conn_high` / `rem_conn_exhausted`: replaced `pg.connections.active` + `pg.connections.max_connections` with `pg.connections.utilization_pct` (already a percentage, no division needed)
- `rem_commit_ratio_low`: replaced `pg.transactions.commit_ratio` with `pg.transactions.commit_ratio_pct`
- `rem_repl_lag_bytes` / `rem_repl_lag_critical`: replaced `pg.replication.replay_lag_bytes` with `pg.replication.lag.replay_bytes`
- `rem_repl_slot_inactive`: replaced `pg.replication.slot_inactive` (>0) with `pg.replication.slot.active` (==0, inverted logic); Diagnose mode returns nil (per-slot labels make snapshot lookup unreliable)
- `rem_long_txn_warn` / `rem_long_txn_crit`: replaced `pg.transactions.oldest_active_sec` with `pg.long_transactions.oldest_seconds`
- `rem_pgss_fill`: replaced `pg.statements.fill_pct` with `pg.extensions.pgss_fill_pct`
- `rem_bloat_high` / `rem_bloat_extreme`: replaced `pg.db.bloat.ratio` with `pg.db.bloat.table_ratio`
- `rem_wraparound_warn` / `rem_wraparound_crit`: key unchanged (`pg.server.wraparound_pct`), metric now emitted by collector

### internal/remediation/rules_os.go
- Added `getOS(snap, suffix)` helper — checks both `os.*` and `pg.os.*` prefixes
- Added `isOSMetric(key, suffix)` helper — matches alert-triggered metric key against both prefixes
- Updated all 8 OS rules (`rem_cpu_high`, `rem_cpu_iowait`, `rem_mem_pressure`, `rem_mem_overcommit`, `rem_load_high`, `rem_disk_util`, `rem_disk_read_latency`, `rem_disk_write_latency`) to use these helpers

### internal/collector/server_info.go
- Added `pg.server.wraparound_pct` metric: `max(age(datfrozenxid))::float / 2147483647 * 100` from `pg_database`
- Graceful failure with slog.Warn on error (does not abort collection)

### internal/remediation/rules_test.go
- Updated all snapshot keys across existing test cases to match new metric keys
- Updated slot rule test: value 0 = inactive (positive), value 1 = active (negative)
- Added `TestGetOS_BothPrefixes` — verifies `getOS` finds values with either prefix, os.* priority
- Added `TestOSRules_PGOSPrefix` — verifies OS rules fire with `pg.os.*` prefixed keys
- Added `TestWraparound_Fires` — verifies wraparound rules fire/don't fire at correct thresholds

### internal/remediation/engine_test.go
- Updated all connection-related snapshot keys from `pg.connections.active` + `max_connections` to `pg.connections.utilization_pct`

---

## Verification

| Check | Result |
|-------|--------|
| `go build ./cmd/... ./internal/...` | Clean |
| `go test ./cmd/... ./internal/... -count=1` | All pass (17 packages) |
| `golangci-lint run ./cmd/... ./internal/...` | 0 issues |
| `npm run build` | Clean |
| `npm run typecheck` | Clean |
| `npm run lint` | Clean (1 pre-existing warning) |
| Total rules | 25 (unchanged) |
| Test coverage | All 25 rules have test cases |

---

## Metric Key Mapping Reference

| Old Key | New Key | Rules Affected |
|---------|---------|----------------|
| `pg.connections.active` | `pg.connections.utilization_pct` | rem_conn_high, rem_conn_exhausted |
| `pg.connections.max_connections` | *(removed)* | rem_conn_high, rem_conn_exhausted |
| `pg.transactions.commit_ratio` | `pg.transactions.commit_ratio_pct` | rem_commit_ratio_low |
| `pg.replication.replay_lag_bytes` | `pg.replication.lag.replay_bytes` | rem_repl_lag_bytes, rem_repl_lag_critical |
| `pg.replication.slot_inactive` | `pg.replication.slot.active` | rem_repl_slot_inactive |
| `pg.transactions.oldest_active_sec` | `pg.long_transactions.oldest_seconds` | rem_long_txn_warn, rem_long_txn_crit |
| `pg.statements.fill_pct` | `pg.extensions.pgss_fill_pct` | rem_pgss_fill |
| `pg.db.bloat.ratio` | `pg.db.bloat.table_ratio` | rem_bloat_high, rem_bloat_extreme |
| `os.*` only | `os.*` or `pg.os.*` | all 8 OS rules |

---

## Decisions

- **Slot rule in Diagnose mode returns nil**: Per-slot labeled metrics don't map cleanly to the flat snapshot lookup. Alert-triggered mode works correctly since the engine passes the specific metric key.
- **Dual OS prefix via helpers**: Rather than normalizing metric keys at ingestion, we check both prefixes at rule evaluation time. This is simpler and handles both agent-sourced (`os.*`) and SQL-collector-sourced (`pg.os.*`) metrics transparently.
- **Wraparound metric placement**: Added to `ServerInfoCollector` (medium frequency, 60s) since it queries `pg_database` which is a lightweight catalog scan.
