# M14_04 — Requirements: Guided Remediation Playbooks

**Iteration:** M14_04
**Date:** 2026-03-24
**Predecessor:** M14_03 (Expansion, Calibration, Knowledge Integration — complete)
**Locked Decisions:** D600–D609
**Reference:** ADR-M14_04-Guided-Remediation-Playbooks.md

---

## 1. Objective

Add a fourth operational layer to PGPulse: **Guided Remediation Playbooks** — interactive, database-stored, step-by-step diagnostic and remediation scenarios that PGPulse executes safely through its existing connection pool. This transforms PGPulse from a tool that tells you "what happened" (RCA) into one that tells you "what to do now" and does it for you.

**Target users:** L1/L2 operators, on-call duty shifts, application engineers — anyone who can follow a guided workflow but lacks the DBA expertise to improvise from raw `pg_` views.

---

## 2. Decision Summary

| ID | Decision |
|----|----------|
| D600 | M14_04 under M14 (RCA family) |
| D601 | Database-stored playbooks, editable from UI |
| D602 | Four-tier execution safety model |
| D603 | Playbook steps with SQL templates, safety tiers, result interpretation, branching |
| D604 | Playbook Resolver with 5-level priority ranking |
| D605 | Core 10 seed pack |
| D606 | Bounded conditional branching, no loops/nesting |
| D607 | Static declarative rules for result interpretation |
| D608 | 2 agents (Backend + Frontend) |
| D609 | PlaybookRun persisted with resume; approval queue deferred |

---

## 3. Scope

### 3.1 In Scope

| # | Work Item | Description |
|---|-----------|-------------|
| W1 | Playbook schema + migration | `playbooks`, `playbook_steps`, `playbook_runs`, `playbook_run_steps` tables |
| W2 | Playbook execution engine | Go package `internal/playbook/` — SQL execution with tier enforcement, timeouts, READ ONLY, LIMIT |
| W3 | Playbook Resolver | Selects best playbook for a given context (RCA hook, root cause key, alert metric, adviser class) |
| W4 | Playbook CRUD API | Create, read, update, delete, list playbooks; version management; draft/stable lifecycle |
| W5 | Playbook execution API | Start run, execute step, get run status, resume run |
| W6 | Seed migration | 10 built-in playbooks covering top operational scenarios |
| W7 | Playbook catalog UI | Browse, search, filter playbooks; create/edit form with step builder |
| W8 | Guided Remediation wizard UI | Step-by-step execution with inline results, tier badges, branching, resume |
| W9 | Integration: Alert → Playbook | Alert detail panel shows "Run Playbook" when Resolver finds a match |
| W10 | Integration: RCA → Playbook | Incident detail page shows recommended playbook from chain's remediation hook |
| W11 | Integration: Adviser → Playbook | Recommendation row shows "Open Guided Remediation" when playbook is available |
| W12 | Feedback instrumentation | Implicit signals (alert auto-resolve → success, escalation click → escalation) + explicit rating |

### 3.2 Out of Scope (Deferred per ADR Section 6)

- Parameterized inputs (interactive PID injection into step SQL)
- Dry-run / pre-flight EXPLAIN validation
- GitOps playbook sync (YAML export/import)
- A/B testing of playbooks
- Full approval queue workflow with notifications and delegation
- Loops and nested workflows in branching

---

## 4. Detailed Requirements

### W1 — Playbook Schema

**R1.1: `playbooks` table**

| Column | Type | Description |
|--------|------|-------------|
| `id` | BIGSERIAL PK | |
| `slug` | TEXT UNIQUE NOT NULL | URL-safe identifier (e.g., `wal-archive-failure`) |
| `name` | TEXT NOT NULL | Human-readable name |
| `description` | TEXT | Detailed description of what this playbook diagnoses/fixes |
| `version` | INT NOT NULL DEFAULT 1 | Monotonically increasing version number |
| `status` | TEXT NOT NULL DEFAULT 'draft' | `draft`, `stable`, `deprecated` |
| `category` | TEXT NOT NULL DEFAULT 'general' | `replication`, `storage`, `connections`, `locks`, `vacuum`, `performance`, `configuration`, `general` |
| `trigger_bindings` | JSONB NOT NULL DEFAULT '{}' | Resolver bindings: `{"hooks": ["remediation.checkpoint_tuning"], "root_causes": ["root_cause.checkpoint_storm"], "metrics": ["pg.checkpoint.write_time_ms"], "adviser_rules": ["rem_checkpoint_warn"]}` |
| `estimated_duration_min` | INT | Estimated time to complete in minutes |
| `requires_permission` | TEXT NOT NULL DEFAULT 'view_all' | Minimum RBAC permission to start this playbook |
| `author` | TEXT | Creator username |
| `is_builtin` | BOOL NOT NULL DEFAULT false | True for seed playbooks (prevents deletion, allows reset) |
| `created_at` | TIMESTAMPTZ NOT NULL DEFAULT NOW() | |
| `updated_at` | TIMESTAMPTZ NOT NULL DEFAULT NOW() | |

