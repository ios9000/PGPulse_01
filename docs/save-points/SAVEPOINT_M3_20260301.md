# PGPulse — Save Point

**Save Point:** M3 — Auth & Security (complete)
**Date:** 2026-03-01
**Commit:** [update with actual commit hash after push]
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
PGPulse is a real-time PostgreSQL monitoring tool that collects metrics from PG 14–18 instances (connections, cache hit ratio, replication lag, locks, wait events, pg_stat_statements, vacuum progress, bloat), stores them in PostgreSQL (TimescaleDB-ready), serves them via a REST API with JSON and CSV export, and includes JWT-based authentication with RBAC. Planned features include alerting (M4), an embedded web UI (M5), and ML-based anomaly detection (M8). It's designed as a single Go binary with an embedded web UI, targeting PostgreSQL DBAs who need a lightweight, PG-specialized alternative to heavyweight platforms like PMM or SolarWinds.

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
┌──────────────────────────────────────────────────────────────┐
│                    PGPulse Server (Go binary)                 │
│                                                               │
│  ┌─────────────────────────────────────────────────────────┐ │
│  │  HTTP Server (chi v5, :8080)                   [M2+M3] │ │
│  │  ┌──────────┐ ┌──────────┐ ┌───────────────────────┐   │ │
│  │  │ /health  │ │/instances│ │/instances/{id}/metrics│   │ │
│  │  │ (public) │ │(viewer+) │ │(viewer+)              │   │ │
│  │  └──────────┘ └──────────┘ └───────────────────────┘   │ │
│  │  ┌─────────────────┐ ┌──────────────┐ ┌────────────┐   │ │
│  │  │ /auth/login     │ │/auth/refresh │ │ /auth/me   │   │ │
│  │  │ (public+ratelim)│ │(public)      │ │ (viewer+)  │   │ │
│  │  └─────────────────┘ └──────────────┘ └────────────┘   │ │
│  │  Middleware: RequestID → Logger → Recoverer → CORS →   │ │
│  │              JWT Auth (or AuthStub when disabled)        │ │
│  └──────────────────────────────┬──────────────────────────┘ │
│                                  │ Query                      │
│  ┌───────────────────────────────▼─────────────────────────┐ │
│  │  Auth (internal/auth/)                        [M3]      │ │
│  │  JWT HS256 (access 24h + refresh 7d) │ bcrypt passwords │ │
│  │  RBAC: admin / viewer │ Rate limiter (10/15min/IP)      │ │
│  │  UserStore (PG-backed) │ Initial admin seed from config │ │
│  └─────────────────────────────────────────────────────────┘ │
│                                                               │
│  ┌───────────────────────────────┬─────────────────────────┐ │
│  │  Storage (PGStore | LogStore) │              [M2]       │ │
│  │  Write: CopyFrom (10s)       │                          │ │
│  │  Query: dynamic WHERE (30s)  │                          │ │
│  │  Migrations: 001_metrics +   │                          │ │
│  │    002_timescaledb (cond.) + │                          │ │
│  │    003_users [M3]            │                          │ │
│  └──────────────────────────────▲──────────────────────────┘ │
│                                  │ Write                      │
│  ┌───────────────────────────────┴─────────────────────────┐ │
│  │  Orchestrator                                 [M2]      │ │
│  │  instanceRunners → intervalGroups (high/med/low)        │ │
│  └───────────────────────────────┬─────────────────────────┘ │
│                                  │                            │
│  ┌───────────────────────────────▼─────────────────────────┐ │
│  │  Collectors (20 files, 23 constructors)        [M1]     │ │
│  │  connections, cache, wait_events, lock_tree,             │ │
│  │  long_txns, replication_*, checkpoint,                   │ │
│  │  statements_*, progress_*, server_info, ...              │ │
│  └───────┬─────────────────────────────────────────────────┘ │
│          ↓                                                    │
│  ┌───────▼─────────┐                                         │
│  │  Version Gate   │                                         │
│  │  PG 14–18 SQL  │                                         │
│  └─────────────────┘                                         │
└──────────────────────────────────────────────────────────────┘
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
| D7 | ~~Hybrid agent workflow~~ → Direct execution | Claude Code v2.1.63 fixed bash on Windows. Agents run build/test/lint/commit directly | 2026-03-01 |
| D8 | Agent Teams (4 agents max) | Right-sized for 1-dev project | 2026-02-25 |
| D9 | Three-tier persistence | Save Points + Handoffs + Session-logs | 2026-02-25 |
| D10 | Base struct with point() helper | Auto-prefixes "pgpulse.", fills InstanceID + Timestamp | 2026-02-25 |
| D11 | Registry pattern for collectors | CollectAll() with partial-failure semantics; registered explicitly in runner.go | 2026-02-25 |
| D12 | 5s statement_timeout for live collectors | Via context.WithTimeout in queryContext() helper | 2026-02-25 |
| D13 | InstanceContext SSoT | Orchestrator queries pg_is_in_recovery() once per cycle | 2026-02-25 |
| D14 | Version in Base, IsRecovery in InstanceContext | Version = structural (immutable). Recovery = dynamic | 2026-02-25 |
| D15 | Defer logical replication Q41 | Requires per-DB connections, breaks single-conn interface | 2026-02-25 |
| D16 | golangci-lint v2 config format | v1 doesn't support Go 1.24 | 2026-02-25 |
| D17 | Docker Desktop not available | BIOS virtualization disabled. Integration tests CI-only | 2026-02-25 |
| D18–D34 | M1 collector decisions | See SAVEPOINT_M2_20260227.md for full list | 2026-02-26 |
| D35–D52 | M2 storage/API decisions | See SAVEPOINT_M2_20260227.md for full list | 2026-02-27 |
| D53 | JWT HS256 with access+refresh tokens | Stateless, no DB-stored refresh tokens. Access 24h, refresh 7d | 2026-03-01 |
| D54 | Stateless refresh tokens | No revocation until M5. Simplicity for monitoring tool | 2026-03-01 |
| D55 | RBAC: admin + viewer only | admin = full access, viewer = read-only. HasRole() level comparison | 2026-03-01 |
| D56 | Users table in PGPulse's own DB | auth package owns its store (internal/auth/store.go). UserStore interface | 2026-03-01 |
| D57 | Initial admin seeded from config | On first run when users table empty. bcrypt-hashed. Log warns to change | 2026-03-01 |
| D58 | Rate limiting: in-memory per-IP | 10 failed attempts / 15 min window. Login endpoint only. Resets on restart | 2026-03-01 |
| D59 | CSRF deferred to M5 | No browser client until web UI. Cookie-based auth not implemented yet | 2026-03-01 |
| D60 | errorWriter callback pattern | Auth middleware accepts func(w, code, errCode, msg) to write JSON errors without importing api package | 2026-03-01 |
| D61 | Chi middleware ordering | Use() before Get()/Post() on same mux. Auth-disabled routes in r.Group() | 2026-03-01 |
| D62 | Auth requires storage | auth.enabled=true + empty storage.dsn → startup error | 2026-03-01 |

