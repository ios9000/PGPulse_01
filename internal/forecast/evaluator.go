package forecast

import (
	"context"
	"log/slog"
	"time"

	"github.com/ios9000/PGPulse_01/internal/collector"
	"github.com/ios9000/PGPulse_01/internal/config"
	"github.com/ios9000/PGPulse_01/internal/ml"
)

// NeedEvaluator is a background goroutine that computes maintenance need forecasts.
type NeedEvaluator struct {
	metricStore      collector.MetricStore
	forecastStore    ForecastStore
	baselineProvider BaselineProvider
	instanceLister   InstanceLister
	thresholdQuerier ThresholdQuerier
	cfg              config.MaintenanceForecastConfig
	logger           *slog.Logger
}

// NewNeedEvaluator creates a need evaluator.
func NewNeedEvaluator(
	metricStore collector.MetricStore,
	forecastStore ForecastStore,
	baselineProvider BaselineProvider,
	instanceLister InstanceLister,
	thresholdQuerier ThresholdQuerier,
	cfg config.MaintenanceForecastConfig,
	logger *slog.Logger,
) *NeedEvaluator {
	return &NeedEvaluator{
		metricStore:      metricStore,
		forecastStore:    forecastStore,
		baselineProvider: baselineProvider,
		instanceLister:   instanceLister,
		thresholdQuerier: thresholdQuerier,
		cfg:              cfg,
		logger:           logger,
	}
}

// Run is the main loop. Runs immediately on startup, then on 5-minute ticker.
func (e *NeedEvaluator) Run(ctx context.Context) {
	e.evaluate(ctx) // Run immediately on startup.

	ticker := time.NewTicker(e.cfg.EvaluationInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.evaluate(ctx)
		}
	}
}

// evaluate runs one full evaluation cycle across all active instances.
func (e *NeedEvaluator) evaluate(ctx context.Context) {
	instances, err := e.instanceLister.ActiveInstanceIDs(ctx)
	if err != nil {
		e.logger.Error("forecast: evaluator list instances", "err", err)
		return
	}

	for _, instanceID := range instances {
		if ctx.Err() != nil {
			return
		}
		e.evaluateInstance(ctx, instanceID)
	}
}

// evaluateInstance runs forecasts for a single instance.
func (e *NeedEvaluator) evaluateInstance(ctx context.Context, instanceID string) {
	now := time.Now()

	// Step 1: Get table thresholds (cached per cycle).
	thresholds, err := e.thresholdQuerier.GetTableThresholds(ctx, instanceID)
	if err != nil {
		e.logger.Warn("forecast: threshold query failed", "instance", instanceID, "err", err)
		return
	}

	var forecasts []MaintenanceForecast

	// Step 2-3: Evaluate vacuum and analyze needs for all tables.
	for _, t := range thresholds {
		vacForecast := e.evaluateVacuum(ctx, instanceID, t, now)
		forecasts = append(forecasts, vacForecast)

		anForecast := e.evaluateAnalyze(ctx, instanceID, t, now)
		forecasts = append(forecasts, anForecast)
	}

	// Step 4: Evaluate reindex needs (from bloat metrics).
	reindexForecasts := e.evaluateReindexBatch(ctx, instanceID, now)
	forecasts = append(forecasts, reindexForecasts...)

	// Step 5: Evaluate basebackup (C4: disabled when interval=0).
	if e.cfg.BasebackupInterval > 0 {
		bbForecast := e.evaluateBasebackup(ctx, instanceID, now)
		forecasts = append(forecasts, bbForecast)
	}

	// Step 6: Batch UPSERT.
	if len(forecasts) > 0 {
		if err := e.forecastStore.UpsertForecasts(ctx, forecasts); err != nil {
			e.logger.Error("forecast: batch upsert failed", "instance", instanceID, "err", err)
		}
	}

	e.logger.Debug("forecast: evaluation complete",
		"instance", instanceID,
		"tables", len(thresholds),
		"forecasts", len(forecasts))
}

// evaluateVacuum computes vacuum forecast for a single table.
func (e *NeedEvaluator) evaluateVacuum(ctx context.Context, instanceID string, t TableThresholds, now time.Time) MaintenanceForecast {
	base := MaintenanceForecast{
		InstanceID:     instanceID,
		Database:       t.Database,
		Table:          t.Table,
		Operation:      "vacuum",
		ThresholdValue: t.EffectiveVacuumLimit,
		Method:         "threshold_projection",
		EvaluatedAt:    now,
	}

	// Query dead_tuples history.
	metricKey := "pg.db.vacuum.dead_tuples"
	points := e.queryMetricHistory(ctx, instanceID, metricKey, t.Database, t.Table)

	if len(points) < e.cfg.MinDataPoints {
		base.Status = "insufficient_data"
		return base
	}

	currentValue := points[len(points)-1].Value
	base.CurrentValue = currentValue

	// Check overdue.
	if currentValue >= t.EffectiveVacuumLimit {
		base.Status = "overdue"
		base.TimeUntilSec = 0
		return base
	}

	// Compute accumulation rate.
	rate := e.computeRate(points)
	base.AccumulationRate = rate

	if rate <= 0 {
		base.Status = "not_needed"
		return base
	}

	// Project threshold crossing time.
	remaining := t.EffectiveVacuumLimit - currentValue
	timeToThreshold := remaining / rate
	base.TimeUntilSec = timeToThreshold

	predictedAt := now.Add(time.Duration(timeToThreshold * float64(time.Second)))
	base.PredictedAt = &predictedAt

	if timeToThreshold <= 3600 {
		base.Status = "imminent"
	} else {
		base.Status = "predicted"
	}

	return base
}

