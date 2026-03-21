# PGPulse Desktop for Windows

PGPulse Desktop is a native Windows application for monitoring PostgreSQL fleets. It wraps the full PGPulse monitoring platform — real-time dashboards, intelligent alerting, ML-based anomaly detection, and workload analysis — in a native desktop window with system tray integration and OS-level alert notifications.

## What You Get

**Native desktop experience** — PGPulse runs as a proper Windows application with a system tray icon, not as a browser tab pointing at localhost. Close the window and it minimizes to tray. Alerts pop up as Windows Toast notifications.

**Two ways to start** — Open an existing config file to monitor your full PostgreSQL fleet with persistent history, or quick-connect to a single instance with just a DSN for instant diagnostics.

**Everything in one binary** — No database to install, no Docker containers, no browser. One `.exe` contains the monitoring backend, the React dashboard, and the native window shell. The NSIS installer puts it in Program Files with desktop and Start Menu shortcuts.

**Same platform, different packaging** — PGPulse Desktop uses the exact same monitoring engine, collectors, alert rules, and UI as the server edition. If you already run PGPulse on a Linux server, the desktop app gives you the same experience locally on Windows.

## Key Features

- **Real-time dashboards** — connections, cache hit ratio, replication lag, WAL, wait events, lock trees, active queries
- **13 monitoring pages** — fleet overview, instance detail, per-database analysis, query insights, workload reports, advisor recommendations
- **27 metric collectors** — 220+ metric keys covering server health, replication, statements, I/O, OS, and cluster state
- **Intelligent alerting** — threshold rules with hysteresis, cooldown, forecast-based alerts, Windows Toast notifications
- **ML anomaly detection** — STL seasonal decomposition with Z-score anomaly scoring and confidence-band forecasting
- **Query analysis** — pg_stat_statements snapshots, diff engine, per-query time-series, workload reports with HTML export
- **System tray** — color-coded icon (green/yellow/red) showing fleet health at a glance
- **PostgreSQL 14–18** — version-adaptive SQL collectors that handle API changes automatically

## System Requirements

- Windows 10 or 11 (64-bit)
- WebView2 Runtime (pre-installed on Windows 10 1803+ and all Windows 11)
- PostgreSQL 14, 15, 16, 17, or 18 (monitored instances)
- `pg_stat_statements` extension enabled on monitored instances (recommended)

## Quick Start

**Option A — Installer:**
1. Run `PGPulse-Setup.exe`
2. Launch PGPulse from the desktop shortcut
3. In the connection dialog, enter your PostgreSQL DSN or browse for a config file

**Option B — Portable:**
1. Download `pgpulse-desktop.exe`
2. Double-click to launch
3. Enter a DSN in the quick-connect dialog

See the [Setup Guide](SETUP_GUIDE.md) for detailed installation instructions and the [User Guide](USER_GUIDE.md) for a walkthrough of all features.

## Monitoring Modes

| Mode | Storage | History | Use Case |
|------|---------|---------|----------|
| **Persistent** | PostgreSQL + TimescaleDB | 30 days (configurable) | Production fleet monitoring |
| **Quick Connect** | In-memory ring buffer | Current session only | Quick diagnostics, troubleshooting |

In persistent mode, PGPulse stores metrics in a dedicated PostgreSQL database with TimescaleDB hypertables, enabling historical charts, trend analysis, and ML forecasting. In quick-connect mode, it monitors a single instance with no external dependencies — useful for one-off diagnostics.

## Building from Source

```bash
# Prerequisites: Go 1.25+, Node.js 22+, NSIS 3.x (for installer)

# Build the frontend
cd web && npm install && npm run build && cd ..

# Build desktop binary (GUI subsystem — no console window)
go build -tags desktop -ldflags="-s -w -H windowsgui" -o pgpulse-desktop.exe ./cmd/pgpulse-server

# Build NSIS installer (optional)
cp pgpulse-desktop.exe deploy/nsis/
makensis deploy/nsis/pgpulse.nsi
```

The standard server binary (no desktop features) is also available:

```bash
go build -ldflags="-s -w" -o pgpulse-server.exe ./cmd/pgpulse-server
```

## License

MIT
