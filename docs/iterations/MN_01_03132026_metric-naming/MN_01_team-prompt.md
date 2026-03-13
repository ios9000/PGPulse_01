# MN_01 — Metric Naming Standardization: Team Prompt

**Paste this into Claude Code to spawn the agent team.**

---

Rename all metric keys in PGPulse to conform to the naming standard from the
competitive research session. Read `docs/iterations/MN_01_03132026_metric-naming/design.md`
for the complete mapping table and implementation plan.

## Context

PGPulse has ~157 metric keys with inconsistent prefixes:
- `pgpulse.*` is used for all PG metrics (should be `pg.*`)
- `os.diskstat.*` should be `os.disk.*`
- OS metrics via SQL path get `pgpulse.os.*` prefix (should be `os.*`)
- `cluster.*` stays as-is (decision D200 — no change)

The naming standard uses four top-level prefixes:
- `pg.` = PostgreSQL metrics
- `os.` = OS metrics (both agent and SQL paths)
- `cluster.` = HA infrastructure (Patroni, etcd) — UNCHANGED
- `pgpulse.` = reserved for internal/meta (none exist)

**CRITICAL RULE:** Every rename must be applied atomically across all layers.
A partial rename (e.g., collector changed but frontend not) breaks the dashboard.

Create a team of 4 specialists:

---

## COLLECTOR AGENT

**Territory:** `internal/collector/*`, `internal/agent/*`, `cmd/pgpulse-agent/`

### Task 1: Change Base.point() prefix
Find the `point()` method on the Base struct (likely in `internal/collector/base.go`
or wherever the Base collector struct is defined). Change the prefix from `"pgpulse."`
to `"pg."`. This single change renames ~120 PG metric keys automatically.

### Task 2: Fix OSSQLCollector prefix
After Task 1, `Base.point("os.cpu.user_pct")` would produce `"pg.os.cpu.user_pct"` —
wrong. OS metrics must use `os.*` prefix without any additional prefix.

In `internal/collector/os_sql.go`, modify OSSQLCollector to emit metric names
directly as `"os.*"` WITHOUT going through `Base.point()`. Search for all uses of
`b.point()` or `c.point()` or similar that produce `os.*` keys and replace with
direct string literals or a helper that does NOT add the `pg.` prefix.

### Task 3: Rename diskstat → disk hierarchy (7 keys)
In ALL of these files, replace `os.diskstat.` with `os.disk.` in metric key strings:
- `internal/collector/os_sql.go`
- `internal/collector/os.go`
- `internal/agent/osmetrics_linux.go`
- `internal/agent/osmetrics.go`

Specific renames:
- `os.diskstat.read_kb` → `os.disk.read_bytes_per_sec`
- `os.diskstat.write_kb` → `os.disk.write_bytes_per_sec`
- `os.diskstat.reads_completed` → `os.disk.reads_completed`
- `os.diskstat.writes_completed` → `os.disk.writes_completed`
- `os.diskstat.read_await_ms` → `os.disk.read_await_ms`
- `os.diskstat.write_await_ms` → `os.disk.write_await_ms`
- `os.diskstat.util_pct` → `os.disk.util_pct`

**IMPORTANT — VERIFY VALUE UNITS:** Check `ParseDiskStats()` in
`internal/agent/osmetrics_linux.go`. If `read_kb` currently returns kilobytes,
multiply the value by 1024 when renaming to `read_bytes_per_sec`. If it already
returns bytes/sec, only the name changes. Document what you find.

### Task 4: DO NOT touch ClusterCollector
`cluster.*` keys stay as-is. No changes to `internal/collector/cluster.go`.

### Task 5: Update all test files
Update metric key assertions in:
- `internal/agent/osmetrics_test.go`
- `internal/agent/scraper_test.go`
- Any `internal/collector/*_test.go` files that assert specific metric key strings
- The `pgpulse.` prefix in test expectations becomes `pg.`
- The `os.diskstat.` in test expectations becomes `os.disk.`

### Verification
```bash
# Must return ZERO matches (excluding comments and this design doc)
grep -rn '"pgpulse\.' internal/collector/ internal/agent/ cmd/
# Must return ZERO matches
grep -rn 'os\.diskstat' internal/collector/ internal/agent/ cmd/
# cluster.go must NOT contain "pg.cluster" (keys stay as cluster.*)
grep -n '"pg\.cluster' internal/collector/cluster.go
# Build
go build ./cmd/pgpulse-server ./cmd/pgpulse-agent
go test ./internal/collector/... ./internal/agent/... -count=1
```

