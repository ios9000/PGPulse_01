# M14_04 — Design: Guided Remediation Playbooks

**Iteration:** M14_04
**Date:** 2026-03-24
**Companion:** M14_04_requirements.md, ADR-M14_04-Guided-Remediation-Playbooks.md
**Locked Decisions:** D600–D609

---

## 1. Architecture Overview

M14_04 introduces a new Go package `internal/playbook/` and extends the API, frontend, and migration layers:

```
NEW:  internal/playbook/          — execution engine, resolver, store, types
NEW:  migrations/018_playbooks.sql — schema + seed data
MOD:  internal/api/               — playbook CRUD + execution + resolver endpoints
MOD:  internal/config/            — PlaybookConfig
MOD:  cmd/pgpulse-server/main.go  — wire playbook subsystem
NEW:  web/src/pages/Playbook*.tsx — catalog, detail, editor, wizard pages
NEW:  web/src/components/playbook/ — step builder, result table, tier badges
NEW:  web/src/hooks/usePlaybooks.ts — React Query hooks
MOD:  web/src/pages/RCAIncidentDetail.tsx — playbook integration
MOD:  web/src/components/alerts/AlertDetailPanel.tsx — playbook integration
MOD:  web/src/components/advisor/AdvisorRow.tsx — playbook integration
MOD:  web/src/components/layout/Sidebar.tsx — playbooks nav item
```

Data flow:

```
Alert fires / RCA chain fires / Adviser recommendation appears
            │
            ▼
    Playbook Resolver (5-level priority)
            │
            ▼
    Best-matching Playbook (stable, version-pinned)
            │
            ▼
    PlaybookRun created (persisted in DB)
            │
            ▼
    ┌─── Step-by-Step Execution ───┐
    │                               │
    │  Tier 1: Auto-run diagnostic  │──→ SET TRANSACTION READ ONLY → Execute → Render results
    │  Tier 2: Confirm + execute    │──→ Show SQL preview → Confirm → Execute → Render
    │  Tier 3: DBA approval         │──→ Check RBAC → Approve → Execute → Render
    │  Tier 4: Manual instructions  │──→ Show instructions + escalation contact
    │                               │
    │  Result interpretation:       │
    │    rules → green/yellow/red   │
    │    branch logic → next step   │
    │                               │
    └───────────────────────────────┘
            │
            ▼
    Run completes → Feedback (implicit + explicit)
```

---

## 2. Database Schema

### 2.1 Migration 018

```sql
-- M14_04: Guided Remediation Playbooks

-- Playbook definitions
CREATE TABLE IF NOT EXISTS playbooks (
    id                    BIGSERIAL PRIMARY KEY,
    slug                  TEXT UNIQUE NOT NULL,
    name                  TEXT NOT NULL,
    description           TEXT NOT NULL DEFAULT '',
    version               INT NOT NULL DEFAULT 1,
    status                TEXT NOT NULL DEFAULT 'draft',
    category              TEXT NOT NULL DEFAULT 'general',
    trigger_bindings      JSONB NOT NULL DEFAULT '{}',
    estimated_duration_min INT,
    requires_permission   TEXT NOT NULL DEFAULT 'view_all',
    author                TEXT NOT NULL DEFAULT '',
    is_builtin            BOOL NOT NULL DEFAULT false,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_playbooks_status ON playbooks(status);
CREATE INDEX IF NOT EXISTS idx_playbooks_category ON playbooks(category);
CREATE INDEX IF NOT EXISTS idx_playbooks_trigger_bindings ON playbooks USING GIN(trigger_bindings);

-- Playbook steps
CREATE TABLE IF NOT EXISTS playbook_steps (
    id                    BIGSERIAL PRIMARY KEY,
    playbook_id           BIGINT NOT NULL REFERENCES playbooks(id) ON DELETE CASCADE,
    step_order            INT NOT NULL,
    name                  TEXT NOT NULL,
    description           TEXT NOT NULL DEFAULT '',
    sql_template          TEXT,
    safety_tier           TEXT NOT NULL DEFAULT 'diagnostic',
    timeout_seconds       INT NOT NULL DEFAULT 5,
    result_interpretation JSONB NOT NULL DEFAULT '{}',
    branch_rules          JSONB NOT NULL DEFAULT '[]',
    next_step_default     INT,
    manual_instructions   TEXT,
    escalation_contact    TEXT,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(playbook_id, step_order)
);

-- Playbook run state (persisted for resume)
CREATE TABLE IF NOT EXISTS playbook_runs (
    id                  BIGSERIAL PRIMARY KEY,
    playbook_id         BIGINT NOT NULL REFERENCES playbooks(id),
    playbook_version    INT NOT NULL,
    instance_id         TEXT NOT NULL,
    started_by          TEXT NOT NULL,
    status              TEXT NOT NULL DEFAULT 'in_progress',
    current_step_order  INT NOT NULL DEFAULT 1,
    trigger_source      TEXT,
    trigger_id          TEXT,
    started_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at        TIMESTAMPTZ,
    feedback_useful     BOOL,
    feedback_resolved   BOOL,
    feedback_notes      TEXT
);

CREATE INDEX IF NOT EXISTS idx_playbook_runs_instance ON playbook_runs(instance_id);
CREATE INDEX IF NOT EXISTS idx_playbook_runs_status ON playbook_runs(status) WHERE status = 'in_progress';
CREATE INDEX IF NOT EXISTS idx_playbook_runs_playbook ON playbook_runs(playbook_id);

-- Individual step execution records
CREATE TABLE IF NOT EXISTS playbook_run_steps (
    id              BIGSERIAL PRIMARY KEY,
    run_id          BIGINT NOT NULL REFERENCES playbook_runs(id) ON DELETE CASCADE,
    step_order      INT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'pending',
    sql_executed    TEXT,
    result_json     JSONB,
    result_verdict  TEXT,
    result_message  TEXT,
    error           TEXT,
    executed_at     TIMESTAMPTZ,
    duration_ms     INT,
    confirmed_by    TEXT,
    UNIQUE(run_id, step_order)
);
```

