# PGPulse

PGPulse is a PostgreSQL health and activity monitor: a single Go binary with an embedded React UI.
It collects metrics from PostgreSQL 14+, alerts on them, traces incidents to likely root causes and guides remediation.

<!-- TODO(owner): screenshot -->

## Features

Counts are taken from the code.

- **Collection.** 27 collector types: 26 instance-level collectors on 10 s / 60 s / 300 s tiers, plus a per-database collector that runs 17 queries against every database.
  - PostgreSQL activity: connections, wait events, lock trees, long transactions.
  - Replication: physical and logical, slots and lag.
  - Query and background stats: `pg_stat_statements`, checkpoints, I/O stats (PG 16+).
  - Progress: vacuum, analyze, index builds, base backups, `COPY` and `CLUSTER`.
  - Configuration: settings and extensions.
  - Per database: sizes, bloat, table and index statistics.
  - Environment: OS metrics, and Patroni/etcd cluster state.
- **Version-adaptive SQL.** Queries are version-gated for PostgreSQL 14–17. Newer versions use the latest variant but are untested.
- **Alerting.** 22 built-in rules: thresholds with hysteresis and cooldown, plus forecast and ML-anomaly rules. Notifications go out by email.
- **Root cause analysis.** A causal graph of 49 nodes and 38 edges forms 20 causal chains (16 stable, 4 experimental). Analysis runs on critical alerts or on demand.
- **Remediation.**
  - 25 advisor rules: 17 for PostgreSQL, 8 for the OS.
  - 10 built-in guided playbooks with 45 steps. Steps are grouped into diagnostic, external and dangerous safety tiers.
  - You can write your own playbooks in the UI or through the API.
- **Forecasting.**
  - Per-metric baselines (EWMA trend and folded seasonality) with forecast bands, Z-score/IQR anomaly scoring and forecast alerts.
  - Maintenance operation forecasting: ETAs for running vacuum, analyze, base backup and concurrent reindex, and need forecasts for vacuum, analyze and reindex. This part has not yet been validated against a live instance.
- **Operations.**
  - EXPLAIN plan viewer and settings snapshots with diff.
  - Workload report.
  - Session cancel and terminate.
- **Security.**
  - JWT authentication with bcrypt and login rate limiting.
  - Four-role RBAC: `super_admin`, `roles_admin`, `dba`, `app_admin`.
  - Parameterized SQL only.
  - The monitoring user needs `pg_monitor`, never superuser.
- **Deployment.**
  - One binary serves 96 REST routes and the UI.
  - Storage: PostgreSQL, optionally with TimescaleDB, or an in-memory live mode.
  - Optional Linux OS-metrics agent.
  - Optional Windows desktop build (Wails v3) with tray icon and installer.

## Quick start

### Build from source

Needs Go 1.25 and Node.js with npm. The UI is embedded at build time, so build it first:

```bash
git clone https://github.com/ios9000/PGPulse_01.git
cd PGPulse_01
cd web && npm ci && npm run build && cd ..
go build -o pgpulse-server ./cmd/pgpulse-server
```

`scripts/build-release.sh <version>` builds the same thing for Linux and Windows (amd64) into `dist/`.

### Monitor one instance (live mode)

No config file and no storage database needed:

```bash
./pgpulse-server --target "postgres://pgpulse_monitor:PASSWORD@db-host:5432/postgres"
```

Then open <http://localhost:8989>.
- Metrics stay in memory for 2 hours (`--history`).
- Live mode shows the monitoring dashboards only. Alerting, authentication, ML, RCA, remediation, playbooks and forecasting need persistent mode.

### Persistent mode (full feature set)

```bash
cp config.sample.yaml pgpulse.yml   # configs/pgpulse.example.yml is a fuller example
# edit pgpulse.yml: storage.dsn, instances, and the features you want (auth, alerting, ml, rca, ...)
./pgpulse-server --config pgpulse.yml
```

- **Listen address:** with a config file the server listens on `:8080` unless `server.listen` is set.
- **Storage:** PGPulse applies its own migrations to the `storage.dsn` database on startup.
- **Initial admin:** when `auth.enabled` is true and no users exist yet, the first start creates a `super_admin` from `auth.initial_admin` and logs `created initial admin user — change password immediately`.

<!-- TODO(owner): the Docker Compose quick start is left out on purpose. deploy/docker does not work as committed:
     the builder image is Go 1.23 while go.mod requires 1.25, there is no frontend build stage, and no config file
     is mounted, so the container exits on start. Fix it, then add:
     docker compose -f deploy/docker/docker-compose.yml up -d -->

### Command-line options

