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

func TestExtensionsCollector_WithPGSS(t *testing.T) {
	conn := setupPGWithStatements(t, "17")
	ctx := context.Background()

	v, err := version.Detect(ctx, conn)
	require.NoError(t, err)

	c := collector.NewExtensionsCollector("test-instance", v)
	points, err := c.Collect(ctx, conn, collector.InstanceContext{})
	require.NoError(t, err)
	require.NotEmpty(t, points)

	// pg_stat_statements is installed — must report 1.0
	installed := findMetric(points, "pgpulse.extensions.pgss_installed")
	require.NotNil(t, installed)
	assert.Equal(t, 1.0, installed.Value)

	// Fill percentage must be present and in [0, 100]
	fillPct := findMetric(points, "pgpulse.extensions.pgss_fill_pct")
	require.NotNil(t, fillPct, "pgss_fill_pct must be emitted when pgss is installed")
	assert.GreaterOrEqual(t, fillPct.Value, 0.0)
	assert.LessOrEqual(t, fillPct.Value, 100.0)

	// Stats reset must be present (PG 17 ≥ 14)
	statsReset := findMetric(points, "pgpulse.extensions.pgss_stats_reset_unix")
	require.NotNil(t, statsReset, "pgss_stats_reset_unix must be emitted on PG ≥ 14 with pgss installed")
}

func TestExtensionsCollector_WithoutPGSS(t *testing.T) {
	conn := setupPG(t, "17") // no pg_stat_statements
	ctx := context.Background()

	v, err := version.Detect(ctx, conn)
	require.NoError(t, err)

	c := collector.NewExtensionsCollector("test-instance", v)
	points, err := c.Collect(ctx, conn, collector.InstanceContext{})
	require.NoError(t, err)

	// Not installed — must report 0.0
	installed := findMetric(points, "pgpulse.extensions.pgss_installed")
	require.NotNil(t, installed)
	assert.Equal(t, 0.0, installed.Value)

	// Fill percentage and stats reset must NOT be emitted
	assert.Nil(t, findMetric(points, "pgpulse.extensions.pgss_fill_pct"),
		"pgss_fill_pct must not be emitted when pgss is not installed")
	assert.Nil(t, findMetric(points, "pgpulse.extensions.pgss_stats_reset_unix"),
		"pgss_stats_reset_unix must not be emitted when pgss is not installed")
}

func TestExtensionsCollector_Name(t *testing.T) {
	c := collector.NewExtensionsCollector("x", version.PGVersion{})
	assert.Equal(t, "extensions", c.Name())
}

func TestExtensionsCollector_Interval(t *testing.T) {
	c := collector.NewExtensionsCollector("x", version.PGVersion{})
	assert.Equal(t, 300*time.Second, c.Interval())
}
