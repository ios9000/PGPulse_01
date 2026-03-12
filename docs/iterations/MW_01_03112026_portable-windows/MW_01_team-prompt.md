# MW_01 — Team Prompt

**Iteration:** MW_01
**Date:** 2026-03-11
**Agents:** 3 specialists, parallel worktrees

---

## DO NOT RE-DISCUSS

- `internal/alert` must NEVER import `internal/ml` — use `alert.ForecastProvider` interface
- `internal/mlerrors` is the canonical home for `ErrNotBootstrapped` and `ErrNoBaseline`
- Sustained crossing (N consecutive) is the only supported mode for forecast alerts
- YAML seeds the database on startup; database becomes source of truth after first run
- `go:embed` bakes the React build into the Go binary — PGPulse is its own web server
- Test scope must be `./cmd/... ./internal/...` (not `./...`) to prevent scanning `web/node_modules/`
- Build scope: `go build ./cmd/pgpulse-server` (not `go build ./...`)
- OSSQLCollector reuses agent parsers from `internal/agent/` — no code duplication
- Per-instance `os_metrics_method` config: "sql" (default), "agent", "disabled"
- `docs/CODEBASE_DIGEST.md` is auto-generated at end of each iteration — always re-upload to Project Knowledge

### MW_01-Specific Decisions (FINAL — do not change)

- **D-MW-1:** In-memory ring buffer (MemoryStore) — 2-hour default retention
- **D-MW-2:** CLI flags override YAML; YAML overrides defaults
- **D-MW-3:** `--target` DSN for single instance; multi-instance via YAML only
- **D-MW-4:** Auth auto-skipped on localhost; `--no-auth` flag for explicit opt-out
- **D-MW-5:** No storage DSN → MemoryStore automatically (live mode)
- **D-MW-6:** 3 specialists: Storage & Config, API & Auth, Frontend & Build

---

## Step 0 — READ BEFORE WRITING

**Every specialist must read the existing code before writing anything.**

Before creating or modifying any file, read:
1. `CLAUDE.md` (project root) — module ownership, interfaces, rules
2. The specific files you will modify (listed in your specialist section below)
3. `internal/collector/collector.go` — MetricStore and MetricPoint interfaces
4. `internal/config/config.go` — current config structure
5. `cmd/pgpulse-server/main.go` — current startup sequence

---

## Specialist A — Storage & Config

### Scope
1. Create `internal/storage/memory.go` — MemoryStore
2. Create `internal/storage/memory_test.go` — unit tests
3. Modify `cmd/pgpulse-server/main.go` — CLI flags, config merge, MemoryStore wiring

### Step 0 — Read First
```bash
cat CLAUDE.md
cat internal/collector/collector.go  # MetricStore interface
cat internal/config/config.go        # Config struct
cat cmd/pgpulse-server/main.go       # Current startup
ls internal/storage/                  # Existing storage files
cat internal/storage/pgstore.go      # Existing PGStore for reference
```

### Task 1: MemoryStore — `internal/storage/memory.go`

Create a new file implementing `collector.MetricStore`:

```go
package storage

import (
    "context"
    "sort"
    "strings"
    "sync"
    "time"

    "github.com/ios9000/PGPulse_01/internal/collector"
)
```

**MemoryStore struct:**
- `mu sync.RWMutex`
- `retention time.Duration`
- `data map[string][]collector.MetricPoint` — key is `"instanceID\x00metric\x00sortedLabels"`
- `done chan struct{}`

**Functions to implement:**
- `NewMemoryStore(retention time.Duration) *MemoryStore` — creates store, starts expiry goroutine
- `Write(ctx context.Context, points []collector.MetricPoint) error` — append points to their key's slice
- `Query(ctx context.Context, query collector.MetricQuery) ([]collector.MetricPoint, error)` — filter and return matching points, sorted by timestamp ascending, respecting Limit
- `Close() error` — signal expiry goroutine to stop
- `storageKey(instanceID, metric string, labels map[string]string) string` — deterministic key from sorted labels
- `matchLabels(pointLabels, queryLabels map[string]string) bool` — all queryLabels must match
- `expireLoop()` — goroutine, runs every 30s, trims data older than retention
- `expire()` — binary search for cutoff timestamp, trim prefix of each slice, delete empty keys

