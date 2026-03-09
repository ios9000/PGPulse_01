#!/usr/bin/env bash
# Connection Flood — opens N idle connections
# PGPulse shows: connection gauge amber/red, connection count chart spikes, alert fires
# Chaos instance has max_connections=120
#
# Usage: ./connection-flood.sh [count]       (default: 90)
#        ./connection-flood.sh stop          (kill all)

set -euo pipefail
PORT=5434
DB=demo_app
USER=pgpulse_monitor
PASS=pgpulse_monitor_demo
COUNT="${1:-90}"

if [[ "${COUNT}" == "stop" ]]; then
    echo "Killing all flood sessions..."
    PGPASSWORD="${PASS}" psql -h localhost -p "${PORT}" -U "${USER}" -d "${DB}" -c "
        SELECT pg_terminate_backend(pid)
        FROM pg_stat_activity
        WHERE application_name = 'pgpulse_chaos_flood'
          AND pid <> pg_backend_pid();
    "
    echo "Done."
    exit 0
fi

echo "Opening ${COUNT} idle connections on chaos:${PORT} (max_connections=120)..."
echo "  PGPulse connection gauge will go amber (>80%) then red (>99%)."

PIDS=()
for i in $(seq 1 "${COUNT}"); do
    (
        PGPASSWORD="${PASS}" psql -h localhost -p "${PORT}" -U "${USER}" -d "${DB}" \
            -c "SET application_name = 'pgpulse_chaos_flood'" \
            -c "SELECT pg_sleep(3600)" \
            2>/dev/null
    ) &
    PIDS+=($!)

    # Progress every 10
    if (( i % 10 == 0 )); then
        echo "  Opened ${i}/${COUNT} connections..."
    fi
done

echo ""
echo "  ${COUNT} idle connections opened."
echo "  Connection utilization: ~$((COUNT * 100 / 120))% of max_connections=120"
echo "  Run '$0 stop' to release all."
