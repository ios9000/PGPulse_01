package orchestrator

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/ios9000/PGPulse_01/internal/collector"
	"github.com/ios9000/PGPulse_01/internal/config"
)

// Orchestrator manages collection across all configured PostgreSQL instances.
// It creates one instanceRunner per enabled instance, each with three interval groups.
type Orchestrator struct {
	cfg     config.Config
	store   collector.MetricStore
	runners []*instanceRunner
	wg      sync.WaitGroup
	cancel  context.CancelFunc
	logger  *slog.Logger
}

// New creates an Orchestrator. Call Start to begin collection.
func New(cfg config.Config, store collector.MetricStore, logger *slog.Logger) *Orchestrator {
	return &Orchestrator{
		cfg:    cfg,
		store:  store,
		logger: logger,
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
			cfg:    instCfg,
			store:  o.store,
			logger: o.logger,
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
