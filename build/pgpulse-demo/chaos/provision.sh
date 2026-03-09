#!/usr/bin/env bash
#
# PGPulse Demo — Full Provisioning Script
# Target: Fresh Ubuntu 24.04 LTS (cloud VM)
#
# Creates:
#   - PostgreSQL 16: primary (:5432), replica (:5433), standalone (:5434)
#   - Streaming replication: primary → replica
#   - Logical replication: primary → standalone (demo_orders table)
#   - PGPulse server (:8989) with storage on primary
#   - Monitoring user (pgpulse_monitor) on all instances
#   - Demo data (pgbench + custom tables)
#   - Chaos scripts in /opt/pgpulse/chaos/
#
# Usage:
#   # Copy the pgpulse-server binary to this directory first:
#   scp pgpulse-server user@vm:/tmp/
#
#   # Then run:
#   sudo bash provision.sh /tmp/pgpulse-server
#
# Or build + deploy in one shot from the dev machine:
#   make build-linux
#   scp build/pgpulse-server user@vm:/tmp/
#   ssh user@vm 'sudo bash /opt/pgpulse-demo/provision.sh /tmp/pgpulse-server'

set -euo pipefail

# ── Configuration ────────────────────────────────────────────────
PG_VERSION=16
PG_PRIMARY_PORT=5432
PG_REPLICA_PORT=5433
PG_CHAOS_PORT=5434

PGPULSE_USER="pgpulse"
PGPULSE_HOME="/opt/pgpulse"
PGPULSE_PORT=8989
PGPULSE_DB="pgpulse_storage"

MONITOR_USER="pgpulse_monitor"
MONITOR_PASS="pgpulse_monitor_demo"    # Change in production!

REPL_USER="replicator"
REPL_PASS="replicator_demo"            # Change in production!

ADMIN_USER="admin"
ADMIN_PASS="pgpulse_admin"             # PGPulse web UI login

DEMO_DB="demo_app"

# ── Colours ──────────────────────────────────────────────────────
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

info()  { echo -e "${BLUE}[INFO]${NC}  $*"; }
ok()    { echo -e "${GREEN}[OK]${NC}    $*"; }
warn()  { echo -e "${YELLOW}[WARN]${NC}  $*"; }
err()   { echo -e "${RED}[ERR]${NC}   $*" >&2; }
step()  { echo -e "\n${GREEN}━━━ $* ━━━${NC}\n"; }

# ── Pre-checks ───────────────────────────────────────────────────
if [[ $EUID -ne 0 ]]; then
    err "This script must be run as root (sudo)"
    exit 1
fi

BINARY_PATH="${1:-}"
if [[ -z "$BINARY_PATH" || ! -f "$BINARY_PATH" ]]; then
    err "Usage: sudo bash provision.sh /path/to/pgpulse-server"
    err "  Build it first: make build-linux"
    exit 1
fi

# ── Step 1: Install PostgreSQL ───────────────────────────────────
step "1/9  Installing PostgreSQL ${PG_VERSION}"

if ! command -v psql &>/dev/null; then
    apt-get update -qq
    apt-get install -y -qq curl ca-certificates gnupg lsb-release

    # PGDG repository
    install -d /usr/share/postgresql-common/pgdg
    curl -fsSL https://www.postgresql.org/media/keys/ACCC4CF8.asc \
        | gpg --dearmor -o /usr/share/postgresql-common/pgdg/apt.postgresql.org.gpg
    echo "deb [signed-by=/usr/share/postgresql-common/pgdg/apt.postgresql.org.gpg] \
        https://apt.postgresql.org/pub/repos/apt $(lsb_release -cs)-pgdg main" \
        > /etc/apt/sources.list.d/pgdg.list

    apt-get update -qq
    apt-get install -y -qq "postgresql-${PG_VERSION}"
    ok "PostgreSQL ${PG_VERSION} installed"
else
    ok "PostgreSQL already installed: $(psql --version)"
fi

# ── Step 2: Create 3 PostgreSQL Clusters ─────────────────────────
step "2/9  Creating PostgreSQL clusters"

create_cluster() {
    local name="$1" port="$2"
    if pg_lsclusters -h | grep -q "^${PG_VERSION} ${name}"; then
        warn "Cluster ${name} already exists on port ${port}"
    else
        pg_createcluster "${PG_VERSION}" "${name}" -p "${port}" -- \
            --auth-local=peer --auth-host=scram-sha-256
        ok "Created cluster: ${name} on port ${port}"
    fi
}

