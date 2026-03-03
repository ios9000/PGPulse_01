# PGPulse

## ⚡ START HERE — Project Continuity

If you're starting a new session on this project, read in this order:

1. **This file** — project overview, stack, interfaces, rules
2. **Latest save point** → `docs/save-points/LATEST.md`
   Full project snapshot: architecture, decisions, codebase state, milestone status.
   If this file exists, it IS your comprehensive context. Start from there.
3. **Latest handoff** → most recent `docs/iterations/HANDOFF_*.md`
   What changed in the last iteration, what's next, known issues.
4. **Rules** → `.claude/rules/` directory
   - `code-style.md` — Go conventions, commit format
   - `architecture.md` — module boundaries, concurrency model
   - `security.md` — SQL injection prevention, auth requirements
   - `postgresql.md` — version gates, PG-specific conventions
   - `chat-transition.md` — how context transfers between Claude.ai chats
   - `save-point.md` — how to create/restore project snapshots
5. **Roadmap** → `docs/roadmap.md` — milestone status and query porting tracker
6. **Legacy reference** → `docs/legacy/PGAM_FEATURE_AUDIT.md` — 76 SQL queries to port

> **DO NOT** make architecture decisions without checking the save point first.
> Decisions were already made. Check before re-deciding.

---

## Description
PostgreSQL Health & Activity Monitor — Go rewrite of legacy PGAM PHP tool.
Real-time monitoring, alerting, ML-based anomaly detection, and cross-stack RCA.

## Stack
- Language: Go 1.24.0
- PG Driver: jackc/pgx v5 (5.8.0)
- HTTP: go-chi/chi v5 (5.2.5)
- JWT: golang-jwt/jwt v5 (5.2.2)
- Crypto: x/crypto (bcrypt)
- Storage: PostgreSQL + TimescaleDB (conditional)
- Config: koanf v2 (YAML + env vars)
- Logging: log/slog
- Testing: testing + testcontainers-go
- Linter: golangci-lint v2.10.1 (v2 config format)
- Frontend: React + TypeScript + Tailwind CSS + Apache ECharts (embedded via go:embed, M5)
- ML: gonum.org/v1/gonum (M8)

## Agent Teams Configuration
This project uses Claude Code Agent Teams (in-process mode on Windows/Git Bash).

### Build & Test
Claude Code v2.1.63 — bash works on Windows. EINVAL temp path bug is FIXED.
Agents run go build, go test, golangci-lint, git commit directly.
No hybrid workflow needed.

All team prompts should include validation steps:
  go mod tidy && go build ./... && go vet ./...
  go test -race ./... && golangci-lint run
Fix any issues before declaring done.

