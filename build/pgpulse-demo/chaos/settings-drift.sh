#!/usr/bin/env bash
# Settings Drift — changes several settings via ALTER SYSTEM
# PGPulse shows: Settings Timeline captures the change, diff view highlights it
#
# Usage: ./settings-drift.sh            (apply changes)
#        ./settings-drift.sh stop       (revert to defaults)

set -euo pipefail
PORT=5434

psql_cmd() {
    sudo -u postgres psql -p "${PORT}" "$@"
}

if [[ "${1:-}" == "stop" ]]; then
    echo "Reverting settings on chaos:${PORT}..."
    psql_cmd -c "ALTER SYSTEM RESET work_mem;"
    psql_cmd -c "ALTER SYSTEM RESET maintenance_work_mem;"
    psql_cmd -c "ALTER SYSTEM RESET effective_cache_size;"
    psql_cmd -c "ALTER SYSTEM RESET random_page_cost;"
    psql_cmd -c "ALTER SYSTEM RESET log_min_duration_statement;"
    psql_cmd -c "SELECT pg_reload_conf();"
    echo "Done. Settings reverted to defaults."
    exit 0
fi

echo "Applying settings drift on chaos:${PORT}..."

psql_cmd -c "ALTER SYSTEM SET work_mem = '256MB';"
psql_cmd -c "ALTER SYSTEM SET maintenance_work_mem = '1GB';"
psql_cmd -c "ALTER SYSTEM SET effective_cache_size = '512MB';"
psql_cmd -c "ALTER SYSTEM SET random_page_cost = 1.1;"
psql_cmd -c "ALTER SYSTEM SET log_min_duration_statement = 100;"
psql_cmd -c "SELECT pg_reload_conf();"

cat <<EOF

  Settings changed on chaos:${PORT}:
    work_mem:                   64MB → 256MB
    maintenance_work_mem:       64MB → 1GB
    effective_cache_size:       4GB  → 512MB
    random_page_cost:           4.0  → 1.1
    log_min_duration_statement: -1   → 100ms

  PGPulse will show:
    - Settings Timeline: new snapshot captured (next cycle)
    - Settings Diff: compare before/after
    - Cross-instance diff: chaos vs primary shows divergence

  Note: work_mem change requires "pending restart" = false (reload-safe).

  Run '$0 stop' to revert.
EOF
