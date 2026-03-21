# PGPulse Desktop — Setting Up Email Alerts

This guide covers configuring PGPulse Desktop to send email notifications when PostgreSQL issues are detected. Email alerts are the most common request from production teams — especially for replication lag, connection saturation, and wraparound risk.

## Prerequisites

Email alerting requires **persistent mode**. This means you need a PostgreSQL database for PGPulse's own storage — this is where alert rules, alert history, and metric time-series are kept.

| Mode | Email Alerts | Why |
|------|-------------|-----|
| Quick connect (live mode) | Not available | No storage for alert rules or history |
| Persistent mode (with `storage.dsn`) | Full support | Rules seeded on startup, evaluator runs every cycle |

The storage database can be on the same PostgreSQL server you're monitoring, on a separate instance, or even on your local Windows machine if you have PostgreSQL installed. TimescaleDB is recommended for better metric retention performance but is not required.

## Step 1 — Create the Storage Database

On any accessible PostgreSQL instance:

```sql
CREATE DATABASE pgpulse_storage;

-- Optional: install TimescaleDB for better time-series performance
\c pgpulse_storage
CREATE EXTENSION IF NOT EXISTS timescaledb;
```

PGPulse creates its tables automatically on first startup — no manual schema setup needed.

## Step 2 — Get SMTP Details

You need the following from your email administrator:

| Setting | Example | Notes |
|---------|---------|-------|
| SMTP host | `smtp.company.com` | Your mail server |
| SMTP port | `587` | 587 for STARTTLS, 465 for SSL, 25 for unencrypted |
| Username | `alerts@company.com` | SMTP authentication username |
| Password | `smtp_password` | SMTP authentication password |
| From address | `pgpulse@company.com` | Sender address shown in emails |
| Recipients | `dba-team@company.com` | Comma-separated list in config |

If your organization uses Gmail, Office 365, or similar:

- **Gmail:** host `smtp.gmail.com`, port `587`, use an App Password (not your regular password)
- **Office 365:** host `smtp.office365.com`, port `587`
- **Amazon SES:** host `email-smtp.us-east-1.amazonaws.com`, port `587`, use IAM SMTP credentials

## Step 3 — Configure PGPulse

Edit your `pgpulse.yml` (or create a new one):

```yaml
server:
  listen: ":8989"
  log_level: info

# Storage database — REQUIRED for email alerts
storage:
  dsn: "postgresql://pgpulse_admin:storage_password@storage-host:5432/pgpulse_storage?sslmode=disable"
  retention_days: 30

# Authentication — recommended for production
auth:
  enabled: true
  jwt_secret: "your-secret-key-at-least-32-characters-long"
  initial_admin:
    username: admin
    password: changeme

# Alerting — this enables the alert engine and email delivery
alerting:
  enabled: true
  default_consecutive_count: 3      # alert fires after 3 consecutive violations
  default_cooldown_minutes: 15      # don't re-send same alert within 15 minutes
  default_channels:
    - email                          # send alerts via email by default
  dashboard_url: "http://pgpulse-host:8989"   # link in email body (optional)
  email:
    host: smtp.company.com
    port: 587
    username: alerts@company.com
    password: smtp_password
    from: pgpulse@company.com
    recipients:
      - dba-team@company.com
      - oncall@company.com
    tls_skip_verify: false           # set true only for self-signed SMTP certs
    send_timeout_seconds: 10

# Monitored instances
instances:
  - id: primary
    name: "Production Primary"
    dsn: "postgresql://pgpulse_monitor:monitor_password@db-primary:5432/postgres?sslmode=disable"

  - id: replica-1
    name: "Read Replica 1"
    dsn: "postgresql://pgpulse_monitor:monitor_password@db-replica:5432/postgres?sslmode=disable"
```

## Step 4 — Launch and Verify

Launch PGPulse Desktop with your config:

```
pgpulse.exe --mode=desktop --config=pgpulse.yml
```

Or if using the installer, edit `C:\Program Files\PGPulse\pgpulse.yml` and relaunch from the Start Menu.

### Test the Email Connection

Once PGPulse is running, send a test email through the API. Open a browser or use curl:

```
POST http://localhost:8989/api/v1/alerts/test
Content-Type: application/json
Authorization: Bearer <your-token>

{
  "channel": "email",
  "recipient": "your-email@company.com"
}
```

