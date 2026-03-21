# PGPulse Desktop — User Guide

This guide walks through the features of PGPulse Desktop. It assumes you've completed installation and have PGPulse running with at least one monitored PostgreSQL instance (see the [Setup Guide](SETUP_GUIDE.md) if not).

## 1. The Desktop Shell

### System Tray

PGPulse lives in the Windows system tray (notification area, bottom-right of the taskbar). The tray icon shows the overall health of your fleet:

| Icon Color | Meaning |
|-----------|---------|
| Green | All instances healthy — no active alerts |
| Yellow | One or more WARNING-level alerts active |
| Red | One or more CRITICAL-level alerts active |

**Left-click** the tray icon to show or hide the main window. **Right-click** for a context menu with Show, status summary, and Quit.

### Window Behavior

Closing the PGPulse window does not stop monitoring — it minimizes to the system tray. Monitoring, alerting, and metric collection continue in the background. To fully quit PGPulse, right-click the tray icon and select "Quit".

### Toast Notifications

When an alert fires (WARNING or CRITICAL severity), PGPulse sends a Windows Toast notification. The toast shows the instance name, alert rule, and current metric value. Click the notification to bring the PGPulse window to the foreground.

Notifications are rate-limited to one per alert rule every 5 minutes to prevent notification spam during sustained issues.

## 2. Fleet Overview

The landing page shows all monitored instances as cards. Each card displays the instance name, connection status, and key metrics (connection utilization, cache hit ratio, replication lag where applicable).

Click any instance card to open its detailed dashboard.

If authentication is enabled, you'll see a login page first. The default credentials (if you haven't changed them) are in your config file under `auth.initial_admin`.

## 3. Instance Dashboard

The instance dashboard is the primary monitoring view. It's organized into sections that you can scroll through:

### Header

Shows the instance name, PostgreSQL version, hostname, uptime, and current status (primary or replica). The header also displays active alert count and a quick-action button to run diagnostics.

### Key Metrics Row

Four summary cards at the top: connection utilization percentage, buffer cache hit ratio, transaction commit ratio, and replication lag (for replicas). These update in real time at the high-frequency collection interval (default 10 seconds).

### Time-Series Charts

Interactive ECharts showing metric history for connections, cache hit ratio, transactions, and replication lag. Hover for exact values, click-drag to zoom into a time range, double-click to reset zoom. In persistent mode, charts show up to 30 days of history. In live mode, charts show data since the current session started.

### Diagnose Panel

Click "Run Diagnostics" to evaluate all advisor rules against the current instance. Results appear as a list of recommendations sorted by priority, each with a category, description, and documentation link. Common findings include unused indexes, high connection utilization, disabled track_io_timing, and approaching wraparound thresholds.

### Replication

For primary servers: shows connected replicas, WAL lag per replica (bytes and seconds), and replication slot status. For replicas: shows WAL receiver connection state and lag from primary.

### Logical Replication

Tables pending synchronization for logical replication subscriptions, grouped by subscription name.

### Wait Events

Breakdown of current wait events by type (CPU, IO, Lock, IPC, etc.) from `pg_stat_activity`. Useful for identifying what backends are waiting on.

### Long Transactions

Active transactions running longer than 1 minute, showing duration, query text, state, and user. Includes buttons to cancel or terminate sessions (requires appropriate RBAC permissions).

### Statements

Top queries by total execution time from `pg_stat_statements`, showing calls, mean time, rows, and I/O metrics. Click any query to see its full text or run EXPLAIN.

### Lock Tree

Hierarchical view of blocking lock chains. Shows which sessions are blocking others, with query text, wait event, and duration for each node. Helpful for diagnosing lock contention.

### Progress Monitoring

Active maintenance operations with completion percentage: VACUUM, ANALYZE, CREATE INDEX, CLUSTER, BASEBACKUP, and COPY.

### Active Alerts

List of currently unresolved alerts for this instance with severity, rule name, metric value, and when the alert fired.

### OS Metrics

CPU utilization, memory usage, disk I/O, and load averages. Available when OS metrics collection is enabled (default: SQL method via `pg_read_file`). Shows CPU user/system/iowait breakdown, memory available vs. total, and per-device disk throughput.

### Cluster Status

