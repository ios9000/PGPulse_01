# Session: 2026-03-01 — M3_01 Auth & Security

## Goal

Add JWT-based authentication and role-based access control to PGPulse REST API.
Replace auth stub middleware with real JWT validation. Two roles: admin (full access),
viewer (read-only). Auth toggle: disabled by default, enabled via config.

## Session Flow

### Phase 1: Planning (Claude.ai)

1. Restored context from HANDOFF_M2_03_to_M3_01.md
2. Worked through 7 design questions from the handoff:
   - User storage → same PGPulse DB, same pool
   - Initial admin → config file seed (on first run when users table empty)
   - Token structure → HS256 JWT with uid/usr/role/type claims
   - Refresh token → stateless (longer-lived JWT, no DB table)
   - Rate limiting → in-memory per-IP, 10 attempts / 15 min window
   - CSRF → deferred to M5 (no browser client yet)
   - Health endpoint → stays public
3. Three additional design questions resolved via developer input:
   - Auth toggle: enabled/disabled (toggle wins over always-require)
   - User management: minimal (config seed + login only, no CRUD endpoints)
   - User repository location: internal/auth/store.go (auth owns its storage)
4. Produced three deliverables: M3_01_requirements.md, M3_01_design.md, M3_01_team-prompt.md

### Phase 2: Implementation (Claude Code — single Sonnet session)

5. Agent created all files per design spec
6. Developer ran `go test` → chi panic: "all middlewares must be defined before routes on a mux"
7. Root cause: `r.Use(authStubMiddleware)` called after `r.Get("/health")` on same mux
8. Fix: wrapped auth-disabled routes in `r.Group()` so middleware has its own sub-router
9. All tests green after fix
10. Committed as 4765b84, pushed to origin

## Agent Configuration

- Planning: Claude.ai (Opus 4.6) in PGPulse project
- Implementation: Claude Code single Sonnet session
- Duration: ~1.5 hours (planning) + ~1.5 hours (implementation + fix)

## Files Created

| File | Lines | Purpose |
|------|-------|---------|
| internal/auth/store.go | ~80 | User, UserStore interface, PGUserStore (pgxpool) |
| internal/auth/password.go | ~15 | HashPassword, CheckPassword (bcrypt) |
| internal/auth/rbac.go | ~30 | RoleAdmin/RoleViewer, HasRole, ValidRole |
| internal/auth/jwt.go | ~120 | JWTService: GenerateTokenPair, ValidateToken, Claims |
| internal/auth/ratelimit.go | ~90 | Sliding-window RateLimiter, ClientIP |
| internal/auth/middleware.go | ~80 | RequireAuth, RequireRole, ClaimsFromContext |
| internal/auth/jwt_test.go | — | 6 tests |
| internal/auth/password_test.go | — | 3 tests |
| internal/auth/rbac_test.go | — | 6 tests |
| internal/auth/ratelimit_test.go | — | 6 tests |
| internal/auth/middleware_test.go | — | 6 tests |
| internal/auth/store_test.go | — | 3 integration tests (//go:build integration) |
| internal/api/auth.go | ~100 | handleLogin, handleRefresh, handleMe |
| internal/api/auth_test.go | — | 7 tests |
| internal/storage/migrations/003_users.sql | ~12 | Users table DDL |
| **Total new files: 15** | | |

## Files Modified

| File | Change |
|------|--------|
| internal/config/config.go | Added AuthConfig + InitialAdminConfig structs |
| internal/config/load.go | Added validateAuth() |
| internal/config/config_test.go | +3 auth config tests |
| internal/api/server.go | New fields, updated New(), restructured Routes() |
| internal/api/middleware.go | UserFromContext checks JWT claims first |
| internal/api/response.go | Added writeErrorRaw callback |
| internal/api/helpers_test.go | Updated newTestServer for new constructor signature |
| cmd/pgpulse-server/main.go | Extracted startServer(), wires auth + seeding |
| configs/pgpulse.example.yml | Added auth section |
| go.mod | Added golang-jwt/jwt/v5 v5.2.2, promoted x/crypto |

## Test Results

| Suite | Tests | Result |
|-------|-------|--------|
| internal/auth (unit) | 28 | ✅ All pass |
| internal/auth (integration) | 3 | ⏭️ Skipped (no Docker) |
| internal/api (all) | 31 (24 prior + 7 new) | ✅ All pass |
| internal/config | 10 (7 prior + 3 new) | ✅ All pass |
| go build ./... | — | ✅ Clean |
| golangci-lint run | — | ✅ 0 issues |

## Architecture Decisions

| Decision | Rationale |
|----------|-----------|
| errorWriter callback pattern | auth middleware needs to write JSON errors matching API envelope, but auth must not import api. Callback func(w, code, errCode, msg) keeps dependency one-directional: api→auth |
| Stateless refresh tokens | No extra DB table, no cleanup jobs. Trade-off: no revocation until M5 |
| Rate limiter records failures only | Successful logins don't count, so legitimate users aren't locked out |
| Same error for wrong user / wrong password | Prevents username enumeration |
| r.Group() for auth-disabled routes | chi requires Use() before Get() on same mux; Group() creates isolated sub-router |

## Bug Found & Fixed

**Chi middleware ordering panic.** In Routes(), the `else` branch (auth disabled) called
`r.Use(authStubMiddleware)` after `r.Get("/health")` was already registered on the parent
mux. Chi enforces middleware-before-routes. Fix: wrap auth-disabled routes in `r.Group()`.

## Commit

```
4765b84 feat(auth): add JWT authentication and RBAC (M3_01)
29 files changed, 3054 insertions(+), 27 deletions(-)
```

## Housekeeping Items Noted

1. M2 storage files (migrate.go, pool.go) still uncommitted — need separate commit
2. Iteration folder typo: `auth-securit` → `auth-security`
3. LF/CRLF warnings — .gitattributes not yet added
4. New naming convention adopted: iteration deliverables prefixed with iteration ID
   (e.g. M3_01_requirements.md)

## PGAM Queries Ported

None — PGAM had zero authentication (`_auth.php` was empty). M3_01 is entirely new functionality.

## Not Done / Deferred

- [ ] User CRUD endpoints (POST/DELETE /auth/users) — future
- [ ] Password change endpoint — future
- [ ] Token revocation / blacklist — M5
- [ ] CSRF tokens — M5 (when browser client exists)
- [ ] OAuth / OIDC / SSO — future
- [ ] API key authentication — future
- [ ] Audit logging of auth events — future
