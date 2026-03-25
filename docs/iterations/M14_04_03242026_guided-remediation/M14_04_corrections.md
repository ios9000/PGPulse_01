# M14_04 — Corrections

**Iteration:** M14_04
**Date:** 2026-03-24
**Sources:** Design review (2026-03-24) + Final architectural review (2026-03-24)

---

## Summary

| # | Correction | Severity | Source | Impact |
|---|-----------|----------|--------|--------|
| C1 | Transaction-scoped execution | 🔴 Critical | Design review | executor.go rewrite |
| C2 | Multi-statement injection guard | 🟡 Medium | Design review | executor.go, 5 lines |
| C3 | Interpreter scope — MVP-only `"first"` | 🟡 Medium | Final review override | interpreter.go simplified; `any`/`all` deferred |
| C4 | Feedback worker missing | 🟡 Medium | Design review | New file: feedback_worker.go |
| C5 | Concurrency guard for step execution | 🔴 Critical | Final review | API handler atomic lock |
| C6 | Explicit error state machine | 🔴 Critical | Final review | Handler error path + "Retry" UI |
| C7 | Lightweight approval flow | 🟡 Medium | Final review (scoped per D609) | `pending_approval` status, DBA approves in-run |
| C8 | Config-bound row limit | 🟢 Low | Final review | executor.go reads from config |
| C9 | array_agg ordering in FindByTriggerBinding | 🟢 Low | Final review | pgstore.go SQL fix |

---

## C1: Transaction-Scoped Execution (MANDATORY)

**Overrides:** design.md Section 3.4 (Executor)

The executor MUST NOT use session-level `SET` commands. All SQL execution MUST occur inside a transaction block using `SET LOCAL`.

```go
// CORRECT:
tx, err := conn.Begin(ctx)
if err != nil { return nil, err }
defer tx.Rollback(ctx)

if step.SafetyTier == "diagnostic" {
    tx.Exec(ctx, "SET LOCAL default_transaction_read_only = ON")
}
tx.Exec(ctx, fmt.Sprintf("SET LOCAL statement_timeout = '%ds'", timeout))
tx.Exec(ctx, "SET LOCAL lock_timeout = '5s'")
rows, err := tx.Query(ctx, step.SQLTemplate)
// ... collect results ...
// tx.Rollback() via defer — connection always clean

// FORBIDDEN:
// conn.Exec(ctx, "SET default_transaction_read_only = ON")
// defer conn.Exec(ctx, "SET default_transaction_read_only = OFF")
```

**Why:** A panic, network drop, or timeout on the `defer` leaves a poisoned connection in the pool. Subsequent collectors or adviser queries inherit the dirty session state.

---

## C2: Multi-Statement Injection Guard (MANDATORY)

**Overrides:** design.md Section 3.4 (Executor)

Before executing any SQL, reject templates with multiple statements:

```go
trimmed := strings.TrimSpace(step.SQLTemplate)
trimmed = strings.TrimRight(trimmed, ";")
if strings.Contains(trimmed, ";") {
    return nil, fmt.Errorf("multi-statement SQL is forbidden in playbook steps")
}
```

**Why:** Even within a READ ONLY transaction, `SET LOCAL default_transaction_read_only = OFF; DROP TABLE` could bypass the sandbox if pgx allows multi-statement execution.

---

## C3: Interpreter Scope — MVP Simplification

**Overrides:** Team prompt Correction 3 (interpreter scope)

For M14_04 MVP, the interpreter implements `scope: "first"` ONLY. The `scope` field remains in the `InterpretationRule` struct (forward compatibility) but `"any"` and `"all"` evaluation is NOT implemented.

**Rationale:** The mandatory Single-Row Authoring Rule (all seed SQL returns exactly 1 aggregated row) makes `any`/`all` unnecessary. Implementing multi-row evaluation without rigorous testing introduces false-verdict risks. Deferred to backlog.

**Agent action:**
- Keep `Scope string` field in `InterpretationRule` struct
- `Interpret()` function always evaluates against `rows[0]` only
- If `Scope` is set to `"any"` or `"all"`, log a warning and fall back to `"first"`
- Document: "any/all scope deferred to future iteration"

