# PGPulse — Roadmap

**Last updated:** 2026-11-04

**Direction (since M16):** PGPulse is a fleet incident-intelligence system with cross-tier root cause analysis. Its product form is a DBA agent system: PGPulse is the evidence layer, DBPortal is the execution layer with human approval. M16–M19 below are the milestones of that direction; the PGAM-era milestones remain for history only.

---

## Milestone Status

| Milestone | Name | Status | Completion Date |
|---|---|---|---|
| M0 | Project Setup | ✅ Done | 2026-02-25 |
| M1 | Core Collector | ✅ Done | 2026-02-26 |
| M2 | Storage & API | ✅ Done | 2026-02-27 |
| M3 | Auth & Security | ✅ Done | 2026-03-01 |
| M4 | Alerting | ✅ Done | 2026-03-01 |
| M5 | Web UI (MVP) | ✅ Done | 2026-03-04 |
| M6 | Agent Mode + Cluster | ✅ Done | 2026-03-05 |
| M7 | Per-Database Analysis | ✅ Done | 2026-03-08 |
| M8 | P1 Features + ML Phase 1 | ✅ Done | 2026-03-09 |
| REM_01a | Remediation Engine (Backend) | ✅ Done | 2026-03-13 |
| REM_01b | Remediation Frontend + Backend Gaps | ✅ Done | 2026-03-14 |
| REM_01c | Remediation Metric Key Fix (bugfix) | ✅ Done | 2026-03-14 |
| M9_01 | Alert & Advisor Polish | ✅ Done | 2026-03-14 |
| M_UX_01 | Alert Detail Panel + UX Polish | ✅ Done | 2026-03-15 |
| M10_01 | Advisor Background Evaluation | ✅ Done | 2026-03-16 |
| M11_01 | PGSS Snapshots (Backend) | ✅ Done | 2026-03-16 |
| M11_02 | Query Insights UI + Workload Report | ✅ Done | 2026-03-16 |
| M12_01 | Core Desktop (Wails v3) | ✅ Done | 2026-03-17 |
| M14_01 | RCA Engine (Backend) | ✅ Done | 2026-03-21 |
| M14_02 | RCA UI | ✅ Done | 2026-03-21 |
| M14_03 | RCA Expansion + Calibration | ✅ Done | 2026-03-22 |
| M14_04 | Guided Remediation Playbooks | ✅ Done | 2026-03-25 |
| M15_01 | Maintenance Forecasting (ETA + Need) | ✅ Done | 2026-03-28 |
| M15_02 | Window Feasibility + Full UI + Integrations | ⛔ Superseded by M18 | — |
| M16 | Cut the anchor | 🔲 Next | — |
| M17 | Cluster model + cross-tier RCA | 🔲 Not Started | — |
| M18 | Operation forecasting | 🔲 Not Started | — |
| M19 | Agent loop | 🔲 Not Started | — |

### Retired milestones

These were carried over from the original PGAM feature list and are no longer milestones:

| Was | Became |
|---|---|
| M12_02 UX + Installer (Wails v3) | Windows desktop build is **frozen** at M12_01; no further desktop work is planned |
| M9 Reports & Export | **Incident evidence export** in M17 — the report is the evidence for an incident, not a dashboard printout |
| M10 Polish & Release | **Per-milestone exit criteria** — every milestone from M16 on carries its own acceptance checks; there is no separate polish phase |

---

## M16 — Cut the anchor

Remove the PGAM lineage from the project so that nothing downstream inherits it, and put the collector and metric foundations in a state M17 can build on.

| Item | Scope |
|---|---|
| Archive PGAM docs | `docs/legacy/` and PGAM feature-audit material move to `docs/archive/pgam/` with a README marking it historical; strategy, roadmap, README and CLAUDE.md reframed |
| Strip lineage comments | Remove "PGAM query #N" / `analiz2.php` references from code comments and docs that are not archived |
| Re-document collectors by tier | `docs/collectors.md`: every collector grouped by tier (PostgreSQL instance, per-database, replication, progress, statements, locks, OS, cluster), with metric keys and version gates |
| Metric naming standardization | One naming scheme for all metric keys (resolving `os.*` vs `pg.os.*`, `os.diskstat.*` vs `os.disk.*`, cache-hit-ratio mismatch), applied with a storage migration and a compatibility read path; closes the MW_01 parking note |
| Agent as default OS path | `os_metrics.method: agent` becomes the default; the SQL (`pg_read_file`) path stays as a fallback |
| Security-vetting package | A self-contained package (threat model, privilege list, SQL audit, dependency review) that a security reviewer can read without the rest of the repo |
| PG 18 in the conformance set | PostgreSQL 18 joins the test matrix (14, 15, 16, 17, 18); version gates verified |

