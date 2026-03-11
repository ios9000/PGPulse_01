# M8_11 Technical Design — OS Metrics via PostgreSQL

**Iteration:** M8_11
**Date:** 2026-03-10
**Author:** Claude.ai (Brain contour)

---

## Design Decisions

| # | Decision | Rationale |
|---|----------|-----------|
| D-M8_11-1 | Medium interval (60s) | Matches agent cycle; CPU/load changes meaningfully at this scale |
| D-M8_11-2 | Single OSSQLCollector with stateful delta fields | 5 pg_read_file calls cheap as batch; one struct holds CPU/disk previous state |
| D-M8_11-3 | Individual pg_read_file() per file, not batched SELECT | Per-file graceful fallback; if one fails, others still collected |
| D-M8_11-4 | Hostname + OS distribution → server_info collector (existing) | Metadata, not time-series; server_info already runs at low interval |
| D-M8_11-5 | Per-instance os_metrics_method config with global default | Different instances may be on different platforms (Linux vs container) |
| D-M8_11-6 | Agent inventories existing metric keys as Step 0 | Ensures frontend compatibility without manual key mapping |
| D-M8_11-7 | First Codebase Digest at end of iteration | Establishes the code map for all future planning chats |

---

## 1. Collector Design

### 1.1 OSSQLCollector struct

```go
// internal/collector/os_sql.go

type OSSQLCollector struct {
    instanceID string
    pgVersion  version.PGVersion

    mu            sync.Mutex
    prevCPU       *cpuSnapshot    // nil on first cycle
    prevDisk      *diskSnapshot   // nil on first cycle
    prevTimestamp  time.Time
}

type cpuSnapshot struct {
    User    uint64
    Nice    uint64
    System  uint64
    Idle    uint64
    IOWait  uint64
    IRQ     uint64
    SoftIRQ uint64
    Steal   uint64
}

type diskSnapshot struct {
    Devices map[string]diskDeviceCounters // keyed by device name
}

type diskDeviceCounters struct {
    ReadsCompleted  uint64
    SectorsRead     uint64
    WritesCompleted uint64
    SectorsWritten  uint64
    IOMillis        uint64 // time spent doing I/O (ms)
}
```

### 1.2 Collect() flow

```
Collect(ctx, conn, ic) → []MetricPoint, error
  │
  ├── readMeminfo(ctx, conn)      → memPoints, err   // /proc/meminfo
  ├── readUptime(ctx, conn)       → uptimePoints, err // /proc/uptime
  ├── readLoadAvg(ctx, conn)      → loadPoints, err   // /proc/loadavg
  ├── readCPUStat(ctx, conn)      → cpuPoints, err    // /proc/stat (stateful delta)
  └── readDiskStats(ctx, conn)    → diskPoints, err   // /proc/diskstats (stateful delta)
      │
      └── each returns nil, nil on permission error (logged as warning)
          aggregate all non-nil points into result slice
```

### 1.3 pg_read_file helper

```go
func (c *OSSQLCollector) readProcFile(ctx context.Context, conn *pgx.Conn, path string) (string, error) {
    var content string
    err := conn.QueryRow(ctx, "SELECT pg_read_file($1)", path).Scan(&content)
    if err != nil {
        // Permission denied, file not found, or non-Linux → warning, skip
        slog.Warn("os_sql: cannot read proc file", "path", path, "instance", c.instanceID, "error", err)
        return "", err
    }
    return content, nil
}
```

---

## 2. /proc Parsing

### 2.1 /proc/meminfo → memory metrics

Input format:
```
MemTotal:       16384000 kB
MemFree:         8192000 kB
MemAvailable:   12000000 kB
Buffers:          512000 kB
Cached:          3072000 kB
SwapTotal:       2097152 kB
SwapFree:        2097152 kB
Committed_AS:    4096000 kB
```

Parse: split lines by `:`, trim, extract key and numeric value. Map to metric keys:

