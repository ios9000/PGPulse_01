package collector

import (
	"testing"
	"time"

	"github.com/ios9000/PGPulse_01/internal/version"
)

func TestStatementsConfigCollector_NameAndInterval(t *testing.T) {
	c := NewStatementsConfigCollector("test", version.PGVersion{Major: 16})
	if c.Name() != "statements_config" {
		t.Errorf("Name() = %q, want %q", c.Name(), "statements_config")
	}
	if c.Interval() != 60*time.Second {
		t.Errorf("Interval() = %v, want 60s", c.Interval())
	}
}

// TestStatementsConfigCollector_PgssNotInstalled verifies that Collect() returns
// nil, nil when pg_stat_statements is not installed. Requires a live PostgreSQL
// connection without the extension loaded.
func TestStatementsConfigCollector_PgssNotInstalled(t *testing.T) {
	t.Skip("integration test: requires Docker with PostgreSQL")
}

// TestStatementsConfigCollector_Normal verifies that buildMetrics emits exactly
// 6 metrics when all settings are present and stats_reset is not NULL.
func TestStatementsConfigCollector_Normal(t *testing.T) {
	c := NewStatementsConfigCollector("test", version.PGVersion{Major: 16})

	settings := map[string]string{
		"pg_stat_statements.max":   "10000",
		"pg_stat_statements.track": "all",
		"track_io_timing":          "on",
	}
	count := 500.0
	resetAge := 3600.0

	points := c.buildMetrics(settings, count, &resetAge)

	if len(points) != 6 {
		t.Errorf("expected 6 metrics, got %d: %v", len(points), stmtMetricNames(points))
	}

	// statements.max = 10000
	assertStmtVal(t, points, "pgpulse.statements.max", 10000.0)

	// statements.fill_pct = 500/10000*100 = 5.0
	assertStmtVal(t, points, "pgpulse.statements.fill_pct", 5.0)

	// statements.track = 1 with label value="all"
	m, ok := findCheckpointMetric(points, "pgpulse.statements.track")
	if !ok {
		t.Error("expected pgpulse.statements.track metric")
	} else {
		if m.Value != 1.0 {
			t.Errorf("statements.track value = %v, want 1.0", m.Value)
		}
		if m.Labels["value"] != "all" {
			t.Errorf("statements.track label[value] = %q, want %q", m.Labels["value"], "all")
		}
	}

	// statements.track_io_timing = 1 (on)
	assertStmtVal(t, points, "pgpulse.statements.track_io_timing", 1.0)

	// statements.count = 500
	assertStmtVal(t, points, "pgpulse.statements.count", 500.0)

	// statements.stats_reset_age_seconds = 3600
	assertStmtVal(t, points, "pgpulse.statements.stats_reset_age_seconds", 3600.0)
}

// TestStatementsConfigCollector_NullStatsReset verifies that buildMetrics emits
// only 5 metrics when stats_reset is NULL (resetAge == nil).
func TestStatementsConfigCollector_NullStatsReset(t *testing.T) {
	c := NewStatementsConfigCollector("test", version.PGVersion{Major: 16})

	settings := map[string]string{
		"pg_stat_statements.max":   "10000",
		"pg_stat_statements.track": "top",
		"track_io_timing":          "off",
	}
	count := 100.0

	points := c.buildMetrics(settings, count, nil) // nil = NULL stats_reset

	if len(points) != 5 {
		t.Errorf("expected 5 metrics (no reset_age when NULL), got %d: %v",
			len(points), stmtMetricNames(points))
	}

	for _, p := range points {
		if p.Metric == "pgpulse.statements.stats_reset_age_seconds" {
			t.Error("stats_reset_age_seconds must not be emitted when stats_reset is NULL")
		}
	}
}

// TestStatementsConfigCollector_TrackIoTimingOn verifies that track_io_timing
// setting "on" is encoded as 1.0.
func TestStatementsConfigCollector_TrackIoTimingOn(t *testing.T) {
	c := NewStatementsConfigCollector("test", version.PGVersion{Major: 16})

	settings := map[string]string{
		"pg_stat_statements.max":   "10000",
		"pg_stat_statements.track": "all",
		"track_io_timing":          "on",
	}
	resetAge := 0.0

	points := c.buildMetrics(settings, 0, &resetAge)
	assertStmtVal(t, points, "pgpulse.statements.track_io_timing", 1.0)
}

// TestStatementsConfigCollector_TrackIoTimingOff verifies that any non-"on"
// value for track_io_timing is encoded as 0.0.
func TestStatementsConfigCollector_TrackIoTimingOff(t *testing.T) {
	c := NewStatementsConfigCollector("test", version.PGVersion{Major: 16})

	settings := map[string]string{
		"pg_stat_statements.max":   "10000",
		"pg_stat_statements.track": "all",
		"track_io_timing":          "off",
	}
	resetAge := 0.0

	points := c.buildMetrics(settings, 0, &resetAge)
	assertStmtVal(t, points, "pgpulse.statements.track_io_timing", 0.0)
}

// --- helpers local to this file ---

// stmtMetricNames extracts metric names for error messages.
func stmtMetricNames(points []MetricPoint) []string {
	names := make([]string, len(points))
	for i, p := range points {
		names[i] = p.Metric
	}
	return names
}

// assertStmtVal finds a metric by name and asserts its value.
func assertStmtVal(t *testing.T, points []MetricPoint, name string, want float64) {
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
