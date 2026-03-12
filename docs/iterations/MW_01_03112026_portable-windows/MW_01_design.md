# MW_01 — Design Document

**Iteration:** MW_01
**Date:** 2026-03-11
**Based on:** MW_01_requirements.md

---

## DO NOT RE-DISCUSS

All decisions from the M8_11 handoff remain in force, plus:

- **D-MW-1:** In-memory ring buffer (MemoryStore) — not SQLite, not snapshot-only
- **D-MW-2:** 2-hour default retention, configurable via `--history`
- **D-MW-3:** CLI flags override YAML; YAML overrides defaults
- **D-MW-4:** `--target` DSN for single instance; multi-instance via YAML only
- **D-MW-5:** Auth auto-skipped on localhost; `--no-auth` flag for explicit opt-out
- **D-MW-6:** No storage DSN configured → MemoryStore automatically (live mode)
- **D-MW-7:** ZIP distribution: binary + config.sample.yaml + README.txt
- **D-MW-8:** 3 specialist agents: Storage & Config, API & Auth, Frontend & Build

---

## 1. Architecture Overview

```
                         ┌──────────────────────────┐
  pgpulse.exe            │  cmd/pgpulse-server/     │
  --target=DSN           │  main.go                 │
  --listen=:8989         │                          │
  --history=2h           │  CLI flag parsing (NEW)  │
  --no-auth              │  ↓                       │
                         │  Config merge:           │
                         │  flags > YAML > defaults │
                         │  ↓                       │
                         │  if no storage.dsn:      │
                         │    → MemoryStore (NEW)   │
                         │  else:                   │
                         │    → PGStore (existing)  │
                         │  ↓                       │
                         │  if localhost || no-auth: │
                         │    → skip JWT            │
                         │  else:                   │
                         │    → JWT auth (existing) │
                         └──────────────────────────┘
```

### Mode Detection Logic

```
storage.dsn configured?
  ├── YES → mode = "persistent"
  │         storage = PGStore (existing TimescaleDB path)
  │         all features enabled (ML, forecasting, alerting with history)
  │
  └── NO  → mode = "live"
            storage = MemoryStore (ring buffer, 2h default)
            disabled: ML/forecast, alert history persistence
            enabled: collectors, basic threshold alerting, all dashboards
```

---

## 2. MemoryStore — `internal/storage/memory.go`

### Data Structure

```go
package storage

import (
    "context"
    "sync"
    "time"

    "github.com/ios9000/PGPulse_01/internal/collector"
)

// MemoryStore implements collector.MetricStore using an in-memory ring buffer.
// Data expires automatically after the configured retention duration.
type MemoryStore struct {
    mu        sync.RWMutex
    retention time.Duration
    data      map[string][]collector.MetricPoint // key: "instanceID:metric:labelsHash"
    done      chan struct{}
}

func NewMemoryStore(retention time.Duration) *MemoryStore
func (m *MemoryStore) Write(ctx context.Context, points []collector.MetricPoint) error
func (m *MemoryStore) Query(ctx context.Context, query collector.MetricQuery) ([]collector.MetricPoint, error)
func (m *MemoryStore) Close() error
```

### Storage Key

The map key is a composite of `instanceID + metric + sorted labels`:

```go
func storageKey(instanceID, metric string, labels map[string]string) string {
    // Sort label keys, join as "k1=v1,k2=v2"
    // Return "instanceID:metric:labelHash"
}
```

Each map entry holds a slice of `MetricPoint` ordered by timestamp. `Write()` appends to the slice. The expiry goroutine periodically trims entries older than `retention`.

### Expiry Goroutine

Started by `NewMemoryStore()`, runs every 30 seconds:

```go
func (m *MemoryStore) expireLoop() {
    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()
    for {
        select {
        case <-ticker.C:
            m.expire()
        case <-m.done:
            return
        }
    }
}

func (m *MemoryStore) expire() {
    cutoff := time.Now().Add(-m.retention)
    m.mu.Lock()
    defer m.mu.Unlock()
    for key, points := range m.data {
        // Binary search for cutoff, trim prefix
        idx := sort.Search(len(points), func(i int) bool {
            return points[i].Timestamp.After(cutoff)
        })
        if idx == len(points) {
            delete(m.data, key)
        } else if idx > 0 {
            m.data[key] = points[idx:]
        }
    }
}
```