# Primary is the default 'main' cluster
if ! pg_lsclusters -h | grep -q "^${PG_VERSION} main"; then
    warn "Default 'main' cluster missing — creating"
    pg_createcluster "${PG_VERSION}" main -p "${PG_PRIMARY_PORT}"
fi

create_cluster "replica" "${PG_REPLICA_PORT}"
create_cluster "chaos"   "${PG_CHAOS_PORT}"

# ── Step 3: Configure Primary for Replication ────────────────────
step "3/9  Configuring primary for replication"

PG_CONF="/etc/postgresql/${PG_VERSION}/main/postgresql.conf"
PG_HBA="/etc/postgresql/${PG_VERSION}/main/pg_hba.conf"

# postgresql.conf overrides for primary
cat >> "${PG_CONF}" <<EOF

# === PGPulse Demo Overrides ===
wal_level = logical
max_wal_senders = 10
max_replication_slots = 10
hot_standby = on
shared_preload_libraries = 'pg_stat_statements'
pg_stat_statements.max = 5000
pg_stat_statements.track = all
track_io_timing = on
track_functions = all
log_checkpoints = on
log_lock_waits = on
log_temp_files = 0
listen_addresses = 'localhost'
EOF

# pg_hba.conf: allow replication
if ! grep -q "${REPL_USER}" "${PG_HBA}"; then
    cat >> "${PG_HBA}" <<EOF

# Replication
local   replication     ${REPL_USER}                    scram-sha-256
host    replication     ${REPL_USER}    127.0.0.1/32    scram-sha-256
host    replication     ${REPL_USER}    ::1/128         scram-sha-256

# PGPulse monitoring
host    all             ${MONITOR_USER} 127.0.0.1/32    scram-sha-256
host    all             ${MONITOR_USER} ::1/128         scram-sha-256
EOF
fi

# Configure chaos instance
CHAOS_CONF="/etc/postgresql/${PG_VERSION}/chaos/postgresql.conf"
cat >> "${CHAOS_CONF}" <<EOF

# === PGPulse Demo Overrides (chaos) ===
shared_preload_libraries = 'pg_stat_statements'
pg_stat_statements.max = 5000
pg_stat_statements.track = all
track_io_timing = on
track_functions = all
log_checkpoints = on
listen_addresses = 'localhost'
max_connections = 120
EOF

CHAOS_HBA="/etc/postgresql/${PG_VERSION}/chaos/pg_hba.conf"
if ! grep -q "${MONITOR_USER}" "${CHAOS_HBA}"; then
    cat >> "${CHAOS_HBA}" <<EOF

# PGPulse monitoring
host    all             ${MONITOR_USER} 127.0.0.1/32    scram-sha-256
host    all             ${MONITOR_USER} ::1/128         scram-sha-256
EOF
fi

# Start primary and chaos
pg_ctlcluster "${PG_VERSION}" main start || true
pg_ctlcluster "${PG_VERSION}" chaos start || true
ok "Primary (:${PG_PRIMARY_PORT}) and chaos (:${PG_CHAOS_PORT}) started"

# ── Step 4: Create Users and Databases ───────────────────────────
step "4/9  Creating users and databases"

run_sql() {
    local port="$1"; shift
    sudo -u postgres psql -p "${port}" -Atq -c "$@" 2>/dev/null || true
}

run_sql_db() {
    local port="$1" db="$2"; shift 2
    sudo -u postgres psql -p "${port}" -d "${db}" -Atq -c "$@" 2>/dev/null || true
}

# On primary
run_sql ${PG_PRIMARY_PORT} "DO \$\$ BEGIN
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = '${REPL_USER}') THEN
        CREATE ROLE ${REPL_USER} WITH REPLICATION LOGIN PASSWORD '${REPL_PASS}';
    END IF;
END \$\$;"

run_sql ${PG_PRIMARY_PORT} "DO \$\$ BEGIN
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = '${MONITOR_USER}') THEN
        CREATE ROLE ${MONITOR_USER} WITH LOGIN PASSWORD '${MONITOR_PASS}';
    END IF;
END \$\$;"
run_sql ${PG_PRIMARY_PORT} "GRANT pg_monitor TO ${MONITOR_USER};"
run_sql ${PG_PRIMARY_PORT} "GRANT pg_signal_backend TO ${MONITOR_USER};"

# Storage database
run_sql ${PG_PRIMARY_PORT} "SELECT 1 FROM pg_database WHERE datname='${PGPULSE_DB}'" | grep -q 1 \
    || run_sql ${PG_PRIMARY_PORT} "CREATE DATABASE ${PGPULSE_DB} OWNER ${MONITOR_USER};"

