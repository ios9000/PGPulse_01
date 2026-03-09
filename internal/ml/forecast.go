package ml

import (
	"math"
	"time"
)

// ForecastPoint is one predicted future sample.
type ForecastPoint struct {
	Offset      int       `json:"offset"`
	PredictedAt time.Time `json:"predicted_at"`
	Value       float64   `json:"value"`
	Lower       float64   `json:"lower"`
	Upper       float64   `json:"upper"`
}

// ForecastResult is the full output of a Forecast call.
type ForecastResult struct {
	InstanceID                string          `json:"instance_id"`
	MetricKey                 string          `json:"metric_key"`
	GeneratedAt               time.Time       `json:"generated_at"`
	CollectionIntervalSeconds int             `json:"collection_interval_seconds"`
	Horizon                   int             `json:"horizon"`
	ConfidenceZ               float64         `json:"confidence_z"`
	Points                    []ForecastPoint `json:"points"`
}

// residualStddev computes sample standard deviation (n-1 denominator).
// Returns 0 if fewer than 2 values.
func residualStddev(vals []float64) float64 {
	n := len(vals)
	if n < 2 {
		return 0
	}
	sum := 0.0
	for _, v := range vals {
		sum += v
	}
	mean := sum / float64(n)
	sumSq := 0.0
	for _, v := range vals {
		d := v - mean
		sumSq += d * d
	}
	return math.Sqrt(sumSq / float64(n-1))
}
