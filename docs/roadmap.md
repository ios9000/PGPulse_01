# PGPulse Roadmap

> **Last updated:** 2026-02-25  
> **MVP target:** M0–M5 (~8–9 weeks with Agent Teams)  
> **Full release target:** M0–M10 (~16–18 weeks)

## Milestone Status

### MVP Phase

| Milestone | Name | Agents | Duration | Start | End | Status |
|---|---|---|---|---|---|---|
| **M0** | Project Setup | Lead + 2 | 3–4 days | 2026-02-26 | — | 🔲 Not started |
| **M1** | Core Collector | Lead + 3 | 2 weeks | — | — | 🔲 Not started |
| **M2** | Storage & API | Lead + 3 | 1.5 weeks | — | — | 🔲 Not started |
| **M3** | Auth & Security | Lead + 2 | 4–5 days | — | — | 🔲 Not started |
| **M4** | Alerting | Lead + 3 | 1.5 weeks | — | — | 🔲 Not started |
| **M5** | Web UI (MVP) | Lead + 4 | 2 weeks | — | — | 🔲 Not started |

**MVP Release: feature parity with PGAM + multi-server + auth + time-series + alerting + modern UI**

### Post-MVP Phase

| Milestone | Name | Agents | Duration | Start | End | Status |
|---|---|---|---|---|---|---|
| **M6** | Agent Mode | Lead + 3 | 1.5 weeks | — | — | 🔲 Not started |
| **M7** | P1 Features | Lead + 3 | 2 weeks | — | — | 🔲 Not started |
| **M8** | ML Phase 1 | Lead + 4 | 2 weeks | — | — | 🔲 Not started |
| **M9** | Reports & Export | Lead + 3 | 1.5 weeks | — | — | 🔲 Not started |
| **M10** | Polish & Release | Lead + 3 | 1.5 weeks | — | — | 🔲 Not started |

**v1.0 Release: full monitoring suite with ML, RCA, reports, Helm chart**

## Iteration Log

### M0: Project Setup
| Iteration | Description | Date | Status |
|---|---|---|---|
| M0_01 | Repo init, scaffold, Dockerfile, CI, CLAUDE.md | 2026-02-26 | 🔲 Not started |

### M1: Core Collector
| Iteration | Description | Date | Status |
|---|---|---|---|
| M1_01 | Instance metrics (PGAM queries 1–19) | — | 🔲 |
| M1_02 | Replication metrics (PGAM queries 20–41) | — | 🔲 |
| M1_03 | Progress monitoring (PGAM queries 42–47) | — | 🔲 |
| M1_04 | pg_stat_statements (PGAM queries 48–52) | — | 🔲 |
| M1_05 | Locks & wait events (PGAM queries 53–58) | — | 🔲 |

### M2: Storage & API
| Iteration | Description | Date | Status |
|---|---|---|---|
| M2_01 | TimescaleDB schema + MetricStore + REST API | — | 🔲 |

### M3: Auth & Security
| Iteration | Description | Date | Status |
|---|---|---|---|
| M3_01 | JWT + RBAC + middleware | — | 🔲 |

### M4: Alerting
| Iteration | Description | Date | Status |
|---|---|---|---|
| M4_01 | Rule engine + evaluator + state machine | — | 🔲 |
| M4_02 | Notifiers (Telegram, Slack, Email, Webhook) | — | 🔲 |

### M5: Web UI
| Iteration | Description | Date | Status |
|---|---|---|---|
| M5_01 | Dashboard + instance view + real-time SSE | — | 🔲 |
| M5_02 | Settings, alerts panel, auth flow | — | 🔲 |

## PGAM Query Porting Tracker

| Source File | Queries | Target | Milestone | Status |
|---|---|---|---|---|
| analiz2.php #1–19 | Instance metrics | collector/instance.go | M1_01 | 🔲 |
| analiz2.php #20–41 | Replication | collector/replication.go | M1_02 | 🔲 |
| analiz2.php #42–47 | Progress | collector/progress.go | M1_03 | 🔲 |
| analiz2.php #48–52 | Statements | collector/statements.go | M1_04 | 🔲 |
| analiz2.php #53–58 | Locks | collector/locks.go | M1_05 | 🔲 |
| analiz_db.php #1–18 | Per-DB analysis | collector/database.go | M2_01 | 🔲 |
| **Total: 76 queries** | | | | **0/76 ported** |
