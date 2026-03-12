PGPulse — PostgreSQL Health & Activity Monitor
===============================================

Quick Start
-----------

1. Monitor a PostgreSQL instance instantly (no config needed):

   pgpulse-server --target "postgres://user:pass@host:5432/postgres"

   Then open http://localhost:8989 in your browser.

2. Monitor with separate host/port/user:

   pgpulse-server --target-host 192.168.1.100 --target-user pgpulse_monitor

3. Use a config file for multiple instances:

   cp config.sample.yaml pgpulse.yml
   # Edit pgpulse.yml with your settings
   pgpulse-server --config pgpulse.yml

Command-Line Options
--------------------

  --target DSN          PostgreSQL connection string (quick-start mode)
  --target-host HOST    PostgreSQL host (alternative to --target)
  --target-port PORT    PostgreSQL port (default: 5432)
  --target-user USER    PostgreSQL user (default: pgpulse_monitor)
  --target-password PW  PostgreSQL password
  --target-dbname DB    PostgreSQL database (default: postgres)
  --listen ADDR:PORT    HTTP listen address (default: :8989)
  --history DURATION    Memory retention for live mode (default: 2h)
  --no-auth             Disable authentication
  --config PATH         Config file path (default: pgpulse.yml)

Live Mode vs Persistent Mode
-----------------------------

Live Mode (default when no storage DSN):
  - Metrics stored in memory (default 2h retention)
  - No ML forecasting, plan capture, or settings snapshots
  - Perfect for quick diagnostics and troubleshooting

Persistent Mode (when storage.dsn is configured):
  - Metrics stored in PostgreSQL/TimescaleDB
  - Full feature set: ML anomaly detection, plan capture, alerting
  - Required for production monitoring

PostgreSQL User Setup
---------------------

  CREATE ROLE pgpulse_monitor LOGIN PASSWORD 'your_password';
  GRANT pg_monitor TO pgpulse_monitor;

For more information, visit: https://github.com/ios9000/PGPulse_01
