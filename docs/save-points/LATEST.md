# PGPulse — Save Point

**Save Point:** M8 Complete — P1 Features + ML Phase 1 + Hotfixes
**Date:** 2026-03-10
**Commit:** e773c01
**Developer:** Archer
**AI Tool:** Claude.ai (Opus 4.6) + Claude Code (Agent Teams, bash works on Windows)

---

## 1. PROJECT IDENTITY

**Name:** PGPulse — PostgreSQL Health & Activity Monitor
**Repo:** https://github.com/ios9000/PGPulse_01
**Legacy repo:** https://github.com/ios9000/pgam-legacy
**Go module:** github.com/ios9000/PGPulse_01
**License:** [TBD]

### What PGPulse Does
PGPulse is a real-time PostgreSQL monitoring, alerting, and analytics tool built in Go. It connects to one or more PostgreSQL instances via pg_monitor role, collects ~70 metrics (ported from 76 legacy PGAM queries), stores time-series data in PostgreSQL/TimescaleDB, evaluates 23 alert rules (including ML-based forecast alerts), and presents everything through an embedded React web UI.

PGPulse is no longer read-only — DBAs can cancel/terminate sessions, run on-demand EXPLAIN plans, compare pg_settings across instances, browse auto-captured query plan history, view temporal settings timelines, and monitor logical replication — all directly from the UI. ML anomaly detection uses STL decomposition for seasonal baselines with Z-score/IQR scoring, plus Holt-Winters forecasting with confidence bands displayed on charts.

It supports PostgreSQL 14-18 via version-adaptive SQL gates, runs as a single binary with the frontend embedded via go:embed, and provides JWT authentication with 4-role RBAC. Instances can be managed through the web UI (add/edit/delete, CSV bulk import) with YAML seeding on startup and orchestrator hot-reload every 60 seconds.

### Origin Story
Rewrite of PGAM — a legacy PHP PostgreSQL Activity Monitor used internally at VTB Bank.
PGAM had 76 SQL queries across 2 PHP files (analiz2.php + analiz_db.php), zero auth,
SQL injection vulnerabilities, and relied on COPY TO PROGRAM for OS metrics (superuser required).
PGPulse is a clean-room rewrite in Go that preserves the SQL monitoring knowledge while
fixing every architectural and security flaw.

---

## 2. ARCHITECTURE SNAPSHOT

### Tech Stack
| Component | Choice | Version | Why |
|---|---|---|---|
| Language | Go | 1.24.0 | Performance, single binary, goroutines for collectors |
| PG Driver | jackc/pgx v5 | 5.8.0 | Best Go PG driver, parameterized queries |
| HTTP Router | go-chi/chi v5 | 5.2.5 | Lightweight, middleware-friendly |
| JWT | golang-jwt/jwt v5 | 5.2.2 | Authentication tokens |
| Storage | PostgreSQL + TimescaleDB | — | Time-series hypertables for metrics |
| Frontend | React 18 + TypeScript + Tailwind CSS + Apache ECharts | — | Embedded via go:embed |
| State Mgmt | Zustand 5.0 + TanStack Query 5 | — | Client state + server state |
| Config | koanf v2 | 2.3.2 | YAML + env vars |
| Logging | log/slog | stdlib | Structured logging |
| ML | gonum | 0.17.0 | Pure Go statistics (STL, Holt-Winters) |
| Testing | testcontainers-go | 0.40.0 | Real PG instances in tests |
| Concurrency | golang.org/x/sync | 0.19.0 | errgroup for parallel operations |
| CI | GitHub Actions | — | Lint + test + build |
| Linter | golangci-lint | v2.10.1 | v2 config format required |

### Architecture Diagram
```
+-----------------------------------------+
|         PGPulse Server (Go binary)      |
|                                         |
|  +---------+  +------+  +----------+   |
|  |Collectors|->|Storage|<-| REST API |   |
|  |(pgxpool) |  |(TSDB) |  |(chi+JWT) |   |
|  +----+----+  +-------+  +----+-----+   |
|       |                       |          |
|  +----v----+            +----v-----+    |
|  | Version |            |  Auth    |    |
|  |  Gate   |            | (RBAC)   |    |
|  +---------+            +----------+    |
|                                         |
|  +---------+  +------------------+      |
|  |  Alert  |  |  Web UI (embed)  |      |
|  | Engine  |  |  React+Tailwind  |      |
|  +---------+  +------------------+      |
|                                         |
|  +------------------------------+       |
|  |  ML Engine (M8)             |       |
|  |  STL Decomposition +        |       |
|  |  Holt-Winters Forecast +    |       |
|  |  Anomaly Detection          |       |
|  +------------------------------+       |
|                                         |
|  +------------------------------+       |
|  |  Instance Store (DB-backed)  |       |
|  |  YAML seed -> DB truth       |       |
|  |  Hot-reload every 60s        |       |
|  +------------------------------+       |
|                                         |
|  +------------------------------+       |
|  |  Plan Capture + Settings     |       |
|  |  Snapshots (background)      |       |
|  +------------------------------+       |
+-----------------------------------------+
         |
    +----v-----+
    | PGPulse  |  (optional, separate binary)
    |  Agent   |  OS metrics via procfs
    +----------+
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
| D7 | React 18 + TypeScript | Type safety, ecosystem maturity | 2026-03-03 |
| D8 | pgxpool.Pool per instance | Eliminates connection contention between collectors | 2026-03-04 |
| D9 | DB is source of truth for instances | YAML seeds on first start, DB overrides after | 2026-03-04 |
| D10 | DBCollector parallel to Collector | Per-database metrics without touching instance-level interface | 2026-03-08 |
| D11 | Simplified STL (EWMA + folded mean) | Full Loess too complex for pure-Go implementation | 2026-03-09 |
| D12 | mlerrors package breaks circular import | alert cannot import ml; shared sentinel errors in mlerrors | 2026-03-09 |
| D13 | Sustained crossing for forecast alerts | N consecutive points must cross threshold before alert fires | 2026-03-09 |

---

## 3. CODEBASE STATE

### Key Interfaces

```go
// internal/collector/collector.go

