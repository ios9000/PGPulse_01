# M2_02 Requirements — Storage Layer & Migrations

**Iteration:** M2_02
**Milestone:** M2 (Storage & API)
**Date:** 2026-02-26

---

## Goal

Replace LogStore with a real PG-backed MetricStore. After M2_02, collected metrics are persisted to PGPulse's own PostgreSQL database and queryable by time range, instance, and metric name. Migrations run automatically on startup.

## Scope

### In Scope

| Package | Purpose |
|---------|---------|
| `internal/storage/` | PGStore (MetricStore implementation), migration runner |
| `migrations/` | Embedded SQL migration files |
| Wire into orchestrator | Replace LogStore with PGStore when storage.dsn is configured |

### Out of Scope

- REST API (M2_03)
- TimescaleDB hypertable creation (conditional, flagged but not required)
- Retention policy / data cleanup (future)
- Connection pooling for monitored instances (PGStore is for PGPulse's own DB only)

## Functional Requirements

### FR-1: Migration Runner

- Embeds `migrations/*.sql` via `go:embed`
- Creates `schema_migrations` table on first run (tracks applied versions)
- Scans embedded files, sorts by filename (NNN_name.sql)
- Applies unapplied migrations in order, within a transaction per migration
- Idempotent: safe to run on every startup
- Logs each migration applied

### FR-2: Initial Schema (001_metrics.sql)

- `metrics` table: time (timestamptz), instance_id (text), metric (text), value (double precision), labels (jsonb)
- Indexes on (instance_id, time DESC) and (metric, time DESC)
- `schema_migrations` table created by runner (not in migration file)

### FR-3: Optional TimescaleDB (002_timescaledb.sql)

- Conditional: only applied when `storage.use_timescaledb = true` in config
- Creates hypertable on metrics table
- If TimescaleDB extension not available, skip with warning (don't fail)

### FR-4: PGStore

- Implements `collector.MetricStore` interface (Write, Query, Close)
- Uses `pgxpool.Pool` for concurrent access (multiple interval groups write simultaneously)
- `Write()`: batch insert via `pgx.CopyFrom` (COPY protocol) for performance
- `Query()`: SELECT with filters on instance_id, metric, time range, labels; ORDER BY time DESC; optional LIMIT
- `Close()`: closes the pool

### FR-5: Wiring

- In main.go: if `storage.dsn` is non-empty, create PGStore (run migrations, create pool); otherwise fall back to LogStore
- Pass PGStore to Orchestrator as MetricStore
- On shutdown: close PGStore after orchestrator stops

## Non-Functional Requirements

- **NFR-1:** Batch writes via COPY protocol (not individual INSERTs)
- **NFR-2:** pgxpool with max 5 connections (PGPulse's own DB, not monitored instances)
- **NFR-3:** Write timeout: 10s per batch
- **NFR-4:** Query timeout: 30s
- **NFR-5:** All new code passes golangci-lint
- **NFR-6:** Unit tests for migration runner logic, PGStore Write/Query (with mocks for pool)

## Acceptance Criteria

1. `go build ./...` passes
2. All unit tests pass
3. golangci-lint 0 issues
4. With a valid storage.dsn: migrations run on startup, metrics persist, Query returns stored data
5. With empty storage.dsn: falls back to LogStore (no regression)
6. CopyFrom batch insert works correctly for MetricPoint slices
