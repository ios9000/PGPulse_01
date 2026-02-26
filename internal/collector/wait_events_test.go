package collector

import (
	"testing"
	"time"

	"github.com/ios9000/PGPulse_01/internal/version"
)

func TestWaitEventsCollector_Name(t *testing.T) {
	c := NewWaitEventsCollector("test", version.PGVersion{Major: 16})
	if c.Name() != "wait_events" {
		t.Errorf("Name() = %q, want %q", c.Name(), "wait_events")
	}
}

func TestWaitEventsCollector_Interval(t *testing.T) {
	c := NewWaitEventsCollector("test", version.PGVersion{Major: 16})
	if c.Interval() != 10*time.Second {
		t.Errorf("Interval() = %v, want 10s", c.Interval())
	}
}

// TestWaitEventsCollector_Collect verifies that 3 event rows produce 3 per-event
// count points plus 1 total_backends point (4 total), with correct labels and sum.
func TestWaitEventsCollector_Collect(t *testing.T) {
	c := NewWaitEventsCollector("test", version.PGVersion{Major: 16})

	rows := []waitEventRow{
		{eventType: "IO", event: "WALRead", count: 5},
		{eventType: "Lock", event: "relation", count: 3},
		{eventType: "CPU", event: "Running", count: 10},
	}

	points := c.buildMetrics(rows)

	if len(points) != 4 {
		t.Errorf("expected 4 points (3 count + 1 total), got %d: %v",
			len(points), stmtMetricNames(points))
	}

	// Verify individual count points.
	io := findWaitEventMetric(points, "pgpulse.wait_events.count", "IO", "WALRead")
	if io == nil {
		t.Error("expected wait_events.count for IO/WALRead")
	} else if io.Value != 5 {
		t.Errorf("IO/WALRead count = %v, want 5", io.Value)
	}

	lock := findWaitEventMetric(points, "pgpulse.wait_events.count", "Lock", "relation")
	if lock == nil {
		t.Error("expected wait_events.count for Lock/relation")
	} else if lock.Value != 3 {
		t.Errorf("Lock/relation count = %v, want 3", lock.Value)
	}

	cpu := findWaitEventMetric(points, "pgpulse.wait_events.count", "CPU", "Running")
	if cpu == nil {
		t.Error("expected wait_events.count for CPU/Running")
	} else if cpu.Value != 10 {
		t.Errorf("CPU/Running count = %v, want 10", cpu.Value)
	}

	// Verify total = 5+3+10 = 18.
	total, ok := findCheckpointMetric(points, "pgpulse.wait_events.total_backends")
	if !ok {
		t.Fatal("expected pgpulse.wait_events.total_backends")
	}
	if total.Value != 18 {
		t.Errorf("total_backends = %v, want 18", total.Value)
	}
}

// TestWaitEventsCollector_Collect_Empty verifies that zero rows produce a single
// total_backends=0 point (no count points, healthy state).
func TestWaitEventsCollector_Collect_Empty(t *testing.T) {
	c := NewWaitEventsCollector("test", version.PGVersion{Major: 16})

	points := c.buildMetrics(nil)

	if len(points) != 1 {
		t.Errorf("expected 1 point (total_backends only), got %d", len(points))
	}

	total, ok := findCheckpointMetric(points, "pgpulse.wait_events.total_backends")
	if !ok {
		t.Fatal("expected pgpulse.wait_events.total_backends")
	}
	if total.Value != 0 {
		t.Errorf("total_backends = %v, want 0", total.Value)
	}
}

// TestWaitEventsCollector_Collect_NullEventHandling verifies that backends
// active on CPU (COALESCE'd to "CPU"/"Running" by the SQL) produce a point
// with the correct labels and are counted in the total.
func TestWaitEventsCollector_Collect_NullEventHandling(t *testing.T) {
	c := NewWaitEventsCollector("test", version.PGVersion{Major: 16})

	// The SQL uses COALESCE so by the time Go sees the row, the strings are
	// already "CPU" and "Running". Test that they are emitted correctly.
	rows := []waitEventRow{
		{eventType: "CPU", event: "Running", count: 7},
	}

	points := c.buildMetrics(rows)

	// 1 count point + 1 total = 2.
	if len(points) != 2 {
		t.Errorf("expected 2 points, got %d", len(points))
	}

	p := findWaitEventMetric(points, "pgpulse.wait_events.count", "CPU", "Running")
	if p == nil {
		t.Fatal("expected wait_events.count with wait_event_type=CPU, wait_event=Running")
	}
	if p.Value != 7 {
		t.Errorf("CPU/Running count = %v, want 7", p.Value)
	}

	total, ok := findCheckpointMetric(points, "pgpulse.wait_events.total_backends")
	if !ok {
		t.Fatal("expected pgpulse.wait_events.total_backends")
	}
	if total.Value != 7 {
		t.Errorf("total_backends = %v, want 7", total.Value)
	}
}

// --- helpers local to this file ---

// findWaitEventMetric returns the first MetricPoint matching the given name,
// wait_event_type label, and wait_event label, or nil if not found.
func findWaitEventMetric(points []MetricPoint, name, eventType, event string) *MetricPoint {
	for i := range points {
		p := &points[i]
		if p.Metric == name &&
			p.Labels["wait_event_type"] == eventType &&
			p.Labels["wait_event"] == event {
			return p
		}
	}
	return nil
}
