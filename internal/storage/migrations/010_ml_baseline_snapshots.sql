CREATE TABLE IF NOT EXISTS ml_baseline_snapshots (
    id           BIGSERIAL    PRIMARY KEY,
    instance_id  TEXT         NOT NULL,
    metric_key   TEXT         NOT NULL,
    period       INT          NOT NULL,
    state        JSONB        NOT NULL,
    updated_at   TIMESTAMPTZ  NOT NULL DEFAULT now(),
    UNIQUE (instance_id, metric_key)
);

CREATE INDEX IF NOT EXISTS idx_ml_baseline_snapshots_instance
    ON ml_baseline_snapshots(instance_id);
