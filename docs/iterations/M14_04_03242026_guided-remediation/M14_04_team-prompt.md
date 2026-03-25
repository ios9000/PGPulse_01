# M14_04 — Team Prompt: Guided Remediation Playbooks

**Iteration:** M14_04
**Date:** 2026-03-24
**Model:** `claude --model claude-opus-4-6`
**Agents:** 2 (Backend + Frontend)

---

## Pre-Read (MANDATORY)

Before ANY code changes, each agent MUST read:

```
docs/CODEBASE_DIGEST.md
docs/iterations/M14_04_*/design.md
docs/iterations/M14_04_*/requirements.md
docs/iterations/M14_04_*/corrections.md
docs/iterations/M14_04_*/ADR-M14_04-Guided-Remediation-Playbooks.md
CLAUDE.md
```

---

## DO NOT RE-DISCUSS

All decisions D400–D609 are locked. Key points agents must not relitigate:

- Four-tier safety: diagnostic / remediate / dangerous / external
- Playbook Resolver with 5-level priority ranking (hook > root_cause > metric > adviser_rule > manual)
- Core 10 seed pack — exactly 10 playbooks, no more, no fewer for initial release
- Bounded conditional branching — no loops, no nested workflows
- Static declarative interpretation with `scope: "first"/"any"/"all"` — no expression language, no Go evaluators
- Database-stored playbooks — not YAML files, not hardcoded Go
- PlaybookRun persisted with resume — not in-memory
- `BEGIN` + `SET LOCAL` + `ROLLBACK` transaction pattern for executor — NOT session-level SET
- Approval queue deferred to future iteration
- 2 agents

---

## CRITICAL SECURITY CORRECTIONS (from design review)

These corrections override the corresponding sections in `design.md`. Agents MUST implement these versions, not the original design.

### Correction 1: Transaction-Scoped Execution (MANDATORY)

The executor MUST NOT use session-level `SET` commands. All SQL execution MUST occur inside a transaction block using `SET LOCAL`. This prevents connection pool pollution.

```go
// CORRECT pattern — use this:
tx, err := conn.Begin(ctx)
if err != nil { return nil, err }
defer tx.Rollback(ctx) // Always rollback — clean connection guaranteed

if step.SafetyTier == "diagnostic" {
    _, err = tx.Exec(ctx, "SET LOCAL default_transaction_read_only = ON")
    if err != nil { return nil, err }
}
_, err = tx.Exec(ctx, fmt.Sprintf("SET LOCAL statement_timeout = '%ds'", timeout))
if err != nil { return nil, err }
_, err = tx.Exec(ctx, "SET LOCAL lock_timeout = '5s'")
if err != nil { return nil, err }

rows, err := tx.Query(ctx, step.SQLTemplate)
// ... collect results ...
// tx.Rollback() called by defer — connection returned clean to pool

// WRONG pattern — DO NOT use this:
// conn.Exec(ctx, "SET default_transaction_read_only = ON")
// defer conn.Exec(ctx, "SET default_transaction_read_only = OFF")
```

**Why:** If the defer fails (panic, network drop, timeout), a "dirty" connection with read_only=ON and a 5s timeout is returned to pgxpool. Any collector or adviser that subsequently acquires this connection will fail.

### Correction 2: Multi-Statement Injection Guard (MANDATORY)

Before executing any SQL, reject templates containing multiple statements:

```go
// Reject multi-statement SQL (prevents SET LOCAL ... = OFF; DROP TABLE injection)
trimmed := strings.TrimSpace(step.SQLTemplate)
trimmed = strings.TrimRight(trimmed, ";") // Allow single trailing semicolon
if strings.Contains(trimmed, ";") {
    return nil, fmt.Errorf("multi-statement SQL is forbidden in playbook steps")
}
```

### Correction 3: Interpreter Scope Field (MANDATORY)

The `InterpretationRule` struct MUST include a `scope` field:

```go
type InterpretationRule struct {
    Column   string      `json:"column"`
    Operator string      `json:"operator"`
    Value    interface{} `json:"value"`
    Verdict  string      `json:"verdict"`
    Message  string      `json:"message"`
    Scope    string      `json:"scope"` // "first" (default), "any", "all"
}
```

Interpretation logic:
- `"first"` (default): evaluate against the first row only
- `"any"`: fire if ANY row matches the condition
- `"all"`: fire only if ALL rows match

