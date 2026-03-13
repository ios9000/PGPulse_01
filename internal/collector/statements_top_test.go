package collector

import (
	"testing"
	"time"

	"github.com/ios9000/PGPulse_01/internal/version"
)

func TestStatementsTopCollector_NameAndInterval(t *testing.T) {
	c := NewStatementsTopCollector("test", version.PGVersion{Major: 16}, 0)
	if c.Name() != "statements_top" {
		t.Errorf("Name() = %q, want %q", c.Name(), "statements_top")
	}
	if c.Interval() != 60*time.Second {
		t.Errorf("Interval() = %v, want 60s", c.Interval())
	}
	if c.limit != 20 {
		t.Errorf("default limit = %d, want 20", c.limit)
	}
}

// TestStatementsTopCollector_PgssNotInstalled verifies that Collect() returns
// nil, nil when pg_stat_statements is not installed. Requires a live PostgreSQL
// connection without the extension loaded.
func TestStatementsTopCollector_PgssNotInstalled(t *testing.T) {
	t.Skip("integration test: requires Docker with PostgreSQL")
}

// TestStatementsTopCollector_EmptyPgss verifies that buildTopMetrics returns an
// empty (non-nil) slice when pg_stat_statements has no entries.
func TestStatementsTopCollector_EmptyPgss(t *testing.T) {
	c := NewStatementsTopCollector("test", version.PGVersion{Major: 16}, 20)
	points := c.buildTopMetrics(nil)
	if points == nil {
		t.Error("expected empty slice, got nil")
	}
	if len(points) != 0 {
		t.Errorf("expected 0 metrics for empty pgss, got %d", len(points))
	}
}

// TestStatementsTopCollector_NormalTopN verifies that N rows with a larger total
// produce N*6 per-query metrics plus 6 "other" bucket metrics.
func TestStatementsTopCollector_NormalTopN(t *testing.T) {
	c := NewStatementsTopCollector("test", version.PGVersion{Major: 16}, 20)

	// Totals are larger than the sum of the 2 rows → "other" bucket must be emitted.
	rows := []topQueryRow{
		{
			queryID: "1", dbID: "5", userID: "10",
			calls: 100, rowCount: 500, totalTime: 2000, ioTime: 500, cpuTime: 1500, avgTime: 20,
			ttlCalls: 300, ttlRows: 1000, ttlTime: 5000, ttlIO: 1000, ttlCPU: 4000,
		},
		{
			queryID: "2", dbID: "5", userID: "10",
			calls: 50, rowCount: 200, totalTime: 1000, ioTime: 200, cpuTime: 800, avgTime: 20,
			ttlCalls: 300, ttlRows: 1000, ttlTime: 5000, ttlIO: 1000, ttlCPU: 4000,
		},
	}

	points := c.buildTopMetrics(rows)

	// N=2: 2*6=12 per-query + 6 other = 18.
	const want = 18
	if len(points) != want {
		t.Errorf("expected %d metrics (2*6 + 6 other), got %d: %v",
			want, len(points), stmtMetricNames(points))
	}

	// Spot-check that "other" bucket exists.
	if !hasTopBucket(points, "other") {
		t.Error("expected 'other' bucket metrics to be emitted")
	}

	// Spot-check that query "1" bucket exists.
	if !hasTopBucket(points, "1") {
		t.Error("expected queryid='1' metrics to be emitted")
	}
}

// TestStatementsTopCollector_FewerThanLimit verifies that when top-N rows account
// for all calls, no "other" bucket is emitted.
func TestStatementsTopCollector_FewerThanLimit(t *testing.T) {
	c := NewStatementsTopCollector("test", version.PGVersion{Major: 16}, 20)

	// totals == row values → otherCalls = 0 → no "other" bucket.
	rows := []topQueryRow{
		{
			queryID: "1", dbID: "5", userID: "10",
			calls: 100, rowCount: 500, totalTime: 2000, ioTime: 500, cpuTime: 1500, avgTime: 20,
			ttlCalls: 100, ttlRows: 500, ttlTime: 2000, ttlIO: 500, ttlCPU: 1500,
		},
	}

	points := c.buildTopMetrics(rows)

	// N=1: 1*6 = 6, no other.
	if len(points) != 6 {
		t.Errorf("expected 6 metrics (no 'other' when totals == top-N), got %d", len(points))
	}

	if hasTopBucket(points, "other") {
		t.Error("'other' bucket must not be emitted when top-N covers all queries")
	}
}

