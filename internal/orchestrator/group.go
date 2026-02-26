package orchestrator

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/ios9000/PGPulse_01/internal/collector"
)

// icQueryFunc is the signature of the function that queries InstanceContext.
// It is a field on intervalGroup so tests can inject a mock without a real DB.
type icQueryFunc func(ctx context.Context, conn *pgx.Conn) (collector.InstanceContext, error)

// intervalGroup runs a set of collectors on a shared ticker and writes results to a store.
type intervalGroup struct {
	name       string
	interval   time.Duration
	collectors []collector.Collector
	conn       *pgx.Conn
	store      collector.MetricStore
	logger     *slog.Logger
	icFunc     icQueryFunc // defaults to queryInstanceContext; injectable for tests
}

func newIntervalGroup(
	name string,
	interval time.Duration,
	collectors []collector.Collector,
	conn *pgx.Conn,
	store collector.MetricStore,
	logger *slog.Logger,
) *intervalGroup {
	return &intervalGroup{
		name:       name,
		interval:   interval,
		collectors: collectors,
		conn:       conn,
		store:      store,
		logger:     logger,
		icFunc:     queryInstanceContext,
	}
}

// run executes collect() once immediately, then on every ticker tick until ctx is cancelled.
func (g *intervalGroup) run(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()

	ticker := time.NewTicker(g.interval)
	defer ticker.Stop()

	g.collect(ctx) // first collection immediately

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			g.collect(ctx)
		}
	}
}

// collect queries InstanceContext once, then runs all collectors, batching results for a single Write.
func (g *intervalGroup) collect(ctx context.Context) {
	ic, err := g.icFunc(ctx, g.conn)
	if err != nil {
		g.logger.Warn("failed to query instance context, skipping cycle",
			"group", g.name, "err", err)
		return
	}

	var batch []collector.MetricPoint
	for _, c := range g.collectors {
		points, err := c.Collect(ctx, g.conn, ic)
		if err != nil {
			g.logger.Warn("collector error",
				"group", g.name, "collector", c.Name(), "err", err)
			continue
		}
		batch = append(batch, points...)
	}

	if len(batch) == 0 {
		return
	}

	if err := g.store.Write(ctx, batch); err != nil {
		g.logger.Error("failed to write metrics",
			"group", g.name, "points", len(batch), "err", err)
		return
	}
	g.logger.Debug("metrics written", "group", g.name, "points", len(batch))
}

// queryInstanceContext queries pg_is_in_recovery() with a 5-second timeout.
// It is the default icFunc used in production.
func queryInstanceContext(ctx context.Context, conn *pgx.Conn) (collector.InstanceContext, error) {
	qCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var isRecovery bool
	if err := conn.QueryRow(qCtx, "SELECT pg_is_in_recovery()").Scan(&isRecovery); err != nil {
		return collector.InstanceContext{}, fmt.Errorf("query instance context: %w", err)
	}
	return collector.InstanceContext{IsRecovery: isRecovery}, nil
}
