## [M12_01] — 2026-03-17 — Core Desktop (Wails v3)

### Added
- **Wails v3 desktop shell** — native Windows window via `go build -tags desktop`
- **`internal/desktop/` package** — `DesktopApp` (app.go), `SystemTray` (tray.go), icon embedding (icon.go)
- **`--mode` flag** — `--mode=desktop` activates Wails native window; `--mode=server` (default) runs headless HTTP server
- **System tray** — green/yellow/red severity icons, left-click show/hide toggle, right-click context menu (Show / Status / Quit)
- **Window close → hide** — close button minimizes to tray instead of quitting
- **Build-tag gating** — ALL Wails code behind `//go:build desktop`; standard build unchanged, zero Wails symbols
- **Chi router as AssetOptions.Handler** — zero frontend changes; React app works identically through Wails in-memory IPC
- **Placeholder icons** — generated via `cmd/icongen/main.go` (64x64 colored circles + 32x32 ICO)
- **Wails v3 alpha.74** dependency pinned in go.mod

### Changed
- Go version bumped from 1.24 to 1.25 (required by Wails v3 alpha.74)

### Notes
- 2-agent execution (Desktop/Backend + QA/Build), ~20 min total
- 16 files changed, +565 lines
- All checks pass: standard build, desktop build, symbol verification, tests, lint, frontend

---

## [M9_01] — 2026-03-14 — Alert & Advisor Polish

### Fixed
- **12 alert rule metric key mismatches** corrected to match canonical collector keys:
  - Wraparound: `pg.databases.wraparound_pct` → `pg.server.wraparound_pct`
  - Multixact: `pg.databases.multixact_pct` → `pg.server.multixact_pct`
  - Commit ratio: `pg.transactions.commit_ratio` → `pg.transactions.commit_ratio_pct`
  - Replication slot: `pg.replication.slot_active` → `pg.replication.slot.active`
  - Long transactions: `pg.transactions.longest_seconds` → `pg.long_transactions.oldest_seconds`
  - Table bloat: `pg.tables.bloat_pct` → `pg.db.bloat.table_ratio`
  - PGSS fill: `pg.statements.dealloc_count` → `pg.extensions.pgss_fill_pct` (threshold 0 → 95)
  - Replication lag: `pg.replication.lag_bytes` → `pg.replication.lag.total_bytes`
- **DSN port parsing**: `parseHostPort()` and `extractHostPort()` now handle keyword/value DSN format (`host=x port=5433 dbname=z`)

### Added
- **AlertsTabBar**: Three-tab navigation (Active | History | Rules) on AlertsDashboard and AlertRules pages
- **Sidebar expandable Alerts group**: Dashboard and Rules sub-items, auto-expands on `/alerts*`
- **Diagnose MetricKey/MetricValue**: `RuleResult` now carries MetricKey and MetricValue; `Diagnose()` propagates them; all 25 rules (17 PG + 8 OS) set them
- **DiagnosePanel formatting**: Metric values formatted by suffix (`_pct` → %, `_bytes` → bytes, `_seconds` → duration)
- **New tests**: `TestBuiltinRulesMetricKeys`, `TestParseHostPort`, `TestExtractHostPort`, `TestDiagnose_PopulatesMetricKeyAndValue`

### Notes
- 2-agent parallel execution (Backend + Frontend)
- 17 files changed, +374 lines
- All checks pass: go build, go test (14 packages), golangci-lint (0), npm build + typecheck + lint (0 errors)

---

## [REM_01c] — 2026-03-14 — Remediation Metric Key Fix (Bugfix)

### Fixed
- **13 remediation rules** had metric keys that didn't match actual collector output — all corrected:
  - Connection rules: `pg.connections.active` + `max_connections` → `pg.connections.utilization_pct` (already a percentage, no division needed)
  - Commit ratio: `pg.transactions.commit_ratio` → `pg.transactions.commit_ratio_pct`
  - Replication lag: `pg.replication.replay_lag_bytes` → `pg.replication.lag.replay_bytes`
  - Replication slot: `pg.replication.slot_inactive` → `pg.replication.slot.active` (inverted logic: 0 = inactive fires)
  - Long transactions: `pg.transactions.oldest_active_sec` → `pg.long_transactions.oldest_seconds`
  - Statements fill: `pg.statements.fill_pct` → `pg.extensions.pgss_fill_pct`
  - Bloat: `pg.db.bloat.ratio` → `pg.db.bloat.table_ratio`
