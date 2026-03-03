# PGPulse — Context Restoration Prompt

> **Usage:** Paste this entire block into a new Claude.ai chat to resume the project.
> **Last updated:** 2026-03-03

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
- Embedded web UI: React SPA served via go:embed, dark-first monitoring dashboard
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
- M5 in progress: M5_01 (Frontend Scaffold) ✅ done 2026-03-03
- Full pipeline working: Collectors → Orchestrator → Storage → Evaluator → Dispatcher → Email → REST API → Auth → Web UI
- 14 REST API endpoints, 19 alert rules, 4 DB migrations
- Next planned: M5_02 — Auth + RBAC UI
- MVP target: ~2026-03-17

## Frontend (M5_01)
- **Framework:** React 18.3 + TypeScript 5.6 + Vite 6.4
- **Styling:** Tailwind CSS 3.4 (utility-only, CSS variables for theming)
- **Charts:** Apache ECharts 5.6 via echarts-for-react (tree-shakeable)
- **State:** Zustand 5.0 (client: theme, layout, auth) + TanStack Query 5.90 (server: API data, polling)
- **Routing:** React Router 6.30
- **Icons:** Lucide React 0.460
- **Embedding:** go:embed via `web/embed.go` → `internal/api/static.go` (SPA fallback)
- **Theme:** Dark-first design (Bloomberg Terminal density), light mode available
- **Shell:** Collapsible sidebar (240px ↔ 64px) + TopBar + StatusBar + Breadcrumb
- **Pages:** FleetOverview (with mock data + ECharts demo), ServerDetail, DatabaseDetail, AlertsDashboard, AlertRules, Administration, UserManagement, Login, NotFound
- **Bundle:** ~429 KB gzipped (excl. ECharts 347KB chunk)
- **Build:** `cd web && npm run build` → `web/dist/` → `go build ./cmd/... ./internal/...`

### Architecture Decisions (M5)

| ID | Decision | Rationale |
|---|---|---|
| D81 | 4-role RBAC (super_admin, roles_admin, dba, app_admin) | Finer-grained than admin/viewer; matches real DBA team structures |
| D82 | Dark-first design, Bloomberg Terminal density | DBAs live in dark terminals; information density over pretty cards |
| D83 | Zustand (client state) + TanStack Query v5 (server state) | Clean separation, no Redux overhead |
| D84 | React 18 + TypeScript (not Vue/Svelte) | Complex state management, larger ecosystem for dashboards |
| D85 | Apache ECharts via echarts-for-react | Native dark mode, rich chart types, tree-shakeable |
| D88 | go:embed via web/embed.go package export | Single binary deployment |
| D89 | Tailwind utilities only, CSS variables for theme | No CSS-in-JS, consistent design tokens |

### M5 Iteration Plan

| Iteration | Scope | Status |
|---|---|---|
| M5_01 | Frontend Scaffold & Application Shell | ✅ Done 2026-03-03 |
| M5_02 | Auth + RBAC UI (login, guards, user mgmt) | 🔲 Next |
| M5_03 | Fleet and Server Views (real API data) | 🔲 Planned |
| M5_04 | Database and Query Views | 🔲 Planned |
| M5_05 | Alerts UI + Polish | 🔲 Planned |

## What Exists
- **~35 frontend files** in `web/`: React SPA with app shell, component library, stores, hooks, types, ECharts integration
- **20 collector files** covering 33 PGAM queries (instance metrics, replication, progress, statements, locks, wait events)
- **Alert pipeline**: Evaluator (state machine, 19 rules, hysteresis, cooldown), Dispatcher (async, retry), Email notifier (SMTP, HTML templates), History (30-day cleanup)
- **Orchestrator** with 3 interval groups (high=10s, medium=60s, low=300s), per-instance goroutines, post-collect evaluator hook; graceful degradation when no instances connected
- **Config** loader via koanf v2 (YAML + env overrides), AlertingConfig, EmailConfig, CORSEnabled/CORSOrigin
- **Storage** layer: PGStore (CopyFrom writes, dynamic queries), migrations (001-004), LogStore fallback
- **REST API**: 14 endpoints via chi v5 (health, auth ×3, instances ×3, alerts ×7), middleware stack, CSV export
- **Auth**: JWT (HS256, access+refresh), bcrypt, RBAC (admin/viewer), rate limiting, UserStore, initial admin seeding
- **Static file server**: SPA fallback, cache-control for hashed assets, embedded via go:embed
- **Version gates** for PG 14–17 differences

## Environment
| Component | Value |
|---|---|
| OS | Windows 10 (Git Bash / MSYS2) |
| Go | 1.24.0 |
| Node.js | 22.14.0 |
| PostgreSQL | 16 (local, native installer) |
| golangci-lint | v2.10.1 |
| Claude Code | 2.1.63+ |
| Git | 2.52.0 |

## Build Commands
```bash
# Frontend
cd web && npm install && npm run build   # → web/dist/

# Backend (with embedded frontend)
go build ./cmd/... ./internal/...

# Tests
go test ./cmd/... ./internal/...

# Lint
cd web && npm run lint && npm run typecheck
golangci-lint run

# Dev mode (two terminals)
cd web && npm run dev                    # Vite HMR on :5173
go run ./cmd/pgpulse-server --config configs/pgpulse.yml  # API on :8080
```

## Quick Milestone Reference
| Milestone | Name | Duration | Status |
|---|---|---|---|
| M0 | Project Setup | 1 day | ✅ Done |
| M1 | Core Collectors | 2 days | ✅ Done |
| M2 | Storage & API | 1 day | ✅ Done |
| M3 | Auth & Security | 1 day | ✅ Done |
| M4 | Alerting | 1 day | ✅ Done |
| M5 | Web UI (MVP) | ~2 weeks | 🔄 In Progress (M5_01 done) |
| MVP Release | — | ~2026-03-17 | 🔲 |