Exit criteria: no PGAM-rewrite framing anywhere outside `docs/archive/`; link check green; conformance suite green on PG 14–18.

## M17 — Cluster model + cross-tier RCA

The cluster, not the instance, becomes the entity PGPulse reasons about, and RCA follows an incident across tiers.

| Item | Scope |
|---|---|
| Cluster entity | `internal/cluster` becomes the owner of the cluster entity and its timeline; instances, pools, proxies and HA state hang off it |
| PgBouncer and HAProxy collectors | Pool and proxy tiers collected natively (PgBouncer `SHOW` commands, HAProxy stats socket/endpoint) |
| Tier-adapter endpoint | A single endpoint through which an external tool contributes tier state to the timeline; first adapter: an external Patroni tool |
| Attribution | `application_name` and trace-context attribution so an incident can be tied to the application that caused it |
| Tier nodes and chains | Causal graph gains tier nodes; chains may start or end outside PostgreSQL (application → pool → proxy → HA → PostgreSQL → OS) |
| Evidence export | An incident's timeline, ranked chains and supporting metrics exported as a self-contained bundle (what M9 "reports" became) |

Exit criteria: one cross-tier incident reproduced on the demo kit is traced to a cause outside PostgreSQL and exported.

## M18 — Operation forecasting

| Item | Scope |
|---|---|
| Window feasibility on live runs | The M15_01 ETA/need forecasts and the window-feasibility model are validated against live maintenance runs, not synthetic data (supersedes M15_02) |
| Playbook hand-off to the execution seam | A forecast or playbook produces a proposal in the shape DBPortal's execution seam accepts; PGPulse does not execute |

Exit criteria: forecast error measured on at least one live vacuum, analyze and reindex run; one playbook handed to the seam end to end.

## M19 — Agent loop

| Item | Scope |
|---|---|
| propose → approve → execute → verify | The loop runs over M17 evidence and DBPortal's seam: PGPulse proposes with evidence, a human approves in DBPortal, DBPortal executes, PGPulse verifies the outcome on the cluster timeline |

Exit criteria: the loop completes on the demo kit with a human approval step and a recorded verification.

---

## REM_01a Sub-Iterations

| Sub | Scope | Date | Status |
|-----|-------|------|--------|
| REM_01a | Rule-based remediation engine: 25 rules (17 PG + 8 OS), Engine, PGStore, NullStore, AlertAdapter, 5 API endpoints, dispatcher integration | 2026-03-13 | ✅ Done |
| REM_01b | Remediation frontend + backend gaps: Advisor page, Diagnose button, alert enrichment, email template recommendations, AlertRow expand/collapse, handler/store tests | 2026-03-14 | ✅ Done |
| REM_01c | Metric key fix: 13 broken rules corrected, dual OS prefix support (os.*/pg.os.*), wraparound metric added to server_info collector | 2026-03-14 | ✅ Done |

---

## M14 Sub-Iterations

| Sub | Scope | Date | Status |
|-----|-------|------|--------|
| M14_01 | RCA Engine: 20 causal chains (16 Tier A + 4 Tier B), 9-step correlation algorithm, dual anomaly source (ML + threshold), incident timeline, confidence scoring, auto-trigger on CRITICAL, 5 API endpoints, migration 016, MetricStatsProvider | 2026-03-21 | ✅ Done |
| M14_02 | RCA UI: incidents list (fleet + per-instance), incident detail with timeline visualization, causal graph page (ECharts), confidence badges, quality banners, Investigate button on alerts, sidebar navigation | 2026-03-21 | ✅ Done |
| M14_03 | RCA Expansion: threshold hardening (4h+calm), WhileEffective temporal semantics, statement diff integration, Tier B activation (all 20 chains), RCA→Adviser bridge (Upsert, urgency scoring, EvaluateHook), review instrumentation, confidence refinement, JSON tag cleanup, migration 017 | 2026-03-22 | ✅ Done |
| M14_04 | Guided Remediation Playbooks: 4-table schema (migration 018), playbook engine (executor, interpreter, resolver), 10 seed playbooks, feedback worker, 19 API endpoints, catalog/detail/wizard/editor/history pages, Alert→Playbook/RCA→Playbook/Adviser→Playbook integration, 4-tier safety model, transaction-scoped execution | 2026-03-25 | ✅ Done |

