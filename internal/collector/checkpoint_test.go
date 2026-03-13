package collector

import (
	"strings"
	"testing"
	"time"

	"github.com/ios9000/PGPulse_01/internal/version"
)

// --- test helpers ---

func hasMetric(points []MetricPoint, name string) bool {
	for _, p := range points {
		if p.Metric == name {
			return true
		}
	}
	return false
}

func findCheckpointMetric(points []MetricPoint, name string) (MetricPoint, bool) {
	for _, p := range points {
		if p.Metric == name {
			return p, true
		}
	}
	return MetricPoint{}, false
}

func hasAnyMetricContaining(points []MetricPoint, substr string) bool {
	for _, p := range points {
		if strings.Contains(p.Metric, substr) {
			return true
		}
	}
	return false
}

// --- tests ---

func TestCheckpoint_NameAndInterval(t *testing.T) {
	c := NewCheckpointCollector("test", version.PGVersion{Major: 16})
	if c.Name() != "checkpoint" {
		t.Errorf("Name() = %q, want %q", c.Name(), "checkpoint")
	}
	if c.Interval() != 60*time.Second {
		t.Errorf("Interval() = %v, want 60s", c.Interval())
	}
}

func TestCheckpoint_GateSelectPG14(t *testing.T) {
	v := version.PGVersion{Major: 14, Minor: 0, Num: 140000}
	sql, ok := checkpointGate.Select(v)
	if !ok {
		t.Fatal("expected gate to select a variant for PG 14")
	}
	if !strings.Contains(sql, "FROM pg_stat_bgwriter") {
		t.Error("PG 14 variant must query pg_stat_bgwriter")
	}
	if strings.Contains(sql, "pg_stat_checkpointer") {
		t.Error("PG 14 variant must NOT reference pg_stat_checkpointer")
	}
}

func TestCheckpoint_GateSelectPG17(t *testing.T) {
	v := version.PGVersion{Major: 17, Minor: 0, Num: 170000}
	sql, ok := checkpointGate.Select(v)
	if !ok {
		t.Fatal("expected gate to select a variant for PG 17")
	}
	if !strings.Contains(sql, "pg_stat_checkpointer") {
		t.Error("PG 17 variant must query pg_stat_checkpointer")
	}
}

func TestCheckpoint_FirstCycleNoRates(t *testing.T) {
	c := NewCheckpointCollector("test", version.PGVersion{Major: 16})

	curr := checkpointSnapshot{
		checkpointsTimed:      100,
		checkpointsReq:  20,
		writeTimeMs:   5000,
		syncTimeMs:    1000,
		buffersWritten:     800,
		buffersClean:          200,
		maxwrittenClean:       10,
		buffersAlloc:          5000,
		buffersBackend:        300,
		buffersBackendFsync:   5,
		restartpointsTimed:    -1,
		restartpointsDone:     -1,
		restartpointsRequested: -1,
	}

	now := time.Now()
	// First cycle: prev is nil → no rates should be emitted.
	points := c.computeMetrics(curr, nil, time.Time{}, now)

	if len(points) == 0 {
		t.Fatal("expected absolute metrics on first cycle, got 0 points")
	}

	// Verify absolute metrics are present.
	if !hasMetric(points, "pg.checkpoint.timed") {
		t.Error("expected pgpulse.checkpoint.timed in first cycle")
	}
	if !hasMetric(points, "pg.bgwriter.buffers_clean") {
		t.Error("expected pgpulse.bgwriter.buffers_clean in first cycle")
	}

	// Verify NO rate metrics are present.
	if hasAnyMetricContaining(points, "_per_second") {
		t.Error("first cycle must NOT emit _per_second metrics")
	}
}