type InstanceContext struct { IsRecovery bool }

type MetricPoint struct {
    InstanceID string
    Metric     string
    Value      float64
    Labels     map[string]string
    Timestamp  time.Time
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

// M7 — per-database interfaces
type Queryer interface {
    Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
    QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type DBCollector interface {
    Name() string
    Interval() time.Duration
    CollectDB(ctx context.Context, q Queryer, dbName string, ic InstanceContext) ([]MetricPoint, error)
}
```

```go
// internal/auth/rbac.go — 4 roles, 5 permissions
var RolePermissions = map[Role][]Permission{
    RoleSuperAdmin: {PermUserManagement, PermInstanceManagement, PermAlertManagement, PermViewAll, PermSelfManagement},
    RoleRolesAdmin: {PermUserManagement, PermViewAll, PermSelfManagement},
    RoleDBA:        {PermInstanceManagement, PermAlertManagement, PermViewAll, PermSelfManagement},
    RoleAppAdmin:   {PermAlertManagement, PermViewAll, PermSelfManagement},
}
```

### Dependencies (go.mod)
```
module github.com/ios9000/PGPulse_01
go 1.24.0

require (
    github.com/go-chi/chi/v5 v5.2.5
    github.com/golang-jwt/jwt/v5 v5.2.2
    github.com/jackc/pgx/v5 v5.8.0
    github.com/knadh/koanf/parsers/yaml v1.1.0
    github.com/knadh/koanf/providers/env v1.1.0
    github.com/knadh/koanf/providers/file v1.2.1
    github.com/knadh/koanf/v2 v2.3.2
    github.com/stretchr/testify v1.11.1
    github.com/testcontainers/testcontainers-go v0.40.0
    github.com/testcontainers/testcontainers-go/modules/postgres v0.40.0
    golang.org/x/crypto v0.43.0
    gonum.org/v1/gonum v0.17.0
)
```

### SQL Migrations (001-011)
001_metrics, 002_timescaledb, 003_users, 004_alerts, 005_expand_roles, 006_instances, 007_session_audit_log, 008_plan_capture, 009_settings_snapshots, 010_ml_baseline_snapshots, 011_forecast_alert_consecutive

---

## 4. LEGACY KNOWLEDGE (PGAM)

### Query Porting Status

| Source | Queries | Ported | Status |
|--------|---------|--------|--------|
| analiz2.php #1-19 | 19 | 13 | Q4-Q8 OS -> agent M6 |
| analiz2.php #20-41 | 22 | 5 | Q20-Q21, Q37-Q40, Q41 |
| analiz2.php #42-47 | 6 | 6 | Complete |
| analiz2.php #48-52 | 5 | 4 | Q52 deferred |
| analiz2.php #53-58 | 6 | 5 | Q58 deferred |
| analiz_db.php #1-18 | 18 | 17 | Q1 dup skip |
| New (not in PGAM) | — | 6 | checkpoint, pg_stat_io, OS agent, cluster |
| **Total** | **76** | **~70** | |

---

## 5. MILESTONE STATUS

| Milestone | Name | Status | Date |
|---|---|---|---|
| M0 | Project Setup | Done | 2026-02-25 |
| M1 | Core Collector | Done | 2026-02-26 |
| M2 | Storage & API | Done | 2026-02-27 |
| M3 | Auth & Security | Done | 2026-03-01 |
| M4 | Alerting | Done | 2026-03-01 |
| M5 | Web UI (MVP) | Done | 2026-03-04 |
| M6 | Agent Mode + Cluster | Done | 2026-03-05 |
| M7 | Per-Database Analysis | Done | 2026-03-08 |
| M8 | P1 Features + ML Phase 1 | Done | 2026-03-10 |
| M9 | Reports & Export | Not Started | — |
| M10 | Polish & Release | Not Started | — |

### What's Next
M9 — Reports & Export (scheduled PDF/HTML reports, CSV/JSON export, dashboard snapshots)

---

## 6. REST API (38+ endpoints)

See full endpoint table in SAVEPOINT_M8_10_20260310.md section 8.

---

## 7. HOW TO RESTORE THIS SAVE POINT

### Option A: Continue in Same Claude.ai Project
1. Open new chat in the PGPulse project
2. Upload this save point file
3. Say: "Restoring from save point. Continue from M9."

### Option B: New Claude.ai Project from Scratch
1. Create new Claude.ai Project named "PGPulse"
2. Upload to Project Knowledge: this file + PGAM_FEATURE_AUDIT.md + PGPulse_Development_Strategy_v2.md
3. Open new chat, upload this save point
4. Say: "Restoring PGPulse project from save point. All context is in this file."

### Option C: Different AI Tool / New Developer
1. Clone repo: `git clone https://github.com/ios9000/PGPulse_01.git`
2. Read this save point + `.claude/CLAUDE.md` + `docs/roadmap.md`
3. Continue from "What's Next" above
