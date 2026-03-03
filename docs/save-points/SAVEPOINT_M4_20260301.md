# PGPulse — Save Point

**Save Point:** M4 — Alerting (complete)
**Date:** 2026-03-01
**Commit:** 7c6ab36
**Developer:** Evlampiy (ios9000)
**AI Tool:** Claude.ai (Opus 4.6) + Claude Code 2.1.63 (bash fixed, agents run build/test/lint directly)

---

## 1. PROJECT IDENTITY

**Name:** PGPulse — PostgreSQL Health & Activity Monitor
**Repo:** https://github.com/ios9000/PGPulse_01
**Legacy repo:** https://github.com/ios9000/pgam-legacy
**Go module:** github.com/ios9000/PGPulse_01
**License:** TBD

### What PGPulse Does
PGPulse is a real-time PostgreSQL monitoring tool that collects metrics from PG 14–18 instances (connections, cache hit ratio, replication lag, locks, wait events, pg_stat_statements, vacuum progress, bloat), stores them in PostgreSQL (TimescaleDB-ready), serves them via a REST API with JSON and CSV export, includes JWT-based authentication with RBAC, and now features a complete alerting pipeline with threshold evaluation, state machine (OK→PENDING→FIRING→OK), 19 built-in alert rules, email notifications with rich HTML templates, and an async dispatcher with retry logic. Planned features include an embedded web UI (M5), OS agent mode (M6), and ML-based anomaly detection (M8). It's designed as a single Go binary with an embedded web UI, targeting PostgreSQL DBAs who need a lightweight, PG-specialized alternative to heavyweight platforms like PMM or SolarWinds.

### Origin Story
Rewrite of PGAM — a legacy PHP PostgreSQL Activity Monitor used internally at VTB Bank. PGAM had 76 SQL queries across 2 PHP files (analiz2.php + analiz_db.php), zero authentication, SQL injection vulnerabilities via raw GET params, and relied on COPY TO PROGRAM for OS metrics (requiring superuser). PGPulse is a clean-room rewrite in Go that preserves the SQL monitoring knowledge while fixing every architectural and security flaw.

---

## 2. ARCHITECTURE SNAPSHOT

### Tech Stack
| Component | Choice | Version | Why |
|---|---|---|---|
| Language | Go | 1.24.0 | Upgraded from 1.23.6; pgx v5.8.0 requires ≥ 1.24 |
| PG Driver | jackc/pgx v5 | 5.8.0 | Best Go PG driver, parameterized queries, named args, CopyFrom |
| HTTP Router | go-chi/chi v5 | 5.2.5 | Lightweight, middleware-friendly |
| JWT | golang-jwt/jwt v5 | 5.2.2 | Added in M3 for authentication |
| Crypto | x/crypto | (transitive→direct) | bcrypt for password hashing, promoted in M3 |
| Storage | PostgreSQL + TimescaleDB | — | PG-native; hypertable creation conditional on config flag |
| Config | koanf v2 | 2.3.2 | YAML + env var overrides; mapstructure for duration parsing |
| Frontend | Svelte + Tailwind | — | Embedded via go:embed (M5) |
| Logging | log/slog | stdlib | Structured logging |
| Testing | testcontainers-go | 0.40.0 | Real PG instances in CI tests |
| ML (Phase 1) | gonum | — | Pure Go statistics (M8) |
| CI | GitHub Actions | — | Lint + test + build |
| Linter | golangci-lint | v2.10.1 | Go 1.24 required v2 config format |

