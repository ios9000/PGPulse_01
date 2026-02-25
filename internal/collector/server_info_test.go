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

func TestServerInfoCollector_PG17(t *testing.T) {
	conn := setupPG(t, "17")
	ctx := context.Background()

	v, err := version.Detect(ctx, conn)
	require.NoError(t, err)

	c := collector.NewServerInfoCollector("test-instance", v)
	points, err := c.Collect(ctx, conn)
	require.NoError(t, err)
	require.NotEmpty(t, points)

	names := metricNames(points)
	assert.Contains(t, names, "pgpulse.server.start_time_unix")
	assert.Contains(t, names, "pgpulse.server.uptime_seconds")
	assert.Contains(t, names, "pgpulse.server.is_in_recovery")
	assert.Contains(t, names, "pgpulse.server.is_in_backup")

	// Start time must be after 2020-01-01
	startTime := findMetric(points, "pgpulse.server.start_time_unix")
	require.NotNil(t, startTime)
	assert.Greater(t, startTime.Value, float64(1577836800), "start_time should be after 2020-01-01")

	// Uptime must be positive
	uptime := findMetric(points, "pgpulse.server.uptime_seconds")
	require.NotNil(t, uptime)
	assert.Greater(t, uptime.Value, 0.0)

	// Fresh standalone instance should not be in recovery
	recovery := findMetric(points, "pgpulse.server.is_in_recovery")
	require.NotNil(t, recovery)
	assert.Equal(t, 0.0, recovery.Value)

	// PG 17: pg_is_in_backup() removed — must emit 0.0 without error
	backup := findMetric(points, "pgpulse.server.is_in_backup")
	require.NotNil(t, backup)
	assert.Equal(t, 0.0, backup.Value)
}

func TestServerInfoCollector_PG14(t *testing.T) {
	conn := setupPG(t, "14")
	ctx := context.Background()

	v, err := version.Detect(ctx, conn)
	require.NoError(t, err)

	c := collector.NewServerInfoCollector("test-instance", v)
	points, err := c.Collect(ctx, conn)
	require.NoError(t, err)

	// PG 14: pg_is_in_backup() must execute without error and return 0.0 (no backup running)
	backup := findMetric(points, "pgpulse.server.is_in_backup")
	require.NotNil(t, backup, "PG14 must emit is_in_backup via pg_is_in_backup()")
	assert.Equal(t, 0.0, backup.Value, "no backup should be running in test container")
}

func TestServerInfoCollector_Name(t *testing.T) {
	c := collector.NewServerInfoCollector("x", version.PGVersion{})
	assert.Equal(t, "server_info", c.Name())
}

func TestServerInfoCollector_Interval(t *testing.T) {
	c := collector.NewServerInfoCollector("x", version.PGVersion{})
	assert.Equal(t, 60*time.Second, c.Interval())
}
