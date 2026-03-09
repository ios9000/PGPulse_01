-- 007_session_audit_log.sql — Audit trail for session cancel/terminate operations (M8_01)
CREATE TABLE IF NOT EXISTS session_audit_log (
    id              BIGSERIAL   PRIMARY KEY,
    instance_id     TEXT        NOT NULL,
    operator_user   TEXT        NOT NULL,
    target_pid      INT         NOT NULL,
    operation       TEXT        NOT NULL CHECK (operation IN ('cancel','terminate')),
    result          TEXT        NOT NULL CHECK (result IN ('ok','error')),
    error_message   TEXT,
    executed_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_session_audit_log_instance
    ON session_audit_log (instance_id, executed_at DESC);

CREATE INDEX IF NOT EXISTS idx_session_audit_log_operator
    ON session_audit_log (operator_user, executed_at DESC);
