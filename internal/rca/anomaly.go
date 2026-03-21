package rca

import (
	"context"
	"math"
	"time"

	"github.com/ios9000/PGPulse_01/internal/collector"
	"github.com/ios9000/PGPulse_01/internal/ml"
)

// AnomalySource detects anomalies in metric data within a time window.
type AnomalySource interface {
	// GetAnomalies returns anomalies for an instance across the given metric keys
	// within the specified window. The jitter parameter extends the window by
	// the given duration on each side to handle collection timing differences.
	GetAnomalies(ctx context.Context, instanceID string, metricKeys []string,
		from, to time.Time, jitter time.Duration) (map[string][]AnomalyEvent, error)
}

// AnomalyEvent represents a detected anomaly for a single metric.
type AnomalyEvent struct {
	InstanceID  string
	MetricKey   string
	Timestamp   time.Time
	Value       float64
	BaselineVal float64 // mean/expected value for context
	ZScore      float64 // anomaly magnitude (ML mode; 0 in threshold mode)
	RateChange  float64 // rate of change vs baseline (threshold mode)
	Strength    float64 // normalized 0.0-1.0 evidence strength
	Source      string  // "ml" or "threshold"
}

// ThresholdAnomalySource detects anomalies by comparing values against
// statistical baselines computed from historical data.
type ThresholdAnomalySource struct {
	metricStore collector.MetricStore
	stats       MetricStatsProvider // optional, for batch stats
}

// NewThresholdAnomalySource creates a threshold-based anomaly source.
// If metricStore implements MetricStatsProvider, it will be used for
// efficient batch statistics. Otherwise, raw query + in-Go computation is used.
func NewThresholdAnomalySource(metricStore collector.MetricStore) *ThresholdAnomalySource {
	var stats MetricStatsProvider
	if sp, ok := metricStore.(MetricStatsProvider); ok {
		stats = sp
	}
	return &ThresholdAnomalySource{
		metricStore: metricStore,
		stats:       stats,
	}
}

func (s *ThresholdAnomalySource) GetAnomalies(ctx context.Context, instanceID string,
	metricKeys []string, from, to time.Time, jitter time.Duration,
) (map[string][]AnomalyEvent, error) {
	result := make(map[string][]AnomalyEvent)

	// Compute baseline stats from 1 hour before the analysis window.
	baselineFrom := from.Add(-1 * time.Hour)
	baselineTo := from

	var baselineStats map[string]MetricStats
	if s.stats != nil {
		var err error
		baselineStats, err = s.stats.GetMetricStats(ctx, instanceID, metricKeys, baselineFrom, baselineTo)
		if err != nil {
			// Fall back to in-Go computation on error.
			baselineStats = nil
		}
	}

	if baselineStats == nil {
		baselineStats = make(map[string]MetricStats)
		for _, key := range metricKeys {
			stats, err := s.computeStatsFromQuery(ctx, instanceID, key, baselineFrom, baselineTo)
			if err != nil {
				continue
			}
			baselineStats[key] = stats
		}
	}

	// Query values in the analysis window (extended by jitter).
	queryFrom := from.Add(-jitter)
	queryTo := to.Add(jitter)

	for _, key := range metricKeys {
		stats, ok := baselineStats[key]
		if !ok || stats.Count < 2 {
			continue
		}

		points, err := s.metricStore.Query(ctx, collector.MetricQuery{
			InstanceID: instanceID,
			Metric:     key,
			Start:      queryFrom,
			End:        queryTo,
		})
		if err != nil || len(points) == 0 {
			continue
		}

		for _, p := range points {
			if stats.StdDev == 0 {
				continue
			}
			deviation := math.Abs(p.Value - stats.Mean)
			zScore := deviation / stats.StdDev
			if zScore < 2.0 {
				continue // not anomalous
			}

			rateChange := 0.0
			if stats.Mean != 0 {
				rateChange = p.Value / stats.Mean
			}

			// Normalize strength to 0-1 range: zScore of 2 -> 0.3, zScore of 5+ -> 1.0
			strength := math.Min(1.0, (zScore-2.0)/3.0*0.7+0.3)

			result[key] = append(result[key], AnomalyEvent{
				InstanceID:  instanceID,
				MetricKey:   key,
				Timestamp:   p.Timestamp,
				Value:       p.Value,
				BaselineVal: stats.Mean,
				ZScore:      zScore,
				RateChange:  rateChange,
				Strength:    strength,
				Source:      "threshold",
			})
		}
	}

	return result, nil
}

