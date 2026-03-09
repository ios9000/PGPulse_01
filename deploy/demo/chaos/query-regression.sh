#!/usr/bin/env bash
# Query Regression — drops an index to cause plan changes and slower queries
# PGPulse shows: plan hash changes in Plan History (regression), query cost jumps
#
# Usage: ./query-regression.sh           (drop index + run queries)
#        ./query-regression.sh stop      (recreate index)

set -euo pipefail
PORT=5434
DB=demo_app
USER=pgpulse_monitor
PASS=pgpulse_monitor_demo

psql_cmd() {
    PGPASSWORD="${PASS}" psql -h localhost -p "${PORT}" -U "${USER}" -d "${DB}" "$@"
}

if [[ "${1:-}" == "stop" ]]; then
    echo "Recreating index on chaos:${PORT}..."
    psql_cmd -c "CREATE INDEX IF NOT EXISTS idx_demo_queries_user ON demo_queries(user_id);"
    psql_cmd -c "ANALYZE demo_queries;"
    echo "Done. Index restored — queries will use index scan again."
    exit 0
fi

echo "Creating query regression on chaos:${PORT}..."

# First, run some queries WITH the index (establishes baseline plan)
echo "  Running baseline queries with index..."
for i in $(seq 1 20); do
    psql_cmd -c "SELECT count(*) FROM demo_queries WHERE user_id = $((RANDOM % 10000));" >/dev/null
done

# Drop the index
echo "  Dropping idx_demo_queries_user..."
psql_cmd -c "DROP INDEX IF EXISTS idx_demo_queries_user;"
psql_cmd -c "ANALYZE demo_queries;"

# Run the same queries WITHOUT the index (triggers seq scan, plan hash changes)
echo "  Running queries without index (seq scan)..."
for i in $(seq 1 20); do
    psql_cmd -c "SELECT count(*) FROM demo_queries WHERE user_id = $((RANDOM % 10000));" >/dev/null
done

cat <<EOF

  Query regression created:
    - Index idx_demo_queries_user dropped
    - Same queries now use seq scan instead of index scan
    - Plan hash will differ from previous captures

  PGPulse will show:
    - Plan History: new plan captured with different hash (regression)
    - Regressions tab: before/after cost comparison
    - Top queries: higher total time for user_id lookups

  Run '$0 stop' to recreate the index.
EOF
