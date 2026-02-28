# M2_03 — REST API & Wiring: Design

**Iteration:** M2_03  
**Milestone:** M2 — Storage & API  
**Date:** 2026-02-27  
**Session type:** Single Claude Code (Sonnet) — focused scope, ~6 files

---

## Architecture Overview

```
                         HTTP Clients
                              │
                              ▼
                    ┌──────────────────┐
                    │   chi.Router     │
                    │                  │
                    │  RequestID       │
                    │  Logger          │
                    │  Recoverer       │
                    │  CORS (optional) │
                    │  AuthStub        │
                    └────────┬─────────┘
                             │
              ┌──────────────┼──────────────┐
              ▼              ▼              ▼
         /health      /instances    /instances/{id}/metrics
              │              │              │
              ▼              ▼              ▼
          Ping pool     Config lookup   MetricStore.Query()
                                            │
                                     ┌──────┴──────┐
                                     │   JSON      CSV
                                     │  encoder   writer
                                     └─────────────┘
```

### Dependency Wiring (main.go additions)

```
main.go (updated flow)
  → config.Load(path)
  → store = PGStore | LogStore (existing)
  → orch = orchestrator.New(cfg, store, logger) (existing)
  → apiServer = api.New(cfg, store, logger)        ← NEW
  → httpServer = &http.Server{
      Addr:         cfg.Server.Address,
      Handler:      apiServer.Routes(),
      ReadTimeout:  cfg.Server.ReadTimeout,
      WriteTimeout: cfg.Server.WriteTimeout,
    }
  → go httpServer.ListenAndServe()                 ← NEW goroutine
  → orch.Start(ctx)                                (existing)
  → signal.Notify(SIGINT, SIGTERM)
  → httpServer.Shutdown(shutdownCtx)               ← NEW
  → orch.Stop()
  → store.Close()
```

---

## File Plan

| File | Action | Lines (est.) |
|------|--------|-------------|
| `internal/api/server.go` | Create | ~90 |
| `internal/api/health.go` | Create | ~60 |
| `internal/api/instances.go` | Create | ~80 |
| `internal/api/metrics.go` | Create | ~140 |
| `internal/api/middleware.go` | Create | ~100 |
| `internal/api/response.go` | Create | ~80 |
| `internal/config/config.go` | Update | +15 |
| `cmd/pgpulse-server/main.go` | Update | +40 |
| `internal/api/server_test.go` | Create | ~50 |
| `internal/api/health_test.go` | Create | ~80 |
| `internal/api/instances_test.go` | Create | ~100 |
| `internal/api/metrics_test.go` | Create | ~180 |
| `internal/api/middleware_test.go` | Create | ~60 |
| **Total new** | | **~1080** |

---

## Structs & Interfaces

### APIServer (internal/api/server.go)

```go
package api

import (
    "log/slog"
    "net/http"
    "time"

    "github.com/go-chi/chi/v5"
    "github.com/ios9000/PGPulse_01/internal/collector"
    "github.com/ios9000/PGPulse_01/internal/config"
)

// Version is set at build time or defaults to "dev".
var Version = "0.1.0-dev"

// APIServer holds dependencies for all HTTP handlers.
type APIServer struct {
    store     collector.MetricStore
    instances []config.InstanceConfig
    logger    *slog.Logger
    startTime time.Time
    pool      Pinger // nil when using LogStore
}

// Pinger is satisfied by *pgxpool.Pool. Allows mock in tests.
type Pinger interface {
    Ping(ctx context.Context) error
}

// New creates an APIServer. pool may be nil (LogStore mode).
func New(cfg config.Config, store collector.MetricStore, pool Pinger, logger *slog.Logger) *APIServer {
    return &APIServer{
        store:     store,
        instances: cfg.Instances,
        logger:    logger,
        startTime: time.Now(),
        pool:      pool,
    }
}

// Routes builds the chi router with all middleware and endpoints.
func (s *APIServer) Routes() http.Handler {
    r := chi.NewRouter()

    // Middleware stack (order matters)
    r.Use(requestIDMiddleware)
    r.Use(loggerMiddleware(s.logger))
    r.Use(recovererMiddleware(s.logger))
    if cfg.Server.CORSEnabled {
        r.Use(corsMiddleware)
    }
    r.Use(authStubMiddleware)

    // Routes
    r.Get("/api/v1/health", s.handleHealth)
    r.Get("/api/v1/instances", s.handleListInstances)
    r.Get("/api/v1/instances/{id}", s.handleGetInstance)
    r.Get("/api/v1/instances/{id}/metrics", s.handleQueryMetrics)

    return r
}
```