---

## M15 Sub-Iterations

| Sub | Scope | Date | Status |
|-----|-------|------|--------|
| M15_01 | Maintenance Forecasting Foundation: OperationTracker (debounce, REINDEX gate), ETACalculator (WMA, min-samples, confidence), NeedEvaluator (vacuum/analyze/reindex/basebackup threshold projection), PGThresholdQuerier (SET LOCAL transactions), ForecastEngine coordinator, migration 019 (2 tables), 5 API endpoints, 6 frontend components, LinearRegression + WMA ML helpers, MaintenanceForecastConfig (15 fields) | 2026-03-28 | ✅ Done |
| M15_02 | Window Feasibility, Forecast Dashboard, Adviser/Playbook/Alert integrations | — | ⛔ Superseded — window feasibility moved to M18 (validated on live runs); dashboard and integrations folded into M17/M18 exit criteria |

---

## M12 Sub-Iterations

| Sub | Scope | Date | Status |
|-----|-------|------|--------|
| M12_01 | Core Desktop: Wails v3 scaffold, build tags, chi→Wails integration, native window (1440x900), system tray (show/hide, severity icons), --mode flag, placeholder icons | 2026-03-17 | ✅ Done |
| M12_02 | UX + Installer: connection dialog, OS toast notifications, NSIS installer | — | ❄️ Frozen — desktop build stays at M12_01 |

---

## M11 Sub-Iterations

| Sub | Scope | Date | Status |
|-----|-------|------|--------|
| M11_01 | PGSS Snapshots backend: SnapshotCapturer, ComputeDiff, pgss_snapshots tables, 7 API endpoints | 2026-03-16 | ✅ Done |
| M11_02 | Query Insights UI + Workload Report: 10 React components, HTML export, snapshot selector | 2026-03-16 | ✅ Done |

---

## M10_01 Sub-Iterations

| Sub | Scope | Date | Status |
|-----|-------|------|--------|
| M10_01 | Advisor background evaluation: BackgroundEvaluator worker, PGStore for recommendations, retention cleanup | 2026-03-16 | ✅ Done |

---

## M9_01 Sub-Iterations

| Sub | Scope | Date | Status |
|-----|-------|------|--------|
| M9_01 | Alert rules metric key audit (12 fixes), AlertsTabBar UI, sidebar expandable Alerts group, DSN keyword/value port parsing, Diagnose MetricKey/MetricValue population, DiagnosePanel formatting | 2026-03-14 | ✅ Done |

---

## M8 Sub-Iterations

M8 combined the original "P1 Features" scope (session kill, EXPLAIN, settings diff)
with "ML Phase 1" (anomaly detection, forecasting, forecast alerts) into a single
milestone, then extended with deferred UI and logical replication monitoring across
8 sub-iterations.

| Sub | Scope | Date | Status |
|-----|-------|------|--------|
| M8_01 | P1 backends: session kill API, on-demand EXPLAIN API, cross-instance settings diff API | 2026-03-09 | ✅ Done |
| M8_02 | ML anomaly detection: STL decomposition, detector bootstrap, baseline fitting | 2026-03-09 | ✅ Done |
| M8_03 | ML integration: detector wiring into orchestrator, alert evaluation hooks | 2026-03-09 | ✅ Done |
| M8_04 | ML forecasting: Holt-Winters engine, forecast API endpoint | 2026-03-09 | ✅ Done |
| M8_05 | Forecast alerts + forecast chart: ForecastProvider interface, sustained-crossing logic, ECharts overlay on connections_active | 2026-03-09 | ✅ Done |
| M8_06 | UI catch-up: session kill UI, settings diff UI, query plan viewer UI, forecast extension to all charts, toast system | 2026-03-09 | ✅ Done |
| M8_07 | Deferred UI: plan history UI, settings timeline UI, application_name enrichment, Administration.tsx lint fix | 2026-03-09 | ✅ Done |
| M8_08 | Logical replication monitoring: DB sub-collector (Q41), API endpoint, frontend section, alert rule | 2026-03-09 | ✅ Done |
| M8_09 | HOTFIX: TDZ crash, CSP, bloat PG16 compat, WAL receiver, sequences NULL, port config | 2026-03-09 | ✅ Done |
| M8_10 | HOTFIX: explain handler, breadcrumb, replication/lock/progress scan errors | 2026-03-10 | ✅ Done |

