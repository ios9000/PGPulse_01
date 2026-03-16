package remediation

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// InstanceLister returns the list of enabled instance IDs.
type InstanceLister interface {
	ListInstanceIDs(ctx context.Context) ([]string, error)
}

// BackgroundEvaluator periodically runs all remediation rules against each
// monitored instance and persists results. Stale recommendations that no
// longer fire are automatically resolved.
type BackgroundEvaluator struct {
	engine       *Engine
	store        *PGStore
	metricSource MetricSource
	lister       InstanceLister
	interval     time.Duration
	retention    time.Duration
	logger       *slog.Logger
	cancel       context.CancelFunc
}

// NewBackgroundEvaluator creates a BackgroundEvaluator.
func NewBackgroundEvaluator(
	engine *Engine,
	store *PGStore,
	metricSource MetricSource,
	lister InstanceLister,
	interval time.Duration,
	retentionDays int,
	logger *slog.Logger,
) *BackgroundEvaluator {
	return &BackgroundEvaluator{
		engine:       engine,
		store:        store,
		metricSource: metricSource,
		lister:       lister,
		interval:     interval,
		retention:    time.Duration(retentionDays) * 24 * time.Hour,
		logger:       logger,
	}
}

// Start launches the background evaluation loop in a goroutine.
func (b *BackgroundEvaluator) Start(ctx context.Context) {
	ctx, b.cancel = context.WithCancel(ctx)
	go b.run(ctx)
}

// Stop cancels the background evaluation loop.
func (b *BackgroundEvaluator) Stop() {
	if b.cancel != nil {
		b.cancel()
	}
}

func (b *BackgroundEvaluator) run(ctx context.Context) {
	// Run once immediately on start.
	b.runCycle(ctx)

	ticker := time.NewTicker(b.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			b.runCycle(ctx)
		}
	}
}

func (b *BackgroundEvaluator) runCycle(ctx context.Context) {
	start := time.Now()

	instanceIDs, err := b.lister.ListInstanceIDs(ctx)
	if err != nil {
		b.logger.Error("background eval: failed to list instances", "error", err)
		return
	}

	totalRecs := 0
	for _, instanceID := range instanceIDs {
		if ctx.Err() != nil {
			return
		}

		recs, err := b.evaluateInstance(ctx, instanceID)
		if err != nil {
			b.logger.Warn("background eval: instance evaluation failed",
				"instance", instanceID, "error", err)
			continue
		}
		totalRecs += recs
	}

	// Retention cleanup at end of cycle.
	if err := b.store.CleanOld(ctx, b.retention); err != nil {
		b.logger.Warn("background eval: retention cleanup failed", "error", err)
	}

	b.logger.Info("background eval cycle complete",
		"instances", len(instanceIDs),
		"recommendations", totalRecs,
		"duration", time.Since(start).Round(time.Millisecond),
	)
}

func (b *BackgroundEvaluator) evaluateInstance(ctx context.Context, instanceID string) (int, error) {
	snapshot, err := b.metricSource.CurrentSnapshot(ctx, instanceID)
	if err != nil {
		return 0, fmt.Errorf("get snapshot for %s: %w", instanceID, err)
	}

	recs := b.engine.Diagnose(ctx, instanceID, snapshot)

	// Collect current rule IDs that fired, for stale resolution.
	currentRuleIDs := make([]string, len(recs))
	for i, r := range recs {
		currentRuleIDs[i] = r.RuleID
	}

	// Resolve stale recommendations that no longer fire.
	if err := b.store.ResolveStale(ctx, instanceID, currentRuleIDs); err != nil {
		b.logger.Warn("background eval: failed to resolve stale",
			"instance", instanceID, "error", err)
	}

	// Write new/update existing recommendations.
	if len(recs) > 0 {
		written, err := b.store.WriteOrUpdate(ctx, recs)
		if err != nil {
			return 0, fmt.Errorf("write recommendations for %s: %w", instanceID, err)
		}
		return written, nil
	}

	return 0, nil
}
