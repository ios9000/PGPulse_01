# M8_11 Team Prompt — OS Metrics via PostgreSQL

Read CLAUDE.md and docs/iterations/M8_11_*/design.md for full context.

**CRITICAL STEP 0 — before writing ANY code:**
Read `internal/agent/` and `web/src/` to inventory the exact metric keys the agent
emits and the frontend consumes for OS metrics. The os_sql collector MUST emit
identical keys. Document the key mapping in a comment at the top of os_sql.go.
Also read `internal/collector/server_info.go` and `internal/orchestrator/runner.go`
to understand the existing patterns before modifying them.

Create a team of 2 specialists:

---

## COLLECTOR AGENT

### Step 0: Inventory existing metric keys
- Read `internal/agent/` — find every `os.*` metric key
- Read `web/src/` — grep for `os.memory`, `os.cpu`, `os.load`, `os.disk` to find consuming components
- Document the complete key list — the os_sql collector must match exactly

### Step 1: Create `internal/collector/os_sql.go`

Implement `OSSQLCollector` with the `Collector` interface:
- `Name()` → `"os_sql"`
- `Interval()` → `60 * time.Second` (medium group)
- `Collect(ctx, conn, ic)` → reads 5 proc files, returns []MetricPoint

**Helper: readProcFile(ctx, conn, path)**
```go
func (c *OSSQLCollector) readProcFile(ctx context.Context, conn *pgx.Conn, path string) (string, error) {
    var content string
    err := conn.QueryRow(ctx, "SELECT pg_read_file($1)", path).Scan(&content)
    if err != nil {
        slog.Warn("os_sql: cannot read proc file", "path", path, "instance", c.instanceID, "error", err)
        return "", err
    }
    return content, nil
}
```

**Readers (each called from Collect, each returns nil,nil on failure):**

1. `readMeminfo(ctx, conn)` — parse `/proc/meminfo`
   - Parse key:value lines (e.g., "MemTotal:       16384000 kB")
   - Emit: `os.memory.total_kb`, `os.memory.free_kb`, `os.memory.available_kb`,
     `os.memory.buffers_kb`, `os.memory.cached_kb`, `os.memory.swap_total_kb`,
     `os.memory.swap_free_kb`, `os.memory.committed_kb`
   - Derive: `os.memory.used_kb` = MemTotal - MemFree - Buffers - Cached
   - **Match the exact keys from the agent** (discovered in Step 0)

2. `readUptime(ctx, conn)` — parse `/proc/uptime`
   - Two space-separated floats: uptime_seconds idle_seconds
   - Emit: `os.uptime.seconds`

3. `readLoadAvg(ctx, conn)` — parse `/proc/loadavg`
   - "0.35 0.22 0.18 2/245 12345"
   - Emit: `os.load.1m`, `os.load.5m`, `os.load.15m`

4. `readCPUStat(ctx, conn)` — parse `/proc/stat`, stateful delta
   - Read first line: "cpu  user nice system idle iowait irq softirq steal ..."
   - Store as cpuSnapshot. On second+ cycle, compute deltas:
     total_delta = sum(all deltas); pct = (field_delta / total_delta) * 100
   - Emit: `os.cpu.user_pct`, `os.cpu.system_pct`, `os.cpu.idle_pct`,
     `os.cpu.iowait_pct`, `os.cpu.steal_pct`
   - First cycle: store snapshot, return nil (no baseline)
   - Use sync.Mutex to protect prevCPU

5. `readDiskStats(ctx, conn)` — parse `/proc/diskstats`, stateful delta
   - One line per device: "   8  0 sda reads ... sectors_read ... writes ... sectors_written ... io_millis ..."
   - Filter: only whole-disk devices matching `sd[a-z]+`, `nvme[0-9]+n[0-9]+`, `vd[a-z]+`, `xvd[a-z]+`
   - Skip partitions (sda1), virtual (loop*, dm-*)
   - Delta per device: rates = delta / elapsed_seconds; sector_size = 512 bytes
   - Emit per device (label: device=sda):
     `os.disk.read_bytes_per_sec`, `os.disk.write_bytes_per_sec`,
     `os.disk.read_iops`, `os.disk.write_iops`, `os.disk.io_util_pct`
   - First cycle: store snapshot, return nil
   - Use sync.Mutex to protect prevDisk

### Step 2: Modify `internal/collector/server_info.go`

Add hostname and OS distribution reads (these run at LOW interval, 300s):