| Flag | Default | Meaning |
|---|---|---|
| `--target DSN` | | PostgreSQL connection string (live mode) |
| `--target-host`, `--target-port`, `--target-user`, `--target-password`, `--target-dbname` | –, `5432`, `pgpulse_monitor`, –, `postgres` | The same target as separate fields |
| `--listen ADDR:PORT` | `:8989` with `--target` | HTTP listen address |
| `--history DURATION` | `2h` | In-memory retention in live mode |
| `--no-auth` | | Disable authentication |
| `--config PATH` | `pgpulse.yml` | Config file |

### PostgreSQL user

```sql
CREATE ROLE pgpulse_monitor LOGIN PASSWORD 'your_password';
GRANT pg_monitor TO pgpulse_monitor;
-- Optional: OS metrics over SQL (os_metrics.method: sql, the default) read /proc with pg_read_file()
GRANT pg_read_server_files TO pgpulse_monitor;
-- Optional: cancel or terminate sessions from the UI
GRANT pg_signal_backend TO pgpulse_monitor;
```

For OS metrics without `pg_read_server_files`, run the Linux agent (`cmd/pgpulse-agent`) on the database host. Then set `os_metrics.method: agent` and the instance's `agent_url`.

## Download

Prebuilt demo artifacts are attached to the [M15_01-demo release](https://github.com/ios9000/PGPulse_01/releases/tag/M15_01-demo):
- a Linux x86-64 server binary;
- a kit that provisions a single Ubuntu 24.04 VM with a primary, a replica, a chaos target and PGPulse. See [deploy/demo/README.md](deploy/demo/README.md).

These are older demo builds. Build from source to get the current code.

## Architecture

```
cmd/pgpulse-server   one binary: orchestrator, storage, REST API, embedded UI, alerting, ML, RCA, remediation
cmd/pgpulse-agent    optional Linux agent that serves OS metrics from procfs over HTTP
internal/            collector, version, orchestrator, storage (+ migrations), api, auth, alert, ml, rca,
                     remediation, playbook, forecast, plans, settings, statements, cluster, agent, config, desktop
web/                 React + TypeScript + Tailwind CSS + Apache ECharts, embedded with go:embed
deploy/              Docker, demo VM kit, NSIS installer, systemd unit
```

How it runs:
- **Collection.** Each monitored instance gets its own collector goroutines and a small pgx pool. Collectors choose SQL by the detected PostgreSQL version.
- **Storage.** Metrics are written to PostgreSQL or TimescaleDB, or kept in memory in live mode.
- **Serving.** Dashboards read collected metrics from storage. Interactive features (EXPLAIN, session actions, playbook steps) connect to the monitored instance on demand.

Where to read more:
- [docs/CODEBASE_DIGEST.md](docs/CODEBASE_DIGEST.md): file inventory, interfaces, metric keys and API routes.
- [docs/save-points/LATEST.md](docs/save-points/LATEST.md): architecture snapshot and decision log.
- [docs/README.md](docs/README.md): index of all project documents.

## How it was built

PGPulse began as a Go rewrite of PGAM, a legacy PHP PostgreSQL activity monitor from 2019.

**My role.** I set the requirements and made the architecture decisions, working through designs with Claude.ai.

**The code.** Claude Code wrote it: parallel agent teams in separate git worktrees for larger milestones, and single-agent sessions for smaller ones.

**Iteration documents.** Each iteration has a folder in [docs/iterations/](docs/iterations/). Most folders have a requirements document, a design, a team prompt and a session log. Checklists start at M8.

**Decisions** are recorded in four places:
- numbered decision tables in the save points;
- the decision sections of session logs;
- the "DO NOT RE-DISCUSS" list in every handoff;
- two formal ADRs (M14_03, M14_04).

**Save points.** [docs/save-points/](docs/save-points/) lets any new session resume from a known state.

**Testing.** The agents write and test the code, and the session logs record the build, vet, lint and test results. `make test` runs the tests with the race detector. On the Windows development machine they run without it, because cgo is not available there.

## Development

```bash
cd web && npm run build && npm run lint && npm run typecheck && cd ..
go build ./... && go vet ./...
go test ./cmd/... ./internal/...     # never ./... (it would scan web/node_modules)
golangci-lint run
```

`make build`, `make test` (with `-race`) and `make lint` wrap the Go steps. Integration tests are tagged `integration` and need Docker or `PGPULSE_TEST_DSN`.

## License

Proprietary.

<!-- TODO(owner): there is no LICENSE file. This README said "Proprietary", deploy/nsis/license.txt ships the MIT text
     with the Windows installer, and the save points say "[TBD]". Choose one and add a LICENSE file. -->
