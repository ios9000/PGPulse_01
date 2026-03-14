package remediation

import "context"

// Engine evaluates compiled-in remediation rules.
type Engine struct {
	rules []Rule
}

// NewEngine creates an Engine with all compiled-in rules.
func NewEngine() *Engine {
	rules := make([]Rule, 0, 30)
	rules = append(rules, pgRules()...)
	rules = append(rules, osRules()...)
	return &Engine{rules: rules}
}

// EvaluateMetric runs rules that match a specific metric key.
// Called when an alert fires. Returns zero or more recommendations.
func (e *Engine) EvaluateMetric(
	_ context.Context,
	instanceID, metricKey string,
	value float64,
	labels map[string]string,
	severity string,
	snapshot MetricSnapshot,
) []Recommendation {
	evalCtx := EvalContext{
		InstanceID: instanceID,
		MetricKey:  metricKey,
		Value:      value,
		Labels:     labels,
		Severity:   severity,
		Snapshot:   snapshot,
	}
	var recs []Recommendation
	for _, rule := range e.rules {
		result := rule.Evaluate(evalCtx)
		if result != nil {
			recs = append(recs, Recommendation{
				RuleID:      rule.ID,
				InstanceID:  instanceID,
				MetricKey:   metricKey,
				MetricValue: value,
				Priority:    rule.Priority,
				Category:    rule.Category,
				Title:       result.Title,
				Description: result.Description,
				DocURL:      result.DocURL,
			})
		}
	}
	return recs
}

// Diagnose runs ALL rules against a full metric snapshot for an instance.
// Called on-demand via the Diagnose API endpoint.
func (e *Engine) Diagnose(
	_ context.Context,
	instanceID string,
	snapshot MetricSnapshot,
) []Recommendation {
	var recs []Recommendation
	for _, rule := range e.rules {
		evalCtx := EvalContext{
			InstanceID: instanceID,
			Snapshot:   snapshot,
		}
		result := rule.Evaluate(evalCtx)
		if result != nil {
			recs = append(recs, Recommendation{
				RuleID:      rule.ID,
				InstanceID:  instanceID,
				Priority:    rule.Priority,
				Category:    rule.Category,
				Title:       result.Title,
				Description: result.Description,
				DocURL:      result.DocURL,
			})
		}
	}
	return recs
}

// Rules returns all registered rules (for introspection/listing).
func (e *Engine) Rules() []Rule {
	return e.rules
}