func TestCheckpoint_SecondCycleEmitsRates(t *testing.T) {
	c := NewCheckpointCollector("test", version.PGVersion{Major: 16})

	prev := checkpointSnapshot{
		checkpointsTimed:      10,
		checkpointsReq:  5,
		writeTimeMs:   1000,
		syncTimeMs:    200,
		buffersWritten:     100,
		buffersClean:          50,
		maxwrittenClean:       2,
		buffersAlloc:          1000,
		buffersBackend:        100,
		buffersBackendFsync:   1,
		restartpointsTimed:    -1,
		restartpointsDone:     -1,
		restartpointsRequested: -1,
	}

	curr := checkpointSnapshot{
		checkpointsTimed:      70,
		checkpointsReq:  15,
		writeTimeMs:   3000,
		syncTimeMs:    500,
		buffersWritten:     400,
		buffersClean:          200,
		maxwrittenClean:       5,
		buffersAlloc:          2000,
		buffersBackend:        400,
		buffersBackendFsync:   3,
		restartpointsTimed:    -1,
		restartpointsDone:     -1,
		restartpointsRequested: -1,
	}

	// Set prev on the collector so ratePoints can compute deltas.
	c.prev = &prev

	prevTime := time.Now()
	now := prevTime.Add(60 * time.Second)

	points := c.computeMetrics(curr, &prev, prevTime, now)

	// Verify absolute metrics are present.
	if !hasMetric(points, "pg.checkpoint.timed") {
		t.Error("expected pgpulse.checkpoint.timed")
	}
	if !hasMetric(points, "pg.bgwriter.buffers_alloc") {
		t.Error("expected pgpulse.bgwriter.buffers_alloc")
	}

	// Verify rate metrics are present.
	if !hasAnyMetricContaining(points, "_per_second") {
		t.Fatal("second cycle must emit _per_second metrics")
	}

	// Verify rate math: timed went from 10 to 70 over 60s → delta=60, rate=1.0/s.
	m, ok := findCheckpointMetric(points, "pg.checkpoint.timed_per_second")
	if !ok {
		t.Fatal("expected pgpulse.checkpoint.timed_per_second metric")
	}
	if m.Value != 1.0 {
		t.Errorf("timed_per_second = %v, want 1.0 (delta 60 / 60s)", m.Value)
	}

	// Verify rate math: buffers_clean went from 50 to 200 over 60s → delta=150, rate=2.5/s.
	m, ok = findCheckpointMetric(points, "pg.bgwriter.buffers_clean_per_second")
	if !ok {
		t.Fatal("expected pgpulse.bgwriter.buffers_clean_per_second metric")
	}
	if m.Value != 2.5 {
		t.Errorf("buffers_clean_per_second = %v, want 2.5 (delta 150 / 60s)", m.Value)
	}

	// Verify PG ≤ 16 emits buffers_backend rate.
	if !hasMetric(points, "pg.bgwriter.buffers_backend_per_second") {
		t.Error("PG 16 must emit bgwriter.buffers_backend_per_second")
	}
}

func TestCheckpoint_StatsResetSkipsRates(t *testing.T) {
	c := NewCheckpointCollector("test", version.PGVersion{Major: 16})

	// prev has higher values than curr → simulates pg_stat_reset().
	prev := checkpointSnapshot{
		checkpointsTimed:      100,
		checkpointsReq:  50,
		writeTimeMs:   5000,
		syncTimeMs:    1000,
		buffersWritten:     800,
		buffersClean:          200,
		maxwrittenClean:       10,
		buffersAlloc:          5000,
		buffersBackend:        300,
		buffersBackendFsync:   5,
		restartpointsTimed:    -1,
		restartpointsDone:     -1,
		restartpointsRequested: -1,
	}

	curr := checkpointSnapshot{
		checkpointsTimed:      5,
		checkpointsReq:  2,
		writeTimeMs:   100,
		syncTimeMs:    20,
		buffersWritten:     10,
		buffersClean:          3,
		maxwrittenClean:       0,
		buffersAlloc:          50,
		buffersBackend:        5,
		buffersBackendFsync:   0,
		restartpointsTimed:    -1,
		restartpointsDone:     -1,
		restartpointsRequested: -1,
	}

	c.prev = &prev

	if !c.isStatsReset(curr) {
		t.Error("isStatsReset() should return true when current counters < previous")
	}
}

