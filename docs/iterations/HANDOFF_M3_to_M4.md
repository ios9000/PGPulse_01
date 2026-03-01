# PGPulse — Iteration Handoff: M3_01 → M4_01

> **Purpose:** Upload this file when starting the next Claude.ai chat.
> Contains EVERYTHING needed to begin M4_01 without re-discovery.
> **Created:** 2026-03-01 (end of M3_01)

---

## DO NOT RE-DISCUSS

These decisions are final. The new chat must not revisit them:

1. **Stack**: Go 1.24.0, pgx v5.8.0, chi v5.2.5, golang-jwt/jwt/v5 v5.2.2, koanf, slog, testcontainers-go
2. **Architecture**: Single binary, version-adaptive SQL via Gate pattern, Collector interface with InstanceContext
3. **Granularity**: One file = one collector = one struct implementing Collector
4. **Module ownership**: Collector Agent owns internal/collector/* and internal/version/*
5. **Claude Code bash**: Fixed in v2.1.63. Agents run go build/test/lint/commit directly. No hybrid workflow needed.
6. **Go module path**: `github.com/ios9000/PGPulse_01`
7. **Project path**: `C:\Users\Archer\Projects\PGPulse_01`
8. **PG version support**: 14, 15, 16, 17 (18 optional)
9. **Monitoring user**: pg_monitor role, never superuser
10. **OS metrics**: via Go agent (M6), NEVER via COPY TO PROGRAM
11. **Metric naming**: `pgpulse.<category>.<metric>` with labels as map[string]string
12. **Statement timeout**: 5s for live dashboard collectors via context.WithTimeout
13. **golangci-lint**: v2.10.1. Config requires `version: "2"` field.
14. **Docker Desktop**: not available on developer workstation. Integration tests run in CI only.
15. **InstanceContext SSoT**: Orchestrator queries `pg_is_in_recovery()` once per cycle, passes `InstanceContext{IsRecovery: bool}` to all collectors.
16. **Collector registration**: Explicit in runner.go buildCollectors(), NOT via init() auto-registration.
17. **Orchestrator architecture**: Goroutine per interval group (high=10s, medium=60s, low=300s). Collectors within a group run sequentially sharing one pgx.Conn.
18. **Server inventory**: YAML config only for M2-M3. Database-backed inventory deferred.
19. **Storage approach**: Plain PG first, TimescaleDB-ready. Schema works on plain PG; hypertable creation is conditional.
20. **Migration runner**: Embedded SQL files via go:embed + simple runner. No external migration framework.
21. **Connection resilience**: Skip cycle on error, no auto-reconnect.
22. **LogStore fallback**: When storage.dsn is empty, use LogStore (log-only mode). PGStore when DSN configured.
23. **Claude Code**: v2.1.63. Bash works on Windows. Agents run build/test/lint/commit. Single Sonnet for focused tasks; Agent Teams for multi-file iterations.
24. **pgss availability check**: `pgssAvailable()` helper in base.go.
25. **Checkpoint version gate**: PG ≤ 16 → pg_stat_bgwriter. PG ≥ 17 → pg_stat_checkpointer CROSS JOIN pg_stat_bgwriter.
26. **Lock tree**: pg_blocking_pids() + Go BFS graph. Summary metrics only; full tree deferred to API.
27. **Storage: CopyFrom for writes**: Batch insert via COPY protocol. 10s write timeout, 30s query timeout.
28. **Storage: buildQuery()**: Pure function for dynamic WHERE construction. Supports instance_id, metric prefix, time range, labels @> jsonb, LIMIT.
29. **Storage pool**: pgxpool MaxConns=5, application_name="pgpulse_storage". For PGPulse's own DB only.
30. **Conditional migrations**: 002_timescaledb.sql skipped when use_timescaledb=false. MigrateOptions struct controls this.
31. **REST API response format**: JSON envelope `{"data":..., "meta":...}`. Errors: `{"error":{"code":"...","message":"..."}}`.
32. **CSV export**: GET /api/v1/instances/{id}/metrics?format=csv OR Accept: text/csv. Labels as JSON string.
33. **Auth toggle**: auth.enabled=false (default) → authStubMiddleware, all open. auth.enabled=true → JWT middleware.
34. **Graceful shutdown order**: HTTP server → Orchestrator → Store.
35. **Pinger interface**: Abstracts pgxpool.Pool.Ping() for health endpoint testability.
36. **Design docs rule**: Always write full paths, never use "..." or abbreviations in paths.
37. **JWT tokens**: HS256, access (24h default) + refresh (7d default), stateless. Claims: uid, usr, role, type. Secret from config (min 32 chars).
38. **Refresh tokens**: Stateless longer-lived JWT. No DB-stored refresh tokens. No revocation until M5.
39. **RBAC**: admin (full access) and viewer (read-only). roleHierarchy map, HasRole() level comparison.
40. **User storage**: users table in PGPulse's own DB. auth package owns its store (internal/auth/store.go). UserStore interface for testability.
41. **Initial admin**: Seeded from config on first run when users table is empty. bcrypt-hashed. Log warns to change password.
42. **Rate limiting**: In-memory per-IP, 10 failed attempts / 15 min window. Login endpoint only. Resets on restart.
43. **CSRF**: Deferred to M5 (no browser client).
44. **errorWriter callback pattern**: auth middleware accepts func(w, code, errCode, msg) to write JSON errors without importing api package. Keeps dependency one-directional: api→auth.
45. **Chi middleware ordering**: Use() must come before Get()/Post() on same mux. Auth-disabled routes wrapped in r.Group() to avoid panic.
46. **Auth requires storage**: auth.enabled=true + empty storage.dsn → startup error.
47. **Iteration deliverable naming**: Prefix files with iteration ID (e.g. M3_01_requirements.md, M4_01_design.md).

---

## What Exists After M3_01

### Repository Structure

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
│   │   ├── middleware.go          ← requestID, logger, recoverer, CORS, authStub, UserFromContext (checks JWT first)
│   │   ├── health.go              ← GET /api/v1/health (always public)
│   │   ├── instances.go           ← GET /api/v1/instances, GET /api/v1/instances/{id}
│   │   ├── metrics.go             ← GET /api/v1/instances/{id}/metrics (JSON + CSV)
│   │   ├── auth.go                ← [NEW M3] POST /auth/login, POST /auth/refresh, GET /auth/me
│   │   ├── helpers_test.go        ← mockStore, mockPinger, mockUserStore, newTestServer
│   │   ├── health_test.go         ← 5 tests
│   │   ├── instances_test.go      ← 5 tests
│   │   ├── metrics_test.go        ← 10 tests
│   │   ├── middleware_test.go     ← 4 tests
│   │   └── auth_test.go           ← [NEW M3] 7 tests
│   │
│   ├── auth/                      ← [NEW M3 — entire package]
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
│   │   ├── registry.go            ← RegisterCollector(), CollectAll()
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
│   │   ├── checkpoint.go          ← Checkpoint/BGWriter (stateful)
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
│   │   │   └── 003_users.sql      ← [NEW M3] Users table with role CHECK constraint
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
│   └── iterations/
│       ├── M1_01 through M1_05 folders
│       ├── M2_01_02272026_config-orchestrator/
│       ├── M2_02_02272026_storage-layer/
│       ├── M2_03_02272026_rest-api/
│       └── M3_01_03012026_auth-security/
│           ├── M3_01_requirements.md
│           ├── M3_01_design.md
│           └── M3_01_team-prompt.md
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

type MetricQuery struct {
    InstanceID string
    Metric     string
    Labels     map[string]string
    Start      time.Time
    End        time.Time
    Limit      int
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
func RequireAuth(jwtSvc *JWTService, errorWriter func(w http.ResponseWriter, code int, errCode, message string)) func(http.Handler) http.Handler
func RequireRole(requiredRole string, errorWriter func(w http.ResponseWriter, code int, errCode, message string)) func(http.Handler) http.Handler

// ratelimit.go
type RateLimiter struct { /* mu, attempts, maxAttempts, window */ }
func NewRateLimiter(maxAttempts int, window time.Duration) *RateLimiter
func (rl *RateLimiter) Allow(ip string) bool
func (rl *RateLimiter) RecordFailure(ip string)
func (rl *RateLimiter) RetryAfter(ip string) int
func ClientIP(r *http.Request) string
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

