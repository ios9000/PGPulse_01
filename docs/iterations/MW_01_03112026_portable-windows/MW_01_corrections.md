# MW_01 — Pre-Flight Corrections

**Date:** 2026-03-11
**Based on:** Pre-flight grep results against actual codebase

**Print this file and keep it next to the team-prompt. Agents must read this BEFORE executing their tasks.**

---

## Correction 1: Auth System Uses Claims, Not User

**Affects:** Specialist B (API & Auth)

The design doc assumed a `User` struct and `UserContextKey` in context. The actual system uses:

```go
// internal/auth/jwt.go
type Claims struct {
    jwt.RegisteredClaims
    UserID      int64     `json:"uid"`
    Username    string    `json:"usr"`
    Role        string    `json:"role"`
    Type        TokenType `json:"type"`
    Permissions []string  `json:"perms,omitempty"`
}
```

```go
// internal/auth/middleware.go
type contextKey string                              // unexported
const claimsContextKey contextKey = "auth_claims"   // unexported

func ClaimsFromContext(ctx context.Context) *Claims  // exported
```

**Correction for NewAuthMiddleware (AuthDisabled mode):**

Instead of injecting a `User`, inject a `Claims`:

```go
func NewAuthMiddleware(jwtSvc *JWTService, errorWriter func(w http.ResponseWriter, code int, errCode, message string), mode AuthMode) func(http.Handler) http.Handler {
    if mode == AuthDisabled {
        return func(next http.Handler) http.Handler {
            return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                implicitClaims := &Claims{
                    UserID:   0,
                    Username: "admin",
                    Role:     "admin",
                    Permissions: []string{"*"},
                }
                ctx := context.WithValue(r.Context(), claimsContextKey, implicitClaims)
                next.ServeHTTP(w, r.WithContext(ctx))
            })
        }
    }
    return RequireAuth(jwtSvc, errorWriter)
}
```

Note: `claimsContextKey` is in the same package (`internal/auth`), so it's accessible. `NewAuthMiddleware` takes the same `errorWriter` param as `RequireAuth` for API consistency (it's unused in disabled mode but keeps the signature uniform).

---

## Correction 2: RequireAuth Signature Has Two Params

**Affects:** Specialist B (API & Auth), Specialist A (main.go wiring)

The design doc showed `RequireAuth(jwtService)`. Actual signature:

```go
func RequireAuth(jwtSvc *JWTService, errorWriter func(w http.ResponseWriter, code int, errCode, message string)) func(http.Handler) http.Handler
```

Usage in `internal/api/server.go` line 139:

```go
r.Use(auth.RequireAuth(s.jwtService, writeErrorRaw))
```

**Correction:** `NewAuthMiddleware` must match — see Correction 1 above. In `server.go`, replace the `RequireAuth` call with `NewAuthMiddleware`:

```go
// OLD:
r.Use(auth.RequireAuth(s.jwtService, writeErrorRaw))

// NEW:
r.Use(auth.NewAuthMiddleware(s.jwtService, writeErrorRaw, s.authMode))
```

---

## Correction 3: APIServer Uses api.New() Factory with 11 Params

**Affects:** Specialist A (main.go), Specialist B (server.go)

The design doc assumed struct literal initialization. Actual factory:

```go
func New(cfg config.Config, store collector.MetricStore, pool Pinger,
    jwtSvc *auth.JWTService, userStore auth.UserStore, logger *slog.Logger,
    alertRuleStore alert.AlertRuleStore, alertHistoryStore alert.AlertHistoryStore,
    evaluator *alert.Evaluator, registry *alert.NotifierRegistry,
    instanceStore storage.InstanceStore,
) *APIServer {
```

Current usage in `main.go` (three call sites):

```go
// Line 244 (auth enabled, ML present):
apiServer := api.New(cfg, store, pinger, jwtSvc, userStore, logger,
    alertRuleStore, alertHistoryStore, evaluator, registry, instanceStore)

// Line 256 (auth disabled):
apiServer := api.New(cfg, store, pinger, nil, nil, logger,
    alertRuleStore, alertHistoryStore, nil, nil, instanceStore)

// Line 265 (minimal):
apiServer := api.New(cfg, store, pinger, nil, nil, logger, nil, nil, nil, nil, nil)
```

There are also setter methods: `SetPlanStore()`, `SetSnapshotStore()`, `SetMLDetector()`, `SetConnProvider()`.

**Correction — use setter pattern (consistent with existing codebase):**

Do NOT add more params to `api.New()`. Instead:

**Specialist B adds to `internal/api/server.go`:**

```go
// Add fields to APIServer struct
type APIServer struct {
    // ... existing fields ...
    liveMode        bool
    memoryRetention time.Duration
    authMode        auth.AuthMode
}

// Add setter method (consistent with SetPlanStore, SetMLDetector, etc.)
func (s *APIServer) SetLiveMode(live bool, retention time.Duration) {
    s.liveMode = live
    s.memoryRetention = retention
}

func (s *APIServer) SetAuthMode(mode auth.AuthMode) {
    s.authMode = mode
}
```

