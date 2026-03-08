# M6_01 — OS Agent: Team Prompt

**Iteration:** M6_01_03082026_os-agent
**Date:** 2026-03-08
**Paste this into Claude Code after updating CLAUDE.md "Current Iteration" section.**

---

## Context

We are implementing M6: OS Agent for PGPulse. This ports all 19 PGAM OS/cluster
queries (Q4–Q8, Q22–Q35) from PGAM's dangerous COPY-TO-PROGRAM approach to native Go.

Read the full design at `docs/iterations/M6_01_03082026_os-agent/design.md` before
writing any code. All interfaces, types, struct fields, and response formats are
specified there.

## Critical Rules

- NEVER use `COPY ... TO PROGRAM` — this is the exact anti-pattern we are replacing
- All procfs files (`/proc/*`, `/etc/os-release`) MUST be guarded with `//go:build linux`
  and stub implementations for `//go:build !linux` — the dev machine is Windows
- Agent binary has ZERO database connections
- Patroni/ETCD errors are WARN-level log entries, not errors returned to the caller
- Do NOT modify any existing collector files (instance.go, replication.go, etc.)
- Do NOT modify internal/auth/ or internal/api/auth.go

---

## Create a team of 3 specialists:

---

### SPECIALIST 1 — OS AGENT BINARY + procfs

**Your scope:**
- `cmd/pgpulse-agent/main.go`
- `internal/agent/server.go`
- `internal/agent/osmetrics.go`
- `internal/agent/osmetrics_linux.go` (build-tagged implementation)
- `internal/agent/osmetrics_stub.go` (build-tagged stub for non-Linux)
- `internal/agent/scraper.go`
- `configs/pgpulse-agent.example.yml`
- `deploy/systemd/pgpulse-agent.service`

**Task 1: Create internal/agent/osmetrics.go**

Define all types from the design doc: `OSSnapshot`, `OSRelease`, `LoadAvg`,
`MemoryInfo`, `CPUInfo`, `DiskInfo`, `DiskStatInfo`.

Define the collection function signature (no body here — implementation is Linux-only):
```go
func CollectOS(mountPoints []string) (*OSSnapshot, error)
```

**Task 2: Create internal/agent/osmetrics_linux.go** (build tag: `//go:build linux`)

Implement `CollectOS`:
- `collectHostname()` — `os.Hostname()`
- `collectOSRelease()` — parse `/etc/os-release`, fallback to `/etc/lsb-release`
  Parse KEY=VALUE and KEY="VALUE" lines; extract NAME, VERSION_ID, ID
- `collectUptime()` — read `/proc/uptime`, parse first field (seconds as float)
- `collectLoadAvg()` — read `/proc/loadavg`, parse first 3 space-separated floats
- `collectMemory()` — parse `/proc/meminfo`:
  keys needed: MemTotal, MemAvailable, MemFree, Buffers, Cached, CommitLimit, Committed_AS
  UsedKB = TotalKB - AvailableKB
- `collectCPU()` — read `/proc/stat` twice, 1s apart; calculate delta:
  Parse line starting with "cpu " (aggregate): user nice system idle iowait irq softirq steal
  active = user+nice+system+irq+softirq+steal; total = active+idle+iowait
  cpu_pct field = (active_delta/total_delta)*100
  Also read `/proc/cpuinfo` or use runtime.NumCPU() for num_cpus
- `collectDisks(mountPoints []string)` — use `syscall.Statfs` per mount point:
  Read `/proc/mounts` to get device and fstype per mountpoint
  Filter: skip if fstype in {"tmpfs","devtmpfs","sysfs","proc","cgroup","devpts","hugetlbfs","mqueue","pstore"}
  If mountPoints config is non-empty, only collect those mounts
- `collectDiskStats()` — parse `/proc/diskstats`:
  Fields: major minor name reads_completed reads_merged sectors_read read_time
          writes_completed writes_merged sectors_written write_time io_in_progress io_time weighted_io_time
  Maintain a package-level previousDiskStats map[string]diskStatRaw for delta calculation
  read_kb = sectors_read_delta * 512 / 1024
  read_await_ms = read_time_delta / reads_completed_delta (if reads > 0)
  util_pct = io_time_delta / interval_ms * 100
  Filter: only include devices where reads_completed+writes_completed > 0 (skip empty loop devices)

