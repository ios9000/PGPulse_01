# PGPulse — Save Point

**Save Point:** M0 — Project Setup  
**Date:** 2026-02-25  
**Commit:** 182173d  
**Developer:** Evlampiy (ios9000)  
**AI Tool:** Claude.ai (Opus 4.6) + Claude Code 2.1.53 (Agent Teams)

---

## 1. PROJECT IDENTITY

**Name:** PGPulse — PostgreSQL Health & Activity Monitor  
**Repo:** https://github.com/ios9000/PGPulse_01  
**Legacy repo:** https://github.com/ios9000/pgam-legacy  
**Go module:** github.com/ios9000/PGPulse_01  

### What PGPulse Does
PGPulse is a real-time PostgreSQL monitoring tool that collects metrics from PG 14–18 instances (connections, cache hit ratio, replication lag, locks, wait events, pg_stat_statements, vacuum progress, bloat), stores them in TimescaleDB, provides alerting via Telegram/Slack/Email/Webhook, and will include ML-based anomaly detection. It's designed as a single Go binary with an embedded web UI, targeting PostgreSQL DBAs who need a lightweight, PG-specialized alternative to heavyweight platforms like PMM or SolarWinds.

### Origin Story
Rewrite of PGAM — a legacy PHP PostgreSQL Activity Monitor used internally at VTB Bank. PGAM had 76 SQL queries across 2 PHP files (analiz2.php + analiz_db.php), zero authentication, SQL injection vulnerabilities via raw GET params, and relied on COPY TO PROGRAM for OS metrics (requiring superuser). PGPulse is a clean-room rewrite in Go that preserves the SQL monitoring knowledge while fixing every architectural and security flaw.

---

## 2. ARCHITECTURE SNAPSHOT

### Tech Stack
| Component | Choice | Version | Why |
|---|---|---|---|
| Language | Go | 1.25.7 | Performance, single binary, goroutines for collectors |
| PG Driver | jackc/pgx v5 | 5.8.0 | Best Go PG driver, parameterized queries |
| HTTP Router | go-chi/chi v5 | — | Lightweight, middleware-friendly |
| Storage | PostgreSQL + TimescaleDB | — | PG-native time-series |
| Frontend | Svelte + Tailwind | — | Embedded via go:embed |
| Config | koanf | — | YAML + env vars |
| Logging | log/slog | stdlib | Structured logging |
| Testing | testcontainers-go | — | Real PG instances in tests |
| ML (Phase 1) | gonum | — | Pure Go statistics |
| CI | GitHub Actions | — | Lint + test + build |

### Key Design Decisions

| # | Decision | Rationale | Date |
|---|----------|-----------|------|
| D1 | Single binary deployment | Simplicity, go:embed frontend | 2026-02-25 |
| D2 | pgx v5 (not database/sql) | Named args, COPY protocol, pgx-specific features | 2026-02-25 |
| D3 | Version-adaptive SQL via Gate pattern | Support PG 14-18 without code branches | 2026-02-25 |
| D4 | No COPY TO PROGRAM ever | PGAM's worst security flaw — eliminated | 2026-02-25 |
| D5 | pg_monitor role only | Least privilege, never superuser | 2026-02-25 |
| D6 | One Collector per module | Testable, enable/disable, independent intervals | 2026-02-25 |
| D7 | Hybrid agent workflow | Claude Code bash broken on Windows | 2026-02-25 |
| D8 | Agent Teams (4 agents) | Right-sized for 1-dev project (not 7) | 2026-02-25 |
| D9 | Three-tier persistence | Save Points + Handoffs + Session-logs | 2026-02-25 |

---

## 3. CODEBASE STATE