## M7 Sub-Iterations

| Sub | Scope | Date | Status |
|-----|-------|------|--------|
| M7_01 | Per-database analysis — DBCollector, DBRunner, 16 sub-collectors | 2026-03-08 | ✅ Done |

## M6 Sub-Iterations

| Sub | Scope | Date | Status |
|-----|-------|------|--------|
| M6_01 | OS agent + Patroni/ETCD providers + frontend sections | 2026-03-05 | ✅ Done |

## M5 Sub-Iterations

| Sub | Scope | Date | Status |
|-----|-------|------|--------|
| M5_01 | Frontend Scaffold | 2026-03-03 | ✅ Done |
| M5_02 | Auth + RBAC UI | 2026-03-03 | ✅ Done |
| M5_03 | Live data: Fleet Overview + Server Detail (8 sections) | 2026-03-03 | ✅ Done |
| M5_04 | Statements, Lock Tree, Progress Monitoring | 2026-03-03 | ✅ Done |
| M5_05 | Alert Management UI | 2026-03-04 | ✅ Done |
| M5_06 | Stabilization + Instance Management | 2026-03-04 | ✅ Done |
| M5_07 | User Management Enhancement | 2026-03-04 | ✅ Done |

## M4 Sub-Iterations

| Sub | Scope | Date | Status |
|-----|-------|------|--------|
| M4_01 | Evaluator engine, 19 rules, stores, migration 004_alerts.sql | 2026-03-01 | ✅ Done |
| M4_02 | Email notifier (SMTP), async dispatcher, HTML templates | 2026-03-01 | ✅ Done |
| M4_03 | 7 API endpoints, orchestrator wiring, main.go integration, cleanup | 2026-03-01 | ✅ Done |

## M3 Sub-Iterations

| Sub | Scope | Date | Status |
|-----|-------|------|--------|
| M3_01 | JWT auth, bcrypt, RBAC, rate limiting, users table, auth endpoints, middleware | 2026-03-01 | ✅ Done |

## M2 Sub-Iterations

| Sub | Scope | Date | Status |
|-----|-------|------|--------|
| M2_01 | Config loader (koanf) + Orchestrator + LogStore stub | 2026-02-26 | ✅ Done |
| M2_02 | PGStore (CopyFrom writes, dynamic queries), migration runner, pool | 2026-02-27 | ✅ Done |
| M2_03 | REST API + Wiring (chi router, metric query endpoints) | 2026-02-27 | ✅ Done |

## M1 Sub-Iterations

| Sub | Scope | PGAM Queries | Version Gates | Date | Status |
|-----|-------|-------------|---------------|------|--------|
| M1_01 | Server info, connections, cache, transactions, sizes, settings, extensions | Q2–Q3, Q9–Q19 | backup (PG14), pgss_info | 2026-02-25 | ✅ Done |
| M1_02a | InstanceContext interface refactor | — | — | 2026-02-25 | ✅ Done |
| M1_02b | Replication: physical + logical, slots, WAL receiver | Q20–Q21, Q37–Q40 | slot columns (PG15+, PG16+) | 2026-02-25 | ✅ Done |
| M1_03 | Progress monitoring + checkpoint/bgwriter | Q42–Q47, new | bgwriter/checkpointer split (PG17) | 2026-02-26 | ✅ Done |
| M1_03b | pg_stat_io | new | pg_stat_io (PG16+) | 2026-02-26 | ✅ Done |
| M1_04 | pg_stat_statements config + top-N by exec time | Q48–Q51 | Minimal (PG 14 floor) | 2026-02-26 | ✅ Done |
| M1_05 | Locks, wait events, long transactions | Q53–Q57 | Minimal | 2026-02-26 | ✅ Done |

---

## Query Porting Progress (historical — PGAM lineage, closed at M16)

| Source | Total Queries | Ported | Deferred/Skipped | Remaining |
|---|---|---|---|---|
| analiz2.php Q1–Q19 | 19 | 13 (Q2–Q3, Q9–Q19) | 6 (Q4–Q8 → agent M6) | 0 |
| analiz2.php Q20–Q41 | 22 | 5 (Q20–Q21, Q37–Q40, Q41) | 3 (Q22–Q35 → future, Q36/Q39 < PG14) | 14 |
| analiz2.php Q42–Q47 | 6 | 6 | 0 | 0 |
| analiz2.php Q48–Q52 | 5 | 4 (Q48–Q51) | 1 (Q52 deferred) | 0 |
| analiz2.php Q53–Q58 | 6 | 5 (Q53–Q57) | 1 (Q58 deferred) | 0 |
| analiz_db.php Q1–Q18 | 18 | 17 (Q2–Q18, Q1 dup skip) | 0 | 0 |
| New (not in PGAM) | — | 6 (checkpoint, pg_stat_io, OS agent, cluster) | — | — |
| **Total** | **76** | **~70** | **10** | **~14** |

