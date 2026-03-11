package orchestrator

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ios9000/PGPulse_01/internal/cluster/etcd"
	"github.com/ios9000/PGPulse_01/internal/cluster/patroni"
	"github.com/ios9000/PGPulse_01/internal/collector"
	"github.com/ios9000/PGPulse_01/internal/config"
	"github.com/ios9000/PGPulse_01/internal/version"
)

// instanceRunner manages a single monitored PostgreSQL instance:
// a connection pool, version detection, and a set of interval groups.
type instanceRunner struct {
	cfg             config.InstanceConfig
	globalOSMethod  string // global os_metrics.method from config
	pool            *pgxpool.Pool
	pgVersion       version.PGVersion
	store           collector.MetricStore
	groups          []*intervalGroup
	dbRunner        *DBRunner
	logger          *slog.Logger
	evaluator       AlertEvaluator
	dispatcher      AlertDispatcher
}

// connect opens a connection pool to the instance, detects the PG version, and stores both.
func (r *instanceRunner) connect(ctx context.Context) error {
	poolCfg, err := pgxpool.ParseConfig(r.cfg.DSN)
	if err != nil {
		return fmt.Errorf("parse DSN for instance %q: %w", r.cfg.ID, err)
	}
	poolCfg.MinConns = 1
	poolCfg.MaxConns = int32(r.cfg.MaxConns)
	poolCfg.ConnConfig.ConnectTimeout = 5 * time.Second
	if poolCfg.ConnConfig.RuntimeParams == nil {
		poolCfg.ConnConfig.RuntimeParams = make(map[string]string)
	}
	poolCfg.ConnConfig.RuntimeParams["application_name"] = "pgpulse_collector"

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return fmt.Errorf("connect to instance %q: %w", r.cfg.ID, err)
	}
	r.pool = pool

	// Detect version using an acquired connection.
	conn, err := pool.Acquire(ctx)
	if err != nil {
		pool.Close()
		return fmt.Errorf("acquire conn for version detect on %q: %w", r.cfg.ID, err)
	}
	defer conn.Release()

	v, err := version.Detect(ctx, conn.Conn())
	if err != nil {
		pool.Close()
		return fmt.Errorf("detect PG version for instance %q: %w", r.cfg.ID, err)
	}
	r.pgVersion = v

	r.logger.Info("connected to instance", "id", r.cfg.ID, "pg_version", v.Full)
	return nil
}

