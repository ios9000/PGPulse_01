# PGPulse — M8_01 Requirements
**Milestone:** M8_01 — P1 Features: Session Kill + On-Demand Query Plans + Cross-Instance Settings Diff
**Date:** 2026-03-08
**Iteration folder:** `docs/iterations/M8_01_03082026_p1-features/`

---

## Overview

M8_01 ships three high-value operational features that turn PGPulse from a read-only
monitoring tool into an interactive DBA workbench. All three features are stateless
or use simple storage — no background workers, no new collection loops.

M8_02 (deferred) will add: auto-capture query plans (background), temporal settings diff
(snapshots table + scheduler).

---

## Feature 1: Session Kill

### What
DBA and Admin roles can cancel or terminate PostgreSQL backend sessions directly
from the PGPulse UI. Two operations:

- **Cancel** (`pg_cancel_backend(pid)`) — sends SIGINT to the backend. Cancels the
  current query; the connection stays open. Safe default.
- **Terminate** (`pg_terminate_backend(pid)`) — sends SIGTERM. Kills the connection.
  Use only when cancel is insufficient.

### Why
PGAM was read-only. The single most common DBA action on a stuck session — killing it
— required switching to psql. This closes that gap.

### Roles
- `dba` and `super_admin` can cancel and terminate any session.
- `app_admin` and `viewer` cannot perform either operation.

### Audit
Every cancel/terminate attempt (success or failure) must be written to an audit log:
`instance_id`, `operator_user_id`, `target_pid`, `operation` (cancel/terminate),
`result` (ok/error), `error_message`, `executed_at`. Store in a new `session_audit_log`
table. Retention: keep 90 days by default.

### UI
- Active Sessions section on ServerDetail page gains two icon buttons per row:
  `[✕ Cancel]` `[⚡ Terminate]`.
- Both show a confirmation modal before executing:
  - Cancel: "Cancel query for PID {pid} ({application_name})? The connection will remain open."
  - Terminate: "Terminate session PID {pid} ({application_name})? **This will close the connection.**"
  - Terminate modal uses a warning/destructive color (red).
- After action: show inline toast (success or error), refresh activity list.

### API
```
POST /api/v1/instances/:id/sessions/:pid/cancel
POST /api/v1/instances/:id/sessions/:pid/terminate
```
Both require `Authorization: Bearer <token>` with dba or super_admin role.
Returns `{ "ok": true }` or `{ "ok": false, "error": "..." }`.

---

## Feature 2: On-Demand Query Plans

### What
Any DBA or Admin can submit a SQL query and receive a full `EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON)`
result. The plan is rendered in a structured, human-readable tree view in the UI.

### Why
PGAM had no EXPLAIN integration. Query tuning required copy-pasting from the
pg_stat_statements list into a separate psql session. This closes that loop.