### Architecture Diagram
```
┌──────────────────────────────────────────────────────────────────┐
│                    PGPulse Server (Go binary)                     │
│                                                                   │
│  ┌─────────────────────────────────────────────────────────────┐ │
│  │  HTTP Server (chi v5, :8080)                       [M2+M3] │ │
│  │  ┌──────────┐ ┌──────────┐ ┌───────────────────────┐       │ │
│  │  │ /health  │ │/instances│ │/instances/{id}/metrics│       │ │
│  │  │ (public) │ │(viewer+) │ │(viewer+)              │       │ │
│  │  └──────────┘ └──────────┘ └───────────────────────┘       │ │
│  │  ┌─────────────────┐ ┌──────────────┐ ┌────────────┐       │ │
│  │  │ /auth/login     │ │/auth/refresh │ │ /auth/me   │       │ │
│  │  │ (public+ratelim)│ │(public)      │ │ (viewer+)  │       │ │
│  │  └─────────────────┘ └──────────────┘ └────────────┘       │ │
│  │  ┌───────────────────────────────────────────────────┐      │ │
│  │  │ /alerts, /alerts/history, /alerts/rules (viewer+) │ [M4] │ │
│  │  │ POST /alerts/rules, PUT, DELETE (admin)           │      │ │
│  │  │ POST /alerts/test (admin)                         │      │ │
│  │  └───────────────────────────────────────────────────┘      │ │
│  │  Middleware: RequestID → Logger → Recoverer → CORS →       │ │
│  │              JWT Auth (or AuthStub when disabled)            │ │
│  └──────────────────────────────┬──────────────────────────────┘ │
│                                  │                                │
│  ┌───────────────────────────────▼─────────────────────────────┐ │
│  │  Auth (internal/auth/)                            [M3]      │ │
│  │  JWT HS256 (access 24h + refresh 7d) │ bcrypt passwords     │ │
│  │  RBAC: admin / viewer │ Rate limiter (10/15min/IP)          │ │
│  └─────────────────────────────────────────────────────────────┘ │
│                                                                   │
│  ┌───────────────────────────────┬─────────────────────────────┐ │
│  │  Alert Pipeline                               [M4]          │ │
│  │  Evaluator: state machine (OK→PENDING→FIRING→OK)           │ │
│  │  19 rules (14 PGAM + 2 new + 3 deferred)                   │ │
│  │  Dispatcher: async channel(100), cooldown, retry(1s/2s/4s)  │ │
│  │  Email notifier: SMTP + STARTTLS, rich HTML templates       │ │
│  │  History: alert_history table, 30-day cleanup               │ │
│  └──────────────────────────────┬──────────────────────────────┘ │
│                                  │                                │
│  ┌───────────────────────────────▼─────────────────────────────┐ │
│  │  Storage (PGStore | LogStore)                     [M2]      │ │
│  │  Write: CopyFrom (10s)       │ Query: dynamic WHERE (30s)  │ │
│  │  Migrations: 001_metrics + 002_timescaledb (cond.) +       │ │
│  │    003_users [M3] + 004_alerts [M4]                         │ │
│  └──────────────────────────────▲──────────────────────────────┘ │
│                                  │ Write                          │
│  ┌───────────────────────────────┴─────────────────────────────┐ │
│  │  Orchestrator                                     [M2+M4]  │ │
│  │  instanceRunners → intervalGroups (high/med/low)            │ │
│  │  Post-collect hook: evaluateAlerts() → dispatcher           │ │
│  └───────────────────────────────┬─────────────────────────────┘ │
│                                  │                                │
│  ┌───────────────────────────────▼─────────────────────────────┐ │
│  │  Collectors (20 files, 23 constructors)            [M1]     │ │
│  │  connections, cache, wait_events, lock_tree,                 │ │
│  │  long_txns, replication_*, checkpoint,                       │ │
│  │  statements_*, progress_*, server_info, ...                  │ │
│  └───────┬─────────────────────────────────────────────────────┘ │
│          ↓                                                        │
│  ┌───────▼─────────┐                                             │
│  │  Version Gate   │                                             │
│  │  PG 14–18 SQL  │                                             │
│  └─────────────────┘                                             │
└──────────────────────────────────────────────────────────────────┘
         │
    ┌────▼─────┐
    │ PGPulse  │  (optional, separate binary, M6)
    │  Agent   │  OS metrics via procfs
    └──────────┘
```

### Key Design Decisions

