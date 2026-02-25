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

func TestSettingsCollector_PG17(t *testing.T) {
	conn := setupPG(t, "17")
	ctx := context.Background()

	v, err := version.Detect(ctx, conn)
	require.NoError(t, err)

	c := collector.NewSettingsCollector("test-instance", v)
	points, err := c.Collect(ctx, conn)
	require.NoError(t, err)
	require.NotEmpty(t, points)

	names := metricNames(points)
	assert.Contains(t, names, "pgpulse.settings.track_io_timing")
	assert.Contains(t, names, "pgpulse.settings.shared_buffers_8kb")
	assert.Contains(t, names, "pgpulse.settings.max_locks_per_tx")
	assert.Contains(t, names, "pgpulse.settings.max_prepared_tx")
}

func TestSettingsCollector_BoolConversion(t *testing.T) {
	conn := setupPG(t, "17")
	ctx := context.Background()

	v, err := version.Detect(ctx, conn)
	require.NoError(t, err)

	c := collector.NewSettingsCollector("test-instance", v)
	points, err := c.Collect(ctx, conn)
	require.NoError(t, err)

	trackIO := findMetric(points, "pgpulse.settings.track_io_timing")
	require.NotNil(t, trackIO)
	// Must be exactly 0.0 or 1.0 — no other value is valid for a bool setting
	assert.True(t, trackIO.Value == 0.0 || trackIO.Value == 1.0,
		"track_io_timing must be 0.0 or 1.0, got %v", trackIO.Value)
}

func TestSettingsCollector_SharedBuffersPositive(t *testing.T) {
	conn := setupPG(t, "17")
	ctx := context.Background()

	v, err := version.Detect(ctx, conn)
	require.NoError(t, err)

	c := collector.NewSettingsCollector("test-instance", v)
	points, err := c.Collect(ctx, conn)
	require.NoError(t, err)

	sharedBufs := findMetric(points, "pgpulse.settings.shared_buffers_8kb")
	require.NotNil(t, sharedBufs)
	assert.Greater(t, sharedBufs.Value, 0.0, "shared_buffers must be positive")
}

func TestSettingsCollector_Name(t *testing.T) {
	c := collector.NewSettingsCollector("x", version.PGVersion{})
	assert.Equal(t, "settings", c.Name())
}

func TestSettingsCollector_Interval(t *testing.T) {
	c := collector.NewSettingsCollector("x", version.PGVersion{})
	assert.Equal(t, 300*time.Second, c.Interval())
}
