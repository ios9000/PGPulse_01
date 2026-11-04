# PGPulse — Security Vetting Package

> **Audience:** security office / accreditation reviewers for regulated, air-gapped deployments.
> **Basis:** source code at the commit this file was added in (base `7e22e67`), read on 2026-11-04.
> **Method:** every statement below cites the file (and line, where useful) it was derived from.
> Statements marked **[tested]** were also reproduced against PostgreSQL 16.15 in a sandbox on 2026-11-04.
> This document describes what the code does today, including defects. It is not a statement of intended design.

Paths are repository-relative. `server.go` means `internal/api/server.go`; `main.go` means `cmd/pgpulse-server/main.go`.

---

## 0. Summary for the reviewer

PGPulse is a single Go binary (`pgpulse-server`) that connects to monitored PostgreSQL servers, collects metrics,
stores them in its own PostgreSQL database, and serves an embedded web UI plus a REST API. An optional second
binary (`pgpulse-agent`) runs on database hosts and exposes OS metrics over HTTP.

The items most likely to matter for an accreditation decision:

1. **Authentication is off by default** and, when off, every caller is treated as `super_admin` on a listener bound
   to all interfaces, with no TLS (sections 1, 4).
2. **The default OS-metrics mode (`sql`) requires grants that let the monitoring role read any file the `postgres`
   OS user can read** — tested to include `pg_hba.conf` and the server's TLS private key (sections 2, 7).
3. **PGPulse can change state on monitored servers**: cancel/terminate sessions, run caller-supplied SQL through
   `EXPLAIN ANALYZE`, and run playbook SQL — and the playbook path can be reached by the least-privileged role
   (sections 4, 6).
4. **Several protections that appear in the schema or code are not effective**: the session audit table is never
   written, retention for metrics is never enforced, and the playbook "read-only" guard does not take effect
   (sections 5, 6, 9, 10).

---

## 1. Network

### 1.1 Listening ports

| Process | Default address | Protocol | Source |
|---|---|---|---|
| `pgpulse-server` with a config file | `:8080` (all interfaces) when neither `server.listen` nor `server.port` is set | HTTP | `internal/config/load.go:56-62` |
| `pgpulse-server` started with `--target` and no config file | `:8989` (all interfaces) | HTTP | `main.go:48`, `main.go:109-111` |
| `pgpulse-server --mode=desktop` (build tag `desktop`) | no TCP listener; the router is handed to the Wails webview | in-process | `main.go:622-633` |
| `pgpulse-agent` | `0.0.0.0:9187` | HTTP | `cmd/pgpulse-agent/main.go:39-42`, `internal/agent/server.go:50-60` |

The shipped container image exposes 8080 (`deploy/docker/Dockerfile:20`); compose publishes 8080 and also publishes
the storage PostgreSQL on host port 5432 (`deploy/docker/docker-compose.yml:9`, `:19-20`).

Agent routes: `/health`, `/metrics/os`, `/metrics/cluster` (`internal/agent/server.go:42-46`). They return hostname,
OS release, uptime, load, memory, CPU, mount points, disk statistics (`internal/agent/osmetrics.go:12-61`) and,
if configured, Patroni/etcd cluster membership (`internal/cluster/patroni/provider.go:25-46`).
**The agent has no authentication and no TLS** (`internal/agent/server.go:42-60`).

### 1.2 Outbound connections made by `pgpulse-server`

| Destination | When | Source | Authentication / TLS |
|---|---|---|---|
| Each monitored PostgreSQL (`instances[].dsn`) | always; one pool per instance (`application_name=pgpulse_collector`, `max_conns` default 5), per-database pools of 2, a pg_stat_statements pool of 2, plus a fresh connection per API / playbook action (`application_name=pgpulse_api`) | `internal/orchestrator/runner.go:36-46`, `internal/orchestrator/orchestrator.go:195-245`, `internal/orchestrator/db_runner.go:247`, `main.go:765-820`, `internal/config/load.go:203-204` | Whatever the DSN specifies. pgx's default `sslmode=prefer` does not verify certificates. `--target-host` hard-codes `sslmode=disable` (`main.go:715`). |
| PGPulse storage PostgreSQL (`storage.dsn`) | when configured | `internal/storage/pool.go:19-28` | Per DSN. |
| `pgpulse-agent` (`instances[].agent_url`) | when `agent_url` is set | `internal/agent/scraper.go:22-70`, `internal/collector/os.go:44`, `internal/api/os.go:30`, `:66` | Plain HTTP GET, no auth header, 10 s timeout. |
| Patroni REST (`patroni_url`), etcd HTTP (`etcd_endpoints`) | when configured | `internal/cluster/patroni/rest.go:23-162`, `internal/cluster/etcd/http.go:23-129` | No auth, no client certificates. |
| SMTP (`alerting.email`) | when alerting with the `email` channel is enabled | `internal/alert/notifier/email.go:60-100` | Opportunistic STARTTLS — skipped if the server does not offer it (`:74`); `tls_skip_verify` option (`:77`); PLAIN auth (`:86`). |
| Any PostgreSQL DSN supplied in a request body | `POST /api/v1/instances/{id}/test` | `internal/api/instances_crud.go:308-360` | Connects to the caller-supplied host/port and returns the driver error text — usable for internal port probing. |

No webhook, Slack, Telegram, PagerDuty, telemetry or update-check client exists: `internal/alert/notifier/` contains
only `email.go`, and the only HTTP clients are the agent, Patroni and etcd clients listed above.

PGPulse (server and agent) may also **execute local binaries** `patronictl` and `etcdctl` with fixed arguments when
Patroni/etcd shell integration is configured (`internal/cluster/patroni/shell.go:38`, `:140-160`;
`internal/cluster/etcd/shell.go:36`, `:113`; wired per instance in `internal/orchestrator/runner.go:115-125`).

### 1.3 Outbound requests made by the browser

The embedded web UI references Google Fonts (`fonts.googleapis.com`, `fonts.gstatic.com`) in `web/index.html:8-12`,
and the Content-Security-Policy allows them (`internal/api/middleware.go:116`). In an air-gapped network these
requests fail and the UI falls back to system fonts; no other external origin is loaded. Remediation advice contains
links to postgresql.org and similar sites, which are rendered as links only and never fetched by the server
(`internal/remediation/`).

