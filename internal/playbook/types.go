package playbook

import (
	"encoding/json"
	"time"
)

// Playbook represents a guided remediation playbook.
type Playbook struct {
	ID                   int64           `json:"id"`
	Slug                 string          `json:"slug"`
	Name                 string          `json:"name"`
	Description          string          `json:"description"`
	Version              int             `json:"version"`
	Status               string          `json:"status"`
	Category             string          `json:"category"`
	TriggerBindings      TriggerBindings `json:"trigger_bindings"`
	EstimatedDurationMin *int            `json:"estimated_duration_min,omitempty"`
	RequiresPermission   string          `json:"requires_permission"`
	Author               string          `json:"author"`
	IsBuiltin            bool            `json:"is_builtin"`
	Steps                []Step          `json:"steps,omitempty"`
	CreatedAt            time.Time       `json:"created_at"`
	UpdatedAt            time.Time       `json:"updated_at"`
}

// TriggerBindings defines how a playbook is matched by the Resolver.
type TriggerBindings struct {
	Hooks        []string `json:"hooks,omitempty"`
	RootCauses   []string `json:"root_causes,omitempty"`
	Metrics      []string `json:"metrics,omitempty"`
	AdviserRules []string `json:"adviser_rules,omitempty"`
}

// Step represents a single step in a playbook.
type Step struct {
	ID                   int64              `json:"id"`
	PlaybookID           int64              `json:"playbook_id"`
	StepOrder            int                `json:"step_order"`
	Name                 string             `json:"name"`
	Description          string             `json:"description"`
	SQLTemplate          string             `json:"sql_template,omitempty"`
	SafetyTier           string             `json:"safety_tier"`
	TimeoutSeconds       int                `json:"timeout_seconds"`
	ResultInterpretation InterpretationSpec `json:"result_interpretation"`
	BranchRules          []BranchRule       `json:"branch_rules"`
	NextStepDefault      *int               `json:"next_step_default,omitempty"`
	ManualInstructions   string             `json:"manual_instructions,omitempty"`
	EscalationContact    string             `json:"escalation_contact,omitempty"`
	CreatedAt            time.Time          `json:"created_at"`
}

// Safety tier constants.
const (
	TierDiagnostic = "diagnostic"
	TierRemediate  = "remediate"
	TierDangerous  = "dangerous"
	TierExternal   = "external"
)

// Run represents a playbook execution instance.
type Run struct {
	ID               int64      `json:"id"`
	PlaybookID       int64      `json:"playbook_id"`
	PlaybookVersion  int        `json:"playbook_version"`
	PlaybookName     string     `json:"playbook_name,omitempty"`
	InstanceID       string     `json:"instance_id"`
	StartedBy        string     `json:"started_by"`
	Status           string     `json:"status"`
	CurrentStepOrder int        `json:"current_step_order"`
	TriggerSource    string     `json:"trigger_source,omitempty"`
	TriggerID        string     `json:"trigger_id,omitempty"`
	Steps            []RunStep  `json:"steps,omitempty"`
	StartedAt        time.Time  `json:"started_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	CompletedAt      *time.Time `json:"completed_at,omitempty"`
	FeedbackUseful   *bool      `json:"feedback_useful,omitempty"`
	FeedbackResolved *bool      `json:"feedback_resolved,omitempty"`
	FeedbackNotes    string     `json:"feedback_notes,omitempty"`
}

// RunStep records the execution of a single step in a run.
type RunStep struct {
	ID            int64           `json:"id"`
	RunID         int64           `json:"run_id"`
	StepOrder     int             `json:"step_order"`
	Status        string          `json:"status"`
	SQLExecuted   string          `json:"sql_executed,omitempty"`
	ResultJSON    json.RawMessage `json:"result_json,omitempty"`
	ResultVerdict string          `json:"result_verdict,omitempty"`
	ResultMessage string          `json:"result_message,omitempty"`
	Error         string          `json:"error,omitempty"`
	ExecutedAt    *time.Time      `json:"executed_at,omitempty"`
	DurationMs    int             `json:"duration_ms,omitempty"`
	ConfirmedBy   string          `json:"confirmed_by,omitempty"`
}

// InterpretationSpec defines how to interpret step results.
type InterpretationSpec struct {
	Rules          []InterpretationRule `json:"rules,omitempty"`
	RowCountRules  []RowCountRule       `json:"row_count_rules,omitempty"`
	DefaultVerdict string               `json:"default_verdict"`
	DefaultMessage string               `json:"default_message"`
}

// InterpretationRule evaluates a column value against a threshold.
type InterpretationRule struct {
	Column   string `json:"column"`
	Operator string `json:"operator"` // >, <, >=, <=, ==, !=, is_null, is_not_null
	Value    any    `json:"value"`
	Verdict  string `json:"verdict"` // green, yellow, red
	Message  string `json:"message"` // Template with {{column_name}}
	Scope    string `json:"scope,omitempty"`
}

// RowCountRule evaluates the total row count.
type RowCountRule struct {
	Operator string `json:"operator"`
	Value    any    `json:"value"`
	Verdict  string `json:"verdict"`
	Message  string `json:"message"`
}

// BranchRule defines conditional navigation between steps.
type BranchRule struct {
	Condition BranchCondition `json:"condition"`
	GotoStep  int             `json:"goto_step"`
	Reason    string          `json:"reason"`
}

// BranchCondition specifies when a branch should be taken.
type BranchCondition struct {
	Column   string `json:"column,omitempty"`
	Operator string `json:"operator,omitempty"`
	Value    any    `json:"value,omitempty"`
	Verdict  string `json:"verdict,omitempty"` // Alternative: branch on computed verdict
}

// PlaybookListOpts filters playbook listings.
type PlaybookListOpts struct {
	Status   string
	Category string
	Search   string
	Limit    int
	Offset   int
}

// RunListOpts filters run listings.
type RunListOpts struct {
	Status string
	Limit  int
	Offset int
}

// ExecutionResult holds the output of executing a step's SQL.
type ExecutionResult struct {
	Columns   []string `json:"columns"`
	Rows      [][]any  `json:"rows"`
	RowCount  int      `json:"row_count"`
	TotalRows int      `json:"total_rows"`
	Truncated bool     `json:"truncated"`
	Duration  int      `json:"duration_ms"`
}

// PlaybookConfig holds playbook subsystem configuration.
type PlaybookConfig struct {
	Enabled                 bool          `yaml:"enabled" koanf:"enabled"`
	DefaultStatementTimeout int           `yaml:"default_statement_timeout" koanf:"default_statement_timeout"`
	DefaultLockTimeout      int           `yaml:"default_lock_timeout" koanf:"default_lock_timeout"`
	ResultRowLimit          int           `yaml:"result_row_limit" koanf:"result_row_limit"`
	RunRetentionDays        int           `yaml:"run_retention_days" koanf:"run_retention_days"`
	ImplicitFeedbackWindow  time.Duration `yaml:"implicit_feedback_window" koanf:"implicit_feedback_window"`
}

// intPtr returns a pointer to the given int.
func intPtr(n int) *int {
	return &n
}
