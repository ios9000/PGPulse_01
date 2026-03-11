# PGPulse Development Strategy v2.0
## Claude AI + Claude Code Agent Teams — Project-Specific Rules

**Project:** PGPulse — PostgreSQL Health & Activity Monitor  
**Rewrite from:** PGAM (legacy PHP) → Go  
**Date:** 2026-02-25  
**Version:** 2.4 — Agent Teams + Save Point System + Codebase Digest

---

## What Changed from v1.0

| v1.0 (Two Contours) | v2.0 (Two Contours + Agent Teams) |
|---|---|
| Claude Code = single sequential session | Claude Code = Team Lead + parallel specialists |
| One collector, then one API, then tests | Collector + API + QA working simultaneously |
| Manual task decomposition by developer | Team Lead decomposes and coordinates automatically |
| Context bloat in long sessions | Each agent has fresh context window (~40% usage vs ~90%) |
| Estimated 24 weeks solo | Estimated 16–18 weeks with parallel agents |

**What stays the same:** Claude.ai = Brain (architecture, planning, review). Agent Teams only replaces the *execution* contour.

---

## Problems We Are Solving

| Problem | Solution |
|---------|----------|
| **Loss of Context** — New chat can't read repo or previous chats | **Iteration Handoff** documents (self-contained, uploaded per chat); **Project Knowledge** for stable docs |
| **Loss of History** — Unclear which prompts led to which decisions | **Session-log.md** after every iteration with prompt → result → commit mapping |
| **Project migration** — Must be able to restart from scratch in new Project or AI tool | **Save Points** — full project snapshots at each milestone (see `.claude/rules/save-point.md`) |
| **Chat transition** — Context doesn't transfer between Claude.ai chats | **Three-tier system:** Project Knowledge (auto-loaded) + Handoff (uploaded) + Save Point (emergency). See `.claude/rules/chat-transition.md` |
| **Context compaction** — "Compacting our conversation…" | New chat for each iteration; keep iterations focused |
| **Sequential bottleneck** — Single Claude Code session does one thing at a time | Agent Teams: 3 specialists work in parallel on independent workstreams |
| **Context window exhaustion** — Single session fills 80–90% context | Each agent has own context window; results come back summarized (~40% usage) |
| **Bash broken on Windows** — Claude Code can't run shell commands | ~~**Hybrid workflow:** agents create files, developer runs bash manually~~ **RESOLVED in v2.1.63** — agents run build/test/lint/commit directly |
| **Manual copying** — Code and docs manually transferred between environments | Claude Code reads/writes directly; Git is the single source of truth |
| **Legacy knowledge preservation** — 76 SQL queries from PGAM must not be lost | PGAM_FEATURE_AUDIT.md in **Project Knowledge** (always available) |
| **Planning blind spot** — Claude.ai (Brain) can't see the codebase during planning | **Codebase Digest** — auto-generated code map (files, interfaces, metrics, endpoints, components). See `.claude/rules/codebase-digest.md` |

---

## Persistence & Continuity System

Four layers protect project context across sessions, projects, and tools:

```
┌─────────────────────────────────────────────────────────────────┐
│  CODEBASE DIGEST (code map)                                     │
│  Machine-generated reference — files, interfaces, metrics,      │
│  endpoints, components, collectors, config schema.               │
│  Created: per iteration (after build verification)               │
│  Location: docs/CODEBASE_DIGEST.md                              │
│  Rules: .claude/rules/codebase-digest.md                         │
│  Also uploaded to: Project Knowledge (Claude.ai)                 │
├─────────────────────────────────────────────────────────────────┤
│  SAVE POINT (Mass Effect save)                                  │
│  Complete project snapshot — architecture, code, decisions,     │
│  issues, environment. Restores entire project from scratch.     │
│  Created: per milestone or monthly                              │
│  Location: docs/save-points/SAVEPOINT_M{X}_{date}.md           │
│  Rules: .claude/rules/save-point.md                             │
├─────────────────────────────────────────────────────────────────┤
│  ITERATION HANDOFF (mission briefing)                           │
│  What changed, what's next, key interfaces, known issues.       │
│  Self-contained — includes actual code, not just file paths.    │
│  Created: end of every chat                                     │
│  Location: docs/iterations/HANDOFF_M{from}_to_M{to}.md         │
│  Rules: .claude/rules/chat-transition.md                        │
├─────────────────────────────────────────────────────────────────┤
│  SESSION-LOG (audit trail)                                      │
│  What happened in one iteration — agents, commits, decisions.   │
│  Created: end of every iteration                                │
│  Location: docs/iterations/M{X}_{NN}_.../session-log.md        │
│  Stays in repo only — never uploaded to new chats.              │
└─────────────────────────────────────────────────────────────────┘
```

### Project Knowledge (Claude.ai — auto-loaded in every chat)

Upload these ONCE to Project Knowledge. They persist across all chats:

| Document | Purpose |
|---|---|
| PGPulse_Development_Strategy_v2.md | Process rules (this file) |
| PGAM_FEATURE_AUDIT.md | Legacy SQL reference (76 queries) |
| CODEBASE_DIGEST.md | Auto-generated code map (re-upload after each iteration) |
| Chat_Transition_Process.md | How to move between chats |
| Save_Point_System.md | How to create/restore save points |

### CLAUDE.md — The Entry Point

`.claude/CLAUDE.md` is the first file Claude Code reads. It contains:
- "START HERE" section pointing to latest save point and handoff
- Stack, interfaces, module ownership, rules
- Links to all rules files

If you're continuing development after any break, CLAUDE.md tells you
where to find everything. It is the index, not the content.

---

## Architecture: Two Contours + Agent Teams

