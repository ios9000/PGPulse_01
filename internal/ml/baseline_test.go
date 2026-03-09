package ml

import (
	"math"
	"testing"
)

func TestSTLBaseline_NeedsMinPoints(t *testing.T) {
	b := NewSTLBaseline("test", 10)
	// Feed fewer than Period*2 (20) points
	for i := 0; i < 19; i++ {
		b.Update(100)
	}
	if b.Ready() {
		t.Error("should not be ready with 19 points")
	}
	zScore, isIQR := b.Score(100)
	if zScore != 0 || isIQR {
		t.Errorf("expected (0, false) with insufficient points, got (%f, %v)", zScore, isIQR)
	}
}

func TestSTLBaseline_Ready(t *testing.T) {
	b := NewSTLBaseline("test", 10)
	if b.Ready() {
		t.Error("should not be ready with 0 points")
	}
	for i := 0; i < 19; i++ {
		b.Update(100)
	}
	if b.Ready() {
		t.Error("should not be ready with 19 points (need 20)")
	}
	b.Update(100)
	if !b.Ready() {
		t.Error("should be ready with 20 points")
	}
}

func TestSTLBaseline_StableSignal(t *testing.T) {
	b := NewSTLBaseline("test", 10)
	// Feed constant value -- residuals should be near 0
	for i := 0; i < 100; i++ {
		b.Update(50.0)
	}
	zScore, isIQR := b.Score(50.0)
	if math.Abs(zScore) > 0.5 {
		t.Errorf("stable signal should have near-zero z-score, got %f", zScore)
	}
	if isIQR {
		t.Error("stable signal should not be IQR outlier")
	}
}

func TestSTLBaseline_DetectsOutlier(t *testing.T) {
	b := NewSTLBaseline("test", 10)
	// Feed 100 points with mean=100, small variation
	for i := 0; i < 100; i++ {
		b.Update(100.0 + float64(i%3)) // values: 100, 101, 102, 100, 101, ...
	}
	// Score a massive outlier
	zScore, _ := b.Score(500.0)
	if math.Abs(zScore) < 3.0 {
		t.Errorf("outlier should have |z-score| > 3, got %f", zScore)
	}
}

func TestSTLBaseline_ResidualStddev_Empty(t *testing.T) {
	b := NewSTLBaseline("test", 10)
	if b.ResidualStddev() != 0 {
		t.Error("empty baseline should have 0 stddev")
	}
}

func TestSTLBaseline_ResidualStddev_SinglePoint(t *testing.T) {
	b := NewSTLBaseline("test", 10)
	b.Update(42.0)
	if b.ResidualStddev() != 0 {
		t.Error("single-point baseline should have 0 stddev")
	}
}

func TestSTLBaseline_ResidualStddev_Positive(t *testing.T) {
	b := NewSTLBaseline("test", 10)
	for i := 0; i < 50; i++ {
		b.Update(float64(i % 5)) // 0,1,2,3,4,0,1,2,3,4,...
	}
	stddev := b.ResidualStddev()
	if stddev <= 0 {
		t.Errorf("expected positive stddev, got %f", stddev)
	}
}

func TestSTLBaseline_SeasonalPattern(t *testing.T) {
	b := NewSTLBaseline("test", 10)
	// Feed a clear seasonal pattern with many cycles so EWMA stabilizes
	for cycle := 0; cycle < 100; cycle++ {
		for i := 0; i < 10; i++ {
			b.Update(float64(i * 10))
		}
	}
	// Score a value that matches the pattern mean (45) -- should not be extreme
	// The EWMA-based decomposition won't perfectly capture seasonality,
	// so we just verify that the mean value gets a lower z-score than an extreme outlier.
	zMean, _ := b.Score(45.0)
	zOutlier, _ := b.Score(9999.0)
	if math.Abs(zMean) >= math.Abs(zOutlier) {
		t.Errorf("mean value z-score (%f) should be less extreme than outlier z-score (%f)",
			zMean, zOutlier)
	}
}

func TestNewSTLBaseline_Fields(t *testing.T) {
	b := NewSTLBaseline("my.metric", 24)
	if b.MetricKey != "my.metric" {
		t.Errorf("MetricKey = %q, want %q", b.MetricKey, "my.metric")
	}
	if b.Period != 24 {
		t.Errorf("Period = %d, want 24", b.Period)
	}
	if len(b.seasonal) != 24 {
		t.Errorf("seasonal len = %d, want 24", len(b.seasonal))
	}
	if len(b.seasonN) != 24 {
		t.Errorf("seasonN len = %d, want 24", len(b.seasonN))
	}
}

func TestSTLBaseline_WindowSize(t *testing.T) {
	// period=10 -> windowSize = max(30, 1000) = 1000
	b := NewSTLBaseline("test", 10)
	if b.windowSize != 1000 {
		t.Errorf("windowSize = %d, want 1000 for period=10", b.windowSize)
	}
	// period=500 -> windowSize = max(1500, 1000) = 1500
	b2 := NewSTLBaseline("test", 500)
	if b2.windowSize != 1500 {
		t.Errorf("windowSize = %d, want 1500 for period=500", b2.windowSize)
	}
}
