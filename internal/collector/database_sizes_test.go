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

func TestDatabaseSizesCollector_PG17(t *testing.T) {
	conn := setupPG(t, "17")
	ctx := context.Background()

	v, err := version.Detect(ctx, conn)
	require.NoError(t, err)

	c := collector.NewDatabaseSizesCollector("test-instance", v)
	points, err := c.Collect(ctx, conn, collector.InstanceContext{})
	require.NoError(t, err)
	require.NotEmpty(t, points)

	// testdb must be present
	testdbSize := findMetricWithLabel(points, "pgpulse.database.size_bytes", "database", "testdb")
	require.NotNil(t, testdbSize, "testdb must appear in database sizes")
	assert.Greater(t, testdbSize.Value, 0.0, "testdb size must be positive")

	// All emitted metrics must have positive sizes and database labels
	for _, p := range points {
		assert.Equal(t, "pgpulse.database.size_bytes", p.Metric)
		assert.NotEmpty(t, p.Labels["database"])
		assert.Greater(t, p.Value, 0.0)
	}
}

func TestDatabaseSizesCollector_Name(t *testing.T) {
	c := collector.NewDatabaseSizesCollector("x", version.PGVersion{})
	assert.Equal(t, "database_sizes", c.Name())
}

func TestDatabaseSizesCollector_Interval(t *testing.T) {
	c := collector.NewDatabaseSizesCollector("x", version.PGVersion{})
	assert.Equal(t, 300*time.Second, c.Interval())
}