Seed playbooks MUST use aggregated SQL queries (returning exactly 1 row) with `scope: "first"`. The `"any"/"all"` scopes exist for custom playbooks that return multi-row results.

### Correction 4: Feedback Worker (MANDATORY)

Add `internal/playbook/feedback_worker.go`. This background goroutine:
- Runs every 60 seconds
- Scans `playbook_runs WHERE status = 'completed' AND feedback_resolved IS NULL AND completed_at > NOW() - interval`
- For each run with a `trigger_id` pointing to an alert event, checks `AlertHistoryStore` whether that alert has resolved
- If resolved: sets `feedback_resolved = true`
- If run was abandoned: sets `feedback_resolved = false`

Wire in `main.go` alongside the existing background workers.

---

## Agent 1: BACKEND AGENT

### Ownership

All Go code: playbook package, migration, API handlers, main.go wiring, tests.

### Phase 1 — Foundation (parallel with Agent 2 Phase 1)

#### Task 1A: Migration 018

**Create:** `internal/storage/migrations/018_playbooks.sql`

Four tables: `playbooks`, `playbook_steps`, `playbook_runs`, `playbook_run_steps`. Exact DDL in design doc Section 2.1. Include:
- GIN index on `trigger_bindings`
- Partial index on `playbook_runs(status)` for in-progress runs
- UNIQUE constraints on `(playbook_id, step_order)` and `(run_id, step_order)`

#### Task 1B: Types + Store Interface

**Create:**
- `internal/playbook/types.go` — all structs from design doc Section 3.2, including the corrected `InterpretationRule` with `Scope` field
- `internal/playbook/store.go` — `PlaybookStore` interface from design doc Section 3.6

#### Task 1C: PGStore Implementation

**Create:**
- `internal/playbook/pgstore.go` — PostgreSQL implementation of `PlaybookStore`

Key queries:
- `FindByTriggerBinding`: uses `trigger_bindings @> $1::jsonb` with GIN index
- `Create/Update`: JSONB marshaling for `trigger_bindings`, `result_interpretation`, `branch_rules`
- `Get`: loads playbook + all steps in a single query (JOIN or two queries)
- `CreateRun`: version-pins `playbook_version` from the playbook at creation time
- `SeedBuiltins`: `INSERT ON CONFLICT (slug) DO NOTHING` — only seeds if playbook doesn't exist

**Create:** `internal/playbook/nullstore.go` — no-op for live mode

#### Task 1D: Executor (with security corrections)

**Create:** `internal/playbook/executor.go`

**CRITICAL:** Follow the transaction-scoped pattern from Correction 1. The executor:
1. Gets a connection from `InstanceConnProvider`
2. Opens a transaction: `conn.Begin(ctx)`
3. Sets `SET LOCAL default_transaction_read_only = ON` for Tier 1
4. Sets `SET LOCAL statement_timeout` and `SET LOCAL lock_timeout`
5. Checks for multi-statement injection (Correction 2)
6. Executes the query within the transaction
7. Collects results (cap at `ResultRowLimit`, default 100)
8. Defers `tx.Rollback()` — connection always returned clean
9. For Tier 4 (external): returns manual instructions, no SQL execution

```go
type Executor struct {
    connProv     InstanceConnProvider
    rowLimit     int
    logger       *slog.Logger
}

func (e *Executor) ExecuteStep(ctx context.Context, instanceID string, step Step) (*ExecutionResult, error)
```

**Create:** `internal/playbook/executor_test.go` — tests:
- `TestExecuteStep_Tier1_ReadOnly` — verify write operations are rejected
- `TestExecuteStep_Tier1_MultiStatement` — verify semicolon guard blocks injection
- `TestExecuteStep_Timeout` — verify statement_timeout kills long queries
- `TestExecuteStep_RowLimit` — verify result capped at configured limit
- `TestExecuteStep_Tier4_NoExecution` — verify external steps never execute SQL
- `TestExecuteStep_ConnectionFailure` — verify graceful error on unreachable instance

#### Task 1E: Interpreter (with scope correction)

**Create:** `internal/playbook/interpreter.go`

Implement the `Interpret()` function from design doc Section 3.3, with the `Scope` field from Correction 3:

```go
func Interpret(spec InterpretationSpec, columns []string, rows [][]interface{}, rowCount int) (string, string)
```

