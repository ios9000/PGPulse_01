package collector

import (
	"testing"
	"time"

	"github.com/ios9000/PGPulse_01/internal/version"
)

func TestLockTreeCollector_Name(t *testing.T) {
	c := NewLockTreeCollector("test", version.PGVersion{Major: 16})
	if c.Name() != "lock_tree" {
		t.Errorf("Name() = %q, want %q", c.Name(), "lock_tree")
	}
}

func TestLockTreeCollector_Interval(t *testing.T) {
	c := NewLockTreeCollector("test", version.PGVersion{Major: 16})
	if c.Interval() != 10*time.Second {
		t.Errorf("Interval() = %v, want 10s", c.Interval())
	}
}

// TestLockTreeCollector_Collect verifies that a pair of blocking edges produces
// exactly 3 metric points with the correct values.
func TestLockTreeCollector_Collect(t *testing.T) {
	c := NewLockTreeCollector("test", version.PGVersion{Major: 16})

	// 100 blocks 200, 100 also blocks 300 → root=1, blocked=2, depth=1.
	edges := []lockEdge{
		{BlockedPID: 200, BlockerPID: 100},
		{BlockedPID: 300, BlockerPID: 100},
	}
	stats := computeLockStats(edges)
	points := c.statsToPoints(stats)

	if len(points) != 3 {
		t.Fatalf("expected 3 metric points, got %d", len(points))
	}
	assertLockVal(t, points, "pgpulse.locks.blocker_count", 1)
	assertLockVal(t, points, "pgpulse.locks.blocked_count", 2)
	assertLockVal(t, points, "pgpulse.locks.max_chain_depth", 1)
}

// TestLockTreeCollector_Collect_NoBlocking verifies that zero edges produce
// 3 metric points all equal to 0.
func TestLockTreeCollector_Collect_NoBlocking(t *testing.T) {
	c := NewLockTreeCollector("test", version.PGVersion{Major: 16})

	points := c.statsToPoints(computeLockStats(nil))

	if len(points) != 3 {
		t.Fatalf("expected 3 metric points, got %d", len(points))
	}
	assertLockVal(t, points, "pgpulse.locks.blocker_count", 0)
	assertLockVal(t, points, "pgpulse.locks.blocked_count", 0)
	assertLockVal(t, points, "pgpulse.locks.max_chain_depth", 0)
}

// --- computeLockStats pure function tests ---

func TestComputeLockStats_NoEdges(t *testing.T) {
	s := computeLockStats(nil)
	if s.BlockerCount != 0 || s.BlockedCount != 0 || s.MaxChainDepth != 0 {
		t.Errorf("no edges: want {0,0,0}, got {%d,%d,%d}",
			s.BlockerCount, s.BlockedCount, s.MaxChainDepth)
	}
}

// TestComputeLockStats_SingleBlocker: A blocks B → blockers=1, blocked=1, depth=1.
func TestComputeLockStats_SingleBlocker(t *testing.T) {
	edges := []lockEdge{{BlockedPID: 2, BlockerPID: 1}}
	s := computeLockStats(edges)
	assertLockStats(t, "SingleBlocker", s, lockStats{1, 1, 1})
}

// TestComputeLockStats_Chain: A blocks B, B blocks C → blockers=1, blocked=2, depth=2.
func TestComputeLockStats_Chain(t *testing.T) {
	edges := []lockEdge{
		{BlockedPID: 2, BlockerPID: 1}, // A blocks B
		{BlockedPID: 3, BlockerPID: 2}, // B blocks C
	}
	s := computeLockStats(edges)
	assertLockStats(t, "Chain", s, lockStats{1, 2, 2})
}

// TestComputeLockStats_Wide: A blocks B, A blocks C → blockers=1, blocked=2, depth=1.
func TestComputeLockStats_Wide(t *testing.T) {
	edges := []lockEdge{
		{BlockedPID: 2, BlockerPID: 1}, // A blocks B
		{BlockedPID: 3, BlockerPID: 1}, // A blocks C
	}
	s := computeLockStats(edges)
	assertLockStats(t, "Wide", s, lockStats{1, 2, 1})
}

// TestComputeLockStats_MultiRoot: A blocks B, C blocks D → blockers=2, blocked=2, depth=1.
func TestComputeLockStats_MultiRoot(t *testing.T) {
	edges := []lockEdge{
		{BlockedPID: 2, BlockerPID: 1}, // A blocks B
		{BlockedPID: 4, BlockerPID: 3}, // C blocks D
	}
	s := computeLockStats(edges)
	assertLockStats(t, "MultiRoot", s, lockStats{2, 2, 1})
}

// TestComputeLockStats_Diamond: A blocks B, A blocks C, B blocks D, C blocks D
// → blockers=1, blocked=3, depth=2.
func TestComputeLockStats_Diamond(t *testing.T) {
	edges := []lockEdge{
		{BlockedPID: 2, BlockerPID: 1}, // A blocks B
		{BlockedPID: 3, BlockerPID: 1}, // A blocks C
		{BlockedPID: 4, BlockerPID: 2}, // B blocks D
		{BlockedPID: 4, BlockerPID: 3}, // C blocks D
	}
	s := computeLockStats(edges)
	assertLockStats(t, "Diamond", s, lockStats{1, 3, 2})
}

// TestComputeLockStats_Cycle: A blocks B, B blocks A (deadlock).
// Both are in blockerSet AND blockedSet → no roots → blockers=0, blocked=2, depth=0.
func TestComputeLockStats_Cycle(t *testing.T) {
	edges := []lockEdge{
		{BlockedPID: 2, BlockerPID: 1}, // A blocks B
		{BlockedPID: 1, BlockerPID: 2}, // B blocks A
	}
	s := computeLockStats(edges)
	assertLockStats(t, "Cycle", s, lockStats{0, 2, 0})
}

// --- helpers local to this file ---

// assertLockStats compares a lockStats against expected values.
func assertLockStats(t *testing.T, name string, got, want lockStats) {
	t.Helper()
	if got.BlockerCount != want.BlockerCount {
		t.Errorf("%s: BlockerCount = %d, want %d", name, got.BlockerCount, want.BlockerCount)
	}
	if got.BlockedCount != want.BlockedCount {
		t.Errorf("%s: BlockedCount = %d, want %d", name, got.BlockedCount, want.BlockedCount)
	}
	if got.MaxChainDepth != want.MaxChainDepth {
		t.Errorf("%s: MaxChainDepth = %d, want %d", name, got.MaxChainDepth, want.MaxChainDepth)
	}
}

// assertLockVal finds a metric by name and asserts its float value.
func assertLockVal(t *testing.T, points []MetricPoint, name string, want float64) {
	t.Helper()
	m, ok := findCheckpointMetric(points, name)
	if !ok {
		t.Errorf("metric %q not found", name)
		return
	}
	if m.Value != want {
		t.Errorf("metric %q = %v, want %v", name, m.Value, want)
	}
}