```
┌─────────────────────────────────────────────────────────────────────┐
│                                                                     │
│  CONTOUR 1: Claude.ai (Project / Chat)                              │
│  🧠 BRAIN — Architecture, Planning, Review                          │
│                                                                     │
│  • Architecture decisions          • Diagrams & schemes             │
│  • Milestone planning              • Review & session-log           │
│  • Requirements discussion         • Competitive research           │
│  • SQL query design for PG         • Agent Team spawn prompts       │
│  • ML module algorithms            • Produces: requirements.md,     │
│  • RCA methodology                   design.md, team-prompt.md      │
│                                                                     │
└───────────────────────────┬─────────────────────────────────────────┘
                            │
                  You prepare the prompt
                  and copy docs to iteration folder
                            │
                            ▼
┌─────────────────────────────────────────────────────────────────────┐
│                                                                     │
│  CONTOUR 2: Claude Code Agent Teams (Terminal)                      │
│  🤲 HANDS — Parallel Implementation                                 │
│                                                                     │
│  ┌─────────────────────────────────────────────────────────────┐    │
│  │                    TEAM LEAD (Opus 4.6)                     │    │
│  │                                                             │    │
│  │  • Reads CLAUDE.md + design.md for current iteration        │    │
│  │  • Decomposes milestone into parallel tasks                 │    │
│  │  • Creates shared task list with dependencies               │    │
│  │  • Assigns work to specialists                              │    │
│  │  • Resolves merge conflicts between agents                  │    │
│  │  • Runs final validation before commit                      │    │
│  │  • Synthesizes results into summary                         │    │
│  └─────────┬───────────────────┬───────────────────┬───────────┘    │
│            │                   │                   │                 │
│  ┌─────────▼──────┐  ┌────────▼────────┐  ┌──────▼──────────┐     │
│  │   COLLECTOR     │  │   API &         │  │   QA &          │     │
│  │   AGENT         │  │   SECURITY      │  │   REVIEW        │     │
│  │                 │  │   AGENT         │  │   AGENT         │     │
│  │ Own worktree    │  │ Own worktree    │  │ Own worktree    │     │
│  │ Own context     │  │ Own context     │  │ Own context     │     │
│  │                 │  │                 │  │                 │     │
│  │ Responsibilities│  │ Responsibilities│  │ Responsibilities│     │
│  │ • Go collectors │  │ • REST API      │  │ • Unit tests    │     │
│  │ • pgx queries   │  │ • JWT + RBAC    │  │ • Integration   │     │
│  │ • Version gates │  │ • Alert engine  │  │   tests (PG     │     │
│  │ • SQL porting   │  │ • Storage layer │  │   14–17)        │     │
│  │   from PGAM     │  │ • TimescaleDB   │  │ • golangci-lint │     │
│  │ • OS agent code │  │   migrations    │  │ • SQL injection  │     │
│  │                 │  │ • Config/YAML   │  │   audit         │     │
│  │ Owns:           │  │                 │  │ • Version gate  │     │
│  │ internal/       │  │ Owns:           │  │   validation    │     │
│  │   collector/*   │  │ internal/       │  │ • Benchmarks    │     │
│  │   version/*     │  │   api/*         │  │                 │     │
│  │ cmd/            │  │   auth/*        │  │ Owns:           │     │
│  │   pgpulse-agent │  │   alert/*       │  │ *_test.go files │     │
│  │                 │  │   storage/*     │  │ .golangci.yml   │     │
│  │                 │  │ migrations/*    │  │ testdata/        │     │
│  │                 │  │ configs/*       │  │                 │     │
│  └─────────────────┘  └─────────────────┘  └─────────────────┘     │
│                                                                     │
│  Coordination: shared task list + inter-agent messaging             │
│  Isolation: separate git worktrees per agent                        │
│  Merge: only when Team Lead validates + tests pass                  │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘
```

### Key Principles

> **Claude.ai does not touch the code.**
> **Claude Code does not make architectural decisions.**
> **Team Lead coordinates — specialists execute.**
> **Agents work in parallel on independent workstreams.**
> **Merge happens only when tests pass.**

### PGPulse-Specific Boundaries

| Decision Type | Where | Who |
|---|---|---|
| "Should we use pgx or database/sql?" | Claude.ai | You + Brain |
| "Port PGAM queries 1–19 to instance.go" | Claude Code | Collector Agent |
| "Create GET /api/v1/instances/:id/metrics" | Claude Code | API & Security Agent |
| "Write integration tests for PG 14–17" | Claude Code | QA & Review Agent |
| "How to handle PG version branching?" | Claude.ai | You + Brain |
| "Which ML algorithm for anomaly detection?" | Claude.ai | You + Brain |
| "Decompose M1 into parallel subtasks" | Claude Code | Team Lead |
| "Resolve merge conflict between collector and API" | Claude Code | Team Lead |

---

## Agent Team Roster

### Core Team (M0–M5: MVP)

#### 🎯 Team Lead

| Attribute | Value |
|---|---|
| Model | Claude Opus 4.6 |
| Role | Orchestrator — never writes production code |
| Reads | CLAUDE.md, design.md, PGAM_FEATURE_AUDIT.md |
| Creates | Shared task list with dependencies and blockers |
| Coordinates | Task assignment, dependency ordering, merge sequencing |
| Validates | All tests pass before merging agent work into main |

**Team Lead does NOT:**
- Write Go code directly (delegates to specialists)
- Make architectural decisions (those come from Claude.ai via design.md)
- Skip tests before merging

#### 🔧 Collector Agent

| Attribute | Value |
|---|---|
| Specialty | PostgreSQL metric collection — the domain core |
| Owns | `internal/collector/*`, `internal/version/*`, `cmd/pgpulse-agent/` |
| Key skill | Porting PGAM PHP queries to Go with pgx + version-adaptive SQL |
| References | `docs/legacy/PGAM_FEATURE_AUDIT.md` (76 SQL queries) |
| Rules | `.claude/rules/postgresql.md`, `.claude/rules/security.md` |

**Query-to-file mapping this agent follows:**

| PGAM Source | Queries | Target File |
|---|---|---|
| analiz2.php #1–19 | Instance metrics | `internal/collector/instance.go` |
| analiz2.php #20–41 | Replication | `internal/collector/replication.go` |
| analiz2.php #42–47 | Progress monitoring | `internal/collector/progress.go` |
| analiz2.php #48–52 | pg_stat_statements | `internal/collector/statements.go` |
| analiz2.php #53–58 | Locks & wait events | `internal/collector/locks.go` |
| analiz_db.php #1–18 | Per-DB analysis | `internal/collector/database.go` |

**Critical rules for this agent:**
- All SQL uses parameterized queries via pgx (`$1`, `$2` or named args)
- Every query must have version-gated variants (PG 14/15/16/17/18)
- NEVER use `COPY ... TO PROGRAM` — OS metrics via Go agent (procfs) or pg_read_file('/proc/*') SQL method
- Set `application_name = 'pgpulse_<collector>'` on every connection
- `statement_timeout` per category: 5s (live), 60s (analysis), 30s (background)

#### 🌐 API & Security Agent

| Attribute | Value |
|---|---|
| Specialty | HTTP layer, authentication, storage, alerting |
| Owns | `internal/api/*`, `internal/auth/*`, `internal/alert/*`, `internal/storage/*`, `migrations/*`, `configs/*` |
| Key skill | REST API design, JWT/RBAC, TimescaleDB schema, alert rule engine |
| Rules | `.claude/rules/security.md`, `.claude/rules/architecture.md` |

