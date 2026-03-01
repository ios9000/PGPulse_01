# PGPulse — Context Restoration Prompt

> **Usage:** Paste this entire block into a new Claude.ai chat to resume the project.
> **Last updated:** 2026-03-01

---

You are helping develop **PGPulse** — a PostgreSQL Health & Activity Monitor
written in Go, rewritten from a legacy PHP project called PGAM.

## What PGPulse Does
- Monitors PostgreSQL instances (PG 14–18): connections, cache hit, replication,
  locks, wait events, pg_stat_statements, vacuum progress, bloat
- Multi-server inventory with authentication (JWT + RBAC)
- Time-series metric storage (PostgreSQL, TimescaleDB-ready)
- REST API for querying metrics (JSON + CSV export)
- Alerting with Telegram/Slack/Email/Webhook delivery (planned M4)
- ML-based anomaly detection and workload forecasting (future)
- Root Cause Analysis across App → DB → OS layers (future)
- Optional OS agent for CPU/RAM/disk/iostat metrics

## Development Method
We use a two-contour model:
- **Claude.ai (Brain)**: architecture, planning, requirements.md, design.md, team-prompt.md, review, session-log
- **Claude Code (Hands)**: single Sonnet session for focused tasks; Agent Teams for multi-file iterations
- One chat = one iteration. Never mix planning and coding in same session.

**Claude Code v2.1.63:** Bash works on Windows. EINVAL temp path bug is FIXED. Agents run go build/test/lint/commit directly. No more hybrid workflow.

## Key Documents to Read
1. `docs/legacy/PGAM_FEATURE_AUDIT.md` — legacy inventory (76 SQL queries)
2. `docs/PGPulse_Development_Strategy_v2.md` — full development strategy
3. `docs/roadmap.md` — milestone plan with current status
4. `CHANGELOG.md` — what has been implemented
5. Latest `docs/save-points/SAVEPOINT_M3_20260301.md` — full project snapshot
6. Latest handoff in `docs/iterations/HANDOFF_M3_to_M4.md` — transition context
7. `.claude/CLAUDE.md` — Claude Code context file (interfaces, rules, ownership)

## Repos
- PGPulse (active): https://github.com/ios9000/PGPulse_01
- PGAM (legacy archive): https://github.com/ios9000/pgam-legacy

## Current State
- Milestones completed: M0 ✅, M1 ✅, M2 ✅, M3 ✅
- Last completed: M3_01 — Auth & RBAC (JWT, bcrypt, role-based access, rate limiting)
- Full pipeline working: Collectors → Orchestrator → Storage → REST API → Auth
- Next planned: M4_01 — Alert Engine & Notifications
- Go version: 1.24.0
- golangci-lint: v2.10.1
- Claude Code: 2.1.63 (bash works on Windows)

## What Exists
- **20 collector files** covering 33 PGAM queries (instance metrics, replication, progress, statements, locks, wait events)
- **Orchestrator** with 3 interval groups (high=10s, medium=60s, low=300s), per-instance goroutines
- **Config** loader via koanf v2 (YAML + env overrides)
- **Storage** layer: PGStore (CopyFrom writes, dynamic WHERE queries), migrations (001_metrics, 002_timescaledb conditional, 003_users), LogStore fallback
- **REST API**: 7 endpoints via chi v5 (health, auth login/refresh/me, list instances, get instance, query metrics), middleware stack (request ID, logging, recovery, CORS, JWT auth or stub), CSV export
- **Auth**: JWT (HS256, access+refresh), bcrypt, RBAC (admin/viewer), rate limiting, UserStore, initial admin seeding
- **Version gates** for PG 14–17 differences (checkpoint, replication slots, pg_stat_io, etc.)

## Quick Milestone Reference
| Milestone | Name | Duration | Status |
|---|---|---|---|
| M0 | Project Setup | 1 day | ✅ Done |
| M1 | Core Collectors | 2 days | ✅ Done |
| M2 | Storage & API | 1 day | ✅ Done |
| M3 | Auth & Security | 1 day | ✅ Done |
| M4 | Alerting | ~1.5 weeks | 🔲 Next |
| M5 | Web UI (MVP) | ~2 weeks | 🔲 Not started |
| MVP Release | — | ~8–9 weeks total | 🔲 |
