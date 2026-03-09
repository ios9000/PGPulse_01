# PGPulse — Save Point

**Save Point:** M8 — P1 Features + ML Phase 1
**Date:** 2026-03-09
**Developer:** ios9000
**AI Tool:** Claude.ai (Opus 4.6) + Claude Code 2.1.71 (Agent Teams, bash works on Windows)

---

## 1. PROJECT IDENTITY

**Name:** PGPulse — PostgreSQL Health & Activity Monitor
**Repo:** https://github.com/ios9000/PGPulse_01
**Legacy repo:** https://github.com/ios9000/pgam-legacy
**Go module:** github.com/ios9000/PGPulse_01
**License:** TBD

### What PGPulse Does

PGPulse is a real-time PostgreSQL monitoring platform that replaces a legacy PHP tool
called PGAM. It monitors PostgreSQL 14–18 instances for connections, cache hit ratios,
replication health, lock contention, wait events, pg_stat_statements query performance,
vacuum progress, table bloat, and per-database object analysis. It stores time-series
metrics, evaluates 22 alert rules (including 3 ML-forecast-based rules), and delivers
notifications via email. An embedded React SPA provides a dark-first dashboard with
interactive charts, session management (cancel/terminate), on-demand EXPLAIN plans,
settings comparison, and forecast confidence band overlays.

The ML subsystem uses STL (Seasonal-Trend-Loess) decomposition to fit metric baselines,
Z-score anomaly detection, and Holt-Winters forecasting with configurable horizons and
confidence intervals. Forecast alerts use a sustained-crossing model requiring N
consecutive forecast points to breach a threshold before firing.

PGPulse deploys as a single Go binary (frontend embedded via go:embed) plus a YAML config
file. An optional OS agent binary collects CPU/RAM/disk/iostat metrics on Linux via procfs.
Cluster monitoring supports Patroni and ETCD.

### Origin Story

Rewrite of PGAM — a legacy PHP PostgreSQL Activity Monitor used internally at VTB Bank.
PGAM had 76 SQL queries across 2 PHP files (analiz2.php + analiz_db.php), zero auth,
SQL injection vulnerabilities, and relied on COPY TO PROGRAM for OS metrics (superuser
required). PGPulse is a clean-room rewrite in Go that preserves the SQL monitoring
knowledge while fixing every architectural and security flaw.

---

## 2. ARCHITECTURE SNAPSHOT

### Tech Stack
| Component | Choice | Version | Why |
|---|---|---|---|
| Language | Go | 1.24.0 | Performance, single binary, goroutines for collectors |
| PG Driver | jackc/pgx v5 | 5.8.0 | Best Go PG driver, parameterized queries, COPY |
| HTTP Router | go-chi/chi v5 | 5.2.5 | Lightweight, middleware-friendly |
| JWT | golang-jwt/jwt v5 | 5.2.2 | Authentication tokens |
| Storage | PostgreSQL + TimescaleDB-ready | — | Time-series metrics, hypertable-compatible schema |
| Frontend | React 18 + TypeScript + Tailwind CSS 4 + Apache ECharts | — | Embedded via go:embed |
| State Mgmt | Zustand 5.0 + TanStack Query 5 | — | Client state + server cache |
| Config | koanf v2 | 2.3.2 | YAML + env vars |
| Logging | log/slog | stdlib | Structured logging |
| Testing | testcontainers-go | 0.40.0 | Real PG instances in tests |
| ML | internal/ml (custom) | — | STL, Holt-Winters, Z-score |
| Linter | golangci-lint | v2.10.1 | v2 config format |

### Architecture Diagram
```
┌─────────────────────────────────────────────────────────┐
│              PGPulse Server (Go binary)                  │
│                                                          │
│  ┌───────────┐  ┌────────┐  ┌───────────┐  ┌────────┐  │
│  │ Collectors │→ │Storage │← │ REST API  │← │  Auth  │  │
│  │ (pgxpool)  │  │(PGStore)│  │(chi+JWT) │  │ (RBAC) │  │
│  └─────┬─────┘  └────────┘  └─────┬─────┘  └────────┘  │
│        │                          │                      │
│  ┌─────▼─────┐  ┌────────────┐   │                      │
│  │  Version   │  │ ML Detector│   │                      │
│  │   Gate     │  │ (STL+HW)  │   │                      │
│  └───────────┘  └──────┬─────┘   │                      │
│                         │         │                      │
│  ┌─────────────┐  ┌────▼─────┐   │                      │
│  │   Alert     │← │ Forecast │   │                      │
│  │  Evaluator  │  │ Provider │   │                      │
│  └──────┬──────┘  └──────────┘   │                      │
│         │                         │                      │
│  ┌──────▼──────┐  ┌─────────────────────────────────┐   │
│  │ Dispatcher  │  │  Web UI (go:embed)               │   │
│  │ (Email)     │  │  React + Tailwind + ECharts      │   │
│  └─────────────┘  │  Forecast bands, plan viewer,    │   │
│                    │  session kill, settings diff     │   │
│                    └─────────────────────────────────┘   │
└─────────────────────────────────────────────────────────┘
         │
    ┌────▼──────┐
    │  PGPulse  │  (optional, separate binary)
    │   Agent   │  OS metrics via procfs (Linux)
    └───────────┘
```

