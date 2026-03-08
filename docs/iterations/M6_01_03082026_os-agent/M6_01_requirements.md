# M6_01 — OS Agent: Requirements

**Iteration:** M6_01_03082026_os-agent
**Date:** 2026-03-08
**Milestone:** M6 (OS Agent)

---

## Goal

Port the 19 PGAM OS and cluster metric queries (Q4–Q8, Q22–Q35) from shell-via-PostgreSQL
to native Go. Replace PGAM's dangerous `COPY TO PROGRAM` approach with direct procfs reads
and a dedicated `pgpulse-agent` binary. Add Patroni and ETCD integration using a
pluggable provider pattern.

This milestone eliminates the last major gap between PGAM parity and PGPulse MVP.

---

## Context: PGAM's Approach vs. PGPulse's Approach

PGAM collected OS metrics by running shell commands through PostgreSQL:
```sql
COPY (SELECT 1) TO PROGRAM 'top -c -b -n 1 | head -12 > /tmp/log'
SELECT pg_read_file('/tmp/log')
```
This required the monitoring user to be a PostgreSQL superuser. It also created
race conditions (multiple page loads clobbering the same temp files) and made
OS metrics inseparable from the database connection.

PGPulse eliminates this entirely. OS metrics come from:
1. A separate `pgpulse-agent` binary running on the monitored host, reading procfs/sysfs directly
2. The main server scraping the agent over HTTP
3. Graceful degradation when no agent is configured

---

## Deployment Model: Optional Agent with Graceful Degradation

```
┌─ Monitored Host ──────────────────────┐
│  PostgreSQL (port 5432)               │
│  pgpulse-agent (port 9187)  ← NEW    │
│    reads /proc/*, /sys/*              │
│    reads /etc/os-release              │
│    calls Patroni REST API             │
│    runs patronictl (fallback)         │
└───────────────────────────────────────┘
           ↑ HTTP GET /metrics/os
┌─ PGPulse Server ──────────────────────┐
│  orchestrator                         │
│    → pg collector (existing)          │
│    → os scraper  (NEW, if agent_url)  │
│    → local procfs (if same host)      │
└───────────────────────────────────────┘
```

**Behavior matrix:**

| agent_url configured | Same host | OS metrics result |
|---------------------|-----------|-------------------|
| Yes | — | Scrape agent via HTTP |
| No | Yes (auto-detected) | Read local procfs |
| No | No | "Agent not available" (partial data, no error) |

Auto-detect same-host: compare PostgreSQL hostname in DSN against `os.Hostname()`.
If they match → attempt local procfs read. If they differ → no OS metrics without agent_url.

---

## PGAM Queries in Scope (19 total)

| PGAM # | Metric | PGAM Method | PGPulse M6 Approach |
|--------|--------|-------------|---------------------|
| Q4 | Hostname (FQDN) | `COPY TO PROGRAM 'hostname -f'` | `os.Hostname()` |
| Q5 | OS distribution | `pg_read_file('/etc/lsb-release')` | `os.ReadFile("/etc/os-release")` |
| Q8 | OS uptime + load avg | `COPY TO PROGRAM 'uptime'` | `/proc/uptime`, `/proc/loadavg` |
| Q9 | System time | `COPY TO PROGRAM 'date'` | `time.Now()` |
| Q12 | Total RAM | `COPY TO PROGRAM 'free -m'` | `/proc/meminfo` (MemTotal) |
| Q22 | Memory overcommit | `grep Comm /proc/meminfo` | `/proc/meminfo` (CommitLimit, Committed_AS) |
| Q23 | Full meminfo | `grep ommit /proc/meminfo && free -h` | `/proc/meminfo` (full parse) |
| Q24 | CPU/top summary | `COPY TO PROGRAM 'top -b'` | `/proc/stat` (CPU%) + `/proc/loadavg` |
| Q25 | Disk usage | `COPY TO PROGRAM 'df -Tih'` | `syscall.Statfs` per mount point |
| Q26 | I/O stats snapshot | `COPY TO PROGRAM 'iostat -Nmhx'` | `/proc/diskstats` |
| Q27 | I/O stats 1s delta | `COPY TO PROGRAM 'iostat -kx 1 2'` | Two `/proc/diskstats` reads, 1s apart |
| Q28 | Patroni binary check | `pg_stat_file('/usr/bin/patronictl')` | `os.Stat(patroniCtlPath)` |
| Q29 | Patroni cluster state | `COPY TO PROGRAM 'patronictl list'` | PatroniProvider (REST → shell fallback) |
| Q30 | Patroni version | `COPY TO PROGRAM 'patronictl version'` | PatroniProvider |
| Q31 | Patroni switchover history | `COPY TO PROGRAM 'patronictl history'` | PatroniProvider |
| Q32 | ETCD binary check | `pg_stat_file('/app/etcd/bin/etcdctl')` | `os.Stat(etcdCtlPath)` |
| Q33 | ETCD member list | `COPY TO PROGRAM 'etcdctl member list'` | ETCDProvider (HTTP API → shell fallback) |
| Q33b | ETCD endpoint status | `COPY TO PROGRAM 'etcdctl endpoint status'` | ETCDProvider |
| Q33c | ETCD endpoint health | `COPY TO PROGRAM 'etcdctl endpoint health'` | ETCDProvider |

