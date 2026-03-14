# REM_01c — Team Prompt

**Paste this into Claude Code. Single-agent bugfix — no team spawn needed.**

---

Fix 13 broken remediation rules whose metric keys don't match the actual collector output.
Read CLAUDE.md for project context, then read docs/iterations/REM_01c_03142026_metric-key-fix/design.md for the exact changes.

This is a bugfix iteration. No new packages, no new files, no migrations.

## Step 1: Read the Actual Code

Before making ANY changes, read these files to understand what exists:

```
internal/remediation/rules_pg.go
internal/remediation/rules_os.go
internal/remediation/rules_test.go
internal/collector/server_info.go
```

Also read `docs/CODEBASE_DIGEST.md` Section 3 (Metric Key Catalog) to verify the correct key names.

## Step 2: Fix PG Rules — internal/remediation/rules_pg.go

### Connection rules (rem_conn_high, rem_conn_exhausted)
- Replace `pg.connections.active` with `pg.connections.utilization_pct`
- Remove all references to `pg.connections.max_connections`
- Simplify logic: `pg.connections.utilization_pct` is already a percentage (0-100), no division needed
- `rem_conn_high`: fires when utilization_pct > 80 AND < 99
- `rem_conn_exhausted`: fires when utilization_pct >= 99

### Commit ratio rule (rem_commit_ratio_low)
- Replace `pg.transactions.commit_ratio` with `pg.transactions.commit_ratio_pct`
- Value is already a percentage — verify threshold logic matches (< 90 means < 90%)

### Replication lag rules (rem_repl_lag_bytes, rem_repl_lag_critical)
- Replace `pg.replication.replay_lag_bytes` with `pg.replication.lag.replay_bytes`
- Thresholds unchanged (> 10MB and > 100MB)

### Replication slot rule (rem_repl_slot_inactive)
- Replace `pg.replication.slot_inactive` with `pg.replication.slot.active`
- INVERT logic: the collector emits 1 for active, 0 for inactive
- Alert-triggered: fire when `ctx.Value == 0` (slot is inactive)
- Diagnose mode: this rule has labeled metrics (per-slot), simple snapshot lookup won't work reliably. Return nil in Diagnose mode for this rule (add a comment explaining why).

### Long transaction rules (rem_long_txn_warn, rem_long_txn_crit)
- Replace `pg.transactions.oldest_active_sec` with `pg.long_transactions.oldest_seconds`
- Thresholds unchanged (> 60s and > 300s)

### Statements fill rule (rem_pgss_fill)
- Replace `pg.statements.fill_pct` with `pg.extensions.pgss_fill_pct`
- Threshold unchanged (>= 95)

### Wraparound rules (rem_wraparound_warn, rem_wraparound_crit)
- Key stays as `pg.server.wraparound_pct` — we're adding this to the collector in Step 4
- No changes needed in the rules themselves

### Bloat rules (rem_bloat_high, rem_bloat_extreme)
- Replace `pg.db.bloat.ratio` with `pg.db.bloat.table_ratio`
- Thresholds unchanged (> 2x and > 50x)
- Note: bloat metrics are per-table (labeled), similar issue to slots in Diagnose mode. Keep Diagnose path but acknowledge it may only catch the latest value.

## Step 3: Fix OS Rules — internal/remediation/rules_os.go

### Add helper function at the top of the file:

```go
// getOS looks up an OS metric in the snapshot, checking both the agent prefix
// (os.*) and the SQL collector prefix (pg.os.*).
func getOS(snap MetricSnapshot, suffix string) (float64, bool) {
    if v, ok := snap.Get("os." + suffix); ok {
        return v, true
    }
    return snap.Get("pg.os." + suffix)
}

// isOSMetric checks if a metric key matches an OS metric with either prefix.
func isOSMetric(key, suffix string) bool {
    return key == "os."+suffix || key == "pg.os."+suffix
}
```

### Update all 8 OS rules:

For each rule, change:
1. Alert-triggered `ctx.MetricKey` check: use `isOSMetric(ctx.MetricKey, "cpu.user_pct")` instead of `ctx.MetricKey == "os.cpu.user_pct"`
2. Snapshot lookups: use `getOS(ctx.Snapshot, "cpu.user_pct")` instead of `ctx.Snapshot.Get("os.cpu.user_pct")`

