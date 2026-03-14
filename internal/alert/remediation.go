package alert

import "context"

// RemediationResult is a recommendation returned by the remediation engine.
// Defined here to avoid importing internal/remediation from internal/alert.
type RemediationResult struct {
	RuleID      string `json:"rule_id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Priority    string `json:"priority"`
	Category    string `json:"category"`
	DocURL      string `json:"doc_url,omitempty"`
}

// RemediationProvider evaluates remediation rules for a fired alert.
type RemediationProvider interface {
	EvaluateForAlert(
		ctx context.Context,
		instanceID, metricKey string,
		value float64,
		labels map[string]string,
		severity string,
	) []RemediationResult
}
