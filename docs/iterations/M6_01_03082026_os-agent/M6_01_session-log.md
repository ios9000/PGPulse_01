# Session: 2026-03-08 — M6_01 OS Agent

## Goal
Port all 19 PGAM OS/cluster queries (Q4–Q8, Q22–Q35) from COPY-TO-PROGRAM
to native Go. Ship pgpulse-agent binary, Patroni/ETCD Smart Provider pattern,
OS and cluster collectors, and frontend sections for System, Disk, I/O, Cluster.

## Agent Team Configuration
- Team Lead: Claude Code
- Specialists: 3 (Agent+procfs / Cluster providers / Wiring+frontend)
- Duration: ~20m 38s active agent time
- Build result: all green

## What Was Built

### Specialist 1 — OS Agent binary + procfs
New packages: `internal/agent/`
- `osmetrics.go` — OSSnapshot and all types (OSRelease, MemoryInfo, CPUInfo, DiskInfo, DiskStatInfo, LoadAvg)
- `osmetrics_linux.go` (build tag: linux) — CollectOS: hostname, /etc/os-release, /proc/uptime, /proc/loadavg, /proc/meminfo, /proc/stat (CPU delta), syscall.Statfs (disks), /proc/diskstats (delta)
- `osmetrics_stub.go` (build tag: !linux) — returns ErrOSMetricsUnavailable, no panic on Windows
- `server.go` — chi HTTP server: GET /health, GET /metrics/os, GET /metrics/cluster
- `scraper.go` — HTTP client (10s timeout): ScrapeOS, ScrapeCluster, IsAlive
- `cmd/pgpulse-agent/main.go` — binary entrypoint, koanf config, signal handling
- `configs/pgpulse-agent.example.yml`
- `deploy/systemd/pgpulse-agent.service`

Unit tests: TestParseMeminfo, TestCPUDelta, TestDiskStatsDelta,
TestParseOSRelease_Ubuntu, TestParseOSRelease_RHEL,
TestScraper_ScrapeOS_Success/Non200, TestScraper_IsAlive_True/False

### Specialist 2 — Cluster providers (Patroni + ETCD)
New packages: `internal/cluster/patroni/`, `internal/cluster/etcd/`

Patroni:
- `provider.go` — PatroniProvider interface, ClusterMember, ClusterState, SwitchoverEvent types, NewProvider factory
- `rest.go` — RESTProvider (GET /cluster, /history, /) with 5s timeout
- `shell.go` — ShellProvider (patronictl list -f json, history, version via os/exec)
- `fallback.go` — FallbackProvider: REST → shell transparent fallback
- `noop.go` — NoOpProvider when Patroni not configured

ETCD: same pattern (HTTPProvider, ShellProvider, FallbackProvider, NoOpProvider)

Unit tests: TestFallbackProvider_PrimarySucceeds/PrimaryFails/BothFail (Patroni + ETCD),
TestRESTProvider_GetClusterState_Success/Non200, TestHTTPProvider mock tests

### Specialist 3 — Collector wiring + config + frontend
- `internal/collector/os.go` — OSCollector with OSSourceNone/Local/Agent, isLocalHost DSN parse, snapshotToMetricPoints (25 metric names with labels)
- `internal/collector/cluster.go` — ClusterCollector wrapping PatroniProvider + ETCDProvider; errors → WARN log, not returned
- `internal/config/config.go` — InstanceConfig extended: AgentURL, PatroniURL, PatroniConfig, PatroniCtlPath, ETCDEndpoints, ETCDCtlPath; new AgentConfig top-level section
- `internal/orchestrator/runner.go` — OSCollector and ClusterCollector added to per-instance collector set
- `internal/api/instances.go` — AgentAvailable bool added to instance detail response
- `web/src/components/server/DiskSection.tsx` — mount table, usage bars (yellow >80%, red >90%)
- `web/src/components/server/IOStatsSection.tsx` — device I/O table, util% color coding
- `web/src/components/server/ClusterSection.tsx` — Patroni member table + ETCD table, hidden when not configured
- `web/src/pages/ServerDetail.tsx` — wired all 4 new sections below InstanceAlerts

### Lint fixes caught during session (5)
- 4 unchecked resp.Body.Close() in scraper.go, etcd/http.go, patroni/rest.go
- Unused cpuRaw.active() method in osmetrics.go
- Unchecked listener.Close() in scraper_test.go

## Architecture Decisions
- D-M6-01: Option C deployment — optional agent, graceful degradation (OSSourceNone returns nil, nil)
- D-M6-02: FallbackProvider REST→shell pattern for both Patroni and ETCD
- D-M6-03: Build tags linux/!linux for all procfs code — dev machine is Windows
- D-M6-04: CPU% and disk I/O await require delta state — agent maintains package-level previous snapshots
- D-M6-05: ClusterCollector errors are WARN-level, not propagated — partial data is acceptable
- D-M6-06: isLocalHost compares DSN host against os.Hostname() for same-host auto-detection

## Build Verification
- `go build ./...` ✅ (both binaries: pgpulse-server + pgpulse-agent)
- `go test ./internal/agent/... ./internal/cluster/... ./internal/api/... ./internal/config/...` ✅
- `golangci-lint run ./...` ✅ 0 issues
- `npm run build` ✅ (frontend clean)

## What's Next
M7_01 — Per-Database Analysis. Port analiz_db.php Q1–Q18 (18 queries):
bloat estimation, vacuum health, index usage, TOAST sizes, schema sizes,
large objects, function stats, sequences, unlogged tables.