### Design Decisions
- **On-demand only in M8_01.** Auto-capture (background, threshold-triggered) is M8_02.
- **EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON)** — user explicitly opts in to execution cost.
- **EXPLAIN without ANALYZE** also supported via a toggle (for read-only queries or when
  the user doesn't want to execute).
- **Database selector required** — the plan runs against a specific database on the instance.
  The UI provides a dropdown populated from the instance's database list.
- **Role restriction:** DBA and Admin only. This is a write-side operation — it executes
  arbitrary SQL on the monitored instance.
- **Statement timeout:** 30 seconds hard limit on the EXPLAIN query. No exceptions.
- **No plan persistence in M8_01** — results returned in the API response only, not stored.
  Plan history (storage + retrieval) is M8_02.
- **application_name:** `pgpulse_explain` on the explain connection.

### API
```
POST /api/v1/instances/:id/explain
```
Request body:
```json
{
  "database": "mydb",
  "query": "SELECT * FROM orders WHERE user_id = 42",
  "analyze": true,
  "buffers": true
}
```
Response:
```json
{
  "plan_json": [...],    // raw EXPLAIN FORMAT JSON output
  "execution_time_ms": 12.4,
  "planning_time_ms": 0.8
}
```
Errors: 400 (bad query/db), 403 (wrong role), 408 (timeout), 500 (PG error).

### UI
- New "Query Plan" tab or section on ServerDetail page (or accessible via
  pg_stat_statements row → "Explain" button which pre-fills the query).
- Input area: database selector dropdown + multi-line SQL textarea + `[Analyze]` toggle +
  `[Run EXPLAIN]` button.
- Output: collapsible JSON tree showing plan nodes. Each node shows:
  Node Type, Relation Name, Startup Cost, Total Cost, Plan Rows, Actual Rows
  (if analyze=true), Actual Total Time, Shared Hit Blocks, Shared Read Blocks.
- Nodes with high cost or high actual vs. estimated row count discrepancy highlighted
  in yellow/red.
- Raw JSON toggle for power users.

---

## Feature 3: Cross-Instance Settings Diff

### What
Compare `pg_settings` between any two monitored instances. Returns three categories:
- **Changed** — setting exists in both, values differ.
- **Only in A** — setting present in instance A but not B (e.g., extension-specific GUCs).
- **Only in B** — setting present in instance B but not A.
- **Matching** — identical (shown collapsed by default).

### Why
The most common use case: "Why does this query run fast on staging but slow on prod?"
Often the answer is a settings difference (work_mem, enable_hashjoin, etc.).

### Design Decisions
- **Stateless in M8_01** — both instances queried live at diff time, nothing stored.
  Temporal diff (current vs. historical snapshot) is M8_02.
- **Filter noise** — exclude volatile/irrelevant settings from the diff by default:
  `server_version`, `data_directory`, `hba_file`, `ident_file`, `config_file`,
  `log_directory`, `unix_socket_directories`, `lc_*` locale settings.
  User can toggle "Show all settings" to see everything.
- **Category grouping** — group diff results by `pg_settings.category` for readability.
- **No role restriction** — read-only, available to all authenticated users including viewer.

### API
```
GET /api/v1/settings/compare?instance_a=:id&instance_b=:id
```
Response:
```json
{
  "instance_a": { "id": "...", "name": "prod-primary" },
  "instance_b": { "id": "...", "name": "staging" },
  "changed": [
    { "name": "work_mem", "category": "Resource Usage / Memory",
      "value_a": "4MB", "value_b": "64MB" }
  ],
  "only_in_a": [...],
  "only_in_b": [...],
  "matching_count": 243
}
```

### UI
- New "Settings Diff" page accessible from the navigation (Administration section or
  top-level menu).
- Two instance selectors (dropdown of all monitored instances).
- `[Compare]` button triggers the API call.
- Results rendered in grouped accordion by category.
- Changed settings highlighted — value_a and value_b shown side-by-side.
- Export as CSV button.

---

## What Is NOT in M8_01

| Feature | Milestone |
|---------|-----------|
| Auto-capture query plans (background, threshold-triggered) | M8_02 |
| Plan history (store + retrieve past plans) | M8_02 |
| Temporal settings diff (current vs. historical snapshot) | M8_02 |
| pg_stat_activity kill-by-filter (bulk kill) | M8_03 or later |
| EXPLAIN ANALYZE for auto-identified slow queries | M8_02 |
| Logical replication monitoring (Q41) | M8_02 or later |

---

## Success Criteria

- [ ] `POST /api/v1/instances/:id/sessions/:pid/cancel` and `/terminate` work correctly
- [ ] Session kill correctly rejected for `app_admin` and `viewer` roles (403)
- [ ] Every kill attempt (success and failure) written to `session_audit_log`
- [ ] `POST /api/v1/instances/:id/explain` returns valid plan JSON within 30s timeout
- [ ] EXPLAIN correctly rejected for `app_admin` and `viewer` (403)
- [ ] `GET /api/v1/settings/compare` returns accurate diff across two live instances
- [ ] Settings compare accessible to `viewer` role (200)
- [ ] UI: kill buttons visible on activity list, confirmation modals work
- [ ] UI: Query Plan input → output renders plan tree
- [ ] UI: Settings Diff page renders grouped accordion with changed/only_in_a/only_in_b
- [ ] `go build`, `go vet`, `go test ./cmd/... ./internal/...`, `golangci-lint` all clean
- [ ] `npm run build`, `npm run lint`, `npm run typecheck` all clean
