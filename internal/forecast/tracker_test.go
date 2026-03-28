package forecast

import (
	"testing"
	"time"
)

func TestAppendSample_RingBuffer(t *testing.T) {
	op := &TrackedOperation{}
	for i := 0; i < 15; i++ {
		appendSample(op, ProgressSample{PctDone: float64(i)}, 10)
	}
	if len(op.Samples) != 10 {
		t.Fatalf("expected 10 samples, got %d", len(op.Samples))
	}
	// Oldest should be 5 (0-4 evicted).
	if op.Samples[0].PctDone != 5 {
		t.Errorf("expected oldest sample PctDone=5, got %f", op.Samples[0].PctDone)
	}
	if op.Samples[9].PctDone != 14 {
		t.Errorf("expected newest sample PctDone=14, got %f", op.Samples[9].PctDone)
	}
}

func TestClassifyOutcome_Completed(t *testing.T) {
	tracker := &OperationTracker{}
	op := &TrackedOperation{
		StartedAt:  time.Now().Add(-5 * time.Minute),
		LastSeenAt: time.Now(),
		Samples:    []ProgressSample{{PctDone: 100.0}},
	}
	if outcome := tracker.classifyOutcome(op); outcome != "completed" {
		t.Errorf("expected 'completed', got %q", outcome)
	}
}

func TestClassifyOutcome_Disappeared(t *testing.T) {
	tracker := &OperationTracker{}
	op := &TrackedOperation{
		StartedAt:  time.Now().Add(-5 * time.Minute),
		LastSeenAt: time.Now(),
		Samples:    []ProgressSample{{PctDone: 45.0}},
	}
	if outcome := tracker.classifyOutcome(op); outcome != "disappeared" {
		t.Errorf("expected 'disappeared', got %q", outcome)
	}
}

func TestClassifyOutcome_Unknown(t *testing.T) {
	tracker := &OperationTracker{}
	op := &TrackedOperation{
		StartedAt:  time.Now(),
		LastSeenAt: time.Now().Add(1 * time.Second),
		Samples:    []ProgressSample{{PctDone: 5.0}},
	}
	if outcome := tracker.classifyOutcome(op); outcome != "unknown" {
		t.Errorf("expected 'unknown', got %q", outcome)
	}
}

func TestClassifyOutcome_EmptySamples(t *testing.T) {
	tracker := &OperationTracker{}
	op := &TrackedOperation{}
	if outcome := tracker.classifyOutcome(op); outcome != "unknown" {
		t.Errorf("expected 'unknown', got %q", outcome)
	}
}

func TestAtoi(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"123", 123},
		{"0", 0},
		{"", 0},
		{"42abc", 42},
	}
	for _, tt := range tests {
		if got := atoi(tt.input); got != tt.want {
			t.Errorf("atoi(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestDebounce_SingleMissDoesNotFinalize(t *testing.T) {
	op := &TrackedOperation{
		InstanceID: "test",
		PID:        100,
		Operation:  "vacuum",
		MissedPolls: 0,
		Samples:     []ProgressSample{{PctDone: 50.0}},
	}
	// Simulate one missed poll.
	op.MissedPolls++
	if op.MissedPolls >= 2 {
		t.Fatal("operation should NOT be finalized after 1 missed poll")
	}

	// Simulate second missed poll.
	op.MissedPolls++
	if op.MissedPolls < 2 {
		t.Fatal("operation SHOULD be finalized after 2 missed polls")
	}
}
