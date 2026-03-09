#!/usr/bin/env bash
# Replication Lag — pauses WAL replay on the streaming replica
# PGPulse shows: replication lag chart spikes, lag bytes increase, alert fires
#
# Usage: ./replication-lag.sh           (pause replay)
#        ./replication-lag.sh stop      (resume replay)

set -euo pipefail
REPLICA_PORT=5433
PRIMARY_PORT=5432
DB=demo_app

if [[ "${1:-}" == "stop" ]]; then
    echo "Resuming WAL replay on replica:${REPLICA_PORT}..."
    sudo -u postgres psql -p "${REPLICA_PORT}" -c "SELECT pg_wal_replay_resume();"
    echo "Done. Replica will catch up shortly."
    exit 0
fi

echo "Pausing WAL replay on replica:${REPLICA_PORT}..."
sudo -u postgres psql -p "${REPLICA_PORT}" -c "SELECT pg_wal_replay_pause();"

echo "Generating WAL on primary to create visible lag..."
sudo -u postgres pgbench -c 4 -T 10 -p "${PRIMARY_PORT}" "${DB}" 2>/dev/null

# Show current lag
sudo -u postgres psql -p "${PRIMARY_PORT}" -c "
    SELECT
        client_addr,
        state,
        pg_wal_lsn_diff(pg_current_wal_lsn(), replay_lsn) AS replay_lag_bytes,
        replay_lag
    FROM pg_stat_replication;
"

cat <<EOF

  WAL replay paused on replica.
  PGPulse will show:
    - Replication lag growing in the Replication section
    - Lag bytes increasing in the chart
    - Replication lag alert fires (if lag > threshold)

  Run '$0 stop' to resume replay.
EOF
