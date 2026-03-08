# M6_01 — OS Agent: Design

**Iteration:** M6_01_03082026_os-agent
**Date:** 2026-03-08

---

## New Files and Directories

```
cmd/pgpulse-agent/
  main.go                         — agent binary entrypoint

internal/agent/
  server.go                       — HTTP server for the agent (chi router, /health, /metrics/os, /metrics/cluster)
  osmetrics.go                    — procfs readers (CPU, memory, disk, iostat, hostname, OS release, uptime)
  scraper.go                      — main server side: HTTP client that fetches agent metrics

internal/cluster/
  patroni/
    provider.go                   — PatroniProvider interface + shared types
    rest.go                       — RESTProvider implementation
    shell.go                      — ShellProvider implementation (os/exec patronictl)
    fallback.go                   — FallbackProvider (REST → shell)
    noop.go                       — NoOpProvider
  etcd/
    provider.go                   — ETCDProvider interface + shared types
    http.go                       — HTTPProvider implementation
    shell.go                      — ShellProvider implementation (os/exec etcdctl)
    fallback.go                   — FallbackProvider
    noop.go                       — NoOpProvider

internal/collector/
  os.go                           — OSCollector (wraps agent.Scraper, implements Collector interface)
  cluster.go                      — ClusterCollector (wraps PatroniProvider + ETCDProvider)

configs/
  pgpulse-agent.example.yml       — agent config example

deploy/systemd/
  pgpulse-agent.service           — systemd unit for the agent
```

---

## Key Interfaces and Types

### Agent OS Metrics (internal/agent/osmetrics.go)

```go
package agent

import "time"

// OSSnapshot is the full OS metrics payload returned by the agent.
type OSSnapshot struct {
    CollectedAt time.Time      `json:"collected_at"`
    Hostname    string         `json:"hostname"`
    OSRelease   OSRelease      `json:"os_release"`
    UptimeSecs  float64        `json:"uptime_seconds"`
    LoadAvg     LoadAvg        `json:"load_avg"`
    Memory      MemoryInfo     `json:"memory"`
    CPU         CPUInfo        `json:"cpu"`
    Disks       []DiskInfo     `json:"disks"`
    DiskStats   []DiskStatInfo `json:"diskstats"`
}

type OSRelease struct {
    Name    string `json:"name"`
    Version string `json:"version"`
    ID      string `json:"id"`
}

type LoadAvg struct {
    One  float64 `json:"1m"`
    Five float64 `json:"5m"`
    Fifteen float64 `json:"15m"`
}

type MemoryInfo struct {
    TotalKB       int64 `json:"total_kb"`
    AvailableKB   int64 `json:"available_kb"`
    UsedKB        int64 `json:"used_kb"`
    CommitLimitKB int64 `json:"commit_limit_kb"`
    CommittedAsKB int64 `json:"committed_as_kb"`
}

type CPUInfo struct {
    UserPct   float64 `json:"user_pct"`
    SystemPct float64 `json:"system_pct"`
    IOWaitPct float64 `json:"iowait_pct"`
    IdlePct   float64 `json:"idle_pct"`
    NumCPUs   int     `json:"num_cpus"`
}

type DiskInfo struct {
    Mount       string `json:"mount"`
    Device      string `json:"device"`
    FSType      string `json:"fstype"`
    TotalBytes  int64  `json:"total_bytes"`
    UsedBytes   int64  `json:"used_bytes"`
    FreeBytes   int64  `json:"free_bytes"`
    InodesTotal int64  `json:"inodes_total"`
    InodesUsed  int64  `json:"inodes_used"`
}

type DiskStatInfo struct {
    Device          string  `json:"device"`
    ReadsCompleted  int64   `json:"reads_completed"`
    WritesCompleted int64   `json:"writes_completed"`
    ReadKB          int64   `json:"read_kb"`
    WriteKB         int64   `json:"write_kb"`
    IOInProgress    int64   `json:"io_in_progress"`
    ReadAwaitMs     float64 `json:"read_await_ms"`
    WriteAwaitMs    float64 `json:"write_await_ms"`
    UtilPct         float64 `json:"util_pct"`
}
```

### Agent Scraper (internal/agent/scraper.go)

Used by the main server to fetch data from a remote agent:

```go
package agent

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "time"
)

type Scraper struct {
    baseURL    string
    httpClient *http.Client
}

func NewScraper(baseURL string) *Scraper {
    return &Scraper{
        baseURL: strings.TrimRight(baseURL, "/"),
        httpClient: &http.Client{Timeout: 10 * time.Second},
    }
}

func (s *Scraper) ScrapeOS(ctx context.Context) (*OSSnapshot, error) { ... }
func (s *Scraper) IsAlive(ctx context.Context) bool { ... }
```

### OS Collector (internal/collector/os.go)

