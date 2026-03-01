# M3_01 — Authentication & RBAC: Design

**Iteration:** M3_01  
**Date:** 2026-03-01  
**Path:** docs/iterations/M3_01_03012026_auth-security/design.md

---

## 1. Architecture Overview

Auth is a self-contained package (`internal/auth/`) with no imports from
`internal/api/`, `internal/collector/`, or `internal/orchestrator/`.
The API layer imports auth; auth never imports API.

```
cmd/pgpulse-server/main.go
  │
  ├─ config.Load() → Config (now includes AuthConfig)
  │
  ├─ storage.NewPool() → pool
  ├─ storage.Migrate() → runs 003_users.sql
  │
  ├─ auth.NewPGUserStore(pool) → userStore
  ├─ auth.NewJWTService(cfg.Auth) → jwtSvc
  ├─ auth.SeedInitialAdmin(ctx, userStore, cfg.Auth, logger)
  │
  ├─ api.New(cfg, store, pool, jwtSvc, userStore, logger) → apiServer
  │   └─ Routes():
  │       ├─ /health → public
  │       ├─ /auth/login → rate-limited, public
  │       ├─ /auth/refresh → public
  │       └─ /instances/* → auth.RequireAuth(jwtSvc) middleware
  │
  └─ orchestrator.New(cfg, store, logger) → orch (unchanged)
```

### Dependency Graph

```
internal/auth/
├── jwt.go          → golang-jwt/jwt/v5, config (AuthConfig)
├── password.go     → golang.org/x/crypto/bcrypt
├── rbac.go         → (no external deps)
├── ratelimit.go    → sync, time
├── middleware.go    → jwt.go, net/http
└── store.go        → pgx/v5/pgxpool

internal/api/
├── server.go       → imports auth (JWTService, UserStore)
├── auth.go         → imports auth (JWTService, UserStore, password)
└── middleware.go    → imports auth (middleware functions)
```

---

## 2. Config: AuthConfig

### Struct (internal/config/config.go)

```go
type AuthConfig struct {
    Enabled         bool          `koanf:"enabled"`           // default false
    JWTSecret       string        `koanf:"jwt_secret"`        // required when enabled
    AccessTokenTTL  time.Duration `koanf:"access_token_ttl"`  // default 24h
    RefreshTokenTTL time.Duration `koanf:"refresh_token_ttl"` // default 168h (7d)
    BcryptCost      int           `koanf:"bcrypt_cost"`       // default 12
    InitialAdmin    *InitialAdminConfig `koanf:"initial_admin"`
}

type InitialAdminConfig struct {
    Username string `koanf:"username"` // required
    Password string `koanf:"password"` // required
}
```

Add `Auth AuthConfig` field to the top-level `Config` struct.

### Validation Rules (internal/config/load.go)

Add to `validate()`:

```go
if c.Auth.Enabled {
    if c.Storage.DSN == "" {
        return fmt.Errorf("auth.enabled=true requires storage.dsn to be configured")
    }
    if len(c.Auth.JWTSecret) < 32 {
        return fmt.Errorf("auth.jwt_secret must be at least 32 characters")
    }
    if c.Auth.AccessTokenTTL == 0 {
        c.Auth.AccessTokenTTL = 24 * time.Hour
    }
    if c.Auth.RefreshTokenTTL == 0 {
        c.Auth.RefreshTokenTTL = 7 * 24 * time.Hour
    }
    if c.Auth.BcryptCost == 0 {
        c.Auth.BcryptCost = 12
    }
    // initial_admin validated at seed time, not here
    // (table may already have users, making it unnecessary)
}
```

### Example YAML (configs/pgpulse.example.yml — additions)

```yaml
auth:
  enabled: false
  jwt_secret: "change-me-to-at-least-32-characters-long"
  access_token_ttl: 24h
  refresh_token_ttl: 168h
  bcrypt_cost: 12
  initial_admin:
    username: "admin"
    password: "changeme"
```

---

## 3. Migration: 003_users.sql

**Path:** `internal/storage/migrations/003_users.sql`

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