Logic:
1. Check `RowCountRules` first (these always apply to total row count)
2. For column rules with `scope: "first"`: evaluate against `rows[0]`
3. For column rules with `scope: "any"`: iterate all rows, fire on first match
4. For column rules with `scope: "all"`: iterate all rows, fire only if every row matches
5. Fall back to `DefaultVerdict` / `DefaultMessage`

Template expansion: `{{column_name}}` → value from result, `{{row_count}}` → total count.

**Create:** `internal/playbook/interpreter_test.go` — tests:
- `TestInterpret_RedRule` — matching condition returns red verdict
- `TestInterpret_GreenRule` — non-matching falls through to green
- `TestInterpret_RowCountRule` — fires on excessive row count
- `TestInterpret_DefaultVerdict` — no rules match returns default
- `TestInterpret_ScopeAny` — fires when any row matches
- `TestInterpret_ScopeAll` — fires only when all rows match
- `TestInterpret_TemplateExpansion` — `{{column_name}}` replaced correctly
- `TestInterpret_EmptyResult` — handles zero rows gracefully

### Phase 2 — Resolver + Seeds + API

#### Task 2A: Resolver

**Create:** `internal/playbook/resolver.go`

5-level priority from design doc Section 3.5. Returns `(*Playbook, matchReason string, error)`.

**Create:** `internal/playbook/resolver_test.go` — tests:
- `TestResolve_HookMatch` — highest priority wins
- `TestResolve_RootCauseMatch` — when no hook, root cause matches
- `TestResolve_MetricMatch` — fallback to metric key
- `TestResolve_AdviserMatch` — fallback to adviser rule
- `TestResolve_NoMatch` — returns nil
- `TestResolve_OnlyStable` — draft/deprecated playbooks excluded
- `TestResolve_HighestVersion` — when multiple match at same level, highest version wins

#### Task 2B: Seed Playbooks (Core 10)

**Create:**
- `internal/playbook/seed.go` — `SeedPlaybooks()` function returning `[]Playbook`
- `internal/playbook/seed_wal.go` — WAL Archive Failure (5 steps, design doc Section 4 is the reference)
- `internal/playbook/seed_replication.go` — Replication Lag (4 steps)
- `internal/playbook/seed_connections.go` — Connection Saturation (5 steps)
- `internal/playbook/seed_locks.go` — Lock Contention (5 steps)
- `internal/playbook/seed_longtx.go` — Long Transactions (4 steps)
- `internal/playbook/seed_checkpoint.go` — Checkpoint Storm (4 steps)
- `internal/playbook/seed_disk.go` — Disk Full Emergency (5 steps)
- `internal/playbook/seed_vacuum.go` — Autovacuum Health Check (5 steps)
- `internal/playbook/seed_wraparound.go` — Wraparound Risk (4 steps)
- `internal/playbook/seed_query.go` — Heavy Query Diagnostics (4 steps)

**Authoring rules for ALL seed playbooks:**
1. ALL diagnostic SQL MUST return aggregated data (1 row). Use `count(*)`, `max()`, `sum()`, NOT raw `SELECT *`.
2. ALL steps use `scope: "first"` (default) because SQL is aggregated.
3. ALL Tier 1 steps: `safety_tier: "diagnostic"`, `timeout_seconds: 5-10`.
4. Branch rules: use result column values or computed verdict, NOT arbitrary expressions.
5. Every playbook MUST have at least one Tier 4 (external) escalation step as a fallback.
6. `trigger_bindings` must reference actual ontology Hook constants from `internal/rca/ontology.go`.

**Before writing seed playbooks:** Grep `internal/rca/ontology.go` to get exact Hook constant values for trigger_bindings. Also grep `internal/remediation/hooks.go` for the hookToRuleID map to populate adviser_rules bindings.

#### Task 2C: Feedback Worker

**Create:** `internal/playbook/feedback_worker.go`

```go
type FeedbackWorker struct {
    store      PlaybookStore
    alertStore AlertHistoryStore
    interval   time.Duration
    window     time.Duration
    logger     *slog.Logger
}

func (w *FeedbackWorker) Start(ctx context.Context)
func (w *FeedbackWorker) runCycle(ctx context.Context)
```

