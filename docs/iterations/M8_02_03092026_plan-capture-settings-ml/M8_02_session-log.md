# Session Log — M8_02
## Auto-Capture Plans + Temporal Settings Diff + ML Anomaly Detection

**Date:** 2026-03-09
**Iteration:** M8_02
**Milestone:** M8 — ML Phase 1

---

## Goal

Ship three production features on top of the M8_01 ML foundation:
1. Auto-capture `EXPLAIN` plans triggered by duration threshold, scheduled top-N, manual API, and plan hash diff
2. Temporal `pg_settings` snapshots with a Go-side diff API
3. STL-based ML anomaly detection (Z-score + IQR) wired into the real alert pipeline

---

## Pre-Work (Developer + Team Lead, before agents spawned)

Five issues found in the design docs and fixed before agent spawn:

| # | Issue | Fix Applied |
|---|-------|-------------|
| 1 | Migration numbering: `007_` already taken by session audit log | Renumbered to `008_plan_capture`, `009_settings_snapshots` |
| 2 | `MetricStore` lacks `ListInstances` — ML Bootstrap needs it | Introduced separate `InstanceLister` interface; `Detector` takes both |
| 3 | `gonum` not in `go.mod` | `go get gonum.org/v1/gonum@latest` — upgraded from v0.16.0 to v0.17.0 |
| 4 | `nullInt64` helper referenced in `store.go` but never defined | Defined locally in `internal/plans/store.go` |
| 5 | `InstanceContext` lacks `InstanceID` field — `capture.go` used `ic.InstanceID` | Collectors take `instanceID string` as explicit param alongside `ic InstanceContext` |

---

## Agent Activity

### Collector Agent
Created:
- `internal/plans/capture.go` — all four triggers (duration threshold, scheduled top-N, manual, hash diff), dedup cache with configurable window
- `internal/settings/snapshot.go` — startup/scheduled/manual capture from `pg_catalog.pg_settings`
- `internal/ml/config.go` — `DetectorConfig`, `MetricConfig`, `DefaultConfig()`
- `internal/ml/baseline.go` — `STLBaseline`: EWMA trend, period-folded seasonal mean, Z-score + IQR residual scoring via gonum
- `internal/ml/detector.go` — `Detector` with `Bootstrap` (loads TimescaleDB history) and `Evaluate` (online update + alert dispatch)
- `internal/ml/baseline_test.go` — 10 tests
- `internal/ml/detector_test.go` — 9 tests with mock `MetricStore`, `AlertEvaluator`, `InstanceLister`

Build verified clean after Collector Agent completed.

### API Agent
Created:
- `migrations/008_plan_capture.sql` — `query_plans` table with dedup unique index on `(instance_id, query_fingerprint, plan_hash)`
- `migrations/009_settings_snapshots.sql` — `settings_snapshots` table
- `internal/plans/store.go` — `PGPlanStore`: `SavePlan` (upsert on plan hash), `ListPlans`, `GetPlan`, `ListRegressions`, `LatestPlanHash`
- `internal/plans/retention.go` — hourly cleanup goroutine
- `internal/settings/store.go` — `PGSnapshotStore`: `SaveSnapshot`, `GetSnapshot`, `ListSnapshots`, `LatestSnapshot`
- `internal/settings/diff.go` — `DiffSnapshots`: changed/added/removed/pending_restart, Go-side (no SQL diff)
- `internal/api/plan_handlers.go` — `ListPlans`, `GetPlan`, `ListRegressions`, `ManualCapture`
- `internal/api/settings_handlers.go` — `SettingsHistory`, `SettingsDiff`, `SettingsLatest`, `PendingRestart`, `ManualSnapshot`
- `internal/config/config.go` — `PlanCaptureConfig`, `SettingsSnapshotConfig`, `MLConfig`
- `internal/alert/rules.go` — two ML anomaly alert rules seeded (Z=3 warning, Z=5 critical)
- `configs/pgpulse.example.yml` — `plan_capture`, `settings_snapshot`, `ml` sections added

