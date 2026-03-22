package remediation

import (
	"context"
	"fmt"
)

// Engine evaluates compiled-in remediation rules.
type Engine struct {
	rules        []Rule
	store        RecommendationStore
	metricSource MetricSource
}

// NewEngine creates an Engine with all compiled-in rules.
func NewEngine() *Engine {
	rules := make([]Rule, 0, 30)
	rules = append(rules, pgRules()...)
	rules = append(rules, osRules()...)
	return &Engine{rules: rules}
}

// SetStore sets the recommendation store for upsert operations.
func (e *Engine) SetStore(store RecommendationStore) {
	e.store = store
}

// SetMetricSource sets the metric source for snapshot lookups.
func (e *Engine) SetMetricSource(source MetricSource) {
	e.metricSource = source
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
				MetricKey:   result.MetricKey,
				MetricValue: result.MetricValue,
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

// EvaluateHook is called by the RCA engine when an incident's chain edge
// has a remediation hook. It maps the hook to a remediation rule, evaluates
// it, and upserts the recommendation with the incident ID.
func (e *Engine) EvaluateHook(
	ctx context.Context,
	hook, instanceID, metricKey string,
	value float64,
	incidentID int64,
	urgencyDelta float64,
) error {
	if e.store == nil {
		return nil // no store, no-op
	}

	ruleID, ok := HookToRuleID[hook]
	if !ok || ruleID == "" {
		return nil // no matching rule for this hook
	}

	// Find the rule.
	var matchedRule *Rule
	for i := range e.rules {
		if e.rules[i].ID == ruleID {
			matchedRule = &e.rules[i]
			break
		}
	}
	if matchedRule == nil {
		return nil // rule not found
	}

	// Build evaluation context.
	var snapshot MetricSnapshot
	if e.metricSource != nil {
		snapshot, _ = e.metricSource.CurrentSnapshot(ctx, instanceID)
	}

	evalCtx := EvalContext{
		InstanceID: instanceID,
		MetricKey:  metricKey,
		Value:      value,
		Snapshot:   snapshot,
	}

	result := matchedRule.Evaluate(evalCtx)
	if result == nil {
		return nil // rule did not fire
	}

	rec := Recommendation{
		RuleID:       ruleID,
		InstanceID:   instanceID,
		MetricKey:    metricKey,
		MetricValue:  value,
		Priority:     matchedRule.Priority,
		Category:     matchedRule.Category,
		Title:        result.Title,
		Description:  result.Description,
		DocURL:       result.DocURL,
		Source:       "rca",
		UrgencyScore: urgencyDelta,
		IncidentIDs:  []int64{incidentID},
	}

	if err := e.store.Upsert(ctx, rec); err != nil {
		return fmt.Errorf("evaluate hook %s: %w", hook, err)
	}
	return nil
}