func TestCheckpoint_PG16NoRestartpoints(t *testing.T) {
	c := NewCheckpointCollector("test", version.PGVersion{Major: 16})

	snap := checkpointSnapshot{
		checkpointsTimed:      100,
		checkpointsReq:  20,
		writeTimeMs:   5000,
		syncTimeMs:    1000,
		buffersWritten:     800,
		buffersClean:          200,
		maxwrittenClean:       10,
		buffersAlloc:          5000,
		buffersBackend:        300,
		buffersBackendFsync:   5,
		restartpointsTimed:    -1, // sentinel: not available on PG ≤ 16
		restartpointsDone:     -1,
		restartpointsRequested: -1,
	}

	points := c.absolutePoints(snap)

	if hasAnyMetricContaining(points, "restartpoints") {
		t.Error("PG 16 must NOT emit restartpoints metrics (sentinel = -1)")
	}
}

func TestCheckpoint_PG17HasRestartpoints(t *testing.T) {
	c := NewCheckpointCollector("test", version.PGVersion{Major: 17})

	snap := checkpointSnapshot{
		checkpointsTimed:      100,
		checkpointsReq:  20,
		writeTimeMs:   5000,
		syncTimeMs:    1000,
		buffersWritten:     800,
		buffersClean:          200,
		maxwrittenClean:       10,
		buffersAlloc:          5000,
		buffersBackend:        -1, // not available on PG 17
		buffersBackendFsync:   -1, // not available on PG 17
		restartpointsTimed:    5,
		restartpointsDone:     3,
		restartpointsRequested: 2,
	}

	points := c.absolutePoints(snap)

	// Restartpoints must be present.
	if !hasMetric(points, "pg.checkpoint.restartpoints_timed") {
		t.Error("PG 17 must emit checkpoint.restartpoints_timed")
	}
	if !hasMetric(points, "pg.checkpoint.restartpoints_done") {
		t.Error("PG 17 must emit checkpoint.restartpoints_done")
	}
	if !hasMetric(points, "pg.checkpoint.restartpoints_req") {
		t.Error("PG 17 must emit checkpoint.restartpoints_req")
	}

	// buffers_backend must NOT be present (sentinel = -1).
	if hasAnyMetricContaining(points, "buffers_backend") {
		t.Error("PG 17 must NOT emit buffers_backend metrics (sentinel = -1)")
	}
}

func TestCheckpoint_ZeroElapsedSafe(t *testing.T) {
	c := NewCheckpointCollector("test", version.PGVersion{Major: 16})

	prev := checkpointSnapshot{
		checkpointsTimed:      10,
		checkpointsReq:  5,
		writeTimeMs:   1000,
		syncTimeMs:    200,
		buffersWritten:     100,
		buffersClean:          50,
		maxwrittenClean:       2,
		buffersAlloc:          1000,
		buffersBackend:        100,
		buffersBackendFsync:   1,
		restartpointsTimed:    -1,
		restartpointsDone:     -1,
		restartpointsRequested: -1,
	}

	curr := checkpointSnapshot{
		checkpointsTimed:      20,
		checkpointsReq:  10,
		writeTimeMs:   2000,
		syncTimeMs:    400,
		buffersWritten:     200,
		buffersClean:          100,
		maxwrittenClean:       4,
		buffersAlloc:          2000,
		buffersBackend:        200,
		buffersBackendFsync:   2,
		restartpointsTimed:    -1,
		restartpointsDone:     -1,
		restartpointsRequested: -1,
	}

	c.prev = &prev

	now := time.Now()
	// Zero elapsed time: prevTime == now.
	points := c.computeMetrics(curr, &prev, now, now)

	// Must not panic.

	// Must not emit rate metrics when elapsed is zero.
	if hasAnyMetricContaining(points, "_per_second") {
		t.Error("zero elapsed time must NOT produce _per_second metrics")
	}
}

// TestCheckpoint_Integration is a stub for future Docker-based integration testing.
// It verifies that checkpoint and bgwriter metrics are emitted when querying a real PG instance.
func TestCheckpoint_Integration(t *testing.T) {
	t.Skip("integration test: requires Docker")
}