### Query Implementation

`Query()` filters by `InstanceID`, `Metric`, `Labels`, `Start`, `End`, and `Limit`:

```go
func (m *MemoryStore) Query(ctx context.Context, q collector.MetricQuery) ([]collector.MetricPoint, error) {
    m.mu.RLock()
    defer m.mu.RUnlock()

    var results []collector.MetricPoint

    for _, points := range m.data {
        for _, p := range points {
            if q.InstanceID != "" && p.InstanceID != q.InstanceID { continue }
            if q.Metric != "" && p.Metric != q.Metric { continue }
            if !matchLabels(p.Labels, q.Labels) { continue }
            if !q.Start.IsZero() && p.Timestamp.Before(q.Start) { continue }
            if !q.End.IsZero() && p.Timestamp.After(q.End) { continue }
            results = append(results, p)
        }
    }

    // Sort by timestamp ascending
    sort.Slice(results, func(i, j int) bool {
        return results[i].Timestamp.Before(results[j].Timestamp)
    })

    if q.Limit > 0 && len(results) > q.Limit {
        results = results[len(results)-q.Limit:]
    }

    return results, nil
}
```

**Note:** This is a linear scan. For the expected data volumes (2 hours × 30 keys × 3 instances × 10s interval ≈ 65,000 points total), this is perfectly fine. If we ever needed better query performance, we could add per-metric indexing, but YAGNI for now.

### Tests — `internal/storage/memory_test.go`

Required test cases:

1. `TestMemoryStore_WriteAndQuery` — write points, query back, verify order
2. `TestMemoryStore_QueryFilters` — filter by instanceID, metric, labels, time range
3. `TestMemoryStore_Expiry` — write points with old timestamps, wait for expiry, verify gone
4. `TestMemoryStore_Limit` — query with Limit, verify truncation
5. `TestMemoryStore_Concurrent` — goroutine safety: parallel writes + queries
6. `TestMemoryStore_Close` — verify expiry goroutine stops

---

## 3. CLI Flag Parsing — `cmd/pgpulse-server/main.go`

### Flag Definitions

Using Go standard library `flag` package (no new dependency):

```go
var (
    flagTarget         = flag.String("target", "", "PostgreSQL DSN for single-instance mode")
    flagTargetHost     = flag.String("target-host", "", "PostgreSQL host (alternative to --target)")
    flagTargetPort     = flag.Int("target-port", 5432, "PostgreSQL port")
    flagTargetUser     = flag.String("target-user", "pgpulse_monitor", "PostgreSQL user")
    flagTargetPassword = flag.String("target-password", "", "PostgreSQL password")
    flagTargetDBName   = flag.String("target-dbname", "postgres", "PostgreSQL database name")
    flagListen         = flag.String("listen", "", "HTTP listen address (overrides server.listen + server.port)")
    flagHistory        = flag.Duration("history", 2*time.Hour, "In-memory metric retention (live mode)")
    flagNoAuth         = flag.Bool("no-auth", false, "Disable authentication")
    flagConfig         = flag.String("config", "config.yaml", "Path to configuration file")
)
```

### Config Merge Logic

The existing `loadConfig()` function in main.go uses koanf. We extend it:

```
1. flag.Parse()
2. Load YAML from --config path (if file exists; not an error if missing)
3. Apply CLI overrides:
   - --listen   → server.listen (parsed as host:port, overrides both listen + port)
   - --no-auth  → auth.enabled = false
   - --history  → (stored in a local var, passed to MemoryStore constructor)
4. Synthesize instance from --target / --target-* flags:
   - If --target is set, parse DSN
   - Else if --target-host is set, build DSN from components
   - Create a single-instance config entry with id="cli-target", name=host:port
   - This REPLACES any instances[] from YAML
5. Storage auto-detection:
   - If config has no storage.dsn → set liveMode = true
```

### DSN Synthesis from --target

