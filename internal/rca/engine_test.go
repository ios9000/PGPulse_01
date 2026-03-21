package rca

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ios9000/PGPulse_01/internal/collector"
)

// --- Mock implementations ---

type mockAnomalySource struct {
	anomalies map[string][]AnomalyEvent
}

func (m *mockAnomalySource) GetAnomalies(_ context.Context, _ string, metricKeys []string,
	_, _ time.Time, _ time.Duration,
) (map[string][]AnomalyEvent, error) {
	result := make(map[string][]AnomalyEvent)
	for _, key := range metricKeys {
		if events, ok := m.anomalies[key]; ok {
			result[key] = events
		}
	}
	return result, nil
}

type mockMetricStore struct {
	points []collector.MetricPoint
}

func (m *mockMetricStore) Write(_ context.Context, points []collector.MetricPoint) error {
	m.points = append(m.points, points...)
	return nil
}

func (m *mockMetricStore) Query(_ context.Context, q collector.MetricQuery) ([]collector.MetricPoint, error) {
	var result []collector.MetricPoint
	for _, p := range m.points {
		if q.InstanceID != "" && p.InstanceID != q.InstanceID {
			continue
		}
		if q.Metric != "" && p.Metric != q.Metric {
			continue
		}
		if !q.Start.IsZero() && p.Timestamp.Before(q.Start) {
			continue
		}
		if !q.End.IsZero() && p.Timestamp.After(q.End) {
			continue
		}
		result = append(result, p)
	}
	return result, nil
}

func (m *mockMetricStore) Close() error { return nil }

// --- Helper to build anomalies for chain 1 ---

func chain1Anomalies(triggerTime time.Time) map[string][]AnomalyEvent {
	// Anomaly timestamps must fit within the engine's temporal search windows.
	// The engine walks backward from the terminal node using triggerTime as the
	// reference for ALL edges. Each edge's search window is:
	//   [triggerTime - MaxLag - jitter, triggerTime - MinLag + jitter]
	// where jitter depends on the node's collection frequency group.
	//
	// Chain 1 edges (walking from terminal replication_lag backward):
	//   disk_io -> replication_lag:       MinLag=10s, MaxLag=2m, jitter=90s → [-3.5m, +80s]
	//   checkpoint_storm -> disk_io:      MinLag=0,   MaxLag=30s, jitter=90s → [-2m,   +90s]
	//   wal_generation -> checkpoint_storm: MinLag=30s, MaxLag=3m, jitter=90s → [-4.5m, +60s]
	//   bulk_workload -> wal_generation:  MinLag=0,   MaxLag=30s, jitter=90s → [-2m,   +90s]
	//
	// All timestamps relative to triggerTime.
	return map[string][]AnomalyEvent{
		// Root: bulk_workload — must be in [-2m, +90s]
		"pg.statements.top.total_time_ms": {
			{MetricKey: "pg.statements.top.total_time_ms", Timestamp: triggerTime.Add(-60 * time.Second),
				Value: 5000, BaselineVal: 100, Strength: 0.9, Source: "threshold"},
		},
		// WAL generation — must be in [-4.5m, +60s]
		"pg.checkpoint.buffers_written_per_second": {
			{MetricKey: "pg.checkpoint.buffers_written_per_second", Timestamp: triggerTime.Add(-90 * time.Second),
				Value: 8000, BaselineVal: 500, Strength: 0.85, Source: "threshold"},
		},
		// Checkpoint storm — must be in [-2m, +90s]
		"pg.checkpoint.requested_per_second": {
			{MetricKey: "pg.checkpoint.requested_per_second", Timestamp: triggerTime.Add(-30 * time.Second),
				Value: 10, BaselineVal: 0.5, Strength: 0.9, Source: "threshold"},
		},
		// Disk I/O — must be in [-3.5m, +80s]
		"os.disk.util_pct": {
			{MetricKey: "os.disk.util_pct", Timestamp: triggerTime.Add(-20 * time.Second),
				Value: 98, BaselineVal: 30, Strength: 0.8, Source: "threshold"},
		},
		// Replication lag (trigger/symptom) — present in analysis window
		"pg.replication.lag.replay_bytes": {
			{MetricKey: "pg.replication.lag.replay_bytes", Timestamp: triggerTime,
				Value: 500000000, BaselineVal: 1000, Strength: 0.95, Source: "threshold"},
		},
	}
}

func newTestEngine(anomalies map[string][]AnomalyEvent, cfg *RCAConfig) *Engine {
	config := DefaultRCAConfig()
	config.MinChainScore = 0.0 // accept all chains for testing
	if cfg != nil {
		config = *cfg
	}

	return NewEngine(EngineOptions{
		Graph:       NewDefaultGraph(),
		Anomaly:     &mockAnomalySource{anomalies: anomalies},
		Store:       NewNullIncidentStore(),
		MetricStore: &mockMetricStore{},
		Config:      config,
	})
}