---

## C4: Feedback Worker (MANDATORY)

**Adds to:** design.md (missing from original)

Create `internal/playbook/feedback_worker.go`:
- Runs every 60s
- Scans completed runs with NULL `feedback_resolved` within the implicit feedback window
- Checks alert resolution via `AlertHistoryStore`
- Sets `feedback_resolved = true` for auto-resolved alerts
- Sets `feedback_resolved = false` for abandoned runs

Wire in `main.go` alongside existing background workers.

---

## C5: Concurrency Guard for Step Execution (MANDATORY — NEW)

**Overrides:** design.md Section 6 (Execute Step Handler)

Before calling `executor.ExecuteStep()`, the handler MUST atomically lock the run to prevent double-click and client retry race conditions:

```go
// In handleExecuteStep, BEFORE any execution:
locked, err := store.LockStepForExecution(ctx, runID, stepOrder)
if err != nil || !locked {
    respondJSON(w, 409, map[string]string{
        "error": "Step is already being executed or has completed",
    })
    return
}
```

**Store method:**

```go
// Add to PlaybookStore interface:
LockStepForExecution(ctx context.Context, runID int64, stepOrder int) (bool, error)
```

**SQL:**

```sql
UPDATE playbook_run_steps
SET status = 'running'
WHERE run_id = $1
  AND step_order = $2
  AND status IN ('pending', 'awaiting_confirmation', 'awaiting_approval')
RETURNING id;
```

If zero rows returned → step is already running or completed → reject the request with 409 Conflict.

**Also add to NullStore:**
```go
func (n *NullPlaybookStore) LockStepForExecution(ctx context.Context, runID int64, stepOrder int) (bool, error) {
    return true, nil
}
```

---

## C6: Explicit Error State Machine (MANDATORY — NEW)

**Overrides:** design.md Section 6 (Execute Step Handler)

When `executor.ExecuteStep()` returns an error, the handler MUST NOT silently proceed. The contract:

```go
result, err := executor.ExecuteStep(ctx, run.InstanceID, step)
if err != nil {
    // 1. Mark step as failed
    runStep := &RunStep{
        RunID:     run.ID,
        StepOrder: step.StepOrder,
        Status:    "failed",
        Error:     err.Error(),
        ExecutedAt: timePtr(time.Now()),
    }
    store.UpdateRunStep(ctx, runStep)

    // 2. Do NOT advance current_step_order
    // 3. Do NOT change run.Status (remains in_progress — operator can retry)
    // 4. Respond with error + retry hint
    respondJSON(w, 200, map[string]interface{}{
        "step_result": runStep,
        "next_step":   step.StepOrder, // Stay on same step
        "run_status":  "in_progress",
        "can_retry":   true,
    })
    return
}
```

**Frontend (Wizard):**
- When a step has `status: "failed"`, show the error message in a red banner
- Show a "Retry Step" button that re-calls the execute endpoint
- The concurrency guard (C5) allows retry because the step status reverts to a retryable state

**Retry mechanism:**
```sql
-- Reset step status to allow retry
UPDATE playbook_run_steps
SET status = 'pending', error = NULL
WHERE run_id = $1 AND step_order = $2 AND status = 'failed';
```

Add `RetryStep` to the store interface and API (`POST /playbook-runs/{runId}/steps/{stepOrder}/retry`).

---

## C7: Lightweight Approval Flow (MANDATORY — NEW, scoped per D609)

**Overrides:** design.md Section 6 (Tier 3 handling)

The current design conflates "you lack permission" with "this needs approval." The corrected flow:

**When L1 operator reaches a Tier 3 step:**
1. Frontend shows "🔒 Request DBA Approval" button (NOT "awaiting_approval" error)
2. Clicking it calls `POST /playbook-runs/{runId}/steps/{stepOrder}/request-approval`
3. Backend sets `RunStep.status = 'pending_approval'`
4. Response tells the operator: "Approval requested. A DBA can approve this step from the run URL."

