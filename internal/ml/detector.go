package ml

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"sync"
	"time"

	"github.com/ios9000/PGPulse_01/internal/collector"
)

// InstanceLister returns the list of known instance IDs.
// Separated from MetricStore because MetricStore doesn't have this method.
type InstanceLister interface {
	ListInstanceIDs(ctx context.Context) ([]string, error)
}

// AnomalyResult represents a detected anomaly.
type AnomalyResult struct {
	InstanceID string
	Metric     string
	Value      float64
	ZScore     float64
	IsIQR      bool
	IsAnomaly  bool
	Timestamp  time.Time
}

// Detector manages per-instance-metric baselines and evaluates new points for anomalies.
type Detector struct {
	config    DetectorConfig
	baselines map[string]*STLBaseline // "instanceID:metricKey"
	mu        sync.RWMutex
	store     collector.MetricStore
	lister    InstanceLister
	evaluator collector.AlertEvaluator
}

// NewDetector creates an ML anomaly detector.
func NewDetector(cfg DetectorConfig, store collector.MetricStore, lister InstanceLister, evaluator collector.AlertEvaluator) *Detector {
	return &Detector{
		config:    cfg,
		baselines: make(map[string]*STLBaseline),
		store:     store,
		lister:    lister,
		evaluator: evaluator,
	}
}

// SetAlertEvaluator replaces the alert evaluator. Called from main.go
// after the alert pipeline is initialized, upgrading from the initial
// no-op to a real adapter.
func (d *Detector) SetAlertEvaluator(e collector.AlertEvaluator) {
	d.evaluator = e
}

// Bootstrap loads historical data from MetricStore and fits baselines.
// Should be called before the collector loop starts.
func (d *Detector) Bootstrap(ctx context.Context) error {
	instances, err := d.lister.ListInstanceIDs(ctx)
	if err != nil {
		return fmt.Errorf("listing instances for ML bootstrap: %w", err)
	}

	for _, instanceID := range instances {
		for _, mc := range d.config.Metrics {
			if !mc.Enabled {
				continue
			}

			windowSize := max(3*mc.Period, 1000)
			lookback := time.Duration(windowSize) * d.config.CollectionInterval
			points, err := d.store.Query(ctx, collector.MetricQuery{
				InstanceID: instanceID,
				Metric:     mc.Key,
				Start:      time.Now().Add(-lookback),
				End:        time.Now(),
				Limit:      windowSize,
			})
			if err != nil || len(points) < mc.Period*2 {
				have := 0
				if err == nil {
					have = len(points)
				}
				slog.Warn("insufficient history for ML baseline",
					"instance", instanceID,
					"metric", mc.Key,
					"have", have,
					"need", mc.Period*2)
				continue
			}

			b := NewSTLBaseline(mc.Key, mc.Period)
			// Points come in DESC order from store; reverse for chronological
			for i := len(points) - 1; i >= 0; i-- {
				b.Update(points[i].Value)
			}

			key := instanceID + ":" + mc.Key
			d.mu.Lock()
			d.baselines[key] = b
			d.mu.Unlock()

			slog.Info("ML baseline fitted",
				"instance", instanceID,
				"metric", mc.Key,
				"points", len(points),
				"residual_stddev", b.ResidualStddev())
		}
	}
	return nil
}

// Evaluate scores a batch of metric points against baselines and returns anomalies.
func (d *Detector) Evaluate(ctx context.Context, points []collector.MetricPoint) ([]AnomalyResult, error) {
	var results []AnomalyResult

	for _, p := range points {
		key := p.InstanceID + ":" + p.Metric
		d.mu.RLock()
		b, ok := d.baselines[key]
		d.mu.RUnlock()
		if !ok || !b.Ready() {
			continue
		}

		zScore, isIQR := b.Score(p.Value)

		d.mu.Lock()
		b.Update(p.Value)
		d.mu.Unlock()

		isAnomaly := false
		az := math.Abs(zScore)
		switch d.config.AnomalyLogic {
		case "and":
			isAnomaly = az >= d.config.ZScoreWarn && isIQR
		default: // "or"
			isAnomaly = az >= d.config.ZScoreWarn || isIQR
		}

		if !isAnomaly {
			continue
		}

		result := AnomalyResult{
			InstanceID: p.InstanceID,
			Metric:     p.Metric,
			Value:      p.Value,
			ZScore:     zScore,
			IsIQR:      isIQR,
			IsAnomaly:  true,
			Timestamp:  p.Timestamp,
		}
		results = append(results, result)

		method := "zscore"
		if isIQR && az < d.config.ZScoreWarn {
			method = "iqr"
		} else if isIQR {
			method = "both"
		}

		labels := map[string]string{
			"instance_id":     p.InstanceID,
			"original_metric": p.Metric,
			"method":          method,
			"original_value":  strconv.FormatFloat(p.Value, 'f', -1, 64),
		}
		if err := d.evaluator.Evaluate(ctx, "anomaly."+p.Metric, az, labels); err != nil {
			slog.Warn("anomaly alert dispatch failed", "err", err, "metric", p.Metric)
		}
	}
	return results, nil
}
