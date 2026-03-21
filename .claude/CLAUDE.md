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
- Language: Go 1.25.0
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
- Desktop: Wails v3 alpha.74 (build-tag gated `//go:build desktop`, M12)

## Agent Teams Configuration
This project uses Claude Code Agent Teams (in-process mode on Windows/Git Bash).

### Build & Test
Claude Code v2.1.63 — bash works on Windows. EINVAL temp path bug is FIXED.
Agents run go build, go test, golangci-lint, git commit directly.
No hybrid workflow needed.

All team prompts should include validation steps:
  cd web && npm run build && npm run lint && npm run typecheck && cd ..
  go mod tidy && go build ./... && go vet ./...
  go test -race ./cmd/... ./internal/... && golangci-lint run
Note: NEVER use `go test ./...` — it scans web/node_modules/ and fails.
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
| Collector | internal/collector/*, internal/version/*, internal/agent/*, internal/cluster/*, cmd/pgpulse-agent/ | internal/api/*, internal/auth/*, internal/storage/* |
| API & Security | internal/api/*, internal/auth/*, internal/alert/*, internal/storage/*, migrations/*, configs/* | internal/collector/*, internal/version/*, internal/agent/*, internal/cluster/* |
| QA & Review | *_test.go, .golangci.yml, testdata/, .github/workflows/ | Production code (only tests and CI) |

### Merge Rules
- All agents work in separate git worktrees
- Team Lead merges only after QA Agent confirms tests pass
- Merge order: version/interfaces → collector → storage → API → auth → QA tests

## Project Structure
- cmd/ — binary entrypoints (server + agent + icongen tool)
- internal/ — all business logic (collector, storage, api, auth, alert, config, orchestrator, version, desktop)
- internal/desktop/ — Wails v3 desktop shell (build-tag gated, M12)
- web/ — embedded frontend (React/TS/Tailwind/ECharts, complete as of M5, go:embed)
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
  - analiz_db.php queries 1–18 → internal/collector/database.go (M7)

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

// M7 — per-database analysis interfaces (append only — do NOT modify above)

// Queryer is the minimal SQL execution interface.
// Both *pgx.Conn and *pgxpool.Pool satisfy it natively.
// Use Queryer in DBCollectors to enable mock injection in unit tests.
type Queryer interface {
    Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
    QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// DBCollector collects metrics for a single database.
// Dispatched once per discovered database per cycle by DBRunner.
// Parallel interface to Collector — DO NOT merge them.
type DBCollector interface {
    Name() string
    Interval() time.Duration
    CollectDB(ctx context.Context, q Queryer, dbName string, ic InstanceContext) ([]MetricPoint, error)
}
```

```go
// internal/auth/ (complete as of M5_07)

type UserStore interface {
    GetByUsername(ctx context.Context, username string) (*User, error)
    GetByID(ctx context.Context, id int64) (*User, error)
    Create(ctx context.Context, username, passwordHash, role string) (*User, error)
    Count(ctx context.Context) (int64, error)
    CountActiveByRole(ctx context.Context, role string) (int64, error)
    List(ctx context.Context) ([]*User, error)
    Update(ctx context.Context, id int64, fields UpdateFields) error
    UpdatePassword(ctx context.Context, id int64, passwordHash string) error
    UpdateLastLogin(ctx context.Context, id int64) error
    Delete(ctx context.Context, id int64) error
}

// JWT: HS256, access (24h) + refresh (7d), stateless
// RBAC — 4 roles:
//   super_admin: user_management + instance_management + alert_management + view_all + self_management
//   roles_admin: user_management + view_all + self_management
//   dba:         instance_management + alert_management + view_all + self_management
//   app_admin:   alert_management + view_all + self_management
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
- All procfs/sysfs code (internal/agent/) MUST use `//go:build linux` with `//go:build !linux` stubs — dev machine is Windows, /proc does not exist

## Current Iteration
M14_02 — RCA UI (incidents page, timeline visualization, alert integration)
See: docs/iterations/M14_02_03212026_rca-ui

### What Was Just Completed
M12_01 — Core Desktop (Wails v3):
- Wails v3 alpha.74 dependency (build-tag gated: `//go:build desktop`)
- `internal/desktop/` package: app.go, tray.go, icon.go, stub.go
- `--mode=desktop|server` flag via `cmd/pgpulse-server/desktop.go` + `desktop_stub.go`
- Chi router as `AssetOptions.Handler` — zero frontend changes
- System tray with show/hide toggle, status menu, severity-colored icons
- Window close → hide to tray; 1440x900, min 1024x700
- Standard build unchanged — no Wails symbols in binary
- 16 files changed, 565 lines added
Build: clean. See: docs/iterations/M12_01_03172026_core-desktop/

### What's Next
M12_02 — Connection dialog (C-01→C-05), OS notifications (N-01→N-06), NSIS installer (I-01→I-07).
See: docs/iterations/M12_01_03172026_core-desktop/HANDOFF_M12_01_to_M12_02.md