**Task 3: Create internal/agent/osmetrics_stub.go** (build tag: `//go:build !linux`)

```go
//go:build !linux

package agent

import "errors"

var ErrOSMetricsUnavailable = errors.New("OS metrics only available on Linux")

func CollectOS(mountPoints []string) (*OSSnapshot, error) {
    return nil, ErrOSMetricsUnavailable
}
```

**Task 4: Create internal/agent/server.go**

HTTP server using chi router:
```go
type Server struct {
    cfg             ServerConfig
    router          *chi.Mux
    patroniProvider patroni.PatroniProvider
    etcdProvider    etcd.ETCDProvider
}

type ServerConfig struct {
    ListenAddr      string
    MountPoints     []string
    PatroniProvider patroni.PatroniProvider
    ETCDProvider    etcd.ETCDProvider
}
```

Routes:
- `GET /health` → `{"status":"ok","timestamp":"..."}`
- `GET /metrics/os` → call `CollectOS(cfg.MountPoints)`, return JSON
- `GET /metrics/cluster` → collect Patroni + ETCD, return JSON

For `/metrics/os`: if `CollectOS` returns `ErrOSMetricsUnavailable`, return 200 with
`{"available": false, "reason": "OS metrics not available on this platform"}` — not a 5xx.

For `/metrics/cluster`: if both Patroni and ETCD return `ErrNotConfigured`, return 200
with `{"patroni": null, "etcd": null}`.

**Task 5: Create internal/agent/scraper.go**

```go
type Scraper struct {
    baseURL    string
    httpClient *http.Client
}

func NewScraper(baseURL string) *Scraper
func (s *Scraper) ScrapeOS(ctx context.Context) (*OSSnapshot, error)
func (s *Scraper) ScrapeCluster(ctx context.Context) (*ClusterSnapshot, error)
func (s *Scraper) IsAlive(ctx context.Context) bool
```

HTTP GET with 10s timeout. Parse JSON into OSSnapshot. Return error on non-200 or parse failure.

**Task 6: Create cmd/pgpulse-agent/main.go**

Load config from `configs/pgpulse-agent.yml` (using koanf, same pattern as main server).
Build PatroniProvider and ETCDProvider using `NewProvider` from each package.
Start `agent.Server`. Handle SIGINT/SIGTERM gracefully.

**Task 7: Write unit tests**

Create `internal/agent/osmetrics_test.go`:
- `TestParseMeminfo` — parse a known /proc/meminfo string, assert fields
- `TestCPUDelta` — feed two cpuRaw readings, assert delta calculations
- `TestDiskStatsDelta` — feed two diskStatRaw readings, assert await and util
- `TestParseOSRelease_Ubuntu` — feed Ubuntu os-release content, assert Name/Version/ID
- `TestParseOSRelease_RHEL` — feed RHEL os-release content

Create `internal/agent/scraper_test.go`:
- `TestScraper_ScrapeOS_Success` — httptest server returns valid JSON, assert parsed correctly
- `TestScraper_ScrapeOS_Non200` — httptest server returns 503, assert error
- `TestScraper_IsAlive_True` — /health returns 200
- `TestScraper_IsAlive_False` — /health returns 503 or connection refused

---

### SPECIALIST 2 — CLUSTER PROVIDERS (Patroni + ETCD)

**Your scope:**
- `internal/cluster/patroni/provider.go`
- `internal/cluster/patroni/rest.go`
- `internal/cluster/patroni/shell.go`
- `internal/cluster/patroni/fallback.go`
- `internal/cluster/patroni/noop.go`
- `internal/cluster/etcd/provider.go`
- `internal/cluster/etcd/http.go`
- `internal/cluster/etcd/shell.go`
- `internal/cluster/etcd/fallback.go`
- `internal/cluster/etcd/noop.go`

**Task 1: Create internal/cluster/patroni/provider.go**

