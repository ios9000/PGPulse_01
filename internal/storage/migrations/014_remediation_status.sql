-- 014_remediation_status.sql — Add status lifecycle columns for background evaluation

ALTER TABLE remediation_recommendations
    ADD COLUMN IF NOT EXISTS status VARCHAR(20) NOT NULL DEFAULT 'active',
    ADD COLUMN IF NOT EXISTS evaluated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ADD COLUMN IF NOT EXISTS resolved_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_remediation_status
    ON remediation_recommendations (instance_id, status);

CREATE INDEX IF NOT EXISTS idx_remediation_evaluated
    ON remediation_recommendations (evaluated_at);

-- Unique partial index: at most one active recommendation per rule+instance.
CREATE UNIQUE INDEX IF NOT EXISTS idx_remediation_active_unique
    ON remediation_recommendations (rule_id, instance_id)
    WHERE status = 'active';