| # | Decision | Rationale | Date |
|---|----------|-----------|------|
| D1 | Single binary deployment | Simplicity, go:embed frontend | 2026-02-25 |
| D2 | pgx v5 (not database/sql) | Named args, COPY protocol, pgx-specific features | 2026-02-25 |
| D3 | Version-adaptive SQL via Gate pattern | Support PG 14-18 without code branches | 2026-02-25 |
| D4 | No COPY TO PROGRAM ever | PGAM's worst security flaw — eliminated | 2026-02-25 |
| D5 | pg_monitor role only | Least privilege, never superuser | 2026-02-25 |
| D6 | One Collector per module file | Testable, enable/disable, independent intervals | 2026-02-25 |
| D7 | ~~Hybrid agent workflow~~ → Direct execution | Claude Code v2.1.63 fixed bash on Windows | 2026-03-01 |
| D8 | Agent Teams (4 agents max) | Right-sized for 1-dev project | 2026-02-25 |
| D9 | Three-tier persistence | Save Points + Handoffs + Session-logs | 2026-02-25 |
| D10 | Base struct with point() helper | Auto-prefixes "pgpulse.", fills InstanceID + Timestamp | 2026-02-25 |
| D11 | Registry pattern for collectors | CollectAll() with partial-failure semantics | 2026-02-25 |
| D12 | 5s statement_timeout for live collectors | Via context.WithTimeout in queryContext() helper | 2026-02-25 |
| D13 | InstanceContext SSoT | Orchestrator queries pg_is_in_recovery() once per cycle | 2026-02-25 |
| D14 | Version in Base, IsRecovery in InstanceContext | Version = structural (immutable). Recovery = dynamic | 2026-02-25 |
| D15 | Defer logical replication Q41 | Requires per-DB connections, breaks single-conn interface | 2026-02-25 |
| D16 | golangci-lint v2 config format | v1 doesn't support Go 1.24 | 2026-02-25 |
| D17 | Docker Desktop not available | BIOS virtualization disabled. Integration tests CI-only | 2026-02-25 |
| D18–D34 | M1 collector decisions | See SAVEPOINT_M2_20260227.md for full list | 2026-02-26 |
| D35–D52 | M2 storage/API decisions | See SAVEPOINT_M2_20260227.md for full list | 2026-02-27 |
| D53–D62 | M3 auth decisions | JWT HS256, stateless refresh, RBAC admin+viewer, PG-backed users, config-seeded admin, in-memory rate limit, errorWriter pattern, chi middleware ordering, auth-requires-storage, CSRF deferred to M5 | 2026-03-01 |
| D63 | M4 split: 3 iterations | M4_01 evaluator, M4_02 email+dispatcher, M4_03 wiring | 2026-03-01 |
| D64 | MVP notifiers: email only | Telegram/Slack/webhook deferred to M7+ | 2026-03-01 |
| D65 | Alert rules: YAML builtins + DB custom | Builtins seeded via UpsertBuiltin on startup, preserving user overrides | 2026-03-01 |
| D66 | Alert history table, 30-day retention | Cleanup goroutine every 1 hour | 2026-03-01 |
| D67 | Evaluator post-collect hook | Synchronous in orchestrator after store.Write(), errors don't abort cycle | 2026-03-01 |
| D68 | In-memory state machine | Reconstruct from alert_history on restart (unresolved alerts) | 2026-03-01 |
| D69 | Global default channels in config | Per-rule override optional. Email only for MVP | 2026-03-01 |
| D70 | Hysteresis: 3 consecutive by default | Configurable per rule via consecutive_count | 2026-03-01 |
| D71 | Cooldown: 15 min default | State transitions (escalation, resolution) always notify immediately | 2026-03-01 |
| D72 | 3 deferred rules (enabled: false) | WAL spike, query regression, disk forecast — need data sources from M6/M8 | 2026-03-01 |
| D73 | Rich HTML email templates | Color-coded severity, table layout, dashboard link, plain text fallback | 2026-03-01 |
| D74 | Async dispatcher (channel buffer 100) | Non-blocking for evaluator, drop + warn if full | 2026-03-01 |
| D75 | 3 retries, exponential backoff (1s/2s/4s) | Then log and drop | 2026-03-01 |
| D76 | Go stdlib net/smtp only | No external email libraries | 2026-03-01 |
| D77 | Orchestrator interfaces for alert | AlertEvaluator/AlertDispatcher defined in orchestrator package, not imported from alert | 2026-03-01 |
| D78 | Builtin rules cannot be deleted | 409 Conflict; only disabled via enabled=false | 2026-03-01 |
| D79 | Rule mutations refresh evaluator | create/update/delete → evaluator.LoadRules() | 2026-03-01 |
| D80 | Shutdown order updated | HTTP → Orchestrator → Dispatcher → Store | 2026-03-01 |

---

## 3. CODEBASE STATE

### Repository Structure (after M4)