Logic:
1. Query `playbook_runs WHERE status = 'completed' AND feedback_resolved IS NULL AND completed_at > NOW() - window`
2. For each run with `trigger_source = 'alert'` and `trigger_id` set:
   - Parse `trigger_id` as alert event ID
   - Query `AlertHistoryStore` for that event's current status
   - If resolved: `UPDATE playbook_runs SET feedback_resolved = true`
3. For abandoned runs: `UPDATE playbook_runs SET feedback_resolved = false WHERE status = 'abandoned' AND feedback_resolved IS NULL`

#### Task 2D: API Handlers

**Create:** `internal/api/playbooks.go`

All handlers from design doc Section 5:

**CRUD:**
- `handleListPlaybooks` — GET /playbooks, filters: status, category, search
- `handleGetPlaybook` — GET /playbooks/{id}, includes steps
- `handleCreatePlaybook` — POST /playbooks (alert_management)
- `handleUpdatePlaybook` — PUT /playbooks/{id} (alert_management) — bumps version, resets to draft
- `handleDeletePlaybook` — DELETE /playbooks/{id} (user_management) — blocks builtins
- `handlePromotePlaybook` — POST /playbooks/{id}/promote (user_management)
- `handleDeprecatePlaybook` — POST /playbooks/{id}/deprecate (alert_management)
- `handleResolvePlaybook` — GET /playbooks/resolve?hook=...&root_cause=...&metric=...&adviser_rule=...

**Execution:**
- `handleStartRun` — POST /instances/{id}/playbooks/{playbookId}/run — creates run, version-pins
- `handleGetRun` — GET /playbook-runs/{runId} — returns full run with step results
- `handleExecuteStep` — POST /playbook-runs/{runId}/steps/{stepOrder}/execute — tier-aware execution
- `handleConfirmStep` — POST /.../{stepOrder}/confirm (instance_management)
- `handleApproveStep` — POST /.../{stepOrder}/approve (instance_management)
- `handleSkipStep` — POST /.../{stepOrder}/skip
- `handleAbandonRun` — POST /playbook-runs/{runId}/abandon
- `handleSubmitFeedback` — POST /playbook-runs/{runId}/feedback
- `handleListAllRuns` — GET /playbook-runs
- `handleListInstanceRuns` — GET /instances/{id}/playbook-runs

**Execute step handler logic (design doc Section 6):**
1. Load run + step definition
2. Check tier permissions
3. For Tier 1: auto-execute via Executor
4. For Tier 2 without confirmation: return `awaiting_confirmation` + SQL preview
5. For Tier 3 without approval: return `awaiting_approval` + message
6. For Tier 4: return `manual_action` + instructions
7. After execution: interpret results, resolve branch, save run step, update run state
8. Respond with step result + next step + run status

**Create:** `internal/api/playbooks_test.go` — basic handler tests with mock store

#### Task 2E: Wiring

**Modify:** `internal/api/server.go` — register all playbook routes
**Modify:** `internal/config/config.go` — add `PlaybookConfig` struct
**Modify:** `internal/config/load.go` — defaults for PlaybookConfig
**Modify:** `cmd/pgpulse-server/main.go`:
- Create playbook store (PGPlaybookStore or NullPlaybookStore based on storage mode)
- Create executor with InstanceConnProvider
- Create resolver with store
- Seed builtins on startup
- Start feedback worker
- Pass store/executor/resolver to API server

### Build Verification

```bash
cd web && npm run build && npm run lint && npm run typecheck && cd .. && go build ./cmd/pgpulse-server && go test ./cmd/... ./internal/... -count=1 && golangci-lint run ./cmd/... ./internal/...
```

---

## Agent 2: FRONTEND AGENT

### Ownership

All TypeScript/React: types, hooks, pages, components, integration points.

### Phase 1 — Types + Component Skeletons

#### Task 1F: TypeScript Types

**Create:** `web/src/types/playbook.ts`

```typescript
export type SafetyTier = 'diagnostic' | 'remediate' | 'dangerous' | 'external';
export type PlaybookStatus = 'draft' | 'stable' | 'deprecated';
export type RunStatus = 'in_progress' | 'completed' | 'abandoned' | 'escalated';
export type StepStatus = 'pending' | 'running' | 'completed' | 'skipped' | 'failed' | 'awaiting_confirmation' | 'awaiting_approval';
export type Verdict = 'green' | 'yellow' | 'red';

export interface Playbook { ... }         // All fields from Go struct
export interface PlaybookStep { ... }     // Including result_interpretation, branch_rules
export interface PlaybookRun { ... }      // Including steps array
export interface PlaybookRunStep { ... }  // Including result_json, verdict
export interface ResolverResult {
  playbook: Playbook | null;
  match_reason: string;
  match_value: string;
}
```