**Important details:**
- `Write()` uses `m.mu.Lock()` (exclusive)
- `Query()` uses `m.mu.RLock()` (shared)
- `expire()` uses `m.mu.Lock()` (exclusive)
- Points within each key's slice are naturally ordered by write time (collectors produce monotonically increasing timestamps)
- `Query()` iterates all matching keys and collects points. For 2h × 30 metrics × 3 instances at 10s intervals = ~65K points total. Linear scan is fine.
- If `query.Limit > 0` and results exceed limit, return the LAST `Limit` points (most recent)

### Task 2: MemoryStore Tests — `internal/storage/memory_test.go`

```go
package storage

import (
    "context"
    "sync"
    "testing"
    "time"

    "github.com/ios9000/PGPulse_01/internal/collector"
)
```

Required test cases:
1. `TestMemoryStore_WriteAndQuery` — write 10 points, query all, verify order and count
2. `TestMemoryStore_QueryByInstance` — write points for 2 instances, query one, verify filtering
3. `TestMemoryStore_QueryByMetric` — write points for 2 metrics, query one
4. `TestMemoryStore_QueryByLabels` — write points with different labels, query with label filter
5. `TestMemoryStore_QueryTimeRange` — write points across time range, query subset
6. `TestMemoryStore_QueryLimit` — write 100 points, query with Limit=10, verify returns last 10
7. `TestMemoryStore_Expiry` — create store with 100ms retention, write points, sleep 200ms, trigger expire, verify points gone
8. `TestMemoryStore_ConcurrentAccess` — 10 goroutines writing + 10 reading, no race (run with `-race`)
9. `TestMemoryStore_Close` — close store, verify expiry stops (no panics after close)

### Task 3: CLI Flags & Config Merge — `cmd/pgpulse-server/main.go`

**Read `main.go` carefully first.** The existing startup sequence is ~400 lines. You need to insert flag parsing at the very beginning and modify the config/storage initialization section.

**Add at the top of main() before any config loading:**

```go
// CLI flags
var (
    flagTarget         = flag.String("target", "", "PostgreSQL DSN for quick-start mode")
    flagTargetHost     = flag.String("target-host", "", "PostgreSQL host (alternative to --target)")
    flagTargetPort     = flag.Int("target-port", 5432, "PostgreSQL port")
    flagTargetUser     = flag.String("target-user", "pgpulse_monitor", "PostgreSQL user")
    flagTargetPassword = flag.String("target-password", "", "PostgreSQL password")
    flagTargetDBName   = flag.String("target-dbname", "postgres", "PostgreSQL database")
    flagListen         = flag.String("listen", "", "HTTP listen address:port (default :8989)")
    flagHistory        = flag.Duration("history", 2*time.Hour, "Memory retention for live mode")
    flagNoAuth         = flag.Bool("no-auth", false, "Disable authentication")
    flagConfig         = flag.String("config", "config.yaml", "Config file path")
)
flag.Parse()
```

**Config merge (after loading YAML):**

1. If `--target` and `--target-host` are both set → `log.Fatal("--target and --target-host are mutually exclusive")`
2. If `--target` or `--target-host` is set → synthesize instance config, replace `cfg.Instances`
3. If `--listen` is set → parse into `cfg.Server.Listen` and `cfg.Server.Port`
4. If `--no-auth` is set → `cfg.Auth.Enabled = false`
5. If `--config` file doesn't exist AND no `--target` → `log.Fatal("No PostgreSQL instances configured...")`
6. If `--config` file doesn't exist AND `--target` is set → proceed with defaults + CLI instance (this is the quick-start path)

**Storage auto-detection:**

```go
var (
    metricStore collector.MetricStore
    liveMode    bool
)

if cfg.Storage.DSN == "" {
    // Live mode — in-memory storage
    liveMode = true
    memStore := storage.NewMemoryStore(*flagHistory)
    metricStore = memStore
    slog.Info("starting in live mode", "storage", "memory", "retention", flagHistory.String())

    // Disable features that require persistent storage
    cfg.ML.Enabled = false
    cfg.PlanCapture.Enabled = false
    cfg.SettingsSnapshot.Enabled = false
} else {
    // Persistent mode — existing TimescaleDB path
    // ... existing PGStore initialization code ...
    slog.Info("starting in persistent mode", "storage", "postgresql")
}
```