Patroni cluster state and ETCD health (if configured). Shows cluster members, roles, lag, and timeline information.

### Settings Timeline

Visual timeline of PostgreSQL configuration changes. Each dot represents a settings snapshot. Click to see a diff of what changed between snapshots. Useful for tracking down when a parameter was modified.

### Plan History

Captured query execution plans with regression indicators. Plans are captured automatically for slow queries (if plan capture is enabled) or on demand via the EXPLAIN page.

## 4. Per-Database Analysis

Navigate to any instance, then click "Databases" in the sidebar. Select a database to see detailed per-database metrics including table sizes and bloat, index usage and unused indexes, vacuum health (dead tuples, last vacuum/analyze times), sequence utilization, TOAST sizes, and function call statistics.

The bloat analysis shows table and index bloat ratios with estimated wasted bytes. High bloat ratios (shown in red) indicate tables that would benefit from VACUUM FULL or pg_repack.

## 5. Query Insights

Found in the sidebar under each instance. Query Insights uses `pg_stat_statements` snapshots to show how query workload changes over time.

### Snapshot Diffs

The main view shows a sortable table of queries with their delta metrics between two snapshots: change in calls, execution time, rows returned, shared blocks read, and more. You can select any two snapshots to compare using the snapshot selector dropdowns.

### Per-Query Drill-Down

Click any query in the diff table to see four mini time-series charts: total execution time, calls, average execution time, and I/O read ratio over all captured snapshots. This helps identify query regressions — a query that suddenly takes 10x longer will show a clear spike.

### Capture Now

Click "Capture Now" to take an immediate `pg_stat_statements` snapshot. Useful before and after a deployment to measure query impact.

### Stats Reset Warning

If `pg_stat_statements` counters were reset between two snapshots (via `pg_stat_statements_reset()`), a banner warns that the diff may be misleading.

## 6. Workload Report

The Workload Report provides a summary of database workload between two snapshots. It's found in the sidebar under each instance.

### Summary Card

Total queries analyzed, total execution time, time span covered, and average queries per second.

### Top-N Sections

Five ranked lists: top queries by total execution time, by call count, by rows returned, by I/O reads, and by average execution time. Each section is collapsible.

### New and Evicted Queries

Queries that appeared for the first time in the newer snapshot (new deployments, ad-hoc queries) and queries that disappeared (evicted from `pg_stat_statements` due to reaching `pg_stat_statements.max`).

### HTML Export

Click "Export HTML" to download a standalone HTML report with inline CSS that opens in any browser. Useful for sharing with team members or archiving before a deployment.

## 7. Advisor (Recommendations)

The advisor automatically evaluates your PostgreSQL instances against a set of built-in remediation rules. Navigate to the Advisor page from the sidebar.

### How It Works

A background evaluator runs periodically (default every 5 minutes) and checks each instance against rules covering connection management, cache efficiency, replication health, vacuum status, wraparound risk, bloat, statement statistics configuration, and OS resource utilization.

### Reading Recommendations

Each recommendation has a priority (critical, high, medium, low, info), category, title, description with the current metric value, and a documentation link. Recommendations can be acknowledged to track that they've been reviewed.

### Alert Integration

Recommendations triggered by active alert events include a link to the originating alert. The advisor works alongside the alert engine — alerts notify you in real time, while the advisor provides a structured list of all current issues with remediation guidance.

## 8. Alerting

### Built-In Alert Rules

PGPulse ships with pre-configured alert rules:

| Rule | Default Threshold | Severity |
|------|-------------------|----------|
| Connection utilization > 80% | 80% | WARNING |
| Connection utilization ≥ 99% | 99% | CRITICAL |
| Cache hit ratio < 90% | 90% | WARNING |
| Commit ratio < 90% | 90% | WARNING |
| Replication lag > 1 MB | 1 MB | WARNING |
| Replication lag > 100 MB | 100 MB | CRITICAL |
| Inactive replication slots | any inactive | WARNING |
| Long transactions > 5 min | 5 min | CRITICAL |
| Wraparound > 20% | 20% | WARNING |
| Wraparound > 50% | 50% | CRITICAL |
| pg_stat_statements fill ≥ 95% | 95% | WARNING |

