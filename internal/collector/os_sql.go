package collector

// OSSQLCollector — OS Metrics via PostgreSQL pg_read_file()
//
// Reads /proc files on the monitored server via pg_read_file(), parses them,
// and emits MetricPoints. Requires the pg_read_server_files grant or superuser.
//
// Metric Key Mapping (matches agent's snapshotToMetricPoints for compatibility):
//
//   /proc/meminfo:
//     os.memory.total_kb        — MemTotal
//     os.memory.available_kb    — MemAvailable (fallback: Free+Buffers+Cached)
//     os.memory.used_kb         — Total - Available
//     os.memory.commit_limit_kb — CommitLimit
//     os.memory.committed_as_kb — Committed_AS
//
//   /proc/uptime:
//     os.uptime_seconds         — first float (system uptime)
//
//   /proc/loadavg:
//     os.load.1m                — 1-minute load average
//     os.load.5m                — 5-minute load average
//     os.load.15m               — 15-minute load average
//
//   /proc/stat (stateful — delta between cycles):
//     os.cpu.user_pct           — user + nice %
//     os.cpu.system_pct         — system + irq + softirq + steal %
//     os.cpu.iowait_pct         — iowait %
//     os.cpu.idle_pct           — idle %
//
//   /proc/diskstats (stateful — delta between cycles):
//     os.diskstat.reads_completed  (label: device)
//     os.diskstat.writes_completed (label: device)
//     os.diskstat.read_kb          (label: device)
//     os.diskstat.write_kb         (label: device)
//     os.diskstat.read_await_ms    (label: device)
//     os.diskstat.write_await_ms   (label: device)
//     os.diskstat.util_pct         (label: device)
//
// Disk space metrics (os.disk.total_bytes, etc.) require syscall.Statfs
// and cannot be collected via pg_read_file — those remain agent-only.

import (
	"context"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/ios9000/PGPulse_01/internal/agent"
	"github.com/ios9000/PGPulse_01/internal/version"
)

// devicePattern matches whole-disk device names; partitions (sda1) are excluded.
var devicePattern = regexp.MustCompile(`^(sd[a-z]+|nvme\d+n\d+|vd[a-z]+|xvd[a-z]+)$`)

// OSSQLCollector collects OS metrics by reading /proc files via pg_read_file().
type OSSQLCollector struct {
	instanceID string
	pgVersion  version.PGVersion

	mu            sync.Mutex
	prevCPU       *agent.CPURaw
	prevDisk      map[string]agent.DiskStatRaw
	prevTimestamp time.Time
}

// NewOSSQLCollector creates an OSSQLCollector for the given instance.
func NewOSSQLCollector(instanceID string, v version.PGVersion) *OSSQLCollector {
	return &OSSQLCollector{
		instanceID: instanceID,
		pgVersion:  v,
	}
}

func (c *OSSQLCollector) Name() string            { return "os_sql" }
func (c *OSSQLCollector) Interval() time.Duration { return 60 * time.Second }

// Collect reads /proc files via pg_read_file and returns OS metric points.
func (c *OSSQLCollector) Collect(ctx context.Context, conn *pgx.Conn, _ InstanceContext) ([]MetricPoint, error) {
	var points []MetricPoint

	if pts := c.readMeminfo(ctx, conn); pts != nil {
		points = append(points, pts...)
	}
	if pts := c.readUptime(ctx, conn); pts != nil {
		points = append(points, pts...)
	}
	if pts := c.readLoadAvg(ctx, conn); pts != nil {
		points = append(points, pts...)
	}
	if pts := c.readCPUStat(ctx, conn); pts != nil {
		points = append(points, pts...)
	}
	if pts := c.readDiskStats(ctx, conn); pts != nil {
		points = append(points, pts...)
	}

	return points, nil
}

// readProcFile reads a file from the monitored server via pg_read_file.
func (c *OSSQLCollector) readProcFile(ctx context.Context, conn *pgx.Conn, path string) (string, error) {
	qctx, cancel := queryContext(ctx)
	defer cancel()
	var content string
	err := conn.QueryRow(qctx, "SELECT pg_read_file($1)", path).Scan(&content)
	if err != nil {
		slog.Warn("os_sql: cannot read proc file", "path", path, "instance", c.instanceID, "error", err)
		return "", err
	}
	return content, nil
}

// point creates a MetricPoint with the given metric key (no prefix added).
func (c *OSSQLCollector) point(metric string, value float64, labels map[string]string) MetricPoint {
	return MetricPoint{
		InstanceID: c.instanceID,
		Metric:     metric,
		Value:      value,
		Labels:     labels,
		Timestamp:  time.Now(),
	}
}

// readMeminfo parses /proc/meminfo and emits memory metrics.
func (c *OSSQLCollector) readMeminfo(ctx context.Context, conn *pgx.Conn) []MetricPoint {
	content, err := c.readProcFile(ctx, conn, "/proc/meminfo")
	if err != nil {
		return nil
	}

	mem := agent.ParseMeminfo(content)

	return []MetricPoint{
		c.point("os.memory.total_kb", float64(mem.TotalKB), nil),
		c.point("os.memory.available_kb", float64(mem.AvailableKB), nil),
		c.point("os.memory.used_kb", float64(mem.UsedKB), nil),
		c.point("os.memory.commit_limit_kb", float64(mem.CommitLimitKB), nil),
		c.point("os.memory.committed_as_kb", float64(mem.CommittedAsKB), nil),
	}
}