Or from the PGPulse UI: navigate to **Alerts** in the sidebar. If SMTP is configured correctly, you'll see the alert rules listed and active alerts when thresholds are breached.

### Check the Logs

In the console output, look for:

- `msg="alert rule seeded"` — built-in rules loaded on startup (good)
- `msg="alert fired"` — a rule threshold was breached
- `msg="notification sent" channel=email` — email delivered
- `msg="notification failed" channel=email` — SMTP error (check credentials/host)

## Built-In Alert Rules

PGPulse ships with these rules pre-configured. They are automatically seeded into the storage database on first startup:

### Replication Alerts

| Rule | Threshold | Severity | What It Catches |
|------|-----------|----------|-----------------|
| Replication lag > 1 MB | 1 MB | WARNING | Replica falling behind |
| Replication lag > 100 MB | 100 MB | CRITICAL | Replica severely behind, risk of slot overflow |
| Inactive replication slot | any inactive | WARNING | Slot not consumed, WAL retention growing |

### Connection Alerts

| Rule | Threshold | Severity | What It Catches |
|------|-----------|----------|-----------------|
| Connection utilization > 80% | 80% | WARNING | Approaching max_connections |
| Connection utilization ≥ 99% | 99% | CRITICAL | Near connection exhaustion |

### Performance Alerts

| Rule | Threshold | Severity | What It Catches |
|------|-----------|----------|-----------------|
| Cache hit ratio < 90% | 90% | WARNING | Excessive disk reads, shared_buffers may be too small |
| Commit ratio < 90% | 90% | WARNING | High rollback rate |
| Long transactions > 5 min | 5 min | CRITICAL | Transaction holding locks/preventing vacuum |

### Maintenance Alerts

| Rule | Threshold | Severity | What It Catches |
|------|-----------|----------|-----------------|
| Wraparound > 20% | 20% | WARNING | Transaction ID approaching wraparound |
| Wraparound > 50% | 50% | CRITICAL | Urgent vacuum needed |
| pg_stat_statements fill ≥ 95% | 95% | WARNING | Statement tracking nearly full, older entries being evicted |

All rules use **hysteresis** — they only fire after 3 consecutive threshold violations (configurable via `default_consecutive_count`). This prevents flapping on momentary spikes.

All rules use **cooldown** — after firing, the same rule won't send another email for 15 minutes (configurable via `default_cooldown_minutes`). This prevents inbox flooding during sustained issues.

## Customizing Alert Rules

### Through the UI

Navigate to **Alerts → Rules** in the sidebar. You can:

- Enable or disable any rule
- Adjust thresholds (e.g., change replication lag warning from 1 MB to 10 MB)
- Change severity levels
- Modify hysteresis count and cooldown duration

Changes take effect on the next evaluation cycle (within seconds).

### Creating Custom Rules

Click "Create Rule" in the Alerts page to define new rules for any metric PGPulse collects (220+ metrics available). Specify the metric key, operator, threshold, severity, and notification channels.

Example custom rule: alert when `pg.connections.utilization_pct > 70` with severity WARNING — a more conservative threshold than the default 80%.

## What the Email Looks Like

Alert emails include:

- **Subject:** `[PGPulse] CRITICAL: Replication lag > 100 MB on Production Primary`
- **Body:** Instance name, alert rule name, current metric value, threshold, severity, timestamp, and a direct link to the instance dashboard (if `dashboard_url` is configured)

## Troubleshooting

### "Alerts not firing"

- Verify `alerting.enabled: true` in your config
- Verify `storage.dsn` is set and PGPulse can connect to it
- Check logs for `msg="alert rule seeded"` on startup
- Wait 2–3 collection cycles (30 seconds) for hysteresis to complete
- Verify the monitored metric actually exceeds the threshold

### "Alert fires but no email"

- Check `default_channels` includes `email`
- Verify SMTP settings: host, port, credentials
- Check logs for `msg="notification failed"` with the error detail
- Test with the `/api/v1/alerts/test` endpoint
- Check spam folder — first emails from a new sender often land there

### "Too many emails"

- Increase `default_cooldown_minutes` (e.g., from 15 to 60)
- Increase `default_consecutive_count` (e.g., from 3 to 5)
- Disable rules you don't need through the UI
- Adjust thresholds to match your environment's normal baseline

### "Emails stopped after replication was fixed"

This is expected. PGPulse automatically resolves alerts when the metric returns below the threshold. No manual acknowledgment needed — the alert history is preserved in the Alerts page.