```go
func synthesizeCLIInstance(target, host string, port int, user, password, dbname string) (*config.InstanceConfig, error) {
    dsn := target
    if dsn == "" && host != "" {
        // Build DSN from components
        // Default sslmode=disable for convenience (matches dev environment)
        dsn = fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable",
            url.PathEscape(user), url.PathEscape(password), host, port, dbname)
    }
    if dsn == "" {
        return nil, nil // no CLI target, fall through to YAML
    }
    // Parse to extract host:port for display name
    cfg, err := pgx.ParseConfig(dsn)
    if err != nil {
        return nil, fmt.Errorf("invalid target DSN: %w", err)
    }
    return &config.InstanceConfig{
        ID:      "cli-target",
        Name:    fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
        DSN:     dsn,
        Enabled: true,
    }, nil
}
```

### Exit Conditions

```
- No --target AND no instances in YAML → exit with error:
  "No PostgreSQL instances configured. Use --target=<DSN> or provide a config file."

- Both --target and --target-host → exit with error:
  "--target and --target-host are mutually exclusive"
```

---

## 4. Auth Bypass — `internal/auth/middleware.go`

### Current State

The existing `RequireAuth` middleware in `internal/auth/middleware.go` checks the JWT token on every request. `auth.enabled` in config controls whether auth routes are registered.

### Changes

Add a new middleware constructor that wraps `RequireAuth`:

```go
// AuthMode determines the authentication behavior.
type AuthMode int

const (
    AuthEnabled  AuthMode = iota // Full JWT auth (existing behavior)
    AuthDisabled                  // Skip auth, treat all requests as admin
)

// NewAuthMiddleware returns middleware based on the auth mode.
func NewAuthMiddleware(jwtService *JWTService, mode AuthMode) func(http.Handler) http.Handler {
    if mode == AuthDisabled {
        return func(next http.Handler) http.Handler {
            return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                // Inject an implicit admin user into context
                ctx := context.WithValue(r.Context(), UserContextKey, &User{
                    ID:       "implicit-admin",
                    Username: "admin",
                    Role:     RoleAdmin,
                })
                next.ServeHTTP(w, r.WithContext(ctx))
            })
        }
    }
    return RequireAuth(jwtService) // existing behavior
}
```

### Mode Detection in main.go

```go
func resolveAuthMode(cfg *config.Config, listenAddr string, noAuthFlag bool) auth.AuthMode {
    if noAuthFlag {
        slog.Info("auth disabled", "reason", "no-auth flag")
        return auth.AuthDisabled
    }
    host, _, _ := net.SplitHostPort(listenAddr)
    if host == "" || host == "127.0.0.1" || host == "localhost" || host == "::1" {
        slog.Info("auth disabled", "reason", "localhost binding")
        return auth.AuthDisabled
    }
    if !cfg.Auth.Enabled {
        slog.Info("auth disabled", "reason", "config auth.enabled=false")
        return auth.AuthDisabled
    }
    slog.Info("auth enabled")
    return auth.AuthEnabled
}
```

**Important:** When auth is disabled, the login page must not be shown. The frontend should redirect `/login` to `/` and the API should still respond to `/api/v1/auth/me` with the implicit admin user (so the frontend's auth check doesn't break).

---

## 5. System Mode Endpoint — `internal/api/server.go`

### New Endpoint

```go
// GET /api/v1/system/mode — no auth required
func (s *APIServer) handleSystemMode(w http.ResponseWriter, r *http.Request) {
    mode := "persistent"
    resp := map[string]interface{}{"mode": mode}

    if s.liveMode {
        mode = "live"
        resp["mode"] = mode
        resp["retention"] = s.memoryRetention.String()
    }

    writeJSON(w, http.StatusOK, resp)
}
```

Register outside the auth-protected group in Routes():

```go
r.Get("/api/v1/system/mode", s.handleSystemMode) // no auth
```

### APIServer Extension

Add fields to `APIServer`:

```go
type APIServer struct {
    // ... existing fields ...
    liveMode        bool
    memoryRetention time.Duration
}
```

These are set during construction in `main.go`.

---

## 6. Feature Gating in Live Mode

When `liveMode = true`, certain subsystems are either not started or return graceful "not available" responses:

| Subsystem | Live Mode Behavior |
|-----------|-------------------|
| Collectors | All run normally — write to MemoryStore |
| Basic threshold alerts | Evaluate on each cycle (in-memory state only) |
| Alert history persistence | Disabled (no alert history store) |
| ML / anomaly detection | Not started (requires days of baseline) |
| Forecast endpoint | Returns 404 or `{"error": "forecasting requires persistent storage"}` |
| Forecast alerts | Not evaluated |
| Plan capture | Disabled (requires storage DB for plan table) |
| Settings snapshots | Disabled (requires storage DB) |
| User management | Disabled when auth is off (implicit admin) |