#### Task 1G: React Query Hooks

**Create:** `web/src/hooks/usePlaybooks.ts`

```typescript
// Playbook CRUD
usePlaybooks(filters)           // GET /playbooks
usePlaybook(id)                 // GET /playbooks/{id}
useCreatePlaybook()             // POST /playbooks
useUpdatePlaybook()             // PUT /playbooks/{id}
useDeletePlaybook()             // DELETE /playbooks/{id}
usePromotePlaybook()            // POST /playbooks/{id}/promote
useDeprecatePlaybook()          // POST /playbooks/{id}/deprecate

// Resolver
useResolvePlaybook(params)      // GET /playbooks/resolve?...

// Runs
useStartRun()                   // POST /instances/{id}/playbooks/{playbookId}/run
usePlaybookRun(runId)           // GET /playbook-runs/{runId}
useExecuteStep()                // POST /playbook-runs/{runId}/steps/{stepOrder}/execute
useConfirmStep()                // POST .../confirm
useApproveStep()                // POST .../approve
useSkipStep()                   // POST .../skip
useAbandonRun()                 // POST /playbook-runs/{runId}/abandon
useSubmitFeedback()             // POST /playbook-runs/{runId}/feedback
usePlaybookRuns(filters)        // GET /playbook-runs
useInstancePlaybookRuns(id)     // GET /instances/{id}/playbook-runs
```

#### Task 1H: Reusable Components

**Create all in `web/src/components/playbook/`:**

- `TierBadge.tsx` — color-coded badge: diagnostic=green, remediate=amber, dangerous=red, external=gray. Grep `PriorityBadge.tsx` or `ConfidenceBadge.tsx` for pattern.
- `VerdictBadge.tsx` — green/yellow/red result badge with message text
- `ResultTable.tsx` — formatted query result table from `{columns, rows}` JSON. Handles truncation notice ("Showing 100 of 523 rows").
- `RunProgressBar.tsx` — horizontal step indicator: completed (filled circles) → current (pulse) → future (empty). Shows step count and branch path.
- `BranchIndicator.tsx` — small text showing "→ Jumped to Step 3: Failures detected" when branching occurs.

### Phase 2 — Pages

#### Task 2F: Playbook Catalog Page

**Create:** `web/src/pages/PlaybookCatalog.tsx` at route `/playbooks`

- Grid of `PlaybookCard` components
- Filters: status dropdown (All/Stable/Draft/Deprecated), category dropdown, text search
- Each card shows: name, category badge, step count, tier badges for steps, estimated duration
- Click navigates to detail page
- "Create Playbook" button (visible for alert_management+ permission)

**Create:** `web/src/components/playbook/PlaybookCard.tsx`
**Create:** `web/src/components/playbook/PlaybookFilters.tsx`

#### Task 2G: Playbook Detail Page

**Create:** `web/src/pages/PlaybookDetail.tsx` at route `/playbooks/{id}`

- Header: name, description, status badge, version, author, category
- Steps list: numbered steps with tier badges, SQL preview (collapsed), interpretation rules summary
- Trigger bindings display: hooks, metrics, root causes, adviser rules
- "Run on Instance" dropdown → starts run on selected instance
- "Edit" button (alert_management+)
- "Promote to Stable" button (user_management+, only for draft status)
- Run history for this playbook

#### Task 2H: Playbook Wizard Page (KEY DELIVERABLE)

**Create:** `web/src/pages/PlaybookWizard.tsx` at route `/instances/{id}/playbook-runs/{runId}`

This is the core L1 operator experience. Structure:

```
┌─ Header: Playbook name, instance, started by, elapsed time ─────┐
│                                                                   │
│  [RunProgressBar: ●──●──○──○──○ Step 2 of 5]                    │
│                                                                   │
│  ┌─ Completed Steps (collapsed, expandable) ───────────────────┐ │
│  │ ✓ Step 1: Check archive status  🔴 847 failures detected    │ │
│  └──────────────────────────────────────────────────────────────┘ │
│                                                                   │
│  ┌─ Current Step ──────────────────────────────────────────────┐ │
│  │ Step 2: Check WAL accumulation                              │ │
│  │ 🟢 Diagnostic — executes safely                             │ │
│  │                                                             │ │
│  │ Count WAL files in pg_wal directory...                      │ │
│  │                                                             │ │
│  │ [▶ Run Diagnostic]                                          │ │
│  │                                                             │ │
│  │ [Show SQL ▼]  ← expandable, collapsed by default            │ │
│  │                                                             │ │
│  │ ── After execution: ──                                      │ │
│  │ [ResultTable with columns + rows]                           │ │
│  │ [VerdictBadge: 🔴 523 WAL files — archiving behind]        │ │
│  │ [BranchIndicator: → Next: Step 3 (check config)]           │ │
│  │                                                             │ │
│  │ [Next Step]  [Skip]                                         │ │
│  └─────────────────────────────────────────────────────────────┘ │
│                                                                   │
│  ┌─ Future Steps (grayed out) ─────────────────────────────────┐ │
│  │ Step 3: Check archive_command config                        │ │
│  │ Step 4: Emergency escalation          📋 Manual             │ │
│  │ Step 5: Verify recovery                                     │ │
│  └──────────────────────────────────────────────────────────────┘ │
│                                                                   │
│  [Abandon Run]                                                    │
└───────────────────────────────────────────────────────────────────┘
```

**Step execution flow per tier:**
- **Tier 1:** Click "▶ Run Diagnostic" → loading spinner → result table + verdict appears → "Next Step" button enabled
- **Tier 2:** Click "⚠️ Execute" → confirmation modal (shows SQL, target instance, tier warning) → confirm → execute → results
- **Tier 3:** Shows "🔒 Requires DBA Approval" → if user has `instance_management` permission, shows approve button; otherwise shows "Contact DBA" message
- **Tier 4:** Shows manual instructions block with escalation contact. "Mark as Done" button to proceed.

**Resume:** On page load, call `GET /playbook-runs/{runId}` → populate all completed steps with their stored results → position on `current_step_order`.

**Create:** `web/src/components/playbook/StepCard.tsx` — renders one step in all states (pending, current, completed, skipped)

#### Task 2I: Playbook Editor Page

**Create:** `web/src/pages/PlaybookEditor.tsx` at route `/playbooks/{id}/edit`

- Form fields: name, slug, description, category, estimated_duration, requires_permission
- Step builder: sortable list of steps, each with:
  - Name, description fields
  - SQL editor (use a `<textarea>` with monospace font — full CodeMirror deferred)
  - Safety tier dropdown
  - Timeout field
  - Result interpretation builder: add rules (column, operator, value, verdict, message, scope)
  - Branch rules builder: add conditions (column/verdict → goto step N)
  - Manual instructions (for Tier 4)
  - Escalation contact
- Add Step / Remove Step buttons
- Trigger bindings editor: multi-input for hooks, root_causes, metrics, adviser_rules
- Save (creates as draft) / Preview (shows how wizard would look)

**Create:** `web/src/components/playbook/StepBuilder.tsx`

#### Task 2J: Run History + Feedback

**Create:** `web/src/pages/PlaybookRunHistory.tsx` at route `/playbook-runs`

- Table of all runs: playbook name, instance, started by, status, duration, verdict summary
- Filters: status, instance, playbook

**Create:** `web/src/components/playbook/FeedbackModal.tsx`

- Shown after run completes (status changes to "completed")
- "Was this playbook helpful?" Yes/No
- "Did it resolve the issue?" Yes/No/Partially
- Optional notes textarea
- Submit calls `POST /playbook-runs/{runId}/feedback`

### Phase 3 — Integration Points

#### Task 2K: Sidebar + Routing

**Modify:** `web/src/components/layout/Sidebar.tsx` — add "Playbooks" nav item between Adviser and RCA Incidents at fleet level
**Modify:** App router — add routes for PlaybookCatalog, PlaybookDetail, PlaybookEditor, PlaybookWizard, PlaybookRunHistory

#### Task 2L: Alert → Playbook

**Modify:** `web/src/components/alerts/AlertDetailPanel.tsx`

Below the existing ROOT CAUSE ANALYSIS section (if present), add:

```typescript
const { data: resolved } = useResolvePlaybook({
  metric: alert.metric,
  instance_id: alert.instance_id
});

// If match found: render "▶ Run Playbook: {name}" button
// On click: POST start run, navigate to wizard
```

