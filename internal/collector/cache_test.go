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

func TestCacheCollector_PG17(t *testing.T) {
	conn := setupPG(t, "17")
	ctx := context.Background()

	v, err := version.Detect(ctx, conn)
	require.NoError(t, err)

	c := collector.NewCacheCollector("test-instance", v)
	points, err := c.Collect(ctx, conn, collector.InstanceContext{})
	require.NoError(t, err)
	require.Len(t, points, 1)

	hitRatio := findMetric(points, "pg.cache.hit_ratio")
	require.NotNil(t, hitRatio)
	assert.GreaterOrEqual(t, hitRatio.Value, 0.0)
	assert.LessOrEqual(t, hitRatio.Value, 100.0)
}

func TestCacheCollector_ZeroDivisionGuard(t *testing.T) {
	// A fresh container may have no reads yet. The NULLIF+COALESCE guard must return 0, not error.
	conn := setupPG(t, "17")
	ctx := context.Background()

	v, err := version.Detect(ctx, conn)
	require.NoError(t, err)

	c := collector.NewCacheCollector("test-instance", v)
	points, err := c.Collect(ctx, conn, collector.InstanceContext{})
	require.NoError(t, err, "fresh instance with no reads must not error")

	hitRatio := findMetric(points, "pg.cache.hit_ratio")
	require.NotNil(t, hitRatio)
	assert.GreaterOrEqual(t, hitRatio.Value, 0.0)
}

func TestCacheCollector_Name(t *testing.T) {
	c := collector.NewCacheCollector("x", version.PGVersion{})
	assert.Equal(t, "cache", c.Name())
}

func TestCacheCollector_Interval(t *testing.T) {
	c := collector.NewCacheCollector("x", version.PGVersion{})
	assert.Equal(t, 60*time.Second, c.Interval())
}