```
PGPulse_01/
├── .claude/
│   ├── CLAUDE.md
│   ├── settings.json
│   └── rules/
│       ├── code-style.md
│       ├── architecture.md
│       ├── security.md
│       └── postgresql.md
│
├── cmd/
│   ├── pgpulse-server/
│   │   └── main.go               ← Config → Storage → Migrate → Auth → Alert pipeline → HTTP → Orchestrator → shutdown
│   └── pgpulse-agent/
│       └── main.go               ← OS agent placeholder
│
├── configs/
│   └── pgpulse.example.yml       ← Includes auth + alerting + email sections
│
├── internal/
│   ├── alert/                     ← NEW in M4
│   │   ├── alert.go               ← Rule, AlertEvent, Severity, Operator, AlertState, stateEntry
│   │   ├── store.go               ← AlertRuleStore, AlertHistoryStore, AlertHistoryQuery interfaces
│   │   ├── evaluator.go           ← Evaluator: state machine, LoadRules, RestoreState, Evaluate, StartCleanup
│   │   ├── rules.go               ← BuiltinRules() — 19 rules
│   │   ├── pgstore.go             ← PGAlertRuleStore, PGAlertHistoryStore (CRUD + UpsertBuiltin)
│   │   ├── seed.go                ← SeedBuiltinRules() — idempotent startup seeding
│   │   ├── notifier.go            ← Notifier interface, NotifierRegistry
│   │   ├── template.go            ← HTML + text email templates, FormatSubject, severity helpers
│   │   ├── dispatcher.go          ← Async dispatcher: channel(100), cooldown, routing, retry
│   │   ├── notifier/
│   │   │   ├── email.go           ← SMTP EmailNotifier: STARTTLS, PLAIN auth, MIME multipart
│   │   │   └── email_test.go
│   │   ├── alert_test.go
│   │   ├── evaluator_test.go
│   │   ├── rules_test.go
│   │   ├── pgstore_test.go
│   │   ├── seed_test.go
│   │   ├── template_test.go
│   │   └── dispatcher_test.go
│   │
│   ├── api/
│   │   ├── server.go              ← APIServer with auth + alert fields, conditional Routes()
│   │   ├── response.go            ← Envelope, writeJSON, writeError, writeErrorRaw
│   │   ├── middleware.go          ← requestID, logger, recoverer, CORS, authStub, UserFromContext
│   │   ├── health.go              ← GET /api/v1/health (always public)
│   │   ├── instances.go           ← GET /api/v1/instances, GET /api/v1/instances/{id}
│   │   ├── metrics.go             ← GET /api/v1/instances/{id}/metrics (JSON + CSV)
│   │   ├── auth.go                ← POST /auth/login, POST /auth/refresh, GET /auth/me
│   │   ├── alerts.go              ← NEW: 7 alert handlers (CRUD rules, active, history, test)
│   │   ├── helpers_test.go        ← mockStore, mockPinger, mockUserStore, mock alert stores, newTestServer
│   │   ├── health_test.go, instances_test.go, metrics_test.go, middleware_test.go, auth_test.go
│   │   └── alerts_test.go         ← NEW: alert handler tests
│   │
│   ├── auth/                      ← M3
│   │   ├── store.go, password.go, rbac.go, jwt.go, ratelimit.go, middleware.go
│   │   └── [test files]
│   │
│   ├── collector/                 ← M1 (20 collector files + tests)
│   │   ├── collector.go, base.go, registry.go
│   │   └── [20 collector files + test files]
│   │
│   ├── config/
│   │   ├── config.go              ← Config, ServerConfig, AuthConfig, AlertingConfig, EmailConfig, StorageConfig, InstanceConfig
│   │   ├── load.go                ← Load() via koanf, validate(), validateAuth(), validateAlerting()
│   │   └── config_test.go
│   │
│   ├── orchestrator/
│   │   ├── orchestrator.go        ← New(), Start(), Stop() — accepts evaluator + dispatcher
│   │   ├── runner.go              ← instanceRunner with evaluator/dispatcher pass-through
│   │   ├── group.go               ← intervalGroup: collect() + evaluateAlerts() post-collect hook
│   │   ├── logstore.go
│   │   └── [test files]
│   │
│   ├── storage/
│   │   ├── migrations/
│   │   │   ├── 001_metrics.sql, 002_timescaledb.sql, 003_users.sql
│   │   │   └── 004_alerts.sql     ← NEW: alert_rules + alert_history tables
│   │   ├── migrate.go, pgstore.go, pool.go
│   │   └── [test files]
│   │
│   └── version/
│       ├── version.go
│       └── gate.go
│
├── docs/
│   ├── iterations/
│   │   ├── M1_01 through M1_05 folders
│   │   ├── M2_01, M2_02, M2_03 folders
│   │   ├── M3_01_03012026_auth-security/
│   │   ├── M4_01_03012026_alert-evaluator/
│   │   ├── M4_02_03012026_email-dispatcher/
│   │   ├── M4_03_03012026_alert-wiring/
│   │   ├── HANDOFF_M2_03_to_M3_01.md
│   │   └── HANDOFF_M3_to_M4.md
│   │
│   ├── save-points/
│   │   ├── SAVEPOINT_M0 through SAVEPOINT_M3 files
│   │   ├── SAVEPOINT_M4_20260301.md    ← THIS FILE
│   │   └── LATEST.md                   ← Copy of this file
│   │
│   ├── roadmap.md, CHANGELOG.md, RESTORE_CONTEXT.md
│
├── .golangci.yml, go.mod, go.sum, Makefile
```

### Key Interfaces

