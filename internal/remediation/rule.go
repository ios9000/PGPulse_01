package remediation

import "time"

// Priority levels for recommendations (decoupled from alert severity).
type Priority string

const (
	PriorityInfo           Priority = "info"
	PrioritySuggestion     Priority = "suggestion"
	PriorityActionRequired Priority = "action_required"
)

// Category groups recommendations for filtering.
type Category string

const (
	CategoryPerformance   Category = "performance"
	CategoryCapacity      Category = "capacity"
	CategoryConfiguration Category = "configuration"
	CategoryReplication   Category = "replication"
	CategoryMaintenance   Category = "maintenance"
)

// Rule defines a compiled-in remediation rule.
type Rule struct {
	ID       string
	Priority Priority
	Category Category
	// Evaluate checks whether this rule fires given the context.
	// Returns nil if the rule does not match.
	Evaluate func(ctx EvalContext) *RuleResult
}

// EvalContext provides metric context to a rule's Evaluate function.
type EvalContext struct {
	InstanceID string
	MetricKey  string            // the specific metric that triggered (empty for Diagnose)
	Value      float64           // current value of the triggering metric
	Labels     map[string]string // metric labels
	Severity   string            // "warning", "critical", or "" for Diagnose
	Snapshot   MetricSnapshot
}

// MetricSnapshot provides read access to current metric values.
type MetricSnapshot map[string]float64

// Get returns a metric value and whether it exists.
func (s MetricSnapshot) Get(key string) (float64, bool) {
	v, ok := s[key]
	return v, ok
}

// RuleResult is what a matched rule returns.
type RuleResult struct {
	Title       string
	Description string
	DocURL      string
	MetricKey   string  // which metric triggered this result
	MetricValue float64 // the value of that metric
}

// Recommendation is the output persisted to the database and returned via API.
type Recommendation struct {
	ID             int64      `json:"id"`
	RuleID         string     `json:"rule_id"`
	InstanceID     string     `json:"instance_id"`
	AlertEventID   *int64     `json:"alert_event_id,omitempty"`
	MetricKey      string     `json:"metric_key"`
	MetricValue    float64    `json:"metric_value"`
	Priority       Priority   `json:"priority"`
	Category       Category   `json:"category"`
	Title          string     `json:"title"`
	Description    string     `json:"description"`
	DocURL         string     `json:"doc_url,omitempty"`
	Status         string     `json:"status"`
	CreatedAt      time.Time  `json:"created_at"`
	EvaluatedAt    time.Time  `json:"evaluated_at"`
	ResolvedAt     *time.Time `json:"resolved_at,omitempty"`
	AcknowledgedAt *time.Time `json:"acknowledged_at,omitempty"`
	AcknowledgedBy string     `json:"acknowledged_by,omitempty"`
}
