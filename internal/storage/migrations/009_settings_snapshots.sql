CREATE TABLE IF NOT EXISTS settings_snapshots (
    id           BIGSERIAL    PRIMARY KEY,
    instance_id  TEXT         NOT NULL,
    captured_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),
    trigger_type TEXT         NOT NULL,
    pg_version   TEXT         NOT NULL DEFAULT '',
    settings     JSONB        NOT NULL DEFAULT '{}'
);

CREATE INDEX IF NOT EXISTS idx_settings_snapshots_instance
    ON settings_snapshots(instance_id, captured_at DESC);