### Key Design Decisions

| # | Decision | Rationale | Date |
|---|----------|-----------|------|
| D1 | Single binary deployment | Simplicity, go:embed frontend | 2026-02-25 |
| D2 | pgx v5 (not database/sql) | Named args, COPY protocol, pgx-specific features | 2026-02-25 |
| D3 | Version-adaptive SQL via Gate pattern | Support PG 14-18 without code branches | 2026-02-25 |
| D4 | No COPY TO PROGRAM ever | PGAM's worst security flaw — eliminated entirely | 2026-02-25 |
| D5 | pg_monitor role only | Least privilege, never superuser | 2026-02-25 |
| D6 | One Collector per module | Testable, enable/disable, independent intervals | 2026-02-25 |
| D7 | Agents run build/test/lint directly | Claude Code v2.1.63 fixed EINVAL bash bug on Windows | 2026-03-01 |
| D8 | pgxpool instead of pgx.Conn | Eliminates "conn busy" errors under concurrent collectors | 2026-03-04 |
| D9 | InstanceContext SSoT | Orchestrator queries pg_is_in_recovery() once, passes to all collectors | 2026-02-25 |
| D10 | YAML seeds DB, DB is source of truth | INSERT ON CONFLICT DO NOTHING; source column distinguishes yaml/manual | 2026-03-04 |
| D11 | ForecastProvider interface | alert package never imports ml — adapter in ml/detector_alert.go | 2026-03-09 |
| D12 | mlerrors package for shared sentinels | Breaks circular import between ml and alert packages | 2026-03-09 |
| D13 | Sustained crossing for forecast alerts | N consecutive points required before firing — no first-crossing mode | 2026-03-09 |
| D14 | 4-role RBAC | super_admin, roles_admin, dba, app_admin × 5 permissions | 2026-03-03 |
| D15 | Two-database design | storage.dsn for PGPulse metadata; instances[].dsn per monitored PG | 2026-02-26 |
| D16 | TimescaleDB treated as optional | Migrations fall back to regular tables when extension absent | 2026-03-04 |

---

## 3. CODEBASE STATE

### Key Interfaces

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
// internal/alert/forecast.go

type ForecastPoint struct {
    Offset int
    Value  float64
    Lower  float64
    Upper  float64
}

type ForecastProvider interface {
    ForecastForAlert(ctx context.Context, instanceID, metricKey string, horizon int) ([]ForecastPoint, error)
}
```

```go
// internal/mlerrors/errors.go
var ErrNotBootstrapped = errors.New("ml detector not yet bootstrapped")
var ErrNoBaseline      = errors.New("no fitted baseline for this metric")
```

```go
// internal/ml/detector.go (key types)

type InstanceLister interface {
    ListInstances(ctx context.Context) ([]string, error)
}

type PersistenceStore interface {
    Save(ctx context.Context, instanceID, metricKey string, snapshot BaselineSnapshot) error
    Load(ctx context.Context, instanceID, metricKey string) (*BaselineSnapshot, error)
}
// NewDetector(cfg DetectorConfig, store MetricStore, lister InstanceLister, eval AlertEvaluator, persist PersistenceStore)
// persist may be nil (disables persistence)
```

```go
// internal/version/version.go
type PGVersion struct {
    Major int
    Minor int
    Num   int
    Full  string
}