### 2.2 GIN Index on trigger_bindings

The Resolver queries `trigger_bindings` using JSONB containment:

```sql
-- Find playbooks matching a specific hook
SELECT * FROM playbooks
WHERE status = 'stable'
  AND trigger_bindings @> '{"hooks": ["remediation.checkpoint_tuning"]}'
ORDER BY version DESC LIMIT 1;
```

The GIN index makes this efficient.

---

## 3. Go Package: `internal/playbook/`

### 3.1 Package Structure

```
internal/playbook/
  types.go          — Playbook, Step, Run, RunStep structs
  store.go          — PlaybookStore interface
  pgstore.go        — PostgreSQL implementation
  nullstore.go      — No-op for live mode
  executor.go       — Execution engine (SQL execution with tier enforcement)
  resolver.go       — Playbook Resolver (5-level priority)
  interpreter.go    — Result interpretation (declarative rule evaluation)
  seed.go           — Seed playbook definitions (Core 10)
  seed_wal.go       — WAL archive failure playbook steps
  seed_replication.go — Replication lag playbook steps
  seed_connections.go — Connection saturation playbook steps
  seed_locks.go     — Lock contention playbook steps
  seed_longtx.go    — Long transaction playbook steps
  seed_checkpoint.go — Checkpoint storm playbook steps
  seed_disk.go      — Disk full playbook steps
  seed_vacuum.go    — Autovacuum playbook steps
  seed_wraparound.go — Wraparound risk playbook steps
  seed_query.go     — Heavy query playbook steps
```

### 3.2 Core Types

```go
// types.go

type Playbook struct {
    ID                  int64           `json:"id"`
    Slug                string          `json:"slug"`
    Name                string          `json:"name"`
    Description         string          `json:"description"`
    Version             int             `json:"version"`
    Status              string          `json:"status"`
    Category            string          `json:"category"`
    TriggerBindings     TriggerBindings `json:"trigger_bindings"`
    EstimatedDurationMin *int           `json:"estimated_duration_min,omitempty"`
    RequiresPermission  string          `json:"requires_permission"`
    Author              string          `json:"author"`
    IsBuiltin           bool            `json:"is_builtin"`
    Steps               []Step          `json:"steps,omitempty"`
    CreatedAt           time.Time       `json:"created_at"`
    UpdatedAt           time.Time       `json:"updated_at"`
}

type TriggerBindings struct {
    Hooks        []string `json:"hooks,omitempty"`
    RootCauses   []string `json:"root_causes,omitempty"`
    Metrics      []string `json:"metrics,omitempty"`
    AdviserRules []string `json:"adviser_rules,omitempty"`
}

type Step struct {
    ID                   int64              `json:"id"`
    PlaybookID           int64              `json:"playbook_id"`
    StepOrder            int                `json:"step_order"`
    Name                 string             `json:"name"`
    Description          string             `json:"description"`
    SQLTemplate          string             `json:"sql_template,omitempty"`
    SafetyTier           string             `json:"safety_tier"`
    TimeoutSeconds       int                `json:"timeout_seconds"`
    ResultInterpretation InterpretationSpec `json:"result_interpretation"`
    BranchRules          []BranchRule       `json:"branch_rules"`
    NextStepDefault      *int               `json:"next_step_default,omitempty"`
    ManualInstructions   string             `json:"manual_instructions,omitempty"`
    EscalationContact    string             `json:"escalation_contact,omitempty"`
}

type SafetyTier string

const (
    TierDiagnostic SafetyTier = "diagnostic"
    TierRemediate  SafetyTier = "remediate"
    TierDangerous  SafetyTier = "dangerous"
    TierExternal   SafetyTier = "external"
)

type Run struct {
    ID               int64      `json:"id"`
    PlaybookID       int64      `json:"playbook_id"`
    PlaybookVersion  int        `json:"playbook_version"`
    PlaybookName     string     `json:"playbook_name,omitempty"`
    InstanceID       string     `json:"instance_id"`
    StartedBy        string     `json:"started_by"`
    Status           string     `json:"status"`
    CurrentStepOrder int        `json:"current_step_order"`
    TriggerSource    string     `json:"trigger_source,omitempty"`
    TriggerID        string     `json:"trigger_id,omitempty"`
    Steps            []RunStep  `json:"steps,omitempty"`
    StartedAt        time.Time  `json:"started_at"`
    UpdatedAt        time.Time  `json:"updated_at"`
    CompletedAt      *time.Time `json:"completed_at,omitempty"`
    FeedbackUseful   *bool      `json:"feedback_useful,omitempty"`
    FeedbackResolved *bool      `json:"feedback_resolved,omitempty"`
    FeedbackNotes    string     `json:"feedback_notes,omitempty"`
}

type RunStep struct {
    ID            int64           `json:"id"`
    RunID         int64           `json:"run_id"`
    StepOrder     int             `json:"step_order"`
    Status        string          `json:"status"`
    SQLExecuted   string          `json:"sql_executed,omitempty"`
    ResultJSON    json.RawMessage `json:"result_json,omitempty"`
    ResultVerdict string          `json:"result_verdict,omitempty"`
    ResultMessage string          `json:"result_message,omitempty"`
    Error         string          `json:"error,omitempty"`
    ExecutedAt    *time.Time      `json:"executed_at,omitempty"`
    DurationMs    int             `json:"duration_ms,omitempty"`
    ConfirmedBy   string          `json:"confirmed_by,omitempty"`
}
```