---

## API & SECURITY AGENT

**Territory:** `internal/api/*`, `internal/alert/*`, `internal/ml/*`,
`internal/storage/*`, `migrations/*`, `configs/*`

### Task 1: Update alert seed rules
In `internal/alert/seed.go`, update all metric key strings from
`"pgpulse.*"` to `"pg.*"`. Leave any `cluster.*` or `os.*` references unchanged.

### Task 2: Update alert evaluator and tests
- Check `internal/alert/evaluator.go` for hardcoded metric keys → rename
- Update `internal/alert/evaluator_test.go` — metric key assertions
- Update `internal/alert/evaluator_forecast_test.go` — metric key assertions
- Update `internal/alert/rules_test.go` if it references metric keys
- Update `internal/alert/seed_test.go` if it references metric keys

### Task 3: Update ML detector and config
- Check `internal/ml/detector.go` for metric key references → rename
- Check `internal/ml/config.go` for metric key constants → rename
- Update any `internal/ml/*_test.go` files with metric key assertions

### Task 4: Update API handlers
Check ALL files in `internal/api/` for hardcoded metric key strings.
Common places: `instances.go`, `metrics.go`, `os.go`, `forecast.go`.
Rename `"pgpulse.*"` → `"pg.*"`, `"os.diskstat.*"` → `"os.disk.*"`.
Update test files too.

### Task 5: Create migration script
1. Check the current highest migration number: `ls migrations/ | sort -n | tail -1`
2. Create `migrations/NNN_metric_naming_standardization.sql`:

```sql
BEGIN;

-- Bulk rename pgpulse.* → pg.* in metrics table (except OS metrics)
UPDATE metrics
SET metric = 'pg.' || substring(metric FROM 9)
WHERE metric LIKE 'pgpulse.%'
  AND metric NOT LIKE 'pgpulse.os.%';

-- Fix OS metrics from SQL path: pgpulse.os.* → os.*
UPDATE metrics
SET metric = substring(metric FROM 9)
WHERE metric LIKE 'pgpulse.os.%';

-- Rename os.diskstat.* → os.disk.*
UPDATE metrics
SET metric = replace(metric, 'os.diskstat.', 'os.disk.')
WHERE metric LIKE 'os.diskstat.%';

-- Specific diskstat unit renames
UPDATE metrics SET metric = 'os.disk.read_bytes_per_sec'
WHERE metric IN ('os.disk.read_kb', 'os.diskstat.read_kb');
UPDATE metrics SET metric = 'os.disk.write_bytes_per_sec'
WHERE metric IN ('os.disk.write_kb', 'os.diskstat.write_kb');

-- Alert rules — same renames
UPDATE alert_rules
SET metric = 'pg.' || substring(metric FROM 9)
WHERE metric LIKE 'pgpulse.%'
  AND metric NOT LIKE 'pgpulse.os.%';

UPDATE alert_rules
SET metric = substring(metric FROM 9)
WHERE metric LIKE 'pgpulse.os.%';

UPDATE alert_rules
SET metric = replace(metric, 'os.diskstat.', 'os.disk.')
WHERE metric LIKE 'os.diskstat.%';

-- ML baseline snapshots — same renames
UPDATE ml_baseline_snapshots
SET metric_key = 'pg.' || substring(metric_key FROM 9)
WHERE metric_key LIKE 'pgpulse.%'
  AND metric_key NOT LIKE 'pgpulse.os.%';

UPDATE ml_baseline_snapshots
SET metric_key = substring(metric_key FROM 9)
WHERE metric_key LIKE 'pgpulse.os.%';

UPDATE ml_baseline_snapshots
SET metric_key = replace(metric_key, 'os.diskstat.', 'os.disk.')
WHERE metric_key LIKE 'os.diskstat.%';

COMMIT;
```

NOTE: `cluster.*` keys need NO migration — unchanged per D200.

### Task 6: Update config files
Update `config.sample.yaml` and any example configs in `configs/`:
- `ml.metrics[].key` values: `pgpulse.*` → `pg.*`
- Any hardcoded metric keys in comments

