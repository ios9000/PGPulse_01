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

// CacheCollector reports the instance-wide shared buffer cache hit ratio.
//
//   - Tier: postgres
//   - Question: Is the working set still fitting in shared_buffers, or are reads going to disk?
//   - Views: pg_stat_database
//   - Collection: 10s (high group)
//   - PG versions: 14+ (no version gate)
//
// NULLIF + COALESCE guard against division by zero on a fresh instance with no block reads yet.
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
// Emits: cache.hit_ratio.
func (c *CacheCollector) Collect(ctx context.Context, conn *pgx.Conn, _ InstanceContext) ([]MetricPoint, error) {
	tctx, cancel := queryContext(ctx)
	var hitRatio float64
	err := conn.QueryRow(tctx, sqlCacheHitRatio).Scan(&hitRatio)
	cancel()
	if err != nil {
		return nil, fmt.Errorf("cache collect hit_ratio: %w", err)
	}
	return []MetricPoint{c.point("cache.hit_ratio", hitRatio, nil)}, nil
}