// readUptime parses /proc/uptime and emits the uptime metric.
func (c *OSSQLCollector) readUptime(ctx context.Context, conn *pgx.Conn) []MetricPoint {
	content, err := c.readProcFile(ctx, conn, "/proc/uptime")
	if err != nil {
		return nil
	}

	fields := strings.Fields(content)
	if len(fields) < 1 {
		slog.Warn("os_sql: unexpected /proc/uptime format", "instance", c.instanceID)
		return nil
	}
	uptime, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		slog.Warn("os_sql: parse uptime", "instance", c.instanceID, "error", err)
		return nil
	}

	return []MetricPoint{c.point("os.uptime_seconds", uptime, nil)}
}

// readLoadAvg parses /proc/loadavg and emits load average metrics.
func (c *OSSQLCollector) readLoadAvg(ctx context.Context, conn *pgx.Conn) []MetricPoint {
	content, err := c.readProcFile(ctx, conn, "/proc/loadavg")
	if err != nil {
		return nil
	}

	fields := strings.Fields(content)
	if len(fields) < 3 {
		slog.Warn("os_sql: unexpected /proc/loadavg format", "instance", c.instanceID)
		return nil
	}
	one, _ := strconv.ParseFloat(fields[0], 64)
	five, _ := strconv.ParseFloat(fields[1], 64)
	fifteen, _ := strconv.ParseFloat(fields[2], 64)

	return []MetricPoint{
		c.point("os.load.1m", one, nil),
		c.point("os.load.5m", five, nil),
		c.point("os.load.15m", fifteen, nil),
	}
}

// readCPUStat parses /proc/stat and computes CPU usage deltas.
// Returns nil on the first cycle (no baseline).
func (c *OSSQLCollector) readCPUStat(ctx context.Context, conn *pgx.Conn) []MetricPoint {
	content, err := c.readProcFile(ctx, conn, "/proc/stat")
	if err != nil {
		return nil
	}

	curr, err := agent.ParseCPURaw(content)
	if err != nil {
		slog.Warn("os_sql: parse /proc/stat", "instance", c.instanceID, "error", err)
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.prevCPU == nil {
		c.prevCPU = &curr
		return nil
	}

	// Use 0 for numCPUs — OSSQLCollector cannot determine remote CPU count from /proc/stat alone.
	info := agent.CPUDelta(*c.prevCPU, curr, 0)
	c.prevCPU = &curr

	return []MetricPoint{
		c.point("os.cpu.user_pct", info.UserPct, nil),
		c.point("os.cpu.system_pct", info.SystemPct, nil),
		c.point("os.cpu.iowait_pct", info.IOWaitPct, nil),
		c.point("os.cpu.idle_pct", info.IdlePct, nil),
	}
}

// readDiskStats parses /proc/diskstats and computes I/O deltas per device.
// Returns nil on the first cycle (no baseline).
// Only reports whole-disk devices (sda, nvme0n1, vda, xvda); skips partitions and virtual devices.
func (c *OSSQLCollector) readDiskStats(ctx context.Context, conn *pgx.Conn) []MetricPoint {
	content, err := c.readProcFile(ctx, conn, "/proc/diskstats")
	if err != nil {
		return nil
	}

	allDevices := agent.ParseDiskStats(content)
	now := time.Now()

	// Filter to whole-disk devices only.
	current := make(map[string]agent.DiskStatRaw, len(allDevices))
	for name, raw := range allDevices {
		if devicePattern.MatchString(name) {
			current[name] = raw
		}
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.prevDisk == nil {
		c.prevDisk = current
		c.prevTimestamp = now
		return nil
	}

	intervalMs := float64(now.Sub(c.prevTimestamp).Milliseconds())
	if intervalMs <= 0 {
		intervalMs = 1
	}

	var points []MetricPoint
	for name, curr := range current {
		prev, ok := c.prevDisk[name]
		if !ok {
			continue
		}
		info := agent.DiskStatsDelta(prev, curr, intervalMs)
		labels := map[string]string{"device": name}
		points = append(points,
			c.point("os.diskstat.reads_completed", float64(info.ReadsCompleted), labels),
			c.point("os.diskstat.writes_completed", float64(info.WritesCompleted), labels),
			c.point("os.diskstat.read_kb", float64(info.ReadKB), labels),
			c.point("os.diskstat.write_kb", float64(info.WriteKB), labels),
			c.point("os.diskstat.read_await_ms", info.ReadAwaitMs, labels),
			c.point("os.diskstat.write_await_ms", info.WriteAwaitMs, labels),
			c.point("os.diskstat.util_pct", info.UtilPct, labels),
		)
	}

	c.prevDisk = current
	c.prevTimestamp = now
	return points
}