**Specialist A adds to `cmd/pgpulse-server/main.go` (after api.New calls):**

```go
apiServer.SetLiveMode(liveMode, *flagHistory)
apiServer.SetAuthMode(authMode)
```

**In Routes(), replace RequireAuth with NewAuthMiddleware:**

```go
// Specialist B changes in Routes():
r.Use(auth.NewAuthMiddleware(s.jwtService, writeErrorRaw, s.authMode))
```

---

## Correction 4: AlertHistoryStore Has 5 Methods

**Affects:** Specialist B (NullAlertHistoryStore)

Design doc had `Save` and `Query`. Actual interface:

```go
type AlertHistoryStore interface {
    Record(ctx context.Context, event *AlertEvent) error
    Resolve(ctx context.Context, ruleID, instanceID string, resolvedAt time.Time) error
    ListUnresolved(ctx context.Context) ([]AlertEvent, error)
    Query(ctx context.Context, q AlertHistoryQuery) ([]AlertEvent, error)
    Cleanup(ctx context.Context, olderThan time.Duration) (int64, error)
}
```

**Corrected NullAlertHistoryStore:**

```go
package alert

import (
    "context"
    "time"
)

type NullAlertHistoryStore struct{}

func NewNullAlertHistoryStore() *NullAlertHistoryStore {
    return &NullAlertHistoryStore{}
}

func (n *NullAlertHistoryStore) Record(ctx context.Context, event *AlertEvent) error {
    return nil
}

func (n *NullAlertHistoryStore) Resolve(ctx context.Context, ruleID, instanceID string, resolvedAt time.Time) error {
    return nil
}

func (n *NullAlertHistoryStore) ListUnresolved(ctx context.Context) ([]AlertEvent, error) {
    return nil, nil
}

func (n *NullAlertHistoryStore) Query(ctx context.Context, q AlertHistoryQuery) ([]AlertEvent, error) {
    return nil, nil
}

func (n *NullAlertHistoryStore) Cleanup(ctx context.Context, olderThan time.Duration) (int64, error) {
    return 0, nil
}
```

---

## Correction 5: Live Mode AlertHistoryStore Wiring in main.go

**Affects:** Specialist A (main.go)

When `liveMode == true`, pass `NullAlertHistoryStore` to `api.New()` instead of nil:

```go
var alertHistoryStore alert.AlertHistoryStore

if liveMode {
    alertHistoryStore = alert.NewNullAlertHistoryStore()
} else {
    // existing PG-backed alert history store initialization
    alertHistoryStore = ... // existing code
}

// Then pass alertHistoryStore to api.New() as before
```

This prevents nil pointer panics in any code path that calls `alertHistoryStore.Record()`.

---

## Correction 6: Frontend Component Targets

**Affects:** Specialist C (Frontend & Build)

Exact file targets confirmed:

| Component | File | Action |
|-----------|------|--------|
| Header / top bar | `web/src/components/layout/TopBar.tsx` | Add Live Mode badge |
| App shell | `web/src/components/layout/AppShell.tsx` | Wrap with SystemModeProvider (or do in App.tsx) |
| Forecast UI | `web/src/components/charts/TimeSeriesChart.tsx` | Gate forecast controls on mode |
| Auth / login | Check `web/src/components/auth/` | Handle auth-disabled redirect |

Only ONE file has forecast references (`TimeSeriesChart.tsx`), making the gating very clean.

---

## Summary of Corrections by Specialist

### Specialist A — Storage & Config
- Live mode wiring: use `NullAlertHistoryStore` instead of nil (Correction 5)
- After `api.New()`, call `apiServer.SetLiveMode(liveMode, *flagHistory)` and `apiServer.SetAuthMode(authMode)` (Correction 3)

### Specialist B — API & Auth
- `NewAuthMiddleware` injects `*Claims` not `User` (Correction 1)
- `NewAuthMiddleware` takes 3 params: `jwtSvc`, `errorWriter`, `mode` (Correction 2)
- Add `SetLiveMode()` and `SetAuthMode()` setters instead of modifying `api.New()` (Correction 3)
- `NullAlertHistoryStore` implements 5 methods: Record, Resolve, ListUnresolved, Query, Cleanup (Correction 4)
- In `Routes()`, replace `auth.RequireAuth(s.jwtService, writeErrorRaw)` with `auth.NewAuthMiddleware(s.jwtService, writeErrorRaw, s.authMode)` (Correction 2)

### Specialist C — Frontend & Build
- TopBar.tsx for badge, TimeSeriesChart.tsx for forecast gating (Correction 6)
- Check `web/src/components/auth/` for login redirect handling (Correction 6)