**Pass `liveMode` and `*flagHistory` to APIServer constructor** (Specialist B will add the fields).

**Listen address handling:**

```go
listenAddr := fmt.Sprintf("%s:%d", cfg.Server.Listen, cfg.Server.Port)
if *flagListen != "" {
    listenAddr = *flagListen
}
```

**DSN synthesis function** (add as a helper in main.go or a separate file):

```go
func synthesizeCLIInstance(target, host string, port int, user, password, dbname string) (*config.InstanceConfig, error) {
    dsn := target
    if dsn == "" && host != "" {
        dsn = fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable",
            url.PathEscape(user), url.PathEscape(password), host, port, dbname)
    }
    if dsn == "" {
        return nil, nil
    }
    connCfg, err := pgx.ParseConfig(dsn)
    if err != nil {
        return nil, fmt.Errorf("invalid target DSN: %w", err)
    }
    return &config.InstanceConfig{
        ID:      "cli-target",
        Name:    fmt.Sprintf("%s:%d", connCfg.Host, connCfg.Port),
        DSN:     dsn,
        Enabled: true,
    }, nil
}
```

### Build Verification

```bash
go build ./cmd/pgpulse-server
go test ./internal/storage/... -race -v
go test ./cmd/... ./internal/... -count=1
golangci-lint run ./cmd/... ./internal/...
```

---

## Specialist B — API & Auth

### Scope
1. Modify `internal/auth/middleware.go` — AuthMode, NewAuthMiddleware
2. Modify `internal/api/server.go` — liveMode fields, /system/mode endpoint, NullAlertHistoryStore
3. Create `internal/alert/nullstore.go` — NullAlertHistoryStore (if not inline)

### Step 0 — Read First
```bash
cat CLAUDE.md
cat internal/auth/middleware.go       # Current RequireAuth
cat internal/auth/middleware_test.go
cat internal/auth/rbac.go             # Roles, UserContextKey
cat internal/api/server.go            # Routes(), APIServer struct
cat internal/api/middleware.go         # Existing middleware stack
cat internal/api/auth.go              # Auth handlers (login, me, etc.)
cat internal/alert/store.go           # AlertHistoryStore interface
```

### Task 1: Auth Bypass — `internal/auth/middleware.go`

Add to the existing file:

```go
// AuthMode controls authentication behavior.
type AuthMode int

const (
    AuthEnabled  AuthMode = iota // Full JWT auth
    AuthDisabled                  // All requests treated as implicit admin
)

// NewAuthMiddleware returns the appropriate auth middleware based on mode.
func NewAuthMiddleware(jwtService *JWTService, mode AuthMode) func(http.Handler) http.Handler {
    if mode == AuthDisabled {
        return func(next http.Handler) http.Handler {
            return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                ctx := context.WithValue(r.Context(), UserContextKey, &User{
                    ID:       "implicit-admin",
                    Username: "admin",
                    Role:     RoleAdmin,
                })
                next.ServeHTTP(w, r.WithContext(ctx))
            })
        }
    }
    return RequireAuth(jwtService)
}
```

**Do NOT modify the existing `RequireAuth` function.** Add alongside it.

Check `UserContextKey` and `User` struct locations — they may be in `rbac.go` or `store.go`. Use the correct types and import paths.

### Task 1b: Auth Middleware Test — `internal/auth/middleware_test.go`

Add test cases:

```go
func TestNewAuthMiddleware_Disabled(t *testing.T) {
    // Create middleware with AuthDisabled
    // Send request without any token
    // Verify 200 OK (not 401)
    // Verify user in context is implicit admin with RoleAdmin
}

func TestNewAuthMiddleware_Enabled(t *testing.T) {
    // Create middleware with AuthEnabled + real JWTService
    // Send request without token → expect 401
    // Send request with valid token → expect 200
}
```

### Task 2: APIServer Extensions — `internal/api/server.go`

**Add fields to APIServer struct:**

```go
type APIServer struct {
    // ... existing fields ...
    liveMode        bool
    memoryRetention time.Duration
    authMode        auth.AuthMode
}
```

