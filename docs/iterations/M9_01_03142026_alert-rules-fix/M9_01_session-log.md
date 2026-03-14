# Session: 2026-03-14 — M9_01 Alert & Advisor Polish

## Goal

Fix metric key mismatches in alert rules, add alert tab bar UI navigation, expand sidebar alerts group, fix DSN port parsing for keyword/value format, and populate MetricKey/MetricValue in Diagnose recommendations.

## Agent Configuration

- **Planning:** Claude Code (Opus 4.6) — task decomposition, audit, coordination
- **Implementation:** Claude Code (Opus 4.6) — two parallel agents (Backend + Frontend)
- **Duration:** ~1 session
- **Scope:** 16 modified files, 1 new file, +374 / -75 lines

## What Was Done

### Task A: Alert Rules Metric Key Audit

Fixed 12 metric key mismatches in `internal/alert/rules.go` so alert rules reference the actual canonical keys emitted by collectors:

| Rule IDs | Old Key | New Key |
|----------|---------|---------|
| wraparound_warning/critical | `pg.databases.wraparound_pct` | `pg.server.wraparound_pct` |
| multixact_warning/critical | `pg.databases.multixact_pct` | `pg.server.multixact_pct` |
| commit_ratio_warning | `pg.transactions.commit_ratio` | `pg.transactions.commit_ratio_pct` |
| replication_slot_inactive | `pg.replication.slot_active` | `pg.replication.slot.active` |
| long_transaction_warning/critical | `pg.transactions.longest_seconds` | `pg.long_transactions.oldest_seconds` |
| table_bloat_warning/critical | `pg.tables.bloat_pct` | `pg.db.bloat.table_ratio` |
| pgss_dealloc_warning | `pg.statements.dealloc_count` (threshold 0) | `pg.extensions.pgss_fill_pct` (threshold 95) |
| replication_lag_warning/critical | `pg.replication.lag_bytes` | `pg.replication.lag.total_bytes` |

Added `TestBuiltinRulesMetricKeys` in `internal/alert/rules_test.go` to assert correct canonical keys for 10 key rules.

### Task B1: Alert Tab Bar

- Created `web/src/components/alerts/AlertsTabBar.tsx` — three tabs: Active | History | Rules
- Integrated into `AlertsDashboard.tsx` (reads `?view=history` query param) and `AlertRules.tsx`

### Task B2: Sidebar Expandable Alerts Group

- Modified `web/src/components/layout/Sidebar.tsx`:
  - Alerts is now an expandable group with Dashboard and Rules sub-items
  - Auto-expands when route matches `/alerts*`
  - Collapsed sidebar shows just the Bell icon

### Task C1: Port Display Parser Fix

- Updated `parseHostPort()` in `internal/api/instances_crud.go` and `extractHostPort()` in `internal/api/instances.go` to handle keyword/value DSN format (`host=x port=5433 dbname=z`)
- Added `parseKeyValueDSN()` helper
- Added 12 test cases in `internal/api/instances_test.go` covering URL and keyword/value formats

### Task C2: Diagnose Panel Metric Value

**Backend:**
- Added `MetricKey` and `MetricValue` fields to `RuleResult` in `internal/remediation/rule.go`
- Updated `Diagnose()` in `internal/remediation/engine.go` to propagate these fields
- Updated all 17 PG rules (`rules_pg.go`) and 8 OS rules (`rules_os.go`) to set MetricKey/MetricValue
- Added `TestDiagnose_PopulatesMetricKeyAndValue` in `engine_test.go`

**Frontend:**
- Updated `web/src/components/server/DiagnosePanel.tsx` to display formatted metric value
- Format based on key suffix: `_pct` -> percentage, `_bytes` -> bytes, `_seconds` -> duration

## Files Changed

| File | Change |
|------|--------|
| `internal/alert/rules.go` | Fixed 12 metric key mismatches |
| `internal/alert/rules_test.go` | Added TestBuiltinRulesMetricKeys |
| `internal/api/instances.go` | extractHostPort handles keyword/value DSN |
| `internal/api/instances_crud.go` | parseHostPort handles keyword/value DSN, added parseKeyValueDSN |
| `internal/api/instances_test.go` | Added TestParseHostPort, TestExtractHostPort |
| `internal/remediation/rule.go` | Added MetricKey, MetricValue to RuleResult |
| `internal/remediation/engine.go` | Diagnose() propagates MetricKey/MetricValue |
| `internal/remediation/engine_test.go` | Added TestDiagnose_PopulatesMetricKeyAndValue |
| `internal/remediation/rules_pg.go` | All 17 rules set MetricKey/MetricValue |
| `internal/remediation/rules_os.go` | All 8 rules set MetricKey/MetricValue |
| `web/src/components/alerts/AlertsTabBar.tsx` | **NEW** — Alert tab bar component |
| `web/src/components/layout/Sidebar.tsx` | Expandable alerts group |
| `web/src/components/server/DiagnosePanel.tsx` | Metric value display |
| `web/src/pages/AlertsDashboard.tsx` | Integrated AlertsTabBar |
| `web/src/pages/AlertRules.tsx` | Integrated AlertsTabBar |

## Build Status

- `go build ./cmd/... ./internal/...` — clean
- `go test ./cmd/... ./internal/... -count=1` — 14 packages pass
- `golangci-lint run` — 0 issues
- `npm run build` — clean
- `npm run typecheck` — clean
- `npm run lint` — 0 errors (1 pre-existing warning in useSystemMode.tsx)

## Decisions

| Decision | Rationale |
|----------|-----------|
| `pg.server.multixact_pct` prefix (not `pg.databases.`) | Aligns with `pg.server.wraparound_pct` pattern; no collector emits this yet but prefix is consistent |
| pgss rule changed from dealloc_count to fill_pct | `pg.statements.dealloc_count` doesn't exist as a metric; fill_pct (threshold 95%) is the actual metric |
| OS rules use `os.*` prefix in MetricKey (not `pg.os.*`) | Consistent with agent collector prefix; dual-prefix handled in rules_os.go already |
| Frontend port display — no changes needed | Frontend uses structured `host`/`port` fields from API, not DSN parsing |