Define:
- `PatroniConfig` struct: PatroniURL, PatroniConfig, PatroniCtlPath string
- `ClusterMember`, `ClusterState`, `SwitchoverEvent` types (from design doc)
- `PatroniProvider` interface: GetClusterState, GetHistory, GetVersion
- Errors: `ErrPatroniNotConfigured`, `ErrPatroniUnavailable`
- `NewProvider(cfg PatroniConfig) PatroniProvider` factory function

**Task 2: Create internal/cluster/patroni/rest.go**

`RESTProvider` — calls Patroni's HTTP API:
- Base URL: `cfg.PatroniURL` (e.g., "http://localhost:8008")
- `GET {base}/cluster` → parse JSON into ClusterState
  Patroni cluster endpoint returns: `{"members": [{"name":"...","host":"...","port":5432,"role":"leader","state":"running","timeline":1,"lag_in_mb":0}]}`
- `GET {base}/history` → parse JSON array into []SwitchoverEvent
- `GET {base}/` → parse `{"patroni":{"version":"3.0.4",...}}` for version
- HTTP client timeout: 5s
- On non-200: return ErrPatroniUnavailable with status code in message

**Task 3: Create internal/cluster/patroni/shell.go**

`ShellProvider` — uses os/exec:
- `patronictl -c {config} list -f json` for cluster state
  Parse JSON output. Patronictl JSON format may differ slightly from REST — map to same ClusterMember type.
- `patronictl -c {config} history` for history (parse tabular output if no JSON flag)
- `patronictl version` for version (parse first line)
- Binary path defaults to `/usr/bin/patronictl` if PatroniCtlPath is empty
- Config path required for list/history; if empty, omit `-c` flag (patronictl may find it automatically)
- Timeout: 10s per command
- If binary doesn't exist (os.Stat fails): return ErrPatroniUnavailable immediately (no exec)

**Task 4: Create internal/cluster/patroni/fallback.go**

```go
type FallbackProvider struct {
    primary   PatroniProvider
    secondary PatroniProvider
}

func NewFallbackProvider(primary, secondary PatroniProvider) *FallbackProvider

// For each method: try primary; if error, try secondary; if both fail, return secondary's error
func (f *FallbackProvider) GetClusterState(ctx context.Context) (*ClusterState, error)
func (f *FallbackProvider) GetHistory(ctx context.Context) ([]SwitchoverEvent, error)
func (f *FallbackProvider) GetVersion(ctx context.Context) (string, error)
```

**Task 5: Create internal/cluster/patroni/noop.go**

All methods return `ErrPatroniNotConfigured`. Used when PatroniURL and PatroniCtlPath are both empty.

**Task 6: Repeat pattern for ETCD (internal/cluster/etcd/)**

`ETCDConfig` struct: Endpoints []string, CtlPath string
`ETCDMember`, `ETCDEndpointStatus` types (from design doc)
`ETCDProvider` interface: GetMembers, GetEndpointHealth
Errors: `ErrETCDNotConfigured`, `ErrETCDUnavailable`

`HTTPProvider` — etcd v3 HTTP API:
- `GET {endpoint}/v3/cluster/member/list` (etcd v3 gRPC-gateway) or
  `GET {endpoint}/members` (etcd v2 API, simpler to parse)
  Use etcd v2 format: `GET http://{endpoint}/members` returns `{"members":[{"id":"...","name":"...","peerURLs":[...],"clientURLs":[...]}]}`
- `GET {endpoint}/health` returns `{"health":"true"}`

`ShellProvider` — `etcdctl member list --write-out=json`

`FallbackProvider` and `NoOpProvider` — same pattern as Patroni.

**Task 7: Write unit tests**

`internal/cluster/patroni/fallback_test.go`:
- `TestFallbackProvider_PrimarySucceeds` — primary called, secondary not called
- `TestFallbackProvider_PrimaryFails_SecondaryCalledAndSucceeds`
- `TestFallbackProvider_BothFail_SecondaryErrorReturned`