| /proc/meminfo field | Metric Key |
|---|---|
| MemTotal | `os.memory.total_kb` |
| MemFree | `os.memory.free_kb` |
| MemAvailable | `os.memory.available_kb` |
| Buffers | `os.memory.buffers_kb` |
| Cached | `os.memory.cached_kb` |
| SwapTotal | `os.memory.swap_total_kb` |
| SwapFree | `os.memory.swap_free_kb` |
| Committed_AS | `os.memory.committed_kb` |

Derived: `os.memory.used_kb` = MemTotal - MemFree - Buffers - Cached

### 2.2 /proc/uptime → uptime metric

Input: `12345.67 98765.43` (uptime_seconds idle_seconds)

| Field | Metric Key |
|---|---|
| uptime_seconds | `os.uptime.seconds` |

### 2.3 /proc/loadavg → load metrics

Input: `0.35 0.22 0.18 2/245 12345`

| Field | Metric Key |
|---|---|
| load 1m | `os.load.1m` |
| load 5m | `os.load.5m` |
| load 15m | `os.load.15m` |

### 2.4 /proc/stat → CPU metrics (stateful)

Input (first line only — aggregate CPU):
```
cpu  12345 678 9012 345678 1234 0 567 89 0 0
```

Fields: user, nice, system, idle, iowait, irq, softirq, steal, guest, guest_nice

**Delta calculation:** Compute total_delta = sum(all field deltas). Each percentage = (field_delta / total_delta) * 100.

| Derived | Metric Key |
|---|---|
| user% | `os.cpu.user_pct` |
| system% | `os.cpu.system_pct` |
| idle% | `os.cpu.idle_pct` |
| iowait% | `os.cpu.iowait_pct` |
| steal% | `os.cpu.steal_pct` |

**First cycle:** Store snapshot, return no CPU metrics (no baseline).

### 2.5 /proc/diskstats → disk I/O metrics (stateful)

Input (one line per device):
```
   8       0 sda 12345 678 901234 5678 9012 345 678901 2345 0 6789 8024
```

Fields (0-indexed from device name): device(2), reads_completed(3), sectors_read(5), writes_completed(7), sectors_written(9), io_millis(12)

**Filter:** Only report devices matching common patterns: `sd[a-z]+`, `nvme[0-9]+n[0-9]+`, `vd[a-z]+`, `xvd[a-z]+`. Skip partitions (sda1, etc.) and virtual devices (loop, dm-).

**Delta calculation per device:** Rates = delta / elapsed_seconds. Sector size = 512 bytes.

| Derived | Metric Key | Labels |
|---|---|---|
| read bytes/sec | `os.disk.read_bytes_per_sec` | device=sda |
| write bytes/sec | `os.disk.write_bytes_per_sec` | device=sda |
| read IOPS | `os.disk.read_iops` | device=sda |
| write IOPS | `os.disk.write_iops` | device=sda |
| I/O utilization % | `os.disk.io_util_pct` | device=sda |

**First cycle:** Store snapshot, return no disk metrics (no baseline).

---

## 3. Configuration Wiring

### 3.1 Config struct addition

```go
// internal/config/ — add to existing config structs

type OSMetricsConfig struct {
    Method string `yaml:"method" koanf:"method"` // "sql" (default), "agent", "disabled"
}

// Global level
type Config struct {
    // ... existing fields
    OSMetrics OSMetricsConfig `yaml:"os_metrics" koanf:"os_metrics"`
}

// Per-instance override
type InstanceConfig struct {
    // ... existing fields
    OSMetricsMethod string `yaml:"os_metrics_method" koanf:"os_metrics_method"` // overrides global
}
```

### 3.2 Orchestrator registration

In `buildCollectors()`, check the resolved method:

```go
func (r *instanceRunner) resolveOSMethod() string {
    if r.cfg.OSMetricsMethod != "" {
        return r.cfg.OSMetricsMethod
    }
    return r.globalConfig.OSMetrics.Method // default: "sql"
}

// In buildCollectors():
osMethod := r.resolveOSMethod()
if osMethod == "sql" {
    medium = append(medium, collector.NewOSSQLCollector(id, v))
}
// "agent" and "disabled" → don't register
```

---

## 4. Server Info Enrichment

Add to existing `server_info.go` (low interval, 300s):

```go
// Read hostname
hostname, err := readProcFile(ctx, conn, "/etc/hostname")
if err == nil {
    points = append(points, MetricPoint{
        Metric: "pgpulse.server.hostname",
        Value:  0,
        Labels: map[string]string{"hostname": strings.TrimSpace(hostname)},
    })
}

// Read OS release
osRelease, err := readProcFile(ctx, conn, "/etc/os-release")
if err == nil {
    // Parse PRETTY_NAME="Ubuntu 24.04 LTS"
    prettyName := parseOSReleasePrettyName(osRelease)
    points = append(points, MetricPoint{
        Metric: "pgpulse.server.os",
        Value:  0,
        Labels: map[string]string{"os": prettyName},
    })
}
```

---

## 5. File Inventory

### New Files

| File | Lines est. | Purpose |
|------|-----------|---------|
| `internal/collector/os_sql.go` | ~350 | OSSQLCollector: 5 proc readers + parsers + delta logic |
| `internal/collector/os_sql_test.go` | ~200 | Unit tests for all parsers + delta calculations |

### Modified Files

| File | Change | Lines est. |
|------|--------|-----------|
| `internal/collector/server_info.go` | Add hostname + os-release reads | +30 |
| `internal/orchestrator/runner.go` | Add OS method resolution + conditional registration | +15 |
| `internal/config/config.go` | Add OSMetricsConfig struct + per-instance field | +10 |
| `configs/pgpulse.example.yml` | Add os_metrics section | +5 |

### Untouched

- `cmd/pgpulse-agent/` — agent binary remains as-is
- `internal/agent/` — agent code remains as-is
- `web/src/` — frontend remains as-is (same metric keys)

---

## 6. PGAM Query Mapping

| PGAM # | PGAM Method | PGPulse os_sql Replacement | Status |
|--------|-------------|---------------------------|--------|
| Q4 | `COPY TO PROGRAM 'hostname -f'` | `pg_read_file('/etc/hostname')` in server_info.go | New |
| Q5 | `pg_stat_file('/etc/lsb-release') + pg_read_file()` | `pg_read_file('/etc/os-release')` in server_info.go | New |
| Q8 | `COPY TO PROGRAM 'uptime'` | `pg_read_file('/proc/uptime')` + `/proc/loadavg` | New |
| Q9 | `COPY TO PROGRAM 'date'` | `clock_timestamp()` — already available via PG | Skip (covered) |
| Q12 | `COPY TO PROGRAM 'free -m'` | `pg_read_file('/proc/meminfo')` parse MemTotal | New |
| Q22 | `COPY TO PROGRAM 'cat /proc/meminfo \| grep Comm'` | Same `/proc/meminfo` parse | New |
| Q23 | `COPY TO PROGRAM 'grep /proc/meminfo && free -h'` | Same `/proc/meminfo` parse | New |
| Q24 | `COPY TO PROGRAM 'top -c -b -n 1'` | **Cannot replicate** — agent only | Skip |
| Q25 | `COPY TO PROGRAM 'df -Tih'` | **Partial** — PG tablespace sizes only | Skip |
| Q26 | `COPY TO PROGRAM 'iostat -Nmhx'` | `pg_read_file('/proc/diskstats')` | New |
| Q27 | `COPY TO PROGRAM 'iostat -kx 1 2'` | Same — delta between 60s cycles | New |

**Net effect:** 7 of 11 PGAM OS queries fully replaced via SQL. 2 skipped (top, df). 1 already covered (date). 1 partial.
