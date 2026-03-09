CREATE TABLE IF NOT EXISTS query_plans (
    id                BIGSERIAL    PRIMARY KEY,
    instance_id       TEXT         NOT NULL,
    database_name     TEXT         NOT NULL DEFAULT '',
    query_fingerprint TEXT         NOT NULL,
    plan_hash         TEXT         NOT NULL,
    plan_text         TEXT,
    plan_json         JSONB,
    trigger_type      TEXT         NOT NULL,
    duration_ms       BIGINT,
    query_text        TEXT,
    truncated         BOOLEAN      NOT NULL DEFAULT FALSE,
    metadata          JSONB,
    captured_at       TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_query_plans_dedup
    ON query_plans(instance_id, query_fingerprint, plan_hash);
CREATE INDEX IF NOT EXISTS idx_query_plans_instance_time
    ON query_plans(instance_id, captured_at DESC);
CREATE INDEX IF NOT EXISTS idx_query_plans_fingerprint
    ON query_plans(instance_id, query_fingerprint, captured_at DESC);
