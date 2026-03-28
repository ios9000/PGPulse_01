package forecast

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/ios9000/PGPulse_01/internal/collector"
	"github.com/ios9000/PGPulse_01/internal/config"
)

// --- Mock implementations ---

type mockMetricStore struct {
	points map[string][]collector.MetricPoint // keyed by metric name
}

func (m *mockMetricStore) Write(_ context.Context, _ []collector.MetricPoint) error { return nil }
func (m *mockMetricStore) Close() error                                            { return nil }

func (m *mockMetricStore) Query(_ context.Context, q collector.MetricQuery) ([]collector.MetricPoint, error) {
	if m.points == nil {
		return nil, nil
	}
	return m.points[q.Metric], nil
}

type mockThresholdQuerier struct {
	thresholds []TableThresholds
	err        error
}

func (m *mockThresholdQuerier) GetTableThresholds(_ context.Context, _ string) ([]TableThresholds, error) {
	return m.thresholds, m.err
}

type mockInstanceLister struct {
	ids []string
}

func (m *mockInstanceLister) ActiveInstanceIDs(_ context.Context) ([]string, error) {
	return m.ids, nil
}

func defaultTestCfg() config.MaintenanceForecastConfig {
	cfg := config.MaintenanceForecastConfig{
		Enabled:                    true,
		MinDataPoints:              3,
		LookbackWindow:             24 * time.Hour,
		VacuumThresholdFallback:    50,
		VacuumScaleFactorFallback:  0.2,
		AnalyzeThresholdFallback:   50,
		AnalyzeScaleFactorFallback: 0.1,
		ReindexBloatThresholdTable: 0.40,
		ReindexBloatThresholdIndex: 0.30,
		EvaluationInterval:         5 * time.Minute,
		RetentionDays:              90,
	}
	cfg.ApplyDefaults()
	return cfg
}

// --- Tests for evaluateVacuum ---

func TestEvaluateVacuum_Predicted(t *testing.T) {
	cfg := defaultTestCfg()

	threshold := TableThresholds{
		Database:             "mydb",
		Table:                "orders",
		RelTuples:            10000,
		AutovacuumEnabled:    true,
		VacuumThreshold:      50,
		VacuumScaleFactor:    0.2,
		EffectiveVacuumLimit: 2050, // 50 + 0.2 * 10000
	}

	// Dead tuples rising: 100 → 400 → 700 → 1000 over 15 minutes.
	// Rate = 1 per second. Threshold = 2050. Current = 1000. Remaining = 1050.
	// time_to_threshold = 1050 / 1 = 1050s < 3600 → should be "imminent"
	// Actually let me make it slower so it's "predicted":
	// Dead tuples: 100 → 200 → 300 → 400 over 15 minutes.
	// Rate = (400-100) / (3*300s) = 300/900 = 0.333/s
	// Remaining = 2050 - 400 = 1650. time_to = 1650/0.333 = ~4950s > 3600 → "predicted"
	points := make([]collector.MetricPoint, 4)
	now := time.Now()
	for i := 0; i < 4; i++ {
		points[i] = collector.MetricPoint{
			InstanceID: "inst-1",
			Metric:     "pg.db.vacuum.dead_tuples",
			Value:      100 + float64(i*100),
			Labels:     map[string]string{"datname": "mydb", "relname": "orders"},
			Timestamp:  now.Add(time.Duration(-3+i) * 5 * time.Minute),
		}
	}

	store := &mockMetricStore{
		points: map[string][]collector.MetricPoint{
			"pg.db.vacuum.dead_tuples": points,
		},
	}

	eval := NewNeedEvaluator(store, &NullForecastStore{}, nil, &mockInstanceLister{}, &mockThresholdQuerier{}, cfg, slog.Default())
	result := eval.evaluateVacuum(context.Background(), "inst-1", threshold, now)

	if result.Status != "predicted" {
		t.Errorf("status = %q, want %q", result.Status, "predicted")
	}
	if result.Operation != "vacuum" {
		t.Errorf("operation = %q, want %q", result.Operation, "vacuum")
	}
	if result.CurrentValue != 400 {
		t.Errorf("current_value = %f, want 400", result.CurrentValue)
	}
	if result.AccumulationRate <= 0 {
		t.Errorf("accumulation_rate = %f, want > 0", result.AccumulationRate)
	}
	if result.TimeUntilSec <= 3600 {
		t.Errorf("time_until_sec = %f, want > 3600 for 'predicted'", result.TimeUntilSec)
	}
	if result.PredictedAt == nil {
		t.Error("predicted_at should not be nil for 'predicted' status")
	}
}

