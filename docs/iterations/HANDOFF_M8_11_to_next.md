# PGPulse — Iteration Handoff: M8_11 → Next

**Date:** 2026-03-11
**From:** M8_11 (OS Metrics via PostgreSQL) + M8_11b (OS Metrics Frontend UI)
**To:** Competitive Research + M6 OS Agent (or Windows executable)

---

## DO NOT RE-DISCUSS

- `internal/alert` must NEVER import `internal/ml` — use `alert.ForecastProvider` interface
- `internal/mlerrors` is the canonical home for `ErrNotBootstrapped` and `ErrNoBaseline`
- Sustained crossing (N consecutive) is the only supported mode for forecast alerts
- Forecast polling in the frontend is 5 minutes
- `forecastUtils.ts` in `web/src/lib/` is canonical for `buildForecastSeries` and `getNowMarkLine`
- YAML seeds the database on startup; database becomes source of truth after first run
- `go:embed` bakes the React build into the Go binary — PGPulse is its own web server
- PGPulse listens on port 8989 on the demo VM
- Test scope must be `./cmd/... ./internal/...` (not `./...`) to prevent scanning `web/node_modules/`
- OSSQLCollector reuses agent parsers (ParseMeminfo, ParseCPURaw, ParseDiskStats) from `internal/agent/` — no code duplication
- Per-instance `os_metrics_method` config: "sql" (default), "agent", "disabled"
- Metric naming standardization is parked for competitive research — do NOT rename keys until then
- `docs/CODEBASE_DIGEST.md` is auto-generated at end of each iteration — always re-upload to Project Knowledge

---

## What Was Just Completed

### M8_11 — OS Metrics via PostgreSQL
- `internal/collector/os_sql.go` (~230 lines) — reads `/proc/meminfo`, `/proc/uptime`, `/proc/loadavg`, `/proc/stat`, `/proc/diskstats` via `pg_read_file()`
- Stateful delta calculations for CPU and disk I/O (same pattern as `checkpoint.go`)
- Graceful per-file fallback — each file read fails independently
- `internal/collector/server_info.go` enriched with hostname (`/etc/hostname`) and OS release (`/etc/os-release`)
- `internal/config/config.go` — added `OSMetricsConfig` struct + per-instance override
- `internal/orchestrator/runner.go` — `resolveOSMethod()` + conditional OSSQLCollector registration in medium tier
- Agent parsers exported: `CPURaw`, `DiskStatRaw`, `ParseCPURaw()`, `ParseDiskStats()` from `internal/agent/osmetrics.go`

### M8_11b — OS Metrics Frontend UI
- Removed `agent_url` gate — section is now data-driven (renders if `os.*` metrics exist)
- 4 stat cards: Memory (used/total GB), CPU (usr+sys%), Load (1m/5m/15m), Disk I/O Util
- 4 time-series charts: Memory Usage (stacked area), CPU Usage (stacked area), Load Average (3 lines), Disk I/O (read/write KB/s)
- Subtle info badge for agent-only extras when agent not configured

### Process Improvements
- **Codebase Digest system** launched — `.claude/rules/codebase-digest.md`, first `docs/CODEBASE_DIGEST.md` generated and uploaded to Project Knowledge
- **Strategy doc v2.4** — 22 stale items fixed (bash bug resolved, 4-layer persistence, RBAC updated, pgxpool, OS SQL method, tool versions)
- **Step 0 pattern** — team-prompts now instruct agents to read existing code before writing anything

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
Storage DB:     pgpulse_storage on port 5432