- **Replication slot Diagnose mode**: returns nil (per-slot labeled metrics don't map to flat snapshot lookup)

### Added
- **Dual OS prefix support**: `getOS()`/`isOSMetric()` helpers check both `os.*` (agent) and `pg.os.*` (SQL collector) prefixes — all 8 OS rules updated
- **Wraparound metric**: `pg.server.wraparound_pct` emitted by `ServerInfoCollector` via `max(age(datfrozenxid))::float / 2147483647 * 100`
- **New tests**: `TestGetOS_BothPrefixes`, `TestOSRules_PGOSPrefix`, `TestWraparound_Fires`

### Notes
- Single-agent bugfix — no new packages, no new files, no migrations
- All 25 remediation rules retain test coverage
- All checks pass: go build, go test (17 packages), golangci-lint (0), npm build + typecheck + lint

---

## [REM_01b] — 2026-03-14 — Remediation Frontend + Backend Gaps

### Added
- **Advisor Page**: Fleet-wide recommendation browser with filters (priority, category, status, instance), expandable rows, pagination, and acknowledge workflow
  - `web/src/pages/Advisor.tsx` — full page with AdvisorFilters, AdvisorRow, PriorityBadge components
  - `web/src/components/advisor/` — PriorityBadge (color-coded badge), AdvisorFilters (4 dropdowns), AdvisorRow (expandable table row with acknowledge)
  - `web/src/hooks/useRecommendations.ts` — 5 hooks: useRecommendations, useInstanceRecommendations, useDiagnose, useAcknowledge, useRemediationRules
  - Sidebar nav entry with Lightbulb icon, route at `/advisor`
- **Diagnose Button**: On-demand instance diagnosis from ServerDetail page
  - `web/src/components/server/DiagnosePanel.tsx` — slide-down results panel with summary, recommendation list, close button
  - HeaderCard receives `instanceId` prop + Diagnose button (Lightbulb icon, loading spinner)
  - ServerDetail wires useDiagnose hook, passes results to DiagnosePanel
- **Alert Row Expand/Collapse**: AlertRow now expandable with chevron toggle
  - Expanded row shows "View Server" link and recommendations sub-section
  - Recommendations display PriorityBadge + title + description + optional doc URL link
- **Alert Response Enrichment**: Alert API responses include per-event recommendations
  - `alertEventResponse` wrapper struct in `internal/api/alerts.go`
  - `enrichAlertEvents()` calls `remediationStore.ListByAlertEvent()` for each event
  - Both `handleGetActiveAlerts` and `handleGetAlertHistory` return enriched responses
- **Email Template Recommendations**: Alert notification emails include recommendation section
  - HTML: colored left border (info=#3B82F6, suggestion=#EAB308, action_required=#EF4444), bold priority, title, description, optional doc link
  - Plain text: bracketed priority labels with description
  - Section omitted entirely when no recommendations exist
- **Handler Tests**: `internal/api/remediation_test.go` — 14 test cases across 5 endpoint test functions
- **Store Integration Tests**: `internal/remediation/pgstore_test.go` — 5 integration tests behind `//go:build integration` tag

### Changed
- `internal/alert/alert.go` — added `ID int64` and `Recommendations []RemediationResult` (transient) fields to `AlertEvent`
- `internal/alert/pgstore.go` — prepended `id` to `eventColumns`, added to `scanEvent()`, `Record()` uses `RETURNING id`
- `internal/alert/dispatcher.go` — `runRemediation()` moved before notification loop, returns `[]RemediationResult`, populates `event.Recommendations`
- `internal/alert/template.go` — `templateData` includes `Recommendations`, render functions accept recommendations parameter
- `internal/alert/notifier/email.go` — passes `event.Recommendations` to render functions
- `web/src/pages/AlertsDashboard.tsx` — table header updated for new expand column
- `web/src/types/models.ts` — added `id` and `recommendations` to AlertEvent, added Recommendation/DiagnoseResponse/RemediationRule types

### Notes
- 3-agent team (API Agent + Frontend Agent + QA Agent), parallel execution
- 27 files changed, +2708 lines
- All checks pass: go build, go test (17 packages), golangci-lint (0), npm build + typecheck + lint (0 errors)
- No new API endpoints — existing 5 from REM_01a enriched with additional data

---

## [REM_01a] — 2026-03-13 — Rule-Based Remediation Engine (Backend)

### Added
- **Remediation Engine**: 25 compiled-in rules (17 PG + 8 OS) evaluate metrics and generate actionable recommendations
  - `internal/remediation/engine.go` — `Engine` with `EvaluateMetric()` (alert-triggered, single metric) and `Diagnose()` (full snapshot scan)
  - `internal/remediation/rule.go` — `Rule`, `Priority` (critical/warning/suggestion/info), `Category` (connections/cache/replication/locks/transactions/bloat/vacuum/checkpoints/statements/temp_files/deadlocks/wal/cpu/memory/swap/disk/io/load/network/oom), `EvalContext`, `MetricSnapshot`
  - `internal/remediation/rules_pg.go` — 17 PostgreSQL rules: conn_high (80-99%), conn_exhausted (>=99%), cache_low (<95%), repl_lag_high, repl_lag_critical, lock_contention, long_tx, bloat_high, vacuum_behind, checkpoint_freq, statements_slow, temp_files_high, deadlocks_detected, wal_growth, idle_in_tx, disabled_autovacuum, connection_pool_sizing
  - `internal/remediation/rules_os.go` — 8 OS rules: cpu_high, memory_pressure, swap_active, disk_full, disk_io_saturated, load_high, net_errors, oom_risk
- **Recommendation Persistence**: `remediation_recommendations` table with PGStore
  - `internal/remediation/pgstore.go` — Write, ListByInstance, ListAll, ListByAlertEvent, Acknowledge, CleanOld
  - `internal/remediation/nullstore.go` — NullStore for live mode (matches NullAlertHistoryStore pattern)
  - `internal/storage/migrations/013_remediation.sql` — table with 4 indexes
- **Alert Pipeline Integration**: Remediation fires automatically when alerts trigger
  - `internal/alert/remediation.go` — `RemediationProvider` interface + `RemediationResult` struct (no import of internal/remediation)
  - `internal/remediation/adapter.go` — `AlertAdapter` bridges Engine to `alert.RemediationProvider`
  - `internal/remediation/metricsource.go` — `StoreMetricSource` queries MetricStore for last 2 minutes to build snapshot
  - `internal/alert/dispatcher.go` — `SetRemediationProvider()` setter, `runRemediation()` called after cooldown
- **5 new API endpoints** (57 total):
  - `GET /api/v1/instances/{id}/recommendations` — list recommendations for instance
  - `POST /api/v1/instances/{id}/diagnose` — on-demand full snapshot scan
  - `GET /api/v1/recommendations` — list all recommendations across instances
  - `GET /api/v1/recommendations/rules` — introspect all 25 rules
  - `PUT /api/v1/recommendations/{id}/acknowledge` — acknowledge a recommendation
- **34 new tests**: 7 engine tests + 27 table-driven rule tests (all 25 rules with positive/negative/boundary/missing-key cases, unique ID check, NullStore compliance)

### Changed
- `internal/api/server.go` — added `SetRemediation()` setter, 5 new routes in both auth-enabled and auth-disabled sections
- `cmd/pgpulse-server/main.go` — wired Engine, PGStore, StoreMetricSource, AlertAdapter; connected to dispatcher and API server

### Notes
- Agent team crashed with OOM — recovered in single-agent session
- Import cycle prevention: alert defines interface, remediation implements it, main.go wires them
- Dual-mode: alert-triggered (real-time, single metric) + Diagnose (advisory, full snapshot)
- All checks pass: go build, go vet, go test, golangci-lint (0), npm build + typecheck

---

## [M8_10] — 2026-03-10 — Hotfix: Explain + Scan Errors

### Fixed
- **Explain handler recreated** (`internal/api/explain.go`) — handler was deleted in M8_02 cleanup, never recreated; now properly wired with SubstituteDatabase, one-shot pgx.Conn, 30s timeout
- **Breadcrumb "Servers" link** — changed from `/servers` (404) to `/fleet`
- **Replication client_addr inet scan** — added `::text` cast for pgx binary protocol compatibility
- **Progress command_desc column** — fixed column name for PG 16 compatibility
- **Lock tree datname NULL scan** — added `COALESCE(d.datname, '')` for non-database-specific locks
- **ConnForDB method** — added to orchestrator for explain handler database-specific connections

### Changed
- `internal/api/server.go` — explain route registered in both auth-enabled and auth-disabled paths
- `internal/orchestrator/orchestrator.go` — added `ConnForDB()` method (connects to specific database on instance)

---

## [M8_09] — 2026-03-09 — Hotfix: Production Crash + PG16 Compat

### Fixed
- **CRITICAL: TDZ crash in production bundle** — circular import `useForecastChart -> ForecastBand -> useForecast` broke minified build; moved `buildForecastSeries` from `components/ForecastBand.ts` to `lib/forecastUtils.ts`
- **CSP blocks Google Fonts** — added `fonts.googleapis.com` and `fonts.gstatic.com` to CSP header in middleware
- **Bloat CTE wrong column names** — used `pg_stats` view (`null_frac`, `avg_width`) instead of `pg_statistic` columns
- **WAL receiver `received_lsn` not found** — changed to `flushed_lsn` in `replication_status.go`
- **Sequences `pct_used` NULL scan** — added `COALESCE` and `IS NOT NULL` guard
- **`server.port` config ignored** — wired `cfg.Server.Port` to listen address when `server.listen` not set
- **Bloat query GROUP BY** — added missing `bitlength` column to GROUP BY clause
- **Bloat query subquery** — passed `bs` through `sml` subquery correctly
- **Databases handler metric prefix** — uses `pgpulse.db.` prefix for DB-level metrics

### Changed
- `internal/api/middleware.go` — updated CSP header
- `internal/collector/database.go` — bloat CTE fixes, sequences NULL fix
- `internal/collector/replication_status.go` — WAL receiver column fix
- `internal/config/config.go` + `load.go` — server.port config wiring
- `web/src/hooks/useForecastChart.ts` — imports from `lib/forecastUtils` instead of `components/ForecastBand`
- `internal/api/databases.go` — metric prefix alignment

---

## [M8_08] — 2026-03-09 — Logical Replication Monitoring

### Added
- **Logical Replication Sub-Collector**: PGAM Q41 ported — 17th DB sub-collector
  - `internal/collector/database.go` — `collectLogicalReplication` queries `pg_subscription_rel JOIN pg_subscription WHERE srsubstate <> 'r'` per database
  - Produces metric: `logical_replication_pending_sync_tables` (count of non-ready tables per DB)
  - Graceful error handling when `pg_subscription` doesn't exist
- **Logical Replication API**: `GET /api/v1/instances/{id}/logical-replication`
  - `internal/api/logical_replication.go` — handler + response structs + per-DB connection logic using `SubstituteDatabase()`
  - Returns: subscriptions array with pending tables, sync states, subscription stats
  - PG 15+ version gate: includes `apply_error_count`, `sync_error_count` when available
  - Route registered in viewer permission group (both auth-enabled and auth-disabled)
- **Logical Replication Frontend Section**
  - `web/src/hooks/useLogicalReplication.ts` — React Query hook, 30s refetch
  - `web/src/components/server/LogicalReplicationSection.tsx` — 4 UI states: no subscriptions (info card), all synced (green checkmark), pending sync (expandable subscription cards with table list and colour-coded state badges), error counts (red badge for PG 15+)
  - Placed after physical ReplicationSection in ServerDetail
- **Alert Rule**: `logical_repl_pending_sync` — builtin, disabled by default, warns when pending tables > 0
- **4 new TypeScript interfaces**: LogicalReplicationResponse, SubscriptionStatus, PendingTable, SubscriptionStats

### Changed
- `internal/alert/rules_test.go` — expected builtin rule count 21 → 22

### Notes
- 3-specialist team (Collector + Frontend + QA), ~10 min execution
- Query porting progress: ~70/76 PGAM queries ported (Q41 added)
- All checks pass: go build, go vet, go test, golangci-lint (0), tsc (0), eslint (0), vite build

---

## [M8_07] — 2026-03-09 — Deferred UI + Small Fixes

### Added
- **Plan Capture History UI**: Browse auto-captured query plans and regressions
  - `web/src/hooks/usePlanHistory.ts` — hooks for plan list, detail, regressions, manual capture
  - `web/src/components/PlanHistory.tsx` — "All Plans" / "Regressions" tabs, expandable rows with PlanNode tree reuse, trigger type badges (duration=blue, scheduled=gray, manual=green, hash_diff=amber), "Capture Now" button (permission-gated)
  - Added as "Plan History" tab in ServerDetail
- **Temporal Settings Timeline UI**: Compare pg_settings at time A vs time B
  - `web/src/hooks/useSettingsTimeline.ts` — hooks for snapshots, time-based diff, pending restart, manual snapshot
  - `web/src/components/SettingsTimeline.tsx` — snapshot timeline list, dual-dropdown selectors for compare, accordion diff view with colour-coded changes (amber=changed, green=added, red=removed), "Take Snapshot" button (permission-gated), "Pending Restart" quick-view
  - Added as "Settings Timeline" tab in ServerDetail (distinct from "Settings Diff" which shows current vs defaults)

### Changed
- `internal/api/activity.go` — Added `ApplicationName` field to `LongTransaction` struct, `COALESCE(application_name, '')` in SQL SELECT, updated `Scan()` call
- `internal/plans/capture.go` — Added JSON tags for proper API serialization (missing from M8_02)
- `web/src/pages/Administration.tsx` — Moved `useState` above early return to fix conditional hook violation (**0 lint errors achieved** — first time in project history)
- `web/src/components/server/LongTransactionsTable.tsx` — Passes `application_name` to `SessionActions` (enables pgpulse_* self-protection guard)
- `web/src/types/models.ts` — Added `application_name` to `LongTransaction` interface

### Notes
- 2-specialist team (Frontend + QA), ~9 min execution
- Route verification: all M8_02 handler routes were already registered in server.go
- All checks pass: 0 lint errors, 0 typecheck errors, go build + go test clean

---

## [M8_06] — 2026-03-09 — UI Catch-Up + Forecast Extension

### Added
- **Session Kill UI**: Cancel/Terminate buttons in activity table with confirmation modals
  - `ConfirmModal.tsx` — generic reusable modal (warning/danger variants, Escape key, backdrop click, loading spinner)
  - `SessionActions.tsx` — role-gated buttons (hidden for viewer, hidden for pgpulse_* connections), toast notifications for all response codes (200/403/404/500)
  - Integrated into `LongTransactionsTable.tsx` as actions column with refresh callback
- **Settings Diff UI**: Per-instance settings diff with accordion layout
  - `SettingsDiff.tsx` — grouped by pg_settings category, pending_restart amber badges, client-side CSV export with proper quoting
  - Added as "Settings Diff" tab in ServerDetail (lazy-loaded)
- **Query Plan Viewer UI**: Interactive EXPLAIN tree with cost highlighting
  - `PlanNode.tsx` — recursive tree rendering with highlight rules (amber >100ms actual time, red border >10x row estimate error)
  - `InlineQueryPlanViewer.tsx` — fetch plan, loading/error states, "Show Raw JSON" toggle
  - `StatementRow.tsx` + `StatementsSection.tsx` — expandable rows in top queries table
- **Forecast Overlay Extension**: Forecast bands on all metric charts
  - `useForecastChart.ts` — reusable helper hook (eliminates copy-paste across charts)
  - Applied to: `connections_active`, `cache_hit_ratio`, `transactions_commit_ratio_pct`, `replication_lag_replay_bytes`
- **Toast Notification System**: Reusable toast infrastructure
  - `Toast.tsx` — success/error/warning toast component
  - `toastStore.ts` — centralized toast state management
  - `AppShell.tsx` — ToastContainer added to root layout

### Changed
- `ServerDetail.tsx` — tab bar (Overview | Settings Diff), forecast overlay on 4 charts, expandable query rows

### Notes
- Frontend-only iteration — zero backend changes, zero new API endpoints
- 2-specialist team (Frontend Agent + QA Agent) — 18 min execution time
- All checks pass: tsc, eslint (pre-existing Administration.tsx error only), vite build, go build, go test

---

## [M8_05] — 2026-03-09 — Forecast Alerts + Forecast Chart

### Added
- **Forecast Alert Wiring**: ML forecasts trigger threshold alerts with sustained-crossing logic
  - `internal/mlerrors/errors.go` — shared sentinel errors (`ErrNotBootstrapped`, `ErrNoBaseline`), breaks `ml` ↔ `alert` circular import
  - `internal/alert/forecast.go` — `ForecastProvider` interface + `ForecastPoint` mirror struct (4 fields, intentionally thin)
  - `internal/ml/detector_alert.go` — `ForecastForAlert` adapter; `*ml.Detector` satisfies `alert.ForecastProvider`
  - `internal/alert/evaluator.go` — `SetForecastProvider(fp, minConsecutive)` setter + `runForecastAlerts()` called from `Evaluate()`
  - `internal/alert/alert.go` — `Rule.ConsecutivePointsRequired int` (0 = use global default of 3)
  - `migrations/011_forecast_alert_consecutive.sql` — column added with DEFAULT 0
  - `internal/config/config.go` + `load.go` — `ForecastConfig.AlertMinConsecutive int`, default 3
  - `cmd/pgpulse-server/main.go` — wiring: `evaluator.SetForecastProvider(mlDetector, cfg.ML.Forecast.AlertMinConsecutive)`
- **Forecast Chart Overlay**: ECharts confidence band + centre line on time-series charts
  - `web/src/hooks/useForecast.ts` — polls forecast API every 5 minutes, returns `ForecastResult | null`
  - `web/src/components/ForecastBand.ts` — `buildForecastSeries(points)` (custom polygon + dashed line), `getNowMarkLine(nowMs)`
  - `web/src/components/charts/TimeSeriesChart.tsx` — new props: `extraSeries`, `xAxisMax`, `nowMarkLine`
  - Wired to `connections_active` chart in ServerDetail

### Notes
- Sustained crossing is the only mode — N consecutive forecast points must cross threshold before alert fires
- `ConsecutivePointsRequired = 0` means "use global default (3)", not "first crossing"
- 4-specialist team (Collector, API & Security, Frontend, QA & Review)
- 13 new tests (9 forecast evaluator + 4 detector alert), all pass
- ECharts custom polygon for confidence band — dark-mode safe, no stack-trick delta pre-computation
- "Now" markLine placed on historical series (not forecast) for correct X positioning
- `internal/ml/errors.go` re-exports sentinels from `mlerrors` for backward compatibility

---

## [M8_04] — 2026-03-09 — Forecast Horizon

### Added
- **STL-based N-step-ahead forecasting** with confidence bounds
  - `internal/ml/forecast.go` — `ForecastPoint`, `ForecastResult`, `residualStddev()` helper
  - `internal/ml/errors.go` — `ErrNotBootstrapped`, `ErrNoBaseline` sentinel errors
  - `STLBaseline.Forecast(n, z, interval, now)` method: linear trend extrapolation (slope from last 2 EWMA values) + seasonal repeat + ±z·σ confidence bounds; returns nil when not warm
  - `trendHistory [2]float64` + `seasonIdx int` added to `STLBaseline`
  - `bootstrapped` flag on `Detector` to gate `Forecast()` calls before `Bootstrap()` completes
- **Forecast REST API**: `GET /api/v1/instances/{id}/metrics/{metric}/forecast?horizon=N`
  - Horizon cap enforced; `ErrNoBaseline` → 404, `ErrNotBootstrapped` → 503
  - Registered in viewer permission group (read-only)
  - `mlDetector` + `mlConfig` fields added to API server, `SetMLDetector()` setter
- **Forecast Configuration**: `ForecastConfig` struct in config package
  - `ForecastZ`, `ForecastHorizon` fields on `DetectorConfig`
  - `MLMetricConfig.ForecastHorizon` per-metric override
  - `ml.forecast` section added to `pgpulse.example.yml`
- **Forecast Alert Rule Type**: `RuleTypeForecastThreshold` constant, `Type` and `UseLowerBound` fields on `Rule` struct (evaluation deferred to M8_05)

### Notes
- Forecast is pure in-memory arithmetic — no DB access, no new table
- `runForecastAlerts()` not implemented this iteration (deferred to M8_05)
- ~7 minute agent execution time (ML Agent + API Agent + QA Agent)

---

## [M8_03] — 2026-03-09 — Instance Lister Fix + Session Kill API + ML Persistence

### Added
- **DB-backed Instance Lister**: `internal/ml/lister.go` — `DBInstanceLister` querying `instances WHERE enabled = true`
  - Replaces `configInstanceLister` which ignored instances added via API after startup
- **ML Baseline Persistence**: Fitted state survives restarts
  - `internal/ml/persistence.go` — `PersistenceStore` interface + `DBPersistenceStore` (JSONB upsert on `(instance_id, metric_key)`)
  - `BaselineSnapshot` struct, `Snapshot()` (exports live ring residuals in chronological order), `LoadFromSnapshot()`
  - Two-phase Bootstrap: snapshot load → TimescaleDB replay fallback
  - `Evaluate` persists all baselines after each cycle
  - `migrations/010_ml_baseline_snapshots.sql` — `ml_baseline_snapshots` table with unique on `(instance_id, metric_key)`
  - `MLPersistenceConfig` added to config, 5th `persist PersistenceStore` param on `NewDetector` (nil-safe)
- **Session Kill API** (reintroduced from M8_01 — routes now properly wired):
  - `internal/api/session_actions.go` — `handleSessionCancel` + `handleSessionTerminate` with own-PID guard, superuser guard, audit log via slog
  - Routes registered in `PermInstanceManagement` group (both auth-enabled and auth-disabled paths)

### Changed
- `configInstanceLister` removed from `main.go`; replaced by `ml.NewDBInstanceLister(storagePool)`
- `ml.NewDetector` expanded to 5-arg signature with persist store
- `Snapshot()` exports only live residuals (ring buffer has pre-allocated stale slots — exporting full slice would corrupt residual distribution)

### Housekeeping
- Removed accidentally committed agent worktree (`.claude/worktrees/agent-a87dfd96`)
- Added `.claude/worktrees/` to `.gitignore` to prevent recurrence

---

## [M8_02] — 2026-03-09 — Auto-Capture Plans + Temporal Settings Diff + ML Anomaly Detection

### Added
- **Auto-Capture Query Plans**: Four trigger modes with dedup
  - `internal/plans/capture.go` — duration threshold, scheduled top-N, manual API, plan hash diff triggers; dedup cache with configurable window
  - `internal/plans/store.go` — `PGPlanStore`: `SavePlan` (upsert on plan hash), `ListPlans`, `GetPlan`, `ListRegressions`, `LatestPlanHash`; `nullInt64` helper for nullable columns
  - `internal/plans/retention.go` — hourly cleanup goroutine
  - `migrations/008_plan_capture.sql` — `query_plans` table with dedup unique index on `(instance_id, query_fingerprint, plan_hash)`
  - Plan API: `ListPlans`, `GetPlan`, `ListRegressions`, `ManualCapture` handlers
  - `PlanCaptureConfig` in config package
- **Temporal Settings Snapshots**: Scheduled pg_settings capture with Go-side diff
  - `internal/settings/snapshot.go` — startup/scheduled/manual capture from `pg_catalog.pg_settings`
  - `internal/settings/store.go` — `PGSnapshotStore`: `SaveSnapshot`, `GetSnapshot`, `ListSnapshots`, `LatestSnapshot`
  - `internal/settings/diff.go` — `DiffSnapshots`: changed/added/removed/pending_restart (Go-side, no SQL diff)
  - `migrations/009_settings_snapshots.sql` — `settings_snapshots` table
  - Settings API: `SettingsHistory`, `SettingsDiff`, `SettingsLatest`, `PendingRestart`, `ManualSnapshot` handlers
  - `SettingsSnapshotConfig` in config package
- **STL-based ML Anomaly Detection**: Baseline fitting + Z-score/IQR scoring
  - `internal/ml/config.go` — `DetectorConfig`, `MetricConfig`, `DefaultConfig()`
  - `internal/ml/baseline.go` — `STLBaseline`: EWMA trend, period-folded seasonal mean, Z-score + IQR residual scoring via gonum (simplified STL — honest about being EWMA + folded mean, not full Loess)
  - `internal/ml/detector.go` — `Detector` with `Bootstrap` (loads TimescaleDB history) and `Evaluate` (online update + alert dispatch)
  - `internal/ml/baseline_test.go` — 10 tests
  - `internal/ml/detector_test.go` — 9 tests with mock `MetricStore`, `AlertEvaluator`, `InstanceLister`
  - Two ML anomaly alert rules seeded in `internal/alert/rules.go` (Z=3 warning, Z=5 critical)
- **`InstanceLister` interface**: Separate from `MetricStore` — ML Bootstrap needs instance list but shouldn't expand MetricStore's contract
- **`MetricAlertAdapter`**: `internal/alert/adapter.go` — wraps `*alert.Evaluator` (batch `[]MetricPoint`) to satisfy `collector.AlertEvaluator` (single metric call); wired in `main.go` via `Detector.SetAlertEvaluator()` setter
- **29 new tests** (10 baseline + 9 detector + ~10 plans/settings)
- **gonum v0.17.0** added to go.mod

### Changed
- `cmd/pgpulse-server/main.go` — full wiring: `PGPlanStore` + `PlanCollector` + `RetentionWorker`, `PGSnapshotStore` + `SnapshotCollector`, `ml.Detector` with 30s Bootstrap timeout, `MetricAlertAdapter` upgrade from noOp
- `configs/pgpulse.example.yml` — `plan_capture`, `settings_snapshot`, `ml` sections added
- `InstanceContext` confirmed to lack `InstanceID` — collectors take `instanceID string` as explicit param alongside `ic InstanceContext`

### Removed
- `internal/api/plans.go`, `internal/api/sessions.go`, `internal/api/settings_diff.go` — deleted (12 unused functions from M8_01; handlers written but routes never registered in `server.go`). Functions reintroduced properly in M8_03.

### Notes
- 5 design-doc issues caught and fixed before agent spawn (migration numbering, missing interfaces, gonum version, nullInt64 helper, InstanceContext field)
- Go-side diff for settings (not SQL JSONB diff) — testable without a database, extensible with custom filtering
- Plan dedup by plan hash — identical plan shapes stored once; regressions always produce new row

---

## [M8_01] — 2026-03-09 — P1 Features: Session Kill, Query Plans, Settings Diff

### Added
- **Session Kill**: Cancel or terminate PostgreSQL backend sessions from the UI
  - `POST /api/v1/instances/{id}/sessions/{pid}/cancel` — pg_cancel_backend (dba/super_admin only)
  - `POST /api/v1/instances/{id}/sessions/{pid}/terminate` — pg_terminate_backend (dba/super_admin only)
  - `session_audit_log` table (migration 007) — every operation logged with operator, PID, result
  - SessionKillButtons component with confirmation modals (cancel = neutral, terminate = destructive red)
- **On-Demand Query Plans**: Run EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) against any database
  - `POST /api/v1/instances/{id}/explain` — one-shot pgx.Conn, 30s statement_timeout, application_name=pgpulse_explain
  - `SubstituteDatabase()` helper for DSN database substitution (key=value and postgres:// formats)
  - QueryPlanViewer page: database selector, SQL textarea, ANALYZE/BUFFERS toggles, recursive plan tree with cost/row discrepancy highlighting (>10x yellow, >100x red), raw JSON toggle
- **Cross-Instance Settings Diff**: Compare pg_settings between any two monitored instances
  - `GET /api/v1/settings/compare?instance_a=X&instance_b=Y` — all authenticated users (viewer OK)
  - Concurrent fetch via errgroup (10s timeout per instance)
  - Noise filter: excludes server_version, data_directory, lc_* etc. by default (?show_all=true to override)
  - SettingsDiff page: dual instance selectors, accordion groups by category, CSV export
- **4 new API endpoints** (37 total)
- **6 new TypeScript interfaces**: SessionKillResult, ExplainRequest, ExplainResponse, PlanNode, SettingEntry, SettingsDiffResponse
- **Settings Diff nav item** in sidebar

### Changed
- `server.go`: 4 new routes registered in both auth-enabled and auth-disabled branches
- `ServerDetail.tsx`: Added "Explain Query" link to instance pages
- `App.tsx`: 2 new routes — /servers/:serverId/explain and /settings/diff
- `Sidebar.tsx`: Added Settings Diff with GitCompareArrows icon

### Notes
- All three features are stateless (no new collection loops or background workers)
- EXPLAIN query body intentionally NOT parameterized (cannot use $1 — auth gate is protection, documented in code)
- Migration is 007 (not 006 as design doc specified — 006 already taken by instances table)
- Backend: 3 new files, 1 modified — go build, go vet, go test, golangci-lint all pass (0 issues)
- Frontend: 3 new files, 4 modified — tsc 0 errors, vite build success
- Pre-existing lint error in Administration.tsx unrelated to M8_01

## [M7_01] — 2026-03-08 — Per-Database Analysis

### Added
- **DBCollector + Queryer interfaces** appended to collector.go (parallel to Collector — not merged)
- **DBRunner** (internal/orchestrator/db_runner.go): dynamic pool map per database, TTL eviction at 3 missed cycles, semaphore fan-out (MaxConcurrentDBs=5), 5 internal telemetry MetricPoints per cycle
- **16 DB sub-collectors** (internal/collector/database.go): bloat CTE, vacuum need, index usage, unused indexes, schema sizes, TOAST sizes, partition hierarchy, large objects, sequences, functions, catalog sizes, autovacuum options, table sizes, cache hit per table, unlogged objects
- **Discovery via pg_database** with include_databases / exclude_databases glob filters
- **New API endpoints**: GET /instances/:id/databases, GET /instances/:id/databases/:dbname/metrics
- **DatabaseDetail.tsx** page: Tables, Vacuum Health, Indexes, Schema Sizes (ECharts bar), Large Objects, Unlogged, Sequences, Functions
- **IncludeDatabases, ExcludeDatabases, MaxConcurrentDBs** fields in InstanceConfig
- ~69/76 PGAM queries ported

## [M6_01] — 2026-03-05 — Agent Mode + Cluster Providers

### Added
- **pgpulse-agent binary** (cmd/pgpulse-agent/): Linux-only OS metrics via procfs/sysfs
- **internal/agent/**: CPU, memory, disk, diskstats, load, uptime, os-release collectors with //go:build linux
- **internal/cluster/patroni/**: Patroni REST API + patronictl provider
- **internal/cluster/etcd/**: ETCD v3 status + health provider
- **New API endpoints**: GET /instances/:id/os, GET /instances/:id/cluster
- **Frontend sections**: OSSystemSection, DiskSection, IOStatsSection, ClusterSection

## [M5_07] — 2026-03-04 — User Management Enhancement

### Added
- **DELETE /api/v1/auth/users/{id}** — Delete user (user_management permission)
- **PUT /api/v1/auth/users/{id}/password** — Admin reset password (user_management permission)

## [M5_06] — 2026-03-04 — Stabilization + Instance Management

### Added
- **Connection pool refactor**: Replaced single `*pgx.Conn` per instance with `*pgxpool.Pool` (min 1, max configurable via `max_conns`) — eliminates connection contention between collectors
- **NoOp evaluator pattern**: `NoOpAlertEvaluator` and `NoOpAlertDispatcher` in orchestrator package — evaluator/dispatcher are never nil, removing nil-guard crashes when alerting is disabled
- **Instance name field**: Added `Name` and `MaxConns` to `InstanceConfig` with koanf tags and defaults
- **Instance store**: `PGInstanceStore` with full CRUD + `Seed()` (INSERT ON CONFLICT DO NOTHING) — DB is source of truth for instances
- **Migration 006_instances.sql**: `instances` table (id, name, dsn, host, port, enabled, source, max_conns, timestamps)
- **YAML seeding**: Startup seeds instances from config YAML with `source='yaml'`, DB overrides after first run
- **Orchestrator hot-reload**: Polls `InstanceStore` every 60s, starts/stops/restarts runners on instance changes without server restart
- **5 new API endpoints** (29 total):
  - `POST /api/v1/instances` — Create instance (requires `instance_management` permission)
  - `PUT /api/v1/instances/{id}` — Update instance
  - `DELETE /api/v1/instances/{id}` — Delete instance
  - `POST /api/v1/instances/bulk` — CSV bulk import (partial success, per-row results)
  - `POST /api/v1/instances/{id}/test` — Test connection (5s timeout, SELECT version(), reports latency)
- **DSN masking**: All API responses mask DSN passwords (`postgres://user:***@host:port/db`)
- **Administration page**: Tabbed layout (Instances + Users) replacing placeholder, permission-gated per tab
- **InstancesTab component**: Table with name, host:port, source badge (yaml=blue, manual=green), enabled toggle, edit/delete actions
- **InstanceForm modal**: Create/edit form with name, DSN (monospace), max connections, enabled toggle, test connection button (edit mode)
- **BulkImportModal**: CSV textarea + file upload + preview table + per-row import results
- **DeleteInstanceModal**: Confirmation dialog with yaml-source reappearance warning
- **useInstanceManagement hooks**: 6 hooks — useManagedInstances, useCreateInstance, useUpdateInstance, useDeleteInstance, useTestConnection, useBulkImport
- **ManagedInstance types**: TypeScript interfaces for instance CRUD request/response shapes

### Changed
- Orchestrator runners changed from slice to `map[string]*instanceRunner` for efficient lookup during hot-reload
- `startServer()` accepts `orchestrator.AlertEvaluator` interface instead of `*alert.Evaluator` concrete type
- `intervalGroup` acquires connection from pool per collect cycle (`pool.Acquire` + `defer conn.Release`)
- Instance list/get endpoints read from `InstanceStore` DB with config fallback, response includes `name` and `source` fields
- `api.New()` signature expanded to accept `InstanceStore`
- Sidebar shows Administration nav for users with `user_management` OR `instance_management` permission
- Sidebar server name uses fallback chain: `name || id || host:port`
- Removed `PermissionGate` from `/admin` route — page handles its own tab-level permissions

### Notes
- 4 commits: pool refactor, instance backend, frontend UI, lint fix
- Backend: 3 new files, 8 modified — go build, go vet, go test, golangci-lint all pass
- Frontend: 5 new files, 4 modified — 935 lines added, tsc + vite build pass

## [M5_05] — 2026-03-04 — Alert Management UI

### Added
- **AlertsDashboard page**: Full active alerts view replacing placeholder — severity/state/instance filters, sortable table with live duration, count badge, "All clear" empty state with CheckCircle icon
- **AlertRules page**: Full rule management replacing placeholder — create/edit/delete rules, enable/disable toggle, system rule protection, channel management
- **RuleFormModal component**: Create/edit alert rule form with validation — builtin rules restrict editable fields (threshold, cooldown, channels, enabled only), test notification button, escape/click-outside to close
- **DeleteConfirmModal component**: Confirmation dialog for custom rule deletion with useDeleteAlertRule mutation
- **AlertFilters component**: Toggle buttons for severity (All/Warning/Critical) and state (Firing/Resolved/All) with instance dropdown, matching TimeRangeSelector button style
- **AlertRow component**: Table row with severity badge, rule name, instance, metric, value vs threshold, state, fired timestamp, live/static duration — click navigates to server detail
- **RuleRow component**: Table row with system badge for builtin rules, operator/threshold display, severity, cooldown, channel chips, toggle switch, edit/delete action buttons
- **useAlerts hook**: Fetches GET /api/v1/alerts with client-side filtering (backend has no query params), 30s refetch
- **useAlertHistory hook**: Fetches GET /api/v1/alerts/history with server-side query params (instance_id, severity, unresolved, limit)
- **useAlertRules hook**: Fetches GET /api/v1/alerts/rules (60s refetch), useSaveAlertRule (POST/PUT mutation), useDeleteAlertRule (DELETE), useTestNotification (POST single channel)
- **AlertRule TypeScript type**: Matches Go alert.Rule struct exactly (operator, source, single threshold+severity, consecutive_count, cooldown_minutes)
- **AlertSeverityFilter, AlertStateFilter types**: Filter state types for alerts page

### Changed
- InstanceAlerts component now includes "View all alerts" link navigating to `/alerts?instance_id=${instanceId}`
- useInstanceAlerts fixed: removed misleading query param from GET /alerts (backend ignores it), now filters client-side

### Notes
- Frontend-only iteration — zero backend changes
- All TypeScript types aligned to actual Go backend structs (not design doc assumptions)
- 11 files, ~1,415 lines of frontend code
- tsc, eslint, vite build, go build, go test, golangci-lint all pass

## [M5_04] — 2026-03-03 — Statements, Lock Tree & Progress Monitoring

### Added
- **StatementsSection component**: Top-N query table with sort by total_time/io_time/cpu_time/calls/rows, pg_stat_statements config display, fill percentage indicator
- **LockTreeSection component**: Hierarchical lock tree with indented depth display, root blocker highlighting, blocked-by/blocking counts, summary card
- **ProgressSection component**: Active maintenance operations (vacuum, analyze, create_index, cluster, basebackup, copy) with phase display and progress bar
- **useStatements hook**: Fetches GET /instances/{id}/activity/statements with sort and limit params, 10s refetch
- **useLockTree hook**: Fetches GET /instances/{id}/activity/locks, 10s refetch
- **useProgress hook**: Fetches GET /instances/{id}/activity/progress, 10s refetch
- **3 new API endpoints**: GET statements, GET locks, GET progress (added to server detail activity group)
- **TypeScript types**: StatementsResponse, StatementEntry, StatementsConfig, LockTreeResponse, LockEntry, LockTreeSummary, ProgressResponse, ProgressOperation

### Changed
- Server Detail page expanded from 8 to 11 sections with statements, lock tree, and progress tabs

## [M5_03] — 2026-03-03 — Live Data Integration

### Added
- **5 new API endpoints**: GET metrics/current, metrics/history, replication, wait-events, long-transactions (24 total)
- **InstanceConnProvider interface**: Live pgx.Conn per API request to monitored instances (separate from collector connections)
- **Orchestrator.ConnFor()**: Opens fresh connection by instance ID with 5s timeout and application_name = pgpulse_api
- **Storage query methods**: CurrentMetrics (DISTINCT ON), HistoryMetrics (date_trunc aggregation), CurrentMetricValues (fleet enrichment)
- **Fleet enrichment**: `?include=metrics,alerts` query param on GET /instances for one-call fleet loading
- **Fleet Overview page**: Real data via useInstances hook, InstanceCard grid with status dots, metric sparklines, alert badges
- **Server Detail page**: 8 sections — header, key metrics row, time range selector, connection/cache charts, replication, wait events, long transactions, alerts
- **TimeRangeSelector component**: Preset buttons (1h, 6h, 24h, 7d, 30d) + custom date range picker
- **ECharts components**: TimeSeriesChart (line/area with reference lines), ConnectionGauge (semicircular green/amber/red), WaitEventsChart (horizontal bars)
- **TanStack Query hooks**: useInstances, useCurrentMetrics, useMetricsHistory, useReplication, useWaitEvents, useLongTransactions, useInstanceAlerts
- **Zustand timeRangeStore**: Preset-based time ranges computing from/to at query time
- **Formatter library**: formatBytes, formatUptime, formatDuration, formatPercent, formatPGVersion, formatTimestamp, thresholdColor
- **ECharts dark theme**: pgpulse-dark registered globally with brand color palette
- **Server detail components**: HeaderCard, KeyMetricsRow, ReplicationSection, WaitEventsSection, LongTransactionsTable, InstanceAlerts
- **AlertBadge component**: Pill badges for critical/warning counts on fleet cards
- **68 API tests passing** (6 new for metrics, 6 for activity, 3 for replication)

### Changed
- Fleet Overview and Server Detail pages fully rewritten — all mock data removed
- Sidebar dynamically loads instance list from API via useInstances()
- `?include=metrics,alerts` enriches instance list response with latest metric values and active alert counts
- ECharts MarkLineComponent added to echarts-setup for reference lines

### Housekeeping
- Fixed static.go errcheck: `f.Close()` → `_ = f.Close()`
- Wired `apiServer.SetConnProvider(orch)` in main.go so replication/activity endpoints work with real instances
- golangci-lint: 0 issues (was 1 pre-existing + 3 new, all fixed)

## [M5_02] — 2026-03-03 — Auth + RBAC UI

### Added
- **Permission-based RBAC**: 4 roles (super_admin, roles_admin, dba, app_admin) × 5 permissions replacing 2-role hierarchy
- **Separate JWT refresh secret**: `refresh_secret` config field with backwards-compatible fallback to `jwt_secret`
- **Claims include permissions**: Access tokens carry `perms` array and `type` (access/refresh) discriminator
- **ValidateRefreshToken()**: Dedicated method using refresh secret, rejects access tokens
- **UserStore expanded**: 5 new methods — GetByID, List, Update, UpdatePassword, UpdateLastLogin
- **User active/deactivation**: `active` field on User, deactivated users rejected on login and refresh
- **5 new API endpoints**: POST /auth/register, GET /auth/users, PUT /auth/users/{id}, PUT /auth/me/password (19 total)
- **RequirePermission middleware**: Permission-based route guards replacing RequireRole
- **Security headers middleware**: CSP, X-Frame-Options: DENY, X-Content-Type-Options: nosniff, Referrer-Policy
- **Migration 005_expand_roles.sql**: admin→super_admin, viewer→dba, adds `active` and `last_login` columns
- **Frontend auth store**: Access token in Zustand memory, refresh token in localStorage with auto-refresh
- **Frontend API client**: apiFetch() with 401 auto-refresh and concurrent request queuing
- **Frontend permissions**: hasPermission(), getPermissions() mirroring backend RBAC
- **ProtectedRoute + PermissionGate**: Route guards for authentication and permission checks
- **Login page**: Real form with error display and 429 rate limit countdown
- **User management page**: UsersPage with create, activate/deactivate, role change
- **User dropdown in TopBar**: Username, role badge, change password, sign out
- **Sidebar permission filtering**: Nav items filtered by user permissions
- **CreateUserModal, DeactivateUserDialog, ChangePasswordModal**: Admin UI components
- **TanStack Query hooks**: useUsers, useCreateUser, useUpdateUser, useChangePassword

### Changed
- Login response now includes `user` object alongside token pair
- Login handler checks `active` field, updates `last_login` timestamp
- Refresh handler validates via separate refresh secret and checks user active status
- Alert mutation routes use `RequirePermission(PermAlertManagement)` instead of `RequireRole("admin")`
- Initial admin seed uses `super_admin` role (was: `admin`)
- `api.New()` signature unchanged but internal wiring uses permission middleware
- User.id in frontend models changed from `string` to `number`

## [M5_01] — 2026-03-03 — Frontend Scaffold & Application Shell

### Added
- **React 18 + TypeScript frontend** with Vite build pipeline
- **Dark-first theme system** with CSS variables, light mode toggle, system preference detection
- **Application shell** layout: collapsible sidebar (240px/64px), top bar with breadcrumb, content area, status bar
- **Component library skeleton**: StatusBadge, MetricCard, DataTable, PageHeader, Spinner, EmptyState, ErrorBoundary, ThemeToggle, EChartWrapper
- **Apache ECharts integration** via echarts-for-react with custom dark/light themes and tree-shaking
- **React Router v6** with 9 routes: Fleet Overview, Server Detail, Database Detail, Alerts Dashboard, Alert Rules, Administration, User Management, Login, 404
- **State management**: Zustand (theme, layout, auth stores) + TanStack Query v5 (server data fetching)
- **Fleet Overview showcase page** with mock metric cards, ECharts line chart with data zoom, sortable server table
- **go:embed integration**: frontend builds to web/dist/, embedded into Go binary, served by chi router with SPA fallback
- **CORS middleware** (optional, for Vite dev server proxy during development)
- **Health check hook**: StatusBar shows API connection status via /api/v1/health

### Changed
- Server continues running when orchestrator has no connected instances (graceful degradation)
- Go test commands use `./cmd/... ./internal/...` instead of `./...` to skip web/node_modules/

### Tech Stack
- React 18.3, TypeScript 5.8, Vite 6.4, Tailwind CSS 4.1
- Apache ECharts 5.6 via echarts-for-react 3.0
- Zustand 5.0, TanStack Query 5.75, React Router 7.6
- Lucide React for icons, ESLint 9.x with flat config
