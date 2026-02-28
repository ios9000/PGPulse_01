# M2_03 — REST API & Wiring: Requirements

**Iteration:** M2_03  
**Milestone:** M2 — Storage & API  
**Date:** 2026-02-27  
**Depends on:** M2_02 (storage layer), M1_01–M1_05 (collectors), M2_01 (orchestrator)

---

## Goal

Expose collected metrics and instance information via a REST API. After M2_03, an HTTP client can:

- Check server health and component status
- List monitored PostgreSQL instances (from config)
- Query stored metrics with time range, metric prefix, and limit filters
- Download metric results as JSON or CSV

This completes the M2 milestone: collectors → orchestrator → storage → **API**.

---

## Functional Requirements

### FR-1: Health Endpoint

| Field | Value |
|-------|-------|
| Method | GET |
| Path | `/api/v1/health` |
| Auth | None (public) |
| Response (JSON) | `{ "status": "ok\|error", "storage": "ok\|error\|disabled", "uptime": "2h35m12s", "version": "0.1.0" }` |

- `status`: "ok" if all components healthy, "error" if any component degraded
- `storage`: "ok" if PGStore pool.Ping() succeeds, "error" on failure, "disabled" if using LogStore
- `uptime`: duration since server start (formatted via `time.Duration.String()`)
- `version`: hardcoded build version string (set in `internal/api/` package var)
- HTTP status: 200 when status="ok", 503 when status="error"

### FR-2: List Instances

| Field | Value |
|-------|-------|
| Method | GET |
| Path | `/api/v1/instances` |
| Auth | Stub (anonymous) |
| Response | `{ "data": [...], "meta": { "count": N } }` |

Each instance object:
```json
{
  "id": "prod-pg-01",
  "host": "10.0.0.1",
  "port": 5432,
  "description": "Production primary",
  "enabled": true
}
```

- Source: `[]config.InstanceConfig` injected at construction time
- No database query — purely config-driven for M2

### FR-3: Instance Detail

| Field | Value |
|-------|-------|
| Method | GET |
| Path | `/api/v1/instances/{id}` |
| Auth | Stub (anonymous) |
| Response | Single instance object (same shape as list item) |
| 404 | `{ "error": { "code": "not_found", "message": "instance 'xyz' not found" } }` |

### FR-4: Query Metrics

| Field | Value |
|-------|-------|
| Method | GET |
| Path | `/api/v1/instances/{id}/metrics` |
| Auth | Stub (anonymous) |

Query parameters:

| Param | Type | Default | Validation |
|-------|------|---------|------------|
| `metric` | string | (all) | Optional. Prefix match via MetricQuery.Metric |
| `start` | RFC3339 | now - 1h | Optional. Parse error → 400 |
| `end` | RFC3339 | now | Optional. Parse error → 400 |
| `limit` | int | 1000 | Optional. Min 1, max 10000. Invalid → 400 |
| `format` | string | "json" | Optional. "json" or "csv". Invalid → 400 |

JSON response:
```json
{
  "data": [
    {
      "instance_id": "prod-pg-01",
      "metric": "pgpulse.connections.active",
      "value": 42.0,
      "labels": { "state": "active" },
      "timestamp": "2026-02-27T12:00:00Z"
    }
  ],
  "meta": {
    "count": 42,
    "instance_id": "prod-pg-01",
    "query": {
      "metric": "pgpulse.connections.active",
      "start": "2026-02-27T11:00:00Z",
      "end": "2026-02-27T12:00:00Z",
      "limit": 1000
    }
  }
}
```

CSV response (when `format=csv` or `Accept: text/csv`):
```
instance_id,metric,value,labels,timestamp
prod-pg-01,pgpulse.connections.active,42.000000,"{""state"":""active""}",2026-02-27T12:00:00Z
```

- Content-Type: `text/csv` with `Content-Disposition: attachment; filename="metrics.csv"`
- Labels serialized as JSON string within CSV field

### FR-5: Error Responses

All errors use a consistent envelope:
```json
{
  "error": {
    "code": "bad_request|not_found|internal_error",
    "message": "human-readable description"
  }
}
```

HTTP status mapping:
- 400: bad query params, invalid format
- 404: instance not found
- 500: storage query failure
- 503: health check failed

---

## Non-Functional Requirements

### NFR-1: Middleware Stack

Applied to all routes in order:
1. **RequestID** — Generate UUID, set `X-Request-ID` response header, inject into slog context
2. **Logger** — Log method, path, status, duration via slog (use request ID from context)
3. **Recoverer** — Catch panics, return 500, log stack trace
4. **CORS** — Only when `server.cors_enabled: true`. Allow-Origin: *, Allow-Methods: GET/POST/PUT/DELETE, Allow-Headers: Authorization/Content-Type
5. **AuthStub** — Set `user=anonymous` in context. M3 replaces with JWT middleware.

### NFR-2: Server Configuration

New/updated fields in `config.ServerConfig`:

```yaml
server:
  address: ":8080"         # Listen address (existing)
  cors_enabled: false      # Enable CORS middleware (new)
  read_timeout: 30s        # HTTP read timeout (new)
  write_timeout: 60s       # HTTP write timeout (new)
  shutdown_timeout: 10s    # Graceful shutdown deadline (new)
```

### NFR-3: Graceful Shutdown

On SIGINT/SIGTERM:
1. `httpServer.Shutdown(shutdownCtx)` — drain in-flight requests (deadline from config)
2. `orch.Stop()` — stop collection goroutines
3. `pgStore.Close()` — close storage pool

### NFR-4: No Auth Enforcement

All endpoints are open for M2_03. Auth middleware is a stub that always passes. M3 adds real JWT validation.

### NFR-5: Testability

- All handlers are methods on `APIServer` struct
- Tests use `httptest.NewRecorder()` + `chi.NewRouter()`
- Storage interaction tested via mock MetricStore
- No dependency on running HTTP server or PostgreSQL in unit tests

---

## Out of Scope for M2_03

- Authentication / JWT (M3)
- Instance CRUD — POST/PUT/DELETE (M3, requires DB inventory)
- WebSocket/SSE streaming (M5)
- Alert endpoints (M4)
- Prometheus /metrics exposition (M4)
- Rate limiting (M3)
- Pagination with cursors (future)
