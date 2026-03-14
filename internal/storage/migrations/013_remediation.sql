-- 013_remediation.sql — Remediation recommendations table

CREATE TABLE IF NOT EXISTS remediation_recommendations (
    id              BIGSERIAL PRIMARY KEY,
    rule_id         TEXT NOT NULL,
    instance_id     TEXT NOT NULL,
    alert_event_id  BIGINT,
    metric_key      TEXT NOT NULL DEFAULT '',
    metric_value    DOUBLE PRECISION NOT NULL DEFAULT 0,
    priority        TEXT NOT NULL CHECK (priority IN ('info', 'suggestion', 'action_required')),
    category        TEXT NOT NULL CHECK (category IN ('performance', 'capacity', 'configuration', 'replication', 'maintenance')),
    title           TEXT NOT NULL,
    description     TEXT NOT NULL,
    doc_url         TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    acknowledged_at TIMESTAMPTZ,
    acknowledged_by TEXT NOT NULL DEFAULT ''
);

-- Query patterns: by instance (Server detail), fleet-wide (Advisor page), by alert event
CREATE INDEX IF NOT EXISTS idx_remediation_instance
    ON remediation_recommendations (instance_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_remediation_priority
    ON remediation_recommendations (priority, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_remediation_alert_event
    ON remediation_recommendations (alert_event_id)
    WHERE alert_event_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_remediation_unacknowledged
    ON remediation_recommendations (created_at DESC)
    WHERE acknowledged_at IS NULL;