COMMENT ON TABLE users IS 'PGPulse user accounts for API authentication';
```

Notes:
- `BIGINT GENERATED ALWAYS AS IDENTITY` — modern PG identity column, no serial.
- `CHECK (role IN (...))` — enforces valid roles at DB level.
- `username` has UNIQUE constraint — application can rely on this.
- No `email` column — not needed for M3. Can be added later.
- `updated_at` must be maintained by application (no trigger for simplicity).

The existing `migrate.go` already handles sequential migration files via go:embed.
File `003_users.sql` will be picked up automatically.

---

## 4. Package: internal/auth/

### 4.1 store.go — User Storage

```go
package auth

import (
    "context"
    "errors"
    "time"

    "github.com/jackc/pgx/v5"
    "github.com/jackc/pgx/v5/pgxpool"
)

var ErrUserNotFound = errors.New("user not found")

// User represents a PGPulse user account.
type User struct {
    ID           int64
    Username     string
    PasswordHash string
    Role         string
    CreatedAt    time.Time
    UpdatedAt    time.Time
}

// UserStore provides access to user data.
type UserStore interface {
    GetByUsername(ctx context.Context, username string) (*User, error)
    Create(ctx context.Context, username, passwordHash, role string) (*User, error)
    Count(ctx context.Context) (int64, error)
}

// PGUserStore implements UserStore backed by PostgreSQL.
type PGUserStore struct {
    pool *pgxpool.Pool
}

func NewPGUserStore(pool *pgxpool.Pool) *PGUserStore {
    return &PGUserStore{pool: pool}
}

func (s *PGUserStore) GetByUsername(ctx context.Context, username string) (*User, error) {
    var u User
    err := s.pool.QueryRow(ctx,
        `SELECT id, username, password_hash, role, created_at, updated_at
         FROM users WHERE username = $1`, username,
    ).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.CreatedAt, &u.UpdatedAt)
    if errors.Is(err, pgx.ErrNoRows) {
        return nil, ErrUserNotFound
    }
    return &u, err
}

func (s *PGUserStore) Create(ctx context.Context, username, passwordHash, role string) (*User, error) {
    var u User
    err := s.pool.QueryRow(ctx,
        `INSERT INTO users (username, password_hash, role)
         VALUES ($1, $2, $3)
         RETURNING id, username, password_hash, role, created_at, updated_at`,
        username, passwordHash, role,
    ).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.CreatedAt, &u.UpdatedAt)
    return &u, err
}

func (s *PGUserStore) Count(ctx context.Context) (int64, error) {
    var count int64
    err := s.pool.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&count)
    return count, err
}
```

### 4.2 password.go — Bcrypt Wrapper

```go
package auth

import "golang.org/x/crypto/bcrypt"

// HashPassword returns the bcrypt hash of the password.
func HashPassword(password string, cost int) (string, error) {
    hash, err := bcrypt.GenerateFromPassword([]byte(password), cost)
    return string(hash), err
}

// CheckPassword compares a plaintext password against a bcrypt hash.
// Returns nil on match, error on mismatch.
func CheckPassword(password, hash string) error {
    return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}
```

### 4.3 jwt.go — Token Service

```go
package auth

import (
    "fmt"
    "time"

    "github.com/golang-jwt/jwt/v5"
)

// TokenType distinguishes access from refresh tokens.
type TokenType string

const (
    TokenAccess  TokenType = "access"
    TokenRefresh TokenType = "refresh"
)

// Claims are the JWT payload for PGPulse tokens.
type Claims struct {
    jwt.RegisteredClaims
    UserID   int64     `json:"uid"`
    Username string    `json:"usr"`
    Role     string    `json:"role"`
    Type     TokenType `json:"type"`
}

// TokenPair holds the access and refresh tokens returned on login.
type TokenPair struct {
    AccessToken  string `json:"access_token"`
    RefreshToken string `json:"refresh_token"`
    ExpiresIn    int64  `json:"expires_in"` // seconds until access token expires
}

