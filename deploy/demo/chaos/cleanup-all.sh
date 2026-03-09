#!/usr/bin/env bash
# Cleanup All — reverses every chaos scenario at once
# Run this to reset the demo to a clean state
#
# Usage: ./cleanup-all.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PORT=5434
DB=demo_app
USER=pgpulse_monitor
PASS=pgpulse_monitor_demo

echo "═══ PGPulse Demo Cleanup ═══"
echo ""

# 1. Kill all chaos sessions
echo "1. Killing all chaos sessions..."
PGPASSWORD="${PASS}" psql -h localhost -p "${PORT}" -U "${USER}" -d "${DB}" -c "
    SELECT pg_terminate_backend(pid)
    FROM pg_stat_activity
    WHERE application_name LIKE 'pgpulse_chaos_%'
      AND pid <> pg_backend_pid();
" 2>/dev/null || true
echo "   Done."

# 2. Resume WAL replay on replica
echo "2. Resuming WAL replay on replica..."
sudo -u postgres psql -p 5433 -c "SELECT pg_wal_replay_resume();" 2>/dev/null || true
echo "   Done."

# 3. Re-enable logical replication
echo "3. Re-enabling logical replication subscription..."
sudo -u postgres psql -p "${PORT}" -d "${DB}" -c \
    "ALTER SUBSCRIPTION pgpulse_demo_sub ENABLE;" 2>/dev/null || true
echo "   Done."

# 4. Revert settings drift
echo "4. Reverting ALTER SYSTEM settings..."
sudo -u postgres psql -p "${PORT}" -c "ALTER SYSTEM RESET ALL;"
sudo -u postgres psql -p "${PORT}" -c "SELECT pg_reload_conf();"
echo "   Done."

# 5. Recreate dropped index
echo "5. Recreating dropped indexes..."
PGPASSWORD="${PASS}" psql -h localhost -p "${PORT}" -U "${USER}" -d "${DB}" -c \
    "CREATE INDEX IF NOT EXISTS idx_demo_queries_user ON demo_queries(user_id);" 2>/dev/null || true
echo "   Done."

# 6. Re-enable autovacuum + vacuum bloated tables
echo "6. Fixing bloat (re-enable autovacuum + VACUUM)..."
PGPASSWORD="${PASS}" psql -h localhost -p "${PORT}" -U "${USER}" -d "${DB}" -c \
    "ALTER TABLE pgbench_accounts SET (autovacuum_enabled = true);" 2>/dev/null || true
PGPASSWORD="${PASS}" psql -h localhost -p "${PORT}" -U "${USER}" -d "${DB}" -c \
    "VACUUM pgbench_accounts;" 2>/dev/null || true
echo "   Done."

# 7. Cleanup lock demo table
echo "7. Cleaning up lock demo table..."
PGPASSWORD="${PASS}" psql -h localhost -p "${PORT}" -U "${USER}" -d "${DB}" -c \
    "DROP TABLE IF EXISTS lock_demo;" 2>/dev/null || true
echo "   Done."

# 8. Analyze all tables
echo "8. Running ANALYZE..."
PGPASSWORD="${PASS}" psql -h localhost -p "${PORT}" -U "${USER}" -d "${DB}" -c \
    "ANALYZE;" 2>/dev/null || true
echo "   Done."

cat <<EOF

  ═══════════════════════════════════
  All chaos scenarios cleaned up.
  Demo environment is back to normal.
  ═══════════════════════════════════

  PGPulse will show metrics returning to normal
  within 1-2 collection cycles (~60 seconds).

  Alerts will auto-resolve once thresholds are
  no longer breached.
EOF
