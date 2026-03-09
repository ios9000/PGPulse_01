#!/usr/bin/env bash
# Lock Contention — creates a blocker → waiter chain
# PGPulse shows: Lock Tree with recursive blocking hierarchy, blocked count badges
#
# Usage: ./lock-contention.sh           (start)
#        ./lock-contention.sh stop      (kill all)

set -euo pipefail
PORT=5434
DB=demo_app
USER=pgpulse_monitor
PASS=pgpulse_monitor_demo

psql_cmd() {
    PGPASSWORD="${PASS}" psql -h localhost -p "${PORT}" -U "${USER}" -d "${DB}" "$@"
}

if [[ "${1:-}" == "stop" ]]; then
    echo "Killing all lock contention sessions..."
    psql_cmd -c "
        SELECT pg_terminate_backend(pid)
        FROM pg_stat_activity
        WHERE application_name LIKE 'pgpulse_chaos_lock%'
          AND pid <> pg_backend_pid();
    "
    echo "Done."
    exit 0
fi

echo "Creating lock contention scenario on chaos:${PORT}..."
echo "  PGPulse will show this in the Lock Tree section."

# Ensure test table exists
psql_cmd -c "CREATE TABLE IF NOT EXISTS lock_demo (id INT PRIMARY KEY, val TEXT);"
psql_cmd -c "INSERT INTO lock_demo VALUES (1, 'original') ON CONFLICT DO NOTHING;"

# Session 1: blocker — holds row lock
(
    PGPASSWORD="${PASS}" psql -h localhost -p "${PORT}" -U "${USER}" -d "${DB}" <<SQL
SET application_name = 'pgpulse_chaos_lock_blocker';
BEGIN;
UPDATE lock_demo SET val = 'blocked' WHERE id = 1;
SELECT 'Blocker holding lock on row id=1';
SELECT pg_sleep(600);
ROLLBACK;
SQL
) &
BLOCKER_PID=$!

sleep 1

# Session 2: waiter 1 — blocked by session 1
(
    PGPASSWORD="${PASS}" psql -h localhost -p "${PORT}" -U "${USER}" -d "${DB}" <<SQL
SET application_name = 'pgpulse_chaos_lock_waiter1';
SET lock_timeout = '600s';
UPDATE lock_demo SET val = 'waiter1' WHERE id = 1;
SQL
) &
WAITER1_PID=$!

sleep 0.5

# Session 3: waiter 2 — also blocked by session 1
(
    PGPASSWORD="${PASS}" psql -h localhost -p "${PORT}" -U "${USER}" -d "${DB}" <<SQL
SET application_name = 'pgpulse_chaos_lock_waiter2';
SET lock_timeout = '600s';
UPDATE lock_demo SET val = 'waiter2' WHERE id = 1;
SQL
) &
WAITER2_PID=$!

cat <<EOF

  Lock chain created:
    Blocker (PID $BLOCKER_PID) holds row lock
    Waiter1 (PID $WAITER1_PID) blocked
    Waiter2 (PID $WAITER2_PID) blocked

  PGPulse Lock Tree will show:
    ├── Blocker [root, 2 blocked]
    │   ├── Waiter1 [waiting]
    │   └── Waiter2 [waiting]

  Run '$0 stop' to end.
EOF
