package remediation

import (
	"context"

	"github.com/ios9000/PGPulse_01/internal/alert"
)

// AlertAdapter wraps Engine to implement alert.RemediationProvider.
type AlertAdapter struct {
	engine       *Engine
	metricSource MetricSource
}

// MetricSource provides a snapshot of current metrics for an instance.
type MetricSource interface {
	CurrentSnapshot(ctx context.Context, instanceID string) (MetricSnapshot, error)
}

// NewAlertAdapter creates an AlertAdapter.
func NewAlertAdapter(engine *Engine, source MetricSource) *AlertAdapter {
	return &AlertAdapter{engine: engine, metricSource: source}
}

// EvaluateForAlert implements alert.RemediationProvider.
func (a *AlertAdapter) EvaluateForAlert(
	ctx context.Context,
	instanceID, metricKey string,
	value float64,
	labels map[string]string,
	severity string,
) []alert.RemediationResult {
	snapshot, _ := a.metricSource.CurrentSnapshot(ctx, instanceID)
	recs := a.engine.EvaluateMetric(ctx, instanceID, metricKey, value, labels, severity, snapshot)
	results := make([]alert.RemediationResult, len(recs))
	for i, r := range recs {
		results[i] = alert.RemediationResult{
			RuleID:      r.RuleID,
			Title:       r.Title,
			Description: r.Description,
			Priority:    string(r.Priority),
			Category:    string(r.Category),
			DocURL:      r.DocURL,
		}
	}
	return results
}
