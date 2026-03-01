CREATE EXTENSION IF NOT EXISTS timescaledb;

SELECT create_hypertable('metrics', 'time',
    if_not_exists => TRUE,
    migrate_data  => TRUE
);