**R1.2: `playbook_steps` table**

| Column | Type | Description |
|--------|------|-------------|
| `id` | BIGSERIAL PK | |
| `playbook_id` | BIGINT FK → playbooks(id) ON DELETE CASCADE | |
| `step_order` | INT NOT NULL | Execution order (1-based) |
| `name` | TEXT NOT NULL | Step title (e.g., "Check archive status") |
| `description` | TEXT | What this step does and why |
| `sql_template` | TEXT | SQL to execute. NULL for Tier 4 (manual) steps |
| `safety_tier` | TEXT NOT NULL | `diagnostic`, `remediate`, `dangerous`, `external` |
| `timeout_seconds` | INT NOT NULL DEFAULT 5 | Per-step statement_timeout |
| `result_interpretation` | JSONB NOT NULL DEFAULT '{}' | Declarative rules for coloring results (see R1.5) |
| `branch_rules` | JSONB NOT NULL DEFAULT '[]' | Conditional next-step logic (see R1.6) |
| `next_step_default` | INT | Default next step_order if no branch matches (NULL = playbook complete) |
| `manual_instructions` | TEXT | For Tier 4 steps: human-readable instructions |
| `escalation_contact` | TEXT | Who to call if this step fails or requires escalation |
| `created_at` | TIMESTAMPTZ NOT NULL DEFAULT NOW() | |

**R1.3: `playbook_runs` table**

| Column | Type | Description |
|--------|------|-------------|
| `id` | BIGSERIAL PK | |
| `playbook_id` | BIGINT FK → playbooks(id) | |
| `playbook_version` | INT NOT NULL | Version-pinned at run start |
| `instance_id` | TEXT NOT NULL | Target instance |
| `started_by` | TEXT NOT NULL | Username who started |
| `status` | TEXT NOT NULL DEFAULT 'in_progress' | `in_progress`, `completed`, `abandoned`, `escalated` |
| `current_step_order` | INT NOT NULL DEFAULT 1 | Where the operator is |
| `trigger_source` | TEXT | `alert`, `rca`, `adviser`, `manual` |
| `trigger_id` | TEXT | Alert event ID, incident ID, or recommendation ID |
| `started_at` | TIMESTAMPTZ NOT NULL DEFAULT NOW() | |
| `updated_at` | TIMESTAMPTZ NOT NULL DEFAULT NOW() | |
| `completed_at` | TIMESTAMPTZ | |
| `feedback_useful` | BOOL | Explicit: was this playbook useful? |
| `feedback_resolved` | BOOL | Explicit: did it resolve the issue? |
| `feedback_notes` | TEXT | Optional operator notes |

**R1.4: `playbook_run_steps` table**

| Column | Type | Description |
|--------|------|-------------|
| `id` | BIGSERIAL PK | |
| `run_id` | BIGINT FK → playbook_runs(id) ON DELETE CASCADE | |
| `step_order` | INT NOT NULL | Which step was executed |
| `status` | TEXT NOT NULL | `pending`, `running`, `completed`, `skipped`, `failed`, `awaiting_confirmation` |
| `sql_executed` | TEXT | Actual SQL that was sent (for audit) |
| `result_json` | JSONB | Query result as `{columns: [...], rows: [...]}` |
| `result_verdict` | TEXT | `green`, `yellow`, `red` — computed from interpretation rules |
| `result_message` | TEXT | Human-readable interpretation |
| `error` | TEXT | Error message if step failed |
| `executed_at` | TIMESTAMPTZ | |
| `duration_ms` | INT | Execution time |
| `confirmed_by` | TEXT | For Tier 2: who confirmed; for Tier 3: who approved |

**R1.5: Result interpretation format (JSONB)**

```json
{
  "rules": [
    {
      "column": "failed_count",
      "operator": ">",
      "value": 0,
      "verdict": "red",
      "message": "Archive failures detected: {{failed_count}} failures since last reset"
    },
    {
      "column": "failed_count",
      "operator": "==",
      "value": 0,
      "verdict": "green",
      "message": "No archive failures — archiving is healthy"
    }
  ],
  "row_count_rules": [
    {
      "operator": ">",
      "value": 100,
      "verdict": "red",
      "message": "Excessive WAL file accumulation ({{row_count}} files)"
    }
  ],
  "default_verdict": "yellow",
  "default_message": "Results require manual review"
}
```

