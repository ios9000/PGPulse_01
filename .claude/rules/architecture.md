# Architecture Rules

## Module Boundaries
These boundaries are enforced by Agent Teams module ownership.
Violations will cause merge conflicts.

internal/collector/ → COLLECTOR AGENT
  - Depends on: internal/version/, internal/collector/collector.go (interfaces)
  - Does NOT import: internal/api/, internal/auth/, internal/alert/

internal/api/ → API & SECURITY AGENT
  - Depends on: internal/storage/, internal/auth/, internal/alert/
  - Does NOT import: internal/collector/ (uses MetricStore interface)

internal/storage/ → API & SECURITY AGENT
  - Depends on: internal/collector/collector.go (MetricPoint struct only)
  - Does NOT import: internal/api/, internal/auth/

internal/auth/ → API & SECURITY AGENT
  - Standalone module, no internal imports

internal/alert/ → SPLIT OWNERSHIP:
  - evaluator.go, rules.go → COLLECTOR AGENT (domain logic)
  - notifier/*, dispatcher.go → API & SECURITY AGENT (HTTP/transport)

internal/version/ → COLLECTOR AGENT
  - Standalone module, no internal imports

internal/ml/ → ML AGENT (future, M8+)
internal/rca/ → ML AGENT (future, M8+)
web/ → FRONTEND AGENT (future, M5+)

## Communication Between Modules
- Modules communicate through interfaces defined in collector.go
- No direct struct access across module boundaries
- Use dependency injection in main.go to wire modules together

## Concurrency
- Each collector runs in its own goroutine
- Metric writes are batched and flushed periodically
- API handlers are stateless (all state in PostgreSQL)
- Use context.Context for cancellation and timeouts everywhere

## Single Binary Deployment
- Frontend embedded via go:embed
- Config via YAML file + env var overrides
- One binary: pgpulse-server (agent binary is separate: pgpulse-agent)
- No runtime dependencies except PostgreSQL + TimescaleDB

## Connection Model
- One persistent connection per monitored PostgreSQL instance
- Connection pool via pgx pool (max 3 connections per instance)
- Async collection: goroutine per collector group on independent schedules
- Decoupled rendering: API serves pre-collected data from storage
