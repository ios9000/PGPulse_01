# PGPulse — Iteration Handoff: M8 → M8_11

---

## DO NOT RE-DISCUSS

- `internal/alert` must NEVER import `internal/ml` — use `alert.ForecastProvider` interface
- `internal/mlerrors` is the canonical home for `ErrNotBootstrapped` and `ErrNoBaseline`
- `ForecastPoint` in `internal/alert/forecast.go` is intentionally a thin mirror (4 fields only)
- Sustained crossing (N consecutive) is the only supported mode for forecast alerts
- Forecast polling in the frontend is 5 minutes
- `TimeSeriesChart.tsx` accepts `extraSeries`, `xAxisMax`, `nowMarkLine` props — use these for all forecast integrations
- The `forecastUtils.ts` file in `web/src/lib/` is the canonical home for `buildForecastSeries` and `getNowMarkLine` (moved from `ForecastBand.ts` in M8_09 to break circular import)
- The TDZ crash was caused by `echartseries.length` referencing itself inside its own `.map()` — fixed by using `series.length` (input array)
- YAML seeds the database on startup; database becomes source of truth after first run
- `go:embed` bakes the React build into the Go binary — PGPulse is its own web server, no Nginx/Apache needed
- PGPulse listens on port 8989 on the demo VM (not 8080 — that's occupied)
- Test scope must be `./cmd/... ./internal/...` (not `./...`) to prevent scanning `web/node_modules/`

---

## What Was Just Completed (M8_06 through M8_10)

### M8_06 — UI Catch-Up
- Session kill UI (ConfirmModal + SessionActions with role gating)
- Per-instance settings diff accordion with CSV export
- Query plan viewer with recursive tree and cost highlighting
- Forecast overlay extended to 4 charts via `useForecastChart` helper hook
- Toast notification system (Toast.tsx + toastStore.ts)

### M8_07 — Deferred UI + Small Fixes
- Plan capture history UI (All Plans / Regressions tabs, PlanNode reuse, manual capture)
- Temporal settings timeline UI (snapshot compare, colour-coded diffs, "Take Snapshot")
- `application_name` enrichment on long transactions API (enables pgpulse_* filter in SessionActions)
- Administration.tsx lint fix — **0 lint errors achieved** project-wide
- JSON tags added to `capture.go` for proper API serialization

### M8_08 — Logical Replication Monitoring
- PGAM Q41 ported via DBRunner per-database framework
- `collectLogicalReplication` sub-collector (17th DB sub-collector)
- API: `GET /instances/{id}/logical-replication` with per-DB connections via SubstituteDatabase
- Frontend: LogicalReplicationSection with 4 UI states, sync state badges
- Alert rule: `logical_repl_pending_sync` (disabled by default)
- Query porting: ~70/76 PGAM queries ported

### M8_09 — Hotfix: Production Crash
- **Circular import crash** (`Cannot access 'h' before initialization`): moved `buildForecastSeries` to `lib/forecastUtils.ts`, deleted dead `ForecastBand.ts`
- **TDZ crash** in TimeSeriesChart: `echartseries.length` → `series.length`
- CSP updated for Google Fonts
- `stanullfrac` → `null_frac` in bloat query
- `received_lsn` → `flushed_lsn` in WAL receiver query
- sequences `pct_used` COALESCE for NULL
- `server.port` config wired (Port field in ServerConfig)

### M8_10 — Hotfix: Remaining Production Bugs
- **Explain handler recreated** (`internal/api/explain.go`) — was deleted in M8_02, never restored
- `ConnForDB(ctx, instanceID, dbName)` added to `InstanceConnProvider` interface + orchestrator
- Breadcrumb "Servers" → redirects to `/fleet`
- `client_addr::text` cast in replication query (inet scan fix)
- `c.command_desc` → `c.command` in progress query (PG 16)
- `COALESCE(sa.datname, '')` in lock tree query (NULL scan fix)

### Manual Deployment Fixes (applied on VM, not in code)
- `pg_largeobject` SELECT grant for monitoring user
- bloat query `GROUP BY` missing `bitlength` — fixed in code
- bloat query `bs` column not passed through `sml` subquery — fixed in code
- `db.` metric prefix → `pgpulse.db.` in databases handler — fixed in code
- `initial_admin` config key (not `seed_admin`)
- `GRANT SELECT ON ALL TABLES IN SCHEMA public TO pgpulse_monitor` for EXPLAIN on user tables

---

## Demo Environment (Live)

```
Ubuntu 24.04 VM: 185.159.111.139

PGPulse UI:     http://185.159.111.139:8989
Login:          admin / pgpulse_admin

PostgreSQL 16.13:
  Primary:      localhost:5432  (production, streaming source)
  Replica:      localhost:5433  (streaming replica)
  Chaos:        localhost:5434  (standalone, chaos target)

Replication:
  Physical:     5432 → 5433 (streaming)
  Logical:      5432.demo_app.demo_orders → 5434.demo_app.demo_orders

Monitor user:   pgpulse_monitor / pgpulse_monitor_demo
TimescaleDB:    installed, hypertable on metrics.time
pg_stat_statements: installed on all instances

Chaos scripts:  /opt/pgpulse/chaos/*.sh (run with: sudo bash /opt/pgpulse/chaos/<script>.sh)
PGPulse logs:   sudo journalctl -u pgpulse -f
Config:         /opt/pgpulse/configs/pgpulse.yml
```

---

## Next Task: M8_11 — OS Metrics via PostgreSQL (Strategy Shift)

### Background: Why This Matters

The current PGPulse architecture has two OS metrics paths:
1. **pgpulse-agent binary** (`cmd/pgpulse-agent/`, `internal/agent/`) — standalone Linux process that reads `/proc` and `/sys`, reports via HTTP. Provides comprehensive OS metrics.
2. **No built-in SQL-based OS collection** — if the agent isn't deployed, PGPulse has zero OS visibility.

### The Problem

Information Security, DBAs, and Linux sysadmins resist deploying an unfamiliar agent binary across every database server. This is a legitimate concern:
- Unknown binaries on production servers require security review
- Agent requires separate deployment, monitoring, updates
- Firewall rules needed for agent → PGPulse communication
- Each server needs agent installation and configuration

### The Strategic Shift

**Default method (out-of-the-box):** Collect OS metrics through PostgreSQL itself using the same SQL connection PGPulse already has. No additional software needed. PGAM did this via `COPY TO PROGRAM` (which required superuser and was a security nightmare). PGPulse must find safer alternatives.

**Optional enhancement:** The pgpulse-agent remains available for teams willing to deploy it. It provides a broader range of metrics that can't be obtained through SQL.

### What PGAM Collected via SQL (from PGAM_FEATURE_AUDIT.md)

| PGAM Query # | Metric | PGAM Method | PGPulse Alternative |
|---|---|---|---|
| Q4 | Hostname | `COPY TO PROGRAM 'hostname -f'` + `pg_read_file()` | `inet_server_addr()`, `pg_read_file('/etc/hostname')` (PG 14+ with pg_read_server_files role) |
| Q5 | OS distribution | `pg_stat_file('/etc/lsb-release')` + `pg_read_file()` | `pg_read_file('/etc/os-release')` with pg_read_server_files |
| Q8 | OS uptime | `COPY TO PROGRAM 'uptime'` | `pg_read_file('/proc/uptime')` with pg_read_server_files |
| Q9 | OS system time | `COPY TO PROGRAM 'date'` | `clock_timestamp()` (PG built-in) |
| Q12 | Total RAM | `COPY TO PROGRAM 'free -m'` | `pg_read_file('/proc/meminfo')` with pg_read_server_files |
| Q22 | Memory overcommit | `COPY TO PROGRAM 'cat /proc/meminfo'` | `pg_read_file('/proc/meminfo')` with pg_read_server_files |
| Q23 | Full meminfo | `COPY TO PROGRAM 'grep /proc/meminfo && free -h'` | `pg_read_file('/proc/meminfo')` |
| Q24 | OS top summary | `COPY TO PROGRAM 'top -c -b -n 1'` | **Cannot replicate via SQL** — needs agent |
| Q25 | Filesystem usage | `COPY TO PROGRAM 'df -Tih'` | `pg_tablespace` sizes + data directory via `pg_stat_file()` — partial |
| Q26-27 | I/O stats | `COPY TO PROGRAM 'iostat'` | **Cannot replicate via SQL** — needs agent |

### Key Design Principle

`pg_read_file()` with the `pg_read_server_files` role can read `/proc/*` on Linux. This gives us:
- `/proc/meminfo` — total RAM, available, buffers, cached, swap
- `/proc/uptime` — OS uptime in seconds
- `/proc/loadavg` — 1/5/15 min load averages
- `/proc/stat` — CPU time breakdown (user, system, idle, iowait)
- `/proc/diskstats` — disk I/O counters
- `/etc/os-release` — OS distribution info
- `/etc/hostname` — server hostname

**What we CANNOT get via SQL:** process lists (top), full filesystem layout (df), iostat formatted output, network interfaces. These remain agent-only.

### Requirements

1. New collector: `internal/collector/os_sql.go` — OS metrics via `pg_read_file('/proc/*')`
2. Requires `pg_read_server_files` role membership for the monitoring user
3. Graceful fallback: if `pg_read_file('/proc/meminfo')` fails (permission denied, Windows, etc.), log warning and skip — don't fail the collection cycle
4. Parse `/proc/meminfo`, `/proc/uptime`, `/proc/loadavg`, `/proc/stat`, `/proc/diskstats` in Go
5. Produce the same metric keys as the existing agent (`os.memory.total_kb`, `os.cpu.user_pct`, etc.) so the frontend OS sections work without changes
6. Configuration: `os_metrics.method: "sql"` (default) or `"agent"` — determines which path is used
7. Documentation: deployment guide should mention the `pg_read_server_files` grant as the recommended setup, with agent as optional enhancement

### Implementation Approach

The collector runs per-instance (not per-database). It's in the same collection cycle as other instance-level collectors. Each `/proc` file is read via a single `SELECT pg_read_file(...)` call and parsed in Go.

Example for `/proc/meminfo`:
```go
var raw string
err := conn.QueryRow(ctx, "SELECT pg_read_file('/proc/meminfo')").Scan(&raw)
if err != nil {
    // Permission denied or file not found — skip gracefully
    return nil, nil
}
// Parse: "MemTotal: 16384000 kB\nMemFree: 8192000 kB\n..."
```

### Grant Required on Each Monitored Instance
```sql
GRANT pg_read_server_files TO pgpulse_monitor;
```

### What Stays the Same
- `cmd/pgpulse-agent/` and `internal/agent/` remain untouched
- Frontend OS sections (OSSystemSection, DiskSection, etc.) remain untouched
- Metric key names stay the same (`os.memory.*`, `os.cpu.*`, `os.load.*`, etc.)
- Agent is still the recommended method for comprehensive OS monitoring

---

## Known Issues (Non-Blocking)

| Issue | Status |
|-------|--------|
| Some `/proc` files may not be readable if PG runs in container | Handled by graceful fallback |
| `pg_read_file` returns text — CPU/disk metrics need delta calculation between cycles | Same pattern as agent collectors |
| Remaining ~14 PGAM queries unported (VTB-internal) | Low priority |

---

## Key Interfaces (Current)

```go
// internal/collector/collector.go
type Collector interface {
    Name() string
    Collect(ctx context.Context, conn *pgx.Conn, ic InstanceContext) ([]MetricPoint, error)
    Interval() time.Duration
}

// internal/api/connprovider.go (updated in M8_10)
type InstanceConnProvider interface {
    ConnFor(ctx context.Context, instanceID string) (*pgx.Conn, error)
    ConnForDB(ctx context.Context, instanceID, dbName string) (*pgx.Conn, error)
}
```

---

## Workflow Reminder

```bash
# Build verification
cd web && npm run build && npm run typecheck && npm run lint
cd .. && go build ./cmd/pgpulse-server && go test ./cmd/... ./internal/...
golangci-lint run

# Cross-compile and deploy
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o build/pgpulse-server ./cmd/pgpulse-server
scp build/pgpulse-server ml4dbs@185.159.111.139:/home/ml4dbs/pgpulse-demo/

# On VM:
sudo systemctl stop pgpulse
sudo cp /home/ml4dbs/pgpulse-demo/pgpulse-server /opt/pgpulse/bin/pgpulse-server
sudo chmod +x /opt/pgpulse/bin/pgpulse-server
sudo systemctl start pgpulse
```
