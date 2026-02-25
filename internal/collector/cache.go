package collector

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/ios9000/PGPulse_01/internal/version"
)

const sqlCacheHitRatio = `
SELECT COALESCE(
    sum(blks_hit) * 100.0 / NULLIF(sum(blks_hit) + sum(blks_read), 0),
    0
) AS hit_ratio
FROM pg_stat_database`

// CacheCollector collects the global PostgreSQL buffer cache hit ratio.
// It covers PGAM query Q14.
// PGAM bug fix: added NULLIF + COALESCE to prevent division by zero on fresh instances.
type CacheCollector struct {
	Base
}

// NewCacheCollector creates a new CacheCollector for the given PostgreSQL instance.
func NewCacheCollector(instanceID string, v version.PGVersion) *CacheCollector {
	return &CacheCollector{
		Base: newBase(instanceID, v, 60*time.Second),
	}
}

// Name returns the collector's identifier.
func (c *CacheCollector) Name() string { return "cache" }

// Collect executes the cache hit ratio query and returns metric points.
// Emits: cache.hit_ratio_pct.
func (c *CacheCollector) Collect(ctx context.Context, conn *pgx.Conn) ([]MetricPoint, error) {
	tctx, cancel := queryContext(ctx)
	var hitRatio float64
	err := conn.QueryRow(tctx, sqlCacheHitRatio).Scan(&hitRatio)
	cancel()
	if err != nil {
		return nil, fmt.Errorf("cache collect hit_ratio: %w", err)
	}
	return []MetricPoint{c.point("cache.hit_ratio_pct", hitRatio, nil)}, nil
}