### 3.3 Result Interpretation

```go
// interpreter.go

type InterpretationSpec struct {
    Rules          []InterpretationRule `json:"rules,omitempty"`
    RowCountRules  []RowCountRule       `json:"row_count_rules,omitempty"`
    DefaultVerdict string               `json:"default_verdict"`
    DefaultMessage string               `json:"default_message"`
}

type InterpretationRule struct {
    Column   string      `json:"column"`
    Operator string      `json:"operator"` // >, <, >=, <=, ==, !=, is_null, is_not_null
    Value    interface{} `json:"value"`
    Verdict  string      `json:"verdict"`  // green, yellow, red
    Message  string      `json:"message"`  // Template with {{column_name}}
}

type BranchRule struct {
    Condition BranchCondition `json:"condition"`
    GotoStep  int             `json:"goto_step"`
    Reason    string          `json:"reason"`
}

type BranchCondition struct {
    Column   string      `json:"column,omitempty"`
    Operator string      `json:"operator,omitempty"`
    Value    interface{} `json:"value,omitempty"`
    Verdict  string      `json:"verdict,omitempty"` // Alternative: branch on computed verdict
}

// Interpret evaluates the result of a step against its interpretation rules.
// Returns (verdict, message).
func Interpret(spec InterpretationSpec, columns []string, rows [][]interface{}, rowCount int) (string, string) {
    // 1. Check row count rules first
    for _, rule := range spec.RowCountRules {
        if evaluateNumeric(float64(rowCount), rule.Operator, rule.Value) {
            return rule.Verdict, expandTemplate(rule.Message, map[string]interface{}{"row_count": rowCount})
        }
    }

    // 2. Check column-based rules against the first row
    if len(rows) > 0 {
        rowMap := zipColumnsAndRow(columns, rows[0])
        for _, rule := range spec.Rules {
            val, ok := rowMap[rule.Column]
            if !ok { continue }
            if evaluateCondition(val, rule.Operator, rule.Value) {
                return rule.Verdict, expandTemplate(rule.Message, rowMap)
            }
        }
    }

    // 3. Default
    return spec.DefaultVerdict, spec.DefaultMessage
}
```

### 3.4 Execution Engine

```go
// executor.go

type Executor struct {
    connProv InstanceConnProvider
    logger   *slog.Logger
}

type ExecutionResult struct {
    Columns    []string        `json:"columns"`
    Rows       [][]interface{} `json:"rows"`
    RowCount   int             `json:"row_count"`
    TotalRows  int             `json:"total_rows"` // Before LIMIT
    Truncated  bool            `json:"truncated"`
    DurationMs int             `json:"duration_ms"`
}

func (e *Executor) ExecuteStep(ctx context.Context, instanceID string, step Step) (*ExecutionResult, error) {
    // 1. Validate tier
    if step.SafetyTier == string(TierExternal) {
        return nil, fmt.Errorf("external steps cannot be executed — manual action required")
    }

    // 2. Get connection
    conn, err := e.connProv.ConnFor(ctx, instanceID)
    if err != nil {
        return nil, fmt.Errorf("cannot connect to instance %s: %w", instanceID, err)
    }
    defer conn.Release()

    // 3. Set session parameters
    timeout := step.TimeoutSeconds
    if timeout == 0 { timeout = 5 }

    _, err = conn.Exec(ctx, fmt.Sprintf("SET statement_timeout = '%ds'", timeout))
    if err != nil { return nil, err }
    _, err = conn.Exec(ctx, "SET lock_timeout = '5s'")
    if err != nil { return nil, err }

    // 4. READ ONLY enforcement for Tier 1
    if step.SafetyTier == string(TierDiagnostic) {
        _, err = conn.Exec(ctx, "SET default_transaction_read_only = ON")
        if err != nil { return nil, err }
        defer conn.Exec(ctx, "SET default_transaction_read_only = OFF")
    }

    // 5. Execute
    start := time.Now()
    rows, err := conn.Query(ctx, step.SQLTemplate)
    if err != nil {
        return nil, fmt.Errorf("step execution failed: %w", err)
    }
    defer rows.Close()

    // 6. Collect results with LIMIT 100 cap
    columns := fieldDescriptionsToNames(rows.FieldDescriptions())
    var resultRows [][]interface{}
    totalRows := 0
    for rows.Next() {
        totalRows++
        if totalRows <= 100 {
            vals, err := rows.Values()
            if err != nil { continue }
            resultRows = append(resultRows, vals)
        }
    }

    return &ExecutionResult{
        Columns:    columns,
        Rows:       resultRows,
        RowCount:   len(resultRows),
        TotalRows:  totalRows,
        Truncated:  totalRows > 100,
        DurationMs: int(time.Since(start).Milliseconds()),
    }, nil
}
```

