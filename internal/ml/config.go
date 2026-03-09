package ml

import "time"

// MetricConfig configures ML baseline tracking for a single metric.
type MetricConfig struct {
	Key             string
	Period          int // seasonal period in data points
	Enabled         bool
	ForecastHorizon int // per-metric forecast horizon override (0 = use global default)
}

// DetectorConfig configures the ML anomaly detector.
type DetectorConfig struct {
	Enabled            bool
	ZScoreWarn         float64
	ZScoreCrit         float64
	ForecastZ          float64       // confidence Z for forecast bands (default: use ZScoreWarn)
	AnomalyLogic       string        // "or" | "and"
	Metrics            []MetricConfig
	CollectionInterval time.Duration
}

// DefaultConfig returns the default ML configuration.
func DefaultConfig() DetectorConfig {
	return DetectorConfig{
		Enabled:            true,
		ZScoreWarn:         3.0,
		ZScoreCrit:         5.0,
		AnomalyLogic:       "or",
		CollectionInterval: 60 * time.Second,
		Metrics: []MetricConfig{
			{Key: "connections.utilization_pct", Period: 1440, Enabled: true},
			{Key: "cache.hit_ratio", Period: 1440, Enabled: true},
			{Key: "transactions.commit_rate", Period: 1440, Enabled: true},
			{Key: "replication.lag_bytes", Period: 1440, Enabled: true},
			{Key: "locks.blocking_count", Period: 1440, Enabled: true},
		},
	}
}