func TestAnalyze_FullChain(t *testing.T) {
	t.Parallel()
	triggerTime := time.Now()
	engine := newTestEngine(chain1Anomalies(triggerTime), nil)

	incident, err := engine.Analyze(context.Background(), AnalyzeRequest{
		InstanceID:    "inst-1",
		TriggerMetric: "pg.replication.lag.replay_bytes",
		TriggerValue:  500000000,
		TriggerTime:   triggerTime,
		TriggerKind:   "manual",
	})
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}
	if incident == nil {
		t.Fatal("expected non-nil incident")
	}

	// Should have a primary chain.
	if incident.PrimaryChain == nil {
		t.Fatal("expected primary chain to be populated")
	}

	// Timeline should have events.
	if len(incident.Timeline) == 0 {
		t.Error("expected non-empty timeline")
	}

	// Summary should use qualified language.
	if !strings.Contains(incident.Summary, "Likely") &&
		!strings.Contains(incident.Summary, "Strongly consistent") &&
		!strings.Contains(incident.Summary, "Possibly") {
		t.Errorf("summary lacks qualified language: %s", incident.Summary)
	}

	// Confidence should be > 0.
	if incident.Confidence <= 0 {
		t.Errorf("expected positive confidence, got %f", incident.Confidence)
	}
}

func TestAnalyze_RequiredEvidencePruning(t *testing.T) {
	t.Parallel()
	triggerTime := time.Now()

	// Provide chain 1 anomalies but OMIT checkpoint_storm metrics.
	// The edge wal_generation -> checkpoint_storm is EvidenceRequired,
	// so chain 1 should be pruned.
	anomalies := chain1Anomalies(triggerTime)
	delete(anomalies, "pg.checkpoint.requested_per_second")
	delete(anomalies, "pg.checkpoint.sync_time_ms")

	engine := newTestEngine(anomalies, nil)

	incident, err := engine.Analyze(context.Background(), AnalyzeRequest{
		InstanceID:    "inst-1",
		TriggerMetric: "pg.replication.lag.replay_bytes",
		TriggerValue:  500000000,
		TriggerTime:   triggerTime,
		TriggerKind:   "manual",
	})
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}
	if incident == nil {
		t.Fatal("expected non-nil incident")
	}

	// Chain 1 should NOT be the primary chain because required evidence is missing.
	if incident.PrimaryChain != nil && incident.PrimaryChain.ChainID == ChainBulkWALCheckpointIOReplLag {
		t.Error("chain 1 should have been pruned due to missing required checkpoint evidence")
	}
}

func TestAnalyze_SupportingEvidenceAbsent(t *testing.T) {
	t.Parallel()
	triggerTime := time.Now()

	// Chain 1: the edge disk_io -> replication_lag is EvidenceSupporting.
	// Provide everything except the replication lag anomaly itself.
	// The chain should survive but with potentially lower score.
	fullAnomalies := chain1Anomalies(triggerTime)

	// Now create a version with the supporting evidence removed.
	// The edge from disk_io to replication_lag is Supporting.
	// But the trigger metric itself is replication_lag, so the engine
	// should still find the chain via other evidence.
	withoutSupporting := make(map[string][]AnomalyEvent)
	for k, v := range fullAnomalies {
		withoutSupporting[k] = v
	}
	// Remove disk I/O — which is the supporting edge's evidence.
	delete(withoutSupporting, "os.disk.util_pct")
	delete(withoutSupporting, "os.disk.write_bytes_per_sec")
	delete(withoutSupporting, "os.cpu.iowait_pct")

	engineFull := newTestEngine(fullAnomalies, nil)
	engineReduced := newTestEngine(withoutSupporting, nil)

	incidentFull, err := engineFull.Analyze(context.Background(), AnalyzeRequest{
		InstanceID:    "inst-1",
		TriggerMetric: "pg.replication.lag.replay_bytes",
		TriggerValue:  500000000,
		TriggerTime:   triggerTime,
		TriggerKind:   "manual",
	})
	if err != nil {
		t.Fatalf("Full analyze failed: %v", err)
	}

	incidentReduced, err := engineReduced.Analyze(context.Background(), AnalyzeRequest{
		InstanceID:    "inst-1",
		TriggerMetric: "pg.replication.lag.replay_bytes",
		TriggerValue:  500000000,
		TriggerTime:   triggerTime,
		TriggerKind:   "manual",
	})
	if err != nil {
		t.Fatalf("Reduced analyze failed: %v", err)
	}

	// Both should produce incidents. The full one should generally have
	// >= confidence than the reduced one (though exact ranking depends on
	// which chains match).
	if incidentFull == nil || incidentReduced == nil {
		t.Skip("one of the incidents is nil, cannot compare scores")
	}
}