Apply to all 8 rules:
- `rem_cpu_high`: `cpu.user_pct` + `cpu.system_pct`
- `rem_cpu_iowait`: `cpu.iowait_pct`
- `rem_mem_pressure`: `memory.available_kb` + `memory.total_kb`
- `rem_mem_overcommit`: `memory.committed_as_kb` + `memory.commit_limit_kb`
- `rem_load_high`: `load.1m`
- `rem_disk_util`: `disk.util_pct`
- `rem_disk_read_latency`: `disk.read_await_ms`
- `rem_disk_write_latency`: `disk.write_await_ms`

## Step 4: Add Wraparound Metric — internal/collector/server_info.go

**FIRST:** Read server_info.go to understand:
- How MetricPoints are constructed (does it use `b.point()` or manual string?)
- How the collector accesses the connection and instance context
- Where in the Collect method to add the new query

**THEN:** Add after existing server info queries:

```go
// Wraparound risk: max transaction age as percentage of 2B limit
var wraparoundPct float64
err = conn.QueryRow(ctx,
    "SELECT COALESCE(max(age(datfrozenxid))::float / 2147483647 * 100, 0) FROM pg_database WHERE datallowconn",
).Scan(&wraparoundPct)
if err != nil {
    slog.Warn("server_info: failed to collect wraparound", "error", err)
} else {
    points = append(points, MetricPoint{
        // Use the same pattern as other points in this collector
        Metric: b.point("server.wraparound_pct"),
        Value:  wraparoundPct,
        // Copy InstanceID and Timestamp from existing points pattern
    })
}
```

Adapt to match the exact patterns used in the file. DO NOT guess the struct field names — read the code first.

## Step 5: Update Tests — internal/remediation/rules_test.go

**FIRST:** Read rules_test.go to understand the table-driven test structure.

**THEN:** Update every affected test case's snapshot keys:

| Old Snapshot Key | New Snapshot Key |
|-----------------|-----------------|
| `"pg.connections.active"` | `"pg.connections.utilization_pct"` |
| `"pg.connections.max_connections"` | *(remove)* |
| `"pg.transactions.commit_ratio"` | `"pg.transactions.commit_ratio_pct"` |
| `"pg.replication.replay_lag_bytes"` | `"pg.replication.lag.replay_bytes"` |
| `"pg.replication.slot_inactive"` | `"pg.replication.slot.active"` |
| `"pg.transactions.oldest_active_sec"` | `"pg.long_transactions.oldest_seconds"` |
| `"pg.statements.fill_pct"` | `"pg.extensions.pgss_fill_pct"` |
| `"pg.db.bloat.ratio"` | `"pg.db.bloat.table_ratio"` |

Also update test values where logic changed:
- Connection rules: snapshot values should be percentages (e.g., 85 not 90/100)
- Slot rule: value 0 means inactive (positive case), value 1 means active (negative case)

Add new test cases:
- `TestGetOS_BothPrefixes` — verify getOS finds values with either prefix
- `TestOSRules_PGOSPrefix` — verify OS rules fire with `pg.os.*` prefixed keys
- `TestWraparound_Fires` — verify wraparound rules fire with `pg.server.wraparound_pct` in snapshot

## Step 6: Build Verification

```bash
go build ./cmd/... ./internal/...
go test ./cmd/... ./internal/... -count=1
golangci-lint run ./cmd/... ./internal/...
cd web && npm run build && npm run typecheck && npm run lint
```

All must pass. Then commit:

```bash
git add -A
git commit -m "fix(remediation): correct 13 metric key mismatches, add wraparound metric, dual OS prefix support"
```

## CRITICAL RULES:
- Read EVERY file before modifying it
- This is a BUGFIX — do not add new rules, new features, or refactor beyond what's needed
- Test scope: `go test ./cmd/... ./internal/...` (not `./...`)
- Verify all 25 rules still have test coverage after changes
- Do not change the remediation API, store, or engine — only rules, collector, and tests