// evaluateAnalyze computes analyze forecast for a single table.
func (e *NeedEvaluator) evaluateAnalyze(ctx context.Context, instanceID string, t TableThresholds, now time.Time) MaintenanceForecast {
	base := MaintenanceForecast{
		InstanceID:     instanceID,
		Database:       t.Database,
		Table:          t.Table,
		Operation:      "analyze",
		ThresholdValue: t.EffectiveAnalyzeLimit,
		Method:         "threshold_projection",
		EvaluatedAt:    now,
	}

	metricKey := "pg.db.vacuum.mod_since_analyze"
	points := e.queryMetricHistory(ctx, instanceID, metricKey, t.Database, t.Table)

	if len(points) < e.cfg.MinDataPoints {
		base.Status = "insufficient_data"
		return base
	}

	currentValue := points[len(points)-1].Value
	base.CurrentValue = currentValue

	if currentValue >= t.EffectiveAnalyzeLimit {
		base.Status = "overdue"
		base.TimeUntilSec = 0
		return base
	}

	rate := e.computeRate(points)
	base.AccumulationRate = rate

	if rate <= 0 {
		base.Status = "not_needed"
		return base
	}

	remaining := t.EffectiveAnalyzeLimit - currentValue
	timeToThreshold := remaining / rate
	base.TimeUntilSec = timeToThreshold

	predictedAt := now.Add(time.Duration(timeToThreshold * float64(time.Second)))
	base.PredictedAt = &predictedAt

	if timeToThreshold <= 3600 {
		base.Status = "imminent"
	} else {
		base.Status = "predicted"
	}

	return base
}

// evaluateReindexBatch evaluates reindex needs from bloat metrics.
func (e *NeedEvaluator) evaluateReindexBatch(ctx context.Context, instanceID string, now time.Time) []MaintenanceForecast {
	// Query table bloat ratios.
	points, err := e.metricStore.Query(ctx, collector.MetricQuery{
		InstanceID: instanceID,
		Metric:     "pg.db.bloat.table_ratio",
		Start:      now.Add(-e.cfg.LookbackWindow),
		End:        now,
	})
	if err != nil {
		return nil
	}

	// Group by database:table, take latest value.
	type tableKey struct{ db, table string }
	latest := make(map[tableKey]float64)
	for _, p := range points {
		k := tableKey{db: p.Labels["datname"], table: p.Labels["relname"]}
		latest[k] = p.Value
	}

	var forecasts []MaintenanceForecast
	for k, ratio := range latest {
		f := MaintenanceForecast{
			InstanceID:     instanceID,
			Database:       k.db,
			Table:          k.table,
			Operation:      "reindex",
			ThresholdValue: e.cfg.ReindexBloatThresholdTable,
			CurrentValue:   ratio,
			Method:         "threshold_projection",
			EvaluatedAt:    now,
		}

		if ratio >= e.cfg.ReindexBloatThresholdTable {
			f.Status = "overdue"
		} else if ratio >= e.cfg.ReindexBloatThresholdTable*0.8 {
			f.Status = "imminent"
		} else {
			f.Status = "not_needed"
		}
		forecasts = append(forecasts, f)
	}

	return forecasts
}

// evaluateBasebackup evaluates basebackup need (C4: disabled when interval=0).
func (e *NeedEvaluator) evaluateBasebackup(_ context.Context, instanceID string, now time.Time) MaintenanceForecast {
	// C4: No pg.wal.bytes_rate metric exists. Return insufficient_data.
	return MaintenanceForecast{
		InstanceID:  instanceID,
		Database:    "", // C5: basebackup has no datname
		Table:       "", // C5: basebackup has no relname
		Operation:   "basebackup",
		Status:      "insufficient_data",
		Method:      "threshold_projection",
		EvaluatedAt: now,
	}
}

// queryMetricHistory fetches metric data for a specific table over the lookback window.
func (e *NeedEvaluator) queryMetricHistory(ctx context.Context, instanceID, metricKey, database, table string) []collector.MetricPoint {
	now := time.Now()
	points, err := e.metricStore.Query(ctx, collector.MetricQuery{
		InstanceID: instanceID,
		Metric:     metricKey,
		Labels:     map[string]string{"datname": database, "relname": table},
		Start:      now.Add(-e.cfg.LookbackWindow),
		End:        now,
	})
	if err != nil {
		e.logger.Debug("forecast: metric query failed",
			"metric", metricKey, "database", database, "table", table, "err", err)
		return nil
	}
	return points
}

// computeRate uses linear regression on metric points to compute accumulation rate.
func (e *NeedEvaluator) computeRate(points []collector.MetricPoint) float64 {
	if len(points) < 2 {
		return 0
	}

	xs := make([]float64, len(points))
	ys := make([]float64, len(points))
	base := points[0].Timestamp.Unix()
	for i, p := range points {
		xs[i] = float64(p.Timestamp.Unix() - base)
		ys[i] = p.Value
	}

	result, err := ml.LinearRegression(xs, ys)
	if err != nil {
		return 0
	}
	return result.Slope // rate in units per second
}