#### Task 2M: RCA → Playbook

**Modify:** `web/src/pages/RCAIncidentDetail.tsx`

Below the Recommended Actions section, add a "Guided Remediation" card:

```typescript
const { data: resolved } = useResolvePlaybook({
  hook: incident.primary_chain?.remediation_hook,
  root_cause: incident.primary_chain?.root_cause_key,
  instance_id: incident.instance_id
});

// Render: playbook name, estimated duration, "Start Guided Remediation" button
```

#### Task 2N: Adviser → Playbook

**Modify:** `web/src/components/advisor/AdvisorRow.tsx`

Add a "▶ Remediate" button:

```typescript
const { data: resolved } = useResolvePlaybook({
  adviser_rule: recommendation.rule_id,
  instance_id: recommendation.instance_id
});
// Only show button if resolved.playbook is non-null
```

### Build Verification

```bash
cd web && npm run build && npm run lint && npm run typecheck
```

---

## Coordination Notes

1. **Agent 1 creates API routes before Agent 2 needs them.** Agent 2 can build pages/components using the known API shapes from the design doc, then integrate once the backend is live.

2. **Seed playbooks are the largest single work item.** 10 playbooks × 4-5 steps each = ~45 step definitions with SQL, interpretation rules, and branch logic. Agent 1 should use `seed_wal.go` from the design doc as the reference pattern and replicate.

3. **The wizard page (Task 2H) is the highest-value deliverable.** If time is constrained, ship the wizard + catalog + seeds. The editor can be simplified (JSON textarea instead of visual builder) without losing core value.

4. **Tier 4 steps have no SQL** — they display instructions only. The frontend must handle `sql_template: null` gracefully.

5. **Resume flow:** The wizard page loads the full run state on mount. If steps 1-2 are completed, render them collapsed with stored results. Position on `current_step_order`. This works because `playbook_run_steps` stores everything.

---

## Expected New Files (Watch List)

### Backend (~25 files)
```
internal/playbook/types.go
internal/playbook/store.go
internal/playbook/pgstore.go
internal/playbook/nullstore.go
internal/playbook/executor.go
internal/playbook/executor_test.go
internal/playbook/resolver.go
internal/playbook/resolver_test.go
internal/playbook/interpreter.go
internal/playbook/interpreter_test.go
internal/playbook/feedback_worker.go
internal/playbook/seed.go
internal/playbook/seed_wal.go
internal/playbook/seed_replication.go
internal/playbook/seed_connections.go
internal/playbook/seed_locks.go
internal/playbook/seed_longtx.go
internal/playbook/seed_checkpoint.go
internal/playbook/seed_disk.go
internal/playbook/seed_vacuum.go
internal/playbook/seed_wraparound.go
internal/playbook/seed_query.go
internal/storage/migrations/018_playbooks.sql
internal/api/playbooks.go
internal/api/playbooks_test.go
```

### Frontend (~23 files)
```
web/src/types/playbook.ts
web/src/hooks/usePlaybooks.ts
web/src/pages/PlaybookCatalog.tsx
web/src/pages/PlaybookDetail.tsx
web/src/pages/PlaybookEditor.tsx
web/src/pages/PlaybookWizard.tsx
web/src/pages/PlaybookRunHistory.tsx
web/src/components/playbook/PlaybookCard.tsx
web/src/components/playbook/PlaybookFilters.tsx
web/src/components/playbook/StepBuilder.tsx
web/src/components/playbook/StepCard.tsx
web/src/components/playbook/TierBadge.tsx
web/src/components/playbook/ResultTable.tsx
web/src/components/playbook/VerdictBadge.tsx
web/src/components/playbook/BranchIndicator.tsx
web/src/components/playbook/RunProgressBar.tsx
web/src/components/playbook/FeedbackModal.tsx
web/src/components/playbook/ResolverButton.tsx
```

## Expected Modified Files

```
internal/config/config.go
internal/config/load.go
internal/api/server.go
cmd/pgpulse-server/main.go
web/src/components/layout/Sidebar.tsx
web/src/components/alerts/AlertDetailPanel.tsx
web/src/pages/RCAIncidentDetail.tsx
web/src/components/advisor/AdvisorRow.tsx
App router (wherever routes are registered)
```
