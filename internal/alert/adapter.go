package alert

import (
	"context"
	"time"

	"github.com/ios9000/PGPulse_01/internal/collector"
)

// MetricAlertAdapter wraps *Evaluator to satisfy collector.AlertEvaluator.
// It converts a single metric/value/labels call into a one-element
// []MetricPoint batch that the stateful Evaluator understands.
type MetricAlertAdapter struct {
	inner *Evaluator
}

// NewMetricAlertAdapter creates an adapter bridging collector.AlertEvaluator
// to the batch-oriented *alert.Evaluator.
func NewMetricAlertAdapter(e *Evaluator) *MetricAlertAdapter {
	return &MetricAlertAdapter{inner: e}
}

// Evaluate satisfies collector.AlertEvaluator.
func (a *MetricAlertAdapter) Evaluate(ctx context.Context, metric string, value float64, labels map[string]string) error {
	points := []collector.MetricPoint{{
		Metric:    metric,
		Value:     value,
		Labels:    labels,
		Timestamp: time.Now(),
	}}
	_, err := a.inner.Evaluate(ctx, points)
	return err
}