Implements the existing `Collector` interface. Source of data is determined at construction:

```go
package collector

type OSCollectorSource int

const (
    OSSourceNone    OSCollectorSource = iota // no OS data
    OSSourceLocal                            // read local /proc directly
    OSSourceAgent                            // scrape remote agent
)

type OSCollector struct {
    source  OSCollectorSource
    scraper *agent.Scraper      // non-nil when OSSourceAgent
    // local reader fields when OSSourceLocal
}

func NewOSCollector(instanceDSN, agentURL string) *OSCollector {
    if agentURL != "" {
        return &OSCollector{source: OSSourceAgent, scraper: agent.NewScraper(agentURL)}
    }
    if isLocalHost(instanceDSN) {
        return &OSCollector{source: OSSourceLocal}
    }
    return &OSCollector{source: OSSourceNone}
}

func (c *OSCollector) Name() string { return "os" }

func (c *OSCollector) Collect(ctx context.Context, conn *pgx.Conn, ic collector.InstanceContext) ([]MetricPoint, error) {
    switch c.source {
    case OSSourceNone:
        return nil, nil  // graceful: no error, no data
    case OSSourceAgent:
        snap, err := c.scraper.ScrapeOS(ctx)
        if err != nil {
            return nil, fmt.Errorf("os scraper: %w", err)
        }
        return snapshotToMetricPoints(snap), nil
    case OSSourceLocal:
        snap, err := collectLocalOS()
        if err != nil {
            return nil, fmt.Errorf("local os: %w", err)
        }
        return snapshotToMetricPoints(snap), nil
    }
    return nil, nil
}

func (c *OSCollector) Interval() time.Duration { return 10 * time.Second }
```

### Cluster Collector (internal/collector/cluster.go)

```go
package collector

type ClusterCollector struct {
    patroni cluster_patroni.PatroniProvider
    etcd    cluster_etcd.ETCDProvider
}

func NewClusterCollector(
    patroniProvider cluster_patroni.PatroniProvider,
    etcdProvider    cluster_etcd.ETCDProvider,
) *ClusterCollector { ... }

func (c *ClusterCollector) Name() string { return "cluster" }
func (c *ClusterCollector) Interval() time.Duration { return 30 * time.Second }
func (c *ClusterCollector) Collect(ctx context.Context, conn *pgx.Conn, ic InstanceContext) ([]MetricPoint, error) {
    // collect patroni state → MetricPoints
    // collect etcd state → MetricPoints
    // errors from either are logged at WARN, not returned (partial data ok)
}
```

### Patroni Provider (internal/cluster/patroni/provider.go)

```go
package patroni

import "context"

var ErrPatroniNotConfigured = errors.New("patroni not configured")
var ErrPatroniUnavailable   = errors.New("patroni unavailable")

type ClusterMember struct {
    Name      string
    Host      string
    Port      int
    Role      string
    State     string
    Timeline  int
    Lag       int64 // bytes
    Tags      map[string]string
}

type ClusterState struct {
    ClusterName string
    Members     []ClusterMember
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

// NewProvider builds the right provider chain based on config.
// If neither patroniURL nor patroniConfig is set, returns NoOpProvider.
func NewProvider(cfg PatroniConfig) PatroniProvider {
    var providers []PatroniProvider

    if cfg.PatroniURL != "" {
        providers = append(providers, NewRESTProvider(cfg.PatroniURL))
    }
    if cfg.PatroniCtlPath != "" || cfg.PatroniConfig != "" {
        providers = append(providers, NewShellProvider(cfg.PatroniCtlPath, cfg.PatroniConfig))
    }

    switch len(providers) {
    case 0:
        return &NoOpProvider{}
    case 1:
        return providers[0]
    default:
        return NewFallbackProvider(providers[0], providers[1])
    }
}
```

### Config Extension (internal/config/config.go)

Add to `InstanceConfig`:

```go
type InstanceConfig struct {
    ID      string `koanf:"id"`
    Name    string `koanf:"name"`
    DSN     string `koanf:"dsn"`
    Enabled bool   `koanf:"enabled"`
    MaxConns int32 `koanf:"max_conns"`

    // OS Agent (M6 - new fields)
    AgentURL string `koanf:"agent_url"` // optional: "http://host:9187"

    // Patroni (M6 - new fields)
    PatroniURL     string `koanf:"patroni_url"`      // optional: "http://host:8008"
    PatroniConfig  string `koanf:"patroni_config"`   // optional: "/etc/patroni/patroni.yml"
    PatroniCtlPath string `koanf:"patroni_ctl_path"` // optional: "/usr/bin/patronictl"

    // ETCD (M6 - new fields)
    ETCDEndpoints []string `koanf:"etcd_endpoints"` // optional: ["http://host:2379"]
    ETCDCtlPath   string   `koanf:"etcd_ctl_path"`  // optional: "/usr/local/bin/etcdctl"
}
```

