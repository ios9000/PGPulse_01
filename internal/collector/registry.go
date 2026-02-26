package collector

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
)

// Registry manages a set of collectors and executes them in batch.
type Registry struct {
	collectors []Collector
}

// NewRegistry creates an empty collector registry.
func NewRegistry() *Registry {
	return &Registry{}
}

// Register adds a collector to the registry.
func (r *Registry) Register(c Collector) {
	r.collectors = append(r.collectors, c)
}

// CollectAll runs every registered collector sequentially and returns all metric points.
// Individual collector failures are logged but do not abort the batch — the registry
// continues to the next collector and returns all successfully collected points.
// ic provides per-cycle instance state (e.g., recovery role) queried once by the caller.
func (r *Registry) CollectAll(ctx context.Context, conn *pgx.Conn, ic InstanceContext) []MetricPoint {
	var results []MetricPoint

	for _, c := range r.collectors {
		tctx, cancel := queryContext(ctx)
		start := time.Now()
		points, err := c.Collect(tctx, conn, ic)
		cancel()
		duration := time.Since(start)

		if err != nil {
			slog.Error("collector failed",
				"collector", c.Name(),
				"error", err,
				"duration", duration,
			)
			continue
		}

		slog.Debug("collector completed",
			"collector", c.Name(),
			"points", len(points),
			"duration", duration,
		)
		results = append(results, points...)
	}

	return results
}