### Verification
```bash
grep -rn '"pgpulse\.' internal/alert/ internal/ml/ internal/api/ internal/storage/ configs/
grep -rn 'os\.diskstat' internal/alert/ internal/ml/ internal/api/ internal/storage/ configs/
go test ./internal/alert/... ./internal/ml/... ./internal/api/... -count=1
```

---

## FRONTEND AGENT

**Territory:** `web/src/**/*`

### Task 1: Update metric key constants
Find `web/src/lib/constants.ts` and update all metric key constants:
- `"pgpulse.*"` → `"pg.*"`
- `"os.diskstat.*"` → `"os.disk.*"`
- Leave `"cluster.*"` and `"os.*"` (non-diskstat) unchanged

### Task 2: Update chart components
Update all chart/display components that reference metric keys:
- `ConnectionsChart.tsx` — `pg.connections.*`
- `CacheHitRatioChart.tsx` — `pg.cache.hit_ratio`
- `TransactionCommitRatioChart.tsx` — `pg.transactions.commit_ratio_pct`
- `ReplicationLagChart.tsx` — `pg.replication.lag.replay_bytes`
- `OSMetricsSection.tsx` — `os.disk.*` (was `os.diskstat.*`)
- `KeyMetricsRow.tsx` — various `pg.*` keys
- `InstanceCard.tsx` or `FleetOverview.tsx` — fleet card metric lookups
- Any other component referencing `"pgpulse."` strings

### Task 3: Update hooks
Update metric key parameters in hooks:
- `useMetrics.ts` — history query keys
- `useOSMetrics.ts` — OS metric key references
- `useForecast.ts` / `useForecastChart.ts` — forecast metric keys
- `useCurrentMetrics.ts` — if it filters by key prefix

### Task 4: Update pages
Check all page files for inline metric key strings:
- `ServerDetail.tsx`
- `DatabaseDetail.tsx`
- `FleetOverview.tsx`

### Task 5: Systematic grep
After all changes, run:
```bash
grep -rn 'pgpulse\.' web/src/
grep -rn 'diskstat' web/src/
```
Both must return ZERO matches.

### Verification
```bash
cd web && npm run build && npm run typecheck && npm run lint
```

---

## QA AGENT

**Territory:** All `*_test.go` files, verification scripts

### Task 1: Cross-layer consistency audit
After all agents finish, verify no orphaned old keys remain:

```bash
# Go backend — no pgpulse. metric keys (except comments)
grep -rn '"pgpulse\.' internal/ cmd/ | grep -v '_test.go' | grep -v '\.md'
# Go tests — no pgpulse. metric keys
grep -rn '"pgpulse\.' internal/ cmd/ | grep '_test.go'
# Frontend — no pgpulse. or diskstat references
grep -rn 'pgpulse\.' web/src/
grep -rn 'diskstat' web/src/
# Go backend — no diskstat references
grep -rn 'diskstat' internal/ cmd/
# cluster.go must NOT have pg.cluster (stays as cluster.*)
grep '"pg\.cluster' internal/collector/cluster.go
```

ALL of the above must return ZERO matches.

### Task 2: Build verification
```bash
cd web && npm run build && npm run typecheck && npm run lint
cd .. && go build ./cmd/pgpulse-server ./cmd/pgpulse-agent
go test ./cmd/... ./internal/... -count=1
golangci-lint run ./cmd/... ./internal/...
```

### Task 3: Verify migration script
- Confirm migration number is correct (one higher than existing max)
- Confirm SQL syntax is valid
- Confirm all three tables are covered: `metrics`, `alert_rules`, `ml_baseline_snapshots`
- Confirm `cluster.*` keys are NOT touched

### Task 4: Spot-check key consistency
Pick 5 metric keys from different categories and trace them end-to-end:
1. `pg.connections.total` — collector → API → frontend
2. `pg.cache.hit_ratio` — collector → API → frontend → forecast
3. `os.disk.read_bytes_per_sec` — collector → API → frontend
4. `pg.replication.lag.replay_bytes` — collector → API → frontend → forecast
5. `cluster.patroni.member_count` — collector → API → frontend (must be UNCHANGED)

For each, verify the same key string appears in collector code, any API handler
references, and frontend component/hook code.

---

## Coordination Rules

- Collector Agent and API & Security Agent can start in parallel (no file overlap)
- Frontend Agent starts once Collector Agent has committed (so key names are finalized)
- QA Agent starts after ALL other agents have committed
- Merge order: Collector → API → Frontend → QA verification
- Build verification after EVERY merge