New top-level config section for agent binary:

```go
type AgentConfig struct {
    ListenAddr     string   `koanf:"listen_addr"`      // default: "0.0.0.0:9187"
    PatroniURL     string   `koanf:"patroni_url"`
    PatroniConfig  string   `koanf:"patroni_config"`
    PatroniCtlPath string   `koanf:"patroni_ctl_path"`
    ETCDEndpoints  []string `koanf:"etcd_endpoints"`
    ETCDCtlPath    string   `koanf:"etcd_ctl_path"`
    MountPoints    []string `koanf:"mount_points"`     // empty = all non-tmpfs
}
```

---

## Agent Binary (cmd/pgpulse-agent/main.go)

```go
package main

func main() {
    cfg := loadConfig()

    patroniProvider := patroni.NewProvider(patroni.PatroniConfig{
        PatroniURL:     cfg.Agent.PatroniURL,
        PatroniConfig:  cfg.Agent.PatroniConfig,
        PatroniCtlPath: cfg.Agent.PatroniCtlPath,
    })

    etcdProvider := etcd.NewProvider(etcd.ETCDConfig{
        Endpoints: cfg.Agent.ETCDEndpoints,
        CtlPath:   cfg.Agent.ETCDCtlPath,
    })

    srv := agent.NewServer(agent.ServerConfig{
        ListenAddr:      cfg.Agent.ListenAddr,
        MountPoints:     cfg.Agent.MountPoints,
        PatroniProvider: patroniProvider,
        ETCDProvider:    etcdProvider,
    })

    ctx := setupSignalContext()
    if err := srv.Run(ctx); err != nil {
        log.Fatal(err)
    }
}
```

---

## Orchestrator Changes (internal/orchestrator/runner.go)

On instance startup, construct OS and cluster collectors using the instance config:

```go
func (r *Runner) buildCollectors(cfg config.InstanceConfig) []collector.Collector {
    collectors := []collector.Collector{
        // existing collectors...
        collector.NewInstanceCollector(),
        collector.NewReplicationCollector(),
        // ... etc

        // NEW: OS and cluster
        collector.NewOSCollector(cfg.DSN, cfg.AgentURL),
        collector.NewClusterCollector(
            patroni.NewProvider(patroni.PatroniConfig{
                PatroniURL:     cfg.PatroniURL,
                PatroniConfig:  cfg.PatroniConfig,
                PatroniCtlPath: cfg.PatroniCtlPath,
            }),
            etcd.NewProvider(etcd.ETCDConfig{
                Endpoints: cfg.ETCDEndpoints,
                CtlPath:   cfg.ETCDCtlPath,
            }),
        ),
    }
    return collectors
}
```

---

## API Changes

Add `agent_available` flag to instance detail response so frontend knows whether to
show OS sections or the "agent not configured" placeholder:

```go
// In internal/api/instances.go, extend the instance response:
type instanceDetailResponse struct {
    // existing fields...
    AgentAvailable bool `json:"agent_available"`
}
```

Add OS metrics to the metrics API endpoint:

```
GET /api/v1/instances/:id/metrics
```

Already returns PG metrics. Extend to include OS metrics under a separate key:
```json
{
  "pg": { ... existing metrics ... },
  "os": { ... OSSnapshot or null if no agent ... },
  "cluster": { ... ClusterState or null if no Patroni ... }
}
```

---

## Frontend Changes

### ServerDetail.tsx — New Sections

**OS System section:**
```
┌─ System ──────────────────────────────────────────────────────┐
│  Hostname: pg-primary-01.example.com    OS: Ubuntu 22.04      │
│  Uptime: 14 days, 3 hours               CPUs: 8               │
│                                                                 │
│  CPU Usage    ████████░░░░░░░░░░░░  17.6%                     │
│  Memory       █████████████░░░░░░░  67.2%  (11GB / 16GB)      │
│  Load Avg     0.45 / 0.38 / 0.31 (1m/5m/15m)                  │
└─────────────────────────────────────────────────────────────────┘
```

**Disk Usage section:**
```
┌─ Disk Usage ──────────────────────────────────────────────────┐
│  Mount     Device      Type   Used    Total  Inodes           │
│  /         /dev/sda1   ext4   30GB    100GB  ██░  4%          │
│  /pgdata   /dev/sdb1   xfs    450GB   1TB    ███  45%         │
└─────────────────────────────────────────────────────────────────┘
```

**I/O Stats section:**
```
┌─ I/O Stats ───────────────────────────────────────────────────┐
│  Device   Read/s  Write/s  Read MB/s  Write MB/s  Await  Util │
│  sda      120     450      4.2 MB/s   18.3 MB/s   1.2ms  5%   │
│  sdb      800     2400     32.1 MB/s  96.0 MB/s   3.4ms  38%  │
└─────────────────────────────────────────────────────────────────┘
```

