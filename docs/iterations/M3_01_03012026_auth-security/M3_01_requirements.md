# M3_01 — Authentication & RBAC: Requirements

**Iteration:** M3_01  
**Date:** 2026-03-01  
**Milestone:** M3 — Auth & Security  
**Predecessor:** M2_03 (REST API complete)  
**Session type:** Single Claude Code session (focused scope)

---

## 1. Objective

Add JWT-based authentication and role-based access control to PGPulse.
After M3_01:

- Protected endpoints require a valid Bearer token.
- Two roles: **admin** (full access) and **viewer** (read-only).
- Auth can be disabled via config toggle (preserves current open behavior).
- Initial admin user is seeded from config on first run.
- Login rate limiting prevents brute-force attacks.

## 2. Functional Requirements

### 2.1 Authentication

| ID | Requirement |
|----|-------------|
| AUTH-01 | System SHALL support JWT-based authentication using HS256 signing |
| AUTH-02 | Access tokens SHALL have configurable TTL (default 24h) |
| AUTH-03 | Refresh tokens SHALL be stateless JWTs with configurable TTL (default 7 days) |
| AUTH-04 | Login endpoint SHALL accept username + password, return access + refresh tokens |
| AUTH-05 | Refresh endpoint SHALL accept a valid refresh token, return a new access token |
| AUTH-06 | /auth/me endpoint SHALL return current user info (id, username, role) |
| AUTH-07 | Passwords SHALL be stored as bcrypt hashes (cost 12) |
| AUTH-08 | JWT signing secret SHALL come from config (`auth.jwt_secret`), not hardcoded |

### 2.2 Role-Based Access Control

| ID | Requirement |
|----|-------------|
| RBAC-01 | Two roles: `admin` (full access) and `viewer` (read-only) |
| RBAC-02 | All GET endpoints under /api/v1/ (except health and auth) require minimum `viewer` role |
| RBAC-03 | All mutation endpoints (POST/PUT/DELETE) require `admin` role |
| RBAC-04 | Unauthenticated requests to protected endpoints return 401 |
| RBAC-05 | Authenticated requests with insufficient role return 403 |
| RBAC-06 | Health endpoint (`GET /api/v1/health`) remains public — no auth required |

### 2.3 Auth Toggle

| ID | Requirement |
|----|-------------|
| TOGGLE-01 | When `auth.enabled=false` (default), all endpoints are open (current authStub behavior) |
| TOGGLE-02 | When `auth.enabled=true`, JWT middleware protects routes per RBAC rules |
| TOGGLE-03 | Auth requires a storage DSN. If auth.enabled=true and storage.dsn is empty, startup SHALL fail with a clear error |

### 2.4 User Seeding

| ID | Requirement |
|----|-------------|
| SEED-01 | On startup, if auth.enabled=true and users table is empty, seed from `auth.initial_admin` config |
| SEED-02 | Seeded password is bcrypt-hashed before insert |
| SEED-03 | Startup log SHALL warn: "Created initial admin user — change password immediately" |
| SEED-04 | If users table is NOT empty, seeding is skipped silently |
| SEED-05 | If auth.enabled=true but initial_admin config is missing and table is empty, startup SHALL fail with a clear error |

### 2.5 Rate Limiting

| ID | Requirement |
|----|-------------|
| RATE-01 | Login endpoint limited to 10 failed attempts per IP per 15-minute window |
| RATE-02 | Exceeded limit returns 429 Too Many Requests with Retry-After header |
| RATE-03 | Rate limiter is in-memory (resets on server restart — acceptable) |
| RATE-04 | Rate limiter applies BEFORE handler execution (middleware) |
| RATE-05 | Successful logins do NOT count against the limit |

### 2.6 User Storage

| ID | Requirement |
|----|-------------|
| STORE-01 | Users table in PGPulse's own database (same pool as metrics) |
| STORE-02 | Migration `003_users.sql` creates the table |
| STORE-03 | UserStore lives in `internal/auth/store.go` (auth owns its storage) |
| STORE-04 | UserStore interface enables testability via mocks |