Supported operators: `>`, `<`, `>=`, `<=`, `==`, `!=`, `is_null`, `is_not_null`.
Template variables: `{{column_name}}` substituted from result row, `{{row_count}}` for total rows.

**R1.6: Branch rules format (JSONB array)**

```json
[
  {
    "condition": {"column": "failed_count", "operator": ">", "value": 0},
    "goto_step": 3,
    "reason": "Failures detected — check WAL accumulation"
  },
  {
    "condition": {"verdict": "green"},
    "goto_step": 5,
    "reason": "Archiving healthy — skip to replication check"
  }
]
```

Branch conditions can reference column values from the current step's result or the computed verdict.

### W2 — Execution Engine

**R2.1:** The execution engine MUST wrap all Tier 1 (diagnostic) queries in `SET TRANSACTION READ ONLY` at the session level. This is a security invariant — no exceptions.

**R2.2:** The execution engine MUST inject `SET statement_timeout = '<timeout_seconds>s'` and `SET lock_timeout = '5s'` before each query execution.

**R2.3:** The execution engine MUST append `LIMIT 100` to any query result before returning to the API. The raw result set is never sent unbounded to the frontend.

**R2.4:** The execution engine uses the existing `InstanceConnProvider` to get connections to monitored instances. No new connection pools.

**R2.5:** All executed SQL is logged to `playbook_run_steps.sql_executed` for audit.

**R2.6:** Tier enforcement:
- **Tier 1 (diagnostic):** Auto-execute. READ ONLY. No user interaction required.
- **Tier 2 (remediate):** Requires explicit UI confirmation. Confirmation recorded in `confirmed_by`.
- **Tier 3 (dangerous):** Requires `instance_management` RBAC permission. If current user lacks it, step enters `awaiting_confirmation` state and displays "Requires DBA approval."
- **Tier 4 (external):** No SQL execution. Displays `manual_instructions` and `escalation_contact`.

**R2.7:** The execution engine MUST handle connection failures gracefully — if the target instance is unreachable, the step fails with a clear message rather than hanging.

### W3 — Playbook Resolver

**R3.1:** The Resolver accepts a context object and returns the single best-matching playbook:

```go
type ResolverContext struct {
    HookID       string // From RCA chain RemediationHook
    RootCauseKey string // From RCA incident primary_chain.root_cause_key
    MetricKey    string // From alert trigger metric
    AdviserRule  string // From recommendation rule_id
    InstanceID   string // Target instance
}
```

**R3.2:** Priority ranking (highest wins):
1. Explicit hook match (`trigger_bindings.hooks` contains `HookID`)
2. Root cause key match (`trigger_bindings.root_causes` contains `RootCauseKey`)
3. Metric key match (`trigger_bindings.metrics` contains `MetricKey`)
4. Adviser rule match (`trigger_bindings.adviser_rules` contains `AdviserRule`)
5. Manual catalog fallback (no automatic selection)

**R3.3:** Only `stable` status playbooks are returned by the Resolver. `draft` and `deprecated` playbooks are excluded.

**R3.4:** If multiple playbooks match at the same priority level, return the one with the highest version number.

### W4 — Playbook CRUD API

| Method | Path | Permission | Description |
|--------|------|------------|-------------|
| GET | /api/v1/playbooks | viewer+ | List all playbooks (with filters: status, category) |
| GET | /api/v1/playbooks/{id} | viewer+ | Get playbook with all steps |
| POST | /api/v1/playbooks | alert_management | Create new playbook (starts as draft) |
| PUT | /api/v1/playbooks/{id} | alert_management | Update playbook (bumps version, resets to draft) |
| DELETE | /api/v1/playbooks/{id} | user_management | Delete playbook (blocked for builtins) |
| POST | /api/v1/playbooks/{id}/promote | user_management | Promote draft → stable (SuperAdmin only) |
| POST | /api/v1/playbooks/{id}/deprecate | alert_management | Mark as deprecated |
| GET | /api/v1/playbooks/resolve | viewer+ | Resolver endpoint: accepts context params, returns best match |

**R4.1:** Any edit to a playbook's SQL steps MUST reset status to `draft` and increment version.

**R4.2:** Built-in playbooks (`is_builtin = true`) cannot be deleted but can be deprecated or overridden (create a new playbook with the same trigger bindings, higher version).

### W5 — Playbook Execution API

