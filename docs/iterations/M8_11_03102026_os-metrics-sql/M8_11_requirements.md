# M8_11 Requirements — OS Metrics via PostgreSQL

**Iteration:** M8_11
**Date:** 2026-03-10
**Goal:** Collect OS metrics (memory, CPU, load, disk I/O, uptime) through PostgreSQL's `pg_read_file('/proc/*')` using the existing monitoring connection — no agent binary needed.

---

## Background

PGPulse has two OS metrics paths:
1. **pgpulse-agent binary** (`cmd/pgpulse-agent/`, `internal/agent/`) — standalone Linux process reading `/proc` and `/sys`
2. **Nothing** — if the agent isn't deployed, PGPulse has zero OS visibility

Information Security teams resist deploying unknown agent binaries on production servers. This iteration adds a third path: collect OS metrics through the SQL connection PGPulse already has, using `pg_read_file('/proc/*')` with the `pg_read_server_files` role.

### PGAM Precedent

PGAM collected OS metrics via `COPY TO PROGRAM` (superuser required, massive security hole). The exact queries from `analiz2.php`:

| Line | PGAM Method | What |
|------|-------------|------|
| 170 | `COPY ... TO PROGRAM 'hostname -f > /tmp/WatchDog_hostname.log'` | Server hostname |
| 171 | `pg_stat_file('/etc/lsb-release') + pg_read_file()` | OS distribution (already safe) |
| 183 | `COPY ... TO PROGRAM 'uptime \|awk...'` | OS uptime + load |
| 186–187 | `COPY ... TO PROGRAM 'date...'` → `pg_read_file('/tmp/WatchDog_timeos.log')` | OS clock |
| 201 | `COPY ... TO PROGRAM 'echo $(free -m \| grep Mem...)'` | Total RAM |
| 290 | `COPY ... TO PROGRAM 'cat /proc/meminfo \| grep -i comm'` | Memory overcommit |
| 300 | `COPY ... TO PROGRAM 'grep ommit /proc/meminfo && free -h'` | Full meminfo |
| 310 | `COPY ... TO PROGRAM 'top -c -b -n 1 -w 128 \| head -n 12'` | Process list |
| 321 | `COPY ... TO PROGRAM 'df -Tih'` | Filesystem usage |
| 331 | `COPY ... TO PROGRAM 'iostat -Nmhx'` | Disk I/O snapshot |
| 341 | `COPY ... TO PROGRAM 'iostat -kx 1 2'` | Disk I/O 1s interval |

PGPulse replaces ALL `COPY TO PROGRAM` calls with `pg_read_file('/proc/*')`.

---

## Functional Requirements

### FR1: OSSQLCollector

Create `internal/collector/os_sql.go` implementing the `Collector` interface. Single stateful collector that reads `/proc/*` files via `SELECT pg_read_file(...)` and parses them in Go.

### FR2: /proc files to read

| File | Metrics Produced | Parse Method |
|------|-----------------|--------------|
| `/proc/meminfo` | MemTotal, MemFree, MemAvailable, Buffers, Cached, SwapTotal, SwapFree, Committed_AS | Line-by-line key:value |
| `/proc/uptime` | OS uptime seconds, idle seconds | Space-separated floats |
| `/proc/loadavg` | Load 1m, 5m, 15m | Space-separated floats |
| `/proc/stat` | CPU user%, system%, idle%, iowait%, steal% | Delta between cycles (cumulative jiffies) |
| `/proc/diskstats` | Read bytes/sec, write bytes/sec, read IOPS, write IOPS, io_util% | Delta between cycles (cumulative counters) |

### FR3: Graceful per-file fallback

Each `pg_read_file()` call runs independently. If one file fails (permission denied, file not found, Windows host, containerized PG), log a warning and skip that metric group. Never fail the entire collection cycle.

### FR4: Metric key compatibility

Emit the same metric keys as the existing `internal/agent/` code so frontend OS sections work without changes. The agent reads the existing metric keys in Step 0 and matches them exactly.

### FR5: Stateful delta calculations

CPU (`/proc/stat`) and disk (`/proc/diskstats`) metrics require delta computation between collection cycles. Use the same stateful pattern as `checkpoint.go` — store previous values, compute delta, use `sync.Mutex`. First cycle after startup returns no CPU/disk metrics (no baseline yet).

### FR6: Configuration

Per-instance config with global default:

```yaml
os_metrics:
  method: "sql"  # default: "sql" | "agent" | "disabled"

instances:
  - id: prod-main
    dsn: "..."
    os_metrics_method: "sql"  # overrides global default
```

Orchestrator checks method when building collectors:
- `"sql"` → register OSSQLCollector
- `"agent"` → don't register (agent reports separately)
- `"disabled"` → skip OS metrics entirely

### FR7: Server info enrichment

Hostname (`/etc/hostname`) and OS distribution (`/etc/os-release`) are read by the existing `server_info` collector at low interval (300s), not by os_sql. These are metadata, not time-series metrics.

### FR8: Interval group

OSSQLCollector runs at **medium frequency (60s)** — same group as replication and statements collectors.

### FR9: Required grants

```sql
GRANT pg_read_server_files TO pgpulse_monitor;
```

Documentation must note this grant as the recommended setup for SQL-based OS metrics.

### FR10: Codebase Digest

Generate `docs/CODEBASE_DIGEST.md` at end of iteration — first-ever digest. Follow the 7-section template in `.claude/rules/codebase-digest.md`.

---

## Non-Functional Requirements

- NFR1: Each `pg_read_file()` call completes within 1s (set statement_timeout = '5s' as safety)
- NFR2: Total os_sql collection cycle < 3s for all 5 files
- NFR3: Zero impact on monitored instance — read-only, no temp files, no COPY TO PROGRAM
- NFR4: Existing agent code (`cmd/pgpulse-agent/`, `internal/agent/`) remains untouched
- NFR5: Frontend OS sections (OSSystemSection, DiskSection, etc.) require zero changes

---

## Acceptance Criteria

1. `go build ./cmd/pgpulse-server` succeeds
2. `go test ./cmd/... ./internal/...` passes
3. `cd web && npm run build && npm run lint && npm run typecheck` passes
4. `golangci-lint run` — 0 new issues
5. OSSQLCollector registered in orchestrator's medium group
6. Memory metrics visible on demo VM dashboard within 60s of restart
7. CPU/disk delta metrics visible after second collection cycle
8. Permission denied on `/proc/meminfo` → warning logged, other metrics still collected
9. `os_metrics.method: "disabled"` → no os_sql collector registered, no errors
10. Codebase Digest generated and committed
