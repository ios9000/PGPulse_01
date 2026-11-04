-- Conformance fixture, run once per cluster as the bootstrap superuser.
-- Used as /docker-entrypoint-initdb.d/ script (Docker route) and fed to psql
-- directly (PGDG / pre-existing cluster route).
--
-- The harness itself connects as pgpulse_monitor, which holds pg_monitor ONLY
-- (never superuser), so every collector runs with production privileges.

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'pgpulse_monitor') THEN
        CREATE ROLE pgpulse_monitor LOGIN PASSWORD 'conformance';
    END IF;
END
$$;
GRANT pg_monitor TO pgpulse_monitor;

CREATE EXTENSION IF NOT EXISTS pg_stat_statements;

-- Replication slots so the slot collector returns rows (wal_level=logical is
-- set on the command line; test_decoding ships with every postgres build).
SELECT pg_create_physical_replication_slot('conformance_physical')
WHERE NOT EXISTS (SELECT 1 FROM pg_replication_slots WHERE slot_name = 'conformance_physical');
SELECT pg_create_logical_replication_slot('conformance_logical', 'test_decoding')
WHERE NOT EXISTS (SELECT 1 FROM pg_replication_slots WHERE slot_name = 'conformance_logical');

CREATE DATABASE conformance;
\connect conformance

CREATE EXTENSION IF NOT EXISTS pg_stat_statements;

-- Seed objects so the per-database collector has something to report on.
CREATE SCHEMA app;
CREATE TABLE app.orders (
    id         bigserial PRIMARY KEY,
    customer   text NOT NULL,
    amount     numeric(12, 2) NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    note       text
);
CREATE INDEX orders_customer_idx ON app.orders (customer);
CREATE INDEX orders_unused_idx ON app.orders (created_at);
ALTER TABLE app.orders SET (autovacuum_vacuum_scale_factor = 0.05);
INSERT INTO app.orders (customer, amount, note)
SELECT 'c' || (g % 100), g * 1.5, repeat('x', 3000)
FROM generate_series(1, 2000) AS g;
DELETE FROM app.orders WHERE id % 3 = 0;

CREATE UNLOGGED TABLE app.scratch (k int PRIMARY KEY, v text);
INSERT INTO app.scratch SELECT g, md5(g::text) FROM generate_series(1, 100) AS g;

CREATE SEQUENCE app.ticket_seq;
SELECT nextval('app.ticket_seq') FROM generate_series(1, 5);

CREATE FUNCTION app.order_total(p_customer text) RETURNS numeric
LANGUAGE sql STABLE AS $fn$
    SELECT coalesce(sum(amount), 0) FROM app.orders WHERE customer = p_customer
$fn$;

SELECT lo_create(0);

ANALYZE;

GRANT CONNECT ON DATABASE conformance TO pgpulse_monitor;