func TestEvaluateVacuum_Overdue(t *testing.T) {
	cfg := defaultTestCfg()
	now := time.Now()

	threshold := TableThresholds{
		Database:             "mydb",
		Table:                "orders",
		EffectiveVacuumLimit: 500,
	}

	// Dead tuples already above threshold.
	points := []collector.MetricPoint{
		{InstanceID: "inst-1", Metric: "pg.db.vacuum.dead_tuples", Value: 300, Labels: map[string]string{"datname": "mydb", "relname": "orders"}, Timestamp: now.Add(-10 * time.Minute)},
		{InstanceID: "inst-1", Metric: "pg.db.vacuum.dead_tuples", Value: 400, Labels: map[string]string{"datname": "mydb", "relname": "orders"}, Timestamp: now.Add(-5 * time.Minute)},
		{InstanceID: "inst-1", Metric: "pg.db.vacuum.dead_tuples", Value: 600, Labels: map[string]string{"datname": "mydb", "relname": "orders"}, Timestamp: now},
	}

	store := &mockMetricStore{
		points: map[string][]collector.MetricPoint{
			"pg.db.vacuum.dead_tuples": points,
		},
	}

	eval := NewNeedEvaluator(store, &NullForecastStore{}, nil, &mockInstanceLister{}, &mockThresholdQuerier{}, cfg, slog.Default())
	result := eval.evaluateVacuum(context.Background(), "inst-1", threshold, now)

	if result.Status != "overdue" {
		t.Errorf("status = %q, want %q", result.Status, "overdue")
	}
	if result.TimeUntilSec != 0 {
		t.Errorf("time_until_sec = %f, want 0 for overdue", result.TimeUntilSec)
	}
}

func TestEvaluateVacuum_NotNeeded(t *testing.T) {
	cfg := defaultTestCfg()
	now := time.Now()

	threshold := TableThresholds{
		Database:             "mydb",
		Table:                "orders",
		EffectiveVacuumLimit: 500,
	}

	// Dead tuples decreasing (vacuum recently ran).
	points := []collector.MetricPoint{
		{InstanceID: "inst-1", Metric: "pg.db.vacuum.dead_tuples", Value: 300, Labels: map[string]string{"datname": "mydb", "relname": "orders"}, Timestamp: now.Add(-10 * time.Minute)},
		{InstanceID: "inst-1", Metric: "pg.db.vacuum.dead_tuples", Value: 200, Labels: map[string]string{"datname": "mydb", "relname": "orders"}, Timestamp: now.Add(-5 * time.Minute)},
		{InstanceID: "inst-1", Metric: "pg.db.vacuum.dead_tuples", Value: 100, Labels: map[string]string{"datname": "mydb", "relname": "orders"}, Timestamp: now},
	}

	store := &mockMetricStore{
		points: map[string][]collector.MetricPoint{
			"pg.db.vacuum.dead_tuples": points,
		},
	}

	eval := NewNeedEvaluator(store, &NullForecastStore{}, nil, &mockInstanceLister{}, &mockThresholdQuerier{}, cfg, slog.Default())
	result := eval.evaluateVacuum(context.Background(), "inst-1", threshold, now)

	if result.Status != "not_needed" {
		t.Errorf("status = %q, want %q", result.Status, "not_needed")
	}
}

func TestEvaluateVacuum_InsufficientData(t *testing.T) {
	cfg := defaultTestCfg()
	now := time.Now()

	threshold := TableThresholds{
		Database:             "mydb",
		Table:                "orders",
		EffectiveVacuumLimit: 500,
	}

	// Only 2 data points — below min_data_points (3).
	points := []collector.MetricPoint{
		{InstanceID: "inst-1", Metric: "pg.db.vacuum.dead_tuples", Value: 100, Labels: map[string]string{"datname": "mydb", "relname": "orders"}, Timestamp: now.Add(-5 * time.Minute)},
		{InstanceID: "inst-1", Metric: "pg.db.vacuum.dead_tuples", Value: 200, Labels: map[string]string{"datname": "mydb", "relname": "orders"}, Timestamp: now},
	}

	store := &mockMetricStore{
		points: map[string][]collector.MetricPoint{
			"pg.db.vacuum.dead_tuples": points,
		},
	}

	eval := NewNeedEvaluator(store, &NullForecastStore{}, nil, &mockInstanceLister{}, &mockThresholdQuerier{}, cfg, slog.Default())
	result := eval.evaluateVacuum(context.Background(), "inst-1", threshold, now)

	if result.Status != "insufficient_data" {
		t.Errorf("status = %q, want %q", result.Status, "insufficient_data")
	}
}

// --- Tests for evaluateAnalyze ---