// JWTService handles token generation and validation.
type JWTService struct {
    secret          []byte
    accessTokenTTL  time.Duration
    refreshTokenTTL time.Duration
}

func NewJWTService(secret string, accessTTL, refreshTTL time.Duration) *JWTService {
    return &JWTService{
        secret:          []byte(secret),
        accessTokenTTL:  accessTTL,
        refreshTokenTTL: refreshTTL,
    }
}

// GenerateTokenPair creates both access and refresh tokens for a user.
func (s *JWTService) GenerateTokenPair(user *User) (*TokenPair, error) {
    now := time.Now()

    accessToken, err := s.generateToken(user, TokenAccess, now, s.accessTokenTTL)
    if err != nil {
        return nil, fmt.Errorf("generate access token: %w", err)
    }

    refreshToken, err := s.generateToken(user, TokenRefresh, now, s.refreshTokenTTL)
    if err != nil {
        return nil, fmt.Errorf("generate refresh token: %w", err)
    }

    return &TokenPair{
        AccessToken:  accessToken,
        RefreshToken: refreshToken,
        ExpiresIn:    int64(s.accessTokenTTL.Seconds()),
    }, nil
}

// GenerateAccessToken creates only a new access token (used by refresh flow).
func (s *JWTService) GenerateAccessToken(user *User) (string, error) {
    return s.generateToken(user, TokenAccess, time.Now(), s.accessTokenTTL)
}

func (s *JWTService) generateToken(user *User, tokenType TokenType, now time.Time, ttl time.Duration) (string, error) {
    claims := Claims{
        RegisteredClaims: jwt.RegisteredClaims{
            Issuer:    "pgpulse",
            IssuedAt:  jwt.NewNumericDate(now),
            ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
        },
        UserID:   user.ID,
        Username: user.Username,
        Role:     user.Role,
        Type:     tokenType,
    }

    token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
    return token.SignedString(s.secret)
}

// ValidateToken parses and validates a JWT string. Returns claims if valid.
func (s *JWTService) ValidateToken(tokenString string) (*Claims, error) {
    token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(t *jwt.Token) (any, error) {
        if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
            return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
        }
        return s.secret, nil
    })
    if err != nil {
        return nil, fmt.Errorf("parse token: %w", err)
    }

    claims, ok := token.Claims.(*Claims)
    if !ok || !token.Valid {
        return nil, fmt.Errorf("invalid token claims")
    }

    return claims, nil
}
```

### 4.4 rbac.go — Role Definitions and Checks

```go
package auth

// Roles
const (
    RoleAdmin  = "admin"
    RoleViewer = "viewer"
)

// roleHierarchy defines the permission level of each role.
// Higher number = more permissions.
var roleHierarchy = map[string]int{
    RoleViewer: 1,
    RoleAdmin:  2,
}

// HasRole checks if the user's role meets or exceeds the required role.
func HasRole(userRole, requiredRole string) bool {
    userLevel, ok1 := roleHierarchy[userRole]
    requiredLevel, ok2 := roleHierarchy[requiredRole]
    if !ok1 || !ok2 {
        return false
    }
    return userLevel >= requiredLevel
}

// ValidRole returns true if the role string is a known role.
func ValidRole(role string) bool {
    _, ok := roleHierarchy[role]
    return ok
}
```

### 4.5 middleware.go — Auth Chi Middleware

```go
package auth

import (
    "context"
    "net/http"
    "strings"
)

// contextKey is unexported to prevent collisions.
type contextKey string

const claimsContextKey contextKey = "auth_claims"

// ClaimsFromContext retrieves the JWT claims from the request context.
// Returns nil if no claims are present (e.g., auth disabled).
func ClaimsFromContext(ctx context.Context) *Claims {
    claims, _ := ctx.Value(claimsContextKey).(*Claims)
    return claims
}

