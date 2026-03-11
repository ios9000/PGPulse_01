package agent

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// OSSnapshot contains a point-in-time snapshot of OS-level metrics.
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

// OSRelease holds parsed /etc/os-release fields.
type OSRelease struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	ID      string `json:"id"`
}

// LoadAvg holds the 1/5/15-minute load averages from /proc/loadavg.
type LoadAvg struct {
	One     float64 `json:"1m"`
	Five    float64 `json:"5m"`
	Fifteen float64 `json:"15m"`
}

// MemoryInfo holds parsed /proc/meminfo values.
type MemoryInfo struct {
	TotalKB       int64 `json:"total_kb"`
	AvailableKB   int64 `json:"available_kb"`
	UsedKB        int64 `json:"used_kb"`
	CommitLimitKB int64 `json:"commit_limit_kb"`
	CommittedAsKB int64 `json:"committed_as_kb"`
}

// CPUInfo holds aggregate CPU usage percentages computed from /proc/stat deltas.
type CPUInfo struct {
	UserPct   float64 `json:"user_pct"`
	SystemPct float64 `json:"system_pct"`
	IOWaitPct float64 `json:"iowait_pct"`
	IdlePct   float64 `json:"idle_pct"`
	NumCPUs   int     `json:"num_cpus"`
}

// DiskInfo holds filesystem size and inode info for a mount point.
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

// DiskStatInfo holds I/O statistics for a block device from /proc/diskstats.
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

// ClusterSnapshot is the payload for the /metrics/cluster endpoint.
type ClusterSnapshot struct {
	Patroni interface{} `json:"patroni"`
	ETCD    interface{} `json:"etcd"`
}

// CPURaw holds raw CPU jiffies parsed from a single /proc/stat "cpu " line.
type CPURaw struct {
	User    int64
	Nice    int64
	System  int64
	Idle    int64
	IOWait  int64
	IRQ     int64
	SoftIRQ int64
	Steal   int64
}

// Total returns the sum of all CPU jiffies.
func (c CPURaw) Total() int64 {
	return c.User + c.Nice + c.System + c.Idle + c.IOWait + c.IRQ + c.SoftIRQ + c.Steal
}


// DiskStatRaw holds raw fields from a /proc/diskstats line.
type DiskStatRaw struct {
	Device          string
	ReadsCompleted  int64
	ReadsMerged     int64
	SectorsRead     int64
	ReadTimeMs      int64
	WritesCompleted int64
	WritesMerged    int64
	SectorsWritten  int64
	WriteTimeMs     int64
	IOInProgress    int64
	IOTimeMs        int64
	WeightedIOTime  int64
}

// ParseMeminfo parses the content of /proc/meminfo and returns a MemoryInfo.
func ParseMeminfo(content string) MemoryInfo {
	m := make(map[string]int64)
	for _, line := range strings.Split(content, "\n") {
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		key := strings.TrimSuffix(parts[0], ":")
		val, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			continue
		}
		m[key] = val
	}

	total := m["MemTotal"]
	available := m["MemAvailable"]
	if available == 0 {
		// Fallback for older kernels without MemAvailable.
		available = m["MemFree"] + m["Buffers"] + m["Cached"]
	}

	return MemoryInfo{
		TotalKB:       total,
		AvailableKB:   available,
		UsedKB:        total - available,
		CommitLimitKB: m["CommitLimit"],
		CommittedAsKB: m["Committed_AS"],
	}
}

// ParseOSRelease parses the content of /etc/os-release (KEY=VALUE format).
func ParseOSRelease(content string) OSRelease {
	fields := make(map[string]string)
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.IndexByte(line, '=')
		if idx < 0 {
			continue
		}
		key := line[:idx]
		val := line[idx+1:]
		// Strip surrounding quotes.
		val = strings.Trim(val, `"'`)
		fields[key] = val
	}

	return OSRelease{
		Name:    fields["NAME"],
		Version: fields["VERSION_ID"],
		ID:      fields["ID"],
	}
}

// CPUDelta computes CPU usage percentages from two CPURaw snapshots.
func CPUDelta(prev, curr CPURaw, numCPUs int) CPUInfo {
	totalDelta := curr.Total() - prev.Total()
	if totalDelta <= 0 {
		return CPUInfo{NumCPUs: numCPUs}
	}

	pct := func(delta int64) float64 {
		return float64(delta) / float64(totalDelta) * 100.0
	}

	return CPUInfo{
		UserPct:   pct((curr.User + curr.Nice) - (prev.User + prev.Nice)),
		SystemPct: pct((curr.System + curr.IRQ + curr.SoftIRQ + curr.Steal) - (prev.System + prev.IRQ + prev.SoftIRQ + prev.Steal)),
		IOWaitPct: pct(curr.IOWait - prev.IOWait),
		IdlePct:   pct(curr.Idle - prev.Idle),
		NumCPUs:   numCPUs,
	}
}