type InitialAdminConfig struct {
    Username string `koanf:"username"`
    Password string `koanf:"password"`
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

---

## Build & Test Status After M3_01

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

## Next Task: M4_01 — Alert Engine & Notifications

### Goal

Build an alert evaluation engine that processes collected metrics against configurable
threshold rules, with notification delivery via Telegram, Slack, Email, and Webhook.

### What to Build (from Strategy doc M4 section)

| Component | Purpose |
|-----------|---------|
| `internal/alert/evaluator.go` | AlertEvaluator: threshold comparison, state machine (OK→WARNING→CRITICAL→OK), hysteresis, cooldown |
| `internal/alert/rules.go` | Default alert rules — 14 from PGAM thresholds + 6 new |
| `internal/alert/notifier/telegram.go` | Telegram Bot API notification |
| `internal/alert/notifier/slack.go` | Slack Webhook notification |
| `internal/alert/notifier/email.go` | SMTP email notification |
| `internal/alert/notifier/webhook.go` | Generic HTTP webhook notification |
| `internal/alert/dispatcher.go` | Routes alerts to configured channels, retry with backoff |
| `internal/api/alerts.go` | GET /api/v1/alerts, GET /api/v1/alerts/rules, POST /api/v1/alerts/rules, POST /api/v1/alerts/test |
| Migration(s) | Alert rules table, alert history table |
| Config | Alert section in pgpulse.yml (channels, default rules) |
| Tests | Evaluator state machine, hysteresis, cooldown, notifier mocks, dispatcher, API |

### PGAM Alert Thresholds to Port

From PGAM_FEATURE_AUDIT.md Section 6 — these are inline HTML color rules:

| Metric | Yellow | Red |
|--------|--------|-----|
| Database wraparound % | > 20% | > 50% |
| Multixact wraparound % | > 20% | > 50% |
| DB connection limit usage | > 80% | ≥ 99% |
| Per-DB cache hit | < 90% | — |
| Commit ratio | < 90% | — |
| Replication slot inactive | — | active='f' |
| Long transaction (active/waiting) | > 1 min | ≥ 5 min |
| Lock tree root with blocked | — | blocked > 0 |
| Object size | > 50 GB | > 100 GB |
| System catalog size | > 100 MB | ≥ 1 GB |
| Table bloat ratio | > 2× | > 50× |
| pg_stat_statements fill | ≥ 95% | — |
| stats_reset age | ≥ 1 day | — |
| track_io_timing=off | yellow | — |

Plus 6 new rules from Strategy doc:
- Replication lag > 1MB WARNING, > 100MB CRITICAL
- WAL spike > 3× baseline WARNING
- Query regression > 2× mean WARNING
- Disk forecast < 7 days CRITICAL
- Connection pool saturation > 90% WARNING
- Schema DDL changes INFO

### Key Design Questions for M4_01 Planning

1. **Alert rule storage** — YAML config only, or DB-backed (editable via API)?
2. **Alert history** — new table for fired alerts? How long to retain?
3. **Evaluator integration** — called after each collector cycle in orchestrator, or separate goroutine polling storage?
4. **State machine persistence** — in-memory (resets on restart) or DB-backed?
5. **Notification channels** — configured per-rule or global?
6. **Hysteresis** — how many consecutive violations before firing? Configurable per rule?
7. **Cooldown** — minimum time between repeat notifications for same alert?
8. **Network access** — Telegram/Slack/Email require outbound HTTP/SMTP. Does the dev environment have outbound access?
9. **Iteration splitting** — M4 is large. Split into M4_01 (evaluator + rules), M4_02 (notifiers + dispatcher), M4_03 (API + wiring)?

### Session Type

Agent Teams (3 agents) — evaluator, notifiers, and tests are independent workstreams.
Agents can now run `go build`, `go test`, `golangci-lint`, `git commit` directly (bash fixed in v2.1.63).
Team prompt should end with: "Run `go test -race ./...` and `golangci-lint run` — fix any issues before declaring done."

---

## Known Issues

1. **Docker Desktop unavailable** — integration tests CI-only
2. **No auto-reconnect** — orchestrator skips cycle on connection error
3. **No retention cleanup** — metrics accumulate indefinitely
4. **Storage DB failure mid-run** — Write() errors logged, metrics lost for that cycle
5. **DSN parsing for Host/Port** — key-value DSN format returns empty host/port. DB-backed inventory deferred.
6. **No token revocation** — stateless JWTs, no blacklist. Deferred to M5.
7. **Rate limiter memory** — unbounded IP map under DDoS. Acceptable for monitoring tool.

### Housekeeping (do before starting M4)

- [ ] Commit uncommitted M2 storage files: `git add internal/storage/ && git commit -m "feat(storage): commit M2 storage layer files"`
- [ ] Fix iteration folder typo: `git mv docs/iterations/M3_01_03012026_auth-securit docs/iterations/M3_01_03012026_auth-security`
- [ ] Add .gitattributes: `echo "* text=auto eol=lf" > .gitattributes`
- [ ] Copy M3_01_session-log.md into docs/iterations/M3_01_03012026_auth-security/
- [ ] Update .claude/CLAUDE.md: remove hybrid workflow note, add bash fixed in v2.1.63

---

## Milestone Progress

| Milestone | Iteration | Scope | Status |
|-----------|-----------|-------|--------|
| M1 | M1_01–M1_05 | Collectors (instance, replication, progress, statements, locks) | ✅ Done |
| M2 | M2_01 | Config + Orchestrator | ✅ Done |
| M2 | M2_02 | Storage Layer + Migrations | ✅ Done |
| M2 | M2_03 | REST API + Wiring | ✅ Done |
| **M2** | | **Milestone complete** | **✅ Done** |
| M3 | M3_01 | Auth & RBAC | ✅ Done |
| **M3** | | **Milestone complete** | **✅ Done** |
| **M4** | **M4_01** | **Alert Engine** | **🔲 Next** |

---

## Environment Reference

| Tool | Version | Notes |
|------|---------|-------|
| Go | 1.24.0 windows/amd64 | |
| golangci-lint | 2.10.1 | v2 config |
| Claude Code | 2.1.63 | Bash works on Windows (fixed!) |
| Docker Desktop | Not installed | Integration tests → CI only |
| Git | 2.52.0 | |
| chi | v5.2.5 | |
| golang-jwt/jwt | v5.2.2 | Added in M3_01 |
| x/crypto | (transitive→direct) | bcrypt, promoted in M3_01 |
| koanf | v2 | |
| pgx | v5.8.0 | Includes pgxpool |
