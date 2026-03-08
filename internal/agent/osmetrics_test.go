package agent

import (
	"math"
	"testing"
)

func TestParseMeminfo(t *testing.T) {
	content := `MemTotal:       16384000 kB
MemFree:         2048000 kB
MemAvailable:    8192000 kB
Buffers:          512000 kB
Cached:          4096000 kB
SwapCached:            0 kB
CommitLimit:    12288000 kB
Committed_AS:    6144000 kB
`
	mem := ParseMeminfo(content)

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

func TestParseMeminfo_FallbackNoMemAvailable(t *testing.T) {
	// Older kernels might not have MemAvailable.
	content := `MemTotal:       16384000 kB
MemFree:         2048000 kB
Buffers:          512000 kB
Cached:          4096000 kB
`
	mem := ParseMeminfo(content)

	expectedAvailable := int64(2048000 + 512000 + 4096000)
	if mem.AvailableKB != expectedAvailable {
		t.Errorf("AvailableKB = %d, want %d (Free+Buffers+Cached fallback)", mem.AvailableKB, expectedAvailable)
	}
}

func TestParseOSRelease_Ubuntu(t *testing.T) {
	content := `NAME="Ubuntu"
VERSION_ID="22.04"
ID=ubuntu
ID_LIKE=debian
PRETTY_NAME="Ubuntu 22.04.3 LTS"
`
	rel := ParseOSRelease(content)

	if rel.Name != "Ubuntu" {
		t.Errorf("Name = %q, want %q", rel.Name, "Ubuntu")
	}
	if rel.Version != "22.04" {
		t.Errorf("Version = %q, want %q", rel.Version, "22.04")
	}
	if rel.ID != "ubuntu" {
		t.Errorf("ID = %q, want %q", rel.ID, "ubuntu")
	}
}

func TestParseOSRelease_RHEL(t *testing.T) {
	content := `NAME="Red Hat Enterprise Linux"
VERSION_ID="9.3"
ID="rhel"
ID_LIKE="fedora"
PLATFORM_ID="platform:el9"
`
	rel := ParseOSRelease(content)

	if rel.Name != "Red Hat Enterprise Linux" {
		t.Errorf("Name = %q, want %q", rel.Name, "Red Hat Enterprise Linux")
	}
	if rel.Version != "9.3" {
		t.Errorf("Version = %q, want %q", rel.Version, "9.3")
	}
	if rel.ID != "rhel" {
		t.Errorf("ID = %q, want %q", rel.ID, "rhel")
	}
}

func TestParseOSRelease_Comments(t *testing.T) {
	content := `# This is a comment
NAME="Test OS"
# Another comment
VERSION_ID="1.0"
ID=testos
`
	rel := ParseOSRelease(content)

	if rel.Name != "Test OS" {
		t.Errorf("Name = %q, want %q", rel.Name, "Test OS")
	}
	if rel.ID != "testos" {
		t.Errorf("ID = %q, want %q", rel.ID, "testos")
	}
}

func TestCPUDelta(t *testing.T) {
	prev := cpuRaw{
		User:    10000,
		Nice:    500,
		System:  3000,
		Idle:    80000,
		IOWait:  1000,
		IRQ:     100,
		SoftIRQ: 200,
		Steal:   50,
	}
	curr := cpuRaw{
		User:    11000,
		Nice:    550,
		System:  3500,
		Idle:    84000,
		IOWait:  1200,
		IRQ:     120,
		SoftIRQ: 230,
		Steal:   60,
	}

	info := CPUDelta(prev, curr, 8)

	// total delta = (11000+550+3500+84000+1200+120+230+60) - (10000+500+3000+80000+1000+100+200+50)
	//             = 100660 - 94850 = 5810
	// user+nice delta = (11000+550) - (10000+500) = 1050
	// system+irq+softirq+steal delta = (3500+120+230+60) - (3000+100+200+50) = 560
	// iowait delta = 1200-1000 = 200
	// idle delta = 84000-80000 = 4000

	totalDelta := 5810.0
	assertClose(t, "UserPct", info.UserPct, 1050.0/totalDelta*100)
	assertClose(t, "SystemPct", info.SystemPct, 560.0/totalDelta*100)
	assertClose(t, "IOWaitPct", info.IOWaitPct, 200.0/totalDelta*100)
	assertClose(t, "IdlePct", info.IdlePct, 4000.0/totalDelta*100)

	if info.NumCPUs != 8 {
		t.Errorf("NumCPUs = %d, want 8", info.NumCPUs)
	}
}

func TestCPUDelta_ZeroTotal(t *testing.T) {
	same := cpuRaw{User: 1000, Nice: 0, System: 500, Idle: 5000}
	info := CPUDelta(same, same, 4)

	if info.UserPct != 0 || info.SystemPct != 0 {
		t.Error("expected zero percentages when total delta is 0")
	}
	if info.NumCPUs != 4 {
		t.Errorf("NumCPUs = %d, want 4", info.NumCPUs)
	}
}

func TestDiskStatsDelta(t *testing.T) {
	prev := diskStatRaw{
		Device:          "sda",
		ReadsCompleted:  1000,
		SectorsRead:     50000,
		ReadTimeMs:      5000,
		WritesCompleted: 2000,
		SectorsWritten:  100000,
		WriteTimeMs:     8000,
		IOTimeMs:        3000,
	}
	curr := diskStatRaw{
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

	intervalMs := 10000.0 // 10 seconds
	info := DiskStatsDelta(prev, curr, intervalMs)

	if info.Device != "sda" {
		t.Errorf("Device = %q, want %q", info.Device, "sda")
	}
	if info.ReadsCompleted != 100 {
		t.Errorf("ReadsCompleted = %d, want 100", info.ReadsCompleted)
	}
	if info.WritesCompleted != 200 {
		t.Errorf("WritesCompleted = %d, want 200", info.WritesCompleted)
	}

	// read_kb = (55000-50000) * 512 / 1024 = 5000 * 512 / 1024 = 2500
	if info.ReadKB != 2500 {
		t.Errorf("ReadKB = %d, want 2500", info.ReadKB)
	}
	// write_kb = (110000-100000) * 512 / 1024 = 10000 * 512 / 1024 = 5000
	if info.WriteKB != 5000 {
		t.Errorf("WriteKB = %d, want 5000", info.WriteKB)
	}

	// read_await = 500 / 100 = 5.0
	assertClose(t, "ReadAwaitMs", info.ReadAwaitMs, 5.0)
	// write_await = 1000 / 200 = 5.0
	assertClose(t, "WriteAwaitMs", info.WriteAwaitMs, 5.0)

	// util_pct = 500 / 10000 * 100 = 5.0
	assertClose(t, "UtilPct", info.UtilPct, 5.0)

	if info.IOInProgress != 3 {
		t.Errorf("IOInProgress = %d, want 3", info.IOInProgress)
	}
}

func TestDiskStatsDelta_ZeroReads(t *testing.T) {
	prev := diskStatRaw{Device: "sdb", ReadsCompleted: 100, WritesCompleted: 50}
	curr := diskStatRaw{Device: "sdb", ReadsCompleted: 100, WritesCompleted: 60, WriteTimeMs: 100}

	info := DiskStatsDelta(prev, curr, 5000)

	if info.ReadAwaitMs != 0 {
		t.Errorf("ReadAwaitMs = %f, want 0 (no reads)", info.ReadAwaitMs)
	}
}

func TestParseCPURaw(t *testing.T) {
	content := `cpu  10000 500 3000 80000 1000 100 200 50 0 0
cpu0 2500 125 750 20000 250 25 50 12 0 0
`
	raw, err := parseCPURaw(content)
	if err != nil {
		t.Fatalf("parseCPURaw returned error: %v", err)
	}

	if raw.User != 10000 {
		t.Errorf("User = %d, want 10000", raw.User)
	}
	if raw.Idle != 80000 {
		t.Errorf("Idle = %d, want 80000", raw.Idle)
	}
	if raw.Steal != 50 {
		t.Errorf("Steal = %d, want 50", raw.Steal)
	}
}

func TestParseDiskStats(t *testing.T) {
	content := `   8       0 sda 1000 500 50000 5000 2000 300 100000 8000 3 3000 13000
   8       1 sda1 900 400 45000 4500 1800 280 90000 7200 2 2700 11700
   8      16 sdb 0 0 0 0 0 0 0 0 0 0 0
`
	stats := parseDiskStats(content)

	if _, ok := stats["sda"]; !ok {
		t.Fatal("expected sda in parsed disk stats")
	}
	if _, ok := stats["sda1"]; !ok {
		t.Fatal("expected sda1 in parsed disk stats")
	}
	// sdb should be filtered out (zero reads+writes).
	if _, ok := stats["sdb"]; ok {
		t.Error("sdb should be filtered out (zero I/O)")
	}

	sda := stats["sda"]
	if sda.ReadsCompleted != 1000 {
		t.Errorf("sda ReadsCompleted = %d, want 1000", sda.ReadsCompleted)
	}
	if sda.SectorsRead != 50000 {
		t.Errorf("sda SectorsRead = %d, want 50000", sda.SectorsRead)
	}
}

func assertClose(t *testing.T, name string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 0.01 {
		t.Errorf("%s = %.4f, want %.4f", name, got, want)
	}
}
