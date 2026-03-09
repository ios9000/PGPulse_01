# PGPulse — Context Restoration Prompt

> **Usage:** Paste this entire block into a new Claude.ai chat to resume the project.
> **Last updated:** 2026-03-09

---

You are helping develop **PGPulse** — a PostgreSQL Health & Activity Monitor
written in Go, rewritten from a legacy PHP project called PGAM.

## What PGPulse Does
- Monitors PostgreSQL instances (PG 14–18): connections, cache hit, replication,
  locks, wait events, pg_stat_statements, vacuum progress, bloat, per-database analysis
- Multi-server inventory with authentication (JWT + RBAC, 4 roles)
- Time-series metric storage (PostgreSQL, TimescaleDB-ready)
- REST API (37 endpoints) for querying metrics + operational actions
- Alerting: threshold evaluation (19 rules), state machine, email notifications
- Embedded web UI: React SPA served via go:embed, dark-first monitoring dashboard
- **Interactive DBA features (M8_01):** session kill, on-demand EXPLAIN, settings diff
- Optional OS agent for CPU/RAM/disk/iostat metrics (Linux)
- Cluster monitoring: Patroni + ETCD providers
- ML-based anomaly detection and workload forecasting (future)

## Development Method
We use a two-contour model:
- **Claude.ai (Brain)**: architecture, planning, requirements.md, design.md, team-prompt.md, review, session-log
- **Claude Code (Hands)**: Agent Teams for multi-file iterations
- One chat = one iteration. Never mix planning and coding in same session.

**Claude Code v2.1.71:** Bash works on Windows. Agents run go build/test/lint/commit directly.

## Key Documents to Read
1. `docs/legacy/PGAM_FEATURE_AUDIT.md` — legacy inventory (76 SQL queries)
2. `docs/roadmap.md` — milestone plan with current status
3. `docs/CHANGELOG.md` — what has been implemented
4. Latest `docs/save-points/LATEST.md` — full project snapshot (M8_01)
5. Latest handoff in `docs/iterations/` — transition context
6. `.claude/CLAUDE.md` — Claude Code context file (interfaces, rules, ownership)

## Repos
- PGPulse (active): https://github.com/ios9000/PGPulse_01
- PGAM (legacy archive): https://github.com/ios9000/pgam-legacy

## Current State
- Milestones completed: M0 ✅, M1 ✅, M2 ✅, M3 ✅, M4 ✅, M5 ✅, M6 ✅, M7 ✅, M8_01 ✅
- Full pipeline working: Collectors → Orchestrator → Storage → Evaluator → Dispatcher → Email → REST API → Auth → Web UI
- 37 REST API endpoints, 19 alert rules, 7 DB migrations, ~69/76 PGAM queries ported
- Next planned: M8_02 — P2 Features (auto-capture plans, plan history, temporal settings diff)

## Stack
| Component | Choice | Version |
|---|---|---|
| Language | Go | 1.24.0 |
| PG Driver | jackc/pgx v5 | 5.8.0 |
| HTTP Router | go-chi/chi v5 | 5.2.5 |
| JWT | golang-jwt/jwt v5 | 5.2.2 |
| Frontend | React 18 + TypeScript + Tailwind CSS + Apache ECharts | — |
| State Mgmt | Zustand 5.0 + TanStack Query 5 | — |
| Config | koanf v2 | 2.3.2 |
| Logging | log/slog | stdlib |
| Testing | testcontainers-go | 0.40.0 |
| Linter | golangci-lint | v2.10.1 |

## Environment
| Component | Value |
|---|---|
| OS | Windows 10 (Git Bash / MSYS2) |
| Go | 1.24.0 |
| Node.js | 22.14.0 |
| PostgreSQL | 16 (local, native installer) |
| golangci-lint | v2.10.1 |
| Claude Code | 2.1.71 |
| Git | 2.52.0 |

## Build Commands
```bash
# Frontend
cd web && npm install && npm run build   # → web/dist/

# Backend (with embedded frontend)
go build ./cmd/... ./internal/...

# Tests (NEVER go test ./... — hits web/node_modules)
go test ./cmd/... ./internal/...

# Lint
cd web && npm run lint && npm run typecheck
golangci-lint run

# Dev mode (two terminals)
cd web && npm run dev                    # Vite HMR on :5173
go run ./cmd/pgpulse-server --config configs/pgpulse.yml  # API on :8080
```

## Quick Milestone Reference
| Milestone | Name | Status |
|---|---|---|
| M0 | Project Setup | ✅ Done |
| M1 | Core Collectors | ✅ Done |
| M2 | Storage & API | ✅ Done |
| M3 | Auth & Security | ✅ Done |
| M4 | Alerting | ✅ Done |
| M5 | Web UI (MVP) | ✅ Done |
| M6 | Agent Mode + Cluster | ✅ Done |
| M7 | Per-Database Analysis | ✅ Done |
| M8_01 | P1 Features (Session Kill, EXPLAIN, Settings Diff) | ✅ Done |
| M8_02 | P2 Features (auto-plan, temporal diff) | 🔲 Next |
| M9 | ML Phase 1 | 🔲 Planned |
| M10 | Reports & Export | 🔲 Planned |