## 3. Non-Functional Requirements

| ID | Requirement |
|----|-------------|
| NFR-01 | No new external services (no Redis, no separate auth DB) |
| NFR-02 | Single new Go dependency: `github.com/golang-jwt/jwt/v5` |
| NFR-03 | bcrypt via `golang.org/x/crypto/bcrypt` (likely already transitive) |
| NFR-04 | All existing tests continue to pass (no regressions) |
| NFR-05 | Auth package has zero imports from internal/api or internal/collector |
| NFR-06 | No CSRF in M3 (deferred to M5 — no browser client yet) |
| NFR-07 | No token revocation/blacklist in M3 (deferred) |
| NFR-08 | JWT secret minimum length: 32 characters. Startup fails if shorter. |

## 4. Out of Scope for M3_01

- User CRUD endpoints (POST/DELETE /auth/users) — future iteration
- CLI tool for user management — future
- Password change endpoint — future
- Token revocation / blacklist — future (M5)
- CSRF tokens — future (M5, when browser client exists)
- OAuth / OIDC / SSO — future
- API key authentication — future
- Audit logging of auth events — future
- Multi-factor authentication — future

## 5. Affected Files

### New Files
| File | Purpose |
|------|---------|
| `internal/auth/jwt.go` | JWTService: Generate, Validate (access + refresh) |
| `internal/auth/password.go` | Hash, Compare (bcrypt) |
| `internal/auth/rbac.go` | Role constants, RequireRole middleware |
| `internal/auth/ratelimit.go` | In-memory per-IP rate limiter |
| `internal/auth/middleware.go` | RequireAuth chi middleware |
| `internal/auth/store.go` | UserStore interface + PGUserStore |
| `internal/auth/jwt_test.go` | JWT generation/validation tests |
| `internal/auth/password_test.go` | bcrypt tests |
| `internal/auth/rbac_test.go` | Role permission tests |
| `internal/auth/ratelimit_test.go` | Rate limiter tests |
| `internal/auth/middleware_test.go` | Auth middleware tests |
| `internal/auth/store_test.go` | UserStore mock + tests |
| `internal/api/auth.go` | Login, refresh, me handlers |
| `internal/api/auth_test.go` | Auth handler tests |
| `internal/storage/migrations/003_users.sql` | Users table DDL |

### Modified Files
| File | Change |
|------|--------|
| `internal/config/config.go` | Add AuthConfig struct |
| `internal/config/load.go` | Add auth config validation |
| `internal/config/config_test.go` | Add auth config tests |
| `internal/api/server.go` | Accept AuthService, conditional middleware wiring |
| `internal/api/middleware.go` | Remove authStubMiddleware (or keep for disabled mode) |
| `cmd/pgpulse-server/main.go` | Wire auth service, run user seeding |
| `configs/pgpulse.example.yml` | Add auth section |
| `go.mod` | Add golang-jwt/jwt/v5, x/crypto |

## 6. Acceptance Criteria

- [ ] `go build ./...` compiles cleanly
- [ ] `golangci-lint run` reports 0 issues
- [ ] All existing tests pass (no regressions)
- [ ] New auth tests: ≥ 30 tests covering JWT, password, RBAC, rate limit, middleware, handlers
- [ ] With auth.enabled=false: all endpoints open, current behavior preserved
- [ ] With auth.enabled=true: unauthenticated GET /instances → 401
- [ ] With auth.enabled=true: login with correct creds → 200 + tokens
- [ ] With auth.enabled=true: login with wrong creds → 401
- [ ] With auth.enabled=true: valid viewer token + GET /instances → 200
- [ ] With auth.enabled=true: valid viewer token + POST (future) → 403
- [ ] Rate limit: 11th failed login from same IP → 429
- [ ] Empty users table + auth enabled → admin seeded from config
- [ ] Non-empty users table → no seeding
- [ ] JWT secret < 32 chars → startup error