| Method | Path | Permission | Description |
|--------|------|------------|-------------|
| POST | /api/v1/instances/{id}/playbooks/{playbookId}/run | viewer+ | Start a new run |
| GET | /api/v1/playbook-runs/{runId} | viewer+ | Get run status with all step results |
| POST | /api/v1/playbook-runs/{runId}/steps/{stepOrder}/execute | viewer+ (Tier 1), instance_management (Tier 2/3) | Execute a step |
| POST | /api/v1/playbook-runs/{runId}/steps/{stepOrder}/confirm | instance_management | Confirm Tier 2 step |
| POST | /api/v1/playbook-runs/{runId}/steps/{stepOrder}/approve | instance_management | Approve Tier 3 step |
| POST | /api/v1/playbook-runs/{runId}/steps/{stepOrder}/skip | viewer+ | Skip a step |
| POST | /api/v1/playbook-runs/{runId}/abandon | viewer+ | Abandon run |
| POST | /api/v1/playbook-runs/{runId}/feedback | viewer+ | Submit feedback |
| GET | /api/v1/instances/{id}/playbook-runs | viewer+ | List runs for an instance |
| GET | /api/v1/playbook-runs | viewer+ | List all runs (fleet-wide) |

**R5.1:** Starting a run version-pins the playbook — the run stores `playbook_version` and uses the step definitions from that version even if the playbook is later edited.

**R5.2:** The execute endpoint for Tier 1 steps auto-runs and returns results immediately. For Tier 2, it returns `awaiting_confirmation`. For Tier 3, it returns `awaiting_approval`. For Tier 4, it returns the manual instructions.

**R5.3:** A run can be resumed after tab close — the GET run endpoint returns the full state including which steps have been executed, their results, and the current step.

### W6 — Seed Playbooks (Core 10)

| # | Slug | Name | Trigger Hook | Steps | Key Diagnostics |
|---|------|------|-------------|-------|-----------------|
| 1 | `wal-archive-failure` | WAL Archive Failure | HookWALConfig | 5 | pg_stat_archiver, pg_ls_waldir(), archive_command check |
| 2 | `replication-lag` | Replication Lag Investigation | HookReplicationLag | 4 | pg_stat_replication, pg_replication_slots, WAL accumulation |
| 3 | `connection-saturation` | Connection Saturation | HookConnectionPooling | 5 | pg_stat_activity by state, idle-in-transaction check, pool config |
| 4 | `lock-contention` | Lock Contention Analysis | HookLockTimeout | 5 | pg_locks blocking tree, long-held locks, candidate pg_terminate |
| 5 | `long-transactions` | Long Transaction Investigation | HookLongTransaction | 4 | pg_stat_activity, vacuum impact check, bloat accumulation |
| 6 | `checkpoint-storm` | Checkpoint Storm Diagnosis | HookCheckpointTuning | 4 | pg_stat_bgwriter/checkpointer, WAL generation rate, config check |
| 7 | `disk-full` | Disk Space Emergency | HookDiskCapacity | 5 | pg_tablespace_size, table bloat, WAL accumulation, temp files |
| 8 | `autovacuum-failing` | Autovacuum Health Check | HookVacuumTuning | 5 | pg_stat_user_tables vacuum stats, dead tuple accumulation, worker status |
| 9 | `wraparound-risk` | Transaction Wraparound Risk | HookWraparound | 4 | datfrozenxid age, aggressive vacuum check, per-table wraparound |
| 10 | `heavy-query` | Heavy Query Diagnostics | HookQueryOptimization | 4 | pg_stat_statements top-N, EXPLAIN candidates, index suggestions |

Each seed playbook includes full step definitions with SQL, interpretation rules, branch conditions, and escalation contacts.

### W7 — Playbook Catalog UI

**R7.1:** New page at `/playbooks` — grid/list of all playbooks with filters (status, category), search by name.

**R7.2:** Playbook detail page at `/playbooks/{id}` — shows name, description, steps overview, trigger bindings, version history.

**R7.3:** Playbook editor (alert_management+ permission) — form to create/edit playbook with:
- Step builder: add/remove/reorder steps
- SQL editor per step with syntax highlighting
- Safety tier selector per step
- Result interpretation rule builder (visual, not raw JSON)
- Branch rule builder
- Test execution: run a single step against a selected instance to verify SQL

**R7.4:** Sidebar: "Playbooks" nav item at fleet level (between Adviser and RCA Incidents).

### W8 — Guided Remediation Wizard UI

