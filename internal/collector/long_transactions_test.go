package collector

import (
	"testing"
	"time"

	"github.com/ios9000/PGPulse_01/internal/version"
)

func TestLongTransactionsCollector_Name(t *testing.T) {
	c := NewLongTransactionsCollector("test", version.PGVersion{Major: 16})
	if c.Name() != "long_transactions" {
		t.Errorf("Name() = %q, want %q", c.Name(), "long_transactions")
	}
}

func TestLongTransactionsCollector_Interval(t *testing.T) {
	c := NewLongTransactionsCollector("test", version.PGVersion{Major: 16})
	if c.Interval() != 10*time.Second {
		t.Errorf("Interval() = %v, want 10s", c.Interval())
	}
}

// TestLongTransactionsCollector_Collect_Both verifies that two rows (active + waiting)
// produce exactly 4 points with correct values.
func TestLongTransactionsCollector_Collect_Both(t *testing.T) {
	c := NewLongTransactionsCollector("test", version.PGVersion{Major: 16})

	rows := []longTxnRow{
		{txnType: "active", count: 3, oldestSeconds: 120.5},
		{txnType: "waiting", count: 1, oldestSeconds: 45.2},
	}

	points := c.buildMetrics(rows)

	if len(points) != 4 {
		t.Fatalf("expected 4 points, got %d: %v", len(points), stmtMetricNames(points))
	}

	assertLongTxnVal(t, points, "pg.long_transactions.count", "active", 3)
	assertLongTxnVal(t, points, "pg.long_transactions.oldest_seconds", "active", 120.5)
	assertLongTxnVal(t, points, "pg.long_transactions.count", "waiting", 1)
	assertLongTxnVal(t, points, "pg.long_transactions.oldest_seconds", "waiting", 45.2)
}

// TestLongTransactionsCollector_Collect_ActiveOnly verifies that when only active
// rows exist, waiting metrics are still emitted with value 0.
func TestLongTransactionsCollector_Collect_ActiveOnly(t *testing.T) {
	c := NewLongTransactionsCollector("test", version.PGVersion{Major: 16})

	rows := []longTxnRow{
		{txnType: "active", count: 5, oldestSeconds: 200.0},
	}

	points := c.buildMetrics(rows)

	if len(points) != 4 {
		t.Fatalf("expected 4 points, got %d", len(points))
	}

	// Active values should be present.
	assertLongTxnVal(t, points, "pg.long_transactions.count", "active", 5)
	assertLongTxnVal(t, points, "pg.long_transactions.oldest_seconds", "active", 200.0)

	// Waiting should be zero-filled.
	assertLongTxnVal(t, points, "pg.long_transactions.count", "waiting", 0)
	assertLongTxnVal(t, points, "pg.long_transactions.oldest_seconds", "waiting", 0)
}

// TestLongTransactionsCollector_Collect_None verifies that zero rows produce
// 4 zero-value points (2 types × 2 metrics).
func TestLongTransactionsCollector_Collect_None(t *testing.T) {
	c := NewLongTransactionsCollector("test", version.PGVersion{Major: 16})

	points := c.buildMetrics(nil)

	if len(points) != 4 {
		t.Fatalf("expected 4 points, got %d", len(points))
	}

	assertLongTxnVal(t, points, "pg.long_transactions.count", "active", 0)
	assertLongTxnVal(t, points, "pg.long_transactions.oldest_seconds", "active", 0)
	assertLongTxnVal(t, points, "pg.long_transactions.count", "waiting", 0)
	assertLongTxnVal(t, points, "pg.long_transactions.oldest_seconds", "waiting", 0)
}

// TestLongTransactionsCollector_Collect_WaitingOnly verifies that when only waiting
// rows exist, active metrics are still emitted with value 0.
func TestLongTransactionsCollector_Collect_WaitingOnly(t *testing.T) {
	c := NewLongTransactionsCollector("test", version.PGVersion{Major: 16})

	rows := []longTxnRow{
		{txnType: "waiting", count: 2, oldestSeconds: 60.0},
	}

	points := c.buildMetrics(rows)

	if len(points) != 4 {
		t.Fatalf("expected 4 points, got %d", len(points))
	}

	// Waiting values should be present.
	assertLongTxnVal(t, points, "pg.long_transactions.count", "waiting", 2)
	assertLongTxnVal(t, points, "pg.long_transactions.oldest_seconds", "waiting", 60.0)

	// Active should be zero-filled.
	assertLongTxnVal(t, points, "pg.long_transactions.count", "active", 0)
	assertLongTxnVal(t, points, "pg.long_transactions.oldest_seconds", "active", 0)
}

// --- helpers local to this file ---

// findLongTxnMetric returns the first MetricPoint matching the given name and
// type label, or nil if not found.
func findLongTxnMetric(points []MetricPoint, name, txnType string) *MetricPoint {
	for i := range points {
		p := &points[i]
		if p.Metric == name && p.Labels["type"] == txnType {
			return p
		}
	}
	return nil
}

// assertLongTxnVal finds a metric by name + type label and asserts its value.
func assertLongTxnVal(t *testing.T, points []MetricPoint, name, txnType string, want float64) {
	t.Helper()
	m := findLongTxnMetric(points, name, txnType)
	if m == nil {
		t.Errorf("metric %q with type=%q not found", name, txnType)
		return
	}
	if m.Value != want {
		t.Errorf("metric %q type=%q = %v, want %v", name, txnType, m.Value, want)
	}
}