### 3.5 Resolver

```go
// resolver.go

type Resolver struct {
    store  PlaybookStore
    logger *slog.Logger
}

type ResolverContext struct {
    HookID       string
    RootCauseKey string
    MetricKey    string
    AdviserRule  string
    InstanceID   string
}

// Resolve returns the best-matching playbook for the given context.
// Returns nil if no match found.
func (r *Resolver) Resolve(ctx context.Context, rc ResolverContext) (*Playbook, string, error) {
    // Priority 1: Explicit hook match
    if rc.HookID != "" {
        pb, err := r.store.FindByTriggerBinding(ctx, "hooks", rc.HookID)
        if err == nil && pb != nil {
            return pb, "rca_hook", nil
        }
    }

    // Priority 2: Root cause key match
    if rc.RootCauseKey != "" {
        pb, err := r.store.FindByTriggerBinding(ctx, "root_causes", rc.RootCauseKey)
        if err == nil && pb != nil {
            return pb, "root_cause", nil
        }
    }

    // Priority 3: Metric key match
    if rc.MetricKey != "" {
        pb, err := r.store.FindByTriggerBinding(ctx, "metrics", rc.MetricKey)
        if err == nil && pb != nil {
            return pb, "metric", nil
        }
    }

    // Priority 4: Adviser rule match
    if rc.AdviserRule != "" {
        pb, err := r.store.FindByTriggerBinding(ctx, "adviser_rules", rc.AdviserRule)
        if err == nil && pb != nil {
            return pb, "adviser_rule", nil
        }
    }

    // Priority 5: No match
    return nil, "", nil
}
```

### 3.6 Store Interface

```go
// store.go

type PlaybookStore interface {
    // CRUD
    Create(ctx context.Context, pb *Playbook) (int64, error)
    Get(ctx context.Context, id int64) (*Playbook, error)
    GetBySlug(ctx context.Context, slug string) (*Playbook, error)
    Update(ctx context.Context, pb *Playbook) error
    Delete(ctx context.Context, id int64) error
    List(ctx context.Context, opts PlaybookListOpts) ([]Playbook, int, error)

    // Status lifecycle
    Promote(ctx context.Context, id int64) error
    Deprecate(ctx context.Context, id int64) error

    // Resolver query
    FindByTriggerBinding(ctx context.Context, bindingType, bindingValue string) (*Playbook, error)

    // Runs
    CreateRun(ctx context.Context, run *Run) (int64, error)
    GetRun(ctx context.Context, id int64) (*Run, error)
    UpdateRun(ctx context.Context, run *Run) error
    ListRuns(ctx context.Context, opts RunListOpts) ([]Run, int, error)
    ListRunsByInstance(ctx context.Context, instanceID string, opts RunListOpts) ([]Run, int, error)

    // Run steps
    CreateRunStep(ctx context.Context, step *RunStep) (int64, error)
    UpdateRunStep(ctx context.Context, step *RunStep) error
    GetRunSteps(ctx context.Context, runID int64) ([]RunStep, error)

    // Seed
    SeedBuiltins(ctx context.Context, playbooks []Playbook) error

    // Cleanup
    CleanOldRuns(ctx context.Context, olderThan time.Duration) (int64, error)
}

type PlaybookListOpts struct {
    Status   string
    Category string
    Search   string
    Limit    int
    Offset   int
}

type RunListOpts struct {
    Status string
    Limit  int
    Offset int
}
```

### 3.7 FindByTriggerBinding SQL

```sql
-- For hooks:
SELECT p.*, array_agg(ps.*) as steps
FROM playbooks p
LEFT JOIN playbook_steps ps ON ps.playbook_id = p.id
WHERE p.status = 'stable'
  AND p.trigger_bindings @> $1::jsonb   -- e.g. '{"hooks": ["remediation.checkpoint_tuning"]}'
GROUP BY p.id
ORDER BY p.version DESC
LIMIT 1;
```

