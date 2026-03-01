# PGPulse — Roadmap

**Last updated:** 2026-03-01

---

## Milestone Status

| Milestone | Name | Status | Completion Date |
|---|---|---|---|
| M0 | Project Setup | ✅ Done | 2026-02-25 |
| M1 | Core Collector | ✅ Done | 2026-02-26 |
| M2 | Storage & API | ✅ Done | 2026-02-27 |
| M3 | Auth & Security | ✅ Done | 2026-03-01 |
| M4 | Alerting | 🔲 Next | — |
| M5 | Web UI (MVP) | 🔲 Not Started | — |
| M6 | Agent Mode | 🔲 Not Started | — |
| M7 | P1 Features | 🔲 Not Started | — |
| M8 | ML Phase 1 | 🔲 Not Started | — |
| M9 | Reports & Export | 🔲 Not Started | — |
| M10 | Polish & Release | 🔲 Not Started | — |

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
