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
)

// Orchestrator manages collection across all configured PostgreSQL instances.
// It creates one instanceRunner per enabled instance, each with three interval groups.
type Orchestrator struct {
	cfg        config.Config
	store      collector.MetricStore
	runners    []*instanceRunner
	wg         sync.WaitGroup
	cancel     context.CancelFunc
	logger     *slog.Logger
	evaluator  AlertEvaluator
	dispatcher AlertDispatcher
}

// New creates an Orchestrator. Call Start to begin collection.
func New(cfg config.Config, store collector.MetricStore, logger *slog.Logger,
	evaluator AlertEvaluator, dispatcher AlertDispatcher) *Orchestrator {
	return &Orchestrator{
		cfg:        cfg,
		store:      store,
		logger:     logger,
		evaluator:  evaluator,
		dispatcher: dispatcher,
	}
}

// Start connects to all enabled instances and launches collection goroutines.
// Returns an error if no instances could be connected.
func (o *Orchestrator) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	o.cancel = cancel

	for _, instCfg := range o.cfg.Instances {
		if instCfg.Enabled != nil && !*instCfg.Enabled {
			o.logger.Info("instance disabled, skipping", "id", instCfg.ID)
			continue
		}

		r := &instanceRunner{
			cfg:        instCfg,
			store:      o.store,
			logger:     o.logger,
			evaluator:  o.evaluator,
			dispatcher: o.dispatcher,
		}

		if err := r.connect(ctx); err != nil {
			o.logger.Warn("failed to connect to instance, skipping",
				"id", instCfg.ID, "err", err)
			continue
		}

		r.buildCollectors()
		r.start(ctx, &o.wg)
		o.runners = append(o.runners, r)
	}

	if len(o.runners) == 0 {
		cancel()
		return fmt.Errorf("orchestrator: no instances connected")
	}

	o.logger.Info("orchestrator started", "instance_count", len(o.runners))
	return nil
}

// ConnFor opens a fresh connection to the monitored instance identified by instanceID.
// The caller is responsible for closing the returned connection.
func (o *Orchestrator) ConnFor(ctx context.Context, instanceID string) (*pgx.Conn, error) {
	for _, r := range o.runners {
		if r.cfg.ID == instanceID {
			connConfig, err := pgx.ParseConfig(r.cfg.DSN)
			if err != nil {
				return nil, fmt.Errorf("parse DSN for instance %q: %w", instanceID, err)
			}
			connConfig.ConnectTimeout = 5 * time.Second
			if connConfig.RuntimeParams == nil {
				connConfig.RuntimeParams = make(map[string]string)
			}
			connConfig.RuntimeParams["application_name"] = "pgpulse_api"

			conn, err := pgx.ConnectConfig(ctx, connConfig)
			if err != nil {
				return nil, fmt.Errorf("connect to instance %q: %w", instanceID, err)
			}
			return conn, nil
		}
	}

	// Also check config for instances that failed to connect at startup.
	for _, instCfg := range o.cfg.Instances {
		if instCfg.ID == instanceID {
			if instCfg.Enabled != nil && !*instCfg.Enabled {
				return nil, fmt.Errorf("instance %q is disabled", instanceID)
			}
			connConfig, err := pgx.ParseConfig(instCfg.DSN)
			if err != nil {
				return nil, fmt.Errorf("parse DSN for instance %q: %w", instanceID, err)
			}
			connConfig.ConnectTimeout = 5 * time.Second
			if connConfig.RuntimeParams == nil {
				connConfig.RuntimeParams = make(map[string]string)
			}
			connConfig.RuntimeParams["application_name"] = "pgpulse_api"

			conn, err := pgx.ConnectConfig(ctx, connConfig)
			if err != nil {
				return nil, fmt.Errorf("connect to instance %q: %w", instanceID, err)
			}
			return conn, nil
		}
	}

	return nil, fmt.Errorf("instance %q not found", instanceID)
}

// Stop signals all goroutines to stop, waits for them, then closes all connections.
func (o *Orchestrator) Stop() {
	if o.cancel != nil {
		o.cancel()
	}
	o.wg.Wait()
	for _, r := range o.runners {
		r.close()
	}
	o.logger.Info("orchestrator stopped")
}