The GIN index on `trigger_bindings` makes the `@>` containment check efficient.

---

## 4. Seed Playbook Example: WAL Archive Failure

```go
// seed_wal.go

func walArchiveFailurePlaybook() Playbook {
    return Playbook{
        Slug:        "wal-archive-failure",
        Name:        "WAL Archive Failure",
        Description: "Diagnoses and helps resolve WAL archiving failures that can lead to disk exhaustion and database shutdown.",
        Status:      "stable",
        Category:    "storage",
        TriggerBindings: TriggerBindings{
            Hooks:      []string{"remediation.wal_config"},
            RootCauses: []string{"root_cause.wal_accumulation"},
            Metrics:    []string{"pg.server.archive_fail_count"},
        },
        EstimatedDurationMin: intPtr(10),
        RequiresPermission:   "view_all",
        IsBuiltin:            true,
        Steps: []Step{
            {
                StepOrder:   1,
                Name:        "Check archive status",
                Description: "Query pg_stat_archiver to see if archiving is healthy or failing.",
                SQLTemplate: "SELECT archived_count, failed_count, last_archived_wal, last_failed_wal, last_archived_time, last_failed_time, stats_reset FROM pg_stat_archiver;",
                SafetyTier:  "diagnostic",
                TimeoutSeconds: 5,
                ResultInterpretation: InterpretationSpec{
                    Rules: []InterpretationRule{
                        {Column: "failed_count", Operator: ">", Value: 0, Verdict: "red",
                         Message: "{{failed_count}} archive failures detected. Last failure: {{last_failed_time}}"},
                        {Column: "failed_count", Operator: "==", Value: 0, Verdict: "green",
                         Message: "No archive failures — archiving is healthy"},
                    },
                    DefaultVerdict: "yellow",
                    DefaultMessage: "Unable to determine archive status",
                },
                BranchRules: []BranchRule{
                    {Condition: BranchCondition{Column: "failed_count", Operator: ">", Value: 0},
                     GotoStep: 2, Reason: "Failures detected — check WAL accumulation"},
                    {Condition: BranchCondition{Verdict: "green"},
                     GotoStep: 5, Reason: "Archiving healthy — verify no residual issues"},
                },
                NextStepDefault: intPtr(2),
            },
            {
                StepOrder:   2,
                Name:        "Check WAL file accumulation",
                Description: "Count WAL files in pg_wal directory. Normal is 10-50 files. Above 100 indicates a backlog.",
                SQLTemplate: "SELECT count(*) AS wal_file_count, pg_size_pretty(sum(size)) AS total_wal_size FROM pg_ls_waldir();",
                SafetyTier:  "diagnostic",
                TimeoutSeconds: 10,
                ResultInterpretation: InterpretationSpec{
                    Rules: []InterpretationRule{
                        {Column: "wal_file_count", Operator: ">", Value: 500, Verdict: "red",
                         Message: "CRITICAL: {{wal_file_count}} WAL files accumulated ({{total_wal_size}}). Disk exhaustion imminent!"},
                        {Column: "wal_file_count", Operator: ">", Value: 100, Verdict: "yellow",
                         Message: "WARNING: {{wal_file_count}} WAL files ({{total_wal_size}}). Archiving is falling behind."},
                        {Column: "wal_file_count", Operator: "<=", Value: 100, Verdict: "green",
                         Message: "WAL file count normal: {{wal_file_count}} files ({{total_wal_size}})"},
                    },
                    DefaultVerdict: "yellow",
                },
                BranchRules: []BranchRule{
                    {Condition: BranchCondition{Column: "wal_file_count", Operator: ">", Value: 500},
                     GotoStep: 4, Reason: "Emergency — escalate immediately"},
                },
                NextStepDefault: intPtr(3),
            },
            {
                StepOrder:   3,
                Name:        "Check archive_command configuration",
                Description: "Verify the archive_command setting and identify where archives are being sent.",
                SQLTemplate: "SELECT name, setting, source, sourcefile FROM pg_settings WHERE name IN ('archive_mode', 'archive_command', 'archive_library', 'archive_timeout') ORDER BY name;",
                SafetyTier:  "diagnostic",
                TimeoutSeconds: 5,
                ResultInterpretation: InterpretationSpec{
                    DefaultVerdict: "yellow",
                    DefaultMessage: "Review the archive_command output. The destination path/host may be unreachable or full.",
                },
                NextStepDefault: intPtr(4),
            },
            {
                StepOrder:      4,
                Name:           "Emergency: Contact infrastructure team",
                Description:    "WAL accumulation indicates the archive target is unavailable. This is typically a storage/network issue outside PostgreSQL.",
                SafetyTier:     "external",
                ManualInstructions: "1. Check the archive target server/storage for disk space.\n2. Verify network connectivity to the archive destination.\n3. If using pg_basebackup or barman, check its logs.\n4. If disk is full on the archive target, free space immediately.\n5. After fixing, verify: SELECT * FROM pg_stat_archiver; -- failed_count should stop increasing.",
                EscalationContact: "Infrastructure team / Storage admin",
                NextStepDefault:   intPtr(5),
            },
            {
                StepOrder:   5,
                Name:        "Verify recovery",
                Description: "After the archive target is restored, verify archiving has resumed.",
                SQLTemplate: "SELECT archived_count, failed_count, last_archived_wal, last_archived_time, NOW() - last_archived_time AS time_since_last_archive FROM pg_stat_archiver;",
                SafetyTier:  "diagnostic",
                TimeoutSeconds: 5,
                ResultInterpretation: InterpretationSpec{
                    Rules: []InterpretationRule{
                        {Column: "failed_count", Operator: "==", Value: 0, Verdict: "green",
                         Message: "Archiving recovered — no more failures"},
                    },
                    DefaultVerdict: "yellow",
                    DefaultMessage: "Archiving may still be catching up. Re-run this step in a few minutes.",
                },
            },
        },
    }
}
```

