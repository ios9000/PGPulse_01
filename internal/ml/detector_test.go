package ml

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/ios9000/PGPulse_01/internal/collector"
)

// mockMetricStore implements collector.MetricStore for testing.
type mockMetricStore struct {
	mu     sync.Mutex
	points []collector.MetricPoint
}

func (m *mockMetricStore) Write(_ context.Context, points []collector.MetricPoint) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.points = append(m.points, points...)
	return nil
}

func (m *mockMetricStore) Query(_ context.Context, q collector.MetricQuery) ([]collector.MetricPoint, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []collector.MetricPoint
	for _, p := range m.points {
		if p.InstanceID == q.InstanceID && p.Metric == q.Metric {
			result = append(result, p)
		}
	}
	return result, nil
}

func (m *mockMetricStore) Close() error { return nil }

// mockAlertEvaluator records Evaluate calls.
type mockAlertEvaluator struct {
	mu    sync.Mutex
	calls []alertCall
}

type alertCall struct {
	metric string
	value  float64
	labels map[string]string
}

func (m *mockAlertEvaluator) Evaluate(_ context.Context, metric string, value float64, labels map[string]string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, alertCall{metric: metric, value: value, labels: labels})
	return nil
}

func (m *mockAlertEvaluator) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.calls)
}

// mockInstanceLister for testing.
type mockInstanceLister struct {
	ids []string
}

func (m *mockInstanceLister) ListInstanceIDs(_ context.Context) ([]string, error) {
	return m.ids, nil
}

