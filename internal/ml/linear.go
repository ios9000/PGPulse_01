package ml

import (
	"errors"
	"math"
)

// LinearRegressionResult holds the output of a simple OLS regression.
type LinearRegressionResult struct {
	Slope     float64 // dy/dx (rate of change)
	Intercept float64
	RSquared  float64 // goodness of fit (0.0–1.0)
	N         int     // number of data points
}

// LinearRegression computes simple OLS regression over (x, y) pairs.
// Returns error if len(xs) < 2 or len(xs) != len(ys).
func LinearRegression(xs, ys []float64) (LinearRegressionResult, error) {
	n := len(xs)
	if n < 2 {
		return LinearRegressionResult{}, errors.New("ml: linear regression requires at least 2 data points")
	}
	if n != len(ys) {
		return LinearRegressionResult{}, errors.New("ml: xs and ys must have equal length")
	}

	var sumX, sumY, sumXY, sumX2 float64
	for i := 0; i < n; i++ {
		sumX += xs[i]
		sumY += ys[i]
		sumXY += xs[i] * ys[i]
		sumX2 += xs[i] * xs[i]
	}

	nf := float64(n)
	denom := nf*sumX2 - sumX*sumX
	if math.Abs(denom) < 1e-15 {
		// All x values are the same — can't fit a line.
		return LinearRegressionResult{
			Slope:     0,
			Intercept: sumY / nf,
			RSquared:  0,
			N:         n,
		}, nil
	}

	slope := (nf*sumXY - sumX*sumY) / denom
	intercept := (sumY - slope*sumX) / nf

	// R² = 1 - SS_res / SS_tot
	meanY := sumY / nf
	var ssTot, ssRes float64
	for i := 0; i < n; i++ {
		predicted := slope*xs[i] + intercept
		ssRes += (ys[i] - predicted) * (ys[i] - predicted)
		ssTot += (ys[i] - meanY) * (ys[i] - meanY)
	}

	rSquared := 0.0
	if ssTot > 1e-15 {
		rSquared = 1.0 - ssRes/ssTot
	}

	return LinearRegressionResult{
		Slope:     slope,
		Intercept: intercept,
		RSquared:  rSquared,
		N:         n,
	}, nil
}
