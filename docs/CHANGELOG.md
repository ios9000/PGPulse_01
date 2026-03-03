# Changelog

All notable changes to PGPulse are documented here.
Format based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

## 2026-03-01

### M4_03 — Alert API, Orchestrator Wiring & Integration
- Added 7 alert REST API endpoints: CRUD rules, active alerts, alert history, test notification
- Orchestrator post-collect evaluateAlerts() hook — evaluator runs after each collector group
- AlertEvaluator/AlertDispatcher interfaces in orchestrator package (clean dependency direction)
- main.go: full alert pipeline wiring (stores → seed → evaluator → notifiers → dispatcher)
- Updated graceful shutdown order: HTTP → Orchestrator → Dispatcher → Store
- History cleanup goroutine (1-hour interval, configurable retention)
- Builtin rules cannot be deleted via API (409 Conflict); only disabled
- Rule mutations (create/update/delete) refresh evaluator cache immediately

### M4_02 — Email Notifier & Dispatcher
- Notifier interface + NotifierRegistry for pluggable notification channels
- SMTP EmailNotifier: STARTTLS, PLAIN auth, MIME multipart/alternative (Go stdlib only)
- Rich HTML email templates: color-coded severity header, details table, dashboard link, plain text fallback
- Resolution emails: green header, duration, resolved timestamp
- Async Dispatcher: buffered channel (100), non-blocking for evaluator
- Cooldown enforcement per rule+instance+severity (resolutions always pass)
- Channel routing: global default_channels + per-rule override
- Retry with exponential backoff (1s, 2s, 4s), then log and drop
- Graceful shutdown drains buffered events
- Config: EmailConfig (host, port, from, recipients, TLS), DashboardURL

### M4_01 — Alert Evaluator Engine
- Alert data model: Rule, AlertEvent, Severity (info/warning/critical), Operator (6 ops), AlertState (ok/pending/firing), RuleSource (builtin/custom)
- Evaluator state machine: OK → PENDING → FIRING → OK with per-rule hysteresis
- Hysteresis: default 3 consecutive breaches before firing, configurable per rule
- Cooldown tracking: default 15 min, state transitions always notify immediately
- 19 builtin rules: 14 PGAM thresholds + 2 new replication lag + 3 deferred (enabled: false)
- AlertRuleStore + AlertHistoryStore interfaces with PG implementations
- Migration 004_alerts.sql: alert_rules (CHECK constraints, JSONB labels/channels) + alert_history (FK, partial index on unresolved)
- UpsertBuiltin: INSERT ON CONFLICT preserves user-modified threshold/enabled/channels
- SeedBuiltinRules: idempotent startup seeding
- In-memory state with restart recovery from unresolved alert_history
- Config: AlertingConfig (enabled, consecutive_count, cooldown_minutes, evaluation_timeout, history_retention_days)
- Label-aware state keys for per-slot/per-DB alert tracking

### M4 Milestone Complete
- Complete alerting pipeline: Evaluate → Notify → History
- 19 alert rules (14 PGAM + 5 new), email notifications, async dispatch
- 7 new API endpoints (14 total)
- 4 DB migrations (001-004)

### M3_01 — Auth & Security
- JWT authentication: HS256, access (24h) + refresh (7d), stateless
- Password hashing: bcrypt with configurable cost
- RBAC: admin (full access) + viewer (read-only), HasRole() level comparison
- User storage: users table (003_users.sql), PGUserStore
- Initial admin seeded from config on first run
- Rate limiting: in-memory per-IP, 10 failed / 15 min window on login
- Auth middleware: RequireAuth (JWT), RequireRole (RBAC), errorWriter callback
- Auth toggle: auth.enabled=false → authStubMiddleware
- API endpoints: POST /auth/login, POST /auth/refresh, GET /auth/me
- Config validation: jwt_secret ≥ 32 chars, auth requires storage DSN

## 2026-02-27

### M2_03 — REST API + Wiring
- REST API with chi v5 router: health, instances, metric query endpoints
- Middleware stack: request ID, structured logging, panic recovery, CORS
- JSON and CSV response formats for metric queries
- HTTP server lifecycle with graceful shutdown
- 24 API unit tests covering all endpoints and middleware

### M2_02 — Storage Layer
- PGStore: CopyFrom batch writes (10s timeout), dynamic WHERE queries (30s timeout)
- Migration runner: embedded SQL files, schema_migrations tracking, idempotent
- Migrations: 001_metrics (table + indexes), 002_timescaledb (conditional hypertable)
- Connection pool: pgxpool with 5 max connections

### M2_01 — Config + Orchestrator
- Config loader via koanf v2: YAML + env var overrides, duration parsing
- Orchestrator: per-instance goroutines, 3 interval groups (high/med/low)
- LogStore fallback for development without PostgreSQL

## 2026-02-26

### M1_05 — Locks & Wait Events
- Added WaitEventsCollector (Q53/Q54 merged) — per-event backend counts
- Added LockTreeCollector (Q55) — pg_blocking_pids() + Go BFS graph, summary metrics
- Added LongTransactionsCollector (Q56/Q57 merged) — parameterized threshold, active/waiting split
- 23 new unit tests including 7 pure graph topology tests for lock chain computation
- First use of Claude Code Agent Teams (2 agents: Collector + QA)

### M1 Milestone Complete
- 33/76 PGAM queries ported across 20 collector files
- 2 new collectors not in PGAM (checkpoint/bgwriter, pg_stat_io)
- 9 PGAM bugs fixed during port
- 7 version gates implemented (PG 14–17)

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
