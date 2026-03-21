# PGPulse Desktop — Setup Guide

This guide covers installing PGPulse Desktop on Windows, preparing your PostgreSQL instances for monitoring, and configuring the application for your environment.

## 1. Installation

### Using the Installer (Recommended)

1. Download `PGPulse-Setup.exe` from the releases page.
2. Run the installer. You may see a Windows SmartScreen prompt — click "More info" then "Run anyway" (the binary is not yet code-signed).
3. Choose your installation directory (default: `C:\Program Files\PGPulse`).
4. Optionally check "Start with Windows" to launch PGPulse automatically on login.
5. Click Install. The installer creates a desktop shortcut and a Start Menu entry.

The installer places these files in the installation directory:

| File | Purpose |
|------|---------|
| `pgpulse.exe` | Main application binary |
| `pgpulse.yml` | Sample configuration file (edit this) |
| `Uninstall.exe` | Uninstaller (also accessible via Add/Remove Programs) |

### Portable Installation (No Installer)

1. Download `pgpulse-desktop.exe`.
2. Place it in any directory you prefer.
3. Double-click to launch. On first run, the connection dialog will appear.

No installation or registry changes are made in portable mode. Settings are saved to `%APPDATA%\PGPulse\settings.json`.

### Uninstalling

Use Add/Remove Programs (Settings → Apps → PGPulse → Uninstall), or run `Uninstall.exe` from the installation directory. This removes the binary, shortcuts, and registry entries. Your configuration file and `%APPDATA%\PGPulse\` settings are preserved.

## 2. Preparing PostgreSQL for Monitoring

PGPulse connects to your PostgreSQL instances as a monitoring user with the `pg_monitor` role. It never requires superuser access.

### 2.1 Create the Monitoring User

Run this on each PostgreSQL instance you want to monitor:

```sql
-- Create the monitoring user
CREATE ROLE pgpulse_monitor WITH LOGIN PASSWORD 'your_secure_password';

-- Grant the pg_monitor role (read access to statistics views)
GRANT pg_monitor TO pgpulse_monitor;

-- Grant pg_read_server_files for OS metrics via SQL (optional)
GRANT pg_read_server_files TO pgpulse_monitor;
```

### 2.2 Enable pg_stat_statements

PGPulse uses `pg_stat_statements` for query analysis, workload reports, and query insights. Add it to `shared_preload_libraries` in `postgresql.conf`:

```
shared_preload_libraries = 'pg_stat_statements'
pg_stat_statements.track = all
```

Restart PostgreSQL, then create the extension in each database:

```sql
CREATE EXTENSION IF NOT EXISTS pg_stat_statements;
```

PGPulse works without `pg_stat_statements` — query analysis features will simply be unavailable, and a warning will be logged.

### 2.3 pg_read_file Permission Fix (Ubuntu/Debian PG 16+)

On Ubuntu 24.04 and Debian with PostgreSQL 16+, the distribution package revokes `EXECUTE` from `PUBLIC` on `pg_read_file`. If you want OS metrics collection via SQL (the default mode), you must explicitly grant execute permission:

```sql
GRANT EXECUTE ON FUNCTION pg_read_file(text) TO pgpulse_monitor;
GRANT EXECUTE ON FUNCTION pg_read_file(text, bigint, bigint) TO pgpulse_monitor;
GRANT EXECUTE ON FUNCTION pg_read_file(text, bigint, bigint, boolean) TO pgpulse_monitor;
```

This is only needed on Ubuntu/Debian with PG 16+. Other distributions and older PostgreSQL versions are unaffected.

### 2.4 Network Access

Ensure PGPulse can reach your PostgreSQL instances over the network. Check `pg_hba.conf` allows connections from the machine running PGPulse Desktop:

```
# Allow PGPulse monitoring from DBA workstation
host    all     pgpulse_monitor    192.168.1.100/32    scram-sha-256
```

Replace the IP with your workstation's address. Reload PostgreSQL after editing `pg_hba.conf`:

```sql
SELECT pg_reload_conf();
```

## 3. Configuration

PGPulse uses a YAML configuration file. The installer includes a sample at `pgpulse.yml` in the installation directory.

### 3.1 Minimal Configuration (One Instance, No Persistent Storage)

If you just want to monitor a single instance without historical data:

```yaml
server:
  listen: ":8989"
  log_level: info

instances:
  - id: my-primary
    name: "Production Primary"
    dsn: "postgresql://pgpulse_monitor:your_password@db-host:5432/postgres?sslmode=prefer"
```

This runs in live mode — metrics are held in memory for the current session.

### 3.2 Full Configuration (Fleet Monitoring with History)

For production fleet monitoring with persistent metric storage, alerting, and ML forecasting:

```yaml
server:
  listen: ":8989"
  log_level: info

storage:
  dsn: "postgresql://pgpulse_admin:storage_password@localhost:5432/pgpulse_storage?sslmode=prefer"
  use_timescaledb: true
  retention_days: 30

auth:
  enabled: true
  jwt_secret: "your-secret-key-at-least-32-characters-long"
  initial_admin:
    username: admin
    password: changeme

alerting:
  enabled: true
  default_consecutive_count: 3
  default_cooldown_minutes: 15
  email:
    host: smtp.yourcompany.com
    port: 587
    username: alerts@yourcompany.com
    password: smtp_password
    from: alerts@yourcompany.com
    recipients:
      - dba-team@yourcompany.com