### Team Structure
- **Team Lead**: Reads this file + design.md, decomposes tasks, coordinates
- **Collector Agent**: internal/collector/*, internal/version/*, cmd/pgpulse-agent/
- **API & Security Agent**: internal/api/*, auth/*, alert/*, storage/*, migrations/*
- **QA & Review Agent**: *_test.go, .golangci.yml, CI config

### Module Ownership (DO NOT CROSS)
Each agent works ONLY in its owned directories. If work requires
cross-module changes, the Team Lead coordinates the handoff via
shared task list.

| Agent | Owns | Does NOT touch |
|-------|------|----------------|
| Collector | internal/collector/*, internal/version/*, cmd/pgpulse-agent/ | internal/api/*, internal/auth/*, internal/storage/* |
| API & Security | internal/api/*, internal/auth/*, internal/alert/*, internal/storage/*, migrations/*, configs/* | internal/collector/*, internal/version/* |
| QA & Review | *_test.go, .golangci.yml, testdata/, .github/workflows/ | Production code (only tests and CI) |

### Merge Rules
- All agents work in separate git worktrees
- Team Lead merges only after QA Agent confirms tests pass
- Merge order: version/interfaces → collector → storage → API → auth → QA tests

## Project Structure
- cmd/ — binary entrypoints (server + agent)
- internal/ — all business logic (collector, storage, api, auth, alert, config, orchestrator, version)
- web/ — embedded frontend (future: M5)
- migrations/ — SQL migrations embedded in internal/storage/migrations/
- deploy/ — Docker, Helm, systemd
- docs/ — documentation, iterations, legacy reference, save points
- docs/save-points/ — project snapshots for continuity and disaster recovery
- .claude/rules/ — development process rules

## Legacy Reference
- PGAM Feature Audit: docs/legacy/PGAM_FEATURE_AUDIT.md
- Legacy repo: https://github.com/ios9000/pgam-legacy
- Query-to-file mapping:
  - analiz2.php queries 1–19 → internal/collector/ (server_info, connections, cache, etc.)
  - analiz2.php queries 20–41 → internal/collector/replication_*.go
  - analiz2.php queries 42–47 → internal/collector/progress_*.go
  - analiz2.php queries 48–52 → internal/collector/statements_*.go
  - analiz2.php queries 53–58 → internal/collector/ (wait_events, lock_tree, long_transactions)
  - analiz_db.php queries 1–18 → internal/collector/database.go (future)

## Shared Interfaces

```go
// internal/collector/collector.go

type InstanceContext struct {
    IsRecovery bool
}

type MetricPoint struct {
    InstanceID string
    Metric     string
    Value      float64
    Labels     map[string]string
    Timestamp  time.Time
}

type MetricQuery struct {
    InstanceID string
    Metric     string
    Labels     map[string]string
    Start      time.Time
    End        time.Time
    Limit      int
}

type Collector interface {
    Name() string
    Collect(ctx context.Context, conn *pgx.Conn, ic InstanceContext) ([]MetricPoint, error)
    Interval() time.Duration
}

type MetricStore interface {
    Write(ctx context.Context, points []MetricPoint) error
    Query(ctx context.Context, query MetricQuery) ([]MetricPoint, error)
    Close() error
}

type AlertEvaluator interface {
    Evaluate(ctx context.Context, metric string, value float64, labels map[string]string) error
}
```

```go
// internal/auth/ (added M3)

type UserStore interface {
    GetByUsername(ctx context.Context, username string) (*User, error)
    Create(ctx context.Context, username, passwordHash, role string) (*User, error)
    Count(ctx context.Context) (int64, error)
}

// JWT: HS256, access (24h) + refresh (7d), stateless
// RBAC: admin (full access), viewer (read-only)
// Rate limiting: 10 failed attempts / 15 min window on login
```

## Rules
- Code in English, comments in English
- All SQL must use parameterized queries (pgx named args) — NEVER string concatenation
- Every collector must handle PG version ranges via internal/version gate
- Commits in English: "feat: ...", "fix: ...", "docs: ...", "refactor: ..."
- Do not change architecture without agreement (discuss in Claude.ai first)
- No COPY TO PROGRAM — OS metrics via Go agent only
- Monitoring user: pg_monitor role, never superuser
- Test against PG 14, 15, 16, 17 using testcontainers-go
- Design docs: always write full paths, never use "..." or abbreviations
- Design docs: never use XXXXXXXX or similar placeholders for dates — use real dates or omit the path until the date is known
- Iteration deliverables: prefix files with iteration ID (e.g. M4_01_requirements.md)

## Current Iteration

[UPDATED BY DEVELOPER BEFORE EACH TEAM SESSION]

   M5_03 Requirements — Live Data Integration: Fleet Overview + Server Detail
   See: docs/iterations/M5_03_03032026_live-data/

### What was just completed
M3_01 — Auth & Security. JWT authentication (HS256, access+refresh tokens),
bcrypt password hashing, RBAC (admin/viewer), rate limiting on login,
UserStore (PG-backed), initial admin seeding, auth middleware, 3 new API endpoints.
28 auth unit tests + 7 API auth tests. All prior tests pass (zero regressions).

### What's next
M4_01 — Alert Engine & Notifications. Evaluator with state machine
(OK→WARNING→CRITICAL→OK), hysteresis, cooldown. 14 PGAM threshold rules + 6 new.
Notifiers: Telegram, Slack, Email, Webhook. Dispatcher with retry logic.
API endpoints for alert management. Alert history table.