// RequireAuth returns middleware that validates the Bearer token
// and sets claims in the request context.
// errorWriter is a function the caller provides to write JSON error responses
// (avoids auth importing the api package's response helpers).
func RequireAuth(jwtSvc *JWTService, errorWriter func(w http.ResponseWriter, code int, errCode, message string)) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            header := r.Header.Get("Authorization")
            if header == "" {
                errorWriter(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing Authorization header")
                return
            }

            parts := strings.SplitN(header, " ", 2)
            if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
                errorWriter(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid Authorization header format")
                return
            }

            claims, err := jwtSvc.ValidateToken(parts[1])
            if err != nil {
                errorWriter(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid or expired token")
                return
            }

            if claims.Type != TokenAccess {
                errorWriter(w, http.StatusUnauthorized, "UNAUTHORIZED", "expected access token")
                return
            }

            ctx := context.WithValue(r.Context(), claimsContextKey, claims)
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}

// RequireRole returns middleware that checks if the authenticated user
// has the required role (or higher).
func RequireRole(requiredRole string, errorWriter func(w http.ResponseWriter, code int, errCode, message string)) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            claims := ClaimsFromContext(r.Context())
            if claims == nil {
                errorWriter(w, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
                return
            }

            if !HasRole(claims.Role, requiredRole) {
                errorWriter(w, http.StatusForbidden, "FORBIDDEN", "insufficient permissions")
                return
            }

            next.ServeHTTP(w, r)
        })
    }
}
```

**Design note — the `errorWriter` callback pattern:** The auth middleware needs to write
JSON error responses matching PGPulse's envelope format, but `internal/auth` must not
import `internal/api`. Passing a `func(w, code, errCode, message)` callback lets the API
layer provide its own `writeError` function at wiring time. This keeps the dependency arrow
one-directional: `api → auth`, never `auth → api`.

### 4.6 ratelimit.go — In-Memory Per-IP Rate Limiter

```go
package auth

import (
    "net"
    "net/http"
    "sync"
    "time"
)

// RateLimiter tracks failed attempts per IP using a sliding window.
type RateLimiter struct {
    mu         sync.Mutex
    attempts   map[string][]time.Time
    maxAttempts int
    window      time.Duration
}

func NewRateLimiter(maxAttempts int, window time.Duration) *RateLimiter {
    return &RateLimiter{
        attempts:    make(map[string][]time.Time),
        maxAttempts: maxAttempts,
        window:      window,
    }
}

// Allow checks whether the IP is within limits. Does NOT record an attempt.
func (rl *RateLimiter) Allow(ip string) bool {
    rl.mu.Lock()
    defer rl.mu.Unlock()

    rl.pruneExpired(ip)
    return len(rl.attempts[ip]) < rl.maxAttempts
}

// RecordFailure records a failed attempt for the given IP.
func (rl *RateLimiter) RecordFailure(ip string) {
    rl.mu.Lock()
    defer rl.mu.Unlock()

    rl.attempts[ip] = append(rl.attempts[ip], time.Now())
}

// RetryAfter returns the number of seconds until the oldest attempt
// in the window expires. Returns 0 if not rate-limited.
func (rl *RateLimiter) RetryAfter(ip string) int {
    rl.mu.Lock()
    defer rl.mu.Unlock()

    rl.pruneExpired(ip)
    entries := rl.attempts[ip]
    if len(entries) < rl.maxAttempts {
        return 0
    }
    oldest := entries[0]
    remaining := rl.window - time.Since(oldest)
    if remaining <= 0 {
        return 0
    }
    return int(remaining.Seconds()) + 1
}

func (rl *RateLimiter) pruneExpired(ip string) {
    cutoff := time.Now().Add(-rl.window)
    entries := rl.attempts[ip]
    i := 0
    for i < len(entries) && entries[i].Before(cutoff) {
        i++
    }
    if i > 0 {
        rl.attempts[ip] = entries[i:]
    }
    if len(rl.attempts[ip]) == 0 {
        delete(rl.attempts, ip)
    }
}