### Implementation

In `main.go`, the startup sequence already has conditional blocks for `ml.enabled`, `alerting.enabled`, etc. In live mode:

```go
if liveMode {
    // Override configs that require persistent storage
    cfg.ML.Enabled = false
    cfg.PlanCapture.Enabled = false
    cfg.SettingsSnapshot.Enabled = false
    // alerting.enabled can stay true for basic threshold alerts
    // but alert history store becomes a no-op
}
```

For alert history, introduce a `NullAlertHistoryStore` that silently discards writes:

```go
type NullAlertHistoryStore struct{}
func (n *NullAlertHistoryStore) Save(ctx context.Context, event AlertEvent) error { return nil }
func (n *NullAlertHistoryStore) Query(...) ([]AlertEvent, error) { return nil, nil }
```

---

## 7. Frontend Changes

### 7.1 System Mode Hook — `web/src/hooks/useSystemMode.ts`

```typescript
interface SystemMode {
  mode: 'live' | 'persistent';
  retention?: string;
}

export function useSystemMode(): SystemMode {
  // Fetch GET /api/v1/system/mode once on app load
  // Cache in React context or SWR
}
```

### 7.2 Live Mode Badge — Header Component

In the main layout header (likely `web/src/components/Layout.tsx` or similar):

```tsx
{systemMode.mode === 'live' && (
  <Tooltip content={`Metrics stored in memory for ${systemMode.retention}. Add a storage database for persistent monitoring.`}>
    <span className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-medium bg-blue-500/10 text-blue-400 border border-blue-500/20">
      <span className="h-1.5 w-1.5 rounded-full bg-blue-400 animate-pulse" />
      Live Mode
    </span>
  </Tooltip>
)}
```

### 7.3 ML/Forecast Visibility

Anywhere ML/forecast controls appear (forecast overlay toggle, ML settings panel):

```tsx
const { mode } = useSystemMode();
if (mode === 'live') return null; // hide entirely
```

Specific locations to gate:
- Forecast toggle button on metric charts (`web/src/components/charts/`)
- Forecast overlay rendering in `forecastUtils.ts` consumers
- ML/Anomaly section in settings or sidebar (if visible)
- Forecast alert rules in alert configuration UI

### 7.4 Auth Redirect

When auth is disabled, the frontend's login route should redirect to the dashboard:

```tsx
// In router or login component
const { mode } = useSystemMode();
// Also check: if /api/v1/auth/me returns an implicit admin, skip login
```

The existing auth flow likely already handles this if `auth.enabled` is false — agents should verify and fix if needed.

### 7.5 Shallow History Handling

Charts should not error when data is less than their default time window. If a chart requests "last 1 hour" but only 10 minutes of data exist, it should render the 10 minutes without error or empty-state messages. This should already work given ECharts behavior, but agents should verify.

---

## 8. Build & Packaging

### 8.1 Build Script — `scripts/build-release.sh`

```bash
#!/usr/bin/env bash
set -euo pipefail

VERSION=${1:-"dev"}
BUILD_DIR="build/release"
rm -rf "$BUILD_DIR"
mkdir -p "$BUILD_DIR"

# Build frontend first
echo "=== Building frontend ==="
(cd web && npm run build)

# Build for each target
for GOOS_GOARCH in linux/amd64 windows/amd64; do
    GOOS="${GOOS_GOARCH%/*}"
    GOARCH="${GOOS_GOARCH#*/}"
    EXT=""
    [[ "$GOOS" == "windows" ]] && EXT=".exe"

    BINARY="pgpulse-server${EXT}"
    OUT_DIR="${BUILD_DIR}/pgpulse-${GOOS}-${GOARCH}"
    mkdir -p "$OUT_DIR"

    echo "=== Building ${GOOS}/${GOARCH} ==="
    CGO_ENABLED=0 GOOS="$GOOS" GOARCH="$GOARCH" \
        go build -ldflags="-s -w -X main.version=${VERSION}" \
        -o "${OUT_DIR}/${BINARY}" ./cmd/pgpulse-server

    # Include support files
    cp config.sample.yaml "${OUT_DIR}/"
    cp README.txt "${OUT_DIR}/"

    # Create ZIP
    (cd "$BUILD_DIR" && zip -r "pgpulse-${VERSION}-${GOOS}-${GOARCH}.zip" "pgpulse-${GOOS}-${GOARCH}/")

    echo "=== Created pgpulse-${VERSION}-${GOOS}-${GOARCH}.zip ==="
done

echo "=== Release build complete ==="
ls -lh "${BUILD_DIR}"/*.zip
```