This pattern is repeated for each of the Core 10 playbooks in their respective `seed_*.go` files.

---

## 5. API Endpoints

### 5.1 Playbook CRUD

```go
// Route registration in internal/api/server.go
r.Route("/api/v1/playbooks", func(r chi.Router) {
    r.Use(requireAuth)
    r.Get("/", handleListPlaybooks)           // viewer+
    r.Get("/resolve", handleResolvePlaybook)   // viewer+
    r.Post("/", requirePerm("alert_management", handleCreatePlaybook))
    r.Route("/{id}", func(r chi.Router) {
        r.Get("/", handleGetPlaybook)          // viewer+
        r.Put("/", requirePerm("alert_management", handleUpdatePlaybook))
        r.Delete("/", requirePerm("user_management", handleDeletePlaybook))
        r.Post("/promote", requirePerm("user_management", handlePromotePlaybook))
        r.Post("/deprecate", requirePerm("alert_management", handleDeprecatePlaybook))
    })
})
```

### 5.2 Playbook Execution

```go
// Instance-scoped run creation
r.Route("/api/v1/instances/{id}/playbooks", func(r chi.Router) {
    r.Use(requireAuth)
    r.Post("/{playbookId}/run", handleStartRun)       // viewer+
    r.Get("/runs", handleListInstanceRuns)             // viewer+
})

// Run management
r.Route("/api/v1/playbook-runs", func(r chi.Router) {
    r.Use(requireAuth)
    r.Get("/", handleListAllRuns)                      // viewer+
    r.Route("/{runId}", func(r chi.Router) {
        r.Get("/", handleGetRun)                       // viewer+
        r.Post("/abandon", handleAbandonRun)           // viewer+
        r.Post("/feedback", handleSubmitFeedback)      // viewer+
        r.Route("/steps/{stepOrder}", func(r chi.Router) {
            r.Post("/execute", handleExecuteStep)      // tier-dependent
            r.Post("/confirm", requirePerm("instance_management", handleConfirmStep))
            r.Post("/approve", requirePerm("instance_management", handleApproveStep))
            r.Post("/skip", handleSkipStep)            // viewer+
        })
    })
})
```

### 5.3 Resolver Endpoint

```
GET /api/v1/playbooks/resolve?hook=remediation.checkpoint_tuning&root_cause=root_cause.checkpoint_storm&metric=pg.checkpoint.write_time_ms&adviser_rule=rem_checkpoint_warn&instance_id=production-primary
```

Returns:
```json
{
  "playbook": { "id": 6, "slug": "checkpoint-storm", "name": "Checkpoint Storm Diagnosis", ... },
  "match_reason": "rca_hook",
  "match_value": "remediation.checkpoint_tuning"
}
```

Returns `{"playbook": null}` if no match.

---

## 6. Execute Step Handler Logic