// TestStatementsTopCollector_NegativeCpuTime verifies that cpu_time_ms is clamped
// to 0 when the computed value is negative (IO timing exceeds total exec time).
func TestStatementsTopCollector_NegativeCpuTime(t *testing.T) {
	c := NewStatementsTopCollector("test", version.PGVersion{Major: 16}, 20)

	// cpuTime = totalTime - ioTime = 100 - 150 = -50 (IO measured > exec time).
	rows := []topQueryRow{
		{
			queryID: "1", dbID: "5", userID: "10",
			calls: 10, rowCount: 100, totalTime: 100, ioTime: 150, cpuTime: -50, avgTime: 10,
			ttlCalls: 10, ttlRows: 100, ttlTime: 100, ttlIO: 150, ttlCPU: -50,
		},
	}

	points := c.buildTopMetrics(rows)

	for _, p := range points {
		if p.Metric == "pg.statements.top.cpu_time_ms" && p.Value < 0 {
			t.Errorf("cpu_time_ms must be clamped to 0, got %v", p.Value)
		}
	}

	// The cpu_time_ms for queryid="1" must be 0.
	found := false
	for _, p := range points {
		if p.Metric == "pg.statements.top.cpu_time_ms" && p.Labels["queryid"] == "1" {
			found = true
			if p.Value != 0 {
				t.Errorf("cpu_time_ms for queryid=1 = %v, want 0 (clamped)", p.Value)
			}
		}
	}
	if !found {
		t.Error("expected pgpulse.statements.top.cpu_time_ms for queryid=1")
	}
}

// TestStatementsTopCollector_OtherBucketArithmetic verifies that the "other" bucket
// values equal totals minus the sum of top-N rows.
func TestStatementsTopCollector_OtherBucketArithmetic(t *testing.T) {
	c := NewStatementsTopCollector("test", version.PGVersion{Major: 16}, 20)

	// Two rows; totals are sum of those two plus a remainder.
	// Top-N sum: calls=150, rows=700, time=3000, io=700, cpu=2300
	// Other:     calls=150, rows=300, time=2000, io=300, cpu=1700
	rows := []topQueryRow{
		{
			queryID: "1", dbID: "5", userID: "10",
			calls: 100, rowCount: 500, totalTime: 2000, ioTime: 500, cpuTime: 1500, avgTime: 20,
			ttlCalls: 300, ttlRows: 1000, ttlTime: 5000, ttlIO: 1000, ttlCPU: 4000,
		},
		{
			queryID: "2", dbID: "5", userID: "10",
			calls: 50, rowCount: 200, totalTime: 1000, ioTime: 200, cpuTime: 800, avgTime: 20,
			ttlCalls: 300, ttlRows: 1000, ttlTime: 5000, ttlIO: 1000, ttlCPU: 4000,
		},
	}

	points := c.buildTopMetrics(rows)

	// Verify "other" bucket values.
	tests := []struct {
		metric string
		want   float64
	}{
		{"pg.statements.top.calls", 150},       // 300 - (100+50)
		{"pg.statements.top.rows", 300},        // 1000 - (500+200)
		{"pg.statements.top.total_time_ms", 2000}, // 5000 - (2000+1000)
		{"pg.statements.top.io_time_ms", 300},  // 1000 - (500+200)
		{"pg.statements.top.cpu_time_ms", 1700}, // 4000 - (1500+800)
		{"pg.statements.top.avg_time_ms", 2000.0 / 150.0}, // otherTime/otherCalls
	}

	for _, tc := range tests {
		var found *MetricPoint
		for i := range points {
			if points[i].Metric == tc.metric && points[i].Labels["queryid"] == "other" {
				found = &points[i]
				break
			}
		}
		if found == nil {
			t.Errorf("other bucket: metric %q not found", tc.metric)
			continue
		}
		if found.Value != tc.want {
			t.Errorf("other bucket: %q = %v, want %v", tc.metric, found.Value, tc.want)
		}
	}
}

// --- helpers local to this file ---

// hasTopBucket returns true if any point with the given queryid label exists.
func hasTopBucket(points []MetricPoint, queryID string) bool {
	for _, p := range points {
		if p.Labels["queryid"] == queryID {
			return true
		}
	}
	return false
}
