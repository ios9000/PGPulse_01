# Session: 2026-02-27 — M2_03 REST API & Wiring

## Goal

Expose collected metrics via a REST API. Complete the M2 milestone by adding HTTP endpoints for health checks, instance listing, and metric queries with JSON + CSV output.

## Agent Configuration

- **Planning:** Claude.ai (Opus 4.6) — architecture decisions, design doc, requirements doc
- **Implementation:** Claude Code (Sonnet) — single-agent session
- **Duration:** ~1 session
- **Scope:** 13 new files, 4 modified files, 24 test cases

## What Was Built

### New Package: internal/api/

| File | Purpose | Lines (approx) |
|------|---------|----------------|
| `internal/api/response.go` | Envelope, ErrorResponse, writeJSON, writeError helpers | ~80 |
| `internal/api/middleware.go` | requestID, logger (with responseWriter wrapper), recoverer, CORS, authStub, UserFromContext | ~100 |
| `internal/api/server.go` | APIServer struct, Pinger interface, New(), Routes() | ~90 |
| `internal/api/health.go` | GET /api/v1/health — pool ping, uptime, version, storage status | ~60 |
| `internal/api/instances.go` | GET /api/v1/instances, GET /api/v1/instances/{id} — config-driven | ~80 |
| `internal/api/metrics.go` | GET /api/v1/instances/{id}/metrics — parseMetricQuery, resolveFormat, writeCSV | ~140 |

### Test Files

| File | Tests | Coverage |
|------|-------|----------|
| `internal/api/helpers_test.go` | mockStore, mockPinger, newTestServer shared helpers | — |
| `internal/api/health_test.go` | 5 (AllOK, StorageError, NoStorage, Version, Uptime) | health.go |
| `internal/api/instances_test.go` | 5 (Empty, Multiple, Found, NotFound, FieldMapping) | instances.go |
| `internal/api/metrics_test.go` | 10 (DefaultParams, CustomRange, InvalidStart, InvalidLimit, NotFound, StorageError, JSON, CSV, CSVAcceptHeader, EmptyResult) | metrics.go |
| `internal/api/middleware_test.go` | 4 (RequestID_Generated, Passthrough, Recoverer, AuthStub) | middleware.go |

### Modified Files

| File | Change |
|------|--------|
| `internal/config/config.go` | Added CORSEnabled, ReadTimeout, WriteTimeout, ShutdownTimeout to ServerConfig; added Description to InstanceConfig |
| `internal/config/load.go` | Added defaults: ReadTimeout=30s, WriteTimeout=60s, ShutdownTimeout=10s |
| `internal/storage/pgstore.go` | Added Pool() *pgxpool.Pool accessor for health checks |
| `cmd/pgpulse-server/main.go` | Wired HTTP server in goroutine, graceful shutdown: HTTP → Orchestrator → Store |

## Architecture Decisions

| Decision | Rationale |
|----------|-----------|
| Pinger interface for health checks | Abstracts pgxpool.Pool for testability; mock in tests, real pool in prod |
| Host/Port parsed from DSN via net/url | Temporary for M2; M3+ will have explicit host/port in DB-backed inventory |
| JSON envelope `{"data":..., "meta":...}` | Consistent response format, extensible for pagination later |
| CSV via ?format=csv OR Accept: text/csv | Both mechanisms supported; labels serialized as JSON string in CSV |
| Auth stub sets "anonymous" in context | UserFromContext() works now; M3 swaps middleware without touching handlers |
| CORS gated by config flag | Off by default; enable for frontend dev mode (M5) |
| Shutdown order: HTTP → Orchestrator → Store | Drain HTTP requests before stopping collection, close pool last |

## Dependency Changes

- Added: `github.com/go-chi/chi/v5` v5.2.5

## Build & Test Results

| Check | Result |
|-------|--------|
| `go build ./...` | ✅ Clean |
| `go vet ./...` | ✅ Clean |
| `golangci-lint run` | ✅ 0 issues |
| `go test ./internal/api/...` | ✅ 24/24 pass |
| `go test ./...` | ✅ All packages pass (no regressions) |

## Commit

```
feat(api): add REST API — health, instances, metrics endpoints (M2_03)
```

## M2 Milestone Summary

M2 is now complete. The full data pipeline works:

```
Collectors (M1) → Orchestrator (M2_01) → Storage (M2_02) → REST API (M2_03)
```

| Iteration | Scope | Status |
|-----------|-------|--------|
| M2_01 | Config + Orchestrator | ✅ Done |
| M2_02 | Storage Layer + Migrations | ✅ Done |
| M2_03 | REST API + Wiring | ✅ Done |

## Not Done / Deferred

- Auth enforcement → M3_01
- Instance CRUD (POST/PUT/DELETE) → M3 (DB-backed inventory)
- WebSocket/SSE streaming → M5
- Alert endpoints → M4
- Prometheus /metrics exposition → M4
- Rate limiting → M3
- Pagination with cursors → future
- Retention cleanup → future
- Auto-reconnect on connection loss → future