**Cluster section (Patroni):**
```
┌─ HA Cluster (Patroni) ────────────────────────────────────────┐
│  Cluster: main                                                  │
│  Member              Role      State    Timeline  Lag           │
│  pg-primary-01       Leader    running  1         —             │
│  pg-replica-01       Replica   running  1         0 B           │
│  pg-replica-02       Replica   running  1         128 KB        │
└─────────────────────────────────────────────────────────────────┘
```

**No-agent placeholder:**
```
┌─ OS Metrics ──────────────────────────────────────────────────┐
│  ⚠ OS Agent not configured                                     │
│  To enable OS metrics, deploy pgpulse-agent on this host       │
│  and set agent_url in the instance configuration.              │
└─────────────────────────────────────────────────────────────────┘
```

---

## procfs Implementation Notes

### CPU Usage (/proc/stat)
CPU% cannot be read from a single snapshot — requires two readings and delta calculation.
Read at t0 and t1 (1s apart for agent startup; subsequent reads at 10s interval):
```
cpu_pct = (active_delta / total_delta) * 100
active = user + nice + system + irq + softirq + steal
total  = active + idle + iowait
```

### Disk I/O Await (/proc/diskstats)
Same delta calculation required for await_ms:
```
read_await_ms  = (read_time_delta / reads_delta) if reads_delta > 0 else 0
write_await_ms = (write_time_delta / writes_delta) if writes_delta > 0 else 0
util_pct       = (io_time_delta / interval_ms) * 100
```
The agent must maintain state between collections for delta calculation.

### /etc/os-release Parsing
Parse `KEY=VALUE` or `KEY="VALUE"` lines. Extract: `NAME`, `VERSION_ID`, `ID`.
If `/etc/os-release` is unavailable, try `/etc/lsb-release` (Ubuntu legacy format).

### Graceful /proc Unavailability
On Windows dev machines, `/proc` does not exist. All procfs functions must return
an error (not panic) when the file is missing. The OSCollector treats these as
`OSSourceNone` on non-Linux builds.

Use build tags to exclude procfs code from Windows builds:
```go
//go:build linux
// +build linux
```
And provide stub implementations for non-Linux:
```go
//go:build !linux
// +build !linux
```

---

## Test Coverage Required

### Unit tests (no real /proc needed)
- `TestOSCollector_NoAgent` — returns nil metrics, no error
- `TestOSCollector_AgentUnreachable` — returns error (not nil)
- `TestFallbackPatroniProvider_RESTSucceeds` — REST called, shell not called
- `TestFallbackPatroniProvider_RESTFails_ShellCalled` — REST fails, shell called
- `TestFallbackETCDProvider_HTTPFails_ShellCalled`
- `TestParseOSRelease_Ubuntu` — parses /etc/os-release format
- `TestParseOSRelease_RHEL` — parses RHEL format
- `TestParseMeminfo` — parses /proc/meminfo key-value pairs
- `TestCPUDelta` — delta calculation with known input/output
- `TestDiskStatsDelta` — delta calculation for await and util

### Integration tests (Linux CI only, build tag: integration)
- `TestAgentServer_HealthEndpoint` — GET /health returns 200
- `TestAgentServer_OSMetricsEndpoint` — GET /metrics/os returns valid JSON
- `TestOSCollector_LocalSource` — collects on Linux host
- `TestRESTPatroniProvider_MockServer` — uses httptest.NewServer

---

## Architecture Decision Log

| # | Decision | Rationale |
|---|----------|-----------|
| D-M6-01 | Option C deployment: optional agent with graceful degradation | Keeps MVP simple (same-host), enables remote monitoring without forcing agent everywhere |
| D-M6-02 | PatroniProvider: FallbackProvider (REST → shell) | Adapts to locked-down containers (API only) and legacy bare-metal (shell only) without config changes |
| D-M6-03 | ETCDProvider: same pattern as Patroni | Consistency; future pluggability (Consul provider would follow same interface) |
| D-M6-04 | procfs guarded by build tags (linux only) | Prevents /proc-related panics on Windows dev machine; CI runs on Linux |
| D-M6-05 | CPU and I/O require delta — agent maintains state between collections | Single snapshot cannot produce meaningful CPU% or await_ms |
| D-M6-06 | Agent binary has zero DB connections | Clean separation; agent is pure OS access |
| D-M6-07 | Fast polling: CPU/mem 10s, iostat 30s, disk 60s | Matches PGAM's intent; avoids over-polling slow syscalls |
| D-M6-08 | No agent auth/TLS in MVP | Runs on trusted internal networks; add in M8+ |
| D-M6-09 | All 19 PGAM queries in one iteration | Scope is well-defined; splitting creates a half-working OS section |