ml:
  enabled: true
  zscore_threshold_warning: 2.0
  zscore_threshold_critical: 3.0

remediation:
  enabled: true
  background_interval: 5m

statement_snapshots:
  enabled: true
  interval: 30m
  retention_days: 30
  capture_on_startup: true
  top_n: 50

instances:
  - id: primary
    name: "Production Primary"
    dsn: "postgresql://pgpulse_monitor:password@db-primary:5432/postgres?sslmode=prefer"
    intervals:
      high: 10s
      medium: 60s
      low: 300s

  - id: replica-1
    name: "Read Replica 1"
    dsn: "postgresql://pgpulse_monitor:password@db-replica-1:5432/postgres?sslmode=prefer"

  - id: replica-2
    name: "Read Replica 2"
    dsn: "postgresql://pgpulse_monitor:password@db-replica-2:5432/postgres?sslmode=prefer"
```

### 3.3 Storage Database Setup

For persistent mode, create a dedicated database for PGPulse's metric storage:

```sql
-- On the storage PostgreSQL instance (can be the same as a monitored instance)
CREATE DATABASE pgpulse_storage;

-- Install TimescaleDB (if available)
\c pgpulse_storage
CREATE EXTENSION IF NOT EXISTS timescaledb;
```

PGPulse automatically runs migrations on startup to create its schema.

### 3.4 Configuration Reference

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `server.listen` | string | `:8080` | HTTP listen address (used in server mode) |
| `server.log_level` | string | `info` | Log level: debug, info, warn, error |
| `storage.dsn` | string | _(empty)_ | Storage DB connection string. Empty = live mode (in-memory) |
| `storage.use_timescaledb` | bool | `true` | Use TimescaleDB hypertables for metric storage |
| `storage.retention_days` | int | `30` | How many days of metric history to keep |
| `auth.enabled` | bool | `false` | Enable JWT authentication |
| `auth.jwt_secret` | string | _(empty)_ | JWT signing secret (minimum 32 characters) |
| `alerting.enabled` | bool | `false` | Enable the alert engine |
| `alerting.default_consecutive_count` | int | `3` | Consecutive violations before alert fires |
| `alerting.default_cooldown_minutes` | int | `15` | Cooldown between repeated alerts |
| `ml.enabled` | bool | `false` | Enable ML anomaly detection and forecasting |
| `remediation.enabled` | bool | `false` | Enable the background advisor |
| `statement_snapshots.enabled` | bool | `false` | Enable pg_stat_statements snapshot capture |
| `statement_snapshots.interval` | duration | `30m` | Capture interval |
| `os_metrics.method` | string | `sql` | OS metrics method: sql, agent, or disabled |
| `instances[].id` | string | _(required)_ | Unique instance identifier |
| `instances[].name` | string | _(empty)_ | Display name |
| `instances[].dsn` | string | _(required)_ | PostgreSQL connection string |
| `instances[].intervals.high` | duration | `10s` | High-frequency collection (connections, locks) |
| `instances[].intervals.medium` | duration | `60s` | Medium-frequency (statements, replication) |
| `instances[].intervals.low` | duration | `300s` | Low-frequency (database sizes, settings) |

## 4. First Launch

### With the Installer

1. Launch PGPulse from the desktop shortcut or Start Menu.
2. Edit `C:\Program Files\PGPulse\pgpulse.yml` with your instance details.
3. Relaunch PGPulse — it will read the config automatically.

### With the Connection Dialog

If you launch PGPulse without specifying a config file (or the default path doesn't exist), a connection dialog appears:

- **Open config file** — Browse for a `.yml` config file using the native file picker.
- **Quick connect** — Enter a PostgreSQL DSN (e.g., `postgresql://user:pass@host:5432/postgres`) for instant live-mode monitoring.
- **Last used** — If you've opened a config file before, PGPulse remembers the path. Click "Use" to reload it.

The last-used config path is saved in `%APPDATA%\PGPulse\settings.json`.

### Command-Line Options

PGPulse Desktop accepts these command-line flags:

| Flag | Default | Description |
|------|---------|-------------|
| `--mode` | `desktop` | Run mode: `desktop` (native window) or `server` (headless HTTP server) |
| `--config` | _(auto-detect)_ | Path to `pgpulse.yml` config file |

Example:

```
pgpulse.exe --mode=desktop --config="C:\PGPulse\pgpulse.yml"
```

## 5. Verifying the Setup

After launching PGPulse with your configuration:

1. The system tray icon should appear (green circle = healthy).
2. The main window shows the fleet overview page.
3. Click on an instance to see its dashboard.
4. Check the sidebar for all monitoring pages (Dashboard, Databases, Query Insights, Workload Report, Advisor, etc.).

If something isn't working:

- **"No instances found"** — Check your config file. Each instance needs an `id` and a `dsn`.
- **Connection errors** — Verify network connectivity, `pg_hba.conf`, and credentials. PGPulse logs errors to the console (run from a terminal to see them).
- **"No issues found" in Advisor** — The advisor needs 1–2 collection cycles to populate. Wait 2–5 minutes after startup.
- **Query Insights shows empty** — Statement snapshots need at least 2 captures to compute a diff. If your interval is 30 minutes, wait 60 minutes for the first useful data.