### File Tree
```
.claude/CLAUDE.md
.claude/rules/architecture.md
.claude/rules/code-style.md
.claude/rules/postgresql.md
.claude/rules/security.md
.claude/rules/chat-transition.md
.claude/rules/save-point.md
.claude/settings.local.json
.github/workflows/ci.yml
.gitignore
.golangci.yml
Makefile
README.md
cmd/pgpulse-agent/main.go
cmd/pgpulse-server/main.go
configs/pgpulse.example.yml
deploy/docker/Dockerfile
deploy/docker/docker-compose.yml
deploy/helm/.gitkeep
deploy/systemd/.gitkeep
docs/CHANGELOG.md
docs/PGPulse_Development_Strategy_v2.md
docs/RESTORE_CONTEXT.md
docs/roadmap.md
docs/iterations/HANDOFF_M0_to_M1.md
docs/iterations/M0_01_02262026_project-setup/design.md
docs/iterations/M0_01_02262026_project-setup/requirements.md
docs/iterations/M0_01_02262026_project-setup/session-log.md
docs/iterations/M0_01_02262026_project-setup/team-prompt.md
docs/save-points/SAVEPOINT_M0_20260225.md
docs/save-points/LATEST.md
go.mod
go.sum
internal/alert/.gitkeep
internal/alert/notifier/.gitkeep
internal/api/.gitkeep
internal/auth/.gitkeep
internal/collector/collector.go
internal/config/.gitkeep
internal/ml/.gitkeep
internal/rca/.gitkeep
internal/storage/.gitkeep
internal/version/gate.go
internal/version/version.go
migrations/.gitkeep
web/.gitkeep
```

### Key Interfaces

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

```go
// internal/version/version.go
type PGVersion struct {
    Major int    // e.g., 16
    Minor int    // e.g., 4
    Num   int    // e.g., 160004
    Full  string // e.g., "16.4 (Ubuntu 16.4-1.pgdg22.04+1)"
}

func Detect(ctx context.Context, conn *pgx.Conn) (PGVersion, error)
func (v PGVersion) AtLeast(major, minor int) bool
```

```go
// internal/version/gate.go
type Gate struct {
    Name     string
    Variants []SQLVariant
}

func (g Gate) Select(v PGVersion) (string, bool)
```

---

## 4. LEGACY KNOWLEDGE (PGAM)

### Query Porting Status

| Source | Queries | Target | Status |
|--------|---------|--------|--------|
| analiz2.php #1–19 | Instance metrics | collector/ (7 files) | 🔲 Not started |
| analiz2.php #20–41 | Replication | collector/replication.go | 🔲 Not started |
| analiz2.php #42–47 | Progress | collector/progress.go | 🔲 Not started |
| analiz2.php #48–52 | Statements | collector/statements.go | 🔲 Not started |
| analiz2.php #53–58 | Locks | collector/locks.go | 🔲 Not started |
| analiz_db.php #1–18 | Per-DB analysis | collector/database.go | 🔲 Not started |
| **Total: 76** | | | **0/76 ported** |

### PGAM Bugs Already Identified
1. Q11: Connection count includes own connection → fix: WHERE pid != pg_backend_pid()
2. Q14: Cache hit ratio division by zero → fix: NULLIF guard
3. Q4-Q8: OS metrics via COPY TO PROGRAM (superuser) → fix: Go agent via procfs

### Version Gates Required
1. Q10: pg_is_in_backup() removed in PG 15 → return false for PG 15+

---

## 5. MILESTONE STATUS

| Milestone | Name | Status | Date |
|---|---|---|---|
| M0 | Project Setup | ✅ Done | 2026-02-25 |
| M1 | Core Collector | 🔲 Not started | — |
| M2 | Storage & API | 🔲 Not started | — |
| M3 | Auth & Security | 🔲 Not started | — |
| M4 | Alerting | 🔲 Not started | — |
| M5 | Web UI (MVP) | 🔲 Not started | — |
| M6 | Agent Mode | 🔲 Not started | — |
| M7 | P1 Features | 🔲 Not started | — |
| M8 | ML Phase 1 | 🔲 Not started | — |
| M9 | Reports & Export | 🔲 Not started | — |
| M10 | Polish & Release | 🔲 Not started | — |

