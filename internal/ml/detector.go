package ml

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"strings"
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
	config       DetectorConfig
	baselines    map[string]*STLBaseline // "instanceID:metricKey"
	mu           sync.RWMutex
	store        collector.MetricStore
	lister       InstanceLister
	evaluator    collector.AlertEvaluator
	persist      PersistenceStore
	bootstrapped bool
}

// NewDetector creates an ML anomaly detector.
func NewDetector(cfg DetectorConfig, store collector.MetricStore,
	lister InstanceLister, evaluator collector.AlertEvaluator,
	persist PersistenceStore) *Detector {
	return &Detector{
		config:    cfg,
		baselines: make(map[string]*STLBaseline),
		store:     store,
		lister:    lister,
		evaluator: evaluator,
		persist:   persist,
	}
}

// SetAlertEvaluator replaces the alert evaluator. Called from main.go
// after the alert pipeline is initialized, upgrading from the initial
// no-op to a real adapter.
func (d *Detector) SetAlertEvaluator(e collector.AlertEvaluator) {
	d.evaluator = e
}

// metricConfig returns the MetricConfig for the given key, or nil if not found.
func (d *Detector) metricConfig(key string) *MetricConfig {
	for i := range d.config.Metrics {
		if d.config.Metrics[i].Key == key {
			return &d.config.Metrics[i]
		}
	}
	return nil
}

// Bootstrap loads historical data from MetricStore and fits baselines.
// Should be called before the collector loop starts.
func (d *Detector) Bootstrap(ctx context.Context) error {
	// Phase 1: Load persisted snapshots
	persisted := map[string]bool{}

	if d.persist != nil {
		snaps, err := d.persist.LoadAll(ctx)
		if err != nil {
			slog.Warn("ML persistence load failed, will replay from TimescaleDB", "err", err)
		} else {
			for _, snap := range snaps {
				mc := d.metricConfig(snap.MetricKey)
				if mc == nil || !mc.Enabled {
					continue
				}
				staleness := time.Duration(2*mc.Period) * d.config.CollectionInterval
				age := time.Since(snap.UpdatedAt)
				if age > staleness {
					slog.Warn("ML baseline snapshot stale, will replay",
						"instance", snap.InstanceID,
						"metric", snap.MetricKey,
						"age", age.Round(time.Minute))
					continue
				}
				key := snap.InstanceID + ":" + snap.MetricKey
				b := LoadFromSnapshot(snap)
				d.mu.Lock()
				d.baselines[key] = b
				d.mu.Unlock()
				persisted[key] = true
				slog.Info("ML baseline loaded from snapshot",
					"instance", snap.InstanceID,
					"metric", snap.MetricKey,
					"age", age.Round(time.Minute))
			}
		}
	}

	// Phase 2: Replay from TimescaleDB for metrics not loaded from snapshot
	instances, err := d.lister.ListInstanceIDs(ctx)
	if err != nil {
		return fmt.Errorf("listing instances for ML bootstrap: %w", err)
	}

	replayCount := 0
	for _, instanceID := range instances {
		for _, mc := range d.config.Metrics {
			if !mc.Enabled {
				continue
			}
			key := instanceID + ":" + mc.Key
			if persisted[key] {
				continue // already loaded from snapshot
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
			for i := len(points) - 1; i >= 0; i-- {
				b.Update(points[i].Value)
			}

			d.mu.Lock()
			d.baselines[key] = b
			d.mu.Unlock()
			replayCount++

			slog.Info("ML baseline fitted",
				"instance", instanceID,
				"metric", mc.Key,
				"points", len(points),
				"residual_stddev", b.ResidualStddev())
		}
	}

	slog.Info("ML bootstrap complete",
		"from_snapshot", len(persisted),
		"from_replay", replayCount)
	d.bootstrapped = true
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

	// After processing all points: persist updated baselines
	if d.persist != nil {
		d.mu.RLock()
		for key, b := range d.baselines {
			parts := strings.SplitN(key, ":", 2)
			if len(parts) != 2 {
				continue
			}
			snap := b.Snapshot(parts[0])
			if err := d.persist.Save(ctx, snap); err != nil {
				slog.Warn("ML baseline persist failed", "key", key, "err", err)
			}
		}
		d.mu.RUnlock()
	}

	return results, nil
}

// Forecast computes a forecast for the given instance and metric.
func (d *Detector) Forecast(_ context.Context, instanceID, metricKey string, horizon int) (*ForecastResult, error) {
	if !d.bootstrapped {
		return nil, ErrNotBootstrapped
	}

	key := instanceID + ":" + metricKey
	d.mu.RLock()
	b, ok := d.baselines[key]
	d.mu.RUnlock()
	if !ok {
		return nil, ErrNoBaseline
	}

	z := d.config.ZScoreWarn // default confidence
	if d.config.ForecastZ > 0 {
		z = d.config.ForecastZ
	}

	points := b.Forecast(horizon, z, d.config.CollectionInterval, time.Now())
	if points == nil {
		return nil, ErrNoBaseline
	}

	return &ForecastResult{
		InstanceID:                instanceID,
		MetricKey:                 metricKey,
		GeneratedAt:               time.Now(),
		CollectionIntervalSeconds: int(d.config.CollectionInterval.Seconds()),
		Horizon:                   horizon,
		ConfidenceZ:               z,
		Points:                    points,
	}, nil
}