> **Note:** The `Routes()` method needs access to `CORSEnabled`. Two options: (a) pass the full config into `APIServer`, or (b) pass just the boolean. Design picks (a) — store `cfg.Server` as a field, avoids parameter proliferation as we add more server config later.

Revised struct:

```go
type APIServer struct {
    store      collector.MetricStore
    instances  []config.InstanceConfig
    serverCfg  config.ServerConfig
    logger     *slog.Logger
    startTime  time.Time
    pool       Pinger
}
```

---

## Response Helpers (internal/api/response.go)

```go
package api

// Envelope wraps successful responses.
type Envelope struct {
    Data any `json:"data"`
    Meta any `json:"meta,omitempty"`
}

// ErrorResponse wraps error responses.
type ErrorResponse struct {
    Error ErrorBody `json:"error"`
}

type ErrorBody struct {
    Code    string `json:"code"`
    Message string `json:"message"`
}

// writeJSON encodes v as JSON with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    json.NewEncoder(w).Encode(v)
}

// writeError writes a standard error response.
func writeError(w http.ResponseWriter, status int, code, message string) {
    writeJSON(w, status, ErrorResponse{
        Error: ErrorBody{Code: code, Message: message},
    })
}
```

---

## Endpoint Implementations

### Health (internal/api/health.go)

```go
type HealthResponse struct {
    Status  string `json:"status"`          // "ok" or "error"
    Storage string `json:"storage"`         // "ok", "error", or "disabled"
    Uptime  string `json:"uptime"`          // e.g. "2h35m12s"
    Version string `json:"version"`         // e.g. "0.1.0-dev"
}

func (s *APIServer) handleHealth(w http.ResponseWriter, r *http.Request) {
    resp := HealthResponse{
        Status:  "ok",
        Uptime:  time.Since(s.startTime).Truncate(time.Second).String(),
        Version: Version,
    }

    if s.pool == nil {
        resp.Storage = "disabled"
    } else {
        ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
        defer cancel()
        if err := s.pool.Ping(ctx); err != nil {
            resp.Storage = "error"
            resp.Status = "error"
        } else {
            resp.Storage = "ok"
        }
    }

    status := http.StatusOK
    if resp.Status == "error" {
        status = http.StatusServiceUnavailable
    }
    writeJSON(w, status, resp)
}
```

### List Instances (internal/api/instances.go)

```go
type InstanceResponse struct {
    ID          string `json:"id"`
    Host        string `json:"host"`
    Port        int    `json:"port"`
    Description string `json:"description,omitempty"`
    Enabled     bool   `json:"enabled"`
}

func (s *APIServer) handleListInstances(w http.ResponseWriter, r *http.Request) {
    items := make([]InstanceResponse, 0, len(s.instances))
    for _, inst := range s.instances {
        items = append(items, toInstanceResponse(inst))
    }
    writeJSON(w, http.StatusOK, Envelope{
        Data: items,
        Meta: map[string]int{"count": len(items)},
    })
}

func (s *APIServer) handleGetInstance(w http.ResponseWriter, r *http.Request) {
    id := chi.URLParam(r, "id")
    for _, inst := range s.instances {
        if inst.ID == id {
            writeJSON(w, http.StatusOK, Envelope{Data: toInstanceResponse(inst)})
            return
        }
    }
    writeError(w, http.StatusNotFound, "not_found",
        fmt.Sprintf("instance '%s' not found", id))
}

func toInstanceResponse(ic config.InstanceConfig) InstanceResponse {
    return InstanceResponse{
        ID:          ic.ID,
        Host:        ic.Host,
        Port:        ic.Port,
        Description: ic.Description,
        Enabled:     ic.Enabled,
    }
}
```

> **Note:** `config.InstanceConfig` needs a `Description` field. Currently has: ID, Host, Port, Database, Enabled, CollectorsEnabled. Add `Description string \`koanf:"description"\`` — backward compatible, YAML field is optional.

### Query Metrics (internal/api/metrics.go)

