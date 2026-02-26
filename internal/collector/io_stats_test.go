package collector

import (
	"context"
	"testing"
	"time"

	"github.com/ios9000/PGPulse_01/internal/version"
)

func TestIOStats_NameAndInterval(t *testing.T) {
	c := NewIOStatsCollector("test", version.PGVersion{Major: 16})
	if c.Name() != "io_stats" {
		t.Errorf("Name() = %q, want %q", c.Name(), "io_stats")
	}
	if c.Interval() != 60*time.Second {
		t.Errorf("Interval() = %v, want 60s", c.Interval())
	}
}

func TestIOStats_PG15ReturnsNil(t *testing.T) {
	c := NewIOStatsCollector("test", version.PGVersion{Major: 15})
	// conn is nil — safe because version check returns before any DB access.
	points, err := c.Collect(context.Background(), nil, InstanceContext{})
	if err != nil {
		t.Fatalf("PG 15 should return nil error, got: %v", err)
	}
	if points != nil {
		t.Errorf("PG 15 should return nil points, got %d points", len(points))
	}
}

func TestIOStats_NullSkipped(t *testing.T) {
	c := NewIOStatsCollector("test", version.PGVersion{Major: 16})

	// Mix of real values and -1 sentinels (representing NULL columns in pg_stat_io).
	row := ioStatsRow{
		backendType: "client backend",
		object:      "relation",
		ioContext:   "normal",
		reads:       100,
		readTime:    -1, // NULL: timing not tracked for this combination
		writes:      50,
		writeTime:   -1, // NULL
		extends:     -1, // NULL
		extendTime:  -1, // NULL
		hits:        200,
		evictions:   0,  // zero is a valid value, must be emitted
		reuses:      -1, // NULL
		fsyncs:      -1, // NULL
		fsyncTime:   -1, // NULL
	}

	points := c.rowPoints(row)

	// No emitted metric should carry a negative value.
	for _, p := range points {
		if p.Value < 0 {
			t.Errorf("metric %q has value %v; -1 sentinel must not be emitted", p.Metric, p.Value)
		}
	}

	// Metrics with non-negative values must be present.
	if !hasMetric(points, "pgpulse.io.reads") {
		t.Error("expected pgpulse.io.reads")
	}
	if !hasMetric(points, "pgpulse.io.writes") {
		t.Error("expected pgpulse.io.writes")
	}
	if !hasMetric(points, "pgpulse.io.hits") {
		t.Error("expected pgpulse.io.hits")
	}
	// evictions=0 is valid; zero must be emitted (only -1 is suppressed).
	if !hasMetric(points, "pgpulse.io.evictions") {
		t.Error("expected pgpulse.io.evictions=0 to be emitted (zero is valid, not NULL)")
	}

	// Metrics mapped from NULL (-1) must not appear.
	if hasMetric(points, "pgpulse.io.read_time") {
		t.Error("pgpulse.io.read_time must not be emitted when value is -1 (NULL)")
	}
	if hasMetric(points, "pgpulse.io.write_time") {
		t.Error("pgpulse.io.write_time must not be emitted when value is -1 (NULL)")
	}
	if hasMetric(points, "pgpulse.io.extends") {
		t.Error("pgpulse.io.extends must not be emitted when value is -1 (NULL)")
	}
	if hasMetric(points, "pgpulse.io.fsyncs") {
		t.Error("pgpulse.io.fsyncs must not be emitted when value is -1 (NULL)")
	}
	if hasMetric(points, "pgpulse.io.fsync_time") {
		t.Error("pgpulse.io.fsync_time must not be emitted when value is -1 (NULL)")
	}
}

// TestIOStats_Integration is a stub for Docker-based integration testing.
// It verifies that pg_stat_io metrics are emitted when querying a real PG 16+ instance.
func TestIOStats_Integration(t *testing.T) {
	t.Skip("integration test: requires Docker with PostgreSQL 16+")
}