# Demo database on primary
run_sql ${PG_PRIMARY_PORT} "SELECT 1 FROM pg_database WHERE datname='${DEMO_DB}'" | grep -q 1 \
    || run_sql ${PG_PRIMARY_PORT} "CREATE DATABASE ${DEMO_DB};"
run_sql_db ${PG_PRIMARY_PORT} "${DEMO_DB}" "CREATE EXTENSION IF NOT EXISTS pg_stat_statements;"

# On chaos instance
run_sql ${PG_CHAOS_PORT} "DO \$\$ BEGIN
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = '${MONITOR_USER}') THEN
        CREATE ROLE ${MONITOR_USER} WITH LOGIN PASSWORD '${MONITOR_PASS}';
    END IF;
END \$\$;"
run_sql ${PG_CHAOS_PORT} "GRANT pg_monitor TO ${MONITOR_USER};"
run_sql ${PG_CHAOS_PORT} "GRANT pg_signal_backend TO ${MONITOR_USER};"

# Demo database on chaos
run_sql ${PG_CHAOS_PORT} "SELECT 1 FROM pg_database WHERE datname='${DEMO_DB}'" | grep -q 1 \
    || run_sql ${PG_CHAOS_PORT} "CREATE DATABASE ${DEMO_DB};"
run_sql_db ${PG_CHAOS_PORT} "${DEMO_DB}" "CREATE EXTENSION IF NOT EXISTS pg_stat_statements;"

ok "Users and databases created"

# ── Step 5: Set Up Streaming Replication ─────────────────────────
step "5/9  Setting up streaming replication (primary → replica)"

pg_ctlcluster "${PG_VERSION}" replica stop 2>/dev/null || true

REPLICA_DATA="/var/lib/postgresql/${PG_VERSION}/replica"

# Wipe and basebackup
if [[ -d "${REPLICA_DATA}" ]]; then
    rm -rf "${REPLICA_DATA}"
fi

sudo -u postgres pg_basebackup \
    -h localhost -p "${PG_PRIMARY_PORT}" -U "${REPL_USER}" \
    -D "${REPLICA_DATA}" -Fp -Xs -P -R 2>/dev/null <<EOF
${REPL_PASS}
EOF

# Replica config
REPLICA_CONF="/etc/postgresql/${PG_VERSION}/replica/postgresql.conf"
cat >> "${REPLICA_CONF}" <<EOF

# === PGPulse Demo Overrides (replica) ===
hot_standby = on
primary_conninfo = 'host=localhost port=${PG_PRIMARY_PORT} user=${REPL_USER} password=${REPL_PASS}'
shared_preload_libraries = 'pg_stat_statements'
pg_stat_statements.max = 5000
pg_stat_statements.track = all
track_io_timing = on
listen_addresses = 'localhost'
EOF

REPLICA_HBA="/etc/postgresql/${PG_VERSION}/replica/pg_hba.conf"
if ! grep -q "${MONITOR_USER}" "${REPLICA_HBA}"; then
    cat >> "${REPLICA_HBA}" <<EOF

# PGPulse monitoring
host    all             ${MONITOR_USER} 127.0.0.1/32    scram-sha-256
host    all             ${MONITOR_USER} ::1/128         scram-sha-256
EOF
fi

chown -R postgres:postgres "${REPLICA_DATA}"
pg_ctlcluster "${PG_VERSION}" replica start
ok "Streaming replica running on port ${PG_REPLICA_PORT}"

# Create replication slot
run_sql ${PG_PRIMARY_PORT} \
    "SELECT pg_create_physical_replication_slot('replica_slot', true);" 2>/dev/null || true

# ── Step 6: Set Up Logical Replication ───────────────────────────
step "6/9  Setting up logical replication (primary → chaos)"

# Create publication on primary
run_sql_db ${PG_PRIMARY_PORT} "${DEMO_DB}" "
    CREATE TABLE IF NOT EXISTS demo_orders (
        id          SERIAL PRIMARY KEY,
        customer    TEXT NOT NULL,
        amount      NUMERIC(10,2) NOT NULL,
        status      TEXT DEFAULT 'pending',
        created_at  TIMESTAMPTZ DEFAULT now()
    );
"
run_sql_db ${PG_PRIMARY_PORT} "${DEMO_DB}" \
    "CREATE PUBLICATION pgpulse_demo FOR TABLE demo_orders;" 2>/dev/null || true

# Create subscription on chaos instance
run_sql_db ${PG_CHAOS_PORT} "${DEMO_DB}" "
    CREATE TABLE IF NOT EXISTS demo_orders (
        id          SERIAL PRIMARY KEY,
        customer    TEXT NOT NULL,
        amount      NUMERIC(10,2) NOT NULL,
        status      TEXT DEFAULT 'pending',
        created_at  TIMESTAMPTZ DEFAULT now()
    );
