package ml

import (
	"math"
	"testing"
	"time"
)

func TestWMA_EvenSamples(t *testing.T) {
	now := time.Now()
	// 5 samples, 10 seconds apart, values increase by 100 each.
	// Rate should be ~10 units/sec.
	timestamps := make([]time.Time, 5)
	values := make([]float64, 5)
	for i := 0; i < 5; i++ {
		timestamps[i] = now.Add(time.Duration(i*10) * time.Second)
		values[i] = float64(i * 100)
	}

	cfg := WMAConfig{WindowSize: 10, DecayFactor: 0.85}
	res, err := WeightedMovingAverage(cfg, timestamps, values)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(res.WeightedRate-10.0) > 0.01 {
		t.Errorf("expected rate=10, got %f", res.WeightedRate)
	}
	if res.SampleCount != 5 {
		t.Errorf("expected sample_count=5, got %d", res.SampleCount)
	}
	// All rates are equal, so stddev should be 0.
	if res.StdDev > 0.01 {
		t.Errorf("expected stddev≈0 for constant rate, got %f", res.StdDev)
	}
}

func TestWMA_DecayCorrectness(t *testing.T) {
	now := time.Now()
	// 3 samples: rate changes from 1 to 10
	timestamps := []time.Time{
		now,
		now.Add(10 * time.Second),
		now.Add(20 * time.Second),
	}
	values := []float64{0, 10, 110} // rates: 1, 10

	cfg := WMAConfig{WindowSize: 10, DecayFactor: 0.5}
	res, err := WeightedMovingAverage(cfg, timestamps, values)
	if err != nil {
		t.Fatal(err)
	}
	// rates = [1.0, 10.0]
	// weights with decay=0.5: w[0]=0.5^1=0.5, w[1]=0.5^0=1.0
	// weighted_rate = (0.5*1 + 1.0*10) / (0.5+1.0) = 10.5/1.5 = 7.0
	if math.Abs(res.WeightedRate-7.0) > 0.01 {
		t.Errorf("expected rate=7.0, got %f", res.WeightedRate)
	}
}

func TestWMA_InsufficientData(t *testing.T) {
	_, err := WeightedMovingAverage(
		WMAConfig{WindowSize: 10, DecayFactor: 0.85},
		[]time.Time{time.Now()},
		[]float64{100},
	)
	if err == nil {
		t.Fatal("expected error for single data point")
	}
}

func TestWMA_WindowSizeTruncation(t *testing.T) {
	now := time.Now()
	timestamps := make([]time.Time, 20)
	values := make([]float64, 20)
	for i := 0; i < 20; i++ {
		timestamps[i] = now.Add(time.Duration(i*10) * time.Second)
		values[i] = float64(i * 50)
	}

	cfg := WMAConfig{WindowSize: 5, DecayFactor: 0.85}
	res, err := WeightedMovingAverage(cfg, timestamps, values)
	if err != nil {
		t.Fatal(err)
	}
	// With window=5, only last 5 samples are used. Rate is constant 5 units/sec.
	if math.Abs(res.WeightedRate-5.0) > 0.01 {
		t.Errorf("expected rate=5, got %f", res.WeightedRate)
	}
}

func TestWMA_ZeroTimeDelta(t *testing.T) {
	now := time.Now()
	timestamps := []time.Time{now, now, now.Add(10 * time.Second)}
	values := []float64{0, 50, 100}

	cfg := WMAConfig{WindowSize: 10, DecayFactor: 0.85}
	res, err := WeightedMovingAverage(cfg, timestamps, values)
	if err != nil {
		t.Fatal(err)
	}
	// Only 1 valid rate interval (50→100 in 10s = 5/s). Zero-delta pair is skipped.
	if math.Abs(res.WeightedRate-5.0) > 0.01 {
		t.Errorf("expected rate=5, got %f", res.WeightedRate)
	}
}