func TestEvaluateAnalyze_Predicted(t *testing.T) {
	cfg := defaultTestCfg()
	now := time.Now()

	threshold := TableThresholds{
		Database:              "mydb",
		Table:                 "orders",
		RelTuples:             10000,
		EffectiveAnalyzeLimit: 1050, // 50 + 0.1 * 10000
	}

	// mod_since_analyze rising: 100 → 250 → 400 → 550 over 15 minutes.
	// Rate ~0.5/s. Remaining = 1050-550 = 500. time_to = 500/0.5 = 1000s < 3600 → "imminent"
	// Make it slower: 100 → 150 → 200 → 250.
	// Rate = 150 / (3*300) = 150/900 = 0.167/s. Remaining = 1050-250 = 800. time_to = 800/0.167 ≈ 4800s > 3600 → "predicted"
	points := make([]collector.MetricPoint, 4)
	for i := 0; i < 4; i++ {
		points[i] = collector.MetricPoint{
			InstanceID: "inst-1",
			Metric:     "pg.db.vacuum.mod_since_analyze",
			Value:      100 + float64(i*50),
			Labels:     map[string]string{"datname": "mydb", "relname": "orders"},
			Timestamp:  now.Add(time.Duration(-3+i) * 5 * time.Minute),
		}
	}

	store := &mockMetricStore{
		points: map[string][]collector.MetricPoint{
			"pg.db.vacuum.mod_since_analyze": points,
		},
	}

	eval := NewNeedEvaluator(store, &NullForecastStore{}, nil, &mockInstanceLister{}, &mockThresholdQuerier{}, cfg, slog.Default())
	result := eval.evaluateAnalyze(context.Background(), "inst-1", threshold, now)

	if result.Status != "predicted" {
		t.Errorf("status = %q, want %q", result.Status, "predicted")
	}
	if result.Operation != "analyze" {
		t.Errorf("operation = %q, want %q", result.Operation, "analyze")
	}
	if result.AccumulationRate <= 0 {
		t.Errorf("accumulation_rate = %f, want > 0", result.AccumulationRate)
	}
}

// --- Tests for evaluateReindexBatch ---

func TestEvaluateReindex_OverThreshold(t *testing.T) {
	cfg := defaultTestCfg()
	now := time.Now()

	bloatPoints := []collector.MetricPoint{
		{
			InstanceID: "inst-1",
			Metric:     "pg.db.bloat.table_ratio",
			Value:      0.55, // 55% bloat — above 40% threshold
			Labels:     map[string]string{"datname": "mydb", "relname": "orders"},
			Timestamp:  now,
		},
	}

	store := &mockMetricStore{
		points: map[string][]collector.MetricPoint{
			"pg.db.bloat.table_ratio": bloatPoints,
		},
	}

	eval := NewNeedEvaluator(store, &NullForecastStore{}, nil, &mockInstanceLister{}, &mockThresholdQuerier{}, cfg, slog.Default())
	results := eval.evaluateReindexBatch(context.Background(), "inst-1", now)

	if len(results) != 1 {
		t.Fatalf("len = %d, want 1", len(results))
	}
	if results[0].Status != "overdue" {
		t.Errorf("status = %q, want %q", results[0].Status, "overdue")
	}
	if results[0].Operation != "reindex" {
		t.Errorf("operation = %q, want %q", results[0].Operation, "reindex")
	}
}

func TestEvaluateReindex_BelowThreshold(t *testing.T) {
	cfg := defaultTestCfg()
	now := time.Now()

	bloatPoints := []collector.MetricPoint{
		{
			InstanceID: "inst-1",
			Metric:     "pg.db.bloat.table_ratio",
			Value:      0.15, // 15% bloat — below 40% threshold and below 80% of threshold
			Labels:     map[string]string{"datname": "mydb", "relname": "orders"},
			Timestamp:  now,
		},
	}

	store := &mockMetricStore{
		points: map[string][]collector.MetricPoint{
			"pg.db.bloat.table_ratio": bloatPoints,
		},
	}

	eval := NewNeedEvaluator(store, &NullForecastStore{}, nil, &mockInstanceLister{}, &mockThresholdQuerier{}, cfg, slog.Default())
	results := eval.evaluateReindexBatch(context.Background(), "inst-1", now)

	if len(results) != 1 {
		t.Fatalf("len = %d, want 1", len(results))
	}
	if results[0].Status != "not_needed" {
		t.Errorf("status = %q, want %q", results[0].Status, "not_needed")
	}
}

// --- Tests for evaluateBasebackup ---

func TestEvaluateBasebackup_DisabledWhenIntervalZero(t *testing.T) {
	cfg := defaultTestCfg()
	cfg.BasebackupInterval = 0 // disabled

	eval := NewNeedEvaluator(&mockMetricStore{}, &NullForecastStore{}, nil, &mockInstanceLister{ids: []string{"inst-1"}}, &mockThresholdQuerier{}, cfg, slog.Default())

	// evaluateBasebackup should not be called when interval is 0
	// (verified by evaluateInstance skipping it), but if called directly:
	result := eval.evaluateBasebackup(context.Background(), "inst-1", time.Now())

	if result.Status != "insufficient_data" {
		t.Errorf("status = %q, want %q", result.Status, "insufficient_data")
	}
	if result.Database != "" {
		t.Errorf("database = %q, want empty (C5)", result.Database)
	}
	if result.Table != "" {
		t.Errorf("table = %q, want empty (C5)", result.Table)
	}
}