```go
// After existing server_info metrics:
hostname, err := c.readProcFile(ctx, conn, "/etc/hostname")
if err == nil {
    points = append(points, MetricPoint{
        InstanceID: c.instanceID, Metric: "pgpulse.server.hostname",
        Value: 0, Labels: map[string]string{"hostname": strings.TrimSpace(hostname)},
        Timestamp: time.Now(),
    })
}

osRelease, err := c.readProcFile(ctx, conn, "/etc/os-release")
if err == nil {
    prettyName := parseOSReleasePrettyName(osRelease) // parse PRETTY_NAME="..."
    points = append(points, MetricPoint{
        InstanceID: c.instanceID, Metric: "pgpulse.server.os",
        Value: 0, Labels: map[string]string{"os": prettyName},
        Timestamp: time.Now(),
    })
}
```

Use the same `readProcFile` helper (extract to shared helper or duplicate — your call).

### Step 3: Modify `internal/orchestrator/runner.go`

Add OS method resolution and conditional registration:

```go
func (r *instanceRunner) resolveOSMethod() string {
    if r.cfg.OSMetricsMethod != "" {
        return r.cfg.OSMetricsMethod
    }
    if r.globalCfg.OSMetrics.Method != "" {
        return r.globalCfg.OSMetrics.Method
    }
    return "sql" // default
}
```

In `buildCollectors()`, add to medium group:
```go
osMethod := r.resolveOSMethod()
if osMethod == "sql" {
    medium = append(medium, collector.NewOSSQLCollector(id, v))
}
```

### Step 4: Modify `internal/config/config.go`

Add config structs:
```go
type OSMetricsConfig struct {
    Method string `yaml:"method" koanf:"method"` // "sql", "agent", "disabled"
}
```

Add `OSMetrics OSMetricsConfig` to the global config struct.
Add `OSMetricsMethod string` to the instance config struct.

### Step 5: Update `configs/pgpulse.example.yml`

Add:
```yaml
# OS metrics collection method (per-instance overridable)
# "sql" — collect via pg_read_file('/proc/*'), requires pg_read_server_files grant
# "agent" — collect via pgpulse-agent binary (requires agent deployment)
# "disabled" — no OS metrics
os_metrics:
  method: "sql"
```

---

## QA & REVIEW AGENT

### Step 1: Create `internal/collector/os_sql_test.go`

Unit tests for all parsers (pure Go, no database needed):

1. `TestParseMeminfo` — feed sample /proc/meminfo content, verify all 9 metric keys + values
2. `TestParseMeminfo_PartialContent` — missing MemAvailable (older kernels), verify graceful skip
3. `TestParseUptime` — feed "12345.67 98765.43", verify os.uptime.seconds = 12345.67
4. `TestParseLoadAvg` — feed "0.35 0.22 0.18 2/245 12345", verify 3 load metrics
5. `TestParseCPUStat_FirstCycle` — first call returns nil (no baseline)
6. `TestParseCPUStat_DeltaCalculation` — two snapshots with known deltas, verify percentages
7. `TestParseCPUStat_AllIdle` — all-idle scenario, verify idle_pct ≈ 100
8. `TestParseDiskStats_DeviceFilter` — verify sda included, sda1 excluded, loop0 excluded, nvme0n1 included
9. `TestParseDiskStats_FirstCycle` — first call returns nil
10. `TestParseDiskStats_DeltaCalculation` — two snapshots, verify bytes/sec and IOPS
11. `TestReadProcFile_PermissionDenied` — mock pgx error, verify nil,nil returned (not error)

### Step 2: Verify metric key compatibility

- Grep `internal/agent/` for all `os.*` metric keys
- Grep `web/src/` for all `os.*` metric key references
- Verify os_sql.go emits identical keys
- Report any mismatches

### Step 3: Build verification

```bash
cd web && npm run build && npm run lint && npm run typecheck
cd .. && go build ./cmd/pgpulse-server && go test ./cmd/... ./internal/...
golangci-lint run
```

### Step 4: Security check

- Verify no string concatenation in SQL (all pg_read_file calls use $1 parameter)
- Verify no COPY TO PROGRAM anywhere in the codebase
- Verify readProcFile uses parameterized query

---

## Coordination

- Collector Agent starts immediately (Step 0 inventory, then implementation)
- QA Agent writes test stubs during Step 0, fills assertions once parsers land
- Merge only when all tests pass and build verification is clean
- After merge: generate docs/CODEBASE_DIGEST.md per .claude/rules/codebase-digest.md