// ClientIP extracts the client IP from the request.
// Checks X-Forwarded-For first, falls back to RemoteAddr.
func ClientIP(r *http.Request) string {
    if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
        // Take the first IP (client IP, not proxies)
        if comma := strings.Index(xff, ","); comma > 0 {
            xff = xff[:comma]
        }
        return strings.TrimSpace(xff)
    }
    if xff := r.Header.Get("X-Real-IP"); xff != "" {
        return strings.TrimSpace(xff)
    }
    host, _, err := net.SplitHostPort(r.RemoteAddr)
    if err != nil {
        return r.RemoteAddr
    }
    return host
}
```

Note: `ClientIP` needs `"strings"` in the import block alongside `"net"`.

---

## 5. Package: internal/api/ Changes

### 5.1 auth.go — Auth Handlers

```go
package api

import (
    "encoding/json"
    "errors"
    "net/http"

    "github.com/ios9000/PGPulse_01/internal/auth"
)

// loginRequest is the JSON body for POST /auth/login.
type loginRequest struct {
    Username string `json:"username"`
    Password string `json:"password"`
}

// refreshRequest is the JSON body for POST /auth/refresh.
type refreshRequest struct {
    RefreshToken string `json:"refresh_token"`
}

// userResponse is the JSON body for GET /auth/me.
type userResponse struct {
    ID       int64  `json:"id"`
    Username string `json:"username"`
    Role     string `json:"role"`
}

func (s *APIServer) handleLogin(w http.ResponseWriter, r *http.Request) {
    var req loginRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
        return
    }

    if req.Username == "" || req.Password == "" {
        writeError(w, http.StatusBadRequest, "BAD_REQUEST", "username and password are required")
        return
    }

    // Rate limiting check (applied by middleware, but double-check here)
    user, err := s.userStore.GetByUsername(r.Context(), req.Username)
    if err != nil {
        if errors.Is(err, auth.ErrUserNotFound) {
            // Record failure for rate limiting
            if s.rateLimiter != nil {
                s.rateLimiter.RecordFailure(auth.ClientIP(r))
            }
            writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid credentials")
            return
        }
        writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "authentication failed")
        return
    }

    if err := auth.CheckPassword(req.Password, user.PasswordHash); err != nil {
        if s.rateLimiter != nil {
            s.rateLimiter.RecordFailure(auth.ClientIP(r))
        }
        writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid credentials")
        return
    }

    pair, err := s.jwtService.GenerateTokenPair(user)
    if err != nil {
        writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to generate tokens")
        return
    }

    writeJSON(w, http.StatusOK, pair)
}

func (s *APIServer) handleRefresh(w http.ResponseWriter, r *http.Request) {
    var req refreshRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
        return
    }

    claims, err := s.jwtService.ValidateToken(req.RefreshToken)
    if err != nil || claims.Type != auth.TokenRefresh {
        writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid refresh token")
        return
    }

    // Look up current user to get fresh role (in case it changed)
    user, err := s.userStore.GetByUsername(r.Context(), claims.Username)
    if err != nil {
        writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "user not found")
        return
    }

    accessToken, err := s.jwtService.GenerateAccessToken(user)
    if err != nil {
        writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to generate token")
        return
    }

    writeJSON(w, http.StatusOK, map[string]any{
        "access_token": accessToken,
        "expires_in":   int64(s.jwtService.AccessTokenTTL().Seconds()),
    })
}

func (s *APIServer) handleMe(w http.ResponseWriter, r *http.Request) {
    claims := auth.ClaimsFromContext(r.Context())
    if claims == nil {
        writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
        return
    }

    writeJSON(w, http.StatusOK, userResponse{
        ID:       claims.UserID,
        Username: claims.Username,
        Role:     claims.Role,
    })
}
```

Note: `handleRefresh` calls a `jwtService.AccessTokenTTL()` accessor — add this
to JWTService:

```go
func (s *JWTService) AccessTokenTTL() time.Duration { return s.accessTokenTTL }
```

### 5.2 server.go — Updated APIServer

```go
// Updated struct — add auth fields
type APIServer struct {
    store       collector.MetricStore
    instances   []config.InstanceConfig
    serverCfg   config.ServerConfig
    authCfg     config.AuthConfig       // NEW
    jwtService  *auth.JWTService        // NEW — nil when auth disabled
    userStore   auth.UserStore          // NEW — nil when auth disabled
    rateLimiter *auth.RateLimiter       // NEW — nil when auth disabled
    logger      *slog.Logger
    startTime   time.Time
    pool        Pinger
}