"
run_sql_db ${PG_CHAOS_PORT} "${DEMO_DB}" "
    CREATE SUBSCRIPTION pgpulse_demo_sub
    CONNECTION 'host=localhost port=${PG_PRIMARY_PORT} dbname=${DEMO_DB} user=${REPL_USER} password=${REPL_PASS}'
    PUBLICATION pgpulse_demo;" 2>/dev/null || true

ok "Logical replication: primary.${DEMO_DB}.demo_orders → chaos.${DEMO_DB}.demo_orders"

# ── Step 7: Seed Demo Data ───────────────────────────────────────
step "7/9  Seeding demo data"

# pgbench tables on chaos instance (for bloat and activity demos)
sudo -u postgres pgbench -i -s 10 -p "${PG_CHAOS_PORT}" "${DEMO_DB}" 2>/dev/null
ok "pgbench tables (scale 10) on chaos:${DEMO_DB}"

# Custom tables on chaos for various demos
run_sql_db ${PG_CHAOS_PORT} "${DEMO_DB}" "
    -- Large table for cache miss demos
    CREATE TABLE IF NOT EXISTS demo_large (
        id      SERIAL PRIMARY KEY,
        data    TEXT,
        value   NUMERIC(12,4),
        ts      TIMESTAMPTZ DEFAULT now()
    );
    INSERT INTO demo_large (data, value)
    SELECT
        md5(random()::text) || md5(random()::text),
        random() * 10000
    FROM generate_series(1, 500000)
    ON CONFLICT DO NOTHING;

    -- Table with an index we can drop for regression demo
    CREATE TABLE IF NOT EXISTS demo_queries (
        id          SERIAL PRIMARY KEY,
        user_id     INT NOT NULL,
        action      TEXT NOT NULL,
        payload     JSONB,
        created_at  TIMESTAMPTZ DEFAULT now()
    );
    CREATE INDEX IF NOT EXISTS idx_demo_queries_user ON demo_queries(user_id);
    INSERT INTO demo_queries (user_id, action, payload)
    SELECT
        (random() * 10000)::int,
        (ARRAY['view','click','purchase','search','login'])[floor(random()*5+1)::int],
        jsonb_build_object('ip', '10.0.' || (random()*255)::int || '.' || (random()*255)::int)
    FROM generate_series(1, 200000)
    ON CONFLICT DO NOTHING;

    ANALYZE;
"

# Seed some orders for logical replication demo
run_sql_db ${PG_PRIMARY_PORT} "${DEMO_DB}" "
    INSERT INTO demo_orders (customer, amount, status)
    SELECT
        'customer_' || (random() * 1000)::int,
        (random() * 500 + 10)::numeric(10,2),
        (ARRAY['pending','shipped','delivered','returned'])[floor(random()*4+1)::int]
    FROM generate_series(1, 5000)
    ON CONFLICT DO NOTHING;
"
ok "Demo data seeded"

# ── Step 8: Install PGPulse ─────────────────────────────────────
step "8/9  Installing PGPulse server"

# Create system user
id "${PGPULSE_USER}" &>/dev/null || useradd -r -s /usr/sbin/nologin -d "${PGPULSE_HOME}" "${PGPULSE_USER}"

# Directory structure
mkdir -p "${PGPULSE_HOME}"/{bin,configs,chaos}
cp "${BINARY_PATH}" "${PGPULSE_HOME}/bin/pgpulse-server"
chmod +x "${PGPULSE_HOME}/bin/pgpulse-server"

# Config file
cat > "${PGPULSE_HOME}/configs/pgpulse.yml" <<YAML
server:
  port: ${PGPULSE_PORT}
  host: "0.0.0.0"

auth:
  enabled: true
  jwt_secret: "$(openssl rand -hex 32)"
  refresh_secret: "$(openssl rand -hex 32)"
  seed_admin:
    username: "${ADMIN_USER}"
    password: "${ADMIN_PASS}"

storage:
  dsn: "host=localhost port=${PG_PRIMARY_PORT} dbname=${PGPULSE_DB} user=${MONITOR_USER} password=${MONITOR_PASS} sslmode=disable"