### 1.4 TLS posture

- **The server has no HTTPS listener.** It calls `httpServer.ListenAndServe()` (`main.go:644`); there is no
  `ListenAndServeTLS`, certificate setting or `tls.Config` for the listener. TLS must be terminated by a reverse proxy.
- **The agent has no HTTPS listener** (`internal/agent/server.go:60`).
- Database connections use TLS only as the DSN requests it (section 1.2).
- The only `tls.Config` in the server is the SMTP client (`internal/alert/notifier/email.go:75`).
- CORS is off by default; when enabled it allows one configured origin with credentials
  (`internal/api/middleware.go:92-108`, default origin `http://localhost:5173` from `internal/config/load.go:79-81`).
- Security headers on every response: `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, `Referrer-Policy`,
  CSP (`internal/api/middleware.go:111-117`).

---

## 2. Identities and roles required on monitored servers

### 2.1 Grants

| Grant | Needed for | Source |
|---|---|---|
| `pg_monitor` (membership) | All SQL metric collectors: `pg_stat_activity` (all sessions), replication views, `pg_stat_statements`, `pg_settings`, progress views, `pg_ls_waldir()` | `README.md:103-104`; collectors under `internal/collector/`; `pg_ls_waldir` in `internal/playbook/seed_wal.go:50` |
| `pg_read_server_files` **and** `GRANT EXECUTE ON FUNCTION pg_read_file(text)` | `os_metrics_method: sql` (**the default**): reads `/proc/meminfo`, `/proc/uptime`, `/proc/loadavg`, `/proc/stat`, `/proc/diskstats`. Also used — regardless of `os_metrics_method` — to read `/etc/hostname` and `/etc/os-release` for the server-info panel. Both fail soft without the grants. | `internal/collector/os_sql.go:106-117`; `internal/collector/server_info.go:109-141`; default in `internal/orchestrator/runner.go:203-213` and `internal/config/config.go` (`EffectiveOSMetricsMethod`) |
| `pg_signal_backend` | Cancel / terminate session buttons (non-superuser sessions only) | `README.md:107-108`; `internal/api/session_actions.go:94-101` |
| `pg_stat_statements` extension created by a DBA (and listed in `shared_preload_libraries`) | Statement collectors and snapshots. PGPulse never runs `CREATE EXTENSION`. | `internal/statements/capture.go:178-189`; `docs/desktop/DESKTOP_USER_GUIDE.md:258` |
| `SELECT` on application tables (e.g. `pg_read_all_data`) | Only if the EXPLAIN / plan-capture features must work on application queries; EXPLAIN checks table privileges. Not documented elsewhere. | `internal/api/explain.go:80`; `internal/api/plan_handlers.go:171-174` |
| `SELECT ON pg_largeobject` | Only for the `large_object_sizes` per-database metric; without it that sub-collector logs a permission error and the rest continue. | **[tested]** log line `large_object_sizes scan: ERROR: permission denied for table pg_largeobject`; `internal/collector/database.go` |

Minimal role for a vetted deployment (no OS metrics over SQL, no session actions):

```sql
CREATE ROLE pgpulse_monitor LOGIN PASSWORD '...';
GRANT pg_monitor TO pgpulse_monitor;
-- and set os_metrics_method: agent or disabled (section 7)
```

### 2.2 Does `pg_monitor` suffice?

**For metric collection, yes, except OS metrics and hostname/OS name.** **[tested]** With a role that is only a
member of `pg_monitor` (which on PG 16 contains `pg_read_all_settings`, `pg_read_all_stats`, `pg_stat_scan_tables`),
every `pg_read_file` call fails with `permission denied for function pg_read_file`; all other collectors ran.

**[tested]** `pg_read_server_files` alone is **not** enough — `pg_read_file` still fails with
`permission denied for function pg_read_file`. `EXECUTE` alone fails with `absolute path not allowed`. Both together
are required. `README.md:105-106` lists only `pg_read_server_files`; `docs/desktop/DESKTOP_SETUP_GUIDE.md:73-78`
describes the EXECUTE grant as Ubuntu/Debian-specific, but it is required on upstream PostgreSQL 11+ generally.

### 2.3 Superuser and write privileges

No collector needs superuser, `CREATE`, or any write privilege. PGPulse **does not verify** that the configured role is
non-superuser or read-only; the `.claude/rules/security.md` rule "pg_monitor role only — NEVER superuser" is a
convention, not an enforced check (no such check in `internal/orchestrator/runner.go` or
`internal/orchestrator/orchestrator.go`). If the DSN role has write rights, the EXPLAIN ANALYZE and playbook paths in
section 6 can write data.

---

## 3. Credentials handling

### 3.1 Where secrets live

| Secret | Location | Source |
|---|---|---|
| Monitored-instance DSNs (contain passwords) | YAML `instances[].dsn`; **plaintext** column `instances.dsn` in the storage DB (seeded from YAML at startup and written by the instance CRUD API); CLI flags `--target` / `--target-password` (visible in the process list) | `internal/config/config.go` (`InstanceConfig.DSN`); `internal/storage/migrations/006_instances.sql:4`; `main.go:534-544`; `internal/storage/instances.go:110-182`; `main.go:42-47` |
| Storage DSN | YAML `storage.dsn` or env `PGPULSE_STORAGE_DSN` | `internal/config/config.go` (`StorageConfig.DSN`); `internal/config/load.go:23` |
| JWT signing secrets | YAML `auth.jwt_secret` (≥ 32 chars enforced), `auth.refresh_secret` (defaults to `jwt_secret`) | `internal/config/load.go:276-278`; `main.go:465-467` |
| Initial admin password | YAML `auth.initial_admin.password` (plaintext in YAML; hashed into `users` only when the table is empty) | `main.go:447-463` |
| User passwords | `users.password_hash`, bcrypt, cost 12 by default | `internal/storage/migrations/003_users.sql`; `internal/auth/password.go:7`; `internal/config/load.go:285-287` |
| SMTP password | YAML `alerting.email.password` | `internal/config/config.go` (`EmailConfig.Password`) |
| Browser refresh token | `localStorage` | `web/src/stores/authStore.ts:60`, `:95` |
| Desktop quick-connect DSN | temp file `pgpulse.yml` (mode 0644 inside a 0700 temp dir), not deleted | `cmd/pgpulse-server/desktop.go:66-78` |

No encryption-at-rest of DSNs exists in the code. No secret manager integration exists.

Environment overrides: `PGPULSE_<SECTION>_<KEY>` is mapped by replacing **every** `_` with `.`
(`internal/config/load.go:42-45`), so keys that themselves contain an underscore (e.g. `auth.jwt_secret`,
`server.log_level`) **cannot** be set from the environment; `PGPULSE_STORAGE_DSN` and `PGPULSE_SERVER_LISTEN` work.
`${VAR}` placeholders in YAML are not expanded (no `os.ExpandEnv` in `internal/config/`).

### 3.2 Logging and display of secrets

- No `slog` call in the repository logs a DSN, password, secret or token field; startup logs only the initial admin
  username (`main.go:461-462`).
- Connection errors are logged and some are returned to API clients (`main.go:142`; `internal/api/instances_crud.go:335-339`;
  `internal/api/explain.go:51-52`). pgx v5.8.0 redacts passwords in `ParseConfigError` and prints only user and
  database in `ConnectError` (`pgconn/errors.go` in the pinned module).
- Instance CRUD responses mask the password in URL DSNs and `password=` key/value DSNs
  (`internal/api/instances_crud.go:22-75`). The `?password=` query-parameter form is not masked.
  `GET /api/v1/instances` does not return DSNs (`internal/api/instances.go:18-25`).

---

## 4. RBAC roles in PGPulse

### 4.1 Roles and permissions

Five permissions (`internal/auth/rbac.go:7-11`) and four roles (`internal/auth/rbac.go:18-33`):

| Role | user_management | instance_management | alert_management | view_all | self_management |
|---|:-:|:-:|:-:|:-:|:-:|
| `super_admin` | ✓ | ✓ | ✓ | ✓ | ✓ |
| `roles_admin` | ✓ | | | ✓ | ✓ |
| `dba` | | ✓ | ✓ | ✓ | ✓ |
| `app_admin` | | | ✓ | ✓ | ✓ |

Enforcement: `auth.RequireAuth` validates an HS256 access token (`internal/auth/middleware.go:55-85`;
`internal/auth/jwt.go:100-107`); `auth.RequirePermission` returns 403 when the role lacks the permission
(`internal/auth/middleware.go:110-127`). No route checks `view_all` or `self_management`, so "any authenticated user"
below means all four roles. The role is read from the token on each request and not re-checked against the
database, so a deactivated or demoted user keeps access until the access token expires (default 24 h,
`internal/config/load.go:279-280`).

Login is rate-limited to 10 failures per 15 minutes per client IP, on `POST /auth/login` only
(`server.go:86-89`, `:191-194`; `internal/auth/ratelimit.go:28-79`).

### 4.2 Auth-disabled mode (the default)

`auth.enabled` defaults to `false` (`internal/config/config.go:95`) and `--no-auth` forces it off
(`main.go:100-102`). Auth also requires `storage.dsn` (`internal/config/load.go:273-275`), so live (in-memory) mode
always runs without auth. With auth off, the router is built from a separate branch (`server.go:377-525`) in which a
middleware injects **implicit `super_admin` claims into every request** (`internal/auth/middleware.go:33-46`) and no
`RequirePermission` is registered. Every route in the table below, including session terminate and EXPLAIN ANALYZE,
is then reachable by anyone who can reach the port. The six user-management routes (#91–#96) are not registered
in that mode.

### 4.3 Route-to-role map

With auth enabled and every optional feature configured, `server.go:186-375` registers **96 API routes** (counted by
reading and by walking the chi router). Optional groups are only registered when their feature is enabled:
alerts (7 routes), remediation (5), RCA (6), playbooks (20), maintenance forecast (5), instance CRUD (5)
(`server.go:252`, `:269`, `:282`, `:296`, `:344`, `:355`). In addition the embedded SPA is served unauthenticated at
`/*` (`server.go:530`; `internal/api/static.go:14-44`).

Legend — **Access**: `public` = no auth; `auth` = any authenticated user (all four roles); `IM` = instance_management
(super_admin, dba); `AM` = alert_management (super_admin, dba, app_admin); `UM` = user_management (super_admin,
roles_admin). **Effect**: `PG-WRITE` = changes state on / executes caller SQL against a monitored server; `PG-READ` =
live read query on a monitored server; `STORE-W` = writes PGPulse's own storage; `EXT` = other outbound call; `R` = read
from PGPulse storage/memory. Paths are under `/api/v1`. "Line" is the `server.go` line in the auth-enabled branch.

| # | Method | Path | Access | Effect | Line |
|---|---|---|---|---|---|
| 1 | GET | /health | public | R | 186 |
| 2 | GET | /system/mode | public | R | 187 |
| 3 | POST | /auth/login | public, rate-limited | STORE-W | 193 |
| 4 | POST | /auth/refresh | public (refresh token) | R | 196 |
| 5 | GET | /auth/me | auth | R | 201 |
| 6 | GET | /instances | auth | R | 202 |
| 7 | GET | /instances/{id} | auth | R | 203 |
| 8 | GET | /instances/{id}/metrics | auth | R | 204 |
| 9 | GET | /instances/{id}/metrics/current | auth | R | 205 |
| 10 | GET | /instances/{id}/metrics/history | auth | R | 206 |
| 11 | GET | /instances/{id}/metrics/{metric}/forecast | auth | R | 207 |
| 12 | GET | /instances/{id}/replication | auth | PG-READ | 208 |
| 13 | GET | /instances/{id}/activity/wait-events | auth | PG-READ | 209 |
| 14 | GET | /instances/{id}/activity/long-transactions | auth | PG-READ | 210 |
| 15 | GET | /instances/{id}/activity/statements | auth | PG-READ | 211 |
| 16 | GET | /instances/{id}/activity/locks | auth | PG-READ | 212 |
| 17 | GET | /instances/{id}/activity/progress | auth | PG-READ | 213 |
| 18 | GET | /instances/{id}/os | auth | EXT (agent) | 214 |
| 19 | GET | /instances/{id}/cluster | auth | EXT (agent) | 215 |
| 20 | GET | /instances/{id}/databases | auth | R | 216 |
| 21 | GET | /instances/{id}/databases/{dbname}/metrics | auth | R | 217 |
| 22 | GET | /instances/{id}/logical-replication | auth | PG-READ | 218 |
| 23 | GET | /instances/{id}/plans | auth | R | 221 |
| 24 | GET | /instances/{id}/plans/regressions | auth | R | 222 |
| 25 | GET | /instances/{id}/plans/{plan_id} | auth | R | 223 |
| 26 | GET | /instances/{id}/settings/history | auth | R | 226 |
| 27 | GET | /instances/{id}/settings/diff | auth | R | 227 |
| 28 | GET | /instances/{id}/settings/latest | auth | R | 228 |
| 29 | GET | /instances/{id}/settings/pending-restart | auth | R | 229 |
| 30 | GET | /instances/{id}/snapshots/diff | auth | R | 232 |
| 31 | GET | /instances/{id}/snapshots/latest-diff | auth | R | 233 |
| 32 | GET | /instances/{id}/snapshots | auth | R | 234 |
| 33 | GET | /instances/{id}/snapshots/{snapId} | auth | R | 235 |
| 34 | GET | /instances/{id}/query-insights/{queryid} | auth | R | 236 |
| 35 | GET | /instances/{id}/workload-report/html | auth | R | 237 |
| 36 | GET | /instances/{id}/workload-report | auth | R | 238 |
| 37 | POST | /instances/{id}/plans/capture | IM | PG-READ + STORE-W (plain EXPLAIN of caller SQL) | 243 |
| 38 | POST | /instances/{id}/settings/snapshot | IM | PG-READ + STORE-W | 244 |
| 39 | POST | /instances/{id}/snapshots/capture | IM | PG-READ + STORE-W | 245 |
| 40 | POST | /instances/{id}/sessions/{pid}/cancel | IM | **PG-WRITE** | 246 |
| 41 | POST | /instances/{id}/sessions/{pid}/terminate | IM | **PG-WRITE** | 247 |
| 42 | POST | /instances/{id}/explain | IM | **PG-WRITE** (with `analyze:true`) | 248 |
| 43 | GET | /alerts | auth | R | 253 |
| 44 | GET | /alerts/history | auth | R | 254 |
| 45 | GET | /alerts/rules | auth | R | 255 |
| 46 | POST | /alerts/rules | AM | STORE-W | 260 |
| 47 | PUT | /alerts/rules/{id} | AM | STORE-W | 261 |
| 48 | DELETE | /alerts/rules/{id} | AM | STORE-W | 262 |
| 49 | POST | /alerts/test | AM | EXT (test e-mail) | 263 |
| 50 | GET | /instances/{id}/recommendations | auth | R | 270 |
| 51 | POST | /instances/{id}/diagnose | auth | R | 271 |
| 52 | GET | /recommendations | auth | R | 272 |
| 53 | GET | /recommendations/rules | auth | R | 273 |
| 54 | PUT | /recommendations/{id}/acknowledge | AM | STORE-W | 277 |
| 55 | POST | /instances/{id}/rca/analyze | auth | STORE-W | 283 |
| 56 | GET | /instances/{id}/rca/incidents | auth | R | 284 |
| 57 | GET | /instances/{id}/rca/incidents/{incidentId} | auth | R | 285 |
| 58 | GET | /rca/incidents | auth | R | 286 |
| 59 | GET | /rca/graph | auth | R | 287 |
| 60 | PUT | /rca/incidents/{incidentId}/review | AM | STORE-W | 291 |
| 61 | GET | /playbooks | auth | R | 297 |
| 62 | GET | /playbooks/resolve | auth | R | 298 |
| 63 | POST | /playbooks | AM | STORE-W (stores caller SQL and caller-chosen tier) | 302 |
| 64 | GET | /playbooks/{id} | auth | R | 306 |
| 65 | PUT | /playbooks/{id} | AM | STORE-W (same; can overwrite built-ins) | 309 |
| 66 | POST | /playbooks/{id}/deprecate | AM | STORE-W | 310 |
| 67 | DELETE | /playbooks/{id} | UM | STORE-W | 314 |
| 68 | POST | /playbooks/{id}/promote | UM | STORE-W | 315 |
| 69 | POST | /instances/{id}/playbooks/{playbookId}/run | auth | STORE-W | 319 |
| 70 | GET | /instances/{id}/playbook-runs | auth | R | 320 |
| 71 | GET | /playbook-runs | auth | R | 323 |
| 72 | GET | /playbook-runs/{runId} | auth | R | 325 |
| 73 | POST | /playbook-runs/{runId}/abandon | auth | STORE-W | 326 |
| 74 | POST | /playbook-runs/{runId}/feedback | auth | STORE-W | 327 |
| 75 | POST | /playbook-runs/{runId}/steps/{stepOrder}/execute | auth + tier check in handler | **PG-WRITE** (section 6) | 329 |
| 76 | POST | /playbook-runs/{runId}/steps/{stepOrder}/skip | auth | STORE-W | 330 |
| 77 | POST | /playbook-runs/{runId}/steps/{stepOrder}/retry | auth | STORE-W | 331 |
| 78 | POST | /playbook-runs/{runId}/steps/{stepOrder}/request-approval | auth | STORE-W | 332 |
| 79 | POST | /playbook-runs/{runId}/steps/{stepOrder}/confirm | IM | none (handler is a no-op) | 335 |
| 80 | POST | /playbook-runs/{runId}/steps/{stepOrder}/approve | IM | STORE-W (resets step) | 336 |
| 81 | GET | /instances/{id}/forecast/eta | auth | R | 346 |
| 82 | GET | /instances/{id}/forecast/eta/{pid} | auth | R | 347 |
| 83 | GET | /instances/{id}/forecast/needs | auth | R | 348 |
| 84 | GET | /instances/{id}/forecast/needs/{database}/{table} | auth | R | 349 |
| 85 | GET | /instances/{id}/forecast/history | auth | R | 350 |
| 86 | POST | /instances | IM | STORE-W (new DSN PGPulse will connect to) | 358 |
| 87 | PUT | /instances/{id} | IM | STORE-W | 359 |
| 88 | DELETE | /instances/{id} | IM | STORE-W | 360 |
| 89 | POST | /instances/bulk | IM | STORE-W | 361 |
| 90 | POST | /instances/{id}/test | IM | EXT (connects to any DSN in the body) | 362 |
| 91 | POST | /auth/register | UM | STORE-W | 369 |
| 92 | GET | /auth/users | UM | R | 370 |
| 93 | PUT | /auth/users/{id} | UM | STORE-W | 371 |
| 94 | DELETE | /auth/users/{id} | UM | STORE-W | 372 |
| 95 | PUT | /auth/users/{id}/password | UM | STORE-W | 373 |
| 96 | PUT | /auth/me/password | auth (self) | STORE-W | 375 |

Counts (auth enabled): public 4 · any authenticated 63 · IM 13 · AM 9 · UM 7. Routes reachable per role:
super_admin 96, dba 89, app_admin 76, roles_admin 74.

### 4.4 In-handler checks and their limits

- `/auth/register` and `PUT /auth/users/{id}`: only `super_admin` can create or assign `super_admin`/`roles_admin`
  (`internal/api/auth.go:225-231`, `:263-281`).
- `DELETE /auth/users/{id}`: blocks self-delete and deleting the last `super_admin` (`internal/api/auth.go:303-327`) —
  a `roles_admin` can therefore delete any other `super_admin`.
- `PUT /auth/users/{id}/password`: **no check on the target's role** (`internal/api/auth.go:338-378`) — a `roles_admin`
  can reset a `super_admin`'s password and log in as that user.
- Playbook execute: tier switch (`internal/api/playbooks.go:357-398`), detailed in section 6.3.

---

## 5. Data stored

PGPulse's own storage database is created by embedded migrations (`internal/storage/migrate.go:14-15`, `:76-96`).
Without `storage.dsn` PGPulse runs in **live mode**: metrics are kept only in memory for `--history` (default 2 h)
and nothing is persisted (`main.go:49`, `:496-518`; `internal/storage/memory.go:209-224`).

### 5.1 Tables

| Table | Migration | Contents | Sensitive fields |
|---|---|---|---|
| `schema_migrations` | `internal/storage/migrate.go:27-31` | applied migration names | — |
| `metrics` (TimescaleDB hypertable if `use_timescaledb`) | `001_metrics.sql`, `002_timescaledb.sql` | time, instance_id, metric, value, labels (JSONB) | labels only; statement metrics are labelled by queryid/dbid/userid, no text (`internal/collector/statements_top.go:175-179`) |
| `users` | `003_users.sql`, `005_expand_roles.sql` | username, bcrypt password_hash, role, active, last_login | password hash |
| `alert_rules`, `alert_history` | `004_alerts.sql`, `011_forecast_alert_consecutive.sql` | rules; fired/resolved events with value and labels | — |
| `instances` | `006_instances.sql` | registry of monitored servers | **`dsn` in plaintext, including password** (`006_instances.sql:4`) |
| `session_audit_log` | `007_session_audit_log.sql` | intended audit of cancel/terminate | **never written** — no Go code references it |
| `query_plans` | `008_plan_capture.sql` | EXPLAIN output | **`query_text`, `plan_text`, `plan_json`** (`008_plan_capture.sql:7-11`) |
| `settings_snapshots` | `009_settings_snapshots.sql` | JSONB of `pg_settings` | server configuration values |
| `ml_baseline_snapshots` | `010_ml_baseline_snapshots.sql` | model state per metric | — |
| `remediation_recommendations` | `013`, `014`, `017` | advisor output, acknowledged_by | usernames |
| `pgss_snapshots`, `pgss_snapshot_entries` | `015_pgss_snapshots.sql` | pg_stat_statements snapshots | **`query` text (normalized, up to 8192 chars)**, database_name, user_name (`015_pgss_snapshots.sql:23-25`; `internal/statements/capture.go:31`) |
| `rca_incidents` | `016_rca_incidents.sql` | RCA timelines, summaries, reviewer comments | usernames, free text |
| `playbooks`, `playbook_steps`, `playbook_runs`, `playbook_run_steps` | `018_playbooks.sql` | runbooks incl. `sql_template`; runs incl. `sql_executed`, `result_json`, `confirmed_by` | SQL; up to 100 result rows per step from monitored DBs (`internal/config/load.go:173-175`) |
| `maintenance_operations`, `maintenance_forecasts` | `019_maintenance_forecasting.sql` | vacuum/analyze/reindex history and forecasts | database and table names |

### 5.2 Is statement text stored?

**Yes, in two places:**

1. `pgss_snapshot_entries.query` — normalized pg_stat_statements text, up to 8192 characters, for every row returned
   by pg_stat_statements (the capture SQL has no top-N limit), when `statement_snapshots.enabled` is true or a
   snapshot is captured manually (`internal/statements/capture.go:31-50`; `main.go:376-390`; route #39).
   Normalized text replaces constants with `$n` placeholders but can still contain literals in utility statements.
2. `query_plans.query_text` — the raw query a user submits to `POST /instances/{id}/plans/capture` (route #37),
   stored with its plan, even when `plan_capture.enabled` is false (`internal/api/plan_handlers.go:183-199`).
   The automatic plan collector is constructed but not started (`main.go:163-173`, `_ = planCollector`).

Live activity, lock-tree and long-transaction views show query text (truncated to 200 characters) but do not persist it
(`internal/api/activity.go:172`; `internal/api/handler_locks.go:97`).

### 5.3 Retention

| Data | Config key (default) | Enforced? | Source |
|---|---|---|---|
| `metrics` | `storage.retention_days` (30) | **No.** The value is defaulted but never read; no DELETE and no TimescaleDB retention policy exists. | `internal/config/load.go:83-85`; no other reference to `Storage.RetentionDays` |
| `alert_history` | `alerting.history_retention_days` (30) | Yes, hourly, resolved events only, when alerting is enabled | `internal/alert/pgstore.go:351-361`; `internal/alert/evaluator.go:415-428` |
| `pgss_snapshots` | `statement_snapshots.retention_days` (30) | Yes, each capture cycle, when enabled | `internal/statements/pgstore.go:331`; `internal/statements/capture.go:141-145` |
| `remediation_recommendations` | `remediation.retention_days` (30) | Yes, acknowledged/resolved rows, when remediation is enabled | `internal/remediation/pgstore.go:338-347` |
| `maintenance_operations` | `maintenance_forecast.retention_days` (90) | Yes, daily, when enabled | `internal/forecast/engine.go:66-88` |
| `query_plans` | `plan_capture.retention_days` (no default → 0) | Only when `plan_capture.enabled` and the value is > 0 | `internal/plans/retention.go:24-44`; `main.go:176-180` |
| `rca_incidents` | `rca.retention_days` (90) | **No** — cleanup function exists but is never called | `internal/rca/pgstore.go:291-299` |
| `playbook_runs` | `playbooks.run_retention_days` (90) | **No** — `CleanOldRuns` is never called | `internal/playbook/pgstore.go:643-650` |
| `settings_snapshots` | `settings_snapshot.retention_days` | **No** — no deletion code | — |
| `users`, `instances`, `alert_rules`, `ml_baseline_snapshots` | — | kept until deleted | — |

---

## 6. Write actions PGPulse can perform on a monitored database

This is the complete list found by reading `internal/api/`, `internal/playbook/`, `internal/plans/`,
`internal/statements/`, `internal/collector/` and `internal/orchestrator/`. No code path issues `pg_stat_reset`,
`pg_stat_statements_reset`, `pg_reload_conf`, `ALTER SYSTEM`, `VACUUM`, `ANALYZE`, `REINDEX`, `CREATE`, `DROP`,
`COPY ... TO PROGRAM` or `auto_explain` — those strings appear only as advice text for humans
(`internal/remediation/rules_pg.go`; `ManualInstructions` in `internal/playbook/seed_*.go`).

### 6.1 Table of actions

| # | Action | SQL | Source | Route / trigger | Required role (auth on) | Safeguards and gaps |
|---|---|---|---|---|---|---|
| W1 | Cancel a session's query | `SELECT pg_cancel_backend($1)` | `internal/api/session_actions.go:94-101` | route #40 | IM | Refuses PGPulse's own API backend and superuser sessions (`:63-89`). Result error discarded (`:101`). Audit is one `slog.Info` line only (`:104-113`). Requires `pg_signal_backend`. |
| W2 | Terminate a session | `SELECT pg_terminate_backend($1)` | `internal/api/session_actions.go:94-101` | route #41 | IM | As W1. |
| W3 | EXPLAIN / **EXPLAIN ANALYZE** of caller SQL | `EXPLAIN (FORMAT JSON[, ANALYZE][, BUFFERS]) <caller text>` built with `fmt.Sprintf` | `internal/api/explain.go:66-83` | route #42 | IM | `statement_timeout = 30s` (`:57`). With `analyze:true` the statement **executes and commits** (autocommit): INSERT/UPDATE/DELETE, `CREATE TABLE AS`, or `SELECT <volatile function>` such as `pg_terminate_backend` (bypassing W2's superuser guard). Single statement only under pgx's default extended protocol. Caller chooses the database. No audit record. |
| W4 | Plain EXPLAIN of caller SQL | `"EXPLAIN (FORMAT JSON) " + <caller text>` | `internal/api/plan_handlers.go:171-174` | route #37 | IM | Plans but does not execute. No statement_timeout. Query text stored (section 5.2). |
| W5 | Playbook step execution | `BEGIN; SET LOCAL statement_timeout; SET LOCAL lock_timeout = '5s'; [diagnostic: SET LOCAL default_transaction_read_only = ON]; <step SQL>; ROLLBACK` | `internal/playbook/executor.go:43-127` | route #75 | any authenticated user, plus tier check (6.3) | Never committed (rollback `:68`), so transactional writes are discarded; **non-transactional effects persist** (e.g. `pg_terminate_backend`, `pg_cancel_backend`, `pg_reload_conf`, `nextval`, statistics resets) within the DSN role's privileges. Multi-statement text rejected (`:49-54`). Result rows capped at 100. Executed SQL recorded in `playbook_run_steps.sql_executed`. |
| W6 | Session-local settings on PGPulse's own connections | `SET statement_timeout`, `SET application_name` | `internal/collector/database.go:29`; `internal/statements/capture.go:192-195`; `internal/api/explain.go:57-64`; `internal/orchestrator/runner.go:46` | background | — | Affect only PGPulse's own sessions. |

### 6.2 Playbook "dangerous" tier — every built-in step

Tiers are `diagnostic`, `remediate`, `dangerous`, `external` (`internal/playbook/types.go:53-59`). Built-in playbooks
are seeded with `INSERT ... ON CONFLICT (slug) DO NOTHING` (`internal/playbook/seed.go:10-11`). Exactly **one**
built-in step is in the `dangerous` tier:

| Playbook | Step | SQL | Source |
|---|---|---|---|
| `lock-contention` | 4 — "Terminate blocking sessions" | `SELECT count(*) AS candidate_count FROM pg_stat_activity WHERE state = 'idle in transaction' AND xact_start < now() - interval '10 minutes' AND pid != pg_backend_pid()` | `internal/playbook/seed_locks.go:81-93` |

That step only counts candidates; its message tells the operator to run `pg_terminate_backend` by hand. No built-in
step uses `remediate`. All other non-diagnostic built-in steps are `external` (manual instructions, never executed):
`internal/playbook/seed_checkpoint.go:75`, `seed_connections.go:94`, `seed_disk.go:87`, `seed_locks.go:98`,
`seed_longtx.go:85`, `seed_query.go:78`, `seed_replication.go:86`, `seed_vacuum.go:94`, `seed_wal.go:87`,
`seed_wraparound.go:83`. Every built-in `SQLTemplate` is a `SELECT`.

### 6.3 Playbook gating as implemented

`internal/api/playbooks.go:357-398`:

- `diagnostic` — runs with no further check.
- `remediate` — runs when the request body contains `{"confirmed": true}`; **no role check**.
- `dangerous` — runs when the *caller* has `instance_management`; no recorded approval is required.
- `external` — never executed; marked completed.
- **any other tier string** — no `default` case, so it **runs with no check**.

The `/confirm` endpoint is a no-op and `/approve` only resets the step (`internal/api/playbooks.go:478-495`).

Playbooks (step SQL **and** tier) can be created and edited by `alert_management`, i.e. including `app_admin`
(`server.go:300-311`; `internal/api/playbooks.go:63-115`); the tier column has no CHECK constraint
(`internal/storage/migrations/018_playbooks.sql:32-33`); runs can be started on draft playbooks
(`internal/api/playbooks.go:210-275`). **Consequence:** an `app_admin` can author a single-statement step and execute it
on any monitored instance as the monitoring role, with the W5 transaction semantics.

**[tested]** The diagnostic tier's read-only guard does not take effect: in PostgreSQL 16,
`BEGIN; SET LOCAL default_transaction_read_only = ON; SHOW transaction_read_only;` returns `off` and a following
`INSERT` succeeds. The setting changes the default for *later* transactions only
(`internal/playbook/executor.go:64`, `:83-87`).

### 6.4 Read-only protection at the connection level

None. No connection sets `default_transaction_read_only` or a read-only transaction mode
(`internal/orchestrator/orchestrator.go:229-245`; `internal/orchestrator/runner.go:36-46`). The effective limit on
W3 and W5 is therefore the privilege set of the role in the DSN (section 2.3).

---

## 7. Blast radius per `os_metrics_method`

The effective method per instance is: `instances[].os_metrics_method`, else global `os_metrics.method`, else
**`sql`** (`internal/orchestrator/runner.go:203-213`; mirrored by `Config.EffectiveOSMetricsMethod` in
`internal/config/config.go`). `pgpulse-server` logs one `WARN` line at startup naming every enabled instance that
resolves to `sql` (`WarnOSMetricsSQL` in `internal/config/config.go`, called from `main.go` after CLI overrides).

| | `sql` (default) | `agent` | `disabled` |
|---|---|---|---|
| Mechanism | `SELECT pg_read_file($1)` for 5 `/proc` files every 60 s through the monitored PostgreSQL connection (`internal/collector/os_sql.go:79-117`; registered at `internal/orchestrator/runner.go:109-112`) | HTTP GET to `pgpulse-agent` on the DB host, which reads procfs/sysfs as its own OS user (`internal/agent/scraper.go:22-70`; `internal/agent/osmetrics_linux.go`) | No OS collector registered |
| Extra grants on the monitored PostgreSQL | `pg_read_server_files` **+** `EXECUTE ON pg_read_file(text)` | none beyond `pg_monitor` | none beyond `pg_monitor` |
| What a compromise of the monitoring credential gives | **Read access to every file the `postgres` OS user can read**, at any path, through any client — not just `/proc`. **[tested]** With these grants the monitoring role read `/etc/postgresql/16/main/pg_hba.conf` and the server TLS key `/etc/ssl/private/ssl-cert-snakeoil.key`; reads of `/etc/shadow` failed only on OS file permissions. This includes the data directory (table files of every database, bypassing SQL-level privileges), configuration and key material. | Same as `pg_monitor`. Separately, anyone who can reach port 9187 can read the host's OS metrics, mount points and cluster membership without authentication (section 1.1). | Same as `pg_monitor`. |
| New network exposure | none | listener on DB hosts (`0.0.0.0:9187`, plain HTTP); outbound HTTP from server to agent | none |
| New process on DB host | none | `pgpulse-agent` (systemd unit runs as `User=pgpulse`, no hardening directives: `deploy/systemd/pgpulse-agent.service:9-16`); may exec `patronictl`/`etcdctl` with fixed arguments | none |
| Metrics lost | — | — | CPU, memory, load, disk I/O, uptime |

Independent of the method: the server-info collector calls `pg_read_file('/etc/hostname')` and
`pg_read_file('/etc/os-release')` every low-frequency cycle (`internal/collector/server_info.go:109-141`). It
fails soft when the grants are absent; with `agent` or `disabled`, simply do not grant them.

**Recommendation for accreditation:** use `agent` (with network ACLs restricting port 9187 to the PGPulse server)
or `disabled`, and grant only `pg_monitor`.

---

## 8. Dependencies and supply chain

### 8.1 Go (`go.mod`)

Go 1.25 (`go.mod:3`). 13 direct requirements (`go.mod:5-19`), 96 indirect.

| Module | Version | Used in |
|---|---|---|
| github.com/go-chi/chi/v5 | v5.2.5 | HTTP router (server) |
| github.com/golang-jwt/jwt/v5 | v5.2.2 | JWT (server) |
| github.com/jackc/pgx/v5 | v5.8.0 | PostgreSQL driver |
| github.com/knadh/koanf/v2 | v2.3.2 | config |
| github.com/knadh/koanf/parsers/yaml | v1.1.0 | config |
| github.com/knadh/koanf/providers/env | v1.1.0 | config |
| github.com/knadh/koanf/providers/file | v1.2.1 | config |
| golang.org/x/crypto | v0.47.0 | bcrypt |
| gonum.org/v1/gonum | v0.17.0 | ML statistics |
| github.com/stretchr/testify | v1.11.1 | tests only |
| github.com/testcontainers/testcontainers-go | v0.40.0 | tests only |
| github.com/testcontainers/testcontainers-go/modules/postgres | v0.40.0 | tests only |
| github.com/wailsapp/wails/v3 | v3.0.0-alpha.74 (**alpha**) | desktop build only (`//go:build desktop`, `internal/desktop/`, `cmd/pgpulse-server/desktop.go:1`) |

Measured with `go list -deps`, the standard `pgpulse-server` and `pgpulse-agent` binaries link 21 third-party modules
(chi, jwt, pgx and its pgpassfile/pgservicefile/puddle, koanf and its maps/yaml/env/file/mapstructure/copystructure/
reflectwalk, fsnotify, go.yaml.in/yaml/v3, x/crypto, x/sync, x/sys, x/text, gonum); `-tags desktop` raises this to 25.
`go.sum` pins hashes for all modules.

### 8.2 Web UI (`web/package.json`)

9 dependencies (@tanstack/react-query, @tanstack/react-query-devtools, echarts, echarts-for-react, lucide-react,
react, react-dom, react-router-dom, zustand) and 17 devDependencies (build/lint tooling: vite, typescript, eslint,
tailwindcss, postcss, autoprefixer, prettier, madge and type packages). `web/package-lock.json` is committed and
resolves 410 packages, 23 of them non-dev. Versions in `package.json` are caret/tilde ranges; reproducible builds rely
on `npm ci` with the lockfile. The built `web/dist` is embedded into the Go binary (`web/embed.go:8`;
`internal/api/static.go:14`), so Node.js and `node_modules` are build-time only. The React Query devtools are rendered
in production builds (`web/src/main.tsx:4`, `:29`).

### 8.3 Build and deploy artefacts

- `deploy/docker/Dockerfile`: builder `golang:1.23-alpine` (older than `go.mod`'s 1.25), does not build `web/dist`,
  runtime image has no `USER` line, so the process runs as root (`deploy/docker/Dockerfile:2`, `:14-22`).
- `deploy/docker/docker-compose.yml`: floating tag `timescale/timescaledb:latest-pg16` (`:18`).
- `deploy/systemd/pgpulse-agent.service`: runs as `pgpulse`, no sandboxing directives. No server unit exists.
- `deploy/helm/`: empty.

---

## 9. Logging

- Both binaries log with `log/slog` text format to **stderr** (`main.go:55-57`, `:119`;
  `cmd/pgpulse-agent/main.go:28-30`). Server level comes from `server.log_level` (default `info`,
  `internal/config/load.go:63-68`); the agent is fixed at info. Under systemd output goes to the journal.
- Some collectors log through the process-default `slog` logger rather than the configured one, so their lines have a
  different format (e.g. `internal/collector/os_sql.go:113`; `internal/api/session_actions.go:104-113`).
- HTTP access log (`internal/api/middleware.go:54-70`): method, path, status, duration, request id. **Client IP,
  user, query strings and bodies are not logged.** The request id is returned as `X-Request-ID`
  (`internal/api/middleware.go:23-29`).
- Panics are logged with stack trace (`internal/api/middleware.go:73-89`).
- **Audit trail:** session cancel/terminate produces one `slog.Info("session action", ...)` line with instance, pid,
  action, success and user (`internal/api/session_actions.go:104-113`); the `session_audit_log` table is never written.
  EXPLAIN execution, plan capture, instance CRUD and user management produce **no audit record**. Playbook steps are
  recorded in `playbook_run_steps` (`sql_executed`, `confirmed_by`).
- No log forwarding (syslog, OTLP, etc.) is built in.
- Startup warning added for vetting: when any enabled instance resolves to `os_metrics_method: sql`, one `WARN` line
  is logged, e.g. **[tested]**
  `level=WARN msg="os_metrics_method=sql: OS metrics are read with pg_read_file() on the monitored server, which needs pg_read_server_files; consider os_metrics_method agent or disabled, see docs/security/VETTING.md" instances=[cli-target]`
  (`internal/config/config.go`, `WarnOSMetricsSQL`).

---

## 10. Known gaps

Each item is a fact about the current code, ordered roughly by severity. None is fixed by the change that added this
document.

1. **Auth off by default; auth-off means implicit `super_admin` for every caller**, on all interfaces, including
   session terminate and EXPLAIN ANALYZE (`internal/config/config.go:95`; `internal/auth/middleware.go:33-46`;
   `server.go:377-525`). Live mode cannot enable auth (`internal/config/load.go:273-275`).
2. **No TLS on the server or agent listeners** (`main.go:644`; `internal/agent/server.go:60`).
3. **Default `os_metrics_method: sql` needs file-read grants equivalent to reading the PostgreSQL data directory and
   key material** (section 7). The default is unchanged pending an owner decision; a startup warning is logged.
4. **`app_admin` can get arbitrary single-statement SQL executed on monitored servers** through playbooks; unknown
   tier strings bypass all gating; approval endpoints do not gate execution (section 6.3).
5. **EXPLAIN ANALYZE executes caller SQL** in autocommit, built by string formatting, with no audit record
   (`internal/api/explain.go:66-83`).
6. **Playbook read-only guard is ineffective** (section 6.3, tested).
7. **No connection-level read-only mode; no check that the monitoring role is not a superuser** (sections 2.3, 6.4).
8. **`roles_admin` can reset any `super_admin`'s password** (`internal/api/auth.go:338-378`).
9. **Unauthenticated `pgpulse-agent`** (`internal/agent/server.go:42-60`).
10. **Plaintext DSNs (with passwords) in the `instances` table** (`internal/storage/migrations/006_instances.sql:4`).
11. **Session audit table never written; no audit for EXPLAIN, instance CRUD or user management; access log omits
    client IP and user** (section 9).
12. **Login rate limit trusts `X-Forwarded-For` / `X-Real-IP` from any client**, so it can be bypassed by header
    spoofing (`internal/auth/ratelimit.go:83-98`); `/auth/refresh` is not rate-limited.
13. **Refresh endpoint does not check the token type**, and `refresh_secret` defaults to `jwt_secret`, so an access
    token is accepted as a refresh token (`internal/auth/jwt.go:125-141`; `main.go:465-467`).
14. **Tokens are not revocable**; role changes and deactivation take effect only at token expiry
    (`internal/auth/middleware.go:70-82`).
15. **`POST /instances/{id}/test` connects to arbitrary hosts** from the request body (`internal/api/instances_crud.go:308-360`).
16. **Retention not enforced** for `metrics`, `rca_incidents`, `playbook_runs`, `settings_snapshots`, and by default
    `query_plans` (section 5.3).
17. **Statement text is persisted** in `pgss_snapshot_entries` and `query_plans` (section 5.2).
18. **Environment overrides cannot set keys containing `_`** (e.g. `auth.jwt_secret`), which pushes secrets into the
    YAML file; `${VAR}` is not expanded (`internal/config/load.go:42-45`).
19. **Read handlers' `SET LOCAL statement_timeout` is issued outside a transaction**, so it has no effect and those
    live reads run without a statement timeout (`internal/api/activity.go:69`, `:162`;
    `internal/api/handler_statements.go:112`; `internal/api/handler_locks.go:81`; `internal/api/replication.go:81`).
20. **Default `max_conns` is 5** per instance plus uncapped per-request API connections, above the project rule of
    3 (`internal/config/load.go:203-204`; `internal/orchestrator/orchestrator.go:195-245`).
21. **Google Fonts referenced by the UI** (`web/index.html:8-12`) — harmless but visible as blocked egress attempts.
22. **Container image runs as root and does not build** as shipped (`deploy/docker/Dockerfile`).
23. **`--target` quick-start without a config file panics** (`non-positive interval for NewTicker`,
    `internal/orchestrator/group.go:72`) because intervals are not defaulted on that path (`main.go:84-89`,
    `:712-736`). **[tested]** on the base commit.
24. **Wails v3 is an alpha release** (desktop build only; `go.mod:16`).
