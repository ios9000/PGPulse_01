-- Alert rules: stores both builtin and custom rules
CREATE TABLE IF NOT EXISTS alert_rules (
    id                TEXT PRIMARY KEY,
    name              TEXT NOT NULL,
    description       TEXT NOT NULL DEFAULT '',
    metric            TEXT NOT NULL,
    operator          TEXT NOT NULL CHECK (operator IN ('>', '>=', '<', '<=', '==', '!=')),
    threshold         DOUBLE PRECISION NOT NULL,
    severity          TEXT NOT NULL CHECK (severity IN ('info', 'warning', 'critical')),
    labels            JSONB NOT NULL DEFAULT '{}',
    consecutive_count INTEGER NOT NULL DEFAULT 3,
    cooldown_minutes  INTEGER NOT NULL DEFAULT 15,
    channels          JSONB NOT NULL DEFAULT '[]',
    source            TEXT NOT NULL CHECK (source IN ('builtin', 'custom')),
    enabled           BOOLEAN NOT NULL DEFAULT true,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Alert history: records fired and resolved events
CREATE TABLE IF NOT EXISTS alert_history (
    id            BIGSERIAL PRIMARY KEY,
    rule_id       TEXT NOT NULL REFERENCES alert_rules(id) ON DELETE CASCADE,
    instance_id   TEXT NOT NULL,
    severity      TEXT NOT NULL CHECK (severity IN ('info', 'warning', 'critical')),
    metric        TEXT NOT NULL,
    value         DOUBLE PRECISION NOT NULL,
    threshold     DOUBLE PRECISION NOT NULL,
    operator      TEXT NOT NULL,
    labels        JSONB NOT NULL DEFAULT '{}',
    fired_at      TIMESTAMPTZ NOT NULL,
    resolved_at   TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Fast lookup for unresolved alerts (state restoration on startup)
CREATE INDEX IF NOT EXISTS idx_alert_history_unresolved
    ON alert_history (rule_id, instance_id) WHERE resolved_at IS NULL;

-- Time-based queries for alert history API
CREATE INDEX IF NOT EXISTS idx_alert_history_fired_at
    ON alert_history (fired_at DESC);

-- Per-instance history queries
CREATE INDEX IF NOT EXISTS idx_alert_history_instance
    ON alert_history (instance_id, fired_at DESC);