instances:
  - id: "production-primary"
    name: "Production Primary"
    dsn: "host=localhost port=${PG_PRIMARY_PORT} dbname=postgres user=${MONITOR_USER} password=${MONITOR_PASS} sslmode=disable"
    enabled: true
    max_conns: 3

  - id: "production-replica"
    name: "Production Replica"
    dsn: "host=localhost port=${PG_REPLICA_PORT} dbname=postgres user=${MONITOR_USER} password=${MONITOR_PASS} sslmode=disable"
    enabled: true
    max_conns: 3

  - id: "staging-chaos"
    name: "Staging (Chaos Target)"
    dsn: "host=localhost port=${PG_CHAOS_PORT} dbname=postgres user=${MONITOR_USER} password=${MONITOR_PASS} sslmode=disable"
    enabled: true
    max_conns: 3

alerting:
  enabled: true
  evaluation_interval: 30

ml:
  enabled: true
  bootstrap_timeout: 30
  metrics:
    - key: "connections_active"
      seasonal_period: 60
      z_threshold: 3.0
    - key: "cache_hit_ratio"
      seasonal_period: 60
      z_threshold: 3.0
  forecast:
    horizon: 30
    confidence_z: 1.96
    alert_min_consecutive: 3
  persistence:
    enabled: true

plan_capture:
  enabled: true
  duration_threshold_ms: 1000
  scheduled_top_n: 10
  retention_days: 7

settings_snapshot:
  enabled: true
  interval_minutes: 60

logging:
  level: "info"
  format: "json"
YAML

# Systemd service
cat > /etc/systemd/system/pgpulse.service <<EOF
[Unit]
Description=PGPulse PostgreSQL Monitor
After=postgresql.service network.target
Wants=postgresql.service

[Service]
Type=simple
User=${PGPULSE_USER}
Group=${PGPULSE_USER}
ExecStart=${PGPULSE_HOME}/bin/pgpulse-server --config ${PGPULSE_HOME}/configs/pgpulse.yml
Restart=on-failure
RestartSec=5
LimitNOFILE=65536

# Security hardening
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=${PGPULSE_HOME}
PrivateTmp=true

[Install]
WantedBy=multi-user.target
EOF

chown -R "${PGPULSE_USER}:${PGPULSE_USER}" "${PGPULSE_HOME}"

systemctl daemon-reload
systemctl enable pgpulse
systemctl start pgpulse
ok "PGPulse running on port ${PGPULSE_PORT}"

# ── Step 9: Install Chaos Scripts ────────────────────────────────
step "9/9  Installing chaos scripts"

# Copy chaos scripts (they should be in the same directory as this script)
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [[ -d "${SCRIPT_DIR}/chaos" ]]; then
    cp "${SCRIPT_DIR}/chaos/"*.sh "${PGPULSE_HOME}/chaos/"
    chmod +x "${PGPULSE_HOME}/chaos/"*.sh
    ok "Chaos scripts installed to ${PGPULSE_HOME}/chaos/"
else
    warn "No chaos/ directory found next to provision.sh — skipping"
fi

# ── Summary ──────────────────────────────────────────────────────
step "Provisioning Complete"

VM_IP=$(hostname -I | awk '{print $1}')

cat <<EOF

  ┌─────────────────────────────────────────────────────────┐
  │  PGPulse Demo Environment                               │
  ├─────────────────────────────────────────────────────────┤
  │                                                         │
  │  PGPulse UI:   http://${VM_IP}:${PGPULSE_PORT}                      │
  │  Login:        ${ADMIN_USER} / ${ADMIN_PASS}                  │
  │                                                         │
  │  PostgreSQL Instances:                                  │
  │    Primary:     localhost:${PG_PRIMARY_PORT}  (production)          │
  │    Replica:     localhost:${PG_REPLICA_PORT}  (streaming replica)   │
  │    Chaos:       localhost:${PG_CHAOS_PORT}  (staging / chaos)      │
  │                                                         │
  │  Monitor user:  ${MONITOR_USER} / ${MONITOR_PASS}  │
  │  Demo DB:       ${DEMO_DB} (on primary + chaos)               │
  │                                                         │
  │  Chaos scripts: ${PGPULSE_HOME}/chaos/                  │
  │    long-transaction.sh    lock-contention.sh            │
  │    connection-flood.sh    bloat-generator.sh            │
  │    replication-lag.sh     settings-drift.sh             │
  │    query-regression.sh    logical-repl-pause.sh         │
  │    cache-thrash.sh        cleanup-all.sh                │
  │                                                         │
  │  Logs:  journalctl -u pgpulse -f                        │
  │  Stop:  systemctl stop pgpulse                          │
  │                                                         │
  └─────────────────────────────────────────────────────────┘

  Wait ~2 minutes for PGPulse to collect initial metrics,
  then open the UI to see the dashboard populate.

  ML anomaly detection needs ~30 minutes of baseline data
  before forecasts appear.

EOF