**When DBA navigates to the same run URL:**
1. Wizard shows the pending step with full context (SQL preview, why it's dangerous, what prior steps found)
2. DBA clicks "Approve and Execute"
3. Backend calls `POST /playbook-runs/{runId}/steps/{stepOrder}/approve`
4. Handler checks `instance_management` permission, executes the step, records `confirmed_by`

**What is NOT built (deferred per D609):**
- No notification to DBAs that approval is pending
- No global "Pending Approvals" queue page
- No delegation or timeout
- No automatic escalation

**New API endpoint:**
```
POST /api/v1/playbook-runs/{runId}/steps/{stepOrder}/request-approval  (viewer+)
```

**New step status value:** `pending_approval` (added to the StepStatus enum)

---

## C8: Config-Bound Row Limit (MANDATORY — NEW)

**Overrides:** design.md Section 3.4 (Executor, result collection loop)

Replace hardcoded `100` with config value:

```go
// BEFORE:
if totalRows <= 100 { ... }

// AFTER:
if totalRows <= e.rowLimit { ... }
// where e.rowLimit is set from PlaybookConfig.ResultRowLimit in the constructor
```

The executor constructor accepts the limit:

```go
func NewExecutor(connProv InstanceConnProvider, cfg PlaybookConfig, logger *slog.Logger) *Executor {
    limit := cfg.ResultRowLimit
    if limit == 0 { limit = 100 } // Sensible default
    return &Executor{connProv: connProv, rowLimit: limit, logger: logger}
}
```

---

## C9: array_agg Ordering in FindByTriggerBinding (MANDATORY — NEW)

**Overrides:** design.md Section 3.7 (FindByTriggerBinding SQL)

The aggregation must include explicit ordering to guarantee deterministic step order:

```sql
-- BEFORE (non-deterministic):
SELECT p.*, array_agg(ps.*) as steps FROM playbooks p ...

-- AFTER (deterministic):
SELECT p.*, array_agg(ps.* ORDER BY ps.step_order) as steps FROM playbooks p ...
```

Without `ORDER BY ps.step_order`, PostgreSQL may return steps in insertion order, which is typically correct but not guaranteed. The `ORDER BY` makes it contractual.

---

## Single-Row Authoring Rule (Mandatory for ALL Seed Playbooks)

This is not a code correction but a **mandatory authoring constraint** enforced during seed playbook creation:

> **Every Tier 1 (diagnostic) SQL query in a seed playbook MUST return exactly 1 row of aggregated data.**

Examples:

```sql
-- ✅ CORRECT: aggregated, single row
SELECT count(*) AS wal_count, pg_size_pretty(sum(size)) AS total FROM pg_ls_waldir();
SELECT max(extract(epoch FROM now()-xact_start)) AS max_age, count(*) AS count FROM pg_stat_activity WHERE state='active';

-- ❌ FORBIDDEN: raw multi-row result
SELECT pid, state, query FROM pg_stat_activity;
SELECT * FROM pg_replication_slots;
```

This rule eliminates the interpreter blind spot (evaluating only row 0) without requiring `any`/`all` scope implementation in the MVP.

---

## Updated Store Interface (incorporating C5, C6, C7)

Three new methods on `PlaybookStore`:

```go
type PlaybookStore interface {
    // ... existing methods from design.md ...

    // C5: Concurrency guard
    LockStepForExecution(ctx context.Context, runID int64, stepOrder int) (bool, error)

    // C6: Retry failed steps
    ResetStepForRetry(ctx context.Context, runID int64, stepOrder int) error

    // C7: Request approval
    RequestStepApproval(ctx context.Context, runID int64, stepOrder int) error
}
```

And one new API endpoint:

```
POST /api/v1/playbook-runs/{runId}/steps/{stepOrder}/request-approval  (viewer+)
POST /api/v1/playbook-runs/{runId}/steps/{stepOrder}/retry             (viewer+)
```

These MUST be added to the route registration in `server.go` and to `NullPlaybookStore`.

---

## Pre-Flight Grep Findings

Pre-flight greps deferred to your local execution (checklist Step 5). The corrections above are based on architectural review. Grep findings may add additional corrections — append them to this document.