```go
func handleExecuteStep(w http.ResponseWriter, r *http.Request) {
    runID := chi.URLParam(r, "runId")
    stepOrder := chi.URLParam(r, "stepOrder")
    user := authFromContext(r.Context())

    // 1. Load run + step
    run, _ := store.GetRun(ctx, runID)
    step := findStep(run, stepOrder)

    // 2. Check tier permissions
    switch SafetyTier(step.SafetyTier) {
    case TierDiagnostic:
        // Auto-execute, no permission check beyond viewer
    case TierRemediate:
        // Check if confirmation was provided
        if !requestHasConfirmation(r) {
            respondJSON(w, 200, map[string]string{
                "status": "awaiting_confirmation",
                "sql":    step.SQLTemplate,
            })
            return
        }
    case TierDangerous:
        if !user.HasPermission("instance_management") {
            respondJSON(w, 200, map[string]string{
                "status": "awaiting_approval",
                "message": "This action requires DBA approval",
            })
            return
        }
    case TierExternal:
        respondJSON(w, 200, map[string]string{
            "status":       "manual_action",
            "instructions": step.ManualInstructions,
            "escalation":   step.EscalationContact,
        })
        return
    }

    // 3. Execute
    result, err := executor.ExecuteStep(ctx, run.InstanceID, step)

    // 4. Interpret
    verdict, message := Interpret(step.ResultInterpretation, result.Columns, result.Rows, result.RowCount)

    // 5. Determine next step (branch logic)
    nextStep := resolveNextStep(step, verdict, result)

    // 6. Save run step
    runStep := &RunStep{
        RunID:         run.ID,
        StepOrder:     step.StepOrder,
        Status:        "completed",
        SQLExecuted:   step.SQLTemplate,
        ResultJSON:    marshalResult(result),
        ResultVerdict: verdict,
        ResultMessage: message,
        ExecutedAt:    timePtr(time.Now()),
        DurationMs:    result.DurationMs,
        ConfirmedBy:   user.Username,
    }
    store.CreateRunStep(ctx, runStep)

    // 7. Update run state
    run.CurrentStepOrder = nextStep
    if nextStep == 0 { // Playbook complete
        run.Status = "completed"
        run.CompletedAt = timePtr(time.Now())
    }
    store.UpdateRun(ctx, run)

    // 8. Respond
    respondJSON(w, 200, map[string]interface{}{
        "step_result":  runStep,
        "next_step":    nextStep,
        "run_status":   run.Status,
    })
}
```

---

## 7. Frontend

### 7.1 New Pages

| Page | Route | Component |
|------|-------|-----------|
| PlaybookCatalog | /playbooks | List/grid of all playbooks with filters |
| PlaybookDetail | /playbooks/{id} | Playbook overview with steps preview |
| PlaybookEditor | /playbooks/{id}/edit | Step builder form (alert_management+) |
| PlaybookWizard | /instances/{id}/playbook-runs/{runId} | Step-by-step execution wizard |
| PlaybookRunHistory | /playbook-runs | Fleet-wide run history |

### 7.2 New Components

```
web/src/components/playbook/
  PlaybookCard.tsx          — Card in catalog grid (name, category, tier badges, step count)
  PlaybookFilters.tsx       — Status/category filter bar
  StepBuilder.tsx           — Add/remove/reorder steps with SQL editor
  StepCard.tsx              — Single step display in wizard
  TierBadge.tsx             — Safety tier badge (diagnostic=green, remediate=yellow, dangerous=red, external=gray)
  ResultTable.tsx           — Formatted query results table
  VerdictBadge.tsx          — Green/yellow/red result interpretation badge
  BranchIndicator.tsx       — Shows which step is next and why
  RunProgressBar.tsx        — Step N of M progress indicator
  FeedbackModal.tsx         — Post-run feedback collection
  ResolverButton.tsx        — "Run Playbook" button that calls resolver + starts run
```

### 7.3 Integration Points

**AlertDetailPanel.tsx:**
```typescript
// After the existing ROOT CAUSE ANALYSIS section
const { data: resolved } = useResolvePlaybook({
  metric: alert.metric,
  instance_id: alert.instance_id
});
if (resolved?.playbook) {
  // Render "▶ Run Playbook: {name}" button
  // On click: POST to start run, navigate to wizard
}
```

**RCAIncidentDetail.tsx:**
```typescript
// Below Recommended Actions section
const { data: resolved } = useResolvePlaybook({
  hook: primaryChain?.remediation_hook,
  root_cause: primaryChain?.root_cause_key,
  instance_id: incident.instance_id
});
if (resolved?.playbook) {
  // Render "Guided Remediation" card with playbook name + "Start" button
}
```

**AdvisorRow.tsx:**
```typescript
// Add "▶ Remediate" button
const { data: resolved } = useResolvePlaybook({
  adviser_rule: recommendation.rule_id,
  instance_id: recommendation.instance_id
});
```

### 7.4 Wizard UI Structure

```
┌─────────────────────────────────────────────────────────────┐
│  ← Back to incidents    Playbook: WAL Archive Failure       │
│  Instance: production-primary   Started: 2 min ago          │
│                                                             │
│  [●──●──○──○──○] Step 2 of 5                               │
│                                                             │
│  ┌─────────────────────────────────────────────────────────┐│
│  │ 🟢 Step 1: Check archive status            ✓ Complete  ││
│  │   archived_count=14523, failed_count=847                ││
│  │   🔴 847 archive failures detected                      ││
│  └─────────────────────────────────────────────────────────┘│
│                                                             │
│  ┌─────────────────────────────────────────────────────────┐│
│  │ Step 2: Check WAL file accumulation    [▶ Run]          ││
│  │ 🟢 Diagnostic — auto-executes safely                    ││
│  │                                                         ││
│  │ Count WAL files in pg_wal directory.                    ││
│  │ Normal: 10-50 files. Above 100 indicates a backlog.    ││
│  │                                                         ││
│  │ [Show query ▼]                                          ││
│  └─────────────────────────────────────────────────────────┘│
│                                                             │
│  Steps 3-5 collapsed...                                     │
│                                                             │
│  [Skip Step]  [Abandon Run]                                 │
└─────────────────────────────────────────────────────────────┘
```

