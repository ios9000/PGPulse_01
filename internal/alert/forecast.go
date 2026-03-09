package alert

import "context"

// ForecastPoint is a thin mirror of ml.ForecastPoint containing only the
// fields that runForecastAlerts needs. Defined here to avoid importing
// internal/ml and creating a circular dependency.
type ForecastPoint struct {
	Offset int
	Value  float64
	Lower  float64
	Upper  float64
}

// ForecastProvider is satisfied by *ml.Detector via its ForecastForAlert
// adapter method. Nil provider disables forecast alert evaluation.
type ForecastProvider interface {
	ForecastForAlert(
		ctx context.Context,
		instanceID, metricKey string,
		horizon int,
	) ([]ForecastPoint, error)
}