---

## Patroni Integration: Smart Provider Pattern

### Interface

```go
// internal/cluster/patroni/provider.go

type ClusterMember struct {
    Name        string
    Host        string
    Port        int
    Role        string    // "Leader", "Replica", "Standby Leader"
    State       string    // "running", "stopped", "start failed"
    TLSlag      string    // timeline lag
    ReplayLag   string
    Tags        map[string]string
}

type ClusterState struct {
    ClusterName string
    Members     []ClusterMember
    Scope       string
}

type SwitchoverEvent struct {
    Timestamp string
    FromNode  string
    ToNode    string
    Reason    string
}

type PatroniProvider interface {
    GetClusterState(ctx context.Context) (*ClusterState, error)
    GetHistory(ctx context.Context) ([]SwitchoverEvent, error)
    GetVersion(ctx context.Context) (string, error)
}
```

### Implementations

**RESTProvider** — calls Patroni's built-in HTTP API (default port 8008):
- `GET http://{patroni_host}:{patroni_port}/cluster` → ClusterState
- `GET http://{patroni_host}:{patroni_port}/history` → SwitchoverEvent slice
- `GET http://{patroni_host}:{patroni_port}/` → version info
- Configurable timeout: 5s
- Available when: Patroni is running and API port is reachable

**ShellProvider** — runs `patronictl` via `os/exec`:
- `patronictl -c {config_path} list -f json`
- `patronictl -c {config_path} history`
- `patronictl version`
- Used as fallback or when REST is unavailable
- Binary path and config path configurable (not hardcoded)

**FallbackProvider** — strategy wrapper:
```go
type FallbackProvider struct {
    primary   PatroniProvider  // RESTProvider
    secondary PatroniProvider  // ShellProvider
}

func (f *FallbackProvider) GetClusterState(ctx context.Context) (*ClusterState, error) {
    state, err := f.primary.GetClusterState(ctx)
    if err != nil {
        return f.secondary.GetClusterState(ctx)
    }
    return state, nil
}
```

**NoOpProvider** — returned when Patroni is not configured:
```go
type NoOpProvider struct{}

func (n *NoOpProvider) GetClusterState(ctx context.Context) (*ClusterState, error) {
    return nil, ErrPatroniNotConfigured
}
```

---

## ETCD Integration: Same Pattern

```go
// internal/cluster/etcd/provider.go

type ETCDMember struct {
    ID       string
    Name     string
    PeerURL  string
    ClientURL string
    IsLeader bool
    Status   string
    DBSize   int64
}

type ETCDProvider interface {
    GetMembers(ctx context.Context) ([]ETCDMember, error)
    GetEndpointHealth(ctx context.Context) (map[string]bool, error)
}
```

- **HTTPProvider**: etcd v3 REST API (default port 2379)
- **ShellProvider**: `etcdctl member list --write-out=json`
- **FallbackProvider**: HTTP → shell fallback
- **NoOpProvider**: when ETCD not configured

---

## Agent Binary: pgpulse-agent

### Purpose
Thin HTTP server running on the monitored PostgreSQL host. Reads procfs/sysfs.
Exposes OS metrics as JSON. No database connection required.

### Endpoints

```
GET /health                — liveness check, returns {"status":"ok"}
GET /metrics/os            — all OS metrics (hostname, uptime, memory, CPU, disk, iostat)
GET /metrics/cluster       — Patroni + ETCD cluster state (if configured)
```

### Response format for /metrics/os

