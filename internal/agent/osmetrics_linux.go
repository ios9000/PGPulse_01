//go:build linux

package agent

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// previousDiskStats stores the last disk stats snapshot for delta computation.
var previousDiskStats map[string]DiskStatRaw

// previousDiskStatsTime stores the timestamp of the last disk stats snapshot.
var previousDiskStatsTime time.Time

// excludedFSTypes are virtual filesystem types that should not be reported.
var excludedFSTypes = map[string]bool{
	"tmpfs":     true,
	"devtmpfs":  true,
	"sysfs":     true,
	"proc":      true,
	"cgroup":    true,
	"cgroup2":   true,
	"devpts":    true,
	"hugetlbfs": true,
	"mqueue":    true,
	"pstore":    true,
	"debugfs":   true,
	"securityfs": true,
	"configfs":  true,
	"fusectl":   true,
	"binfmt_misc": true,
	"autofs":    true,
	"tracefs":   true,
	"overlay":   true,
}

// CollectOS gathers a full OS metrics snapshot from procfs/sysfs.
// mountPoints limits disk info to specific mount points; empty means all.
func CollectOS(mountPoints []string) (*OSSnapshot, error) {
	snap := &OSSnapshot{
		CollectedAt: time.Now().UTC(),
	}

	snap.Hostname = collectHostname()
	snap.OSRelease = collectOSRelease()

	uptime, err := collectUptime()
	if err == nil {
		snap.UptimeSecs = uptime
	}

	loadAvg, err := collectLoadAvg()
	if err == nil {
		snap.LoadAvg = loadAvg
	}

	mem, err := collectMemory()
	if err == nil {
		snap.Memory = mem
	}

	cpu, err := collectCPU()
	if err == nil {
		snap.CPU = cpu
	}

	disks, err := collectDisks(mountPoints)
	if err == nil {
		snap.Disks = disks
	}

	diskStats, err := collectDiskStats()
	if err == nil {
		snap.DiskStats = diskStats
	}

	return snap, nil
}

func collectHostname() string {
	h, _ := os.Hostname()
	return h
}

func collectOSRelease() OSRelease {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		data, err = os.ReadFile("/etc/lsb-release")
		if err != nil {
			return OSRelease{}
		}
	}
	return ParseOSRelease(string(data))
}

func collectUptime() (float64, error) {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0, fmt.Errorf("read /proc/uptime: %w", err)
	}
	fields := strings.Fields(string(data))
	if len(fields) < 1 {
		return 0, fmt.Errorf("unexpected /proc/uptime format")
	}
	return strconv.ParseFloat(fields[0], 64)
}

func collectLoadAvg() (LoadAvg, error) {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return LoadAvg{}, fmt.Errorf("read /proc/loadavg: %w", err)
	}
	fields := strings.Fields(string(data))
	if len(fields) < 3 {
		return LoadAvg{}, fmt.Errorf("unexpected /proc/loadavg format")
	}
	one, _ := strconv.ParseFloat(fields[0], 64)
	five, _ := strconv.ParseFloat(fields[1], 64)
	fifteen, _ := strconv.ParseFloat(fields[2], 64)
	return LoadAvg{One: one, Five: five, Fifteen: fifteen}, nil
}

func collectMemory() (MemoryInfo, error) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return MemoryInfo{}, fmt.Errorf("read /proc/meminfo: %w", err)
	}
	return ParseMeminfo(string(data)), nil
}

func collectCPU() (CPUInfo, error) {
	data1, err := os.ReadFile("/proc/stat")
	if err != nil {
		return CPUInfo{}, fmt.Errorf("read /proc/stat: %w", err)
	}
	prev, err := ParseCPURaw(string(data1))
	if err != nil {
		return CPUInfo{}, fmt.Errorf("parse /proc/stat: %w", err)
	}

	time.Sleep(1 * time.Second)

	data2, err := os.ReadFile("/proc/stat")
	if err != nil {
		return CPUInfo{}, fmt.Errorf("read /proc/stat (second sample): %w", err)
	}
	curr, err := ParseCPURaw(string(data2))
	if err != nil {
		return CPUInfo{}, fmt.Errorf("parse /proc/stat (second sample): %w", err)
	}

	return CPUDelta(prev, curr, runtime.NumCPU()), nil
}

func collectDisks(mountPoints []string) ([]DiskInfo, error) {
	data, err := os.ReadFile("/proc/mounts")
	if err != nil {
		return nil, fmt.Errorf("read /proc/mounts: %w", err)
	}

	wantMount := make(map[string]bool)
	for _, mp := range mountPoints {
		wantMount[mp] = true
	}

	var disks []DiskInfo
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		device := fields[0]
		mount := fields[1]
		fstype := fields[2]

		if excludedFSTypes[fstype] {
			continue
		}

		if len(wantMount) > 0 && !wantMount[mount] {
			continue
		}

		var stat syscall.Statfs_t
		if err := syscall.Statfs(mount, &stat); err != nil {
			continue
		}

		totalBytes := int64(stat.Blocks) * int64(stat.Bsize)
		freeBytes := int64(stat.Bavail) * int64(stat.Bsize)
		usedBytes := totalBytes - int64(stat.Bfree)*int64(stat.Bsize)

		disks = append(disks, DiskInfo{
			Mount:       mount,
			Device:      device,
			FSType:      fstype,
			TotalBytes:  totalBytes,
			UsedBytes:   usedBytes,
			FreeBytes:   freeBytes,
			InodesTotal: int64(stat.Files),
			InodesUsed:  int64(stat.Files) - int64(stat.Ffree),
		})
	}
	return disks, nil
}

func collectDiskStats() ([]DiskStatInfo, error) {
	data, err := os.ReadFile("/proc/diskstats")
	if err != nil {
		return nil, fmt.Errorf("read /proc/diskstats: %w", err)
	}

	current := ParseDiskStats(string(data))
	now := time.Now()

	if previousDiskStats == nil {
		previousDiskStats = current
		previousDiskStatsTime = now
		// First call: return raw counters with zero deltas.
		var stats []DiskStatInfo
		for _, raw := range current {
			stats = append(stats, DiskStatInfo{
				Device:          raw.Device,
				ReadsCompleted:  raw.ReadsCompleted,
				WritesCompleted: raw.WritesCompleted,
				ReadKB:          raw.SectorsRead * 512 / 1024,
				WriteKB:         raw.SectorsWritten * 512 / 1024,
				IOInProgress:    raw.IOInProgress,
			})
		}
		return stats, nil
	}

	intervalMs := float64(now.Sub(previousDiskStatsTime).Milliseconds())
	if intervalMs <= 0 {
		intervalMs = 1
	}

	var stats []DiskStatInfo
	for name, curr := range current {
		prev, ok := previousDiskStats[name]
		if !ok {
			continue
		}
		stats = append(stats, DiskStatsDelta(prev, curr, intervalMs))
	}

	previousDiskStats = current
	previousDiskStatsTime = now
	return stats, nil
}