`internal/cluster/patroni/rest_test.go`:
- `TestRESTProvider_GetClusterState_Success` — httptest server, mock Patroni JSON
- `TestRESTProvider_GetClusterState_Non200` — assert ErrPatroniUnavailable

`internal/cluster/etcd/fallback_test.go` — same three cases as Patroni fallback

---

### SPECIALIST 3 — COLLECTOR WIRING + CONFIG + FRONTEND

**Your scope:**
- `internal/collector/os.go`
- `internal/collector/cluster.go`
- `internal/config/config.go` (add new fields only, no renames)
- `internal/orchestrator/runner.go` (add OS and cluster collector construction)
- `internal/api/instances.go` (add agent_available to response)
- `web/src/` (new sections on ServerDetail page)

**Task 1: Extend internal/config/config.go**

Add to `InstanceConfig` the 7 new fields from the design doc:
AgentURL, PatroniURL, PatroniConfig, PatroniCtlPath, ETCDEndpoints, ETCDCtlPath.
Add `AgentConfig` struct as a new top-level config section.
Do not rename or remove any existing fields.

**Task 2: Create internal/collector/os.go**

`OSCollector` with three sources (None/Local/Agent) as specified in design doc.
`isLocalHost(dsn string) bool` — parse the DSN host field; return true if it is
"localhost", "127.0.0.1", "::1", or matches `os.Hostname()`.

`snapshotToMetricPoints(snap *agent.OSSnapshot, instanceID string) []MetricPoint` —
convert OSSnapshot fields to MetricPoints with appropriate metric names:
```
os.cpu.user_pct, os.cpu.system_pct, os.cpu.iowait_pct, os.cpu.idle_pct
os.memory.total_kb, os.memory.available_kb, os.memory.used_kb
os.memory.commit_limit_kb, os.memory.committed_as_kb
os.load.1m, os.load.5m, os.load.15m
os.uptime_seconds
os.disk.total_bytes{mount="/"}, os.disk.used_bytes{mount="/"}, ...
os.disk.inodes_total{mount="/"}, os.disk.inodes_used{mount="/"}
os.diskstat.reads_completed{device="sda"}, os.diskstat.writes_completed{device="sda"}
os.diskstat.read_kb{device="sda"}, os.diskstat.write_kb{device="sda"}
os.diskstat.read_await_ms{device="sda"}, os.diskstat.write_await_ms{device="sda"}
os.diskstat.util_pct{device="sda"}
```

**Task 3: Create internal/collector/cluster.go**

`ClusterCollector` wraps PatroniProvider and ETCDProvider.
On `Collect()`:
- Call `patroni.GetClusterState()` — on ErrPatroniNotConfigured, skip silently; on other errors, log WARN and continue
- Call `etcd.GetMembers()` — same error handling
- Convert results to MetricPoints:
  ```
  cluster.patroni.member_count
  cluster.patroni.leader_count  (count members with role="leader")
  cluster.patroni.replica_count
  cluster.patroni.member_state{member="name", role="replica"} = 1 if running, 0 if not
  cluster.patroni.member_lag_bytes{member="name"}
  cluster.etcd.member_count
  cluster.etcd.leader_count
  cluster.etcd.member_healthy{member="name"} = 1/0
  ```

**Task 4: Update internal/orchestrator/runner.go**

In the function that builds collectors for a new runner, add:
```go
collectors = append(collectors,
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
)
```

**Task 5: Extend instance API response**

In `internal/api/instances.go`, add `AgentAvailable bool` to the detail response struct.
Set it to true if `cfg.AgentURL != ""` OR if the instance is detected as local.
This allows the frontend to show the "no agent" placeholder correctly.

**Task 6: Frontend — new sections on ServerDetail page**

In `web/src/pages/ServerDetail.tsx` (or the components it uses), add 4 new sections.
All sections are collapsible (same collapse pattern as existing sections).

**Section: System** (between existing "Server Info" and "Connections" sections, or at top)
Show: Hostname, OS name+version, Uptime (format as "X days, Y hours"), CPU count
CPU usage bar (colored: green <60%, yellow 60–80%, red >80%)
Memory usage bar (same coloring), show GB values
Load averages: "0.45 / 0.38 / 0.31 (1m/5m/15m)"

