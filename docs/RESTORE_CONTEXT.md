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
- Alerting: threshold evaluation (19 rules), state machine, email notifications,
  async dispatch with retry, alert history with cleanup
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
5. Latest `docs/save-points/SAVEPOINT_M4_20260301.md` — full project snapshot
6. Latest handoff in `docs/iterations/` — transition context
7. `.claude/CLAUDE.md` — Claude Code context file (interfaces, rules, ownership)

## Repos
- PGPulse (active): https://github.com/ios9000/PGPulse_01
- PGAM (legacy archive): https://github.com/ios9000/pgam-legacy

## Current State
- Milestones completed: M0 ✅, M1 ✅, M2 ✅, M3 ✅, M4 ✅
- Last completed: M4 — Alerting (3 iterations: evaluator, email+dispatcher, API+wiring)
- Full pipeline working: Collectors → Orchestrator → Storage → Evaluator → Dispatcher → Email → REST API → Auth
- 14 REST API endpoints, 19 alert rules, 4 DB migrations
- Next planned: M5 — Web UI (MVP)
- Go version: 1.24.0
- golangci-lint: v2.10.1
- Claude Code: 2.1.63 (bash works on Windows)

## What Exists
- **20 collector files** covering 33 PGAM queries (instance metrics, replication, progress, statements, locks, wait events)
- **Alert pipeline**: Evaluator (state machine, 19 rules, hysteresis, cooldown), Dispatcher (async, retry), Email notifier (SMTP, HTML templates), History (30-day cleanup)
- **Orchestrator** with 3 interval groups (high=10s, medium=60s, low=300s), per-instance goroutines, post-collect evaluator hook
- **Config** loader via koanf v2 (YAML + env overrides), AlertingConfig, EmailConfig
- **Storage** layer: PGStore (CopyFrom writes, dynamic queries), migrations (001-004), LogStore fallback
- **REST API**: 14 endpoints via chi v5 (health, auth ×3, instances ×3, alerts ×7), middleware stack, CSV export
- **Auth**: JWT (HS256, access+refresh), bcrypt, RBAC (admin/viewer), rate limiting, UserStore, initial admin seeding
- **Version gates** for PG 14–17 differences

## Quick Milestone Reference
| Milestone | Name | Duration | Status |
|---|---|---|---|
| M0 | Project Setup | 1 day | ✅ Done |
| M1 | Core Collectors | 2 days | ✅ Done |
| M2 | Storage & API | 1 day | ✅ Done |
| M3 | Auth & Security | 1 day | ✅ Done |
| M4 | Alerting | 1 day | ✅ Done |
| M5 | Web UI (MVP) | ~2 weeks | 🔲 Next |
| MVP Release | — | — | 🔲 |