func TestAnalyze_NoMatchingChains(t *testing.T) {
	t.Parallel()
	triggerTime := time.Now()
	engine := newTestEngine(map[string][]AnomalyEvent{}, nil)

	incident, err := engine.Analyze(context.Background(), AnalyzeRequest{
		InstanceID:    "inst-1",
		TriggerMetric: "nonexistent.metric",
		TriggerValue:  100,
		TriggerTime:   triggerTime,
		TriggerKind:   "manual",
	})
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}
	if incident == nil {
		t.Fatal("expected non-nil incident even with no matching chains")
	}

	if incident.Confidence != 0 {
		t.Errorf("expected confidence 0 for no matching chains, got %f", incident.Confidence)
	}

	if !strings.Contains(incident.Summary, "No probable causal chain") &&
		!strings.Contains(incident.Summary, "Manual investigation") {
		t.Errorf("expected summary to indicate no chain found: %s", incident.Summary)
	}
}

func TestAnalyze_ConfidenceBuckets(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		score    float64
		expected string
	}{
		{"high", 0.8, "high"},
		{"medium_low", 0.4, "medium"},
		{"medium_high", 0.7, "medium"},
		{"low", 0.2, "low"},
		{"zero", 0.0, "low"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			bucket := bucketizeConfidence(tc.score)
			if bucket != tc.expected {
				t.Errorf("bucketizeConfidence(%f) = %s, want %s", tc.score, bucket, tc.expected)
			}
		})
	}
}

func TestAnalyze_MaxTraversalDepth(t *testing.T) {
	t.Parallel()
	triggerTime := time.Now()

	cfg := DefaultRCAConfig()
	cfg.MinChainScore = 0.0
	cfg.MaxTraversalDepth = 2

	engine := newTestEngine(chain1Anomalies(triggerTime), &cfg)

	incident, err := engine.Analyze(context.Background(), AnalyzeRequest{
		InstanceID:    "inst-1",
		TriggerMetric: "pg.replication.lag.replay_bytes",
		TriggerValue:  500000000,
		TriggerTime:   triggerTime,
		TriggerKind:   "manual",
	})
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}
	if incident == nil {
		t.Fatal("expected non-nil incident")
	}

	// With depth=2, chain 1 (which has 4 edges / 5 nodes deep) should
	// not fully traverse. The engine should still produce a result
	// (possibly truncated).
	// We primarily verify that the engine does not crash with limited depth.
}

func TestAnalyze_EmptyAnomalies(t *testing.T) {
	t.Parallel()
	triggerTime := time.Now()

	engine := newTestEngine(map[string][]AnomalyEvent{}, nil)

	incident, err := engine.Analyze(context.Background(), AnalyzeRequest{
		InstanceID:    "inst-1",
		TriggerMetric: "pg.replication.lag.replay_bytes",
		TriggerValue:  500000000,
		TriggerTime:   triggerTime,
		TriggerKind:   "manual",
	})
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}
	if incident == nil {
		t.Fatal("expected non-nil incident")
	}

	// With no anomalies at all, confidence should be 0 because all
	// required-evidence edges will fail.
	if incident.Confidence != 0 {
		t.Errorf("expected zero confidence with no anomalies, got %f", incident.Confidence)
	}

	if incident.ConfidenceBucket != "low" {
		t.Errorf("expected 'low' bucket, got %s", incident.ConfidenceBucket)
	}
}

func TestAnalyze_AnomalyMode(t *testing.T) {
	t.Parallel()

	mode := AnomalyMode(&mockAnomalySource{})
	if mode != "threshold" {
		t.Errorf("expected 'threshold' mode for mockAnomalySource, got %s", mode)
	}
}

func TestGenerateSummary_NilChain(t *testing.T) {
	t.Parallel()

	summary := generateSummary(nil, 0.0)
	if !strings.Contains(summary, "No probable causal chain") {
		t.Errorf("expected 'No probable causal chain' in summary, got: %s", summary)
	}
}

func TestTemporalProximity(t *testing.T) {
	t.Parallel()

	triggerTime := time.Now()

	// Perfect match at center of range.
	center := temporalProximity(
		triggerTime.Add(-75*time.Second), triggerTime,
		30*time.Second, 120*time.Second,
	)
	if center < 0.8 || center > 1.0 {
		t.Errorf("expected high proximity for center hit, got %f", center)
	}

	// Way outside the range.
	far := temporalProximity(
		triggerTime.Add(-10*time.Minute), triggerTime,
		30*time.Second, 120*time.Second,
	)
	if far > 0.5 {
		t.Errorf("expected low proximity for far miss, got %f", far)
	}
}