**Section: Disk Usage**
Table: Mount | Device | Type | Used | Total | Use% (bar) | Inodes
Sort by mount path alphabetically.
Highlight rows >80% disk use in yellow, >90% in red.

**Section: I/O Stats**
Table: Device | Reads/s | Writes/s | Read MB/s | Write MB/s | R-Await | W-Await | Util%
Highlight rows >80% util in yellow, >95% in red.

**Section: HA Cluster (Patroni)**
If Patroni data present:
  Table: Member | Role | State | Timeline | Lag
  Role badge: Leader=blue, Replica=gray, Standby Leader=orange
  State: running=green badge, other=red badge
  Lag: format bytes (B/KB/MB)
If Patroni not configured: small inline note "Patroni not configured for this instance"

**No-agent placeholder:**
If `instance.agent_available === false`, show the placeholder described in requirements.md
for all four new sections (single placeholder card, not four separate ones).

Add new TypeScript types to `web/src/types/models.ts`:
```typescript
export interface OSMetrics {
  collected_at: string;
  hostname: string;
  os_release: { name: string; version: string; id: string };
  uptime_seconds: number;
  load_avg: { '1m': number; '5m': number; '15m': number };
  memory: { total_kb: number; available_kb: number; used_kb: number; commit_limit_kb: number; committed_as_kb: number };
  cpu: { user_pct: number; system_pct: number; iowait_pct: number; idle_pct: number; num_cpus: number };
  disks: DiskInfo[];
  diskstats: DiskStatInfo[];
}

export interface DiskInfo {
  mount: string; device: string; fstype: string;
  total_bytes: number; used_bytes: number; free_bytes: number;
  inodes_total: number; inodes_used: number;
}

export interface DiskStatInfo {
  device: string;
  reads_completed: number; writes_completed: number;
  read_kb: number; write_kb: number;
  io_in_progress: number;
  read_await_ms: number; write_await_ms: number; util_pct: number;
}

export interface ClusterMetrics {
  patroni: PatroniClusterState | null;
  etcd: ETCDState | null;
}

export interface PatroniClusterState {
  cluster_name: string;
  members: PatroniMember[];
}

export interface PatroniMember {
  name: string; host: string; port: number;
  role: string; state: string; timeline: number; lag: number;
}
```

Update the metrics API hook to parse `os` and `cluster` keys from the response.

**Task 7: Write frontend smoke tests**

In `web/src/components/server/` (or wherever existing server sections live):
- `OSSystemSection.test.tsx` — renders with sample data, renders placeholder when no agent
- `DiskSection.test.tsx` — renders disk table
- `ClusterSection.test.tsx` — renders Patroni members table, renders "not configured" state

---

## Coordination and Dependencies

```
Specialist 1 (Agent + procfs)   ←─── independent, start immediately
Specialist 2 (Cluster providers) ←── independent, start immediately
Specialist 3 (Wiring + Frontend) ←── depends on Spec 1 types (OSSnapshot)
                                       and Spec 2 interfaces (PatroniProvider, ETCDProvider)
                                       Wait for Spec 1 + Spec 2 to define their public types,
                                       then proceed in parallel
```

Team Lead: merge Spec 1 and Spec 2 first, then Spec 3.

## Build Verification

```bash
# Build both binaries
go build ./cmd/pgpulse-server
go build ./cmd/pgpulse-agent

# Full test suite (unit tests only, no integration tag)
go test ./cmd/... ./internal/...

# Frontend
cd web && npm run build && npm run lint && npm run typecheck

# Linter
golangci-lint run
```

All six commands must pass with zero errors before committing.

## Commit Messages

```
feat(agent): add pgpulse-agent binary with procfs OS metrics collection
feat(cluster): add Patroni and ETCD Smart Provider with REST+shell fallback
feat(collector): add OS and cluster collectors with graceful degradation
feat(config): add agent_url, patroni_url, etcd_endpoints per-instance config
feat(ui): add OS system, disk, I/O, and cluster sections to server detail page
```