#### Collector (internal/collector/collector.go — unchanged since M1)

```go
type InstanceContext struct { IsRecovery bool }

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
    Start, End time.Time
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
```

#### Alert (internal/alert/ — new in M4)

```go
// alert.go
type Severity string   // "info", "warning", "critical"
type Operator string   // ">", ">=", "<", "<=", "==", "!="
type AlertState string // "ok", "pending", "firing"
type RuleSource string // "builtin", "custom"

type Rule struct {
    ID, Name, Description, Metric string
    Operator         Operator
    Threshold        float64
    Severity         Severity
    Labels           map[string]string
    ConsecutiveCount int
    CooldownMinutes  int
    Channels         []string
    Source           RuleSource
    Enabled          bool
}

type AlertEvent struct {
    RuleID, RuleName, InstanceID string
    Severity     Severity
    Value        float64
    Threshold    float64
    Operator     Operator
    Metric       string
    Labels       map[string]string
    Channels     []string
    FiredAt      time.Time
    ResolvedAt   *time.Time
    IsResolution bool
}

// store.go
type AlertRuleStore interface {
    List(ctx) ([]Rule, error)
    ListEnabled(ctx) ([]Rule, error)
    Get(ctx, id) (*Rule, error)
    Create(ctx, *Rule) error
    Update(ctx, *Rule) error
    Delete(ctx, id) error
    UpsertBuiltin(ctx, *Rule) error
}

type AlertHistoryStore interface {
    Record(ctx, *AlertEvent) error
    Resolve(ctx, ruleID, instanceID string, resolvedAt time.Time) error
    ListUnresolved(ctx) ([]AlertEvent, error)
    Query(ctx, AlertHistoryQuery) ([]AlertEvent, error)
    Cleanup(ctx, retention time.Duration) (int64, error)
}

// evaluator.go
type Evaluator struct { /* ruleStore, historyStore, logger, rules, states, mu */ }
func NewEvaluator(ruleStore, historyStore, logger) *Evaluator
func (e *Evaluator) LoadRules(ctx) error
func (e *Evaluator) RestoreState(ctx) error
func (e *Evaluator) Evaluate(ctx, instanceID, []MetricPoint) ([]AlertEvent, error)
func (e *Evaluator) StartCleanup(ctx, retentionDays)

// notifier.go
type Notifier interface {
    Name() string
    Send(ctx context.Context, event AlertEvent) error
}
type NotifierRegistry struct { /* map[string]Notifier */ }

// dispatcher.go
type Dispatcher struct { /* registry, defaultChannels, cooldownMinutes, events chan, cooldowns map */ }
func NewDispatcher(registry, defaultChannels, cooldownMinutes, logger) *Dispatcher
func (d *Dispatcher) Start()
func (d *Dispatcher) Dispatch(event AlertEvent) bool  // non-blocking
func (d *Dispatcher) Stop()
```

#### Auth (internal/auth/ — unchanged from M3)

```go
type UserStore interface {
    GetByUsername(ctx, username) (*User, error)
    Create(ctx, username, passwordHash, role) (*User, error)
    Count(ctx) (int64, error)
}

type JWTService struct { /* secret, accessTTL, refreshTTL */ }
func (s *JWTService) GenerateTokenPair(user) (*TokenPair, error)
func (s *JWTService) ValidateToken(tokenString) (*Claims, error)
```

### REST API Endpoints (after M4)

| Method | Path | Purpose | Auth |
|--------|------|---------|------|
| GET | `/api/v1/health` | Liveness + storage ping + uptime | Public |
| POST | `/api/v1/auth/login` | Authenticate, return token pair | Public (rate-limited) |
| POST | `/api/v1/auth/refresh` | Refresh access token | Public |
| GET | `/api/v1/auth/me` | Current user info | viewer+ |
| GET | `/api/v1/instances` | List instances from config | viewer+ |
| GET | `/api/v1/instances/{id}` | Single instance detail | viewer+ |
| GET | `/api/v1/instances/{id}/metrics` | Query stored metrics (JSON/CSV) | viewer+ |
| GET | `/api/v1/alerts` | List active (unresolved) alerts | viewer+ |
| GET | `/api/v1/alerts/history` | Query alert history with filters | viewer+ |
| GET | `/api/v1/alerts/rules` | List all alert rules | viewer+ |
| POST | `/api/v1/alerts/rules` | Create custom rule | admin |
| PUT | `/api/v1/alerts/rules/{id}` | Update rule | admin |
| DELETE | `/api/v1/alerts/rules/{id}` | Delete custom rule (builtin → 409) | admin |
| POST | `/api/v1/alerts/test` | Send test notification | admin |