**What this agent builds:**
- REST API with OpenAPI spec (go-chi/chi v5)
- JWT authentication with bcrypt passwords
- RBAC: 4 roles (super_admin, roles_admin, dba, app_admin) with permission-based access control
- TimescaleDB hypertable schema for metric storage
- Server inventory CRUD (replaces PGAM's raw GET params)
- Alert rule engine with configurable thresholds
- Notification dispatch: Telegram, Slack, Email, Webhook
- YAML config loading via koanf

**Critical rules for this agent:**
- Bearer token required for all mutations
- CSRF tokens for browser-submitted forms
- Rate limiting on auth endpoints
- Input validation with struct tags on all API inputs
- No passwords/tokens in source code

#### 🧪 QA & Review Agent

| Attribute | Value |
|---|---|
| Specialty | Testing, linting, security auditing, code quality |
| Owns | All `*_test.go` files, `.golangci.yml`, `testdata/`, CI config |
| Key skill | testcontainers-go (real PG), table-driven tests, security scanning |
| Rules | `.claude/rules/code-style.md`, `.claude/rules/security.md` |

**What this agent does:**
- Writes unit tests for every exported function
- Writes integration tests using testcontainers-go against PG 14, 15, 16, 17
- Runs `golangci-lint` with project config
- Scans for SQL injection patterns (string concatenation in queries)
- Validates version gate coverage (every collector handles PG 14–17)
- Checks parameterized query usage (no `fmt.Sprintf` in SQL)
- Writes benchmarks for collector cycle time
- Verifies test coverage ≥ 80% for `internal/collector/`

**Critical rules for this agent:**
- Tests must be independent (no shared state between test functions)
- Use `t.Parallel()` where safe
- Integration tests tagged with `//go:build integration`
- Each collector must have at least one test per supported PG version

### Extended Team (Unlocked at Later Milestones)

| Milestone | New Agent | Specialty | Owns |
|---|---|---|---|
| M5 | **Frontend Agent** | React + TypeScript + Tailwind CSS + Apache ECharts | `web/*` |
| M6 | **OS Agent Specialist** | procfs, sysfs, Patroni/ETCD plugin | `cmd/pgpulse-agent/`, `internal/collector/os.go`, `internal/collector/cluster.go` |
| M8 | **ML Agent** | gonum, STL decomposition, anomaly detection | `internal/ml/*`, `internal/rca/*` |

These agents spawn only when their milestone begins. No idle agents burning tokens.

---

## Project Structure on Disk

```
C:\Users\Archer\Projects\PGPulse_01\
│
├── .claude/
│   ├── CLAUDE.md                       # Main context for Team Lead
│   ├── settings.json                   # Agent Teams flag enabled
│   └── rules/
│       ├── code-style.md               # Go conventions, linter config
│       ├── architecture.md             # Module ownership, dependencies
│       ├── security.md                 # Security rules (no SQL injection, etc.)
│       ├── postgresql.md               # PG-specific rules (version gates, parameterized queries)
│       ├── chat-transition.md          # How to move context between Claude.ai chats
│       ├── save-point.md              # How to create/restore project snapshots
│       └── codebase-digest.md         # Template for auto-generated code map
│
├── cmd/
│   ├── pgpulse-server/                 # Main server binary
│   │   └── main.go
│   └── pgpulse-agent/                  # Optional OS metrics agent
│       └── main.go
│
├── internal/
│   ├── collector/                      # ← COLLECTOR AGENT territory
│   │   ├── instance.go                 # PGAM analiz2.php queries 1–19
│   │   ├── replication.go              # PGAM queries 20–41
│   │   ├── statements.go              # PGAM queries 48–52
│   │   ├── locks.go                    # PGAM queries 53–58
│   │   ├── database.go                 # PGAM analiz_db.php queries 1–18
│   │   ├── progress.go                 # PGAM queries 42–47
│   │   ├── cluster.go                  # Patroni + ETCD
│   │   ├── os.go                       # OS metrics via procfs (agent binary)
│   │   └── os_sql.go                   # OS metrics via pg_read_file('/proc/*') (no agent needed)
│   ├── version/                        # ← COLLECTOR AGENT territory
│   ├── storage/                        # ← API & SECURITY AGENT territory
│   ├── api/                            # ← API & SECURITY AGENT territory
│   ├── auth/                           # ← API & SECURITY AGENT territory
│   ├── alert/                          # ← API & SECURITY AGENT territory
│   │   └── notifier/
│   ├── ml/                             # ← ML AGENT territory (M8+)
│   ├── rca/                            # ← ML AGENT territory (M8+)
│   └── config/                         # ← API & SECURITY AGENT territory
│
├── web/                                # ← FRONTEND AGENT territory (M5+)
├── migrations/                         # ← API & SECURITY AGENT territory
├── configs/                            # ← API & SECURITY AGENT territory
│
├── deploy/
│   ├── docker/
│   ├── helm/
│   └── systemd/
│
├── docs/
│   ├── README.md
│   ├── CHANGELOG.md
│   ├── CODEBASE_DIGEST.md              # Auto-generated code map (7 sections)
│   ├── RESTORE_CONTEXT.md              # Emergency recovery (if handoff lost)
│   ├── roadmap.md                      # Milestone status tracker
│   ├── architecture.md                 # Full architecture document
│   ├── PGPulse_Development_Strategy.md # THIS FILE (also in Project Knowledge)
│   │
│   ├── save-points/                    # ← Project snapshots (Mass Effect saves)
│   │   ├── SAVEPOINT_M0_20260225.md    # State after M0
│   │   └── LATEST.md                   # Copy of most recent save point
│   │
│   ├── legacy/
│   │   ├── PGAM_FEATURE_AUDIT.md       # Complete legacy inventory (76 queries)
│   │   └── competitive-research.md     # Competitor analysis summary
│   │
│   ├── iterations/
│   │   ├── HANDOFF_M0_to_M1.md         # ← Chat transition document
│   │   ├── M0_01_02262026_project-setup/
│   │   │   ├── requirements.md
│   │   │   ├── design.md
│   │   │   ├── team-prompt.md
│   │   │   └── session-log.md
│   │   │
│   │   ├── M1_01_03012026_collector-instance/
│   │   │   ├── requirements.md
│   │   │   ├── design.md
│   │   │   ├── team-prompt.md
│   │   │   └── session-log.md
│   │   │
│   │   └── M1_02_03032026_collector-replication/
│   │       └── ...
│   │
│   └── claude_sessions/
│       └── session_02262026.md
│
├── go.mod
├── go.sum
└── Makefile
```

### Iteration Naming Convention

```
M{milestone}_{sequence}_{date}_{module-name}/
```

Examples:
- `M0_01_02262026_project-setup/`
- `M1_01_03012026_collector-instance/`
- `M4_02_03222026_alert-telegram/`
- `M8_01_04152026_ml-baseline/`

---

## CLAUDE.md (v2.0 — Agent Teams Aware)

```markdown
# PGPulse

## Description
PostgreSQL Health & Activity Monitor — Go rewrite of legacy PGAM PHP tool.
Real-time monitoring, alerting, ML-based anomaly detection, and cross-stack RCA.

## Stack
- Language: Go 1.24+
- PG Driver: jackc/pgx v5
- HTTP: go-chi/chi v5
- Storage: PostgreSQL + TimescaleDB
- Frontend: React + TypeScript + Tailwind CSS + Apache ECharts (embedded via go:embed)
- ML: gonum.org/v1/gonum (Phase 1)
- Config: koanf (YAML + env vars)
- Logging: log/slog
- Testing: testing + testcontainers-go

## Agent Teams Configuration
This project uses Claude Code Agent Teams with the following specialists:

### Team Structure
- **Team Lead**: Reads this file + design.md, decomposes tasks, coordinates
- **Collector Agent**: internal/collector/*, internal/version/*, cmd/pgpulse-agent/
- **API & Security Agent**: internal/api/*, auth/*, alert/*, storage/*, migrations/*
- **QA & Review Agent**: *_test.go, .golangci.yml, CI config

### Module Ownership (DO NOT CROSS)
Each agent works ONLY in its owned directories. If work requires
cross-module changes, the Team Lead coordinates the handoff via
shared task list.

### Merge Rules
- All agents work in separate git worktrees
- Team Lead merges only after QA Agent confirms tests pass
- Collector Agent must not modify api/ or auth/ directories
- API Agent must not modify collector/ or version/ directories

## Legacy Reference
- PGAM Feature Audit: docs/legacy/PGAM_FEATURE_AUDIT.md
- Legacy repo: https://github.com/ios9000/pgam-legacy
- Query-to-file mapping:
  - analiz2.php queries 1–19 → internal/collector/instance.go
  - analiz2.php queries 20–41 → internal/collector/replication.go
  - analiz2.php queries 42–47 → internal/collector/progress.go
  - analiz2.php queries 48–52 → internal/collector/statements.go
  - analiz2.php queries 53–58 → internal/collector/locks.go
  - analiz_db.php queries 1–18 → internal/collector/database.go

## Shared Interfaces
Agents must agree on these interfaces (defined by Team Lead before work begins):

### Collector → Storage interface
type MetricPoint struct {
    InstanceID string
    Metric     string
    Value      float64
    Labels     map[string]string
    Timestamp  time.Time
}

type MetricStore interface {
    Write(ctx context.Context, points []MetricPoint) error
    Query(ctx context.Context, query MetricQuery) ([]MetricPoint, error)
}

### Collector → Alert interface
type AlertEvaluator interface {
    Evaluate(ctx context.Context, metric string, value float64, labels map[string]string) error
}

## Rules
- All SQL: parameterized queries (pgx named args) — NEVER string concatenation
- Every collector: handle PG version ranges via internal/version gate
- Commits: "feat: ...", "fix: ...", "docs: ...", "refactor: ..."
- No COPY TO PROGRAM — OS metrics via Go agent or pg_read_file('/proc/*') SQL method
- Monitoring user: pg_monitor role (+ pg_read_server_files for SQL-based OS metrics), never superuser
- Test against PG 14, 15, 16, 17 using testcontainers-go

## Current Iteration
[UPDATED BY DEVELOPER BEFORE EACH TEAM SESSION]
```

---

## RESTORE_CONTEXT.md (v2.0)

```markdown
# PGPulse — Context Restoration Prompt

Paste this into a new Claude.ai chat to resume the project.

---

You are helping develop **PGPulse** — a PostgreSQL Health & Activity Monitor
written in Go, rewritten from a legacy PHP project called PGAM.

## What PGPulse Does
- Monitors PostgreSQL instances (PG 14–18): connections, cache hit, replication,
  locks, wait events, pg_stat_statements, vacuum progress, bloat
- Multi-server inventory with authentication (JWT + RBAC)
- Time-series metric storage (TimescaleDB)
- Alerting with Telegram/Slack/Email/Webhook delivery
- ML-based anomaly detection and workload forecasting
- Root Cause Analysis across App → DB → OS layers
- Optional OS agent for CPU/RAM/disk/iostat metrics

## Development Method: Agent Teams
We use Claude Code Agent Teams (experimental) with:
- **Team Lead** (Opus 4.6): orchestrates, decomposes, merges
- **Collector Agent**: ports PGAM SQL → Go collectors with version gates
- **API & Security Agent**: REST API, JWT, storage, alerting
- **QA & Review Agent**: tests (testcontainers PG 14–17), linting, security audit
- Extended agents unlock at M5 (Frontend), M6 (OS), M8 (ML)

## Key Documents to Read
1. `docs/CODEBASE_DIGEST.md` — auto-generated code map (files, metrics, endpoints, components)
2. `docs/legacy/PGAM_FEATURE_AUDIT.md` — legacy inventory (76 SQL queries)
3. `docs/architecture.md` — system design
4. `docs/PGPulse_Development_Strategy.md` — full Agent Teams strategy
5. `docs/roadmap.md` — milestone plan with current status
6. `CHANGELOG.md` — what has been implemented
7. Latest `docs/iterations/M*_*/session-log.md` — most recent decisions

## Repos
- PGPulse (active): https://github.com/ios9000/PGPulse_01
- PGAM (legacy archive): https://github.com/ios9000/pgam-legacy

## Current State
- Milestone: [UPDATE THIS]
- Iteration: [UPDATE THIS]
- Last completed feature: [UPDATE THIS]
- Next planned work: [UPDATE THIS]
- Agents last used: [UPDATE THIS]
```

---

## Agent Team Spawn Prompts

These are ready-to-use prompts for Claude Code. Each milestone has a tailored team-prompt.md.

### Setup (One-Time)

```bash
# 1. Enable Agent Teams
# Add to ~/.claude/settings.json:
{
  "env": {
    "CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS": "1"
  }
}

# 2. Verify Claude Code version (needs 2.1.50+)
claude --version

# 3. Start in project directory
cd ~/Projects/PGPulse_01
claude --model claude-opus-4-6
```

### M0: Project Setup — Team Prompt

```
Initialize the PGPulse project. Read CLAUDE.md for full context.

Create a team of 2 specialists:

SPECIALIST 1 — SCAFFOLD:
- Initialize Go module: github.com/ios9000/PGPulse_01
- Create full directory structure per CLAUDE.md
- Create cmd/pgpulse-server/main.go with placeholder
- Create cmd/pgpulse-agent/main.go with placeholder
- Set up go.mod with dependencies: pgx v5, chi v5, koanf, slog
- Create Makefile with targets: build, test, lint, docker
- Create deploy/docker/Dockerfile (multi-stage Go build)
- Create deploy/docker/docker-compose.yml (pgpulse + postgres + timescaledb)
- Create .golangci.yml with: errcheck, govet, staticcheck, gosimple
- Create .github/workflows/ci.yml (lint + test on push)

SPECIALIST 2 — DOCS & CONFIG:
- Create configs/pgpulse.example.yml with full sample configuration
- Create internal/version/version.go with PG version detection logic
- Create shared interfaces in internal/collector/collector.go:
  MetricPoint struct, Collector interface, MetricStore interface
- Verify all .claude/rules/ files exist and are correct
- Run go mod tidy and verify the project compiles

Coordinate through shared task list. Merge when both are done and
'go build ./...' succeeds.
```

### M1: Core Collector — Instance Metrics — Team Prompt

```
Build the instance metrics collector for PGPulse.
Read docs/iterations/M1_01_.../design.md for detailed requirements.
Reference docs/legacy/PGAM_FEATURE_AUDIT.md for SQL queries to port.

Create a team of 3 specialists:

COLLECTOR AGENT:
- Read PGAM_FEATURE_AUDIT.md queries #1–19 (instance-level metrics)
- Implement internal/collector/instance.go:
  - collectVersion() — PG version string + numeric
  - collectUptime() — server start time and uptime
  - collectConnections() — active, idle, total, max, utilization %
  - collectCacheHit() — buffer cache hit ratio
  - collectCheckpoints() — checkpoint stats
  - collectBgWriter() — background writer stats
  - collectWAL() — WAL generation rate (version-gated: xlog vs wal)
  - collectTransactionStats() — commits, rollbacks, deadlocks
  - collectTempFiles() — temp file usage
  - collectDatabaseSizes() — size per database
- Use internal/version/ for SQL variant selection (PG 14 vs 15 vs 16 vs 17)
- All SQL via pgx parameterized queries
- Set application_name = 'pgpulse_instance' on connection
- Set statement_timeout = '5s' for all queries
- Implement Collector interface from internal/collector/collector.go
- Return []MetricPoint from each collect function

API & SECURITY AGENT:
- Create internal/storage/timescale.go:
  - TimescaleDB schema for metrics (hypertable on timestamp)
  - Implement MetricStore interface (Write + Query)
- Create migrations/001_initial_schema.sql:
  - instances table (server inventory)
  - metrics hypertable
  - alerts_config table
- Create internal/api/router.go with chi router setup
- Create internal/api/instances.go:
  - GET /api/v1/instances — list monitored instances
  - GET /api/v1/instances/:id — instance details
  - GET /api/v1/instances/:id/metrics — current metrics
  - POST /api/v1/instances — add instance to inventory
- All handlers return JSON, accept JSON
- Basic error handling middleware

QA & REVIEW AGENT:
- Create internal/collector/instance_test.go:
  - TestInstanceCollector_PG16 using testcontainers-go
  - TestInstanceCollector_PG15 using testcontainers-go
  - TestInstanceCollector_PG14 using testcontainers-go
  - Test each collect function returns valid MetricPoint slice
  - Test version gate selects correct SQL
- Create internal/version/version_test.go:
  - TestVersionDetection for PG 14.x, 15.x, 16.x, 17.x
  - TestVersionGate_SelectsCorrectSQL
- Create internal/storage/timescale_test.go:
  - TestMetricStoreWrite with testcontainers
  - TestMetricStoreQuery with testcontainers
- Run golangci-lint on all new code
- Verify NO string concatenation in any SQL query
- Verify all exported functions have doc comments
- Run 'go test ./...' and report results

Coordinate through shared task list. Dependencies:
- Collector Agent and API Agent can start in parallel (both use
  interfaces from collector.go)
- QA Agent starts immediately writing test structure, fills in
  assertions once Collector/API agents commit initial code
- Team Lead: merge only when QA Agent confirms all tests pass
```

### M2: Storage & API — Team Prompt

```
Build the complete storage layer and REST API for PGPulse.
Read docs/iterations/M2_01_.../design.md for requirements.

Create a team of 3 specialists:

COLLECTOR AGENT:
- Port PGAM queries #20–41 into internal/collector/replication.go:
  - collectReplicationSlots() — slot name, type, active, lag
  - collectReplicationLag() — version-gated (xlog vs wal_lsn)
  - collectLogicalReplication() — PG ≥ 10 only
  - collectWALReceiverStatus() — standby connection info
- Port PGAM queries #42–47 into internal/collector/progress.go:
  - collectVacuumProgress()
  - collectAnalyzeProgress()
  - collectIndexProgress()
  - collectClusterProgress()
  - collectBaseBackupProgress()
  - collectCopyProgress()
- Wire all collectors into a scheduler (goroutine per group):
  - High frequency (10s): connections, locks, wait events
  - Medium (60s): statements, replication
  - Low (5min): per-DB analysis, bloat

API & SECURITY AGENT:
- Complete REST API with all CRUD endpoints
- Add Prometheus /metrics exposition endpoint
- Create internal/api/metrics.go — historical metric queries with time range
- Create internal/api/alerts.go — alert configuration CRUD
- Add request logging middleware (slog)
- Add request ID middleware
- Implement graceful shutdown
- Add health check: GET /api/v1/health
- Create OpenAPI spec in docs/api/openapi.yml

QA & REVIEW AGENT:
- Write tests for all new collectors (replication, progress)
- Write API integration tests (httptest + testcontainers)
- Test scheduler timing accuracy
- Test Prometheus metrics endpoint format
- Verify all new SQL is parameterized
- Run full test suite: 'go test -race ./...'
- Benchmark collector cycle time

Dependencies:
- Collector can start immediately (uses existing interfaces)
- API can start immediately (extends existing router)
- QA writes test stubs, fills in once code lands
- Merge only when all tests pass and 'go vet ./...' clean
```

### M3: Auth & Security — Team Prompt

```
Add authentication and RBAC to PGPulse.
Read docs/iterations/M3_01_.../design.md for requirements.

Create a team of 2 specialists:

API & SECURITY AGENT:
- Create internal/auth/jwt.go — JWT token generation + validation
- Create internal/auth/password.go — bcrypt hashing + comparison
- Create internal/auth/rbac.go — role definitions (admin, viewer)
- Create internal/auth/middleware.go — chi middleware for JWT validation
- Create internal/api/auth.go:
  - POST /api/v1/auth/login — returns JWT token
  - POST /api/v1/auth/refresh — refresh token
  - GET /api/v1/auth/me — current user info
- Add migrations/002_auth.sql — users table, roles
- Protect all existing endpoints with JWT middleware
- Viewer role: read-only access to all GET endpoints
- Admin role: full access including POST/PUT/DELETE
- CSRF token for cookie-based sessions
- Rate limiting: 10 login attempts per minute per IP

QA & REVIEW AGENT:
- Test JWT generation/validation (unit tests)
- Test password hashing (unit tests)
- Test RBAC enforcement:
  - Viewer CAN access GET /api/v1/instances
  - Viewer CANNOT access POST /api/v1/instances
  - Admin CAN access all endpoints
  - Unauthenticated gets 401
  - Wrong role gets 403
- Test rate limiting on auth endpoints
- Test token expiry and refresh flow
- Security audit: search for any unprotected endpoints
- Run full regression: 'go test -race ./...'

Merge only when QA confirms all auth tests pass AND
all previously passing tests still pass (no regressions).
```

### M4: Alerting — Team Prompt

```
Build the alert engine and notification system for PGPulse.
Read docs/iterations/M4_01_.../design.md for requirements.

Create a team of 3 specialists:

COLLECTOR AGENT:
- Create internal/alert/evaluator.go:
  - Implement AlertEvaluator interface
  - Threshold comparison logic (>, <, >=, <=, ==, !=)
  - State machine: OK → WARNING → CRITICAL → OK
  - Hysteresis: require N consecutive violations before firing
  - Cooldown: don't re-fire same alert within configurable window
- Create internal/alert/rules.go:
  - Default alert rules (all 14 from PGAM + 6 new):
    - wraparound > 50% CRITICAL, > 20% WARNING
    - connections > 80% WARNING, >= 99% CRITICAL
    - cache hit < 90% WARNING
    - commit ratio < 90% WARNING
    - inactive replication slots WARNING
    - long transactions > 1min WARNING, > 5min CRITICAL
    - blocking locks WARNING
    - bloat > 2x WARNING, > 50x CRITICAL
    - pg_stat_statements fill >= 95% WARNING
    - track_io_timing=off INFO
    - NEW: replication lag > 1MB WARNING, > 100MB CRITICAL
    - NEW: WAL spike > 3x baseline WARNING
    - NEW: query regression > 2x mean WARNING
    - NEW: disk forecast < 7 days CRITICAL
    - NEW: connection pool saturation > 90% WARNING
    - NEW: schema DDL changes INFO

API & SECURITY AGENT:
- Create internal/alert/notifier/telegram.go — Telegram Bot API
- Create internal/alert/notifier/slack.go — Slack Webhook
- Create internal/alert/notifier/email.go — SMTP sender
- Create internal/alert/notifier/webhook.go — generic HTTP webhook
- Create internal/alert/dispatcher.go:
  - Routes alerts to configured channels
  - Supports multiple channels per rule
  - Retry logic with exponential backoff
- Add API endpoints:
  - GET /api/v1/alerts — list active alerts
  - GET /api/v1/alerts/rules — list configured rules
  - POST /api/v1/alerts/rules — create/update rule
  - POST /api/v1/alerts/test — send test notification

QA & REVIEW AGENT:
- Test evaluator state machine (OK→WARNING→CRITICAL→OK)
- Test hysteresis (fires only after N violations)
- Test cooldown (doesn't re-fire too quickly)
- Test all 20 default rules with sample metric values
- Test each notifier with mock HTTP server
- Test dispatcher routing logic
- Test alert API endpoints with auth
- Run full regression suite

Collector Agent and API Agent can work in parallel.
QA writes test stubs immediately, fills assertions as code lands.
Merge only when all tests pass.
```

### M5: Web UI (MVP) — Team Prompt (4 Agents)

```
Build the embedded web frontend for PGPulse MVP.
Read docs/iterations/M5_01_.../design.md for requirements.

Create a team of 4 specialists:

COLLECTOR AGENT:
- Add WebSocket/SSE endpoint for real-time metric streaming
- Create internal/api/stream.go — SSE handler for live metrics
- Ensure all collectors emit events that can be streamed

API & SECURITY AGENT:
- Create internal/api/static.go — serve embedded frontend via go:embed
- Add CORS middleware for development mode
- Add API versioning headers

FRONTEND AGENT (NEW):
- Initialize web/ directory with React + TypeScript + Tailwind CSS + Apache ECharts
- Create dashboard views:
  - Server list / overview (all instances at a glance)
  - Instance dashboard (connections, cache, replication, WAL)
  - Database view (sizes, bloat, autovacuum status)
  - Lock tree visualization
  - pg_stat_statements top queries
  - Active alerts panel
  - Settings page (server inventory CRUD, alert rules)
- Real-time updates via SSE connection
- JWT auth flow (login page, token storage, auto-refresh)
- Responsive layout (works on desktop + tablet)
- Build output to web/dist/ for go:embed

QA & REVIEW AGENT:
- Test SSE streaming endpoint
- Test embedded static file serving
- Test CORS headers in dev mode
- Verify frontend builds without errors
- Check bundle size (target < 500KB gzipped)
- Run full backend regression suite
- Lighthouse audit on built frontend

Backend agents start immediately. Frontend agent starts once
API Agent confirms all REST endpoints are stable.
Merge only when full test suite passes.
```

---

## .claude/rules/ Files

### code-style.md

```markdown
# Go Code Style

## Formatting
- Use gofmt/goimports (enforced by CI)
- Max line length: 120 characters (soft limit)
- Use table-driven tests

## Naming
- Package names: lowercase, single word (collector, storage, alert)
- Interfaces: verb-based (Collector, Notifier, Store)
- Unexported helpers: descriptive but concise
- Test files: *_test.go in same package

## Error Handling
- Wrap errors with fmt.Errorf("context: %w", err)
- Never silently ignore errors
- Use structured logging (slog) for error reporting
- Return early on errors (guard clauses)

## Dependencies
- Minimize external dependencies
- Prefer stdlib where reasonable
- All deps must be in go.mod (no vendoring)

## Linting
- golangci-lint with config in .golangci.yml
- Enabled linters: errcheck, govet, staticcheck, gosimple, ineffassign, unused

## Agent Teams Convention
- Each agent commits with scope prefix matching their role:
  feat(collector): ..., feat(api): ..., test(collector): ...
- Agents must NOT modify files outside their owned directories
- Shared interfaces live in internal/collector/collector.go — changes
  require Team Lead coordination
```

### architecture.md

```markdown
# Architecture Rules

## Module Boundaries
These boundaries are enforced by Agent Teams module ownership.
Violations will cause merge conflicts.

internal/collector/ → COLLECTOR AGENT
  - Depends on: internal/version/, internal/collector/collector.go (interfaces)
  - Does NOT import: internal/api/, internal/auth/, internal/alert/

internal/api/ → API & SECURITY AGENT
  - Depends on: internal/storage/, internal/auth/, internal/alert/
  - Does NOT import: internal/collector/ (uses MetricStore interface)

internal/storage/ → API & SECURITY AGENT
  - Depends on: internal/collector/collector.go (MetricPoint struct)
  - Does NOT import: internal/api/, internal/auth/

internal/auth/ → API & SECURITY AGENT
  - Standalone module, no internal imports

internal/alert/ → SPLIT:
  - evaluator.go, rules.go → COLLECTOR AGENT (domain logic)
  - notifier/*, dispatcher.go → API & SECURITY AGENT (HTTP/transport)

internal/version/ → COLLECTOR AGENT
  - Standalone module, no internal imports

## Communication Between Modules
- Modules communicate through interfaces defined in collector.go
- No direct struct access across module boundaries
- Use dependency injection in main.go to wire modules together

## Concurrency
- Each collector runs in its own goroutine
- Metric writes are batched and flushed periodically
- API handlers are stateless (all state in PostgreSQL)
- Use context.Context for cancellation and timeouts everywhere
```

### security.md

```markdown
# Security Rules

## Database Connections
- All SQL MUST use parameterized queries via pgx
- NEVER concatenate user input into SQL strings
- Connection parameters from server registry, NEVER from URL params
- Monitoring user: pg_monitor role only — NEVER superuser
- pgxpool connection pool per monitored instance (max_conns configurable)

## Authentication
- JWT tokens with bcrypt-hashed passwords
- Dual-token: 15min access (memory) + 7d refresh (localStorage)
- RBAC: 4 roles (super_admin, roles_admin, dba, app_admin) with 5 permission groups
- All mutations require Bearer token
- CSRF tokens for browser-submitted forms

## OS Metrics
- NEVER use COPY TO PROGRAM
- Default: OS metrics via pg_read_file('/proc/*') with pg_read_server_files role
- Optional: OS metrics via Go agent (procfs/sysfs) for comprehensive coverage
- Binary paths configurable — never hardcoded

## Secrets
- No passwords/tokens in source code
- Use env vars or YAML config with restricted file permissions
- Future: Vault, K8s secrets

## Input Validation
- Validate all API inputs with struct tags
- Sanitize server names/IPs
- Rate limiting on auth endpoints

## Agent Teams Security
- QA Agent MUST scan for SQL injection patterns after every merge
- No agent may disable or skip security middleware
- Auth tests must cover: unauthenticated (401), wrong role (403),
  valid token (200), expired token (401)
```

### postgresql.md

```markdown
# PostgreSQL-Specific Rules

## Version Handling
- Detect PG version ONCE on first connection, cache in memory
- Use internal/version/ for version gating
- Each collector registers SQL variants per version range
- Minimum supported: PostgreSQL 14
- Target: PG 14, 15, 16, 17, 18

## SQL Conventions
- All queries: parameterized args ($1, $2 or named args)
- statement_timeout per category:
  - Live dashboard: 5s
  - Per-DB analysis: 60s
  - Background collection: 30s
- application_name = 'pgpulse_<collector>'
- pgxpool connection pool per instance (not single connection)

## Version-Specific Differences

### pg_stat_statements
- PG ≤ 12: total_time
- PG ≥ 13: total_exec_time + total_plan_time
- PG ≥ 14: pg_stat_statements_info for reset tracking

### WAL Functions
- PG < 10: pg_xlog_location_diff, pg_current_xlog_insert_location
- PG ≥ 10: pg_wal_lsn_diff, pg_current_wal_insert_lsn

### Removed Functions
- PG ≥ 15: pg_is_in_backup() removed

### System Catalogs
- PG ≥ 15: pg_stat_activity.query_id added natively
- PG ≥ 16: pg_stat_io view added
- PG ≥ 17: pg_stat_bgwriter split into pg_stat_checkpointer + pg_stat_bgwriter

## Testing
- Integration tests with testcontainers-go (real PostgreSQL)
- Test matrix: PG 14, 15, 16, 17
- Each collector: tests verifying SQL executes without error
- Mock tests for version-gating logic
```

---

## Workflow per Iteration (v2.0 — Agent Teams)

### Phase 1: Planning (Claude.ai — Brain)

1. Open a **new chat** in the Claude.ai project
2. State the milestone and feature:
   > "We're working on M1: Core Collector — specifically the instance metrics collector"
3. Discuss architecture, SQL queries to port, edge cases
4. Finalize outputs — ask Claude.ai to produce:
   - `requirements.md` — what needs to be built
   - `design.md` — how it should be built (interfaces, structs, SQL)
   - `team-prompt.md` — **ready-to-paste Agent Team spawn prompt**
5. Download all three files → place in `docs/iterations/M1_01_.../`

### Phase 2: Implementation (Claude Code — Agent Teams)

> **Claude Code v2.1.63+:** Bash works on Windows. Agents run `go build`, `go test`,
> `golangci-lint`, and `git commit` directly. No hybrid workflow needed.

1. Open Claude Code in the project directory:
   ```bash
   cd ~/Projects/PGPulse_01
   claude --model claude-opus-4-6
   ```

2. **Update CLAUDE.md** current iteration section:
   ```
   ## Current Iteration
   M1_01 — Instance Metrics Collector
   See: docs/iterations/M1_01_03012026_collector-instance/
   ```

3. **Paste the team-prompt.md** content into Claude Code

4. **Watch the agents work** in in-process mode:
   - Press `Shift+Down` to cycle to next agent
   - Press `Shift+Up` to cycle to previous agent
   - Monitor the shared task list for progress
   - You can message any agent directly to steer

5. Team Lead will:
   - Decompose the prompt into subtasks
   - Assign to specialists
   - Agents create all `.go` files, configs, and migrations
   - Agents run `go build`, `go test`, `golangci-lint`, and `git commit`

6. **Build verification** (agents do this, but verify yourself):
   ```bash
   cd web && npm run build && npm run lint && npm run typecheck
   cd .. && go build ./cmd/pgpulse-server && go test ./cmd/... ./internal/...
   golangci-lint run
   ```

7. **Feedback loop** (if needed):
   - If build fails, paste errors into Claude Code for fixes
   - Re-run build until clean

### Phase 3: Review, Digest, and Finalization (Claude.ai — Brain)

1. **Generate Codebase Digest** (Claude Code):
   > "Read the entire codebase and regenerate docs/CODEBASE_DIGEST.md
   > following the 7-section template in .claude/rules/codebase-digest.md"
2. Return to Claude.ai, share key results:
   - Which files were created/modified
   - Test results summary
   - Any decisions the Team Lead made
3. Ask Claude.ai:
   > "Create a session-log for this iteration: goals, agents used,
   > PGAM queries ported, key decisions, test results, what's pending."
4. Save as `session-log.md` in the iteration folder
5. Update `docs/roadmap.md` and `CHANGELOG.md`
6. Final push:
   ```bash
   git push origin main
   ```
7. **Upload CODEBASE_DIGEST.md to Project Knowledge** (if changed)

---

## Session-Log Template (v2.0 — Agent Teams)

```markdown
# Session: [date] — M[x]_[xx] [feature name]

## Goal
What we wanted to implement in this iteration.

## Agent Team Configuration
- Team Lead: Opus 4.6
- Specialists: Collector, API & Security, QA & Review
- Duration: ~XX minutes
- Estimated token usage: ~XXk tokens

## PGAM Queries Ported
| Query # | Description | Target Function | Agent |
|---------|-------------|-----------------|-------|
| #1 | version() | instance.go:collectVersion() | Collector |
| #10 | cache hit ratio | instance.go:collectCacheHit() | Collector |
| ... | ... | ... | ... |

## Agent Activity Summary

### Collector Agent
- Created: internal/collector/instance.go (320 lines)
- Created: internal/version/gate.go (version-gated SQL registry)
- Commit: abc1234 "feat(collector): add instance metrics"

### API & Security Agent
- Created: internal/storage/timescale.go (MetricStore implementation)
- Created: migrations/001_initial_schema.sql
- Created: internal/api/instances.go (4 endpoints)
- Commit: def5678 "feat(api): add instance API + TimescaleDB storage"

### QA & Review Agent
- Created: internal/collector/instance_test.go (12 tests)
- Created: internal/version/version_test.go (6 tests)
- All tests passing: ✅ 18/18
- golangci-lint: ✅ 0 issues
- SQL injection scan: ✅ clean
- Commit: ghi9012 "test(collector): add integration tests PG 14-17"

## Architecture Decisions (Made by Team Lead)
- Used pgx.CollectOneRow for single-value queries
- Version gate: map[VersionRange]string in version/ package
- Skipped PGAM query #4 (hostname via COPY TO PROGRAM) — agent will handle

## Merge Sequence
1. Collector Agent → merged first (defines interfaces)
2. API Agent → merged second (implements storage)
3. QA Agent → merged last (tests everything together)
4. Final 'go test -race ./...' — all pass ✅

## Not Done / Next Iteration
- [ ] Port queries 20–41 (replication) → M1_02
- [ ] Add connection pool metrics
- [ ] Benchmark collector cycle time
```

---

## Roadmap with Agent Teams Impact

| Milestone | Name | Duration (v1) | Duration (v2 Teams) | Agents | Status |
|---|---|---|---|---|---|
| M0 | Project Setup | 1 week | 3–4 days | Lead + 2 | ✅ Done |
| M1 | Core Collector | 3 weeks | 2 weeks | Lead + 3 | ✅ Done |
| M2 | Storage & API | 2 weeks | 1.5 weeks | Lead + 3 | ✅ Done |
| M3 | Auth & Security | 1 week | 4–5 days | Lead + 2 | ✅ Done |
| M4 | Alerting | 2 weeks | 1.5 weeks | Lead + 3 | ✅ Done |
| M5 | Web UI (MVP) | 3 weeks | 2 weeks | Lead + 4 | ✅ Done |
| M6 | Agent Mode | 2 weeks | 1.5 weeks | Lead + 3 | 🔲 |
| M7 | Per-Database Analysis | 3 weeks | 2 weeks | Lead + 3 | 🔲 |
| M8 | ML Phase 1 | 3 weeks | 2 weeks | Lead + 4 | 🔄 In Progress (M8_11) |
| M9 | Reports & Export | 2 weeks | 1.5 weeks | Lead + 3 | 🔲 |
| M10 | Polish & Release | 2 weeks | 1.5 weeks | Lead + 3 | 🔲 |

**MVP (M0–M5): ~8–9 weeks** (was 12 weeks in v1)
**Full project: ~16–18 weeks** (was 24 weeks in v1)

---

## Git Discipline (Agent Teams Edition)

### Branch Strategy with Worktrees

```
main                                    ← stable, merged code
  ├── worktree/collector-agent/         ← Collector Agent works here
  ├── worktree/api-agent/               ← API & Security Agent works here
  └── worktree/qa-agent/                ← QA Agent works here
```

Agent Teams automatically manages git worktrees. Each agent's changes are isolated until Team Lead merges.

### Commit Format (Agent-Scoped)

```
feat(collector): add instance metrics with version-adaptive SQL
feat(api): add GET /api/v1/instances/:id/metrics endpoint
feat(auth): add JWT middleware with RBAC enforcement
test(collector): add integration tests for PG 14-17
test(api): add HTTP handler tests with auth
fix(collector): handle PG < 10 xlog function names
docs(iteration): add M1_01 session-log with agent activity
chore(ci): add GitHub Actions workflow for lint+test+build
```

### Merge Order Convention
Team Lead always merges in dependency order:
1. **Version/interfaces first** (shared contracts)
2. **Collector** (produces data)
3. **Storage** (persists data)
4. **API** (serves data)
5. **Auth** (protects API)
6. **QA tests last** (validates everything)

---

## Cost Management

Agent Teams consume more tokens than single sessions. Budget accordingly.

### Estimated Token Usage per Milestone

| Milestone | Agents | Est. Input Tokens | Est. Output Tokens | Notes |
|---|---|---|---|---|
| M0 | 2 | ~200K | ~100K | Simple scaffolding |
| M1 | 3 | ~800K | ~400K | Heavy SQL porting |
| M2 | 3 | ~600K | ~300K | API + storage |
| M3 | 2 | ~300K | ~150K | Auth is focused |
| M4 | 3 | ~500K | ~250K | Alert engine + notifiers |
| M5 | 4 | ~1M | ~500K | Frontend is verbose |

### Cost Optimization Tips
- Use Agent Teams for complex milestones (M1, M2, M4, M5)
- Use single Claude Code session for focused tasks (bugfixes, small features)
- QA Agent can run after Collector + API finish (don't spawn all 3 from start)
- Monitor token usage in Claude Code via `/cost` command

---

## What NOT to Do (v2.0)

| Anti-pattern | Why It's Bad | What to Do Instead |
|---|---|---|
| Spawning 7 agents for every task | Most will idle, burning tokens | Right-size: 2 for simple, 3 for medium, 4 for complex milestones |
| Letting agents modify each other's files | Merge conflicts, inconsistent architecture | Enforce module ownership via CLAUDE.md |
| Skipping Team Lead and spawning agents directly | No coordination, no merge control | Always let Team Lead orchestrate |
| Using Agent Teams for single-file changes | Overhead exceeds benefit | Use single Claude Code session for focused tasks |
| Not pre-approving common operations | Hundreds of permission prompts | Configure allowed_tools in settings |
| Ignoring QA Agent results | Broken code gets merged | Team Lead: NEVER merge without green tests |
| One long chat for the entire project | Context gets cluttered | New chat for each iteration |
| Letting Claude Code decide architecture | Inconsistent decisions | Architecture decisions in Claude.ai only |
| Porting PGAM code literally | Carries security vulnerabilities | Port SQL queries, redesign architecture |
| Skipping session-log | Lose track of agent decisions | Always create session-log after each team session |
| ~~Including bash steps in team-prompt (Windows)~~ | ~~Agents waste time retrying failed bash~~ | **RESOLVED** — bash works in v2.1.63+. Include bash steps freely. |
| Forgetting to run go build after agents finish | Broken code gets committed | Always: go mod tidy → go build → go vet → go test before commit |
| Skipping Codebase Digest generation | Next planning chat starts blind, wastes tokens on grep | Always regenerate CODEBASE_DIGEST.md at end of iteration |
| Not re-uploading CODEBASE_DIGEST.md to Project Knowledge | Claude.ai planning chats use stale code map | Upload after every regeneration |

---

## Quick Reference Card (v2.0)

```
┌─ START ITERATION ─────────────────────────────────────────────────┐
│                                                                    │
│  1. CLAUDE.AI (Brain):                                             │
│     New chat → discuss feature → produce:                          │
│     • requirements.md                                              │
│     • design.md                                                    │
│     • team-prompt.md  ← ready-to-paste agent team prompt           │
│                                                                    │
│  2. YOU:                                                           │
│     • Copy docs to docs/iterations/M*_*/                           │
│     • Update CLAUDE.md "Current Iteration" section                 │
│                                                                    │
│  3. CLAUDE CODE (Agent Teams):                                     │
│     cd ~/Projects/PGPulse_01                                      │
│     claude --model claude-opus-4-6                                 │
│     → Paste team-prompt.md                                         │
│     → Watch agents (Shift+Down/Up to cycle between them)           │
│     → Agents create files, run build/test/lint, commit             │
│                                                                    │
│  3b. BUILD VERIFICATION (agents do this, verify yourself):         │
│     cd web && npm run build && npm run lint && npm run typecheck   │
│     cd .. && go build ./cmd/pgpulse-server                        │
│     go test ./cmd/... ./internal/...                               │
│     golangci-lint run                                              │
│                                                                    │
│  4. CLAUDE CODE (Codebase Digest):                                 │
│     → "Regenerate docs/CODEBASE_DIGEST.md per codebase-digest.md" │
│                                                                    │
│  5. CLAUDE.AI (Brain):                                             │
│     • Review results → produce session-log.md                      │
│     • Include: agent activity, queries ported, decisions, tests    │
│                                                                    │
│  6. YOU:                                                           │
│     • Update roadmap.md + CHANGELOG.md                             │
│     • git push origin main                                         │
│     • Upload CODEBASE_DIGEST.md to Project Knowledge               │
│                                                                    │
└────────────────────────────────────────────────────────────────────┘
```

---

## Known Issues & Workarounds

### ~~Claude Code Bash Broken on Windows~~ (RESOLVED)

**Status:** ✅ **Fixed in Claude Code v2.1.63** (March 2026)
**Bug report:** https://github.com/anthropics/claude-code/issues/26545

The EINVAL temp path bug that prevented all `Bash()` tool calls on Windows has been resolved.
Agents now run `go build`, `go test`, `golangci-lint`, `git commit`, and all other shell
commands directly. The hybrid workflow (agents create files, developer runs bash) is no
longer needed.

**Historical note:** This was the primary blocker from Feb 25 to Mar 1, 2026. All iterations
from M0 through M2_03 used hybrid mode. M2_04+ use direct agent execution.

### LF/CRLF Warnings on Git

**Fix:** Add `.gitattributes` to project root:
```
* text=auto eol=lf
```

### Go Version

Go 1.24.0 is the current project version (required by pgx v5.8.0).
The `go.mod` file specifies `go 1.24`.

---

## Appendix: Environment Setup Checklist

**Platform:** Windows 10/11 with Git Bash (MSYS2)  
**Display mode:** In-process (all agents in one terminal, cycle with Shift+Down/Up)

```bash
# 1. Install Node.js 22 (download MSI from https://nodejs.org)
# After install, close and reopen Git Bash, then verify:
node --version    # need v22.x.x
npm --version

# 2. Install Claude Code
npm install -g @anthropic-ai/claude-code

# Verify:
claude --version  # need 2.1.50+

# 3. Install Go 1.24 (download MSI from https://go.dev/dl/)
# After install, close and reopen Git Bash, then verify:
go version        # need 1.24+

# 4. Install golangci-lint
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
echo 'export PATH=$PATH:$HOME/go/bin' >> ~/.bashrc
source ~/.bashrc
golangci-lint --version

# 5. Configure Git
git config --global user.name "ios9000"
git config --global user.email "YOUR_EMAIL"

# 6. Enable Agent Teams (in-process mode, no tmux needed)
mkdir -p ~/.claude
cat > ~/.claude/settings.json << 'EOF'
{
  "env": {
    "CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS": "1"
  },
  "permissions": {
    "allow": [
      "Read files in project",
      "Write files in project",
      "Execute go commands",
      "Execute git commands",
      "Execute make commands"
    ]
  }
}
EOF

# 7. Clone or initialize project
git clone https://github.com/ios9000/PGPulse_01.git
cd pgpulse

# 8. Start first Agent Team session
claude --model claude-opus-4-6
# Agents work in-process: Shift+Down/Up to cycle between them
```

### Verified Environment (2026-03-10)

| Tool | Version | Status |
|---|---|---|
| Git Bash | MSYS2 / Git 2.52.0 | ✅ |
| Node.js | v22.14.0 | ✅ |
| npm | 10.9.2 | ✅ |
| Claude Code | 2.1.63+ | ✅ (bash works on Windows) |
| Agent Teams flag | enabled | ✅ |
| Go | 1.24.0 windows/amd64 | ✅ |
| golangci-lint | v2.10.1 (v2 config format) | ✅ |
| Display mode | In-process (no tmux) | ✅ |