// --- Tests for full evaluation cycle ---

func TestEvaluateInstance_Integration(t *testing.T) {
	cfg := defaultTestCfg()
	cfg.BasebackupInterval = 0 // disabled
	now := time.Now()

	threshold := TableThresholds{
		Database:              "mydb",
		Schema:                "public",
		Table:                 "orders",
		RelTuples:             10000,
		AutovacuumEnabled:     true,
		VacuumThreshold:       50,
		VacuumScaleFactor:     0.2,
		AnalyzeThreshold:      50,
		AnalyzeScaleFactor:    0.1,
		EffectiveVacuumLimit:  2050,
		EffectiveAnalyzeLimit: 1050,
	}

	deadTuplePoints := make([]collector.MetricPoint, 5)
	modPoints := make([]collector.MetricPoint, 5)
	for i := 0; i < 5; i++ {
		ts := now.Add(time.Duration(-4+i) * 5 * time.Minute)
		deadTuplePoints[i] = collector.MetricPoint{
			InstanceID: "inst-1", Metric: "pg.db.vacuum.dead_tuples",
			Value: 100 + float64(i*80), Labels: map[string]string{"datname": "mydb", "relname": "orders"},
			Timestamp: ts,
		}
		modPoints[i] = collector.MetricPoint{
			InstanceID: "inst-1", Metric: "pg.db.vacuum.mod_since_analyze",
			Value: 50 + float64(i*30), Labels: map[string]string{"datname": "mydb", "relname": "orders"},
			Timestamp: ts,
		}
	}

	forecastStore := &NullForecastStore{}
	metricStore := &mockMetricStore{
		points: map[string][]collector.MetricPoint{
			"pg.db.vacuum.dead_tuples":       deadTuplePoints,
			"pg.db.vacuum.mod_since_analyze": modPoints,
			"pg.db.bloat.table_ratio":        {},
		},
	}

	eval := NewNeedEvaluator(
		metricStore, forecastStore, nil,
		&mockInstanceLister{ids: []string{"inst-1"}},
		&mockThresholdQuerier{thresholds: []TableThresholds{threshold}},
		cfg, slog.Default(),
	)

	// Run one evaluation cycle directly.
	eval.evaluateInstance(context.Background(), "inst-1")
	// No assertions on store writes (NullForecastStore) — just verifying no panic/crash.
}

func TestComputeRate_PositiveSlope(t *testing.T) {
	cfg := defaultTestCfg()
	eval := NewNeedEvaluator(&mockMetricStore{}, &NullForecastStore{}, nil, &mockInstanceLister{}, &mockThresholdQuerier{}, cfg, slog.Default())

	now := time.Now()
	points := []collector.MetricPoint{
		{Value: 100, Timestamp: now.Add(-600 * time.Second)},
		{Value: 200, Timestamp: now.Add(-300 * time.Second)},
		{Value: 300, Timestamp: now},
	}

	rate := eval.computeRate(points)
	// Rate ≈ 200/600 ≈ 0.333/s
	if rate < 0.3 || rate > 0.4 {
		t.Errorf("rate = %f, want ~0.333", rate)
	}
}

func TestComputeRate_NegativeSlope(t *testing.T) {
	cfg := defaultTestCfg()
	eval := NewNeedEvaluator(&mockMetricStore{}, &NullForecastStore{}, nil, &mockInstanceLister{}, &mockThresholdQuerier{}, cfg, slog.Default())

	now := time.Now()
	points := []collector.MetricPoint{
		{Value: 300, Timestamp: now.Add(-600 * time.Second)},
		{Value: 200, Timestamp: now.Add(-300 * time.Second)},
		{Value: 100, Timestamp: now},
	}

	rate := eval.computeRate(points)
	if rate >= 0 {
		t.Errorf("rate = %f, want < 0 for decreasing values", rate)
	}
}

func TestComputeRate_SinglePoint(t *testing.T) {
	cfg := defaultTestCfg()
	eval := NewNeedEvaluator(&mockMetricStore{}, &NullForecastStore{}, nil, &mockInstanceLister{}, &mockThresholdQuerier{}, cfg, slog.Default())

	rate := eval.computeRate([]collector.MetricPoint{
		{Value: 100, Timestamp: time.Now()},
	})
	if rate != 0 {
		t.Errorf("rate = %f, want 0 for single point", rate)
	}
}
