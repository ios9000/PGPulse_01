# PGPulse

**PostgreSQL Health & Activity Monitor**

PGPulse is a modern Go rewrite of the legacy PGAM PHP tool, providing real-time PostgreSQL monitoring, alerting, ML-based anomaly detection, and cross-stack root cause analysis.

## Features

- Real-time PostgreSQL instance monitoring (connections, locks, replication, statements)
- Per-database analysis (bloat, index usage, table statistics)
- Version-adaptive SQL (supports PostgreSQL 14-18)
- Configurable alerting (Telegram, Slack, Email, Webhook)
- ML-based anomaly detection (planned)
- Single binary deployment with embedded web UI

## Quick Start

### Build from source

```bash
go build -o pgpulse-server ./cmd/pgpulse-server/
go build -o pgpulse-agent ./cmd/pgpulse-agent/
```

### Docker Compose

```bash
docker-compose -f deploy/docker/docker-compose.yml up -d
```

This starts PGPulse on port 8080 with a TimescaleDB instance on port 5432.

## Architecture

- **Single binary** server with embedded frontend (Svelte + Tailwind CSS)
- **Separate agent** binary for OS-level metrics collection
- **Version-adaptive SQL** - queries adapt to PostgreSQL version automatically
- **pgx v5** driver with parameterized queries (no SQL injection)
- **TimescaleDB** for time-series metric storage
- **go-chi** HTTP router with JWT authentication

## Development

```bash
make build       # Compile server and agent
make test        # Run tests with race detector
make lint        # Run golangci-lint
make docker-build  # Build Docker image
make docker-up   # Start Docker Compose stack
make docker-down # Stop Docker Compose stack
make clean       # Remove build artifacts
```

## Project Structure

```
cmd/                    # Binary entrypoints
  pgpulse-server/       # Main monitoring server
  pgpulse-agent/        # OS metrics agent
internal/               # Business logic
  collector/            # Metric collectors (PG queries)
  version/              # PG version detection and gating
  storage/              # TimescaleDB storage layer
  api/                  # REST API (go-chi)
  auth/                 # JWT authentication
  alert/                # Alert evaluation and notification
  config/               # Configuration management
  ml/                   # ML anomaly detection (future)
  rca/                  # Root cause analysis (future)
web/                    # Embedded frontend (future)
migrations/             # SQL migrations
configs/                # Configuration templates
deploy/                 # Docker, Helm, systemd
docs/                   # Documentation
```

## Documentation

- [Roadmap](docs/roadmap.md)
- [Changelog](docs/CHANGELOG.md)

## License

Proprietary