```json
{
  "collected_at": "2026-03-08T10:00:00Z",
  "hostname": "pg-primary-01.example.com",
  "os_release": {
    "name": "Ubuntu",
    "version": "22.04",
    "id": "ubuntu"
  },
  "uptime_seconds": 1234567,
  "load_avg": { "1m": 0.45, "5m": 0.38, "15m": 0.31 },
  "memory": {
    "total_kb": 16384000,
    "available_kb": 8192000,
    "used_kb": 8192000,
    "commit_limit_kb": 12000000,
    "committed_as_kb": 4000000
  },
  "cpu": {
    "user_pct": 12.3,
    "system_pct": 4.5,
    "iowait_pct": 0.8,
    "idle_pct": 82.4,
    "num_cpus": 8
  },
  "disks": [
    {
      "mount": "/",
      "device": "/dev/sda1",
      "fstype": "ext4",
      "total_bytes": 107374182400,
      "used_bytes": 32212254720,
      "free_bytes": 75161927680,
      "inodes_total": 6553600,
      "inodes_used": 245000
    }
  ],
  "diskstats": [
    {
      "device": "sda",
      "reads_completed": 123456,
      "writes_completed": 654321,
      "read_kb": 4096000,
      "write_kb": 8192000,
      "io_in_progress": 2,
      "read_await_ms": 1.2,
      "write_await_ms": 3.4,
      "util_pct": 5.6
    }
  ]
}
```

### Configuration

```yaml
# In pgpulse.yml, per-instance agent config:
instances:
  - id: "prod-primary"
    name: "Production Primary"
    dsn: "host=10.0.0.1 port=5432 dbname=postgres user=pgpulse sslmode=disable"
    agent_url: "http://10.0.0.1:9187"       # optional — enables OS metrics
    patroni_url: "http://10.0.0.1:8008"     # optional — enables Patroni REST
    patroni_config: "/etc/patroni/patroni.yml"  # optional — enables patronictl fallback
    patroni_ctl_path: "/usr/bin/patronictl"  # optional — defaults to /usr/bin/patronictl
    etcd_endpoints: ["http://10.0.0.1:2379"] # optional — enables ETCD monitoring
    etcd_ctl_path: "/usr/local/bin/etcdctl"  # optional

# Separate pgpulse-agent config file (configs/pgpulse-agent.yml):
agent:
  listen_addr: "0.0.0.0:9187"
  patroni_url: "http://localhost:8008"
  patroni_config: "/etc/patroni/patroni.yml"
  patroni_ctl_path: "/usr/bin/patronictl"
  etcd_endpoints: ["http://localhost:2379"]
  etcd_ctl_path: "/usr/local/bin/etcdctl"
  mount_points: ["/", "/data", "/pgdata"]  # disks to monitor; empty = all non-tmpfs
```

---

## Polling Intervals

| Metric Group | Interval | Rationale |
|-------------|----------|-----------|
| CPU (user%, sys%, iowait%) | 10s | Fast-changing, matches connection monitoring frequency |
| Memory (used, available, overcommit) | 10s | Fast-changing |
| Load average | 10s | Fast-changing |
| Disk usage (bytes, inodes) | 60s | Slow-changing |
| Diskstats / iostat | 30s | Medium-changing |
| Hostname, OS release, uptime | 300s | Near-static |
| Patroni cluster state | 30s | HA changes can be fast |
| ETCD member/health | 60s | Cluster topology rarely changes |

---

## Frontend Impact

The ServerDetail page already has 11 monitoring sections. Add:
- **OS Metrics section** (new, collapsible): hostname, OS, uptime, CPU usage bar, memory bar, load avg sparkline
- **Disk section** (new): table of mount points with used/total/inode bars
- **I/O section** (new): diskstats table (reads/writes/await/util)
- **Cluster section** (update existing placeholder if present): Patroni member table + ETCD health
- **Graceful absent state**: if `agent_available: false` in API response → show "OS Agent not configured" placeholder with setup instructions link

---

## Non-Functional Requirements

- Agent binary must build to a single static binary (`CGO_ENABLED=0`)
- Agent has zero database connections (pure OS access)
- All procfs parsing must handle `/proc` unavailability gracefully (returns error, not panic)
  — relevant for Windows dev environment where `/proc` does not exist
- All Patroni/ETCD provider errors are logged at WARN level and result in empty/nil metrics, not errors to the UI
- Agent adds no mandatory dependencies to the main server binary — scraping is optional

---

## Out of Scope

- Agent authentication/TLS (MVP: trust network; add in M8+)
- Windows native OS metrics (agent runs on Linux hosts; dev machine doesn't need it)
- Kubernetes pod metrics (future)
- Custom metric plugins
- Alert rules on OS metrics (can be added to M4 alert engine in a follow-up)
