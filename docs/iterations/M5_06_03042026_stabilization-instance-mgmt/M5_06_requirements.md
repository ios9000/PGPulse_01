# M5_06 — Stabilization + Instance Management: Requirements

**Iteration:** M5_06
**Date:** 2026-03-04
**Depends on:** M5_01–M5_05 (complete), M4 (alert engine, complete)

---

## Goal

Fix the three issues discovered during demo (connection contention, missing instance names, evaluator nil panic) and build full instance management — CRUD via UI, CSV bulk import, YAML seeding on startup with DB as source of truth. After M5_06, DBAs can manage their monitored fleet entirely through the web UI.

---

## Decisions (Locked)

| ID | Decision | Rationale |
|----|----------|-----------|
| D108 | pgxpool.Pool replaces single pgx.Conn per instance | Three interval groups need concurrent connections; pool size 3–5 per instance |
| D109 | YAML seeds on startup via INSERT ON CONFLICT DO NOTHING | YAML provides initial fleet; DB additions via UI persist across restarts; YAML never overwrites DB edits |
| D110 | Instance CRUD stored in `instances` table (already exists in migrations) | DB is source of truth after first startup |
| D111 | Orchestrator hot-reload: poll DB for instance changes every 60s | Adding an instance via UI takes effect within 60s without restart |
| D112 | CSV bulk import: id, name, dsn, enabled columns | Minimal schema; matches YAML structure; DBAs can export from spreadsheets |
| D113 | Manual add + CSV bulk import only; no CMDB integration yet | Keep scope tight; CMDB hook designed but deferred to M8 |
| D114 | `source` column on instances: 'yaml' or 'manual' | Lets UI show which instances came from config vs were added manually; informational only |

---

## Functional Requirements

### FR-1: Connection Pool (Backend Stabilization)

**FR-1.1** Replace `pgx.Conn` with `pgxpool.Pool` in the orchestrator's instance runner.

**FR-1.2** Pool configuration per instance: min_conns=1, max_conns=5 (configurable in YAML), idle timeout 5 minutes.

**FR-1.3** Each interval group acquires a connection from the pool for its collection cycle, releases it when done. No more "conn busy" errors.

**FR-1.4** Connection health check: pool automatically reconnects on transient failures (pgxpool default behavior).

**FR-1.5** Graceful shutdown: pool.Close() called during SIGINT/SIGTERM handling.

### FR-2: Evaluator Nil Guard Fix

**FR-2.1** Remove the nil guard hack in `internal/orchestrator/group.go`. Instead, wire the evaluator properly in main.go — always create an evaluator (even if alerting is disabled, create a no-op evaluator).

**FR-2.2** If `alerting.enabled = false`, use a `NoOpEvaluator` that satisfies the `AlertEvaluator` interface but discards all calls.

### FR-3: Instance Name in API

**FR-3.1** Add `name` field to the instance config struct in `internal/config/config.go`.

**FR-3.2** API response for `GET /api/v1/instances` includes `name` field.

**FR-3.3** Frontend sidebar displays: `instance.name || instance.id || ${instance.host}:${instance.port}`.

### FR-4: Instance CRUD API

**FR-4.1** `POST /api/v1/instances` — Create a new instance. Fields: id (optional, auto-generated UUID if omitted), name, dsn, enabled. Returns created instance with id.

**FR-4.2** `PUT /api/v1/instances/:id` — Update instance name, DSN, enabled status.

**FR-4.3** `DELETE /api/v1/instances/:id` — Remove instance from DB. Orchestrator stops monitoring within 60s.

**FR-4.4** `GET /api/v1/instances` — Already exists; add `name`, `source` fields to response.

**FR-4.5** `GET /api/v1/instances/:id` — Already exists; add `name`, `source` fields.

**FR-4.6** All mutation endpoints require `admin` role (RBAC enforced).

**FR-4.7** DSN validation on create/update: attempt a test connection (with 5s timeout) and return error if unreachable. Include PG version in success response.

### FR-5: CSV Bulk Import

**FR-5.1** `POST /api/v1/instances/bulk` — Accept CSV body (Content-Type: text/csv) or JSON array (Content-Type: application/json).

