package ml

import (
	"errors"
	"math"
	"time"
)

// WMAConfig controls the weighted moving average behavior.
type WMAConfig struct {
	WindowSize  int     // max samples to consider (default 10)
	DecayFactor float64 // exponential decay for older samples (default 0.85)
}

// WMAResult holds the output of a weighted moving average calculation.
type WMAResult struct {
	WeightedRate float64 // weighted average rate (units per second)
	SampleCount  int     // actual samples used
	StdDev       float64 // weighted standard deviation of rates
}

// WeightedMovingAverage computes the WMA rate from a slice of (timestamp, value) pairs.
// Pairs must be sorted chronologically (oldest first).
// Returns error if fewer than 2 data points.
//
// Algorithm:
//  1. Compute per-interval rates: rate_i = (value_i - value_{i-1}) / (time_i - time_{i-1})
//  2. Apply exponential decay weights: weight_i = decay^(N - i - 1) where i=0 is oldest
//  3. weighted_rate = Σ(weight_i * rate_i) / Σ(weight_i)
//  4. weighted_stddev = sqrt(Σ(weight_i * (rate_i - weighted_rate)²) / Σ(weight_i))
func WeightedMovingAverage(cfg WMAConfig, timestamps []time.Time, values []float64) (WMAResult, error) {
	n := len(timestamps)
	if n < 2 {
		return WMAResult{}, errors.New("ml: WMA requires at least 2 data points")
	}
	if n != len(values) {
		return WMAResult{}, errors.New("ml: timestamps and values must have equal length")
	}

	// Limit to window size.
	start := 0
	if cfg.WindowSize > 0 && n > cfg.WindowSize {
		start = n - cfg.WindowSize
	}

	decay := cfg.DecayFactor
	if decay <= 0 || decay >= 1 {
		decay = 0.85
	}

	// Compute per-interval rates.
	var rates []float64
	for i := max(start, 1); i < n; i++ {
		dt := timestamps[i].Sub(timestamps[i-1]).Seconds()
		if dt <= 0 {
			continue
		}
		rate := (values[i] - values[i-1]) / dt
		rates = append(rates, rate)
	}

	if len(rates) == 0 {
		return WMAResult{SampleCount: n}, nil
	}

	// Apply exponential decay weights: most recent rate gets weight 1.
	numRates := len(rates)
	var sumWeight, sumWeightedRate float64
	weights := make([]float64, numRates)
	for i := 0; i < numRates; i++ {
		weights[i] = math.Pow(decay, float64(numRates-i-1))
		sumWeight += weights[i]
		sumWeightedRate += weights[i] * rates[i]
	}

	weightedRate := sumWeightedRate / sumWeight

	// Weighted standard deviation.
	var sumWeightedVar float64
	for i := 0; i < numRates; i++ {
		diff := rates[i] - weightedRate
		sumWeightedVar += weights[i] * diff * diff
	}
	stddev := math.Sqrt(sumWeightedVar / sumWeight)

	return WMAResult{
		WeightedRate: weightedRate,
		SampleCount:  n,
		StdDev:       stddev,
	}, nil
}