**R8.1:** Full-page wizard at `/instances/{id}/playbook-runs/{runId}` showing:
- Playbook name and description at top
- Step progress indicator (step N of M, with branch visualization)
- Current step: name, description, safety tier badge
- "▶ Run Diagnostic" button for Tier 1 (auto-executes on click)
- "⚠️ Execute with Confirmation" for Tier 2 (shows SQL preview modal)
- "🔒 Requires DBA Approval" for Tier 3 (shows approval request state)
- "📋 Manual Action" for Tier 4 (shows instructions + escalation contact)
- Result table: formatted query results with column headers
- Result verdict: green/yellow/red badge with interpreted message
- "Next Step" button (follows branch logic or default)
- "Skip Step" button
- "Abandon" button

**R8.2:** Resume capability: if operator returns to a run URL, the wizard picks up from the last executed step.

**R8.3:** SQL preview toggle: "Show query" expandable section on each step for operators who want to learn.

**R8.4:** Timer: show elapsed time per step and total run duration.

### W9 — Alert → Playbook Integration

**R9.1:** In `AlertDetailPanel.tsx`, add a "▶ Run Playbook" button. When clicked, call the Resolver with `{MetricKey: alert.metric, InstanceID: alert.instance_id}`.

**R9.2:** If Resolver returns a match, navigate to the run wizard. If no match, show "No playbook available for this alert type."

### W10 — RCA → Playbook Integration

**R10.1:** In `RCAIncidentDetail.tsx`, below the Recommended Actions section, add a "Guided Remediation" section. Call the Resolver with `{HookID: chain.remediation_hook, RootCauseKey: chain.root_cause_key, InstanceID: incident.instance_id}`.

**R10.2:** If Resolver returns a match, show the playbook name, estimated duration, and a "Start Guided Remediation" button.

### W11 — Adviser → Playbook Integration

**R11.1:** In `AdvisorRow.tsx`, add a "▶ Remediate" button for recommendations that have a matching playbook. Call the Resolver with `{AdviserRule: recommendation.rule_id, InstanceID: recommendation.instance_id}`.

### W12 — Feedback Instrumentation

**R12.1:** Implicit signal: if a `playbook_run` completes and the triggering alert auto-resolves within 5 minutes, set `feedback_resolved = true` automatically.

**R12.2:** Implicit signal: if the operator clicks "Abandon" or "Escalate to DBA" during a run, record the step where escalation occurred.

**R12.3:** Explicit: after run completion, show a brief modal: "Was this playbook helpful?" (Yes/No) + "Did it resolve the issue?" (Yes/No/Partially) + optional notes.

---

## 5. Acceptance Criteria

| # | Criterion | Validates |
|---|-----------|-----------|
| AC1 | 10 seed playbooks visible in `/playbooks` catalog after fresh deployment | W1, W6 |
| AC2 | Starting the "WAL Archive Failure" playbook on an instance auto-executes Step 1 (pg_stat_archiver) and renders results inline | W2, W5, W8 |
| AC3 | Tier 1 step cannot execute `DROP TABLE` — READ ONLY enforcement blocks it | W2 |
| AC4 | Tier 2 step shows confirmation modal with exact SQL and target instance before execution | W2, W8 |
| AC5 | Tier 3 step shows "Requires DBA Approval" for viewer role, executes for instance_management role | W2 |
| AC6 | Closing browser tab and returning to run URL resumes at the correct step with prior results visible | W5, W8, D609 |
| AC7 | Alert detail panel shows "Run Playbook" for connection utilization alert → navigates to connection-saturation playbook | W3, W9 |
| AC8 | RCA incident with HookCheckpointTuning → Resolver returns checkpoint-storm playbook | W3, W10 |
| AC9 | Adviser recommendation with "Consider connection pooling" → shows "Remediate" button | W3, W11 |
| AC10 | Playbook editor: creating a new playbook, adding 3 steps, promoting to stable works end-to-end | W4, W7 |
| AC11 | Editing a stable playbook's SQL resets status to draft (requires re-promotion) | W4 |
| AC12 | Branch logic: if step 1 result is red, wizard navigates to step 3 (skipping step 2) | W6, W8 |
| AC13 | Step execution respects statement_timeout — long query is cancelled after configured seconds | W2 |
| AC14 | After playbook run completes and alert resolves within 5 min, feedback_resolved is set automatically | W12 |
| AC15 | Full build verification passes | All |

---

## 6. Out of Scope

- Parameterized inputs (PID injection) — deferred per ADR
- Dry-run / EXPLAIN pre-flight — deferred per ADR
- GitOps playbook sync — deferred per ADR
- A/B testing — deferred per ADR
- Full approval queue with notifications/delegation — deferred per D609
- Loops and nested workflows — forbidden per D606
- Autonomous feedback-driven calibration — forbidden per ADR
