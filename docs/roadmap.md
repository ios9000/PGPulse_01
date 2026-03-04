# PGPulse — Roadmap

**Last updated:** 2026-03-05

---

## Milestone Status

| Milestone | Name | Status | Completion Date |
|---|---|---|---|
| M0 | Project Setup | ✅ Done | 2026-02-25 |
| M1 | Core Collector | ✅ Done | 2026-02-26 |
| M2 | Storage & API | ✅ Done | 2026-02-27 |
| M3 | Auth & Security | ✅ Done | 2026-03-01 |
| M4 | Alerting | ✅ Done | 2026-03-01 |
| M5 | Web UI (MVP) | 🔨 In Progress | — |
| M6 | Agent Mode | 🔲 Not Started | — |
| M7 | P1 Features | 🔲 Not Started | — |
| M8 | ML Phase 1 | 🔲 Not Started | — |
| M9 | Reports & Export | 🔲 Not Started | — |
| M10 | Polish & Release | 🔲 Not Started | — |

## M5 Web UI (MVP) - In Progress - M5_06 done, polish remains
  M5_01 Frontend Scaffold - Done 2026-03-03
  M5_02 Auth + RBAC UI - Done 2026-03-03
  M5_03 Live data: Fleet Overview + Server Detail (8 sections) - Done 2026-03-03
  M5_04 Statements, Lock Tree, Progress Monitoring - Done 2026-03-03
  M5_05 Alert Management UI - Done 2026-03-04
  M5_06 Stabilization + Instance Management - Done 2026-03-04


## M4 Sub-Iterations

| Iteration | Scope | Status |
|---|---|---|
| M4_01 | Evaluator engine, 19 rules, stores, migration 004_alerts.sql | ✅ Done 2026-03-01 |
| M4_02 | Email notifier (SMTP), async dispatcher, HTML templates | ✅ Done 2026-03-01 |
| M4_03 | 7 API endpoints, orchestrator wiring, main.go integration, cleanup | ✅ Done 2026-03-01 |

## M3 Sub-Iterations

| Iteration | Scope | Status |
|---|---|---|
| M3_01 | JWT auth, bcrypt, RBAC, rate limiting, users table, auth endpoints, middleware | ✅ Done 2026-03-01 |

## M2 Sub-Iterations

| Iteration | Scope | Status |
|---|---|---|
| M2_01 | Config loader (koanf) + Orchestrator + LogStore stub | ✅ Done 2026-02-26 |
| M2_02 | PGStore (CopyFrom writes, dynamic queries), migration runner, pool | ✅ Done 2026-02-27 |
| M2_03 | REST API + Wiring (chi router, metric query endpoints) | ✅ Done 2026-02-27 |

## M1 Sub-Iterations

| Iteration | Scope | PGAM Queries | Version Gates | Status |
|---|---|---|---|---|
| M1_01 | Server info, connections, cache, transactions, sizes, settings, extensions | Q2–Q3, Q9–Q19 | backup (PG14), pgss_info | ✅ Done 2026-02-25 |
| M1_02a | InstanceContext interface refactor | — | — | ✅ Done 2026-02-25 |
| M1_02b | Replication: physical + logical, slots, WAL receiver | Q20–Q21, Q37–Q40 | slot columns (PG15+, PG16+) | ✅ Done 2026-02-25 |
| M1_03 | Progress monitoring (vacuum, cluster, analyze, index, basebackup, copy) + checkpoint/bgwriter | Q42–Q47, new | bgwriter/checkpointer split (PG17) | ✅ Done 2026-02-26 |
| M1_03b | pg_stat_io | new | pg_stat_io (PG16+) | ✅ Done 2026-02-26 |
| M1_04 | pg_stat_statements config + top-N by exec time | Q48–Q51 | Minimal (PG 14 floor) | ✅ Done 2026-02-26 |
| M1_05 | Locks, wait events, long transactions | Q53–Q57 | Minimal | ✅ Done 2026-02-26 |

## Query Porting Progress

| Source | Total Queries | Ported | Deferred/Skipped | Remaining |
|---|---|---|---|---|
| analiz2.php Q1–Q19 | 19 | 13 (Q1–Q3, Q9–Q19) | 6 (Q4–Q8 → M6) | 0 |
| analiz2.php Q20–Q41 | 22 | 4 (Q20–Q21, Q37–Q40) | 3 (Q22–Q35 → M6, Q36/Q39 < PG14, Q41 deferred) | 15 |
| analiz2.php Q42–Q47 | 6 | 6 | 0 | 0 |
| analiz2.php Q48–Q52 | 5 | 4 (Q48–Q51) | 1 (Q52 deferred) | 0 |
| analiz2.php Q53–Q58 | 6 | 5 (Q53–Q57) | 1 (Q58 deferred) | 0 |
| analiz_db.php Q1–Q18 | 18 | 0 | 0 | 18 |
| New (not in PGAM) | — | 2 (checkpoint, pg_stat_io) | — | — |
| **Total** | **76** | **33** | **11** | **33** |

## Alert Rules Summary (M4)

| Category | Count | Status |
|---|---|---|
| PGAM thresholds ported | 14 | ✅ Active |
| New replication lag rules | 2 | ✅ Active |
| Deferred (need M6/M8 data) | 3 | ⏸️ enabled=false |
| **Total** | **19** | |

## REST API Endpoints (29 total)

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
| GET | /api/v1/instances/{id}/replication | M5_03 |
| GET | /api/v1/instances/{id}/wait-events | M5_03 |
| GET | /api/v1/instances/{id}/long-transactions | M5_03 |
| GET | /api/v1/alerts | M4 |
| GET | /api/v1/alerts/history | M4 |
| GET | /api/v1/alerts/rules | M4 |
| POST | /api/v1/alerts/rules | M4 |
| PUT | /api/v1/alerts/rules/{id} | M4 |
| DELETE | /api/v1/alerts/rules/{id} | M4 |
| POST | /api/v1/alerts/test | M4 |
