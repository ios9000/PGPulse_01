# PGPulse — Roadmap

**Last updated:** 2026-02-25

---

## Milestone Status

| Milestone | Name | Status | Completion Date |
|---|---|---|---|
| M0 | Project Setup | ✅ Done | 2026-02-25 |
| M1 | Core Collector | 🔶 In Progress | — |
| M2 | Storage & API | 🔲 Not Started | — |
| M3 | Auth & Security | 🔲 Not Started | — |
| M4 | Alerting | 🔲 Not Started | — |
| M5 | Web UI (MVP) | 🔲 Not Started | — |
| M6 | Agent Mode | 🔲 Not Started | — |
| M7 | P1 Features | 🔲 Not Started | — |
| M8 | ML Phase 1 | 🔲 Not Started | — |
| M9 | Reports & Export | 🔲 Not Started | — |
| M10 | Polish & Release | 🔲 Not Started | — |

## M1 Sub-Iterations

| Iteration | Scope | PGAM Queries | Version Gates | Status |
|---|---|---|---|---|
| M1_01 | Server info, connections, cache, transactions, sizes, settings, extensions | Q2–Q3, Q9–Q19 | backup (PG14), pgss_info | ✅ Done 2026-02-25 |
| M1_02 | Replication: physical + logical, slots, WAL receiver | Q20–Q41 | slot columns (PG15+, PG16+) | 🔲 Not Started |
| M1_03 | Checkpoint, bgwriter, WAL generation, pg_stat_io | — | bgwriter/checkpointer split (PG17), pg_stat_io (PG16+) | 🔲 Not Started |
| M1_04 | pg_stat_statements analysis (IO + CPU sorted) | Q48–Q52 | Minimal (PG 14 floor) | 🔲 Not Started |
| M1_05 | Locks, wait events, long transactions | Q53–Q58 | Minimal | 🔲 Not Started |

## Query Porting Progress

| Source | Total Queries | Ported | Deferred | Remaining |
|---|---|---|---|---|
| analiz2.php Q1–Q19 | 19 | 13 (Q1–Q3, Q9–Q19) | 6 (Q4–Q8 → M6) | 0 |
| analiz2.php Q20–Q41 | 22 | 0 | 0 | 22 |
| analiz2.php Q42–Q47 | 6 | 0 | 0 | 6 |
| analiz2.php Q48–Q52 | 5 | 0 | 0 | 5 |
| analiz2.php Q53–Q58 | 6 | 0 | 0 | 6 |
| analiz_db.php Q1–Q18 | 18 | 0 | 0 | 18 |
| **Total** | **76** | **13** | **6** | **57** |
