# PGPulse — Context Restoration Prompt

> **Usage:** Paste this entire block into a new Claude.ai chat to resume the project.
> **Last updated:** 2026-02-27

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

**Known issue:** Claude Code bash execution broken on Windows (EINVAL temp path) in Agent Teams mode.
Single-session Claude Code works. All bash commands run manually by developer.

## Key Documents to Read
1. `docs/legacy/PGAM_FEATURE_AUDIT.md` — legacy inventory (76 SQL queries)
2. `docs/PGPulse_Development_Strategy_v2.md` — full development strategy
3. `docs/roadmap.md` — milestone plan with current status
4. `CHANGELOG.md` — what has been implemented
5. Latest `docs/save-points/SAVEPOINT_M2_20260227.md` — full project snapshot
6. Latest handoff in `docs/iterations/HANDOFF_M2_03_to_M3_01.md` — transition context
7. `.claude/CLAUDE.md` — Claude Code context file (interfaces, rules, ownership)

## Repos
- PGPulse (active): https://github.com/ios9000/PGPulse_01
- PGAM (legacy archive): https://github.com/ios9000/pgam-legacy

## Current State
- Milestones completed: M0 ✅, M1 ✅, M2 ✅
- Last completed: M2_03 — REST API (health, instances, metrics endpoints with JSON + CSV)
- Full pipeline working: Collectors → Orchestrator → Storage → REST API
- Next planned: M3_01 — Authentication & RBAC (JWT, bcrypt, role-based access)
- Go version: 1.24.0
- golangci-lint: v2.10.1

## What Exists
- **20 collector files** covering 33 PGAM queries (instance metrics, replication, progress, statements, locks, wait events)
- **Orchestrator** with 3 interval groups (high=10s, medium=60s, low=300s), per-instance goroutines
- **Config** loader via koanf v2 (YAML + env overrides)
- **Storage** layer: PGStore (CopyFrom writes, dynamic WHERE queries), migrations (001_metrics, 002_timescaledb conditional), LogStore fallback
- **REST API**: 4 endpoints via chi v5 (health, list instances, get instance, query metrics), middleware stack (request ID, logging, recovery, CORS, auth stub), CSV export
- **Version gates** for PG 14–17 differences (checkpoint, replication slots, pg_stat_io, etc.)

## Quick Milestone Reference
| Milestone | Name | Duration | Status |
|---|---|---|---|
| M0 | Project Setup | 1 day | ✅ Done |
| M1 | Core Collectors | 2 days | ✅ Done |
| M2 | Storage & API | 1 day | ✅ Done |
| M3 | Auth & Security | ~4–5 days | 🔲 Next |
| M4 | Alerting | ~1.5 weeks | 🔲 Not started |
| M5 | Web UI (MVP) | ~2 weeks | 🔲 Not started |
| MVP Release | — | ~8–9 weeks total | 🔲 |
