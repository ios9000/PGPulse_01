package alert

import (
	"context"
	"time"
)

// AlertRuleStore manages persistent alert rule storage.
type AlertRuleStore interface {
	List(ctx context.Context) ([]Rule, error)
	ListEnabled(ctx context.Context) ([]Rule, error)
	Get(ctx context.Context, id string) (*Rule, error)
	Create(ctx context.Context, rule *Rule) error
	Update(ctx context.Context, rule *Rule) error
	Delete(ctx context.Context, id string) error
	UpsertBuiltin(ctx context.Context, rule *Rule) error
}

// AlertHistoryStore manages alert event history.
type AlertHistoryStore interface {
	Record(ctx context.Context, event *AlertEvent) error
	Resolve(ctx context.Context, ruleID, instanceID string, resolvedAt time.Time) error
	ListUnresolved(ctx context.Context) ([]AlertEvent, error)
	Query(ctx context.Context, q AlertHistoryQuery) ([]AlertEvent, error)
	Cleanup(ctx context.Context, olderThan time.Duration) (int64, error)
}

// AlertHistoryQuery defines filters for querying alert history.
type AlertHistoryQuery struct {
	InstanceID     string
	RuleID         string
	Severity       Severity
	Start          time.Time
	End            time.Time
	UnresolvedOnly bool
	Limit          int
}
