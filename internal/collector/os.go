package collector

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/ios9000/PGPulse_01/internal/agent"
)

// OSCollectorSource determines how OS metrics are obtained.
type OSCollectorSource int

const (
	// OSSourceNone means no OS data is available.
	OSSourceNone OSCollectorSource = iota
	// OSSourceLocal means read local /proc directly.
	OSSourceLocal
	// OSSourceAgent means scrape a remote pgpulse-agent.
	OSSourceAgent
)

// OSCollector collects OS-level metrics from a pgpulse-agent or local procfs.
type OSCollector struct {
	instanceID string
	source     OSCollectorSource
	scraper    *agent.Scraper
}

// NewOSCollector creates an OSCollector.
// If agentURL is set, it scrapes the remote agent.
// If the instance DSN points to localhost, it reads local procfs.
// Otherwise, no OS metrics are collected (graceful degradation).
func NewOSCollector(instanceID, dsn, agentURL string) *OSCollector {
	if agentURL != "" {
		return &OSCollector{
			instanceID: instanceID,
			source:     OSSourceAgent,
			scraper:    agent.NewScraper(agentURL),
		}
	}
	if isLocalHost(dsn) {
		return &OSCollector{
			instanceID: instanceID,
			source:     OSSourceLocal,
		}
	}
	return &OSCollector{
		instanceID: instanceID,
		source:     OSSourceNone,
	}
}

func (c *OSCollector) Name() string                { return "os" }
func (c *OSCollector) Interval() time.Duration     { return 10 * time.Second }
func (c *OSCollector) Source() OSCollectorSource    { return c.source }

// Collect gathers OS metrics and converts them to MetricPoints.
func (c *OSCollector) Collect(ctx context.Context, _ *pgx.Conn, _ InstanceContext) ([]MetricPoint, error) {
	switch c.source {
	case OSSourceNone:
		return nil, nil
	case OSSourceAgent:
		snap, err := c.scraper.ScrapeOS(ctx)
		if err != nil {
			return nil, fmt.Errorf("os scraper: %w", err)
		}
		return snapshotToMetricPoints(snap, c.instanceID), nil
	case OSSourceLocal:
		snap, err := agent.CollectOS(nil)
		if err != nil {
			return nil, fmt.Errorf("local os: %w", err)
		}
		return snapshotToMetricPoints(snap, c.instanceID), nil
	}
	return nil, nil
}

// isLocalHost checks whether the DSN host refers to the local machine.
func isLocalHost(dsn string) bool {
	host := extractDSNHost(dsn)
	if host == "" {
		return false
	}
	lower := strings.ToLower(host)
	if lower == "localhost" || lower == "127.0.0.1" || lower == "::1" {
		return true
	}
	hostname, err := os.Hostname()
	if err != nil {
		return false
	}
	return strings.EqualFold(host, hostname)
}

// extractDSNHost extracts the host from a DSN, supporting both URL and key=value formats.
func extractDSNHost(dsn string) string {
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		u, err := url.Parse(dsn)
		if err != nil {
			return ""
		}
		return u.Hostname()
	}
	// key=value format
	for _, part := range strings.Fields(dsn) {
		if strings.HasPrefix(part, "host=") {
			return strings.TrimPrefix(part, "host=")
		}
	}
	return ""
}

// snapshotToMetricPoints converts an OSSnapshot to MetricPoints.
func snapshotToMetricPoints(snap *agent.OSSnapshot, instanceID string) []MetricPoint {
	if snap == nil {
		return nil
	}
	now := snap.CollectedAt
	if now.IsZero() {
		now = time.Now()
	}

	point := func(metric string, value float64, labels map[string]string) MetricPoint {
		return MetricPoint{
			InstanceID: instanceID,
			Metric:     metric,
			Value:      value,
			Labels:     labels,
			Timestamp:  now,
		}
	}

	var points []MetricPoint

	// CPU
	points = append(points,
		point("os.cpu.user_pct", snap.CPU.UserPct, nil),
		point("os.cpu.system_pct", snap.CPU.SystemPct, nil),
		point("os.cpu.iowait_pct", snap.CPU.IOWaitPct, nil),
		point("os.cpu.idle_pct", snap.CPU.IdlePct, nil),
	)

	// Memory
	points = append(points,
		point("os.memory.total_kb", float64(snap.Memory.TotalKB), nil),
		point("os.memory.available_kb", float64(snap.Memory.AvailableKB), nil),
		point("os.memory.used_kb", float64(snap.Memory.UsedKB), nil),
		point("os.memory.commit_limit_kb", float64(snap.Memory.CommitLimitKB), nil),
		point("os.memory.committed_as_kb", float64(snap.Memory.CommittedAsKB), nil),
	)

	// Load
	points = append(points,
		point("os.load.1m", snap.LoadAvg.One, nil),
		point("os.load.5m", snap.LoadAvg.Five, nil),
		point("os.load.15m", snap.LoadAvg.Fifteen, nil),
	)

	// Uptime
	points = append(points, point("os.uptime_seconds", snap.UptimeSecs, nil))

	// Disks
	for _, d := range snap.Disks {
		labels := map[string]string{"mount": d.Mount}
		points = append(points,
			point("os.disk.total_bytes", float64(d.TotalBytes), labels),
			point("os.disk.used_bytes", float64(d.UsedBytes), labels),
			point("os.disk.free_bytes", float64(d.FreeBytes), labels),
			point("os.disk.inodes_total", float64(d.InodesTotal), labels),
			point("os.disk.inodes_used", float64(d.InodesUsed), labels),
		)
	}

	// Disk stats
	for _, ds := range snap.DiskStats {
		labels := map[string]string{"device": ds.Device}
		points = append(points,
			point("os.disk.reads_completed", float64(ds.ReadsCompleted), labels),
			point("os.disk.writes_completed", float64(ds.WritesCompleted), labels),
			point("os.disk.read_bytes_per_sec", float64(ds.ReadKB)*1024, labels),
			point("os.disk.write_bytes_per_sec", float64(ds.WriteKB)*1024, labels),
			point("os.disk.read_await_ms", ds.ReadAwaitMs, labels),
			point("os.disk.write_await_ms", ds.WriteAwaitMs, labels),
			point("os.disk.util_pct", ds.UtilPct, labels),
		)
	}

	return points
}

