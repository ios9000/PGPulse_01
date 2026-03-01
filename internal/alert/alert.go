package alert

import "time"

// Severity levels for alert rules and events.
type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityWarning  Severity = "warning"
	SeverityCritical Severity = "critical"
)

// severityLevel returns numeric level for comparison (higher = more severe).
func severityLevel(s Severity) int {
	switch s {
	case SeverityInfo:
		return 1
	case SeverityWarning:
		return 2
	case SeverityCritical:
		return 3
	default:
		return 0
	}
}

// Operator defines comparison operations for threshold checks.
type Operator string

const (
	OpGreater      Operator = ">"
	OpGreaterEqual Operator = ">="
	OpLess         Operator = "<"
	OpLessEqual    Operator = "<="
	OpEqual        Operator = "=="
	OpNotEqual     Operator = "!="
)

// Compare evaluates: value <op> threshold.
func (op Operator) Compare(value, threshold float64) bool {
	switch op {
	case OpGreater:
		return value > threshold
	case OpGreaterEqual:
		return value >= threshold
	case OpLess:
		return value < threshold
	case OpLessEqual:
		return value <= threshold
	case OpEqual:
		return value == threshold
	case OpNotEqual:
		return value != threshold
	default:
		return false
	}
}

// AlertState represents the current state of a rule+instance combination.
type AlertState string

const (
	StateOK      AlertState = "ok"
	StatePending AlertState = "pending"
	StateFiring  AlertState = "firing"
)

// RuleSource indicates where a rule originated.
type RuleSource string

const (
	SourceBuiltin RuleSource = "builtin"
	SourceCustom  RuleSource = "custom"
)

// Rule defines a threshold-based alert check.
type Rule struct {
	ID               string            `json:"id"`
	Name             string            `json:"name"`
	Description      string            `json:"description,omitempty"`
	Metric           string            `json:"metric"`
	Operator         Operator          `json:"operator"`
	Threshold        float64           `json:"threshold"`
	Severity         Severity          `json:"severity"`
	Labels           map[string]string `json:"labels,omitempty"`
	ConsecutiveCount int               `json:"consecutive_count"`
	CooldownMinutes  int               `json:"cooldown_minutes"`
	Channels         []string          `json:"channels,omitempty"`
	Source           RuleSource        `json:"source"`
	Enabled          bool              `json:"enabled"`
}

// AlertEvent represents a state transition that requires action (notification).
type AlertEvent struct {
	RuleID       string            `json:"rule_id"`
	RuleName     string            `json:"rule_name"`
	InstanceID   string            `json:"instance_id"`
	Severity     Severity          `json:"severity"`
	Value        float64           `json:"value"`
	Threshold    float64           `json:"threshold"`
	Operator     Operator          `json:"operator"`
	Metric       string            `json:"metric"`
	Labels       map[string]string `json:"labels,omitempty"`
	Channels     []string          `json:"channels,omitempty"`
	FiredAt      time.Time         `json:"fired_at"`
	ResolvedAt   *time.Time        `json:"resolved_at,omitempty"`
	IsResolution bool              `json:"is_resolution"`
}

// stateEntry tracks per (rule_id, instance_id) evaluation state.
type stateEntry struct {
	State            AlertState
	ConsecutiveCount int
	FiredAt          time.Time
	LastNotifiedAt   time.Time
	Severity         Severity
}