### What Was Just Completed (M0)
Project scaffold: go.mod, directory structure, Dockerfile, docker-compose, CI pipeline, Makefile, shared interfaces (Collector, MetricStore, AlertEvaluator), PG version detection, sample config. Two commits pushed to GitHub.

### What's Next (M1_01)
Instance Metrics Collector — port PGAM queries 1–19 into Go collectors. 12 real queries (Q1 done in M0, Q4-Q8 skipped to M6). Seven collector files: server_info.go, connections.go, cache.go, transactions.go, database_sizes.go, settings.go, extensions.go. Plus registry.go for orchestration.

---

## 6. DEVELOPMENT ENVIRONMENT

| Component | Value |
|---|---|
| OS | Windows 10 |
| Shell | Git Bash (MSYS2) + PowerShell |
| Go | 1.25.7 |
| Node.js | 22.14.0 |
| Claude Code | 2.1.53 |
| Git | 2.52.0 |
| golangci-lint | v1.64.8 |
| Agent Teams | Enabled (in-process mode) |
| Display mode | In-process (no tmux, WSL unavailable) |

### Known Environment Issues

| Issue | Status | Workaround |
|---|---|---|
| Claude Code bash EINVAL on Windows | Unresolved | Agents create files, dev runs bash manually |
| LF/CRLF warnings | Needs .gitattributes | Add `* text=auto eol=lf` |
| WSL2 unavailable | BIOS virtualization disabled | Using native Git Bash |
| Go auto-upgraded to 1.25.7 | Accepted | pgx v5.8.0 requires Go ≥ 1.24 |

---

## 7. KEY LEARNINGS & DECISIONS LOG

### Issues & Resolutions

| Date | Issue | Resolution |
|---|---|---|
| 2026-02-25 | Claude Code bash EINVAL on Windows | Not resolved — hybrid workflow |
| 2026-02-25 | GitHub PAT missing workflow scope | Added workflow scope to PAT |
| 2026-02-25 | WSL2 install failed (BIOS virtualization) | Using native Git Bash |
| 2026-02-25 | New chat lost all context | Created handoff + save point system |
| 2026-02-25 | Strategy doc used as history log | Separated: strategy=rules, handoff=transition, session-log=history |
| 2026-02-25 | Agent Teams proposed with 7 agents | Reduced to 4 (right-sized for 1-dev) |
| 2026-02-25 | TMPDIR fix attempted for bash bug | Failed — Claude Code uses internal path |

### Competitive Intelligence Summary
- pgwatch v3: Go-based, SQL metrics, 4 storage backends — closest architectural cousin
- Percona PMM: heavyweight, QAN is gold standard for query analytics
- pganalyze: SaaS $149/mo, HypoPG index advisor — bar for query-level analytics
- Tantor Platform: most feature-rich Russian solution, Kubernetes deployment
- SolarWinds DPA: ML anomaly detection with seasonal baselines — our ML target

---

## 8. HOW TO RESTORE THIS SAVE POINT

### Option A: Continue in Same Claude.ai Project
1. Open new chat in the PGPulse project
2. Upload this save point file
3. Say: "Restoring from save point M0. Continue with M1_01."

### Option B: New Claude.ai Project from Scratch
1. Create new Project named "PGPulse"
2. Upload to Project Knowledge: this file + PGAM_FEATURE_AUDIT.md + Strategy doc
3. Open new chat, upload this save point
4. Say: "Restoring PGPulse from save point. All context is in this file."

### Option C: Different AI Tool / New Developer
1. Clone: `git clone https://github.com/ios9000/PGPulse_01.git`
2. Read this file for complete context
3. Read `.claude/CLAUDE.md` for interfaces and rules
4. Continue from "What's Next" section above
