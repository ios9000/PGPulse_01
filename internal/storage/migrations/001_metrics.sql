CREATE TABLE IF NOT EXISTS metrics (
    time        TIMESTAMPTZ      NOT NULL,
    instance_id TEXT             NOT NULL,
    metric      TEXT             NOT NULL,
    value       DOUBLE PRECISION NOT NULL,
    labels      JSONB            NOT NULL DEFAULT '{}'
);

CREATE INDEX IF NOT EXISTS idx_metrics_instance_time
    ON metrics (instance_id, time DESC);

CREATE INDEX IF NOT EXISTS idx_metrics_metric_time
    ON metrics (metric, time DESC);

CREATE INDEX IF NOT EXISTS idx_metrics_instance_metric_time
    ON metrics (instance_id, metric, time DESC);