func TestDetector_EvaluateNoBaseline(t *testing.T) {
	d := NewDetector(DefaultConfig(), &mockMetricStore{}, &mockInstanceLister{}, &mockAlertEvaluator{})
	results, err := d.Evaluate(context.Background(), []collector.MetricPoint{
		{InstanceID: "i1", Metric: "unknown.metric", Value: 100, Timestamp: time.Now()},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results for unknown metric, got %d", len(results))
	}
}

func TestDetector_EvaluateAnomaly(t *testing.T) {
	eval := &mockAlertEvaluator{}
	d := NewDetector(DetectorConfig{
		Enabled:    true,
		ZScoreWarn: 3.0,
		ZScoreCrit: 5.0,
		AnomalyLogic: "or",
		Metrics: []MetricConfig{
			{Key: "test.metric", Period: 10, Enabled: true},
		},
	}, &mockMetricStore{}, &mockInstanceLister{}, eval)

	// Manually inject a fitted baseline
	b := NewSTLBaseline("test.metric", 10)
	for i := 0; i < 100; i++ {
		b.Update(100.0)
	}
	d.baselines["i1:test.metric"] = b

	// Evaluate with an outlier
	results, err := d.Evaluate(context.Background(), []collector.MetricPoint{
		{InstanceID: "i1", Metric: "test.metric", Value: 500.0, Timestamp: time.Now()},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 anomaly, got %d", len(results))
	}
	if !results[0].IsAnomaly {
		t.Error("expected IsAnomaly=true")
	}
	if results[0].InstanceID != "i1" {
		t.Errorf("expected instance i1, got %s", results[0].InstanceID)
	}
	if results[0].Metric != "test.metric" {
		t.Errorf("expected metric test.metric, got %s", results[0].Metric)
	}
	if eval.callCount() != 1 {
		t.Errorf("expected 1 alert call, got %d", eval.callCount())
	}
}

func TestDetector_NormalValueNoAnomaly(t *testing.T) {
	eval := &mockAlertEvaluator{}
	d := NewDetector(DetectorConfig{
		Enabled:      true,
		ZScoreWarn:   3.0,
		AnomalyLogic: "and", // AND mode: need both z-score AND IQR
		Metrics:      []MetricConfig{{Key: "m", Period: 10, Enabled: true}},
	}, &mockMetricStore{}, &mockInstanceLister{}, eval)

	b := NewSTLBaseline("m", 10)
	for i := 0; i < 100; i++ {
		b.Update(100.0)
	}
	d.baselines["i1:m"] = b

	// Feed a normal value
	results, err := d.Evaluate(context.Background(), []collector.MetricPoint{
		{InstanceID: "i1", Metric: "m", Value: 100.0, Timestamp: time.Now()},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results for normal value, got %d", len(results))
	}
	if eval.callCount() != 0 {
		t.Errorf("expected 0 alert calls, got %d", eval.callCount())
	}
}

func TestDetector_AnomalyLogicOr(t *testing.T) {
	eval := &mockAlertEvaluator{}
	d := NewDetector(DetectorConfig{
		Enabled:      true,
		ZScoreWarn:   3.0,
		AnomalyLogic: "or",
		Metrics:      []MetricConfig{{Key: "m", Period: 10, Enabled: true}},
	}, &mockMetricStore{}, &mockInstanceLister{}, eval)

	b := NewSTLBaseline("m", 10)
	for i := 0; i < 100; i++ {
		b.Update(100.0 + float64(i%3))
	}
	d.baselines["i1:m"] = b

	// Large outlier should trigger in OR mode
	results, _ := d.Evaluate(context.Background(), []collector.MetricPoint{
		{InstanceID: "i1", Metric: "m", Value: 999.0, Timestamp: time.Now()},
	})
	if len(results) == 0 || !results[0].IsAnomaly {
		t.Error("OR mode: large outlier should be flagged")
	}
}

func TestDetector_AnomalyLogicAnd(t *testing.T) {
	eval := &mockAlertEvaluator{}
	d := NewDetector(DetectorConfig{
		Enabled:      true,
		ZScoreWarn:   100.0, // Very high threshold -- z-score won't reach this
		AnomalyLogic: "and",
		Metrics:      []MetricConfig{{Key: "m", Period: 10, Enabled: true}},
	}, &mockMetricStore{}, &mockInstanceLister{}, eval)

	b := NewSTLBaseline("m", 10)
	for i := 0; i < 100; i++ {
		b.Update(100.0)
	}
	d.baselines["i1:m"] = b

	// Even a moderate outlier shouldn't trigger when threshold is very high in AND mode
	results, _ := d.Evaluate(context.Background(), []collector.MetricPoint{
		{InstanceID: "i1", Metric: "m", Value: 150.0, Timestamp: time.Now()},
	})
	for _, r := range results {
		if r.IsAnomaly {
			t.Error("AND mode: should not flag when z-score threshold is unreachable")
		}
	}
}

func TestDetector_BaselineNotReady(t *testing.T) {
	d := NewDetector(DetectorConfig{
		Enabled:      true,
		ZScoreWarn:   3.0,
		AnomalyLogic: "or",
		Metrics:      []MetricConfig{{Key: "m", Period: 10, Enabled: true}},
	}, &mockMetricStore{}, &mockInstanceLister{}, &mockAlertEvaluator{})

	// Inject a baseline that's not ready (too few points)
	b := NewSTLBaseline("m", 10)
	for i := 0; i < 5; i++ {
		b.Update(100.0)
	}
	d.baselines["i1:m"] = b

	results, err := d.Evaluate(context.Background(), []collector.MetricPoint{
		{InstanceID: "i1", Metric: "m", Value: 999.0, Timestamp: time.Now()},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results for unready baseline, got %d", len(results))
	}
}

func TestDetector_MultiplePoints(t *testing.T) {
	eval := &mockAlertEvaluator{}
	d := NewDetector(DetectorConfig{
		Enabled:      true,
		ZScoreWarn:   3.0,
		AnomalyLogic: "or",
		Metrics: []MetricConfig{
			{Key: "m1", Period: 10, Enabled: true},
			{Key: "m2", Period: 10, Enabled: true},
		},
	}, &mockMetricStore{}, &mockInstanceLister{}, eval)

	// Set up baselines for two metrics
	for _, key := range []string{"m1", "m2"} {
		b := NewSTLBaseline(key, 10)
		for i := 0; i < 100; i++ {
			b.Update(100.0)
		}
		d.baselines["i1:"+key] = b
	}

	results, _ := d.Evaluate(context.Background(), []collector.MetricPoint{
		{InstanceID: "i1", Metric: "m1", Value: 999.0, Timestamp: time.Now()},
		{InstanceID: "i1", Metric: "m2", Value: 100.0, Timestamp: time.Now()},
	})
	// m1=999 should be anomaly, m2=100 may or may not depending on residual
	anomalyCount := 0
	for _, r := range results {
		if r.IsAnomaly {
			anomalyCount++
		}
	}
	if anomalyCount < 1 {
		t.Error("expected at least 1 anomaly from the outlier metric")
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if !cfg.Enabled {
		t.Error("default config should be enabled")
	}
	if cfg.ZScoreWarn != 3.0 {
		t.Errorf("expected warn threshold 3.0, got %f", cfg.ZScoreWarn)
	}
	if cfg.ZScoreCrit != 5.0 {
		t.Errorf("expected crit threshold 5.0, got %f", cfg.ZScoreCrit)
	}
	if cfg.AnomalyLogic != "or" {
		t.Errorf("expected anomaly logic 'or', got %q", cfg.AnomalyLogic)
	}
	if len(cfg.Metrics) == 0 {
		t.Error("default config should have metrics")
	}
	if cfg.CollectionInterval != 60*time.Second {
		t.Errorf("expected 60s collection interval, got %v", cfg.CollectionInterval)
	}
}

func TestNewDetector_NotNil(t *testing.T) {
	d := NewDetector(DefaultConfig(), &mockMetricStore{}, &mockInstanceLister{}, &mockAlertEvaluator{})
	if d == nil {
		t.Fatal("NewDetector returned nil")
	}
	if d.baselines == nil {
		t.Error("baselines map should be initialized")
	}
}
