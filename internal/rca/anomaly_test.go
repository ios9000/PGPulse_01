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
