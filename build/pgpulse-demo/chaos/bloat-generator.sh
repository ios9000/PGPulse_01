#!/usr/bin/env bash
# Bloat Generator — creates significant table bloat via update+no-vacuum
# PGPulse shows: per-DB bloat estimate goes amber/red, vacuum need analysis
#
# Usage: ./bloat-generator.sh           (start)
#        ./bloat-generator.sh stop      (vacuum + cleanup)

set -euo pipefail
PORT=5434
DB=demo_app
USER=pgpulse_monitor
PASS=pgpulse_monitor_demo

psql_cmd() {
    PGPASSWORD="${PASS}" psql -h localhost -p "${PORT}" -U "${USER}" -d "${DB}" "$@"
}

if [[ "${1:-}" == "stop" ]]; then
    echo "Cleaning up bloat..."
    psql_cmd -c "VACUUM FULL pgbench_accounts;"
    psql_cmd -c "ANALYZE pgbench_accounts;"
    echo "Done. Bloat removed."
    exit 0
fi

echo "Generating table bloat on chaos:${PORT}..."
echo "  Disabling autovacuum on pgbench_accounts, then updating all rows."
echo "  PGPulse will show high bloat ratio and vacuum-need warnings."

# Disable autovacuum on target table
psql_cmd -c "ALTER TABLE pgbench_accounts SET (autovacuum_enabled = false);"

# Update all rows multiple times to create dead tuples
for round in 1 2 3; do
    echo "  Round ${round}/3: updating all rows..."
    psql_cmd -c "UPDATE pgbench_accounts SET abalance = abalance + 1;"
done

# Show dead tuple count
psql_cmd -c "
    SELECT
        relname,
        n_live_tup,
        n_dead_tup,
        round(n_dead_tup::numeric / NULLIF(n_live_tup, 0) * 100, 1) AS dead_pct,
        last_autovacuum
    FROM pg_stat_user_tables
    WHERE relname = 'pgbench_accounts';
"

cat <<EOF

  Bloat generated on pgbench_accounts.
  PGPulse will show:
    - High bloat ratio in per-DB analysis
    - autovacuum_enabled=off warning (red)
    - High dead tuple count in vacuum need
  
  Run '$0 stop' to VACUUM FULL and re-enable autovacuum.
EOF
