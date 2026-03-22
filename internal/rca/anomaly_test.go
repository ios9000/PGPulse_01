package rca

import (
	"context"
	"testing"
	"time"

	"github.com/ios9000/PGPulse_01/internal/collector"
	"github.com/ios9000/PGPulse_01/internal/storage"
)

func TestThresholdAnomalySource_Basic(t *testing.T) {
	t.Parallel()

	ms := storage.NewMemoryStore(time.Hour)
	now := time.Now()

	// Write baseline data (1 hour before analysis window): stable around 100.
	for i := 0; i < 60; i++ {
		ts := now.Add(-2*time.Hour + time.Duration(i)*time.Minute)
		_ = ms.Write(context.Background(), []collector.MetricPoint{{
			InstanceID: "inst-1",
			Metric:     "pg.connections.total",
			Value:      100 + float64(i%5), // small variance: 100-104
			Timestamp:  ts,
		}})
	}

	// Write spike in analysis window.
	_ = ms.Write(context.Background(), []collector.MetricPoint{{
		InstanceID: "inst-1",
		Metric:     "pg.connections.total",
		Value:      500, // well above baseline
		Timestamp:  now.Add(-5 * time.Minute),
	}})

	src := NewThresholdAnomalySource(ms)
	anomalies, err := src.GetAnomalies(context.Background(), "inst-1",
		[]string{"pg.connections.total"},
		now.Add(-30*time.Minute), now,
		90*time.Second,
	)
	if err != nil {
		t.Fatalf("GetAnomalies error: %v", err)
	}

	events := anomalies["pg.connections.total"]
	if len(events) == 0 {
		t.Fatal("expected at least one anomaly for the spike")
	}
	if events[0].Strength <= 0 || events[0].Strength > 1.0 {
		t.Errorf("anomaly strength %f out of valid range", events[0].Strength)
	}
	if events[0].Source != "threshold" {
		t.Errorf("expected source 'threshold', got %s", events[0].Source)
	}
}

func TestThresholdAnomalySource_NoData(t *testing.T) {
	t.Parallel()

	ms := storage.NewMemoryStore(time.Hour)
	src := NewThresholdAnomalySource(ms)

	now := time.Now()
	anomalies, err := src.GetAnomalies(context.Background(), "inst-1",
		[]string{"pg.connections.total"},
		now.Add(-30*time.Minute), now,
		90*time.Second,
	)
	if err != nil {
		t.Fatalf("GetAnomalies error: %v", err)
	}

	if len(anomalies) != 0 {
		t.Errorf("expected empty anomaly map with no data, got %d keys", len(anomalies))
	}
}

func TestThresholdAnomalySource_StableData(t *testing.T) {
	t.Parallel()

	ms := storage.NewMemoryStore(time.Hour)
	now := time.Now()

	// Write uniform baseline data.
	for i := 0; i < 120; i++ {
		ts := now.Add(-2*time.Hour + time.Duration(i)*time.Minute)
		_ = ms.Write(context.Background(), []collector.MetricPoint{{
			InstanceID: "inst-1",
			Metric:     "pg.connections.total",
			Value:      100,
			Timestamp:  ts,
		}})
	}

	src := NewThresholdAnomalySource(ms)
	anomalies, err := src.GetAnomalies(context.Background(), "inst-1",
		[]string{"pg.connections.total"},
		now.Add(-30*time.Minute), now,
		90*time.Second,
	)
	if err != nil {
		t.Fatalf("GetAnomalies error: %v", err)
	}

	// With perfectly uniform data, stddev is 0, so no anomalies should be detected.
	if len(anomalies["pg.connections.total"]) != 0 {
		t.Errorf("expected no anomalies for stable data, got %d", len(anomalies["pg.connections.total"]))
	}
}