func Detect(ctx context.Context, conn *pgx.Conn) (PGVersion, error)
func (v PGVersion) AtLeast(major, minor int) bool
```

---

## 4. LEGACY KNOWLEDGE (PGAM)

### Query Porting Status

| Source | Queries | Target | Status |
|--------|---------|--------|--------|
| analiz2.php #1–19 | Instance metrics | collector/instance group | ✅ 13 ported, 6 → agent (M6) |
| analiz2.php #20–41 | Replication | collector/replication.go | ✅ 4 ported, rest deferred |
| analiz2.php #42–47 | Progress | collector/progress.go | ✅ All 6 ported |
| analiz2.php #48–52 | Statements | collector/statements.go | ✅ 4 ported, Q52 deferred |
| analiz2.php #53–58 | Locks | collector/locks.go | ✅ 5 ported, Q58 deferred |
| analiz_db.php #1–18 | Per-DB analysis | collector/database.go | ✅ 17 ported (Q1 dup skip) |
| New (not in PGAM) | Various | Multiple files | ✅ 6 (checkpoint, pg_stat_io, OS, cluster) |
| **Total** | **76** | | **~69/76 ported** |

### Version Gates Implemented

1. `pg_is_in_backup()` → removed in PG 15, return false
2. `bgwriter/checkpointer` table split in PG 17
3. `pg_stat_io` → PG 16+ only
4. `pg_stat_statements` → `total_time` (PG ≤12) vs `total_exec_time` (PG ≥13)
5. `pg_stat_statements_info` → PG ≥14 only
6. Replication slot columns differ PG 15+, PG 16+

---

## 5. MILESTONE STATUS

### Roadmap

| Milestone | Name | Status | Completion Date |
|---|---|---|---|
| M0 | Project Setup | ✅ Done | 2026-02-25 |
| M1 | Core Collector | ✅ Done | 2026-02-26 |
| M2 | Storage & API | ✅ Done | 2026-02-27 |
| M3 | Auth & Security | ✅ Done | 2026-03-01 |
| M4 | Alerting | ✅ Done | 2026-03-01 |
| M5 | Web UI (MVP) | ✅ Done | 2026-03-04 |
| M6 | Agent Mode + Cluster | ✅ Done | 2026-03-05 |
| M7 | Per-Database Analysis | ✅ Done | 2026-03-08 |
| M8 | P1 Features + ML Phase 1 | ✅ Done | 2026-03-09 |
| M9 | Reports & Export | 🔲 Not Started | — |
| M10 | Polish & Release | 🔲 Not Started | — |

### What Was Just Completed (M8 — 6 sub-iterations)

M8 combined two originally separate milestones into one:

**M8_01 — P1 Feature Backends:** Session kill API (pg_cancel_backend, pg_terminate_backend
with audit logging via migration 007), on-demand EXPLAIN API (one-shot pgx.Conn, 30s timeout,
SubstituteDatabase helper), cross-instance settings diff API (concurrent errgroup fetch, noise
filter). 4 new API endpoints, 6 new TypeScript interfaces. Note: 3 handler files written but
routes not wired — deleted in M8_02, reintroduced properly in M8_03.

**M8_02 — Auto-Capture Plans + Temporal Settings Diff + ML Anomaly Detection:** Three features
in one iteration. Auto-capture EXPLAIN plans with 4 trigger modes (duration threshold, scheduled
top-N, manual API, plan hash diff) + dedup cache + retention worker (migrations 008, 009).
Temporal pg_settings snapshots with Go-side diff (changed/added/removed/pending_restart).
STL-based ML anomaly detection: EWMA trend + period-folded seasonal mean + Z-score/IQR residual
scoring via gonum v0.17.0. InstanceLister interface (separate from MetricStore), MetricAlertAdapter
to bridge batch and single-metric alert interfaces. 29 new tests. 5 design-doc issues caught and
fixed before agent spawn.

**M8_03 — Instance Lister Fix + Session Kill API + ML Persistence:** Replaced
configInstanceLister with DBInstanceLister (queries instances table, picks up API-added instances).
ML baseline persistence via JSONB upsert (migration 010) with two-phase bootstrap (snapshot load →
TimescaleDB replay fallback). Session kill API reintroduced with proper route wiring in
PermInstanceManagement group. Fixed accidentally committed agent worktree, added .claude/worktrees/
to .gitignore.

**M8_04 — Forecast Horizon:** STL-based N-step-ahead forecasting with linear trend extrapolation
(slope from last 2 EWMA values) + seasonal repeat + ±z·σ confidence bounds. Forecast API endpoint
(GET /instances/{id}/metrics/{metric}/forecast) with horizon cap, 404/503 error mapping.
ForecastZ/ForecastHorizon config fields. RuleTypeForecastThreshold + UseLowerBound on Rule struct.
~7 minute agent execution time.

**M8_05 — Forecast Alerts + Chart:** ForecastProvider interface (alert never imports ml — adapter
in ml/detector_alert.go). mlerrors package for shared sentinel errors (breaks circular import).
Sustained-crossing alert logic (N consecutive forecast points must breach threshold, configurable
per rule, global default 3, migration 011). ECharts forecast overlay: custom polygon confidence
band + dashed centre line + "Now" markLine, wired to connections_active chart. 13 new tests.

**M8_06 — UI Catch-Up:** Session kill buttons (ConfirmModal + SessionActions with role gating),
per-instance settings diff accordion with CSV export, query plan viewer with recursive tree
rendering and cost highlighting (amber >100ms, red >10x row error), forecast overlay extended to
4 charts via useForecastChart helper hook, toast notification system (Toast.tsx + toastStore.ts).
2-specialist team, 18 min execution. Frontend-only, zero backend changes.

### What's Next

**M9 — Reports & Export.** Scope TBD. Candidates include:
- Scheduled PDF/HTML reports (daily/weekly instance health summaries)
- CSV export for all data views
- pg_settings full export (already partially done in M8_01)
- Metric data export API
- Custom report builder

### Deferred Items
- Auto-capture query plans + plan history (originally M8_02 scope, replaced by ML)
- Temporal settings diff (originally M8_02 scope, replaced by ML)
- Logical replication monitoring (requires per-database connections)
- Session kill application_name enrichment (LongTransaction model lacks the field)

---

## 6. DEVELOPMENT ENVIRONMENT

### Developer Workstation
| Component | Value |
|---|---|
| OS | Windows 10 |
| Shell | Git Bash (MSYS2) |
| Go | 1.24.0 |
| Node.js | 22.14.0 |
| PostgreSQL | 16 (local, native installer, sslmode=disable) |
| golangci-lint | v2.10.1 |
| Claude Code | 2.1.71 |
| Git | 2.52.0 |
| Docker Desktop | Not installed (BIOS virtualization disabled) |

### Development Method
- **Two-contour model:** Claude.ai (Brain) + Claude Code (Hands)
- **Claude Code v2.1.71:** Bash works on Windows. Agents run go build/test/lint/commit directly.
- **Right-sized teams:** 2 agents for frontend-only work, 3–4 for full-stack iterations
- **One chat per iteration** in Claude.ai
- **Project Knowledge** contains: strategy, PGAM audit, architecture doc, chat transition, save point docs
- **Iteration Handoff** documents bridge between chats

### Build Verification Sequence
```bash
cd web && npm run build && npm run typecheck && npm run lint
cd .. && go build ./cmd/pgpulse-server && go test ./cmd/... ./internal/...
golangci-lint run
```

### Known Environment Issues

| Issue | Status | Notes |
|---|---|---|
| ~~Claude Code bash EINVAL on Windows~~ | **Fixed in v2.1.63** | No workaround needed |
| LF/CRLF warnings | Managed | `.gitattributes` with `* text=auto eol=lf` |
| Docker Desktop unavailable | Permanent | BIOS virtualization disabled; testcontainers CI-only |
| TimescaleDB extension absent locally | Managed | Metrics table created as standard PG table; works without extension |
| Pre-existing Administration.tsx lint error | Known | Not caused by PGPulse code; do not fix unless in scope |

---

## 7. KEY LEARNINGS & DECISIONS LOG

### Architecture Decisions

| Date | Decision | Alternatives Considered | Why This Choice |
|---|---|---|---|
| 2026-02-25 | Go over Rust | Rust steeper learning curve | Faster dev, goroutines natural for collectors |
| 2026-02-25 | pgx over database/sql | database/sql lacks PG-specific features | Named args, COPY, notifications |
| 2026-02-25 | TimescaleDB over InfluxDB | InfluxDB requires separate service | PG-native, one less dependency |
| 2026-03-01 | Agents run bash directly | Hybrid mode (files only) | EINVAL bug fixed in v2.1.63 |
| 2026-03-03 | 4-role RBAC over 2-role | 2 roles too coarse | 5 permissions × 4 roles = right-grained |
| 2026-03-04 | pgxpool over single pgx.Conn | Single conn caused "conn busy" | Pool eliminates contention |
| 2026-03-04 | YAML seed + DB truth | Config-only or DB-only | Seed gives defaults, DB allows runtime changes |
| 2026-03-09 | ML in internal/ml | External ML service | Keeps single-binary deployment; pure Go |
| 2026-03-09 | Simplified STL (EWMA + folded mean) | Full Loess via dedicated library | Valid residuals for Z-score/IQR; honest about limits |
| 2026-03-09 | InstanceLister as separate interface | Add ListInstances to MetricStore | Different concern, different lifecycle |
| 2026-03-09 | Go-side diff for settings | SQL JSONB diff | Testable without DB; extensible with custom filtering |
| 2026-03-09 | Plan dedup by plan hash | Store all plans | Identical shapes stored once; regressions always produce new row |
| 2026-03-09 | MetricAlertAdapter wraps batch interface | Expand batch interface | Preserves existing contract; 1-element slice overhead negligible |
| 2026-03-09 | DBInstanceLister over config-based | configInstanceLister | Picks up API-added instances; single SELECT overhead negligible |
| 2026-03-09 | Persist baselines after every Evaluate | Periodic save | JSONB is small; continuity after restart matters more |
| 2026-03-09 | Forecast via trend + seasonal repeat | Holt-Winters library | Consistent with existing STL decomposition; no new dependency |
| 2026-03-09 | ForecastProvider interface | Direct ml import from alert | Prevents circular dependency |
| 2026-03-09 | Sustained crossing alerts | First-crossing | Reduces false positives on noisy forecasts |
| 2026-03-09 | 2-agent team for UI-only work | Full 4-agent team | Saves tokens, avoids idle agents |

### Issues & Resolutions

| Date | Issue | Resolution |
|---|---|---|
| 2026-02-25 | TMPDIR path mangling in Claude Code | Fixed in v2.1.63 |
| 2026-02-25 | GitHub PAT missing workflow scope | Added workflow scope |
| 2026-02-25 | WSL2 install failed (BIOS) | Abandoned WSL, using native Git Bash |
| 2026-02-25 | New chat lost context | Created handoff document system |
| 2026-03-04 | "conn busy" errors | Replaced pgx.Conn with pgxpool.Pool |
| 2026-03-04 | nil panic when alerting disabled | Created NoOpEvaluator/NoOpDispatcher |
| 2026-03-04 | TimescaleDB extension absent | Create metrics table as standard PG table |
| 2026-03-09 | Migration 007 already taken by session audit log | Renumbered plan_capture to 008, settings to 009 |
| 2026-03-09 | MetricStore lacks ListInstances for ML Bootstrap | Created separate InstanceLister interface |
| 2026-03-09 | gonum v0.16.0 in go.mod, baseline needs v0.17.0 | `go get gonum.org/v1/gonum@latest` |
| 2026-03-09 | InstanceContext lacks InstanceID field | Collectors take instanceID as explicit param |
| 2026-03-09 | 12 unused functions from M8_01 handlers | Deleted files; reintroduced with proper routes in M8_03 |
| 2026-03-09 | AlertEvaluator interface mismatch (batch vs single) | Created MetricAlertAdapter in internal/alert/adapter.go |
| 2026-03-09 | configInstanceLister ignores API-added instances | Replaced with DBInstanceLister querying instances table |
| 2026-03-09 | Agent worktree accidentally committed to git | git rm --cached, added .claude/worktrees/ to .gitignore |
| 2026-03-09 | ml ↔ alert circular import | Created internal/mlerrors package for shared sentinels |

---

## 8. HOW TO RESTORE THIS SAVE POINT

### Option A: Continue in Same Claude.ai Project
1. Open new chat in the PGPulse project
2. Upload this save point file
3. Say: "Restoring from save point. Continue from M9."

### Option B: New Claude.ai Project from Scratch
1. Create new Claude.ai Project named "PGPulse"
2. Upload to Project Knowledge:
   - This save point file
   - PGAM_FEATURE_AUDIT.md
   - PGPulse_Development_Strategy_v2.md
   - Chat_Transition_Process.md
   - Save_Point_System.md
3. Open new chat, upload this save point
4. Say: "Restoring PGPulse project from save point. All context is in this file."

### Option C: Different AI Tool / New Developer
1. Clone repo: `git clone https://github.com/ios9000/PGPulse_01.git`
2. Read this save point file — it contains complete project context
3. Read `.claude/CLAUDE.md` for module ownership and interfaces
4. Read `docs/roadmap.md` for current milestone status
5. Continue development from the "What's Next" section above

### Option D: Complete Disaster Recovery
If the repo is lost:
1. This save point contains all interfaces and key design decisions
2. The architecture and decisions are documented above
3. PGAM SQL queries are in the audit doc (76 queries with exact SQL)
4. Rebuild from the interfaces → implement collectors → add storage → add API → add ML
