-- M14_04: Guided Remediation Playbooks

-- Playbook definitions
CREATE TABLE IF NOT EXISTS playbooks (
    id                     BIGSERIAL PRIMARY KEY,
    slug                   TEXT UNIQUE NOT NULL,
    name                   TEXT NOT NULL,
    description            TEXT NOT NULL DEFAULT '',
    version                INT NOT NULL DEFAULT 1,
    status                 TEXT NOT NULL DEFAULT 'draft',
    category               TEXT NOT NULL DEFAULT 'general',
    trigger_bindings       JSONB NOT NULL DEFAULT '{}',
    estimated_duration_min INT,
    requires_permission    TEXT NOT NULL DEFAULT 'view_all',
    author                 TEXT NOT NULL DEFAULT '',
    is_builtin             BOOL NOT NULL DEFAULT false,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_playbooks_status ON playbooks(status);
CREATE INDEX IF NOT EXISTS idx_playbooks_category ON playbooks(category);
CREATE INDEX IF NOT EXISTS idx_playbooks_trigger_bindings ON playbooks USING GIN(trigger_bindings);

-- Playbook steps
CREATE TABLE IF NOT EXISTS playbook_steps (
    id                     BIGSERIAL PRIMARY KEY,
    playbook_id            BIGINT NOT NULL REFERENCES playbooks(id) ON DELETE CASCADE,
    step_order             INT NOT NULL,
    name                   TEXT NOT NULL,
    description            TEXT NOT NULL DEFAULT '',
    sql_template           TEXT,
    safety_tier            TEXT NOT NULL DEFAULT 'diagnostic',
    timeout_seconds        INT NOT NULL DEFAULT 5,
    result_interpretation  JSONB NOT NULL DEFAULT '{}',
    branch_rules           JSONB NOT NULL DEFAULT '[]',
    next_step_default      INT,
    manual_instructions    TEXT,
    escalation_contact     TEXT,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
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
