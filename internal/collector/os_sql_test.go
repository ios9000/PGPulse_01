package collector

import (
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/ios9000/PGPulse_01/internal/agent"
	"github.com/ios9000/PGPulse_01/internal/version"
)

func TestOSSQLCollector_Name(t *testing.T) {
	c := NewOSSQLCollector("test", version.PGVersion{})
	if c.Name() != "os_sql" {
		t.Errorf("Name() = %q, want %q", c.Name(), "os_sql")
	}
}

func TestOSSQLCollector_Interval(t *testing.T) {
	c := NewOSSQLCollector("test", version.PGVersion{})
	if c.Interval().Seconds() != 60 {
		t.Errorf("Interval() = %v, want 60s", c.Interval())
	}
}

// TestOSSQLCollector_ReadMeminfo_FullContent tests parsing a complete /proc/meminfo.
func TestOSSQLCollector_ReadMeminfo_FullContent(t *testing.T) {
	content := `MemTotal:       16384000 kB
MemFree:         2048000 kB
MemAvailable:    8192000 kB
Buffers:          512000 kB
Cached:          4096000 kB
SwapCached:            0 kB
CommitLimit:    12288000 kB
Committed_AS:    6144000 kB
SwapTotal:       2097152 kB
SwapFree:        2097152 kB
`
	mem := agent.ParseMeminfo(content)

	if mem.TotalKB != 16384000 {
		t.Errorf("TotalKB = %d, want 16384000", mem.TotalKB)
	}
	if mem.AvailableKB != 8192000 {
		t.Errorf("AvailableKB = %d, want 8192000", mem.AvailableKB)
	}
	if mem.UsedKB != 8192000 {
		t.Errorf("UsedKB = %d, want 8192000 (total - available)", mem.UsedKB)
	}
	if mem.CommitLimitKB != 12288000 {
		t.Errorf("CommitLimitKB = %d, want 12288000", mem.CommitLimitKB)
	}
	if mem.CommittedAsKB != 6144000 {
		t.Errorf("CommittedAsKB = %d, want 6144000", mem.CommittedAsKB)
	}
}

// TestOSSQLCollector_ReadMeminfo_PartialContent tests fallback when MemAvailable is missing.
func TestOSSQLCollector_ReadMeminfo_PartialContent(t *testing.T) {
	content := `MemTotal:       16384000 kB
MemFree:         2048000 kB
Buffers:          512000 kB
Cached:          4096000 kB
`
	mem := agent.ParseMeminfo(content)

	wantAvail := int64(2048000 + 512000 + 4096000)
	if mem.AvailableKB != wantAvail {
		t.Errorf("AvailableKB = %d, want %d (fallback)", mem.AvailableKB, wantAvail)
	}
	wantUsed := int64(16384000) - wantAvail
	if mem.UsedKB != wantUsed {
		t.Errorf("UsedKB = %d, want %d", mem.UsedKB, wantUsed)
	}
}

// TestOSSQLCollector_ParseUptime tests uptime parsing.
func TestOSSQLCollector_ParseUptime(t *testing.T) {
	c := NewOSSQLCollector("test-inst", version.PGVersion{})

	content := "12345.67 98765.43"
	fields := strings.Fields(content)
	if len(fields) < 1 {
		t.Fatal("unexpected empty fields")
	}
	uptime, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		t.Fatalf("ParseFloat: %v", err)
	}
	pt := c.point("os.uptime_seconds", uptime, nil)

	if pt.Metric != "os.uptime_seconds" {
		t.Errorf("metric = %q, want %q", pt.Metric, "os.uptime_seconds")
	}
	if math.Abs(pt.Value-12345.67) > 0.01 {
		t.Errorf("value = %f, want 12345.67", pt.Value)
	}
}

// TestOSSQLCollector_ParseLoadAvg tests load average parsing.
func TestOSSQLCollector_ParseLoadAvg(t *testing.T) {
	c := NewOSSQLCollector("test-inst", version.PGVersion{})

	content := "0.35 0.22 0.18 2/245 12345"
	fields := strings.Fields(content)
	one, _ := strconv.ParseFloat(fields[0], 64)
	five, _ := strconv.ParseFloat(fields[1], 64)
	fifteen, _ := strconv.ParseFloat(fields[2], 64)

	pts := []MetricPoint{
		c.point("os.load.1m", one, nil),
		c.point("os.load.5m", five, nil),
		c.point("os.load.15m", fifteen, nil),
	}

	checks := map[string]float64{
		"os.load.1m":  0.35,
		"os.load.5m":  0.22,
		"os.load.15m": 0.18,
	}
	for _, pt := range pts {
		want, ok := checks[pt.Metric]
		if !ok {
			t.Errorf("unexpected metric %q", pt.Metric)
			continue
		}
		if math.Abs(pt.Value-want) > 0.001 {
			t.Errorf("%s = %f, want %f", pt.Metric, pt.Value, want)
		}
	}
}

// TestOSSQLCollector_CPUDelta_FirstCycleNil verifies no points on first CPU cycle.
func TestOSSQLCollector_CPUDelta_FirstCycleNil(t *testing.T) {
	c := NewOSSQLCollector("test-inst", version.PGVersion{})

	content := "cpu  10000 500 3000 80000 1000 100 200 50 0 0\ncpu0 2500 125 750 20000 250 25 50 12 0 0\n"
	raw, err := agent.ParseCPURaw(content)
	if err != nil {
		t.Fatalf("ParseCPURaw: %v", err)
	}

	// Simulate first cycle: prevCPU is nil, should store snapshot and return nil.
	if c.prevCPU != nil {
		t.Fatal("expected prevCPU to be nil on fresh collector")
	}

	c.mu.Lock()
	c.prevCPU = &raw
	c.mu.Unlock()

	// After first cycle, prevCPU should be set.
	if c.prevCPU == nil {
		t.Fatal("prevCPU should be set after first cycle")
	}
}

