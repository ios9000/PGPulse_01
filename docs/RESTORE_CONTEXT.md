# PGPulse — Context Restoration Prompt

> **Usage:** Paste this entire block into a new Claude.ai chat to resume the project.
> **Last updated:** 2026-02-25

---

You are helping develop **PGPulse** — a PostgreSQL Health & Activity Monitor
written in Go, rewritten from a legacy PHP project called PGAM.

## What PGPulse Does
- Monitors PostgreSQL instances (PG 14–18): connections, cache hit, replication,
  locks, wait events, pg_stat_statements, vacuum progress, bloat
- Multi-server inventory with authentication (JWT + RBAC)
- Time-series metric storage (TimescaleDB)
- Alerting with Telegram/Slack/Email/Webhook delivery
- ML-based anomaly detection and workload forecasting (future)
- Root Cause Analysis across App → DB → OS layers (future)
- Optional OS agent for CPU/RAM/disk/iostat metrics

## Development Method: Agent Teams
We use Claude Code Agent Teams (in-process mode on Windows/Git Bash) with:
- **Team Lead** (Opus 4.6): orchestrates, decomposes, merges
- **Collector Agent**: ports PGAM SQL → Go collectors with version gates
- **API & Security Agent**: REST API, JWT, storage, alerting
- **QA & Review Agent**: tests (testcontainers PG 14–17), linting, security audit
- Extended agents unlock at M5 (Frontend), M6 (OS), M8 (ML)

## Two-Contour Model
- **Claude.ai (Brain)**: architecture, planning, requirements.md, design.md, team-prompt.md, review, session-log
- **Claude Code (Hands)**: Agent Teams implementation, testing, commits
- One chat = one iteration. Never mix planning and coding in same session.

## Key Documents to Read
1. `docs/legacy/PGAM_FEATURE_AUDIT.md` — legacy inventory (76 SQL queries)
2. `docs/architecture.md` — system design
3. `docs/PGPulse_Development_Strategy_v2.md` — full Agent Teams strategy
4. `docs/roadmap.md` — milestone plan with current status
5. `CHANGELOG.md` — what has been implemented
6. Latest `docs/iterations/M*_*/session-log.md` — most recent decisions
7. `.claude/CLAUDE.md` — Claude Code context file (interfaces, rules, ownership)

## Repos
- PGPulse (active): https://github.com/ios9000/PGPulse_01
- PGAM (legacy archive): https://github.com/ios9000/pgam-legacy

## Current State
- Milestone: M0 — Project Setup
- Iteration: M0_01_02262026_project-setup
- Last completed feature: Environment setup (Go 1.23.6, Claude Code 2.1.53, Agent Teams enabled)
- Next planned work: M0 — Initialize repo, scaffold project structure, create Dockerfile + CI
- Agents last used: Not yet (first team session pending)

## Quick Milestone Reference
| Milestone | Name | Duration | Status |
|---|---|---|---|
| M0 | Project Setup | 3–4 days | 🔲 Not started |
| M1 | Core Collector | 2 weeks | 🔲 Not started |
| M2 | Storage & API | 1.5 weeks | 🔲 Not started |
| M3 | Auth & Security | 4–5 days | 🔲 Not started |
| M4 | Alerting | 1.5 weeks | 🔲 Not started |
| M5 | Web UI (MVP) | 2 weeks | 🔲 Not started |
| MVP Release | — | ~8–9 weeks total | 🔲 |