// Updated constructor
func New(
    cfg config.Config,
    store collector.MetricStore,
    pool Pinger,
    jwtSvc *auth.JWTService,      // nil when auth disabled
    userStore auth.UserStore,      // nil when auth disabled
    logger *slog.Logger,
) *APIServer {
    var rl *auth.RateLimiter
    if cfg.Auth.Enabled {
        rl = auth.NewRateLimiter(10, 15*time.Minute)
    }
    return &APIServer{
        store:       store,
        instances:   cfg.Instances,
        serverCfg:   cfg.Server,
        authCfg:     cfg.Auth,
        jwtService:  jwtSvc,
        userStore:   userStore,
        rateLimiter: rl,
        logger:      logger,
        startTime:   time.Now(),
        pool:        pool,
    }
}
```

### 5.3 Routes() — Conditional Auth Wiring

```go
func (s *APIServer) Routes() http.Handler {
    r := chi.NewRouter()

    // Global middleware (unchanged)
    r.Use(requestIDMiddleware)
    r.Use(loggerMiddleware(s.logger))
    r.Use(recovererMiddleware(s.logger))
    if s.serverCfg.CORSEnabled {
        r.Use(corsMiddleware)
    }

    r.Route("/api/v1", func(r chi.Router) {
        // Health is always public
        r.Get("/health", s.handleHealth)

        if s.authCfg.Enabled {
            // Public auth routes with rate limiting
            r.Group(func(r chi.Router) {
                r.Use(s.rateLimitMiddleware)
                r.Post("/auth/login", s.handleLogin)
            })
            r.Post("/auth/refresh", s.handleRefresh)

            // Protected routes — require valid JWT
            r.Group(func(r chi.Router) {
                r.Use(auth.RequireAuth(s.jwtService, writeErrorRaw))
                r.Get("/auth/me", s.handleMe)
                r.Get("/instances", s.handleListInstances)
                r.Get("/instances/{id}", s.handleGetInstance)
                r.Get("/instances/{id}/metrics", s.handleGetMetrics)

                // Admin-only group (future mutation endpoints)
                r.Group(func(r chi.Router) {
                    r.Use(auth.RequireRole(auth.RoleAdmin, writeErrorRaw))
                    // POST/PUT/DELETE endpoints go here in future iterations
                })
            })
        } else {
            // Auth disabled — all routes open, authStub sets anonymous user
            r.Use(authStubMiddleware)
            r.Get("/instances", s.handleListInstances)
            r.Get("/instances/{id}", s.handleGetInstance)
            r.Get("/instances/{id}/metrics", s.handleGetMetrics)
        }
    })

    return r
}
```

**`writeErrorRaw`** is a package-level function in `internal/api/response.go` that matches
the callback signature expected by auth middleware:

```go
// writeErrorRaw is a callback-compatible error writer for auth middleware.
func writeErrorRaw(w http.ResponseWriter, code int, errCode, message string) {
    writeError(w, code, errCode, message)
}
```

### 5.4 rateLimitMiddleware (on APIServer)

```go
func (s *APIServer) rateLimitMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if s.rateLimiter == nil {
            next.ServeHTTP(w, r)
            return
        }

        ip := auth.ClientIP(r)
        if !s.rateLimiter.Allow(ip) {
            retryAfter := s.rateLimiter.RetryAfter(ip)
            w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
            writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", "too many login attempts")
            return
        }

        next.ServeHTTP(w, r)
    })
}
```

### 5.5 middleware.go — Keep authStubMiddleware

The existing `authStubMiddleware` stays for auth-disabled mode. However, update
`UserFromContext()` to also check for auth claims:

```go
// UserFromContext returns the username from context.
// Works with both real auth (Claims) and stub (string value).
func UserFromContext(ctx context.Context) string {
    // Check real auth first
    if claims := auth.ClaimsFromContext(ctx); claims != nil {
        return claims.Username
    }
    // Fall back to stub
    if user, ok := ctx.Value(userContextKey).(string); ok {
        return user
    }
    return "unknown"
}
```

---

## 6. User Seeding Logic

In `cmd/pgpulse-server/main.go`, after migration and before starting HTTP:

```go
if cfg.Auth.Enabled {
    userStore := auth.NewPGUserStore(pool)
    count, err := userStore.Count(ctx)
    if err != nil {
        logger.Error("failed to count users", "error", err)
        os.Exit(1)
    }

    if count == 0 {
        if cfg.Auth.InitialAdmin == nil {
            logger.Error("auth enabled but no users exist and auth.initial_admin not configured")
            os.Exit(1)
        }
        hash, err := auth.HashPassword(cfg.Auth.InitialAdmin.Password, cfg.Auth.BcryptCost)
        if err != nil {
            logger.Error("failed to hash initial admin password", "error", err)
            os.Exit(1)
        }
        _, err = userStore.Create(ctx, cfg.Auth.InitialAdmin.Username, hash, auth.RoleAdmin)
        if err != nil {
            logger.Error("failed to create initial admin", "error", err)
            os.Exit(1)
        }
        logger.Warn("created initial admin user — change password immediately",
            "username", cfg.Auth.InitialAdmin.Username)
    }

    jwtSvc := auth.NewJWTService(cfg.Auth.JWTSecret, cfg.Auth.AccessTokenTTL, cfg.Auth.RefreshTokenTTL)
    apiServer = api.New(cfg, store, pool, jwtSvc, userStore, logger)
} else {
    apiServer = api.New(cfg, store, pool, nil, nil, logger)
}
```

---

## 7. Updated main.go Flow

```
main.go
  → config.Load(path) → Config (now includes AuthConfig)
  → if cfg.Storage.DSN != "":
      storage.NewPool(ctx, dsn) → pool
      storage.Migrate(ctx, pool, logger, MigrateOptions{...}) → now runs 003_users.sql
      storage.NewPGStore(pool, logger) → store
    else:
      orchestrator.NewLogStore(logger) → store
  → if cfg.Auth.Enabled:
      auth.NewPGUserStore(pool) → userStore
      seed initial admin if users table empty
      auth.NewJWTService(cfg.Auth) → jwtSvc
      api.New(cfg, store, pool, jwtSvc, userStore, logger) → apiServer
    else:
      api.New(cfg, store, pool, nil, nil, logger) → apiServer
  → &http.Server{Handler: apiServer.Routes(), ...}
  → go httpServer.ListenAndServe()
  → orchestrator.New(cfg, store, logger) → orch  (unchanged)
  → orch.Start(ctx)
  → signal.Notify → shutdown sequence (unchanged)
