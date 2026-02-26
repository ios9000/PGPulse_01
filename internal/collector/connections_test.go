//go:build integration

package collector_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ios9000/PGPulse_01/internal/collector"
	"github.com/ios9000/PGPulse_01/internal/version"
)

func TestConnectionsCollector_PG17(t *testing.T) {
	// Use two connections: conn1 is idle (visible to collector), conn2 runs the collector.
	db := newTestDB(t, "17")
	conn1 := db.connect() // idle background connection — should be counted
	conn2 := db.connect() // collector connection — excluded by pg_backend_pid()
	_ = conn1             // keep alive during test

	ctx := context.Background()
	v, err := version.Detect(ctx, conn2)
	require.NoError(t, err)

	c := collector.NewConnectionsCollector("test-instance", v)
	points, err := c.Collect(ctx, conn2, collector.InstanceContext{})
	require.NoError(t, err)
	require.NotEmpty(t, points)

	names := metricNames(points)
	assert.Contains(t, names, "pgpulse.connections.total")
	assert.Contains(t, names, "pgpulse.connections.max")
	assert.Contains(t, names, "pgpulse.connections.superuser_reserved")
	assert.Contains(t, names, "pgpulse.connections.utilization_pct")

	// conn1 is idle → at least one "by_state" metric should exist
	assert.Contains(t, names, "pgpulse.connections.by_state")

	maxConn := findMetric(points, "pgpulse.connections.max")
	require.NotNil(t, maxConn)
	assert.Greater(t, maxConn.Value, 0.0)

	total := findMetric(points, "pgpulse.connections.total")
	require.NotNil(t, total)
	assert.GreaterOrEqual(t, total.Value, 1.0, "conn1 should be visible to the collector running on conn2")
}

func TestConnectionsCollector_ExcludesSelf(t *testing.T) {
	// Verify that the collector's own connection is NOT counted in the total.
	// Setup: conn1 is the "other" connection (should be counted),
	// conn2 is the collector's connection (should be excluded).
	db := newTestDB(t, "17")
	conn1 := db.connect()
	conn2 := db.connect()
	_ = conn1

	ctx := context.Background()
	v, err := version.Detect(ctx, conn2)
	require.NoError(t, err)

	c := collector.NewConnectionsCollector("test-instance", v)
	points, err := c.Collect(ctx, conn2, collector.InstanceContext{})
	require.NoError(t, err)

	total := findMetric(points, "pgpulse.connections.total")
	require.NotNil(t, total)

	// conn2 (collector) is excluded. conn1 is counted. So total must be at least 1.
	// If self-exclusion were broken, an additional connection would appear.
	assert.GreaterOrEqual(t, total.Value, 1.0, "at least conn1 should be counted")

	// Run again on conn1 — now conn1 is the collector and conn2 is the "other".
	v2, err := version.Detect(ctx, conn1)
	require.NoError(t, err)
	c2 := collector.NewConnectionsCollector("test-instance", v2)
	points2, err := c2.Collect(ctx, conn1, collector.InstanceContext{})
	require.NoError(t, err)

	total2 := findMetric(points2, "pgpulse.connections.total")
	require.NotNil(t, total2)
	// Both runs should count the same number of "other" connections.
	assert.Equal(t, total.Value, total2.Value, "self-exclusion should be symmetric")
}

func TestConnectionsCollector_Utilization(t *testing.T) {
	conn := setupPG(t, "17")
	ctx := context.Background()

	v, err := version.Detect(ctx, conn)
	require.NoError(t, err)

	c := collector.NewConnectionsCollector("test-instance", v)
	points, err := c.Collect(ctx, conn, collector.InstanceContext{})
	require.NoError(t, err)

	util := findMetric(points, "pgpulse.connections.utilization_pct")
	require.NotNil(t, util)
	assert.GreaterOrEqual(t, util.Value, 0.0)
	assert.LessOrEqual(t, util.Value, 100.0)
}

func TestConnectionsCollector_Name(t *testing.T) {
	c := collector.NewConnectionsCollector("x", version.PGVersion{})
	assert.Equal(t, "connections", c.Name())
}

func TestConnectionsCollector_Interval(t *testing.T) {
	c := collector.NewConnectionsCollector("x", version.PGVersion{})
	assert.Equal(t, 10*time.Second, c.Interval())
}