## Alert Rules Summary

| Category | Count | Status |
|---|---|---|
| Original threshold rules (PGAM-era) | 14 | ✅ Active |
| New replication lag rules | 2 | ✅ Active |
| Forecast-based rules (M8) | 3 | ✅ Active (requires ML bootstrap) |
| Logical replication (M8_08) | 1 | ⏸️ disabled by default |
| Deferred (need future data) | 3 | ⏸️ enabled=false |
| **Total** | **23** | |

## REST API Endpoints (87 total)

| Method | Path | Added |
|--------|------|-------|
| GET | /api/v1/health | M2 |
| POST | /api/v1/auth/login | M3 |
| POST | /api/v1/auth/refresh | M3 |
| GET | /api/v1/auth/me | M3 |
| PUT | /api/v1/auth/me/password | M5_02 |
| POST | /api/v1/auth/register | M5_02 |
| GET | /api/v1/auth/users | M5_02 |
| PUT | /api/v1/auth/users/{id} | M5_02 |
| DELETE | /api/v1/auth/users/{id} | M5_07 |
| PUT | /api/v1/auth/users/{id}/password | M5_07 |
| GET | /api/v1/instances | M2 |
| GET | /api/v1/instances/{id} | M2 |
| POST | /api/v1/instances | M5_06 |
| PUT | /api/v1/instances/{id} | M5_06 |
| DELETE | /api/v1/instances/{id} | M5_06 |
| POST | /api/v1/instances/bulk | M5_06 |
| POST | /api/v1/instances/{id}/test | M5_06 |
| GET | /api/v1/instances/{id}/metrics | M2 |
| GET | /api/v1/instances/{id}/metrics/current | M5_03 |
| GET | /api/v1/instances/{id}/metrics/history | M5_03 |
| GET | /api/v1/instances/{id}/metrics/{metric}/forecast | **M8_04** |
| GET | /api/v1/instances/{id}/replication | M5_03 |
| GET | /api/v1/instances/{id}/activity/wait-events | M5_03 |
| GET | /api/v1/instances/{id}/activity/long-transactions | M5_03 |
| GET | /api/v1/instances/{id}/activity/statements | M5_04 |
| GET | /api/v1/instances/{id}/activity/locks | M5_04 |
| GET | /api/v1/instances/{id}/activity/progress | M5_04 |
| GET | /api/v1/instances/{id}/os | M6 |
| GET | /api/v1/instances/{id}/cluster | M6 |
| GET | /api/v1/instances/{id}/databases | M7 |
| GET | /api/v1/instances/{id}/databases/{dbname}/metrics | M7 |
| POST | /api/v1/instances/{id}/sessions/{pid}/cancel | M8_01 |
| POST | /api/v1/instances/{id}/sessions/{pid}/terminate | M8_01 |
| POST | /api/v1/instances/{id}/explain | M8_01 |
| GET | /api/v1/instances/{id}/forecast/eta | **M15_01** |
| GET | /api/v1/instances/{id}/forecast/eta/{pid} | **M15_01** |
| GET | /api/v1/instances/{id}/forecast/needs | **M15_01** |
| GET | /api/v1/instances/{id}/forecast/needs/{db}/{table} | **M15_01** |
| GET | /api/v1/instances/{id}/forecast/history | **M15_01** |
| GET | /api/v1/settings/compare | M8_01 |
| GET | /api/v1/instances/{id}/logical-replication | **M8_08** |
| GET | /api/v1/alerts | M4 |
| GET | /api/v1/alerts/history | M4 |
| GET | /api/v1/alerts/rules | M4 |
| POST | /api/v1/alerts/rules | M4 |
| PUT | /api/v1/alerts/rules/{id} | M4 |
| DELETE | /api/v1/alerts/rules/{id} | M4 |
| POST | /api/v1/alerts/test | M4 |
| GET | /api/v1/instances/{id}/recommendations | REM_01a |
| POST | /api/v1/instances/{id}/diagnose | REM_01a |
| GET | /api/v1/recommendations | REM_01a |
| GET | /api/v1/recommendations/rules | REM_01a |
| PUT | /api/v1/recommendations/{id}/acknowledge | REM_01a |
| POST | /api/v1/instances/{id}/rca/analyze | **M14_01** |
| GET | /api/v1/instances/{id}/rca/incidents | **M14_01** |
| GET | /api/v1/instances/{id}/rca/incidents/{incidentId} | **M14_01** |
| GET | /api/v1/rca/incidents | **M14_01** |
| GET | /api/v1/rca/graph | **M14_01** |
| PUT | /api/v1/rca/incidents/{incidentId}/review | **M14_03** |
| GET | /api/v1/playbooks | **M14_04** |
| GET | /api/v1/playbooks/resolve | **M14_04** |
| POST | /api/v1/playbooks | **M14_04** |
| GET | /api/v1/playbooks/{id} | **M14_04** |
| PUT | /api/v1/playbooks/{id} | **M14_04** |
| DELETE | /api/v1/playbooks/{id} | **M14_04** |
| POST | /api/v1/playbooks/{id}/promote | **M14_04** |
| POST | /api/v1/playbooks/{id}/deprecate | **M14_04** |
| POST | /api/v1/instances/{id}/playbooks/{playbookId}/run | **M14_04** |
| GET | /api/v1/instances/{id}/playbook-runs | **M14_04** |
| GET | /api/v1/playbook-runs | **M14_04** |
| GET | /api/v1/playbook-runs/{runId} | **M14_04** |
| POST | /api/v1/playbook-runs/{runId}/steps/{order}/execute | **M14_04** |
| POST | /api/v1/playbook-runs/{runId}/steps/{order}/confirm | **M14_04** |
| POST | /api/v1/playbook-runs/{runId}/steps/{order}/approve | **M14_04** |
| POST | /api/v1/playbook-runs/{runId}/steps/{order}/skip | **M14_04** |
| POST | /api/v1/playbook-runs/{runId}/steps/{order}/retry | **M14_04** |
| POST | /api/v1/playbook-runs/{runId}/abandon | **M14_04** |
| POST | /api/v1/playbook-runs/{runId}/feedback | **M14_04** |

