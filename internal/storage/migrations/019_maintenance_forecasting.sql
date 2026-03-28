-- Migration 019: Maintenance Operation Forecasting
-- Iteration: M15_01

-- Completed maintenance operations history
CREATE TABLE IF NOT EXISTS maintenance_operations (
    id                BIGSERIAL PRIMARY KEY,
    instance_id       TEXT NOT NULL,
    operation         TEXT NOT NULL CHECK (operation IN ('vacuum', 'analyze', 'reindex_concurrent', 'basebackup')),
    outcome           TEXT NOT NULL DEFAULT 'unknown' CHECK (outcome IN ('completed', 'canceled', 'failed', 'disappeared', 'unknown')),
    database          TEXT NOT NULL DEFAULT '',
    table_name        TEXT NOT NULL DEFAULT '',
    table_size_bytes  BIGINT,
    started_at        TIMESTAMPTZ NOT NULL,
    completed_at      TIMESTAMPTZ NOT NULL,
    duration_sec      DOUBLE PRECISION NOT NULL,
    final_pct         DOUBLE PRECISION,
    avg_rate_per_sec  DOUBLE PRECISION,
    metadata          JSONB NOT NULL DEFAULT '{}',
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_maint_ops_instance_op
    ON maintenance_operations(instance_id, operation);
CREATE INDEX IF NOT EXISTS idx_maint_ops_instance_table
    ON maintenance_operations(instance_id, database, table_name);
CREATE INDEX IF NOT EXISTS idx_maint_ops_completed
    ON maintenance_operations(completed_at DESC);

-- Cached maintenance forecasts (UPSERT by unique constraint)
CREATE TABLE IF NOT EXISTS maintenance_forecasts (
    id                  BIGSERIAL PRIMARY KEY,
    instance_id         TEXT NOT NULL,
    database            TEXT NOT NULL DEFAULT '',
    table_name          TEXT NOT NULL DEFAULT '',
    operation           TEXT NOT NULL CHECK (operation IN ('vacuum', 'analyze', 'reindex', 'basebackup')),
    status              TEXT NOT NULL CHECK (status IN ('predicted', 'imminent', 'overdue', 'not_needed', 'insufficient_data')),
    predicted_at        TIMESTAMPTZ,
    time_until_sec      DOUBLE PRECISION,
    confidence_lower    TIMESTAMPTZ,
    confidence_upper    TIMESTAMPTZ,
    current_value       DOUBLE PRECISION,
    threshold_value     DOUBLE PRECISION,
    accumulation_rate   DOUBLE PRECISION,
    method              TEXT NOT NULL DEFAULT 'threshold_projection',
    evaluated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(instance_id, database, table_name, operation)
);

CREATE INDEX IF NOT EXISTS idx_maint_forecasts_instance
    ON maintenance_forecasts(instance_id);
CREATE INDEX IF NOT EXISTS idx_maint_forecasts_status
    ON maintenance_forecasts(status) WHERE status IN ('imminent', 'overdue');