---

## 8. Configuration

```yaml
# pgpulse.yml
playbooks:
  enabled: true
  default_statement_timeout: 5    # seconds
  default_lock_timeout: 5         # seconds
  result_row_limit: 100           # max rows returned per step
  run_retention_days: 90          # cleanup old runs
  implicit_feedback_window: 5m    # auto-resolve detection window
```

```go
// internal/config/config.go
type PlaybookConfig struct {
    Enabled                 bool          `yaml:"enabled" koanf:"enabled"`
    DefaultStatementTimeout int           `yaml:"default_statement_timeout" koanf:"default_statement_timeout"`
    DefaultLockTimeout      int           `yaml:"default_lock_timeout" koanf:"default_lock_timeout"`
    ResultRowLimit          int           `yaml:"result_row_limit" koanf:"result_row_limit"`
    RunRetentionDays        int           `yaml:"run_retention_days" koanf:"run_retention_days"`
    ImplicitFeedbackWindow  time.Duration `yaml:"implicit_feedback_window" koanf:"implicit_feedback_window"`
}
```

---

## 9. Agent Team Structure

### Agent 1: Backend

**Creates:**
- `internal/playbook/types.go`
- `internal/playbook/store.go`
- `internal/playbook/pgstore.go`
- `internal/playbook/nullstore.go`
- `internal/playbook/executor.go`
- `internal/playbook/resolver.go`
- `internal/playbook/interpreter.go`
- `internal/playbook/interpreter_test.go`
- `internal/playbook/executor_test.go`
- `internal/playbook/resolver_test.go`
- `internal/playbook/seed.go` + 10 `seed_*.go` files
- `migrations/018_playbooks.sql`
- `internal/api/playbooks.go` — all CRUD + execution handlers
- `internal/api/playbooks_test.go`

**Modifies:**
- `internal/config/config.go` — add PlaybookConfig
- `internal/config/load.go` — defaults
- `internal/api/server.go` — register routes
- `cmd/pgpulse-server/main.go` — wire playbook store, executor, resolver, seed

### Agent 2: Frontend

**Creates:**
- `web/src/pages/PlaybookCatalog.tsx`
- `web/src/pages/PlaybookDetail.tsx`
- `web/src/pages/PlaybookEditor.tsx`
- `web/src/pages/PlaybookWizard.tsx`
- `web/src/pages/PlaybookRunHistory.tsx`
- `web/src/hooks/usePlaybooks.ts`
- `web/src/types/playbook.ts`
- `web/src/components/playbook/PlaybookCard.tsx`
- `web/src/components/playbook/PlaybookFilters.tsx`
- `web/src/components/playbook/StepBuilder.tsx`
- `web/src/components/playbook/StepCard.tsx`
- `web/src/components/playbook/TierBadge.tsx`
- `web/src/components/playbook/ResultTable.tsx`
- `web/src/components/playbook/VerdictBadge.tsx`
- `web/src/components/playbook/BranchIndicator.tsx`
- `web/src/components/playbook/RunProgressBar.tsx`
- `web/src/components/playbook/FeedbackModal.tsx`
- `web/src/components/playbook/ResolverButton.tsx`

**Modifies:**
- `web/src/components/layout/Sidebar.tsx` — add Playbooks nav
- `web/src/pages/RCAIncidentDetail.tsx` — guided remediation section
- `web/src/components/alerts/AlertDetailPanel.tsx` — resolver button
- `web/src/components/advisor/AdvisorRow.tsx` — remediate button
- App router — add playbook routes

---

## 10. Dependency Order

```
Phase 1 (parallel):
  Agent 1: Migration 018 + types.go + store interface + pgstore
  Agent 1: executor.go + interpreter.go (no store dependency)
  Agent 2: types/playbook.ts + component skeletons

Phase 2 (depends on Phase 1):
  Agent 1: resolver.go + seed playbooks + API handlers
  Agent 1: main.go wiring
  Agent 2: PlaybookCatalog + PlaybookDetail + PlaybookWizard pages
  Agent 2: hooks/usePlaybooks.ts

Phase 3 (depends on Phase 2):
  Agent 1: tests
  Agent 2: Integration (AlertDetailPanel, RCAIncidentDetail, AdvisorRow)
  Agent 2: PlaybookEditor + FeedbackModal + RunHistory
```

---

## 11. DO NOT RE-DISCUSS

All D400–D609 decisions are locked. Additionally:

- Four-tier safety model (diagnostic/remediate/dangerous/external) — not three
- Playbook Resolver with 5-level priority — not simple metric/hook binding
- Core 10 seed pack — not 5, not 15
- Bounded branching, no loops — not linear, not arbitrary workflows
- Static declarative interpretation — no expression language, no Go evaluators
- Database-stored playbooks — not YAML files, not hardcoded Go
- PlaybookRun persisted with resume — not in-memory
- SET TRANSACTION READ ONLY for Tier 1 — non-negotiable security invariant
- Approval queue deferred to future iteration
- 2 agents