// computeStatsFromQuery fetches raw points and computes statistics in Go.
func (s *ThresholdAnomalySource) computeStatsFromQuery(ctx context.Context,
	instanceID, key string, from, to time.Time,
) (MetricStats, error) {
	points, err := s.metricStore.Query(ctx, collector.MetricQuery{
		InstanceID: instanceID,
		Metric:     key,
		Start:      from,
		End:        to,
	})
	if err != nil {
		return MetricStats{}, err
	}
	if len(points) == 0 {
		return MetricStats{}, nil
	}

	var sum, sumSq, minVal, maxVal float64
	minVal = points[0].Value
	maxVal = points[0].Value
	for _, p := range points {
		sum += p.Value
		sumSq += p.Value * p.Value
		if p.Value < minVal {
			minVal = p.Value
		}
		if p.Value > maxVal {
			maxVal = p.Value
		}
	}
	n := float64(len(points))
	mean := sum / n
	variance := sumSq/n - mean*mean
	if variance < 0 {
		variance = 0
	}

	return MetricStats{
		Mean:   mean,
		StdDev: math.Sqrt(variance),
		Min:    minVal,
		Max:    maxVal,
		Count:  len(points),
	}, nil
}

// MLAnomalySource wraps *ml.Detector and falls back to threshold detection
// for metrics that ML does not track.
type MLAnomalySource struct {
	detector  *ml.Detector
	fallback  *ThresholdAnomalySource
	store     collector.MetricStore
}

// NewMLAnomalySource creates an ML-backed anomaly source with threshold fallback.
func NewMLAnomalySource(detector *ml.Detector, store collector.MetricStore) *MLAnomalySource {
	return &MLAnomalySource{
		detector: detector,
		fallback: NewThresholdAnomalySource(store),
		store:    store,
	}
}

func (s *MLAnomalySource) GetAnomalies(ctx context.Context, instanceID string,
	metricKeys []string, from, to time.Time, jitter time.Duration,
) (map[string][]AnomalyEvent, error) {
	result := make(map[string][]AnomalyEvent)

	// Try ML detection for each metric key by querying points in the window
	// and evaluating them through the detector.
	var fallbackKeys []string

	queryFrom := from.Add(-jitter)
	queryTo := to.Add(jitter)

	for _, key := range metricKeys {
		points, err := s.store.Query(ctx, collector.MetricQuery{
			InstanceID: instanceID,
			Metric:     key,
			Start:      queryFrom,
			End:        queryTo,
		})
		if err != nil || len(points) == 0 {
			fallbackKeys = append(fallbackKeys, key)
			continue
		}

		// Evaluate through ML detector.
		anomalies, err := s.detector.Evaluate(ctx, points)
		if err != nil {
			fallbackKeys = append(fallbackKeys, key)
			continue
		}

		if len(anomalies) == 0 {
			// ML found no anomalies, but the detector might not track this metric.
			// Check if we got any evaluation at all by seeing if the detector
			// returned results. If empty, fall back to threshold.
			fallbackKeys = append(fallbackKeys, key)
			continue
		}

		for _, a := range anomalies {
			zAbs := math.Abs(a.ZScore)
			strength := math.Min(1.0, zAbs/5.0)
			result[key] = append(result[key], AnomalyEvent{
				InstanceID:  instanceID,
				MetricKey:   a.Metric,
				Timestamp:   a.Timestamp,
				Value:       a.Value,
				BaselineVal: 0, // ML detector does not expose baseline value directly
				ZScore:      a.ZScore,
				Strength:    strength,
				Source:      "ml",
			})
		}
	}

	// Fall back to threshold for metrics ML didn't handle.
	if len(fallbackKeys) > 0 {
		fbResult, err := s.fallback.GetAnomalies(ctx, instanceID, fallbackKeys, from, to, jitter)
		if err != nil {
			return result, nil // partial results are acceptable
		}
		for k, v := range fbResult {
			result[k] = v
		}
	}

	return result, nil
}

// AnomalyMode returns the string label for the anomaly source type.
func AnomalyMode(src AnomalySource) string {
	switch src.(type) {
	case *MLAnomalySource:
		return "ml"
	default:
		return "threshold"
	}
}
