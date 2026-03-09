#!/usr/bin/env bash
# Cache Thrash — forces sequential scans on large data to drop cache hit ratio
# PGPulse shows: cache hit ratio chart drops, alert fires if <90%
#
# Usage: ./cache-thrash.sh              (run thrashing queries)
#        ./cache-thrash.sh stop         (nothing to undo — just stop running)

set -euo pipefail
PORT=5434
DB=demo_app
USER=pgpulse_monitor
PASS=pgpulse_monitor_demo

if [[ "${1:-}" == "stop" ]]; then
    echo "Killing cache thrash sessions..."
    PGPASSWORD="${PASS}" psql -h localhost -p "${PORT}" -U "${USER}" -d "${DB}" -c "
        SELECT pg_terminate_backend(pid)
        FROM pg_stat_activity
        WHERE application_name = 'pgpulse_chaos_cache'
          AND pid <> pg_backend_pid();
    "
    echo "Done. Cache hit ratio will recover as buffer cache warms up."
    exit 0
fi

echo "Thrashing buffer cache on chaos:${PORT}..."
echo "  Running sequential scans on demo_large (500k rows) to evict cached pages."
echo "  PGPulse cache hit ratio will drop. Press Ctrl+C or run '$0 stop' to end."

# Continuously scan the large table to pollute the buffer cache
while true; do
    PGPASSWORD="${PASS}" psql -h localhost -p "${PORT}" -U "${USER}" -d "${DB}" \
        -c "SET application_name = 'pgpulse_chaos_cache'" \
        -c "SET enable_indexscan = off; SET enable_bitmapscan = off;" \
        -c "SELECT count(*), sum(length(data)) FROM demo_large WHERE value > random() * 10000;" \
        >/dev/null 2>&1
    sleep 0.5
done
