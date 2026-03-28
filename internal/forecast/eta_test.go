package forecast

import (
	"log/slog"
	"testing"
	"time"
)

func newTestETACalc(tracker *OperationTracker) *ETACalculator {
	return NewETACalculator(tracker, ETAConfig{
		WindowSize:  10,
		DecayFactor: 0.85,
		MinSamples:  4,
	}, slog.Default())
}

func TestETACalculator_Estimating(t *testing.T) {
	tracker := &OperationTracker{
		activeOps: map[string]*TrackedOperation{
			"inst:1:vacuum": {
				InstanceID: "inst",
				PID:        1,
				Operation:  "vacuum",
				StartedAt:  time.Now().Add(-30 * time.Second),
				Samples: []ProgressSample{
					{Timestamp: time.Now().Add(-20 * time.Second), WorkDone: 100, PctDone: 10},
					{Timestamp: time.Now().Add(-10 * time.Second), WorkDone: 200, PctDone: 20},
				},
			},
		},
	}

	calc := newTestETACalc(tracker)
	etas := calc.ComputeAll("inst")
	if len(etas) != 1 {
		t.Fatalf("expected 1 ETA, got %d", len(etas))
	}
	eta := etas[0]
	if eta.Confidence != "estimating" {
		t.Errorf("expected 'estimating', got %q", eta.Confidence)
	}
	if eta.ETASec != -1 {
		t.Errorf("expected ETASec=-1, got %f", eta.ETASec)
	}
}

func TestETACalculator_HighConfidence(t *testing.T) {
	now := time.Now()
	samples := make([]ProgressSample, 10)
	for i := 0; i < 10; i++ {
		samples[i] = ProgressSample{
			Timestamp: now.Add(time.Duration(i*10) * time.Second),
			WorkDone:  float64(i * 100),
			WorkTotal: 1000,
			PctDone:   float64(i * 10),
		}
	}

	tracker := &OperationTracker{
		activeOps: map[string]*TrackedOperation{
			"inst:1:vacuum": {
				InstanceID: "inst",
				PID:        1,
				Operation:  "vacuum",
				StartedAt:  now,
				Samples:    samples,
			},
		},
	}

	calc := newTestETACalc(tracker)
	etas := calc.ComputeAll("inst")
	if len(etas) != 1 {
		t.Fatalf("expected 1 ETA, got %d", len(etas))
	}
	eta := etas[0]
	if eta.Confidence != "high" {
		t.Errorf("expected 'high', got %q", eta.Confidence)
	}
	if eta.ETASec <= 0 {
		t.Errorf("expected positive ETASec, got %f", eta.ETASec)
	}
	if eta.RateCurrent <= 0 {
		t.Errorf("expected positive rate, got %f", eta.RateCurrent)
	}
}

func TestETACalculator_Stalled(t *testing.T) {
	now := time.Now()
	samples := make([]ProgressSample, 5)
	for i := 0; i < 5; i++ {
		samples[i] = ProgressSample{
			Timestamp: now.Add(time.Duration(i*10) * time.Second),
			WorkDone:  500, // no progress
			WorkTotal: 1000,
			PctDone:   50,
		}
	}

	tracker := &OperationTracker{
		activeOps: map[string]*TrackedOperation{
			"inst:2:analyze": {
				InstanceID: "inst",
				PID:        2,
				Operation:  "analyze",
				StartedAt:  now,
				Samples:    samples,
			},
		},
	}

	calc := newTestETACalc(tracker)
	eta := calc.ComputeByPID("inst", 2)
	if eta == nil {
		t.Fatal("expected ETA, got nil")
	}
	if eta.Confidence != "stalled" {
		t.Errorf("expected 'stalled', got %q", eta.Confidence)
	}
	if eta.ETASec != -1 {
		t.Errorf("expected ETASec=-1, got %f", eta.ETASec)
	}
}

func TestETACalculator_NoPIDFound(t *testing.T) {
	tracker := &OperationTracker{
		activeOps: map[string]*TrackedOperation{},
	}
	calc := newTestETACalc(tracker)
	eta := calc.ComputeByPID("inst", 999)
	if eta != nil {
		t.Errorf("expected nil ETA for unknown PID, got %+v", eta)
	}
}

func TestClassifyConfidence(t *testing.T) {
	tests := []struct {
		count int
		want  string
	}{
		{10, "high"},
		{8, "high"},
		{7, "medium"},
		{4, "medium"},
	}
	for _, tt := range tests {
		if got := classifyConfidence(tt.count); got != tt.want {
			t.Errorf("classifyConfidence(%d) = %q, want %q", tt.count, got, tt.want)
		}
	}
}