### 8.2 config.sample.yaml

```yaml
# PGPulse Configuration
# =====================
#
# QUICK START (portable / diagnostic mode):
#   Just use CLI flags — no config file needed:
#   pgpulse-server --target=postgres://user:pass@host:5432/postgres
#
# This file is for PERSISTENT mode or multi-instance monitoring.
#
# Precedence: CLI flags > this file > built-in defaults

server:
  # listen: ""          # Bind address (default: all interfaces)
  port: 8989            # Web UI port
  # log_level: "info"   # debug, info, warn, error

# Storage (OPTIONAL — omit for live/diagnostic mode)
# When omitted, PGPulse runs in Live Mode with in-memory storage.
# storage:
#   dsn: "postgres://pgpulse:secret@localhost:5432/pgpulse_storage?sslmode=disable"
#   use_timescaledb: true
#   retention_days: 30

# Authentication (auto-disabled on localhost)
# auth:
#   enabled: true
#   jwt_secret: "change-me-to-a-random-string"
#   initial_admin:
#     username: admin
#     password: pgpulse_admin

# Instances (for multi-server monitoring)
# When using --target CLI flag, this section is ignored.
# instances:
#   - id: primary
#     name: "Production Primary"
#     dsn: "postgres://pgpulse_monitor:pass@10.0.0.1:5432/postgres?sslmode=disable"
#     enabled: true
#   - id: replica
#     name: "Streaming Replica"
#     dsn: "postgres://pgpulse_monitor:pass@10.0.0.2:5433/postgres?sslmode=disable"
#     enabled: true

# Alerting (works in both live and persistent modes)
# alerting:
#   enabled: true

# ML / Anomaly Detection (requires persistent storage)
# ml:
#   enabled: true
```

### 8.3 README.txt

```
PGPulse — PostgreSQL Health & Activity Monitor
===============================================

QUICK START
-----------

1. Open a terminal in this folder

2. Run (replace with your PostgreSQL connection details):

   pgpulse-server --target=postgres://pgpulse_monitor:password@your-pg-host:5432/postgres

3. Open http://localhost:8989 in your browser

4. You're monitoring! Charts will fill up over the next few minutes.


CLI FLAGS
---------

  --target=<DSN>          PostgreSQL connection string (required, or use config)
  --target-host=<host>    PostgreSQL host (alternative to --target)
  --target-port=<port>    PostgreSQL port (default: 5432)
  --target-user=<user>    PostgreSQL user (default: pgpulse_monitor)
  --target-password=<pw>  PostgreSQL password
  --target-dbname=<db>    PostgreSQL database (default: postgres)
  --listen=<addr>         Web UI address (default: :8989)
  --history=<duration>    Memory retention (default: 2h, e.g., 4h, 30m)
  --no-auth               Disable login (auto-disabled on localhost)
  --config=<path>         Config file path (default: config.yaml)


LIVE MODE vs PERSISTENT MODE
-----------------------------

By default (no storage database), PGPulse runs in Live Mode:
  - Metrics accumulate in memory for the --history duration
  - When PGPulse stops, data is gone — no cleanup needed
  - ML/forecasting features are disabled (need historical data)
  - Perfect for diagnostics: "connect, diagnose, disconnect"

To enable Persistent Mode with full features:
  1. Create a storage database:  CREATE DATABASE pgpulse_storage;
  2. (Optional) Install TimescaleDB extension
  3. Add to config.yaml:
     storage:
       dsn: "postgres://pgpulse:secret@localhost:5432/pgpulse_storage"
  4. Restart PGPulse — it auto-creates tables and enables all features


REQUIRED POSTGRESQL PERMISSIONS
-------------------------------

Minimum role setup (run as superuser on the target instance):

  CREATE ROLE pgpulse_monitor WITH LOGIN PASSWORD 'your-password';
  GRANT pg_monitor TO pgpulse_monitor;

For OS metrics via SQL (Linux targets only):

  GRANT pg_read_server_files TO pgpulse_monitor;
  GRANT EXECUTE ON FUNCTION pg_read_file(text) TO pgpulse_monitor;
  GRANT EXECUTE ON FUNCTION pg_read_file(text, bigint, bigint) TO pgpulse_monitor;
  GRANT EXECUTE ON FUNCTION pg_read_file(text, bigint, bigint, boolean) TO pgpulse_monitor;
```