```

---

## 8. Test Plan

### 8.1 internal/auth/ Tests

| File | Tests | Description |
|------|-------|-------------|
| `jwt_test.go` | 6 | GenerateTokenPair valid; ValidateToken valid; expired token rejected; wrong signing method rejected; access vs refresh type; invalid token string |
| `password_test.go` | 3 | HashPassword produces valid hash; CheckPassword matches; CheckPassword rejects wrong password |
| `rbac_test.go` | 4 | Admin has admin role; admin has viewer role; viewer has viewer role; viewer does NOT have admin role; unknown role returns false |
| `ratelimit_test.go` | 5 | Allow within limit; deny at limit; window expiry resets; RetryAfter returns positive value when limited; concurrent access safety |
| `middleware_test.go` | 6 | Valid token passes through; missing header → 401; malformed header → 401; expired token → 401; refresh token rejected (wrong type) → 401; RequireRole admin blocks viewer → 403 |
| `store_test.go` | 4 | Create user; GetByUsername found; GetByUsername not found → ErrUserNotFound; Count returns correct number |

**Total auth package: ~28 tests**

### 8.2 internal/api/ Auth Handler Tests

| File | Tests | Description |
|------|-------|-------------|
| `auth_test.go` | 8 | Login success → 200 + tokens; login wrong password → 401; login unknown user → 401; login empty body → 400; refresh valid → 200 + new access token; refresh with access token → 401; refresh invalid → 401; me with valid token → 200 + user info |

**Total new API tests: ~8**

### 8.3 Regression

All 24 existing API tests must continue to pass. The test helper `newTestServer()`
should be updated to create the server with `auth.enabled=false` so existing tests
remain unchanged.

### 8.4 Config Tests

| Tests | Description |
|-------|-------------|
| 2 | Auth enabled + valid config → ok; auth enabled + short JWT secret → error |

**Estimated total new tests: ~38**

### 8.5 Store Tests Note

`store_test.go` for `PGUserStore` requires a real PG database (integration test).
Since Docker Desktop is unavailable on the developer workstation, these tests
should be tagged `//go:build integration` and run in CI only. For unit testing the
handlers, use a mock `UserStore` implementation.

