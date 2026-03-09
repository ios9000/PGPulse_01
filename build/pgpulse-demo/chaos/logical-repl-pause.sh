#!/usr/bin/env bash
# Logical Replication Pause — disables the subscription apply worker
# PGPulse shows: Logical Replication section shows pending sync tables
#
# Usage: ./logical-repl-pause.sh           (pause + generate data)
#        ./logical-repl-pause.sh stop      (resume)

set -euo pipefail
PRIMARY_PORT=5432
CHAOS_PORT=5434
DB=demo_app

if [[ "${1:-}" == "stop" ]]; then
    echo "Resuming logical replication on chaos:${CHAOS_PORT}..."
    sudo -u postgres psql -p "${CHAOS_PORT}" -d "${DB}" -c \
        "ALTER SUBSCRIPTION pgpulse_demo_sub ENABLE;"
    echo "Done. Subscription will catch up."
    exit 0
fi

echo "Pausing logical replication on chaos:${CHAOS_PORT}..."

# Disable the subscription (stops apply worker)
sudo -u postgres psql -p "${CHAOS_PORT}" -d "${DB}" -c \
    "ALTER SUBSCRIPTION pgpulse_demo_sub DISABLE;"

# Generate data on the primary that won't replicate
echo "Inserting data on primary (won't replicate while paused)..."
sudo -u postgres psql -p "${PRIMARY_PORT}" -d "${DB}" -c "
    INSERT INTO demo_orders (customer, amount, status)
    SELECT
        'pending_customer_' || i,
        (random() * 100 + 5)::numeric(10,2),
        'pending'
    FROM generate_series(1, 100) AS i;
"

# Show subscription state
sudo -u postgres psql -p "${CHAOS_PORT}" -d "${DB}" -c "
    SELECT subname, subenabled, subslotname
    FROM pg_subscription;
"

cat <<EOF

  Logical replication paused.
  100 new orders inserted on primary but NOT replicated to chaos.

  PGPulse will show:
    - Logical Replication section: subscription disabled
    - When re-enabled: tables will briefly show sync states (i/d/s)

  Run '$0 stop' to resume.
EOF
