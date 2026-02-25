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

func TestTransactionsCollector_PG17(t *testing.T) {
	conn := setupPG(t, "17")
	ctx := context.Background()

	// Run a transaction in testdb so it has xact_commit > 0
	_, err := conn.Exec(ctx, "SELECT 1")
	require.NoError(t, err)

	v, err := version.Detect(ctx, conn)
	require.NoError(t, err)

	c := collector.NewTransactionsCollector("test-instance", v)
	points, err := c.Collect(ctx, conn)
	require.NoError(t, err)

	// At least one database should have transaction data
	require.NotEmpty(t, points, "expected metrics after running transactions")

	// Find commit_ratio_pct for testdb
	commitRatio := findMetricWithLabel(points, "pgpulse.transactions.commit_ratio_pct", "database", "testdb")
	require.NotNil(t, commitRatio, "testdb should have commit_ratio_pct after a transaction")
	assert.GreaterOrEqual(t, commitRatio.Value, 0.0)
	assert.LessOrEqual(t, commitRatio.Value, 100.0)

	// Find deadlocks for testdb
	deadlocks := findMetricWithLabel(points, "pgpulse.transactions.deadlocks", "database", "testdb")
	require.NotNil(t, deadlocks, "testdb should have deadlocks metric")
	assert.GreaterOrEqual(t, deadlocks.Value, 0.0)
}

func TestTransactionsCollector_Name(t *testing.T) {
	c := collector.NewTransactionsCollector("x", version.PGVersion{})
	assert.Equal(t, "transactions", c.Name())
}

func TestTransactionsCollector_Interval(t *testing.T) {
	c := collector.NewTransactionsCollector("x", version.PGVersion{})
	assert.Equal(t, 60*time.Second, c.Interval())
}