---

## 3. CODEBASE STATE

### Repository Structure (after M3_01)

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
│   │   └── main.go               ← Config → Storage → Migrate → Auth seed → HTTP server → Orchestrator → shutdown
│   └── pgpulse-agent/
│       └── main.go               ← OS agent placeholder
│
├── configs/
│   └── pgpulse.example.yml       ← Includes auth section
│
├── internal/
│   ├── api/
│   │   ├── server.go              ← APIServer with auth fields, conditional Routes()
│   │   ├── response.go            ← Envelope, writeJSON, writeError, writeErrorRaw
│   │   ├── middleware.go          ← requestID, logger, recoverer, CORS, authStub, UserFromContext
│   │   ├── health.go              ← GET /api/v1/health (always public)
│   │   ├── instances.go           ← GET /api/v1/instances, GET /api/v1/instances/{id}
│   │   ├── metrics.go             ← GET /api/v1/instances/{id}/metrics (JSON + CSV)
│   │   ├── auth.go                ← POST /auth/login, POST /auth/refresh, GET /auth/me
│   │   ├── helpers_test.go        ← mockStore, mockPinger, mockUserStore, newTestServer
│   │   ├── health_test.go         ← 5 tests
│   │   ├── instances_test.go      ← 5 tests
│   │   ├── metrics_test.go        ← 10 tests
│   │   ├── middleware_test.go     ← 4 tests
│   │   └── auth_test.go           ← 7 tests
│   │
│   ├── auth/                      ← NEW in M3
│   │   ├── store.go               ← User struct, UserStore interface, PGUserStore
│   │   ├── password.go            ← HashPassword, CheckPassword (bcrypt)
│   │   ├── rbac.go                ← RoleAdmin, RoleViewer, HasRole, ValidRole
│   │   ├── jwt.go                 ← JWTService, Claims, TokenPair, GenerateTokenPair, ValidateToken
│   │   ├── ratelimit.go           ← RateLimiter (sliding window), ClientIP
│   │   ├── middleware.go          ← RequireAuth, RequireRole, ClaimsFromContext
│   │   ├── jwt_test.go            ← 6 tests
│   │   ├── password_test.go       ← 3 tests
│   │   ├── rbac_test.go           ← 6 tests
│   │   ├── ratelimit_test.go      ← 6 tests
│   │   ├── middleware_test.go     ← 6 tests
│   │   └── store_test.go          ← 3 integration tests (//go:build integration)
│   │
│   ├── collector/
│   │   ├── collector.go           ← Interfaces: Collector, MetricStore, MetricPoint, MetricQuery, InstanceContext
│   │   ├── base.go                ← Base struct, point(), queryContext(), pgssAvailable()
│   │   ├── registry.go
│   │   ├── server_info.go         ← Q2,Q3,Q9,Q10
│   │   ├── connections.go         ← Q11-Q13
│   │   ├── cache.go               ← Q14
│   │   ├── transactions.go        ← Q15
│   │   ├── database_sizes.go      ← Q16
│   │   ├── settings.go            ← Q17
│   │   ├── extensions.go          ← Q18-Q19
│   │   ├── replication_status.go  ← Q20,Q21
│   │   ├── replication_lag.go     ← Q37,Q38
│   │   ├── replication_slots.go   ← Q40
│   │   ├── progress_vacuum.go     ← Q42
│   │   ├── progress_maintenance.go ← Q43,Q45
│   │   ├── progress_operations.go ← Q44,Q46,Q47
│   │   ├── checkpoint.go          ← Checkpoint/BGWriter (stateful, version-gated)
│   │   ├── io_stats.go            ← pg_stat_io (PG 16+)
│   │   ├── statements_config.go   ← Q48,Q49
│   │   ├── statements_top.go      ← Q50,Q51
│   │   ├── wait_events.go         ← Q53/Q54
│   │   ├── lock_tree.go           ← Q55
│   │   ├── long_transactions.go   ← Q56/Q57
│   │   └── [20 test files]
│   │
│   ├── config/
│   │   ├── config.go              ← Config, ServerConfig, AuthConfig, InitialAdminConfig, StorageConfig, InstanceConfig
│   │   ├── load.go                ← Load() via koanf, validate(), validateAuth()
│   │   └── config_test.go         ← 10 tests
│   │
│   ├── orchestrator/
│   │   ├── orchestrator.go        ← New(), Start(), Stop()
│   │   ├── runner.go              ← instanceRunner: connect, buildCollectors, start, close
│   │   ├── group.go               ← intervalGroup: run(), collect(), queryInstanceContext()
│   │   ├── logstore.go            ← LogStore placeholder
│   │   ├── group_test.go          ← 4 tests
│   │   ├── logstore_test.go       ← 4 tests
│   │   └── orchestrator_test.go   ← 1 test
│   │
│   ├── storage/
│   │   ├── migrations/
│   │   │   ├── 001_metrics.sql    ← Metrics table + 3 indexes
│   │   │   ├── 002_timescaledb.sql ← Conditional hypertable
│   │   │   └── 003_users.sql      ← Users table with role CHECK constraint
│   │   ├── migrate.go             ← Migrate(), go:embed, schema_migrations bootstrap
│   │   ├── pgstore.go             ← PGStore: Write (CopyFrom), Query (dynamic WHERE), Close, Pool()
│   │   ├── pool.go                ← NewPool() (pgxpool, 5 max conns)
│   │   ├── migrate_test.go        ← 5 tests
│   │   ├── pgstore_test.go        ← 9 tests
│   │   └── pool_test.go           ← 1 test
│   │
│   └── version/
│       ├── version.go
│       └── gate.go
│
├── docs/
│   ├── iterations/
│   │   ├── M1_01 through M1_05 folders
│   │   ├── M2_01_02272026_config-orchestrator/
│   │   ├── M2_02_02272026_storage-layer/
│   │   ├── M2_03_02272026_rest-api/
│   │   ├── M3_01_03012026_auth-security/
│   │   │   ├── M3_01_requirements.md
│   │   │   ├── M3_01_design.md
│   │   │   └── M3_01_team-prompt.md
│   │   ├── HANDOFF_M2_03_to_M3_01.md
│   │   └── HANDOFF_M3_to_M4.md
│   │
│   ├── save-points/
│   │   ├── SAVEPOINT_M0_20260225.md
│   │   ├── SAVEPOINT_M1_20260225.md
│   │   ├── SAVEPOINT_M1_20260226.md
│   │   ├── SAVEPOINT_M1_20260226b.md
│   │   ├── SAVEPOINT_M1_20260226c.md
│   │   ├── SAVEPOINT_M2_20260226.md
│   │   ├── SAVEPOINT_M2_20260227.md
│   │   ├── SAVEPOINT_M3_20260301.md    ← THIS FILE
│   │   └── LATEST.md                   ← Copy of this file
│   │
│   ├── roadmap.md
│   ├── CHANGELOG.md
│   └── RESTORE_CONTEXT.md
│
├── .golangci.yml
├── go.mod
├── go.sum
└── Makefile
```

### Key Interfaces

#### Collector (internal/collector/collector.go — unchanged since M1)

```go
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
```

#### Auth (internal/auth/ — new in M3)

```go
// store.go
type User struct {
    ID           int64
    Username     string
    PasswordHash string
    Role         string
    CreatedAt    time.Time
    UpdatedAt    time.Time
}

type UserStore interface {
    GetByUsername(ctx context.Context, username string) (*User, error)
    Create(ctx context.Context, username, passwordHash, role string) (*User, error)
    Count(ctx context.Context) (int64, error)
}

// jwt.go
type TokenType string
const (
    TokenAccess  TokenType = "access"
    TokenRefresh TokenType = "refresh"
)

type Claims struct {
    jwt.RegisteredClaims
    UserID   int64     `json:"uid"`
    Username string    `json:"usr"`
    Role     string    `json:"role"`
    Type     TokenType `json:"type"`
}

type JWTService struct { /* secret, accessTokenTTL, refreshTokenTTL */ }
func NewJWTService(secret string, accessTTL, refreshTTL time.Duration) *JWTService
func (s *JWTService) GenerateTokenPair(user *User) (*TokenPair, error)
func (s *JWTService) GenerateAccessToken(user *User) (string, error)
func (s *JWTService) ValidateToken(tokenString string) (*Claims, error)
func (s *JWTService) AccessTokenTTL() time.Duration

// rbac.go
const RoleAdmin = "admin"
const RoleViewer = "viewer"
func HasRole(userRole, requiredRole string) bool

// middleware.go
func ClaimsFromContext(ctx context.Context) *Claims
func RequireAuth(jwtSvc *JWTService, errorWriter func(...)) func(http.Handler) http.Handler
func RequireRole(requiredRole string, errorWriter func(...)) func(http.Handler) http.Handler

// ratelimit.go
type RateLimiter struct { /* mu, attempts, maxAttempts, window */ }
func NewRateLimiter(maxAttempts int, window time.Duration) *RateLimiter
func (rl *RateLimiter) Allow(ip string) bool
func (rl *RateLimiter) RecordFailure(ip string)
```

#### API Server (internal/api/server.go — updated in M3)

```go
type APIServer struct {
    store       collector.MetricStore
    instances   []config.InstanceConfig
    serverCfg   config.ServerConfig
    authCfg     config.AuthConfig
    jwtService  *auth.JWTService    // nil when auth disabled
    userStore   auth.UserStore      // nil when auth disabled
    rateLimiter *auth.RateLimiter   // nil when auth disabled
    logger      *slog.Logger
    startTime   time.Time
    pool        Pinger
}

func New(cfg config.Config, store collector.MetricStore, pool Pinger,
    jwtSvc *auth.JWTService, userStore auth.UserStore, logger *slog.Logger) *APIServer
```

#### Config (internal/config/config.go — updated in M3)

```go
type AuthConfig struct {
    Enabled         bool                `koanf:"enabled"`
    JWTSecret       string              `koanf:"jwt_secret"`
    AccessTokenTTL  time.Duration       `koanf:"access_token_ttl"`
    RefreshTokenTTL time.Duration       `koanf:"refresh_token_ttl"`
    BcryptCost      int                 `koanf:"bcrypt_cost"`
    InitialAdmin    *InitialAdminConfig `koanf:"initial_admin"`
}
```

### REST API Endpoints (after M3)

| Method | Path | Purpose | Auth |
|--------|------|---------|------|
| GET | `/api/v1/health` | Liveness + storage ping + uptime | Public (always) |
| POST | `/api/v1/auth/login` | Authenticate, return token pair | Public (rate-limited) |
| POST | `/api/v1/auth/refresh` | Refresh access token | Public (needs refresh token) |
| GET | `/api/v1/auth/me` | Current user info | Requires viewer+ |
| GET | `/api/v1/instances` | List instances from config | Requires viewer+ |
| GET | `/api/v1/instances/{id}` | Single instance detail | Requires viewer+ |
| GET | `/api/v1/instances/{id}/metrics` | Query stored metrics (JSON/CSV) | Requires viewer+ |

### main.go Flow (after M3)

```
main.go
  → config.Load(path) → Config (includes AuthConfig)
  → if cfg.Storage.DSN != "":
      storage.NewPool(ctx, dsn) → pool
      storage.Migrate(ctx, pool, logger, MigrateOptions{...}) → runs 001, 002, 003
      storage.NewPGStore(pool, logger) → store
    else:
      orchestrator.NewLogStore(logger) → store
  → if cfg.Auth.Enabled:
      auth.NewPGUserStore(pool) → userStore
      if userStore.Count() == 0 → seed initial admin from config
      auth.NewJWTService(secret, accessTTL, refreshTTL) → jwtSvc
      api.New(cfg, store, pool, jwtSvc, userStore, logger) → apiServer
    else:
      api.New(cfg, store, pool, nil, nil, logger) → apiServer
  → &http.Server{Handler: apiServer.Routes(), ...}
  → go httpServer.ListenAndServe()
  → orchestrator.New(cfg, store, logger) → orch
  → orch.Start(ctx)
  → signal.Notify(SIGINT, SIGTERM)
  → httpServer.Shutdown(shutdownCtx)
  → orch.Stop()
  → store.Close()
```

### Dependencies (go.mod key entries)
```
go 1.24.0
github.com/jackc/pgx/v5 v5.8.0
github.com/go-chi/chi/v5 v5.2.5
github.com/golang-jwt/jwt/v5 v5.2.2
github.com/knadh/koanf/v2 v2.3.2
github.com/knadh/koanf/parsers/yaml v1.1.0
github.com/knadh/koanf/providers/file v1.2.1
github.com/knadh/koanf/providers/env v1.1.0
github.com/go-viper/mapstructure/v2 v2.4.0
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

### Version Gates Implemented

| # | Feature | Gate | Variants |
|---|---------|------|----------|
| 1 | Q10 | is_in_backup | PG ≤ 14: `pg_is_in_backup()` / PG ≥ 15: skip |
| 2 | Q19 | pgss_info | PG ≤ 13: skip / PG ≥ 14: `pg_stat_statements_info` |
| 3 | Q40 | replication_slots | PG 14 / PG 15 (+two_phase) / PG 16+ (+conflicting) |
| 4 | new | checkpoint_stats | PG 14–16: `pg_stat_bgwriter` / PG 17+: `pg_stat_checkpointer` CROSS JOIN |
| 5 | new | io_stats | PG < 16: nil,nil / PG ≥ 16: `pg_stat_io` |

---

## 5. MILESTONE STATUS

### Roadmap

| Milestone | Name | Status | Completion Date |
|---|---|---|---|
| M0 | Project Setup | ✅ Done | 2026-02-25 |
| M1 | Core Collectors | ✅ Done | 2026-02-26 |
| M2 | Storage & API | ✅ Done | 2026-02-27 |
| M3 | Auth & Security | ✅ Done | 2026-03-01 |
| M4 | Alerting | 🔲 Next | — |
| M5 | Web UI (MVP) | 🔲 Not started | — |
| M6 | Agent Mode | 🔲 Not started | — |
| M7–M10 | P1 Features / ML / Reports / Polish | 🔲 Not started | — |

### What Was Just Completed (M3_01 — Auth & Security)

- **JWT authentication**: HS256 tokens, access (24h default) + refresh (7d default), stateless. Claims: uid, usr, role, type
- **Password hashing**: bcrypt with configurable cost
- **RBAC**: admin (full access) and viewer (read-only). roleHierarchy map with HasRole() level comparison
- **User storage**: users table (003_users.sql migration), PGUserStore implementing UserStore interface
- **Initial admin seeding**: From config on first run when users table is empty
- **Rate limiting**: In-memory per-IP, 10 failed attempts / 15 min window on login endpoint
- **Auth middleware**: RequireAuth (JWT validation), RequireRole (RBAC check), errorWriter callback pattern
- **Auth toggle**: auth.enabled=false → authStubMiddleware (all open). auth.enabled=true → full JWT
- **API endpoints**: POST /auth/login, POST /auth/refresh, GET /auth/me
- **Config validation**: validateAuth() ensures jwt_secret ≥ 32 chars, auth requires storage DSN
- **Tests**: 28 auth unit tests + 3 integration tests (CI-only) + 7 API auth tests + 3 config tests

### What's Next

**M4_01 — Alert Engine & Notifications:**
- Alert evaluation engine with threshold comparison, state machine (OK→WARNING→CRITICAL→OK), hysteresis, cooldown
- Default rules from PGAM thresholds (14) + 6 new rules
- Notification channels: Telegram, Slack, Email, Webhook
- Dispatcher with routing and retry logic
- API endpoints for alerts management
- Alert history table

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
- **Claude Code v2.1.63:** Bash works on Windows. EINVAL temp path bug is FIXED. Agents run go build/test/lint/commit directly. No more hybrid workflow.
- **One chat per iteration** in Claude.ai for planning; Claude Code for implementation
- **Single Sonnet for focused tasks; Agent Teams for multi-file iterations**

### Known Environment Issues

| Issue | Status | Workaround |
|---|---|---|
| ~~Claude Code bash EINVAL~~ | **FIXED in v2.1.63** | No workaround needed — agents run bash directly |
| LF/CRLF warnings on git add | Cosmetic | `.gitattributes` with `* text=auto eol=lf` |
| Docker Desktop unavailable | BIOS virtualization disabled | Integration tests CI-only |

---

## 7. IMPLEMENTATION PATTERNS

### Collector Pattern
```go
type XxxCollector struct { Base }
func NewXxxCollector(instanceID string, v version.PGVersion) *XxxCollector
func (c *XxxCollector) Collect(ctx, conn, ic) ([]MetricPoint, error) {
    qCtx, cancel := queryContext(ctx); defer cancel()
    // scan → buildMetrics(scanned)
}
func (c *XxxCollector) buildMetrics(rows []xxxRow) []MetricPoint { /* pure */ }
```

### Auth Pattern
```go
// Auth toggle in Routes()
if s.authCfg.Enabled {
    r.Use(auth.RequireAuth(s.jwtService, writeErrorRaw))
    r.Use(auth.RequireRole("viewer", writeErrorRaw))
}
// Public routes (login, refresh) in separate Group() outside auth middleware
```

### Orchestrator Pattern
```go
high   (10s):  connections, cache, wait_events, lock_tree, long_transactions
medium (60s):  replication_*, statements_*, checkpoint, vacuum/cluster/analyze/index/basebackup/copy progress
low    (300s): server_info, database_sizes, settings, extensions, transactions, io_stats
```

### Metric Naming (complete list)
```
pgpulse.server.*             — uptime, is_in_recovery, is_in_backup, pg_version_num
pgpulse.connections.*        — active, idle, idle_in_transaction, waiting, total, max, utilization_ratio
pgpulse.cache.hit_ratio
pgpulse.transactions.*       — commit_ratio, deadlocks (label: db_name)
pgpulse.database.*           — size_bytes (label: db_name)
pgpulse.extensions.*         — pgss_installed, pgss_fill_pct, pgss_stats_reset_unix
pgpulse.replication.*        — lag.*, slot.*, active_replicas, replica.*, wal_receiver.*
pgpulse.progress.*           — vacuum.*, cluster.*, analyze.*, create_index.*, basebackup.*, copy.*
pgpulse.checkpoint.*         — timed, requested, write/sync_time_ms, buffers_written, restartpoints_*
pgpulse.bgwriter.*           — buffers_clean, maxwritten_clean, buffers_alloc, buffers_backend*
pgpulse.io.*                 — reads, read_time, writes, write_time, extends, hits, evictions, fsyncs*
pgpulse.statements.*         — max, fill_pct, track, track_io_timing, count, stats_reset_age_seconds
pgpulse.statements.top.*     — total_time_ms, io_time_ms, cpu_time_ms, calls, rows, avg_time_ms
pgpulse.wait_events.*        — count (labels: wait_event_type, wait_event), total_backends
pgpulse.locks.*              — blocker_count, blocked_count, max_chain_depth
pgpulse.long_transactions.*  — count, oldest_seconds (label: type=active|waiting)
```

---

## 8. BUILD & TEST STATUS

| Check | Result |
|-------|--------|
| `go build ./...` | ✅ Pass |
| `go vet ./...` | ✅ Pass |
| `golangci-lint run` | ✅ 0 issues |
| Auth unit tests (28) | ✅ All pass |
| Auth integration tests (3) | ⏭️ Skipped (no Docker) |
| API tests (31 total: 24 prior + 7 new) | ✅ All pass |
| Config tests (10 total: 7 prior + 3 new) | ✅ All pass |
| Storage tests (15) | ✅ All pass |
| Orchestrator tests (9) | ✅ All pass |
| Collector tests (all prior) | ✅ All pass |

---

## 9. HOW TO RESTORE THIS SAVE POINT

### Option A: Continue in Same Claude.ai Project
1. Open new chat in the PGPulse project
2. Upload this save point file
3. Say: "Restoring from save point. M3 is complete. Next is M4_01 (Alert Engine)."

### Option B: New Claude.ai Project / Different Tool
1. Clone: `git clone https://github.com/ios9000/PGPulse_01.git`
2. Read this file for complete context
3. Key interfaces: `internal/collector/collector.go`, `internal/auth/`, `internal/api/server.go`, `internal/config/config.go`, `internal/storage/pgstore.go`
4. `go test ./...` — all tests pass

### Option C: Complete Disaster Recovery
If the repo is lost:
1. This save point contains all interfaces, patterns, and architectural decisions
2. PGAM SQL queries are in PGAM_FEATURE_AUDIT.md (76 queries documented)
3. Rebuild order: version/ → collector/ → config/ → orchestrator/ → storage/ → auth/ → api/
4. All design decisions documented in section 2 (D1–D62)
