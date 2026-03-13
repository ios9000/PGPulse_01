# MN_01 — Metric Naming Standardization: Requirements

**Date:** 2026-03-13
**Iteration:** MN_01 (Mordin: Old Blood — first task)
**Predecessor:** MW_01b (bugfixes) + Competitive Research Session
**Dependency:** Competitive Research Synthesis §3.3 naming rules

---

## Goal

Rename all ~157 metric keys across the entire codebase to conform to the naming
standard decided during the competitive research session. Every rename must be
applied atomically across all layers (collector → storage → API → frontend →
alerts → ML). A partial rename breaks dashboards.

## Naming Standard (from Competitive Research §3.3, adapted)

Four top-level prefixes:

| Prefix | Layer | Example |
|--------|-------|---------|
| `pg.` | PostgreSQL metrics | `pg.connections.active`, `pg.cache.hit_ratio` |
| `os.` | OS metrics | `os.cpu.user_pct`, `os.disk.read_bytes_per_sec` |
| `cluster.` | HA infrastructure (Patroni, etcd) | `cluster.patroni.member_count` (unchanged) |
| `pgpulse.` | PGPulse internal/meta | Reserved — no metrics use this prefix currently |

Within each prefix:
1. **Second level = category:** connections, database, transactions, replication, bgwriter, checkpoint, etc.
2. **Third level = specific metric:** descriptive, snake_case within level
3. **Units in name when ambiguous:** `_bytes`, `_percent`, `_per_sec`, `_seconds`, `_count`
4. **Labels/tags for dimensions:** `{instance, database, table, state}`

## Scope Decisions (Locked)

| ID | Decision | Choice |
|----|----------|--------|
| D200 | Cluster metrics prefix | `cluster.*` — keep as-is, no rename |
| D201 | OS prefix for both SQL and agent paths | `os.*` (drop `pgpulse.` from SQL path) |
| D202 | Historical data migration | SQL migration script renames existing data |

## What Must Change

### 1. Collector Layer (~120 keys — bulk rename via Base.point())

`Base.point()` currently prepends `pgpulse.` to all metric names.
Change to `pg.` — handles the bulk of renames in one code change.

### 2. OSSQLCollector — Prefix Fix

Goes through `Base.point()`, which stores OS metrics as `pgpulse.os.*` (soon `pg.os.*`
after Step 1). Must bypass `Base.point()` and emit `os.*` directly, matching the
agent path behavior per D201.

### 3. ClusterCollector — NO CHANGE (D200)

Already emits bare `cluster.*`. Keep as-is.

### 4. Disk Metric Hierarchy Cleanup (7 keys)

`os.diskstat.*` → `os.disk.*` to merge under a single disk namespace.
Unit rename: `read_kb` / `write_kb` → `read_bytes_per_sec` / `write_bytes_per_sec`.
Verify whether actual values need unit conversion or just name change.

### 5. Frontend (all metric key references)

Every hardcoded metric key string in `web/src/**/*.ts{x}` must be updated.
Key files: `constants.ts`, chart components, hooks, pages.

### 6. Alert Rules & Seeds

`internal/alert/seed.go` default rules. Migration script updates existing
`alert_rules.metric` column in the database.

### 7. ML Configuration

`internal/ml/` metric key references + sample config files.

### 8. API Layer

Hardcoded metric keys in `internal/api/*.go` handlers.

### 9. TimescaleDB Migration

SQL migration to UPDATE existing data in `metrics`, `alert_rules`, and
`ml_baseline_snapshots` tables.

## Out of Scope

- Memory metric unit rename (`_kb` → `_bytes`) — follow-up iteration
- Prometheus exporter implementation — separate milestone (NEW-3)
- Counter vs. gauge documentation — separate task
- New metric additions
- Cluster metric renames (D200: keep as-is)

## Build Verification

```bash
cd web && npm run build && npm run typecheck && npm run lint
cd .. && go build ./cmd/pgpulse-server
go test ./cmd/... ./internal/... -count=1
golangci-lint run ./cmd/... ./internal/...
```

## Success Criteria

1. All PG metrics use `pg.*` prefix (not `pgpulse.*`)
2. All OS metrics use `os.*` prefix consistently across both collection paths
3. Cluster metrics remain `cluster.*` (unchanged)
4. `os.diskstat.*` hierarchy eliminated — all under `os.disk.*`
5. Frontend renders all charts correctly with new keys
6. ML forecasting works with new keys
7. Alert rules reference new keys
8. Migration script handles existing TimescaleDB data
9. Full build passes (Go + TypeScript + lint + tests)
