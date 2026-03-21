CREATE TABLE IF NOT EXISTS rca_incidents (
    id                    BIGSERIAL PRIMARY KEY,
    instance_id           TEXT NOT NULL,
    trigger_metric        TEXT NOT NULL,
    trigger_value         DOUBLE PRECISION NOT NULL,
    trigger_time          TIMESTAMPTZ NOT NULL,
    trigger_kind          TEXT NOT NULL DEFAULT 'alert',
    window_from           TIMESTAMPTZ NOT NULL,
    window_to             TIMESTAMPTZ NOT NULL,
    primary_chain_id      TEXT,
    primary_root_cause    TEXT,
    confidence            DOUBLE PRECISION NOT NULL DEFAULT 0,
    confidence_bucket     TEXT,
    quality_status        TEXT NOT NULL DEFAULT 'unknown',
    timeline_json         JSONB NOT NULL,
    summary               TEXT NOT NULL,
    auto_triggered        BOOLEAN NOT NULL DEFAULT false,
    remediation_hooks     TEXT[],
    chain_version         TEXT,
    anomaly_source_mode   TEXT,
    review_status         TEXT,
    reviewed_by           TEXT,
    reviewed_at           TIMESTAMPTZ,
    review_comment        TEXT,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_rca_incidents_instance ON rca_incidents(instance_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_rca_incidents_trigger ON rca_incidents(trigger_metric, trigger_time);
CREATE INDEX IF NOT EXISTS idx_rca_incidents_chain ON rca_incidents(primary_chain_id);
CREATE INDEX IF NOT EXISTS idx_rca_incidents_review ON rca_incidents(review_status) WHERE review_status IS NOT NULL;