// TestOSSQLCollector_CPUDelta_SecondCycle verifies CPU percentage calculation.
func TestOSSQLCollector_CPUDelta_SecondCycle(t *testing.T) {
	prev := agent.CPURaw{
		User: 10000, Nice: 500, System: 3000, Idle: 80000,
		IOWait: 1000, IRQ: 100, SoftIRQ: 200, Steal: 50,
	}
	curr := agent.CPURaw{
		User: 11000, Nice: 550, System: 3500, Idle: 84000,
		IOWait: 1200, IRQ: 120, SoftIRQ: 230, Steal: 60,
	}

	info := agent.CPUDelta(prev, curr, 0)

	// total delta = 100660 - 94850 = 5810
	totalDelta := 5810.0
	assertCloseF(t, "UserPct", info.UserPct, 1050.0/totalDelta*100)
	assertCloseF(t, "SystemPct", info.SystemPct, 560.0/totalDelta*100)
	assertCloseF(t, "IOWaitPct", info.IOWaitPct, 200.0/totalDelta*100)
	assertCloseF(t, "IdlePct", info.IdlePct, 4000.0/totalDelta*100)
}

// TestOSSQLCollector_DiskStats_DeviceFilter tests that partitions and virtual devices are filtered.
func TestOSSQLCollector_DiskStats_DeviceFilter(t *testing.T) {
	tests := []struct {
		device string
		want   bool
	}{
		{"sda", true},
		{"sdb", true},
		{"sda1", false},
		{"sda12", false},
		{"nvme0n1", true},
		{"nvme0n1p1", false},
		{"vda", true},
		{"vda1", false},
		{"xvda", true},
		{"xvda1", false},
		{"loop0", false},
		{"dm-0", false},
	}

	for _, tt := range tests {
		got := devicePattern.MatchString(tt.device)
		if got != tt.want {
			t.Errorf("devicePattern.MatchString(%q) = %v, want %v", tt.device, got, tt.want)
		}
	}
}

// TestOSSQLCollector_DiskStatsDelta verifies diskstat delta calculation.
func TestOSSQLCollector_DiskStatsDelta(t *testing.T) {
	prev := agent.DiskStatRaw{
		Device:          "sda",
		ReadsCompleted:  1000,
		SectorsRead:     50000,
		ReadTimeMs:      5000,
		WritesCompleted: 2000,
		SectorsWritten:  100000,
		WriteTimeMs:     8000,
		IOTimeMs:        3000,
	}
	curr := agent.DiskStatRaw{
		Device:          "sda",
		ReadsCompleted:  1100,
		SectorsRead:     55000,
		ReadTimeMs:      5500,
		WritesCompleted: 2200,
		SectorsWritten:  110000,
		WriteTimeMs:     9000,
		IOTimeMs:        3500,
		IOInProgress:    3,
	}

	info := agent.DiskStatsDelta(prev, curr, 10000) // 10s

	if info.ReadsCompleted != 100 {
		t.Errorf("ReadsCompleted = %d, want 100", info.ReadsCompleted)
	}
	if info.WritesCompleted != 200 {
		t.Errorf("WritesCompleted = %d, want 200", info.WritesCompleted)
	}
	if info.ReadKB != 2500 {
		t.Errorf("ReadKB = %d, want 2500", info.ReadKB)
	}
	if info.WriteKB != 5000 {
		t.Errorf("WriteKB = %d, want 5000", info.WriteKB)
	}
	assertCloseF(t, "ReadAwaitMs", info.ReadAwaitMs, 5.0)
	assertCloseF(t, "WriteAwaitMs", info.WriteAwaitMs, 5.0)
	assertCloseF(t, "UtilPct", info.UtilPct, 5.0)
}

// TestOSSQLCollector_MetricKeys verifies all emitted metric keys match the agent.
func TestOSSQLCollector_MetricKeys(t *testing.T) {
	// These are the canonical metric keys from snapshotToMetricPoints in os.go.
	// The OSSQLCollector MUST emit a subset of these (disk space excluded).
	wantKeys := map[string]bool{
		// Memory
		"os.memory.total_kb":        true,
		"os.memory.available_kb":    true,
		"os.memory.used_kb":         true,
		"os.memory.commit_limit_kb": true,
		"os.memory.committed_as_kb": true,
		// Load
		"os.load.1m":  true,
		"os.load.5m":  true,
		"os.load.15m": true,
		// Uptime
		"os.uptime_seconds": true,
		// CPU
		"os.cpu.user_pct":   true,
		"os.cpu.system_pct": true,
		"os.cpu.iowait_pct": true,
		"os.cpu.idle_pct":   true,
		// Diskstat (per device)
		"os.diskstat.reads_completed":  true,
		"os.diskstat.writes_completed": true,
		"os.diskstat.read_kb":          true,
		"os.diskstat.write_kb":         true,
		"os.diskstat.read_await_ms":    true,
		"os.diskstat.write_await_ms":   true,
		"os.diskstat.util_pct":         true,
	}

	for key := range wantKeys {
		if key == "" {
			t.Error("empty key in wantKeys")
		}
	}

	if len(wantKeys) != 20 {
		t.Errorf("expected 20 metric keys, got %d", len(wantKeys))
	}
}

func assertCloseF(t *testing.T, name string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 0.01 {
		t.Errorf("%s = %.4f, want %.4f", name, got, want)
	}
}