```go
// mockUserStore implements auth.UserStore for testing.
type mockUserStore struct {
    users map[string]*auth.User
}
```

---

## 9. Error Response Codes

New error codes introduced in M3_01:

| HTTP Status | Error Code | When |
|-------------|-----------|------|
| 401 | `UNAUTHORIZED` | Missing/invalid/expired token, wrong credentials |
| 403 | `FORBIDDEN` | Valid token but insufficient role |
| 429 | `RATE_LIMITED` | Too many failed login attempts |
| 400 | `BAD_REQUEST` | Malformed login/refresh request body |

These use the existing envelope format:
```json
{"error": {"code": "UNAUTHORIZED", "message": "invalid credentials"}}
```

---

## 10. Security Considerations

1. **Timing attacks**: `bcrypt.CompareHashAndPassword` is constant-time. Do NOT short-circuit on "user not found" vs "wrong password" — both return the same error message ("invalid credentials") to prevent username enumeration.

2. **JWT secret**: Validated at config load time (minimum 32 chars). Consider using
   `PGPULSE_AUTH_JWT_SECRET` env var override via koanf for production deployments.

3. **Password in config**: The `initial_admin.password` is plaintext in the YAML file.
   The startup log should NOT echo it. The file should have restricted permissions (0600).
   Consider supporting env var: `PGPULSE_AUTH_INITIAL_ADMIN_PASSWORD`.

4. **No token in logs**: The logger middleware must NOT log Authorization header values.
   Verify the existing logger middleware strips sensitive headers.

5. **Rate limiter memory**: The in-memory map grows with unique IPs. The pruning
   in `Allow()` cleans expired entries, but a sustained DDoS from millions of IPs could
   grow memory. Acceptable for M3 (monitoring tool, not public-facing); can add max size
   in future.

---

## 11. Files Summary — Full Paths

### New Files
```
internal/auth/jwt.go
internal/auth/password.go
internal/auth/rbac.go
internal/auth/ratelimit.go
internal/auth/middleware.go
internal/auth/store.go
internal/auth/jwt_test.go
internal/auth/password_test.go
internal/auth/rbac_test.go
internal/auth/ratelimit_test.go
internal/auth/middleware_test.go
internal/auth/store_test.go
internal/api/auth.go
internal/api/auth_test.go
internal/storage/migrations/003_users.sql
```

### Modified Files
```
internal/config/config.go          ← add AuthConfig, InitialAdminConfig
internal/config/load.go            ← add auth validation in validate()
internal/config/config_test.go     ← add auth config tests
internal/api/server.go             ← updated struct, constructor, Routes()
internal/api/response.go           ← add writeErrorRaw callback
internal/api/middleware.go         ← update UserFromContext to check Claims
internal/api/helpers_test.go       ← update newTestServer for new constructor signature
cmd/pgpulse-server/main.go        ← wire auth service, seeding
configs/pgpulse.example.yml       ← add auth section
go.mod                            ← add golang-jwt/jwt/v5, x/crypto
```
