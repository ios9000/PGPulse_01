# M3_01 — Authentication & RBAC: Claude Code Prompt

**Iteration:** M3_01  
**Date:** 2026-03-01  
**Path:** docs/iterations/M3_01_03012026_auth-security/team-prompt.md  
**Session type:** Single Claude Code session (Sonnet) — focused scope

---

## Prompt for Claude Code

Paste the following into Claude Code after updating CLAUDE.md's "Current Iteration" section.

---

```
Implement JWT authentication and RBAC for PGPulse.
Read CLAUDE.md for project context, then read
docs/iterations/M3_01_03012026_auth-security/design.md for full specifications.

⚠️ PLATFORM NOTE: Cannot run bash on Windows. Create all files only.
Developer will run go build/test/commit manually.
At the end, list ALL files created and modified.

## Overview

Add auth to PGPulse REST API. When auth.enabled=true, endpoints require
Bearer JWT tokens. Two roles: admin (full access) and viewer (read-only).
When auth.enabled=false (default), current open behavior is preserved.

## Task Order (dependency-driven)

### Step 1: Config + Migration

1. UPDATE internal/config/config.go:
   - Add AuthConfig struct with fields: Enabled (bool), JWTSecret (string),
     AccessTokenTTL (time.Duration), RefreshTokenTTL (time.Duration),
     BcryptCost (int), InitialAdmin (*InitialAdminConfig)
   - Add InitialAdminConfig struct: Username, Password (both string)
   - Add Auth AuthConfig field to Config struct

2. UPDATE internal/config/load.go:
   - In validate(): if Auth.Enabled, require Storage.DSN non-empty,
     JWTSecret >= 32 chars. Set defaults: AccessTokenTTL=24h,
     RefreshTokenTTL=168h, BcryptCost=12

3. CREATE internal/storage/migrations/003_users.sql:
   ```sql
   CREATE TABLE IF NOT EXISTS users (
       id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
       username      TEXT NOT NULL UNIQUE,
       password_hash TEXT NOT NULL,
       role          TEXT NOT NULL DEFAULT 'viewer'
                     CHECK (role IN ('admin', 'viewer')),
       created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
       updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
   );
   CREATE INDEX idx_users_username ON users (username);
   ```

4. UPDATE configs/pgpulse.example.yml — add auth section

### Step 2: Auth Package (internal/auth/)

All files in internal/auth/. This package has ZERO imports from internal/api,
internal/collector, or internal/orchestrator.

5. CREATE internal/auth/password.go:
   - HashPassword(password string, cost int) (string, error) — bcrypt
   - CheckPassword(password, hash string) error — bcrypt compare

6. CREATE internal/auth/rbac.go:
   - Constants: RoleAdmin = "admin", RoleViewer = "viewer"
   - roleHierarchy map: viewer=1, admin=2
   - HasRole(userRole, requiredRole string) bool — level comparison
   - ValidRole(role string) bool

7. CREATE internal/auth/store.go:
   - ErrUserNotFound sentinel error
   - User struct: ID int64, Username, PasswordHash, Role string,
     CreatedAt, UpdatedAt time.Time
   - UserStore interface: GetByUsername, Create, Count
   - PGUserStore struct with pgxpool.Pool
   - NewPGUserStore(pool) constructor
   - Implement all three methods with parameterized SQL ($1, $2)

8. CREATE internal/auth/jwt.go:
   - TokenType string type: TokenAccess, TokenRefresh constants
   - Claims struct embedding jwt.RegisteredClaims + UserID(int64),
     Username(string), Role(string), Type(TokenType)
   - TokenPair struct: AccessToken, RefreshToken (string), ExpiresIn (int64)
   - JWTService struct: secret []byte, accessTokenTTL, refreshTokenTTL
   - NewJWTService(secret string, accessTTL, refreshTTL time.Duration)
   - GenerateTokenPair(user *User) (*TokenPair, error)
   - GenerateAccessToken(user *User) (string, error)
   - ValidateToken(tokenString string) (*Claims, error)
   - AccessTokenTTL() time.Duration — accessor
   - Internal: generateToken(user, tokenType, now, ttl) (string, error)
   - Signing: HS256. Issuer: "pgpulse"

9. CREATE internal/auth/ratelimit.go:
   - RateLimiter struct: mu sync.Mutex, attempts map[string][]time.Time,
     maxAttempts int, window time.Duration
   - NewRateLimiter(maxAttempts int, window time.Duration)
   - Allow(ip string) bool — check without recording
   - RecordFailure(ip string) — record failed attempt
   - RetryAfter(ip string) int — seconds until window expires
   - pruneExpired(ip string) — remove entries older than window
   - ClientIP(r *http.Request) string — extract from X-Forwarded-For,
     X-Real-IP, or RemoteAddr

10. CREATE internal/auth/middleware.go:
    - claimsContextKey (unexported contextKey type)
    - ClaimsFromContext(ctx) *Claims — retrieve from context
    - RequireAuth(jwtSvc, errorWriter callback) middleware — extracts
      Bearer token, validates, checks type==access, sets claims in ctx
    - RequireRole(requiredRole, errorWriter callback) middleware — checks
      claims role via HasRole
    - errorWriter signature: func(w http.ResponseWriter, code int, errCode, message string)
      This callback pattern avoids auth importing api package.

### Step 3: API Integration

11. UPDATE internal/api/response.go:
    - Add writeErrorRaw function matching errorWriter callback signature
      (just calls writeError)

12. CREATE internal/api/auth.go:
    - loginRequest struct: Username, Password (string, json tags)
    - refreshRequest struct: RefreshToken (string, json tag "refresh_token")
    - userResponse struct: ID(int64), Username, Role (string, json tags)
    - handleLogin: decode body, validate non-empty, look up user,
      check password, record failure on bad creds, generate token pair
    - handleRefresh: decode body, validate refresh token, look up fresh user,
      generate new access token only
    - handleMe: get claims from context, return userResponse
    - CRITICAL: login returns same error for "user not found" AND
      "wrong password" — "invalid credentials" (prevent username enumeration)

13. UPDATE internal/api/server.go:
    - Add fields: authCfg (config.AuthConfig), jwtService (*auth.JWTService),
      userStore (auth.UserStore), rateLimiter (*auth.RateLimiter)
    - Update New() signature: add jwtSvc *auth.JWTService, userStore auth.UserStore
      (both nil when auth disabled)
    - Create rateLimiter in constructor when auth enabled
      (10 attempts, 15 min window)

14. UPDATE internal/api/server.go Routes():
    - Health always public
    - If authCfg.Enabled:
      - /auth/login in rate-limited group (public)
      - /auth/refresh public
      - Everything else behind RequireAuth middleware
      - /auth/me, /instances/*, /instances/{id}/*, /instances/{id}/metrics
        in protected group (minimum viewer)
      - Admin-only group (empty for now, future POST/PUT/DELETE)
    - If !authCfg.Enabled:
      - Keep authStubMiddleware on all routes (current behavior)

15. Add rateLimitMiddleware method on APIServer:
    - Check s.rateLimiter.Allow(ip), return 429 with Retry-After if denied

16. UPDATE internal/api/middleware.go:
    - Update UserFromContext to check auth.ClaimsFromContext first,
      fall back to stub context value

### Step 4: Wiring in main.go

17. UPDATE cmd/pgpulse-server/main.go:
    - If cfg.Auth.Enabled:
      - auth.NewPGUserStore(pool) → userStore
      - Count users; if 0 and InitialAdmin configured → seed
        (HashPassword + Create, log warning)
      - if 0 and no InitialAdmin → fatal error
      - auth.NewJWTService(secret, accessTTL, refreshTTL) → jwtSvc
      - api.New(cfg, store, pool, jwtSvc, userStore, logger)
    - If !cfg.Auth.Enabled:
      - api.New(cfg, store, pool, nil, nil, logger)

### Step 5: Tests

18. CREATE internal/auth/password_test.go (3 tests):
    - TestHashPassword_Valid, TestCheckPassword_Match, TestCheckPassword_Mismatch

19. CREATE internal/auth/rbac_test.go (5 tests):
    - TestHasRole for each role combination + unknown role
    - TestValidRole

20. CREATE internal/auth/jwt_test.go (6 tests):
    - TestGenerateTokenPair_Valid
    - TestValidateToken_Valid
    - TestValidateToken_Expired (use short TTL)
    - TestValidateToken_WrongSecret
    - TestValidateToken_AccessType
    - TestValidateToken_RefreshType

21. CREATE internal/auth/ratelimit_test.go (5 tests):
    - TestAllow_WithinLimit
    - TestAllow_AtLimit
    - TestAllow_WindowExpiry (use short window)
    - TestRetryAfter_WhenLimited
    - TestRecordFailure_ConcurrentAccess (goroutines)

22. CREATE internal/auth/middleware_test.go (6 tests):
    Use httptest.NewRecorder + httptest.NewRequest.
    Create a real JWTService with test secret for token generation.
    - TestRequireAuth_ValidToken
    - TestRequireAuth_MissingHeader → 401
    - TestRequireAuth_MalformedHeader → 401
    - TestRequireAuth_ExpiredToken → 401
    - TestRequireAuth_RefreshTokenRejected → 401
    - TestRequireRole_ViewerBlockedFromAdmin → 403

23. CREATE internal/auth/store_test.go:
    - Mock-based unit tests (no DB needed):
      Define mockUserStore in the test file.
    - TestMockStore_GetByUsername_Found
    - TestMockStore_GetByUsername_NotFound
    - Integration tests tagged //go:build integration (PGUserStore with real DB)

24. CREATE internal/api/auth_test.go (8 tests):
    Use existing test helper pattern (mockStore, newTestServer).
    Add mockUserStore to helpers_test.go.
    - TestLogin_Success
    - TestLogin_WrongPassword → 401
    - TestLogin_UnknownUser → 401
    - TestLogin_EmptyBody → 400
    - TestRefresh_Valid → 200 + new access token
    - TestRefresh_WithAccessToken → 401
    - TestRefresh_Invalid → 401
    - TestMe_Valid → 200 + user info

25. UPDATE internal/api/helpers_test.go:
    - Add mockUserStore implementing auth.UserStore
    - Update newTestServer to pass nil, nil for jwtSvc, userStore
      (tests run with auth disabled by default)
    - Add newTestServerWithAuth helper for auth-enabled tests

26. UPDATE internal/config/config_test.go:
    - TestAuthConfig_Enabled_Valid
    - TestAuthConfig_ShortJWTSecret → error

## Dependencies to Add

- github.com/golang-jwt/jwt/v5
- golang.org/x/crypto (for bcrypt, may already be transitive)

## Critical Rules

- All SQL: parameterized queries ($1, $2) — NEVER string concatenation
- auth package must NOT import api, collector, or orchestrator packages
- Same "invalid credentials" error for wrong username AND wrong password
- Never log passwords, tokens, or Authorization header values
- Existing tests must not break (newTestServer updated for new signature)

## Output

When done, list:
1. All files created (with line counts)
2. All files modified (with description of changes)
3. Any design decisions made that differ from design.md
```

---

## Pre-Flight Checklist (Developer)

Before pasting the prompt:

- [ ] Copy requirements.md, design.md, team-prompt.md to
      `docs/iterations/M3_01_03012026_auth-security/`
- [ ] Update `.claude/CLAUDE.md` Current Iteration section:
      ```
      ## Current Iteration
      M3_01 — Authentication & RBAC
      See: docs/iterations/M3_01_03012026_auth-security/
      ```
- [ ] Commit docs: `git add docs/ && git commit -m "docs: add M3_01 requirements and design"`

## Post-Session Checklist (Developer)

After Claude Code finishes creating files:

```bash
cd C:\Users\Archer\Projects\PGPulse_01
go mod tidy
go build ./...
go vet ./...
go test ./...
golangci-lint run
```

If errors → paste back into Claude Code for fixes.

When clean:
```bash
git add .
git commit -m "feat(auth): add JWT authentication and RBAC (M3_01)"
git push
```