```go
func (s *APIServer) handleQueryMetrics(w http.ResponseWriter, r *http.Request) {
    instanceID := chi.URLParam(r, "id")

    // Validate instance exists
    if !s.instanceExists(instanceID) {
        writeError(w, http.StatusNotFound, "not_found",
            fmt.Sprintf("instance '%s' not found", instanceID))
        return
    }

    // Parse query params
    q, err := parseMetricQuery(r, instanceID)
    if err != nil {
        writeError(w, http.StatusBadRequest, "bad_request", err.Error())
        return
    }

    // Query storage
    points, err := s.store.Query(r.Context(), q)
    if err != nil {
        s.logger.ErrorContext(r.Context(), "metrics query failed",
            "instance_id", instanceID, "error", err)
        writeError(w, http.StatusInternalServerError, "internal_error",
            "failed to query metrics")
        return
    }

    // Determine output format
    format := resolveFormat(r)

    if format == "csv" {
        writeCSV(w, points, q)
        return
    }

    writeJSON(w, http.StatusOK, Envelope{
        Data: points,
        Meta: map[string]any{
            "count":       len(points),
            "instance_id": instanceID,
            "query": map[string]any{
                "metric": q.Metric,
                "start":  q.Start.Format(time.RFC3339),
                "end":    q.End.Format(time.RFC3339),
                "limit":  q.Limit,
            },
        },
    })
}

// parseMetricQuery extracts and validates query params.
func parseMetricQuery(r *http.Request, instanceID string) (collector.MetricQuery, error) {
    now := time.Now()
    q := collector.MetricQuery{
        InstanceID: instanceID,
        Metric:     r.URL.Query().Get("metric"),
        Start:      now.Add(-1 * time.Hour),
        End:        now,
        Limit:      1000,
    }

    if s := r.URL.Query().Get("start"); s != "" {
        t, err := time.Parse(time.RFC3339, s)
        if err != nil {
            return q, fmt.Errorf("invalid 'start' parameter: %w", err)
        }
        q.Start = t
    }

    if s := r.URL.Query().Get("end"); s != "" {
        t, err := time.Parse(time.RFC3339, s)
        if err != nil {
            return q, fmt.Errorf("invalid 'end' parameter: %w", err)
        }
        q.End = t
    }

    if s := r.URL.Query().Get("limit"); s != "" {
        n, err := strconv.Atoi(s)
        if err != nil || n < 1 || n > 10000 {
            return q, fmt.Errorf("invalid 'limit' parameter: must be 1-10000")
        }
        q.Limit = n
    }

    return q, nil
}

// resolveFormat checks ?format= param first, then Accept header.
func resolveFormat(r *http.Request) string {
    if f := r.URL.Query().Get("format"); f == "csv" {
        return "csv"
    }
    if strings.Contains(r.Header.Get("Accept"), "text/csv") {
        return "csv"
    }
    return "json"
}

// writeCSV writes metric points as CSV.
func writeCSV(w http.ResponseWriter, points []collector.MetricPoint, q collector.MetricQuery) {
    w.Header().Set("Content-Type", "text/csv")
    w.Header().Set("Content-Disposition", `attachment; filename="metrics.csv"`)
    w.WriteHeader(http.StatusOK)

    cw := csv.NewWriter(w)
    defer cw.Flush()

    // Header row
    cw.Write([]string{"instance_id", "metric", "value", "labels", "timestamp"})

    for _, p := range points {
        labelsJSON, _ := json.Marshal(p.Labels)
        cw.Write([]string{
            p.InstanceID,
            p.Metric,
            strconv.FormatFloat(p.Value, 'f', 6, 64),
            string(labelsJSON),
            p.Timestamp.Format(time.RFC3339),
        })
    }
}
```

---

## Middleware (internal/api/middleware.go)

### Request ID

```go
type ctxKey string
const requestIDKey ctxKey = "request_id"
const userKey      ctxKey = "user"

func requestIDMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        id := r.Header.Get("X-Request-ID")
        if id == "" {
            id = generateID() // crypto/rand based, 16 hex chars
        }
        w.Header().Set("X-Request-ID", id)
        ctx := context.WithValue(r.Context(), requestIDKey, id)
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}
```

### Logger

```go
func loggerMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            start := time.Now()
            ww := &responseWriter{ResponseWriter: w, status: 200}
            next.ServeHTTP(ww, r)
            reqID, _ := r.Context().Value(requestIDKey).(string)
            logger.Info("http request",
                "method", r.Method,
                "path", r.URL.Path,
                "status", ww.status,
                "duration_ms", time.Since(start).Milliseconds(),
                "request_id", reqID,
            )
        })
    }
}

// responseWriter wraps http.ResponseWriter to capture status code.
type responseWriter struct {
    http.ResponseWriter
    status int
}

func (rw *responseWriter) WriteHeader(code int) {
    rw.status = code
    rw.ResponseWriter.WriteHeader(code)
}
```

### Recoverer