// buildCollectors instantiates all collectors and groups them by frequency tier.
// Constructor names are the actual exported functions from internal/collector/.
func (r *instanceRunner) buildCollectors() {
	id := r.cfg.ID
	v := r.pgVersion

	high := []collector.Collector{
		collector.NewConnectionsCollector(id, v),
		collector.NewCacheCollector(id, v),
		collector.NewWaitEventsCollector(id, v),
		collector.NewLockTreeCollector(id, v),
		collector.NewLongTransactionsCollector(id, v),
	}

	medium := []collector.Collector{
		collector.NewReplicationStatusCollector(id, v),
		collector.NewReplicationLagCollector(id, v),
		collector.NewReplicationSlotsCollector(id, v),
		collector.NewStatementsConfigCollector(id, v),
		collector.NewStatementsTopCollector(id, v, 0), // 0 → default top-20
		collector.NewCheckpointCollector(id, v),
		collector.NewVacuumProgressCollector(id, v),
		collector.NewClusterProgressCollector(id, v),
		collector.NewAnalyzeProgressCollector(id, v),
		collector.NewCreateIndexProgressCollector(id, v),
		collector.NewBasebackupProgressCollector(id, v),
		collector.NewCopyProgressCollector(id, v),
	}

	// OS metrics: resolve method (per-instance override > global > default "sql").
	osMethod := r.resolveOSMethod()

	// OS agent collector (M6 — graceful degradation if not configured).
	// Registered when method is "agent" or when an agent_url is explicitly set.
	osCollector := collector.NewOSCollector(id, r.cfg.DSN, r.cfg.AgentURL)

	// OS SQL collector (M8_11 — reads /proc via pg_read_file).
	if osMethod == "sql" {
		medium = append(medium, collector.NewOSSQLCollector(id, v))
	}

	clusterCollector := collector.NewClusterCollector(id,
		patroni.NewProvider(patroni.PatroniConfig{
			PatroniURL:     r.cfg.PatroniURL,
			PatroniConfig:  r.cfg.PatroniConfig,
			PatroniCtlPath: r.cfg.PatroniCtlPath,
		}),
		etcd.NewProvider(etcd.ETCDConfig{
			Endpoints: r.cfg.ETCDEndpoints,
			CtlPath:   r.cfg.ETCDCtlPath,
		}),
		r.logger,
	)

	high = append(high, osCollector)

	medium = append(medium, clusterCollector)

	low := []collector.Collector{
		collector.NewServerInfoCollector(id, v),
		collector.NewDatabaseSizesCollector(id, v),
		collector.NewSettingsCollector(id, v),
		collector.NewExtensionsCollector(id, v),
		collector.NewTransactionsCollector(id, v),
		collector.NewIOStatsCollector(id, v),
	}

	r.groups = []*intervalGroup{
		newIntervalGroup("high", r.cfg.Intervals.High, high, r.pool, r.store, r.logger, r.evaluator, r.dispatcher),
		newIntervalGroup("medium", r.cfg.Intervals.Medium, medium, r.pool, r.store, r.logger, r.evaluator, r.dispatcher),
		newIntervalGroup("low", r.cfg.Intervals.Low, low, r.pool, r.store, r.logger, r.evaluator, r.dispatcher),
	}

	// Per-database analysis (M7): create a DBRunner with all registered DBCollectors.
	r.dbRunner = NewDBRunner(
		r.cfg.ID, r.cfg.DSN, r.cfg, r.pool,
		[]collector.DBCollector{collector.NewDatabaseCollector()},
		r.store, r.evaluator, r.logger,
	)
}

// start launches one goroutine per interval group and one for the per-DB runner.
func (r *instanceRunner) start(ctx context.Context, wg *sync.WaitGroup) {
	for _, g := range r.groups {
		wg.Add(1)
		go g.run(ctx, wg)
	}

	// Per-database analysis goroutine (M7).
	if r.dbRunner != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			dbTicker := time.NewTicker(5 * time.Minute)
			defer dbTicker.Stop()

			// Run immediately on start.
			ic := r.queryIC(ctx)
			r.dbRunner.Run(ctx, ic)

			for {
				select {
				case <-ctx.Done():
					return
				case <-dbTicker.C:
					ic = r.queryIC(ctx)
					r.dbRunner.Run(ctx, ic)
				}
			}
		}()
	}
}

// queryIC acquires a connection and queries InstanceContext for the per-DB runner.
func (r *instanceRunner) queryIC(ctx context.Context) collector.InstanceContext {
	conn, err := r.pool.Acquire(ctx)
	if err != nil {
		r.logger.Warn("failed to acquire conn for db runner IC query", "err", err)
		return collector.InstanceContext{}
	}
	defer conn.Release()

	ic, err := queryInstanceContext(ctx, conn.Conn())
	if err != nil {
		r.logger.Warn("failed to query IC for db runner", "err", err)
		return collector.InstanceContext{}
	}
	return ic
}

// resolveOSMethod returns the effective OS metrics collection method.
// Per-instance override takes precedence over global config; defaults to "sql".
func (r *instanceRunner) resolveOSMethod() string {
	if r.cfg.OSMetricsMethod != "" {
		return r.cfg.OSMetricsMethod
	}
	if r.globalOSMethod != "" {
		return r.globalOSMethod
	}
	return "sql"
}

// close releases the connection pool and per-DB pools.
func (r *instanceRunner) close() {
	if r.dbRunner != nil {
		r.dbRunner.Close()
	}
	if r.pool != nil {
		r.pool.Close()
	}
}
