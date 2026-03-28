package forecast

import (
	"context"
	"log/slog"
	"time"

	"github.com/ios9000/PGPulse_01/internal/collector"
	"github.com/ios9000/PGPulse_01/internal/config"
)

// ForecastEngine is the top-level coordinator. It owns the OperationTracker,
// NeedEvaluator, and ETACalculator, manages their lifecycle, and provides
// the API surface for handlers.
type ForecastEngine struct {
	Tracker          *OperationTracker
	Evaluator        *NeedEvaluator
	ETA              *ETACalculator
	Store            ForecastStore
	thresholdQuerier *PGThresholdQuerier
	cfg              config.MaintenanceForecastConfig
	logger           *slog.Logger
}

// NewForecastEngine creates the engine and all sub-components.
func NewForecastEngine(
	metricStore collector.MetricStore,
	forecastStore ForecastStore,
	baselineProvider BaselineProvider,
	instanceLister InstanceLister,
	connProv InstanceConnProvider,
	cfg config.MaintenanceForecastConfig,
	logger *slog.Logger,
) *ForecastEngine {
	tracker := NewOperationTracker(metricStore, forecastStore, connProv, 15*time.Second, cfg.ETAWindowSize, logger)
	thresholdQuerier := NewPGThresholdQuerier(connProv, cfg, logger)
	evaluator := NewNeedEvaluator(metricStore, forecastStore, baselineProvider, instanceLister, thresholdQuerier, cfg, logger)
	eta := NewETACalculator(tracker, ETAConfig{
		WindowSize:  cfg.ETAWindowSize,
		DecayFactor: cfg.ETADecayFactor,
		MinSamples:  cfg.ETAMinSamples,
	}, logger)

	return &ForecastEngine{
		Tracker:          tracker,
		Evaluator:        evaluator,
		ETA:              eta,
		Store:            forecastStore,
		thresholdQuerier: thresholdQuerier,
		cfg:              cfg,
		logger:           logger,
	}
}

// SetConnProvider sets the connection provider on sub-components.
// Called from main.go after the orchestrator is created.
func (e *ForecastEngine) SetConnProvider(cp InstanceConnProvider) {
	e.Tracker.SetConnProvider(cp)
	e.thresholdQuerier.SetConnProvider(cp)
}

// Start launches background goroutines. Called from main.go.
func (e *ForecastEngine) Start(ctx context.Context) {
	go e.Tracker.Run(ctx)
	go e.Evaluator.Run(ctx)
	go e.retentionCleanup(ctx)
}

// retentionCleanup runs daily, deletes operations older than retention_days.
func (e *ForecastEngine) retentionCleanup(ctx context.Context) {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cutoff := time.Now().AddDate(0, 0, -e.cfg.RetentionDays)
			deleted, err := e.Store.CleanOldOperations(ctx, cutoff)
			if err != nil {
				e.logger.Error("forecast: retention cleanup failed", "err", err)
			} else if deleted > 0 {
				e.logger.Info("forecast: retention cleanup", "deleted", deleted)
			}
		}
	}
}