```go
func recovererMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            defer func() {
                if rv := recover(); rv != nil {
                    logger.Error("panic recovered",
                        "error", rv,
                        "stack", string(debug.Stack()),
                    )
                    writeError(w, http.StatusInternalServerError,
                        "internal_error", "internal server error")
                }
            }()
            next.ServeHTTP(w, r)
        })
    }
}
```

### CORS

```go
func corsMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Access-Control-Allow-Origin", "*")
        w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
        w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Request-ID")
        w.Header().Set("Access-Control-Max-Age", "86400")

        if r.Method == http.MethodOptions {
            w.WriteHeader(http.StatusNoContent)
            return
        }
        next.ServeHTTP(w, r)
    })
}
```

### Auth Stub

```go
func authStubMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        ctx := context.WithValue(r.Context(), userKey, "anonymous")
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}

// UserFromContext retrieves the authenticated user. Used by handlers now,
// becomes real when M3 swaps in JWT middleware.
func UserFromContext(ctx context.Context) string {
    if u, ok := ctx.Value(userKey).(string); ok {
        return u
    }
    return "anonymous"
}
```

---

## Config Changes

Add to `config.ServerConfig`:

```go
type ServerConfig struct {
    Address         string        `koanf:"address"`          // existing, default ":8080"
    CORSEnabled     bool          `koanf:"cors_enabled"`     // NEW, default false
    ReadTimeout     time.Duration `koanf:"read_timeout"`     // NEW, default 30s
    WriteTimeout    time.Duration `koanf:"write_timeout"`    // NEW, default 60s
    ShutdownTimeout time.Duration `koanf:"shutdown_timeout"` // NEW, default 10s
}
```

Add to `config.InstanceConfig`:

```go
type InstanceConfig struct {
    // ... existing fields ...
    Description string `koanf:"description"` // NEW, optional
}
```

Add defaults in `validate()`:

```go
if c.Server.ReadTimeout == 0 {
    c.Server.ReadTimeout = 30 * time.Second
}
if c.Server.WriteTimeout == 0 {
    c.Server.WriteTimeout = 60 * time.Second
}
if c.Server.ShutdownTimeout == 0 {
    c.Server.ShutdownTimeout = 10 * time.Second
}
```

---

## main.go Updates

```go
func main() {
    // ... existing config load, store setup, orchestrator setup ...

    // NEW: Build API server
    var pool api.Pinger
    if pgStore, ok := store.(*storage.PGStore); ok {
        pool = pgStore.Pool() // Need to expose Pool() on PGStore
    }
    apiServer := api.New(cfg, store, pool, logger)

    httpServer := &http.Server{
        Addr:         cfg.Server.Address,
        Handler:      apiServer.Routes(),
        ReadTimeout:  cfg.Server.ReadTimeout,
        WriteTimeout: cfg.Server.WriteTimeout,
    }

    // Start HTTP server in background
    go func() {
        logger.Info("starting HTTP server", "address", cfg.Server.Address)
        if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            logger.Error("HTTP server error", "error", err)
        }
    }()

    // Start orchestrator (existing)
    orch.Start(ctx)

    // Wait for shutdown signal (existing signal handling)
    <-sigCh

    // Graceful shutdown — NEW ordering
    shutdownCtx, shutdownCancel := context.WithTimeout(
        context.Background(), cfg.Server.ShutdownTimeout)
    defer shutdownCancel()

    logger.Info("shutting down HTTP server")
    if err := httpServer.Shutdown(shutdownCtx); err != nil {
        logger.Error("HTTP shutdown error", "error", err)
    }

    logger.Info("stopping orchestrator")
    orch.Stop()

    logger.Info("closing storage")
    store.Close()
}
```

### PGStore: Expose Pool

Add to `internal/storage/pgstore.go`:

```go
// Pool returns the underlying connection pool. Used by health checks.
func (s *PGStore) Pool() *pgxpool.Pool {
    return s.pool
}
```

---

## Test Strategy

All tests use `httptest.NewRecorder()` + chi router. No real HTTP server or PostgreSQL needed.

### Mock MetricStore

```go
type mockStore struct {
    points []collector.MetricPoint
    err    error
}

func (m *mockStore) Write(ctx context.Context, points []collector.MetricPoint) error {
    return m.err
}

func (m *mockStore) Query(ctx context.Context, q collector.MetricQuery) ([]collector.MetricPoint, error) {
    return m.points, m.err
}

func (m *mockStore) Close() error { return nil }
```

### Mock Pinger

```go
type mockPinger struct {
    err error
}

func (m *mockPinger) Ping(ctx context.Context) error { return m.err }
```