---

## ML Components (M8)

| Component | File | Purpose |
|-----------|------|---------|
| STL Decomposition | `internal/ml/baseline.go` | Seasonal-Trend decomposition for baseline fitting |
| Anomaly Detector | `internal/ml/detector.go` | Bootstrap, baseline, Z-score anomaly evaluation |
| Forecast Engine | `internal/ml/forecast.go` | Holt-Winters forecasting with confidence bands |
| Alert Adapter | `internal/ml/detector_alert.go` | `*Detector` satisfies `alert.ForecastProvider` |
| Shared Errors | `internal/mlerrors/errors.go` | `ErrNotBootstrapped`, `ErrNoBaseline` (breaks circular import) |
| Forecast Provider | `internal/alert/forecast.go` | `ForecastProvider` interface + `ForecastPoint` mirror struct |
| Forecast Evaluator | `internal/alert/evaluator.go` | `runForecastAlerts()` with sustained-crossing logic |

## Remediation Rules (REM_01a)

| Category | Count | Examples |
|---|---|---|
| PostgreSQL | 17 | conn_high, conn_exhausted, cache_low, repl_lag_high, lock_contention, long_tx, bloat, vacuum_behind, checkpoint_freq, statements_slow, temp_files, deadlocks, wal_growth |
| OS | 8 | cpu_high, memory_pressure, swap_active, disk_full, disk_io_saturated, load_high, net_errors, oom_risk |
| **Total** | **25** | |

---

## Deferred Items (Resolved in M8_07/M8_08)

| Item | Originally Deferred | Resolved In |
|------|--------------------|-------------|
| ~~Plan capture history UI~~ | M8_02 | ✅ M8_07 |
| ~~Temporal settings snapshot UI~~ | M8_02 | ✅ M8_07 |
| ~~Session kill application_name enrichment~~ | M8_06 | ✅ M8_07 |
| ~~Administration.tsx lint error~~ | M5_06 | ✅ M8_07 |
| ~~Logical replication monitoring (Q41)~~ | M1_02b | ✅ M8_08 |

## Remaining Deferred Items

| Item | Originally Planned | Reason |
|------|--------------------|--------|
| ~~Remaining ~14 PGAM queries (Q22–Q35 etc.)~~ | M1 | Closed at M16 — PGAM lineage archived; nothing further is ported from it |
