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
- Language: Go 1.25.7 (auto-upgraded for pgx v5.8.0)
- PG Driver: jackc/pgx v5
- HTTP: go-chi/chi v5
- Storage: PostgreSQL + TimescaleDB
- Frontend: Svelte + Tailwind CSS (embedded via go:embed)
- ML: gonum.org/v1/gonum (Phase 1)
- Config: koanf (YAML + env vars)
- Logging: log/slog
- Testing: testing + testcontainers-go

## Agent Teams Configuration
This project uses Claude Code Agent Teams (in-process mode on Windows/Git Bash).

### ⚠️ Windows Bash Bug
Claude Code's Bash() tool is broken on Windows (EINVAL temp path, all versions tested).
**Hybrid workflow:** agents create files, developer runs bash commands manually.
See `docs/save-points/LATEST.md` for full details.

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
- internal/ — all business logic (collector, storage, api, auth, alert, ml, rca, version)
- web/ — embedded frontend (future: M5)
- migrations/ — SQL migrations for PGPulse metadata DB
- deploy/ — Docker, Helm, systemd
- docs/ — documentation, iterations, legacy reference, save points
- docs/save-points/ — project snapshots for continuity and disaster recovery
- .claude/rules/ — development process rules

## Legacy Reference
- PGAM Feature Audit: docs/legacy/PGAM_FEATURE_AUDIT.md
- Legacy repo: https://github.com/ios9000/pgam-legacy
- Query-to-file mapping:
  - analiz2.php queries 1–19 → internal/collector/ (server_info, connections, cache, etc.)
  - analiz2.php queries 20–41 → internal/collector/replication.go
  - analiz2.php queries 42–47 → internal/collector/progress.go
  - analiz2.php queries 48–52 → internal/collector/statements.go
  - analiz2.php queries 53–58 → internal/collector/locks.go
  - analiz_db.php queries 1–18 → internal/collector/database.go

## Shared Interfaces

```go
// internal/collector/collector.go

type MetricPoint struct {
    InstanceID string
    Metric     string
    Value      float64
    Labels     map[string]string
    Timestamp  time.Time
}

type Collector interface {
    Name() string
    Collect(ctx context.Context, conn *pgx.Conn) ([]MetricPoint, error)
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

## Rules
- Code in English, comments in English
- All SQL must use parameterized queries (pgx named args) — NEVER string concatenation
- Every collector must handle PG version ranges via internal/version gate
- Commits in English: "feat: ...", "fix: ...", "docs: ...", "refactor: ..."
- Do not change architecture without agreement (discuss in Claude.ai first)
- No COPY TO PROGRAM — OS metrics via Go agent only
- Monitoring user: pg_monitor role, never superuser
- Test against PG 14, 15, 16, 17 using testcontainers-go
- Agents CANNOT run bash on Windows — create files only, developer runs bash

## Current Iteration
[UPDATED BY DEVELOPER BEFORE EACH SESSION]
M1_02a — Instance Metrics Collector
See: docs/iterations/M1_02a_02252026_interface-refactor/
Check docs/save-points/LATEST.md for current state.
