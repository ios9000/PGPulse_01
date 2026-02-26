package orchestrator

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/ios9000/PGPulse_01/internal/collector"
	"github.com/ios9000/PGPulse_01/internal/config"
	"github.com/ios9000/PGPulse_01/internal/version"
)

// instanceRunner manages a single monitored PostgreSQL instance:
// one persistent connection, version detection, and a set of interval groups.
type instanceRunner struct {
	cfg       config.InstanceConfig
	conn      *pgx.Conn
	pgVersion version.PGVersion
	store     collector.MetricStore
	groups    []*intervalGroup
	logger    *slog.Logger
}

// connect opens a connection to the instance, detects the PG version, and stores both.
func (r *instanceRunner) connect(ctx context.Context) error {
	connConfig, err := pgx.ParseConfig(r.cfg.DSN)
	if err != nil {
		return fmt.Errorf("parse DSN for instance %q: %w", r.cfg.ID, err)
	}
	connConfig.ConnectTimeout = 5 * time.Second
	if connConfig.RuntimeParams == nil {
		connConfig.RuntimeParams = make(map[string]string)
	}
	connConfig.RuntimeParams["application_name"] = "pgpulse_orchestrator"

	conn, err := pgx.ConnectConfig(ctx, connConfig)
	if err != nil {
		return fmt.Errorf("connect to instance %q: %w", r.cfg.ID, err)
	}
	r.conn = conn

	v, err := version.Detect(ctx, r.conn)
	if err != nil {
		_ = r.conn.Close(context.Background())
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

	low := []collector.Collector{
		collector.NewServerInfoCollector(id, v),
		collector.NewDatabaseSizesCollector(id, v),
		collector.NewSettingsCollector(id, v),
		collector.NewExtensionsCollector(id, v),
		collector.NewTransactionsCollector(id, v),
		collector.NewIOStatsCollector(id, v),
	}

	r.groups = []*intervalGroup{
		newIntervalGroup("high", r.cfg.Intervals.High, high, r.conn, r.store, r.logger),
		newIntervalGroup("medium", r.cfg.Intervals.Medium, medium, r.conn, r.store, r.logger),
		newIntervalGroup("low", r.cfg.Intervals.Low, low, r.conn, r.store, r.logger),
	}
}

// start launches one goroutine per interval group.
func (r *instanceRunner) start(ctx context.Context, wg *sync.WaitGroup) {
	for _, g := range r.groups {
		wg.Add(1)
		go g.run(ctx, wg)
	}
}

// close releases the database connection.
func (r *instanceRunner) close() {
	if r.conn != nil {
		_ = r.conn.Close(context.Background())
	}
}