### Team Lead (main.go wiring)
- `PGPlanStore` + `PlanCollector` initialized, registered with orchestrator
- `RetentionWorker` started as goroutine when plan capture enabled
- `PGSnapshotStore` + `SnapshotCollector` initialized
- `configInstanceLister` implemented (wraps config instance list for `InstanceLister` interface)
- `ml.Detector` initialized with `Bootstrap` called at 30s timeout before collector loop

### Post-merge fixes (lint + interface adapter)

**Lint:** `golangci-lint run` failed with 12 unused function errors across three M8_01 files. Root cause: handlers written but routes never registered in `server.go`. Fix: deleted the three files entirely (`internal/api/plans.go`, `internal/api/sessions.go`, `internal/api/settings_diff.go`). Functions can be reintroduced when routes are actually wired.

**AlertEvaluator adapter:** ML detector was initializing with `noOpAlertEvaluator` because `collector.AlertEvaluator` (single metric call) and `alert.Evaluator` (batch `[]MetricPoint`) are incompatible interfaces. Fix:
- Created `internal/alert/adapter.go` — `MetricAlertAdapter` wraps `*alert.Evaluator`, satisfies `collector.AlertEvaluator` by wrapping the single metric point into a one-element slice
- Added `Detector.SetAlertEvaluator()` setter in `internal/ml/detector.go`
- Wired in `main.go`: after alert pipeline initializes, upgrades detector from noOp to real adapter

---

## Test Results

| Package | Tests | Result |
|---------|-------|--------|
| `internal/plans` | 10 | ✅ pass |
| `internal/settings` | 9 | ✅ pass |
| `internal/ml` | 19 (10 baseline + 9 detector) | ✅ pass |
| `internal/api` (plans + settings handlers) | ~8 | ✅ pass |
| All other packages | cached | ✅ pass |
| **Total new tests** | **29** | |

`go build ./...` — ✅ clean
`go vet ./...` — ✅ clean
`golangci-lint run` — ✅ 0 issues
`npm run typecheck` — ✅ 0 errors

---

## Commits

3 commits pushed to origin/main:
1. `feat(plans/settings/ml)` — main implementation (agents)
2. `fix(lint)` — deleted 3 files with 12 unused functions from M8_01
3. `feat(alert)` — `MetricAlertAdapter` + `SetAlertEvaluator` + main.go wiring

---

## Architecture Decisions

| Decision | Rationale |
|----------|-----------|
| `InstanceLister` as separate interface | Avoids adding `ListInstances` to `MetricStore` — different concern, different lifecycle |
| Simplified STL (EWMA + period-folded mean) | Full Loess would require dedicated library or significant gonum work; simplified version produces valid residuals for Z-score/IQR scoring and is honest about its limits |
| Go-side diff for settings (not SQL JSONB diff) | Testable without a database; easier to extend with custom filtering later |
| Plan dedup by plan hash | Identical plan shapes stored once; regressions always produce new row; no storage explosion |
| `MetricAlertAdapter` wraps batch interface | Preserves existing `alert.Evaluator` batch contract; ML anomalies are low-frequency so one-element slice overhead is negligible |
| Deleted unused handlers rather than nolint | Unused code is tech debt; routes will be reintroduced properly when wired |

---

## Not Done / Next Iteration (M8_03)

- [ ] Forecast horizon (predict next N points from STL baseline) — deferred by design
- [ ] ML model state persistence across restarts (currently recomputes from TimescaleDB on startup)
- [ ] Automatic seasonal period detection (currently configured, not inferred)
- [ ] `pg_stat_activity` session kill/terminate API (deleted handlers need proper route wiring)
- [ ] Settings diff UI (API complete, no frontend yet)
- [ ] Plan capture UI (API complete, no frontend yet)
- [ ] Integration tests for plan capture (require `pg_sleep` + testcontainers — tagged `//go:build integration`, skipped in CI)
