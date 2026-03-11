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
	"github.com/ios9000/PGPulse_01/internal/storage"
)

// Orchestrator manages collection across all configured PostgreSQL instances.
// It creates one instanceRunner per enabled instance, each with three interval groups.
type Orchestrator struct {
	cfg           config.Config
	store         collector.MetricStore
	mu            sync.Mutex
	runners       map[string]*instanceRunner
	wg            sync.WaitGroup
	cancel        context.CancelFunc
	logger        *slog.Logger
	evaluator     AlertEvaluator
	dispatcher    AlertDispatcher
	instanceStore storage.InstanceStore // nil when no DB store
}

// New creates an Orchestrator. Call Start to begin collection.
// instanceStore may be nil (no DB-backed instance management).
func New(cfg config.Config, store collector.MetricStore, logger *slog.Logger,
	evaluator AlertEvaluator, dispatcher AlertDispatcher,
	instanceStore ...storage.InstanceStore) *Orchestrator {

	var is storage.InstanceStore
	if len(instanceStore) > 0 {
		is = instanceStore[0]
	}

	return &Orchestrator{
		cfg:           cfg,
		store:         store,
		logger:        logger,
		evaluator:     evaluator,
		dispatcher:    dispatcher,
		runners:       make(map[string]*instanceRunner),
		instanceStore: is,
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
		o.mu.Lock()
		o.runners[instCfg.ID] = r
		o.mu.Unlock()
	}

	o.mu.Lock()
	runnerCount := len(o.runners)
	o.mu.Unlock()

	if runnerCount == 0 {
		cancel()
		return fmt.Errorf("orchestrator: no instances connected")
	}

	// Launch hot-reload loop if we have an instance store.
	if o.instanceStore != nil {
		go o.reloadLoop(ctx)
	}

	o.logger.Info("orchestrator started", "instance_count", runnerCount)
	return nil
}

// reloadLoop polls the instance store every 60 seconds and adjusts runners.
func (o *Orchestrator) reloadLoop(ctx context.Context) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			o.reload(ctx)
		}
	}
}

// reload compares DB instances with running runners and starts/stops as needed.
func (o *Orchestrator) reload(ctx context.Context) {
	records, err := o.instanceStore.List(ctx)
	if err != nil {
		o.logger.Warn("hot-reload: failed to list instances from store", "error", err)
		return
	}

	// Build map of desired instances.
	desired := make(map[string]storage.InstanceRecord, len(records))
	for _, rec := range records {
		if rec.Enabled {
			desired[rec.ID] = rec
		}
	}

	o.mu.Lock()
	defer o.mu.Unlock()

	// Stop runners for instances that are no longer in the desired set.
	for id, r := range o.runners {
		if _, ok := desired[id]; !ok {
			o.logger.Info("hot-reload: stopping removed/disabled instance", "id", id)
			r.close()
			delete(o.runners, id)
		}
	}

	// Start runners for new instances not yet running.
	for id, rec := range desired {
		if _, running := o.runners[id]; running {
			continue
		}

		maxConns := rec.MaxConns
		if maxConns == 0 {
			maxConns = 5
		}

		instCfg := config.InstanceConfig{
			ID:       rec.ID,
			Name:     rec.Name,
			DSN:      rec.DSN,
			MaxConns: maxConns,
		}
		// Set default intervals.
		instCfg.Intervals.High = 10 * time.Second
		instCfg.Intervals.Medium = 60 * time.Second
		instCfg.Intervals.Low = 300 * time.Second

		r := &instanceRunner{
			cfg:        instCfg,
			store:      o.store,
			logger:     o.logger,
			evaluator:  o.evaluator,
			dispatcher: o.dispatcher,
		}

		if err := r.connect(ctx); err != nil {
			o.logger.Warn("hot-reload: failed to connect to new instance",
				"id", id, "error", err)
			continue
		}

		r.buildCollectors()
		r.start(ctx, &o.wg)
		o.runners[id] = r
		o.logger.Info("hot-reload: started new instance", "id", id)
	}
}

// ConnFor opens a fresh connection to the monitored instance identified by instanceID.
// The caller is responsible for closing the returned connection.
func (o *Orchestrator) ConnFor(ctx context.Context, instanceID string) (*pgx.Conn, error) {
	o.mu.Lock()
	r, ok := o.runners[instanceID]
	o.mu.Unlock()

	if ok {
		return o.dialInstance(ctx, instanceID, r.cfg.DSN)
	}

	// Also check config for instances that failed to connect at startup.
	for _, instCfg := range o.cfg.Instances {
		if instCfg.ID == instanceID {
			if instCfg.Enabled != nil && !*instCfg.Enabled {
				return nil, fmt.Errorf("instance %q is disabled", instanceID)
			}
			return o.dialInstance(ctx, instanceID, instCfg.DSN)
		}
	}

	// Check DB store if available.
	if o.instanceStore != nil {
		rec, err := o.instanceStore.Get(ctx, instanceID)
		if err == nil && rec != nil {
			if !rec.Enabled {
				return nil, fmt.Errorf("instance %q is disabled", instanceID)
			}
			return o.dialInstance(ctx, instanceID, rec.DSN)
		}
	}

	return nil, fmt.Errorf("instance %q not found", instanceID)
}

// dialInstance opens a new pgx connection to the given DSN.
func (o *Orchestrator) dialInstance(ctx context.Context, instanceID, dsn string) (*pgx.Conn, error) {
	connConfig, err := pgx.ParseConfig(dsn)
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

// ConnForDB opens a connection to a specific database on the monitored instance.
// The caller is responsible for closing the returned connection.
func (o *Orchestrator) ConnForDB(ctx context.Context, instanceID, dbName string) (*pgx.Conn, error) {
	dsn, err := o.instanceDSN(ctx, instanceID)
	if err != nil {
		return nil, err
	}
	return o.dialInstance(ctx, instanceID, substituteDBName(dsn, dbName))
}

// instanceDSN resolves the DSN for the given instance from runners, config, or DB store.
func (o *Orchestrator) instanceDSN(ctx context.Context, instanceID string) (string, error) {
	o.mu.Lock()
	r, ok := o.runners[instanceID]
	o.mu.Unlock()

	if ok {
		return r.cfg.DSN, nil
	}

	for _, instCfg := range o.cfg.Instances {
		if instCfg.ID == instanceID {
			if instCfg.Enabled != nil && !*instCfg.Enabled {
				return "", fmt.Errorf("instance %q is disabled", instanceID)
			}
			return instCfg.DSN, nil
		}
	}

	if o.instanceStore != nil {
		rec, err := o.instanceStore.Get(ctx, instanceID)
		if err == nil && rec != nil {
			if !rec.Enabled {
				return "", fmt.Errorf("instance %q is disabled", instanceID)
			}
			return rec.DSN, nil
		}
	}

	return "", fmt.Errorf("instance %q not found", instanceID)
}

// Stop signals all goroutines to stop, waits for them, then closes all connections.
func (o *Orchestrator) Stop() {
	if o.cancel != nil {
		o.cancel()
	}
	o.wg.Wait()
	o.mu.Lock()
	for _, r := range o.runners {
		r.close()
	}
	o.mu.Unlock()
	o.logger.Info("orchestrator stopped")
}
