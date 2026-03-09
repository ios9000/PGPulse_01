#!/usr/bin/env bash
# Long Transaction — holds a transaction open for N minutes
# PGPulse shows: Long Transactions table (amber >1min, red >5min), alert fires
#
# Usage: ./long-transaction.sh [minutes]     (default: 10)
#        ./long-transaction.sh stop          (kill all)

set -euo pipefail
PORT=5434
DB=demo_app
USER=pgpulse_monitor
PASS=pgpulse_monitor_demo
DURATION="${1:-10}"

if [[ "${DURATION}" == "stop" ]]; then
    echo "Killing all long-transaction demo sessions..."
    PGPASSWORD="${PASS}" psql -h localhost -p "${PORT}" -U "${USER}" -d "${DB}" -c "
        SELECT pg_terminate_backend(pid)
        FROM pg_stat_activity
        WHERE application_name = 'pgpulse_chaos_long_tx'
          AND pid <> pg_backend_pid();
    "
    echo "Done."
    exit 0
fi

echo "Starting long transaction (${DURATION} minutes) on chaos:${PORT}..."
echo "  PGPulse will show this in Long Transactions within ~10 seconds."
echo "  Run '$0 stop' to end it."

PGPASSWORD="${PASS}" psql -h localhost -p "${PORT}" -U "${USER}" -d "${DB}" \
    -v ON_ERROR_STOP=1 \
    -c "SET application_name = 'pgpulse_chaos_long_tx'" \
    -c "BEGIN" \
    -c "SELECT 'Transaction started at ' || now()" \
    -c "SELECT pg_sleep(${DURATION} * 60)" \
    -c "ROLLBACK" &

echo "PID: $!"
echo "Transaction will auto-rollback after ${DURATION} minutes."