**FR-5.2** CSV format: `id,name,dsn,enabled` with header row. `id` optional (auto-generated if empty). `enabled` defaults to true if omitted.

**FR-5.3** Response: array of results per row — success (with id) or error (with row number and message).

**FR-5.4** Partial success allowed: valid rows are imported, invalid rows return errors. Not transactional (each row independent).

**FR-5.5** Requires `admin` role.

### FR-6: YAML Seeding

**FR-6.1** On startup, read `instances` section from YAML config.

**FR-6.2** For each YAML instance, execute `INSERT INTO instances (id, name, host, port, dsn, enabled, source) VALUES (...) ON CONFLICT (id) DO NOTHING`.

**FR-6.3** `source` column set to `'yaml'` for seeded instances.

**FR-6.4** Instances added via API have `source = 'manual'`.

**FR-6.5** Log: `level=INFO msg="seeded instance from config" id=local-dev source=yaml` for new seeds, `level=DEBUG msg="instance already exists, skipping seed" id=local-dev` for existing.

### FR-7: Orchestrator Hot-Reload

**FR-7.1** Every 60 seconds, orchestrator queries `instances` table for the current list.

**FR-7.2** New instances (not currently monitored): start a new instance runner with pool.

**FR-7.3** Removed instances (in runner but not in DB): stop runner, close pool.

**FR-7.4** Updated instances (DSN changed): stop old runner, start new runner with new DSN.

**FR-7.5** Disabled instances (`enabled = false`): stop runner if running.

**FR-7.6** Log all changes: `level=INFO msg="started monitoring" id=new-server`, `level=INFO msg="stopped monitoring" id=removed-server`.

### FR-8: Administration Page (Frontend)

**FR-8.1** Replace the placeholder Administration page with a real instance management UI.

**FR-8.2** Instance list table: name, host:port (extracted from DSN or stored), source badge (yaml/manual), enabled toggle, status indicator (connected/error), PG version, actions (edit, delete).

**FR-8.3** "Add Instance" button opens a form/modal: name, DSN, enabled toggle. On submit, calls POST /api/v1/instances. Shows connection test result before confirming.

**FR-8.4** Edit button opens same form pre-populated. Calls PUT /api/v1/instances/:id.

**FR-8.5** Delete button with confirmation dialog. Calls DELETE /api/v1/instances/:id. YAML-sourced instances show warning: "This instance will be re-added on next restart if still in YAML config."

**FR-8.6** "Bulk Import" button opens modal with:
- Textarea for pasting CSV
- File upload button for .csv files
- Preview table showing parsed rows before import
- Import button that calls POST /api/v1/instances/bulk
- Results: green checkmarks for success, red X with error message per row

**FR-8.7** Enable/disable toggle calls PUT inline (no modal needed).

**FR-8.8** Auto-refresh instance list every 30 seconds.

**FR-8.9** Permission-gated: entire page requires `admin` role.

### FR-9: Users Management Tab

**FR-9.1** Administration page has two tabs: "Instances" (FR-8) and "Users" (placeholder for M5_08).

**FR-9.2** Users tab shows: "User management coming soon" placeholder — not part of this iteration.

---

## Non-Functional Requirements

**NFR-1** Connection pool overhead: max 5 PG connections per monitored instance (configurable). For 10 instances = 50 connections total.

**NFR-2** Hot-reload poll interval configurable (default 60s). Not real-time — acceptable latency for instance management operations.

**NFR-3** CSV parser handles: quoted fields, commas in DSN, UTF-8 encoding, Windows line endings (CRLF).

**NFR-4** DSN test connection timeout: 5 seconds. Non-blocking — doesn't hold up the API response beyond the timeout.

**NFR-5** All existing tests must continue to pass. New tests for: pool lifecycle, instance CRUD, CSV parsing, YAML seeding, hot-reload logic.

**NFR-6** Frontend: no new npm dependencies. Use existing Tailwind, TanStack Query, Lucide icons.

---

## Out of Scope

- User management CRUD (M5_08)
- CMDB / REST import integration (M8)
- Instance grouping / tagging / folders
- Instance health history / uptime tracking
- Connection pool metrics exposed to the UI
- Notification channel management UI (stays in YAML)
- Per-database collector connections (separate milestone)
