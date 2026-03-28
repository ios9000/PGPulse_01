package ml

import (
	"math"
	"testing"
)

func TestLinearRegression_PositiveSlope(t *testing.T) {
	xs := []float64{1, 2, 3, 4, 5}
	ys := []float64{2, 4, 6, 8, 10}
	res, err := LinearRegression(xs, ys)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(res.Slope-2.0) > 1e-9 {
		t.Errorf("expected slope=2, got %f", res.Slope)
	}
	if math.Abs(res.Intercept-0.0) > 1e-9 {
		t.Errorf("expected intercept=0, got %f", res.Intercept)
	}
	if math.Abs(res.RSquared-1.0) > 1e-9 {
		t.Errorf("expected R²=1, got %f", res.RSquared)
	}
	if res.N != 5 {
		t.Errorf("expected N=5, got %d", res.N)
	}
}

func TestLinearRegression_ZeroSlope(t *testing.T) {
	xs := []float64{1, 2, 3, 4}
	ys := []float64{5, 5, 5, 5}
	res, err := LinearRegression(xs, ys)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(res.Slope) > 1e-9 {
		t.Errorf("expected slope=0, got %f", res.Slope)
	}
	if math.Abs(res.Intercept-5.0) > 1e-9 {
		t.Errorf("expected intercept=5, got %f", res.Intercept)
	}
}

func TestLinearRegression_NegativeSlope(t *testing.T) {
	xs := []float64{0, 1, 2, 3}
	ys := []float64{10, 7, 4, 1}
	res, err := LinearRegression(xs, ys)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(res.Slope-(-3.0)) > 1e-9 {
		t.Errorf("expected slope=-3, got %f", res.Slope)
	}
	if math.Abs(res.RSquared-1.0) > 1e-9 {
		t.Errorf("expected R²=1 for perfect fit, got %f", res.RSquared)
	}
}

func TestLinearRegression_InsufficientData(t *testing.T) {
	_, err := LinearRegression([]float64{1}, []float64{2})
	if err == nil {
		t.Fatal("expected error for single data point")
	}
}

func TestLinearRegression_MismatchedLengths(t *testing.T) {
	_, err := LinearRegression([]float64{1, 2}, []float64{3})
	if err == nil {
		t.Fatal("expected error for mismatched lengths")
	}
}

func TestLinearRegression_SameXValues(t *testing.T) {
	xs := []float64{5, 5, 5}
	ys := []float64{1, 2, 3}
	res, err := LinearRegression(xs, ys)
	if err != nil {
		t.Fatal(err)
	}
	if res.Slope != 0 {
		t.Errorf("expected slope=0 for degenerate case, got %f", res.Slope)
	}
}

func TestLinearRegression_NoisyData(t *testing.T) {
	xs := []float64{1, 2, 3, 4, 5}
	ys := []float64{2.1, 3.9, 6.2, 7.8, 10.1}
	res, err := LinearRegression(xs, ys)
	if err != nil {
		t.Fatal(err)
	}
	if res.Slope < 1.5 || res.Slope > 2.5 {
		t.Errorf("expected slope near 2, got %f", res.Slope)
	}
	if res.RSquared < 0.95 {
		t.Errorf("expected R² > 0.95 for near-linear data, got %f", res.RSquared)
	}
}
