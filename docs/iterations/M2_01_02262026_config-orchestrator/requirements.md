# M2_01 Requirements — Configuration & Orchestrator

**Iteration:** M2_01
**Milestone:** M2 (Storage & API)
**Date:** 2026-02-26

---

## Goal

Make PGPulse a running system. After M2_01, `go run cmd/pgpulse-server/main.go -config configs/pgpulse.yml` starts a process that:
1. Reads YAML config
2. Connects to each configured PostgreSQL instance
3. Detects PG version, queries recovery state
4. Instantiates all applicable collectors
5. Runs collectors on scheduled intervals (10s / 60s / 300s groups)
6. Passes collected MetricPoints to a store (log-based placeholder for now)
7. Shuts down gracefully on SIGINT/SIGTERM

## Scope

### In Scope

| Package | Purpose |
|---------|---------|
| `internal/config/` | YAML config loading via koanf, struct definitions |
| `internal/orchestrator/` | Per-instance connection, version detection, interval-group scheduling |
| `configs/pgpulse.example.yml` | Sample config file |
| `cmd/pgpulse-server/main.go` | Wires config → orchestrator → start, signal handling |

### Out of Scope

- PG-backed MetricStore (M2_02)
- REST API (M2_03)
- Authentication (M3)
- Alerting (M4)
- Dynamic instance add/remove via API (M3+)
- Connection pooling / pgxpool (single conn per instance is fine for M2)

## Functional Requirements

### FR-1: Configuration Loading

- Load from YAML file path passed via `-config` flag (default: `configs/pgpulse.yml`)
- Environment variable overrides via koanf (e.g., `PGPULSE_SERVER_LISTEN` overrides `server.listen`)
- Validate required fields: at least one instance with DSN
- Return clear error on invalid config

Config structure:
```yaml
server:
  listen: ":8080"           # HTTP listen address (used by M2_03)
  log_level: "info"         # debug, info, warn, error

storage:
  dsn: ""                   # PGPulse's own DB (empty = no persistence, log only)
  use_timescaledb: false    # enable hypertable creation
  retention_days: 30        # metric retention

instances:
  - id: "prod-main"
    dsn: "postgres://pg_monitor@10.0.0.1:5432/postgres"
    enabled: true
    intervals:
      high: "10s"           # connections, locks, wait_events, long_transactions
      medium: "60s"         # replication, statements, checkpoint
      low: "300s"           # database_sizes, io_stats, settings, extensions
  - id: "prod-replica"
    dsn: "postgres://pg_monitor@10.0.0.2:5432/postgres"
    enabled: true
```

### FR-2: Orchestrator Lifecycle

- `New(cfg Config, store MetricStore) *Orchestrator`
- `Start(ctx context.Context) error` — connects to all enabled instances, starts collection
- `Stop()` — cancels context, closes connections, waits for goroutines to finish
- Per-instance: connect → detect version → query pg_is_in_recovery() → build collectors → start groups

### FR-3: Instance Runner

- Maintains a single `*pgx.Conn` to the monitored instance
- Sets `application_name = 'pgpulse_orchestrator'` on connect
- Detects PG version once via `version.Detect()`
- Queries `pg_is_in_recovery()` once per collection cycle (before running collector groups)
- Builds InstanceContext{IsRecovery: bool} and passes to all collectors
- Handles connection loss: log error, retry on next cycle (don't crash)

### FR-4: Interval Group Scheduling

- Three groups per instance: high (10s), medium (60s), low (300s)
- Each group runs as a separate goroutine with a `time.Ticker`
- Within a group, collectors run sequentially (sharing one connection)
- Each collector call: Collect(ctx, conn, ic) → []MetricPoint
- Collected points passed to MetricStore.Write()
- Partial failure: if one collector errors, log it, continue with next
- Respect context cancellation for graceful shutdown

### FR-5: Collector Assignment

Collectors are assigned to interval groups by their declared Interval(). Mapping:

| Interval Group | Collectors |
|---------------|------------|
| High (10s) | connections, cache, wait_events, lock_tree, long_transactions |
| Medium (60s) | replication_status, replication_lag, replication_slots, statements_config, statements_top, checkpoint, progress_* |
| Low (300s) | server_info, database_sizes, settings, extensions, transactions, io_stats |

This mapping lives in the orchestrator, not in config (for M2). Collectors self-declare their interval via Interval(), orchestrator groups them accordingly.

### FR-6: Log Store (Placeholder)

- Implements MetricStore interface
- Write(): logs point count and sample metric names to slog at debug level
- Query(): returns nil, nil (not implemented until M2_02)
- Close(): no-op
- Purpose: lets the full pipeline run end-to-end before real storage exists

### FR-7: Graceful Shutdown

- main.go listens for SIGINT and SIGTERM
- Cancels root context → orchestrator stops all goroutines → closes connections
- Timeout: if shutdown takes > 10s, force exit
- Log "shutting down" and "shutdown complete" messages

### FR-8: Structured Logging

- Use slog throughout
- Log at startup: config loaded, instance count, PG versions detected
- Log per cycle: collector errors (warn), total points collected (debug)
- Log on shutdown: clean vs forced

## Non-Functional Requirements

- **NFR-1:** Single pgx.Conn per monitored instance (no pool needed at this scale)
- **NFR-2:** statement_timeout set per-query by collectors via queryContext() (already implemented)
- **NFR-3:** application_name = 'pgpulse_orchestrator' on connection
- **NFR-4:** connect_timeout = 5s on pgx.Connect
- **NFR-5:** All new code passes golangci-lint v2.10.1
- **NFR-6:** Unit tests for config parsing, interval group logic, collector assignment

## Acceptance Criteria

1. `go build ./...` passes
2. `go run cmd/pgpulse-server/main.go -config configs/pgpulse.example.yml` starts (may fail to connect if no PG available, but parses config and logs intent)
3. With a valid PG DSN in config, collectors run on schedule and metrics appear in log output
4. SIGINT triggers graceful shutdown
5. All unit tests pass
6. golangci-lint 0 issues