All rules use hysteresis (default 3 consecutive violations before firing) and cooldown (default 15 minutes between repeated alerts) to reduce noise.

### Managing Rules

Navigate to the Alerts page to see active alerts, alert history, and configured rules. Rules can be created, modified, and disabled through the UI (requires alert_management permission).

### Notification Channels

PGPulse supports email notifications (SMTP) and Windows Toast notifications (desktop mode only). Configure email settings in the `alerting.email` section of your config file.

### Forecast Alerts

When ML is enabled, PGPulse can alert on predicted future issues — for example, "disk space will reach threshold within 7 days based on current growth trend." Forecast alerts use the same hysteresis logic as standard alerts but evaluate predicted values instead of current ones.

## 9. ML Anomaly Detection

When enabled (`ml.enabled: true`), PGPulse builds per-metric baselines using Seasonal-Trend decomposition (STL) and flags anomalies using Z-score analysis.

### What It Detects

Anomalies appear when a metric deviates significantly from its expected seasonal pattern. For example, connection count is normally 50 during business hours and 5 at night. If connections suddenly spike to 200 during a normally quiet period, that's flagged as anomalous even though 200 connections might be normal during peak hours.

### Forecasting

The ML engine projects metric trends forward with confidence bands. These forecasts power the time-series chart projections and forecast-based alerts. Forecasts require sufficient historical data (at least one full seasonal period, typically 24 hours).

### Configuration

```yaml
ml:
  enabled: true
  zscore_threshold_warning: 2.0    # ~5% of normal values exceed this
  zscore_threshold_critical: 3.0   # ~0.3% of normal values exceed this
  collection_interval: 60s
  forecast:
    horizon: 60                    # forecast 60 data points ahead
    confidence_z: 1.96             # 95% confidence interval
```

## 10. Settings History

PGPulse captures snapshots of your PostgreSQL configuration (`pg_settings`) and tracks changes over time. Navigate to any instance and look for the Settings Timeline in the dashboard.

### What You See

A visual timeline showing when configuration changes occurred. Click any point to see a diff: which parameters changed, old values, new values, and whether a restart is required.

### Pending Restart

The settings page also shows parameters that have been changed via `ALTER SYSTEM` but require a PostgreSQL restart to take effect.

## 11. Keyboard Shortcuts and Tips

- **Ctrl+Shift+D** — Open WebView2 Developer Tools (debug builds only)
- **Escape** — Close modal dialogs
- **Click-drag on charts** — Zoom into a time range
- **Double-click on charts** — Reset zoom
- **Scroll down** on the instance dashboard to see all sections — the page is long by design, providing a comprehensive single-page view of instance health

## 12. Troubleshooting

### PGPulse window doesn't open

Ensure WebView2 Runtime is installed. On Windows 10, check via Settings → Apps → Microsoft Edge WebView2 Runtime. If missing, download from [Microsoft](https://developer.microsoft.com/en-us/microsoft-edge/webview2/).

### "Connection refused" for an instance

Check that the PostgreSQL instance is running, the DSN in your config is correct, and `pg_hba.conf` allows connections from your machine. Test with `psql` first.

### Metrics show "N/A" or are missing

Some metrics require specific extensions or configurations. `pg_stat_statements` must be in `shared_preload_libraries` for query metrics. OS metrics require `pg_read_server_files` role (or the pgpulse-agent for non-SQL collection). After first startup, wait 1–2 collection cycles for metrics to populate.

### Advisor says "No issues found"

The advisor evaluates metrics from the MetricStore, which needs time to warm up after startup. Wait 2–5 minutes, then run diagnostics again. In live mode, some advisor rules that depend on historical data may not have enough data to evaluate.

### High memory usage

Each monitored instance uses a connection pool (default 5 connections). If monitoring many instances, reduce `max_conns` per instance in the config, or increase collection intervals.

### Tray icon stays green despite issues

The tray status polling runs every 10 seconds. If alerts fire and resolve within that window, the icon may not change. The tray icon reflects the current alert state, not historical alerts.

### Toast notifications don't appear

Windows Toast notifications require the application to have an AppUserModelID registered (handled by the NSIS installer). In portable mode, notifications may not work on all Windows configurations. Check Windows notification settings (Settings → System → Notifications) to ensure PGPulse is allowed to send notifications.
