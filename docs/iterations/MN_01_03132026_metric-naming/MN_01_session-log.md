# MN_01 — Metric Naming Standardization: Session Log

**Date:** 2026-03-13
**Duration:** Single session
**Commit:** 2f96bed

---

## Goal

Rename all ~157 metric keys to conform to the naming standard from the MW_01b
competitive research session:

| Prefix | Scope | Status |
|--------|-------|--------|
| `pg.*` | PostgreSQL metrics (was `pgpulse.*`) | **Achieved** |
| `os.*` | OS metrics — both SQL and agent paths | **Achieved** |
| `os.disk.*` | Disk I/O hierarchy (was `os.diskstat.*`) | **Achieved** |
| `cluster.*` | HA infrastructure (Patroni, etcd) | **Unchanged per D200** |
| `pgpulse.*` | Reserved for internal/meta (none exist) | **N/A** |

**Goal achieved: YES.** All verification checks pass.

---

## Agent Activity Summary

### Team Lead (main context)
- Read all source files to map the full blast radius
- Executed 7-task plan sequentially with one background agent for test files
- Ran full QA audit (grep + build + test + lint) at the end

### Background Agent (test file updater)
- Updated 24 test files in parallel
- 180 `"pgpulse."` → `"pg."` replacements across 23 files
- 7 `"os.diskstat."` → `"os.disk."` replacements in `os_sql_test.go`
- Confirmed zero remaining old keys via grep

### No worktree isolation needed
All changes were atomic to one commit. No merge conflicts possible since
the rename touched string literals only — no structural code changes.

---

## Files Modified (43 total)

### Go Backend — Production Code (8 files)
| File | Change |
|------|--------|
| `internal/collector/base.go` | `metricPrefix = "pgpulse"` → `"pg"` (the one-line fix for ~120 keys) |
| `internal/collector/os_sql.go` | Disk keys: `os.diskstat.*` → `os.disk.*`, value × 1024 for bytes |
| `internal/collector/os.go` | Same disk rename in `snapshotToMetricPoints()` |
| `internal/alert/rules.go` | 20 builtin rule metric keys: `pgpulse.*` → `pg.*` |
| `internal/api/databases.go` | ~30 metric key references in query/switch statements |
| `internal/api/alerts.go` | Test notification event metric keys |
| `internal/orchestrator/db_runner.go` | Internal telemetry keys (`pgpulse.agent.db.*` → `pg.agent.db.*`) |
| `internal/storage/migrations/012_metric_naming_standardization.sql` | **NEW** — data migration |

### Go Backend — Test Code (24 files)
All `*_test.go` files across collector, alert, api, ml, orchestrator, and storage
packages. Purely string literal replacements in test assertions.

### Frontend (5 files)
| File | Change |
|------|--------|
| `web/src/pages/ServerDetail.tsx` | `pgpulse.*` → `pg.*` in metric keys and `.replace()` calls |
| `web/src/components/fleet/InstanceCard.tsx` | `pgpulse.*` → `pg.*` in metric lookups |
| `web/src/components/server/KeyMetricsRow.tsx` | `pgpulse.*` → `pg.*` in metric lookups |
| `web/src/components/server/HeaderCard.tsx` | `pgpulse.*` → `pg.*` in metric lookups |
| `web/src/components/server/OSMetricsSection.tsx` | `os.diskstat.*` → `os.disk.*`, chart labels updated |

### Design Docs (6 files, from prior commit f32257e)
MN_01 design, requirements, checklist, team-prompt, plus the MW_01b handoff.

---

## Key Decisions Made During Implementation

### D1: OSSQLCollector already has its own `point()` — no prefix fix needed
The OSSQLCollector (line 120 of os_sql.go) already had a standalone `point()` method
that emits metric names directly without any prefix. Changing `Base.point()` from
`"pgpulse."` to `"pg."` does NOT affect OS metrics from the SQL path — they were
already emitting `"os.*"` correctly. This was confirmed by reading the code before
making changes.

### D2: ClusterCollector also has its own `point()` — safely isolated
ClusterCollector uses a local `point()` closure (line 42 of cluster.go) that emits
`"cluster.*"` keys directly. The `Base.point()` prefix change does not affect it.
No guard or special handling was needed.

### D3: Disk value unit conversion — KB to bytes
`DiskStatsDelta` computes `ReadKB = sectorsReadDelta * 512 / 1024`, which is delta
kilobytes (not per-second, not bytes). When renaming from `read_kb` to
`read_bytes_per_sec`, the value was multiplied by 1024 to convert to bytes, as
instructed. Note: the value is still a delta-per-interval, not a true per-second
rate — this naming is aspirational and matches the competitive standard.

### D4: Frontend disk chart labels updated to match
Changed Y-axis label from `"KB/s"` to `"B/s"` and series names from `"Read KB/s"` /
`"Write KB/s"` to `"Read B/s"` / `"Write B/s"` to reflect the unit change.

### D5: Config ML keys unchanged
The `config.sample.yaml` ML section uses bare metric names like `"connections.active"`
without any prefix — these are base keys that get the prefix added by the collector.
No config changes were needed.

### D6: models.ts `diskstats` field left alone
`web/src/types/models.ts` has a TypeScript field `diskstats: OSDiskStatInfo[]` which
matches the Go JSON tag `json:"diskstats"` on the `OSSnapshot` struct. This is a
data structure field name, not a metric key — left unchanged.

---

## Test Results

```
go test ./cmd/... ./internal/... -count=1
  internal/agent          1.045s  PASS
  internal/alert          1.272s  PASS
  internal/alert/notifier 1.239s  PASS
  internal/api            0.430s  PASS
  internal/auth           0.947s  PASS
  internal/cluster/etcd   1.171s  PASS
  internal/cluster/patroni 1.123s PASS
  internal/collector      0.824s  PASS
  internal/config         0.837s  PASS
  internal/ml             0.875s  PASS
  internal/orchestrator   0.836s  PASS
  internal/plans          0.662s  PASS
  internal/settings       0.649s  PASS
  internal/storage        1.076s  PASS

golangci-lint run: 0 issues
npm run build: ✓ built in 11.07s
npm run typecheck: 0 errors
npm run lint: 0 errors (1 pre-existing warning)
```

---

## Grep Audit (Post-Change)

| Pattern | Scope | Matches |
|---------|-------|---------|
| `"pgpulse."` | Production Go | 0 |
| `"pgpulse."` | Test Go | 0 |
| `pgpulse.` | Frontend | 0 |
| `os.diskstat` | All Go | 0 |
| `diskstat` | Frontend (excl. `diskstats` field) | 0 |
| `"pg.cluster"` | cluster.go | 0 (correct — keys are `cluster.*`) |

---

## Issues Discovered

None. The implementation was straightforward because:
1. The `Base.point()` pattern centralizes ~120 PG metric keys behind one prefix constant
2. OSSQLCollector and ClusterCollector already used standalone `point()` methods, isolating them from the prefix change
3. All metric key references in frontend and tests are string literals, making find-and-replace reliable