**Update the constructor** to accept these new parameters. Check the existing constructor signature and add parameters.

**Add the system mode handler:**

```go
func (s *APIServer) handleSystemMode(w http.ResponseWriter, r *http.Request) {
    resp := map[string]interface{}{
        "mode": "persistent",
    }
    if s.liveMode {
        resp["mode"] = "live"
        resp["retention"] = s.memoryRetention.String()
    }
    writeJSON(w, http.StatusOK, resp)
}
```

**Register in Routes():**

Find where public (non-auth) routes are registered. Add:

```go
r.Get("/api/v1/system/mode", s.handleSystemMode)
```

This must be OUTSIDE any auth middleware group.

**Auth handler behavior when auth disabled:**

When `authMode == AuthDisabled`:
- `GET /api/v1/auth/me` should return the implicit admin user (the middleware injects it, so this should work automatically)
- `POST /api/v1/auth/login` should return a success response or redirect (so the frontend doesn't break if it tries to log in)
- Registration and user management endpoints can return 404 or a message saying "auth is disabled"

Review `internal/api/auth.go` handlers. If they check `auth.enabled` config, that path should already handle this. If not, add a guard:

```go
if s.authMode == auth.AuthDisabled {
    writeJSON(w, http.StatusOK, map[string]interface{}{
        "user": map[string]string{"username": "admin", "role": "admin"},
        "message": "authentication disabled",
    })
    return
}
```

### Task 3: NullAlertHistoryStore — `internal/alert/nullstore.go`

Check `internal/alert/store.go` for the `AlertHistoryStore` interface. Create a null implementation:

```go
package alert

import "context"

// NullAlertHistoryStore discards all writes. Used in live mode
// where no persistent storage is available.
type NullAlertHistoryStore struct{}

func NewNullAlertHistoryStore() *NullAlertHistoryStore {
    return &NullAlertHistoryStore{}
}
```

Implement every method of `AlertHistoryStore` — return empty results for reads, nil for writes. Read the interface definition first to know the exact method signatures.

### Build Verification

```bash
go build ./cmd/pgpulse-server
go test ./internal/auth/... -race -v
go test ./internal/api/... -v
go test ./internal/alert/... -v
go test ./cmd/... ./internal/... -count=1
golangci-lint run ./cmd/... ./internal/...
```

---

## Specialist C — Frontend & Build

### Scope
1. Create `web/src/hooks/useSystemMode.ts` — system mode hook
2. Modify header component — Live Mode badge
3. Modify forecast/ML components — gate on mode
4. Modify login/auth routing — handle auth-disabled
5. Create `scripts/build-release.sh` — cross-compile + ZIP
6. Create `config.sample.yaml` — annotated sample config (in project root)
7. Create `README.txt` — quick-start guide (in project root)

### Step 0 — Read First
```bash
cat CLAUDE.md
ls web/src/
ls web/src/components/
ls web/src/hooks/
ls web/src/lib/
cat web/src/App.tsx                    # Router structure
cat web/src/lib/forecastUtils.ts       # Forecast utilities
# Find the main layout/header component:
grep -rl "header\|Header\|layout\|Layout" web/src/components/ --include="*.tsx" | head -10
# Find auth/login components:
grep -rl "login\|Login\|auth\|Auth" web/src/components/ --include="*.tsx" | head -10
# Find forecast toggle usage:
grep -rl "forecast\|Forecast" web/src/components/ --include="*.tsx" | head -10
```

### Task 1: System Mode Hook — `web/src/hooks/useSystemMode.ts`

```typescript
import { useState, useEffect, createContext, useContext } from 'react';

interface SystemMode {
  mode: 'live' | 'persistent';
  retention?: string;
  loading: boolean;
}

const SystemModeContext = createContext<SystemMode>({
  mode: 'persistent',
  loading: true,
});

export function useSystemMode(): SystemMode {
  return useContext(SystemModeContext);
}

export function SystemModeProvider({ children }: { children: React.ReactNode }) {
  const [state, setState] = useState<SystemMode>({ mode: 'persistent', loading: true });

  useEffect(() => {
    fetch('/api/v1/system/mode')
      .then(res => res.json())
      .then(data => setState({ mode: data.mode, retention: data.retention, loading: false }))
      .catch(() => setState({ mode: 'persistent', loading: false })); // fallback to persistent
  }, []);

  return (
    <SystemModeContext.Provider value={state}>
      {children}
    </SystemModeContext.Provider>
  );
}
```

Wrap the app root (in `App.tsx` or equivalent) with `<SystemModeProvider>`.

### Task 2: Live Mode Badge

Find the main header/layout component. Add the Live Mode indicator:

```tsx
import { useSystemMode } from '@/hooks/useSystemMode';

// Inside the header, near the logo or nav area:
const { mode, retention } = useSystemMode();

{mode === 'live' && (
  <div className="relative group">
    <span className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-medium bg-blue-500/10 text-blue-400 border border-blue-500/20">
      <span className="h-1.5 w-1.5 rounded-full bg-blue-400 animate-pulse" />
      Live Mode
    </span>
    <div className="absolute top-full left-0 mt-2 w-64 p-3 bg-gray-800 rounded-lg shadow-lg text-xs text-gray-300 opacity-0 group-hover:opacity-100 transition-opacity z-50">
      Metrics stored in memory for {retention || '2h'}. Add a storage database for persistent monitoring.
    </div>
  </div>
)}
```

**Use only Tailwind utility classes.** The badge should work in dark mode (the existing UI is dark-themed).

### Task 3: ML/Forecast Gating

Find all places where forecast/ML UI is rendered. Common patterns to search for:

```bash
grep -rn "forecast\|Forecast\|ForecastToggle\|showForecast" web/src/components/ --include="*.tsx"
grep -rn "anomal\|Anomal\|mlEnabled\|ml_enabled" web/src/components/ --include="*.tsx"
```

Wrap each forecast/ML control with a mode check:

```tsx
const { mode } = useSystemMode();

// Before rendering any forecast toggle, overlay, or ML panel:
if (mode === 'live') return null;
// OR
{mode !== 'live' && <ForecastToggle ... />}
```

**Do not remove any code.** Only add conditional rendering gates.

### Task 4: Auth Redirect

Find the login page/route. When auth is disabled (the `/api/v1/auth/me` endpoint returns an implicit admin), the login page should redirect to the main dashboard.

Check how auth state is currently managed — likely a React context or hook. The existing auth check probably already handles this if the user is "already logged in." Since the auth middleware now injects an implicit admin, calls to `/api/v1/auth/me` will succeed, and the frontend should treat the user as authenticated.

If the login page has a redirect-if-authenticated check, this should work automatically. Verify and fix if needed.

### Task 5: Build Script — `scripts/build-release.sh`

Create the cross-compile script as specified in the design doc (Section 8.1). Make it executable:

```bash
chmod +x scripts/build-release.sh
```

### Task 6: config.sample.yaml

Create in project root. Content as specified in design doc Section 8.2.

### Task 7: README.txt

Create in project root. Content as specified in design doc Section 8.3.

### Build Verification

```bash
cd web && npm run build && npm run typecheck && npm run lint && cd ..
go build ./cmd/pgpulse-server
```

---

## Final Integration Check

After all three specialists complete their work, verify:

```bash
# Full build
cd web && npm run build && npm run typecheck && npm run lint && cd ..
go build ./cmd/pgpulse-server
go test ./cmd/... ./internal/... -count=1
golangci-lint run ./cmd/... ./internal/...

# Cross-compile check
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o /dev/null ./cmd/pgpulse-server
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o /dev/null ./cmd/pgpulse-server
```

### Expected New/Modified Files

```
NEW:
  internal/storage/memory.go
  internal/storage/memory_test.go
  internal/alert/nullstore.go
  web/src/hooks/useSystemMode.ts
  scripts/build-release.sh
  config.sample.yaml
  README.txt

MODIFIED:
  cmd/pgpulse-server/main.go
  internal/auth/middleware.go
  internal/auth/middleware_test.go
  internal/api/server.go
  web/src/App.tsx (or equivalent — SystemModeProvider wrapper)
  web/src/components/Layout.tsx (or equivalent — Live Mode badge)
  [forecast/ML component files — conditional rendering]
  [login/auth component — redirect when auth disabled]
```