---

## 9. Files Created / Modified

### New Files

| File | Owner | Purpose |
|------|-------|---------|
| `internal/storage/memory.go` | Specialist A | MemoryStore implementation |
| `internal/storage/memory_test.go` | Specialist A | MemoryStore unit tests |
| `scripts/build-release.sh` | Specialist C | Cross-compile + ZIP script |
| `config.sample.yaml` | Specialist C | Annotated sample config |
| `README.txt` | Specialist C | Quick-start guide |
| `web/src/hooks/useSystemMode.ts` | Specialist C | System mode React hook |

### Modified Files

| File | Owner | Changes |
|------|-------|---------|
| `cmd/pgpulse-server/main.go` | Specialist A | CLI flag parsing, config merge, MemoryStore instantiation, live mode wiring |
| `internal/auth/middleware.go` | Specialist B | `AuthMode` enum, `NewAuthMiddleware()` constructor |
| `internal/api/server.go` | Specialist B | `liveMode` + `memoryRetention` fields, `/system/mode` endpoint, `NullAlertHistoryStore` |
| `internal/api/server.go` (Routes) | Specialist B | Register `/api/v1/system/mode` outside auth group |
| `web/src/components/Layout.tsx` (or equivalent) | Specialist C | Live Mode badge in header |
| Chart components with forecast toggle | Specialist C | Gate on `mode !== 'live'` |
| Login / auth routing | Specialist C | Redirect when auth disabled |

---

## 10. Dependency Order

```
Specialist A (Storage & Config)  ─────────────────────────────────────────►
  Creates: MemoryStore, CLI flags, config merge, live mode detection

Specialist B (API & Auth)        ─────────────────────────────────────────►
  Creates: AuthMode middleware, /system/mode endpoint, NullAlertHistoryStore
  Reads: liveMode flag (set by Specialist A's code, but B codes against bool)

Specialist C (Frontend & Build)  ─────────────────────────────────────────►
  Creates: useSystemMode hook, Live Mode badge, feature gating, build script
  Reads: /api/v1/system/mode response shape (defined in this doc)
```

All three run in **parallel worktrees**. No blocking dependencies — they code against shared contracts:
- MemoryStore implements the existing `MetricStore` interface (already defined)
- `/api/v1/system/mode` returns `{"mode": "live"|"persistent", "retention?": "2h"}`
- `liveMode` is a `bool` passed to `APIServer` constructor

---

## 11. Testing Strategy

| Component | Test Type | Notes |
|-----------|-----------|-------|
| MemoryStore | Unit tests | Write/Query/Expiry/Concurrency/Close |
| CLI flag parsing | Manual + build test | Hard to unit test flag.Parse; verify via integration |
| Auth bypass | Unit test | Test NewAuthMiddleware with AuthDisabled, verify context injection |
| /system/mode | Unit test (httptest) | Verify JSON response in both modes |
| NullAlertHistoryStore | Trivial — verify interface compliance | |
| Frontend Live Mode badge | Visual verification | Launch in live mode, check UI |
| Frontend forecast gating | Visual verification | Confirm ML controls hidden in live mode |
| Cross-compile | Build test | `GOOS=windows go build` must succeed |

### Build Verification Sequence

```bash
# Frontend
cd web && npm run build && npm run typecheck && npm run lint && cd ..

# Backend
go build ./cmd/pgpulse-server
go test ./cmd/... ./internal/...
golangci-lint run ./cmd/... ./internal/...

# Cross-compile check
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o /dev/null ./cmd/pgpulse-server
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o /dev/null ./cmd/pgpulse-server
```