**Total: 14 endpoints** (7 prior + 7 new in M4)

### main.go Flow (after M4)

```
main.go
  → config.Load(path) → Config (includes AuthConfig, AlertingConfig)
  → if cfg.Storage.DSN != "":
      storage.NewPool → pool
      storage.Migrate (001, 002, 003, 004) → runs all migrations
      storage.NewPGStore → store
    else:
      orchestrator.NewLogStore → store
  → if cfg.Auth.Enabled:
      auth.NewPGUserStore → userStore
      if userStore.Count() == 0 → seed initial admin
      auth.NewJWTService → jwtSvc
  → if cfg.Alerting.Enabled:
      alert.NewPGAlertRuleStore, NewPGAlertHistoryStore
      alert.SeedBuiltinRules (idempotent upsert 19 rules)
      alert.NewEvaluator → LoadRules → RestoreState → StartCleanup
      alert.NewNotifierRegistry + notifier.NewEmailNotifier (if email config)
      alert.NewDispatcher → Start()
  → api.New(cfg, store, pool, jwtSvc, userStore, logger, alertRuleStore, alertHistoryStore, evaluator, dispatcher, registry)
  → orchestrator.New(cfg, store, logger, evaluator, dispatcher) → Start()
  → signal.Notify(SIGINT, SIGTERM)
  → Shutdown: HTTP → Orchestrator → Dispatcher.Stop() → Store.Close()
```

### Dependencies (go.mod key entries)
```
go 1.24.0
github.com/jackc/pgx/v5 v5.8.0
github.com/go-chi/chi/v5 v5.2.5
github.com/golang-jwt/jwt/v5 v5.2.2
github.com/knadh/koanf/v2 v2.3.2
golang.org/x/crypto (bcrypt)
```

---

## 4. LEGACY KNOWLEDGE (PGAM)

### Query Porting Status

| Source | Queries | Target | Status |
|--------|---------|--------|--------|
| analiz2.php Q1 | PG version string | version.Detect() | ✅ Done (M0) |
| analiz2.php Q2–Q3, Q9–Q10 | Server info | server_info.go | ✅ Done (M1_01) |
| analiz2.php Q4–Q8 | OS metrics | — | ⏭️ Deferred to M6 |
| analiz2.php Q11–Q13 | Connections | connections.go | ✅ Done (M1_01) |
| analiz2.php Q14 | Cache hit ratio | cache.go | ✅ Done (M1_01) |
| analiz2.php Q15 | Transactions | transactions.go | ✅ Done (M1_01) |
| analiz2.php Q16 | Database sizes | database_sizes.go | ✅ Done (M1_01) |
| analiz2.php Q17 | Settings | settings.go | ✅ Done (M1_01) |
| analiz2.php Q18–Q19 | Extensions/pgss | extensions.go | ✅ Done (M1_01) |
| analiz2.php Q20–Q21 | Replication status | replication_status.go | ✅ Done (M1_02b) |
| analiz2.php Q22–Q35 | OS/cluster/overview | — | 🔲 M6/later |
| analiz2.php Q36, Q39 | Replication PG < 10 | — | ⏭️ Skipped (below min) |
| analiz2.php Q37–Q38 | Replication lag | replication_lag.go | ✅ Done (M1_02b) |
| analiz2.php Q40 | Replication slots | replication_slots.go | ✅ Done (M1_02b) |
| analiz2.php Q41 | Logical replication | — | ⏭️ Deferred (PerDatabaseCollector) |
| analiz2.php Q42–Q47 | Progress monitoring | progress_*.go | ✅ Done (M1_03) |
| analiz2.php Q48–Q49 | pgss settings/fill%/reset | statements_config.go | ✅ Done (M1_04) |
| analiz2.php Q50–Q51 | Top-N by IO/CPU time | statements_top.go | ✅ Done (M1_04) |
| analiz2.php Q52 | Normalized query stats | — | ⏭️ Deferred |
| analiz2.php Q53–Q54 | Wait event summary | wait_events.go | ✅ Done (M1_05) |
| analiz2.php Q55 | Lock blocking tree | lock_tree.go | ✅ Done (M1_05) |
| analiz2.php Q56–Q57 | Long transactions | long_transactions.go | ✅ Done (M1_05) |
| analiz2.php Q58 | Lock details | — | ⏭️ Deferred |
| analiz_db.php Q1–Q18 | Per-DB analysis | — | 🔲 Later milestone |
| — (new) | Checkpoint/bgwriter | checkpoint.go | ✅ Done (M1_03) |
| — (new) | pg_stat_io | io_stats.go | ✅ Done (M1_03b) |
| **Total: 76** | | | **33 done, 11 deferred/skipped, 32 remaining** |

