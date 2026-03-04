CREATE TABLE IF NOT EXISTS instances (
    id         TEXT        PRIMARY KEY,
    name       TEXT        NOT NULL DEFAULT '',
    dsn        TEXT        NOT NULL DEFAULT '',
    host       TEXT        NOT NULL DEFAULT '',
    port       INTEGER     NOT NULL DEFAULT 5432,
    enabled    BOOLEAN     NOT NULL DEFAULT true,
    source     TEXT        NOT NULL DEFAULT 'manual',
    max_conns  INTEGER     NOT NULL DEFAULT 5,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