### Test Cases

**health_test.go:**
- TestHealth_AllOK — pool ping succeeds → 200, status=ok, storage=ok
- TestHealth_StorageError — pool ping fails → 503, status=error, storage=error
- TestHealth_NoStorage — pool=nil → 200, status=ok, storage=disabled
- TestHealth_ContainsVersion — response has version string
- TestHealth_ContainsUptime — response has non-empty uptime

**instances_test.go:**
- TestListInstances_Empty — no instances → 200, data=[], count=0
- TestListInstances_Multiple — 3 instances → 200, data=[3], count=3
- TestGetInstance_Found — existing ID → 200, correct fields
- TestGetInstance_NotFound — bad ID → 404, error envelope
- TestListInstances_FieldMapping — verifies Description, Enabled mapping

**metrics_test.go:**
- TestQueryMetrics_DefaultParams — no params → last hour, limit 1000
- TestQueryMetrics_CustomTimeRange — start/end parsed correctly
- TestQueryMetrics_InvalidStart — bad date → 400
- TestQueryMetrics_InvalidLimit — negative → 400, 99999 → 400
- TestQueryMetrics_InstanceNotFound — bad ID → 404
- TestQueryMetrics_StorageError — store returns error → 500
- TestQueryMetrics_JSONFormat — default → JSON with envelope
- TestQueryMetrics_CSVFormat — ?format=csv → CSV content-type, correct rows
- TestQueryMetrics_CSVAcceptHeader — Accept: text/csv → CSV output
- TestQueryMetrics_EmptyResult — no points → 200, data=[], count=0

**middleware_test.go:**
- TestRequestID_Generated — no header → ID generated and set
- TestRequestID_Passthrough — existing header → preserved
- TestRecoverer_CatchesPanic — handler panics → 500 response
- TestAuthStub_SetsAnonymous — UserFromContext returns "anonymous"

---

## Dependency Additions

```
go get github.com/go-chi/chi/v5
```

chi is already in go.mod from M0 setup but verify it's there. No other new dependencies — CSV is stdlib `encoding/csv`.

---

## Implementation Order

1. `internal/api/response.go` — helpers (no deps)
2. `internal/api/middleware.go` — middleware stack (no deps)
3. `internal/api/server.go` — struct + Routes()
4. `internal/api/health.go` — health handler
5. `internal/api/instances.go` — instance handlers
6. `internal/api/metrics.go` — metric query + CSV
7. `internal/config/config.go` — add new fields + defaults
8. `internal/storage/pgstore.go` — add Pool() accessor
9. `cmd/pgpulse-server/main.go` — wire everything
10. All test files

---

## Team Prompt (for Claude Code)

```
Build the REST API for PGPulse.
Read .claude/CLAUDE.md for project context.
Read docs/iterations/M2_03_.../design.md for full specification.

This is a single-agent session (not Agent Teams). Build these files in order:

1. internal/api/response.go — Envelope, ErrorResponse, writeJSON, writeError helpers
2. internal/api/middleware.go — requestID, logger, recoverer, CORS, authStub middleware
3. internal/api/server.go — APIServer struct, New(), Routes()
4. internal/api/health.go — handleHealth with Pinger interface, storage/uptime/version
5. internal/api/instances.go — handleListInstances, handleGetInstance from config
6. internal/api/metrics.go — handleQueryMetrics with parseMetricQuery, resolveFormat, writeCSV
7. Update internal/config/config.go — add CORSEnabled, ReadTimeout, WriteTimeout, ShutdownTimeout to ServerConfig; add Description to InstanceConfig; add defaults
8. Update internal/storage/pgstore.go — add Pool() accessor method
9. Update cmd/pgpulse-server/main.go — wire HTTP server, graceful shutdown ordering
10. internal/api/health_test.go — 5 tests with mock Pinger
11. internal/api/instances_test.go — 5 tests
12. internal/api/metrics_test.go — 10 tests with mock MetricStore
13. internal/api/middleware_test.go — 4 tests

Rules:
- Use chi v5 for routing (github.com/go-chi/chi/v5)
- All response bodies via writeJSON/writeError helpers
- No external dependencies beyond chi (CSV is stdlib)
- Handler methods on *APIServer — testable with httptest
- Set application_name on storage pool as "pgpulse_storage" (already done)
- Agents CANNOT run bash. List all files created so developer can run build manually.

After creating all files, list:
1. Every file created or modified
2. Any go.mod changes needed
3. Suggested build verification commands
```
