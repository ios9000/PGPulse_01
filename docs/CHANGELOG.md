# Changelog

All notable changes to PGPulse are documented here.
Format based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Added — M1_01: Instance Metrics Collector (2026-02-25)

- **ServerInfoCollector** — PG start time, uptime (computed in Go), recovery state, backup state with version gate (PG 14 only, removed in PG 15+)
- **ConnectionsCollector** — per-state breakdown (active, idle, idle_in_transaction, etc.), total count excluding PGPulse's own PID, max_connections, superuser_reserved, utilization percentage. PGAM bug fix: self-connection excluded.
- **CacheCollector** — global buffer cache hit ratio from pg_stat_database. PGAM bug fix: NULLIF guard against division by zero.
- **TransactionsCollector** — per-database commit ratio and deadlock counts. Enhancement: PGAM only reported global ratio.
- **DatabaseSizesCollector** — size in bytes per non-template database.
- **SettingsCollector** — track_io_timing, shared_buffers, max_locks_per_transaction, max_prepared_transactions via pg_settings IN-list query.
- **ExtensionsCollector** — pg_stat_statements presence, fill percentage, stats_reset timestamp (PG ≥ 14).
- **Registry** — RegisterCollector() and CollectAll() with partial-failure tolerance (one failing collector does not abort the batch).
- **Base struct** — shared helpers: point() with metric prefix, queryContext() with 5s timeout.
- Integration tests using testcontainers-go for PG 14 and PG 17.
- Unit tests for registry using mock collectors (no Docker required).

### Added — M0: Project Setup (2026-02-25)

- Go module initialized: github.com/ios9000/PGPulse_01
- Collector interface: MetricPoint, Collector, MetricStore, AlertEvaluator
- Version detection: PGVersion struct, Detect(), AtLeast()
- Version gate: Gate, SQLVariant, VersionRange, Select()
- Project scaffold: cmd/, internal/, configs/, deploy/, docs/
- Makefile, Dockerfile, docker-compose.yml, .golangci.yml, CI pipeline
- Sample configuration: configs/pgpulse.example.yml