func TestThresholdAnomalySource_UnreliableBaseline(t *testing.T) {
	t.Parallel()

	ms := storage.NewMemoryStore(time.Hour)
	now := time.Now()

	// Write highly variable baseline data (CV > 1.5).
	// We need mean close to 0 or very high variance relative to mean.
	// Values: mostly 1 with some at 1000 -> high CV.
	for i := 0; i < 60; i++ {
		ts := now.Add(-2*time.Hour + time.Duration(i)*time.Minute)
		value := 1.0
		if i%10 == 0 {
			value = 1000.0 // rare spikes create high StdDev relative to mean
		}
		_ = ms.Write(context.Background(), []collector.MetricPoint{{
			InstanceID: "inst-1",
			Metric:     "pg.connections.total",
			Value:      value,
			Timestamp:  ts,
		}})
	}

	// Write a spike in analysis window.
	_ = ms.Write(context.Background(), []collector.MetricPoint{{
		InstanceID: "inst-1",
		Metric:     "pg.connections.total",
		Value:      2000,
		Timestamp:  now.Add(-5 * time.Minute),
	}})

	// Use a low calmSigma threshold so the noisy data is flagged as unreliable.
	src := NewThresholdAnomalySourceWithConfig(ms, RCAConfig{
		ThresholdBaselineWindow: 1 * time.Hour,
		ThresholdCalmPeriod:     15 * time.Minute,
		ThresholdCalmSigma:      0.5, // strict: CV must be < 0.5
	})
	anomalies, err := src.GetAnomalies(context.Background(), "inst-1",
		[]string{"pg.connections.total"},
		now.Add(-30*time.Minute), now,
		90*time.Second,
	)
	if err != nil {
		t.Fatalf("GetAnomalies error: %v", err)
	}

	events := anomalies["pg.connections.total"]
	if len(events) == 0 {
		t.Fatal("expected anomaly even with noisy baseline")
	}
	// Anomaly should be marked as unreliable.
	if events[0].Source != "threshold_unreliable" {
		t.Errorf("expected source 'threshold_unreliable' for noisy baseline, got %s", events[0].Source)
	}
	// Strength should be reduced (halved) for unreliable baselines.
	if events[0].Strength > 0.55 {
		t.Errorf("expected reduced strength for unreliable baseline, got %f", events[0].Strength)
	}
}

func TestThresholdAnomalySource_ConfigurableWindow(t *testing.T) {
	t.Parallel()

	ms := storage.NewMemoryStore(5 * time.Hour)
	now := time.Now()

	// Write baseline data in a 4-hour window before analysis.
	for i := 0; i < 240; i++ {
		ts := now.Add(-5*time.Hour + time.Duration(i)*time.Minute)
		_ = ms.Write(context.Background(), []collector.MetricPoint{{
			InstanceID: "inst-1",
			Metric:     "pg.connections.total",
			Value:      100 + float64(i%3),
			Timestamp:  ts,
		}})
	}

	// Spike in analysis window.
	_ = ms.Write(context.Background(), []collector.MetricPoint{{
		InstanceID: "inst-1",
		Metric:     "pg.connections.total",
		Value:      500,
		Timestamp:  now.Add(-5 * time.Minute),
	}})

	cfg := RCAConfig{
		ThresholdBaselineWindow: 4 * time.Hour,
		ThresholdCalmPeriod:     15 * time.Minute,
		ThresholdCalmSigma:      1.5,
	}
	src := NewThresholdAnomalySourceWithConfig(ms, cfg)
	anomalies, err := src.GetAnomalies(context.Background(), "inst-1",
		[]string{"pg.connections.total"},
		now.Add(-30*time.Minute), now,
		90*time.Second,
	)
	if err != nil {
		t.Fatalf("GetAnomalies error: %v", err)
	}

	events := anomalies["pg.connections.total"]
	if len(events) == 0 {
		t.Fatal("expected anomaly with 4h baseline window")
	}
	if events[0].Source != "threshold" {
		t.Errorf("expected source 'threshold' for calm baseline, got %s", events[0].Source)
	}
}

func TestIsBaselineCalm(t *testing.T) {
	t.Parallel()

	s := &ThresholdAnomalySource{calmSigma: 1.5}

	tests := []struct {
		name  string
		stats MetricStats
		want  bool
	}{
		{"calm baseline", MetricStats{Mean: 100, StdDev: 10, Count: 50}, true},
		{"noisy baseline", MetricStats{Mean: 100, StdDev: 200, Count: 50}, false},
		{"zero mean", MetricStats{Mean: 0, StdDev: 10, Count: 50}, false},
		{"insufficient data", MetricStats{Mean: 100, StdDev: 10, Count: 1}, false},
		{"edge case cv=1.5", MetricStats{Mean: 100, StdDev: 150, Count: 50}, true},
		{"slightly over cv=1.5", MetricStats{Mean: 100, StdDev: 151, Count: 50}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := s.isBaselineCalm(tt.stats)
			if got != tt.want {
				t.Errorf("isBaselineCalm(%+v) = %v, want %v", tt.stats, got, tt.want)
			}
		})
	}
}