### PGAM Bugs Fixed During Port

| # | Query | Bug | Fix |
|---|-------|-----|-----|
| 1 | Q11 | Counts own monitoring connection | `WHERE pid != pg_backend_pid()` |
| 2 | Q14 | Division by zero on empty cache | `NULLIF(blks_hit + blks_read, 0)` guard |
| 3 | Q4-Q8 | OS metrics via COPY TO PROGRAM (superuser) | Eliminated — Go agent via procfs (M6) |
| 4 | Q10 | pg_is_in_backup() removed in PG 15 | Version-gated: skip for PG ≥ 15 |
| 5 | Q50-Q51 | Two separate round-trips for IO/CPU | Unified into single CTE |
| 6 | Q55 | Recursive pg_locks CTE — complex, fragile | Replaced with pg_blocking_pids() + Go BFS |
| 7 | Q56-Q57 | Two queries differing only in wait_event IS NULL | Merged with CASE WHEN into one query |
| 8 | _auth.php | No authentication whatsoever | JWT + bcrypt + RBAC in M3 |
| 9 | All pages | SQL injection via raw GET params | Parameterized queries + server inventory |
| 10 | No alerting | Visual color-coding only, no notifications | Full alert pipeline with email in M4 |

### PGAM Alert Thresholds Ported (M4)

All 14 PGAM visual thresholds now implemented as evaluator rules with notifications:

| Rule | Warning | Critical | Source |
|------|---------|----------|--------|
| Wraparound | >20% | >50% | PGAM |
| Multixact | >20% | >50% | PGAM |
| Connections | >80% | ≥99% | PGAM |
| Cache hit | <90% | — | PGAM |
| Commit ratio | <90% | — | PGAM |
| Replication slot inactive | — | active=false | PGAM |
| Long transaction | >1min | ≥5min | PGAM |
| Bloat | >2× | >50× | PGAM |
| pgss fill | ≥95% | — | PGAM |
| Replication lag | >1MB | >100MB | NEW |
| WAL spike | — | — | NEW (deferred) |
| Query regression | — | — | NEW (deferred) |
| Disk forecast | — | — | NEW (deferred) |

---

## 5. MILESTONE STATUS

### Roadmap

| Milestone | Name | Status | Completion Date |
|---|---|---|---|
| M0 | Project Setup | ✅ Done | 2026-02-25 |
| M1 | Core Collectors | ✅ Done | 2026-02-26 |
| M2 | Storage & API | ✅ Done | 2026-02-27 |
| M3 | Auth & Security | ✅ Done | 2026-03-01 |
| M4 | Alerting | ✅ Done | 2026-03-01 |
| M5 | Web UI (MVP) | 🔲 Next | — |
| M6 | Agent Mode | 🔲 Not started | — |
| M7–M10 | P1 Features / ML / Reports / Polish | 🔲 Not started | — |

### What Was Just Completed (M4 — Alerting)

Three iterations delivered in one day:

**M4_01 — Evaluator Engine (commit 0371455):**
- Alert data model: Rule, AlertEvent, Severity, Operator, AlertState, RuleSource
- Evaluator with state machine: OK→PENDING→FIRING→OK
- Hysteresis (3 consecutive default), cooldown tracking
- 19 builtin rules (14 PGAM + 2 new replication lag + 3 deferred)
- AlertRuleStore + AlertHistoryStore interfaces and PG implementations
- Migration 004_alerts.sql (alert_rules + alert_history tables)
- Builtin rule seeding (idempotent UpsertBuiltin)
- Config: AlertingConfig (enabled, consecutive_count, cooldown, retention)

**M4_02 — Email Notifier & Dispatcher (commit eae52e1):**
- Notifier interface + NotifierRegistry
- SMTP EmailNotifier: STARTTLS, PLAIN auth, MIME multipart/alternative
- Rich HTML templates: color-coded severity, table layout, dashboard link, plain text fallback
- Async Dispatcher: buffered channel (100), cooldown enforcement, channel routing
- Retry with exponential backoff (1s, 2s, 4s), graceful shutdown drain
- Config: EmailConfig (host, port, from, recipients, TLS)

**M4_03 — API + Wiring (commit 7c6ab36):**
- 7 alert API endpoints (CRUD rules, active alerts, history, test notification)
- Orchestrator post-collect evaluateAlerts() hook
- AlertEvaluator/AlertDispatcher interfaces in orchestrator package
- main.go: full alert pipeline wiring with updated shutdown order
- History cleanup goroutine (1-hour interval, configurable retention)