OS Metrics:     Flowing on all 3 instances via pg_read_file('/proc/*')
Grants applied: pg_read_server_files + EXECUTE on pg_read_file (all 3 overloads)
```

---

## Known Issues

| Issue | Status | Notes |
|-------|--------|-------|
| PG 16 pg_read_file EXECUTE revoked from PUBLIC | **Documented** | Ubuntu 24.04 default. Need both role membership AND EXECUTE grants. See M8_11b session-log. |
| `os.diskstat.*` vs `os.disk.*` naming inconsistency | **Parked** | Two namespaces for same subsystem. Will fix in metric naming standardization (after competitive research). |
| Cache Hit Ratio metric name mismatch | Open | Pre-existing from earlier iterations |
| `c.command_desc` → `c.command` SQL bug in cluster progress | Open | PG 16 column name |
| `002_timescaledb.sql` migration logs skip warning every startup | Open | Pre-existing |
| Disk I/O chart: shows KB/s, would be better as MB/s | Minor | Frontend display preference — fix in naming standardization iteration |

---

## Current Metric Keys (os.* namespace)

```
os.cpu.idle_pct
os.cpu.iowait_pct
os.cpu.system_pct
os.cpu.user_pct
os.disk.free_bytes          (label: mount)
os.disk.inodes_total        (label: mount)
os.disk.inodes_used         (label: mount)
os.disk.total_bytes         (label: mount)
os.disk.used_bytes          (label: mount)
os.diskstat.read_await_ms   (label: device)
os.diskstat.read_kb         (label: device)
os.diskstat.reads_completed (label: device)
os.diskstat.util_pct        (label: device)
os.diskstat.write_await_ms  (label: device)
os.diskstat.write_kb        (label: device)
os.diskstat.writes_completed (label: device)
os.load.15m
os.load.1m
os.load.5m
os.memory.available_kb
os.memory.commit_limit_kb
os.memory.committed_as_kb
os.memory.total_kb
os.memory.used_kb
os.uptime_seconds
pgpulse.server.hostname     (label: hostname)
pgpulse.server.os           (label: os)
```

---

## Roadmap: Updated Priorities

### Immediate Queue

1. **Competitive research** — PMM, Datadog, Zabbix, pgCenter, pg_profile, pganalyze
   - Scoped to: metric naming standard, M6 OS Agent design, M7 Per-DB features, Prometheus exporter pattern
   - Produces: research doc + naming convention + M6/M7 requirements input

2. **Metric naming standardization** — define standard based on research, apply in dedicated iteration

3. **M6 OS Agent** — full agent binary, informed by competitive research

### New Milestones (not yet sequenced into roadmap)

| # | Milestone | Scope | Priority | Dependencies |
|---|-----------|-------|----------|-------------|
| NEW-1 | Windows executable | Cross-compile + test + installer | First of new items | None — mostly works today |
| NEW-2 | ML/DL remediation | Rule-based first (DBA recipe knowledge base), ML layer later | After Windows | M8 ML complete, competitive research |
| NEW-3 | Prometheus exporter | `/metrics` endpoint for Grafana/Prometheus integration | Needs research | Metric naming standard finalized |

**Priority order decided:** Windows executable → ML remediation → Prometheus exporter (needs more research)

### Deferred

- Session kill UI, settings diff UI, query plan viewer UI
- Forecast overlay on remaining metric charts
- Startup diagnostic for pg_read_file SQLSTATE 42501

---

## Key Interfaces (Current)

```go
// internal/collector/collector.go
type Collector interface {
    Name() string
    Collect(ctx context.Context, conn *pgx.Conn, ic InstanceContext) ([]MetricPoint, error)
    Interval() time.Duration
}

// internal/api/connprovider.go
type InstanceConnProvider interface {
    ConnFor(ctx context.Context, instanceID string) (*pgx.Conn, error)
    ConnForDB(ctx context.Context, instanceID, dbName string) (*pgx.Conn, error)
}

// internal/config/config.go (new in M8_11)
type OSMetricsConfig struct {
    Method string `yaml:"method" koanf:"method"` // "sql", "agent", "disabled"
}
```

---

## Build & Deploy

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

# Required grants (PG 16 hardened installs):
GRANT pg_read_server_files TO pgpulse_monitor;
GRANT EXECUTE ON FUNCTION pg_read_file(text) TO pgpulse_monitor;
GRANT EXECUTE ON FUNCTION pg_read_file(text, bigint, bigint) TO pgpulse_monitor;
GRANT EXECUTE ON FUNCTION pg_read_file(text, bigint, bigint, boolean) TO pgpulse_monitor;
```