// DiskStatsDelta computes I/O statistics deltas between two DiskStatRaw snapshots.
// intervalMs is the time elapsed between snapshots in milliseconds.
func DiskStatsDelta(prev, curr DiskStatRaw, intervalMs float64) DiskStatInfo {
	readsDelta := curr.ReadsCompleted - prev.ReadsCompleted
	writesDelta := curr.WritesCompleted - prev.WritesCompleted
	sectorsReadDelta := curr.SectorsRead - prev.SectorsRead
	sectorsWrittenDelta := curr.SectorsWritten - prev.SectorsWritten
	readTimeDelta := curr.ReadTimeMs - prev.ReadTimeMs
	writeTimeDelta := curr.WriteTimeMs - prev.WriteTimeMs
	ioTimeDelta := curr.IOTimeMs - prev.IOTimeMs

	var readAwait, writeAwait float64
	if readsDelta > 0 {
		readAwait = float64(readTimeDelta) / float64(readsDelta)
	}
	if writesDelta > 0 {
		writeAwait = float64(writeTimeDelta) / float64(writesDelta)
	}

	var utilPct float64
	if intervalMs > 0 {
		utilPct = float64(ioTimeDelta) / intervalMs * 100.0
		if utilPct > 100.0 {
			utilPct = 100.0
		}
	}

	return DiskStatInfo{
		Device:          curr.Device,
		ReadsCompleted:  readsDelta,
		WritesCompleted: writesDelta,
		ReadKB:          sectorsReadDelta * 512 / 1024,
		WriteKB:         sectorsWrittenDelta * 512 / 1024,
		IOInProgress:    curr.IOInProgress,
		ReadAwaitMs:     readAwait,
		WriteAwaitMs:    writeAwait,
		UtilPct:         utilPct,
	}
}

// ParseCPURaw parses the aggregate "cpu " line from /proc/stat content.
func ParseCPURaw(content string) (CPURaw, error) {
	for _, line := range strings.Split(content, "\n") {
		if !strings.HasPrefix(line, "cpu ") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 8 {
			return CPURaw{}, fmt.Errorf("cpu line has %d fields, expected at least 8", len(fields))
		}
		parseInt := func(s string) int64 {
			v, _ := strconv.ParseInt(s, 10, 64)
			return v
		}
		raw := CPURaw{
			User:    parseInt(fields[1]),
			Nice:    parseInt(fields[2]),
			System:  parseInt(fields[3]),
			Idle:    parseInt(fields[4]),
			IOWait:  parseInt(fields[5]),
			IRQ:     parseInt(fields[6]),
			SoftIRQ: parseInt(fields[7]),
		}
		if len(fields) > 8 {
			raw.Steal = parseInt(fields[8])
		}
		return raw, nil
	}
	return CPURaw{}, fmt.Errorf("no aggregate cpu line found in /proc/stat")
}

// ParseDiskStats parses /proc/diskstats content into a map keyed by device name.
func ParseDiskStats(content string) map[string]DiskStatRaw {
	result := make(map[string]DiskStatRaw)
	for _, line := range strings.Split(content, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 14 {
			continue
		}
		parseInt := func(s string) int64 {
			v, _ := strconv.ParseInt(s, 10, 64)
			return v
		}
		name := fields[2]
		raw := DiskStatRaw{
			Device:          name,
			ReadsCompleted:  parseInt(fields[3]),
			ReadsMerged:     parseInt(fields[4]),
			SectorsRead:     parseInt(fields[5]),
			ReadTimeMs:      parseInt(fields[6]),
			WritesCompleted: parseInt(fields[7]),
			WritesMerged:    parseInt(fields[8]),
			SectorsWritten:  parseInt(fields[9]),
			WriteTimeMs:     parseInt(fields[10]),
			IOInProgress:    parseInt(fields[11]),
			IOTimeMs:        parseInt(fields[12]),
			WeightedIOTime:  parseInt(fields[13]),
		}
		// Skip devices with zero total I/O.
		if raw.ReadsCompleted+raw.WritesCompleted == 0 {
			continue
		}
		result[name] = raw
	}
	return result
}
