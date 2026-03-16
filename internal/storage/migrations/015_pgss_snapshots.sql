-- 015: pg_stat_statements snapshot tables

CREATE TABLE IF NOT EXISTS pgss_snapshots (
    id              BIGSERIAL PRIMARY KEY,
    instance_id     TEXT NOT NULL,
    captured_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    pg_version      INT,
    stats_reset     TIMESTAMPTZ,
    total_statements INT NOT NULL DEFAULT 0,
    total_calls     BIGINT NOT NULL DEFAULT 0,
    total_exec_time DOUBLE PRECISION NOT NULL DEFAULT 0,
    CONSTRAINT uq_pgss_snapshot UNIQUE (instance_id, captured_at)
);

CREATE INDEX IF NOT EXISTS idx_pgss_snapshots_instance_time
    ON pgss_snapshots (instance_id, captured_at DESC);

CREATE TABLE IF NOT EXISTS pgss_snapshot_entries (
    snapshot_id       BIGINT NOT NULL REFERENCES pgss_snapshots(id) ON DELETE CASCADE,
    queryid           BIGINT NOT NULL,
    userid            OID,
    dbid              OID,
    database_name     TEXT,
    user_name         TEXT,
    query             TEXT,
    calls             BIGINT,
    total_exec_time   DOUBLE PRECISION,
    total_plan_time   DOUBLE PRECISION,
    rows              BIGINT,
    shared_blks_hit   BIGINT,
    shared_blks_read  BIGINT,
    shared_blks_dirtied BIGINT,
    shared_blks_written BIGINT,
    local_blks_hit    BIGINT,
    local_blks_read   BIGINT,
    temp_blks_read    BIGINT,
    temp_blks_written BIGINT,
    blk_read_time     DOUBLE PRECISION,
    blk_write_time    DOUBLE PRECISION,
    wal_records       BIGINT,
    wal_fpi           BIGINT,
    wal_bytes         NUMERIC,
    mean_exec_time    DOUBLE PRECISION,
    min_exec_time     DOUBLE PRECISION,
    max_exec_time     DOUBLE PRECISION,
    stddev_exec_time  DOUBLE PRECISION,
    PRIMARY KEY (snapshot_id, queryid, dbid, userid)
);

CREATE INDEX IF NOT EXISTS idx_pgss_entries_queryid
    ON pgss_snapshot_entries (queryid);
