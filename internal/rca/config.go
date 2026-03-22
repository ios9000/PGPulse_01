package rca

import "time"

// RCAConfig holds configuration for the RCA engine.
type RCAConfig struct {
	Enabled                 bool          `koanf:"enabled"`
	LookbackWindow          time.Duration `koanf:"lookback_window"`
	AutoTriggerSeverity     string        `koanf:"auto_trigger_severity"`
	MaxIncidentsPerHour     int           `koanf:"max_incidents_per_hour"`
	RetentionDays           int           `koanf:"retention_days"`
	MaxTraversalDepth       int           `koanf:"max_traversal_depth"`
	MaxCandidateChains      int           `koanf:"max_candidate_chains"`
	MaxMetricsPerRun        int           `koanf:"max_metrics_per_run"`
	MinEdgeScore            float64       `koanf:"min_edge_score"`
	MinChainScore           float64       `koanf:"min_chain_score"`
	DeferredForwardTail     time.Duration `koanf:"deferred_forward_tail"`
	QualityBannerEnabled    bool          `koanf:"quality_banner_enabled"`
	RemediationHooksEnabled  bool          `koanf:"remediation_hooks_enabled"`
	ThresholdBaselineWindow  time.Duration `koanf:"threshold_baseline_window"`
	ThresholdCalmPeriod      time.Duration `koanf:"threshold_calm_period"`
	ThresholdCalmSigma       float64       `koanf:"threshold_calm_sigma"`
}

// DefaultRCAConfig returns production-safe defaults.
func DefaultRCAConfig() RCAConfig {
	return RCAConfig{
		Enabled:                 false,
		LookbackWindow:          30 * time.Minute,
		AutoTriggerSeverity:     "critical",
		MaxIncidentsPerHour:     10,
		RetentionDays:           90,
		MaxTraversalDepth:       5,
		MaxCandidateChains:      5,
		MaxMetricsPerRun:        50,
		MinEdgeScore:            0.25,
		MinChainScore:           0.40,
		DeferredForwardTail:     5 * time.Minute,
		QualityBannerEnabled:    true,
		RemediationHooksEnabled:  true,
		ThresholdBaselineWindow:  4 * time.Hour,
		ThresholdCalmPeriod:      15 * time.Minute,
		ThresholdCalmSigma:       1.5,
	}
}