### What's Next

**M5 — Web UI (MVP):**
- Embedded Svelte + Tailwind frontend via go:embed
- Dashboard views: server list, instance detail, database view
- Lock tree visualization
- pg_stat_statements top queries
- Active alerts panel
- Settings page (server inventory CRUD, alert rules)
- SSE for real-time metric streaming
- JWT auth flow (login page, token storage, auto-refresh)

---

## 6. DEVELOPMENT ENVIRONMENT

### Developer Workstation
| Component | Value |
|---|---|
| OS | Windows 10 |
| Shell | Git Bash (MSYS2) |
| Go | 1.24.0 windows/amd64 |
| Claude Code | 2.1.63 (bash WORKS on Windows) |
| Git | 2.52.0 |
| golangci-lint | v2.10.1 |
| Docker Desktop | Not installed (BIOS virtualization disabled) |

### Development Method
- **Two-contour model:** Claude.ai (Brain) + Claude Code (Hands)
- **Claude Code v2.1.63:** Bash works on Windows. Agents run go build/test/lint/commit directly.
- **One chat per iteration** in Claude.ai for planning; Claude Code for implementation
- **Single Sonnet for focused tasks; Agent Teams for multi-file iterations**

### Known Environment Issues

| Issue | Status | Workaround |
|---|---|---|
| ~~Claude Code bash EINVAL~~ | **FIXED in v2.1.63** | No workaround needed |
| LF/CRLF warnings on git add | Cosmetic | `.gitattributes` with `* text=auto eol=lf` |
| Docker Desktop unavailable | BIOS virtualization disabled | Integration tests CI-only |

---

## 7. IMPLEMENTATION PATTERNS

### Collector Pattern
```go
type XxxCollector struct { Base }
func NewXxxCollector(instanceID string, v version.PGVersion) *XxxCollector
func (c *XxxCollector) Collect(ctx, conn, ic) ([]MetricPoint, error)
func (c *XxxCollector) buildMetrics(rows []xxxRow) []MetricPoint { /* pure */ }
```

### Auth Pattern
```go
if s.authCfg.Enabled {
    r.Use(auth.RequireAuth(s.jwtService, writeErrorRaw))
    r.Use(auth.RequireRole("viewer", writeErrorRaw))
}
```

### Alert Pattern
```go
// Post-collect hook in orchestrator group
if g.evaluator != nil && len(allPoints) > 0 {
    events, err := g.evaluator.Evaluate(ctx, instanceID, allPoints)
    for _, event := range events {
        g.dispatcher.Dispatch(event)
    }
}
```

### Orchestrator Intervals
```
high   (10s):  connections, cache, wait_events, lock_tree, long_transactions
medium (60s):  replication_*, statements_*, checkpoint, progress_*
low    (300s): server_info, database_sizes, settings, extensions, transactions, io_stats
```

---

## 8. BUILD & TEST STATUS

| Check | Result |
|-------|--------|
| `go build ./...` | ✅ Pass |
| `go vet ./...` | ✅ Pass |
| `golangci-lint run` | ✅ 0 issues |
| Alert unit tests (evaluator, rules, template, dispatcher) | ✅ All pass |
| Alert integration tests (pgstore, seed) | ✅ All pass |
| Email notifier tests (mock SMTP) | ✅ All pass |
| API tests (alerts + prior) | ✅ All pass |
| Orchestrator tests (evaluator hook + prior) | ✅ All pass |
| Config tests | ✅ All pass |
| All prior tests (auth, storage, collector) | ✅ All pass, zero regressions |

---

## 9. HOW TO RESTORE THIS SAVE POINT

### Option A: Continue in Same Claude.ai Project
1. Open new chat in the PGPulse project
2. Upload this save point file
3. Say: "Restoring from save point. M4 is complete. Next is M5 (Web UI)."

### Option B: New Claude.ai Project / Different Tool
1. Clone: `git clone https://github.com/ios9000/PGPulse_01.git`
2. Read this file for complete context
3. Key interfaces: `internal/collector/collector.go`, `internal/alert/alert.go`, `internal/alert/store.go`, `internal/auth/`, `internal/api/server.go`, `internal/config/config.go`
4. `go test ./...` — all tests pass

### Option C: Complete Disaster Recovery
If the repo is lost:
1. This save point contains all interfaces, patterns, and architectural decisions
2. PGAM SQL queries are in PGAM_FEATURE_AUDIT.md (76 queries documented)
3. Rebuild order: version/ → collector/ → config/ → orchestrator/ → storage/ → auth/ → alert/ → api/
4. All design decisions documented in section 2 (D1–D80)
