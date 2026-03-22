package remediation

import (
	"context"
	"time"
)

// ListOpts controls filtering and pagination for recommendation queries.
type ListOpts struct {
	InstanceID   string
	Priority     string
	Category     string
	Status       string
	Source       string
	Acknowledged *bool
	AlertEventID *int64
	IncidentID   *int64
	OrderBy      string // "created_at" (default), "urgency_score"
	Limit        int
	Offset       int
}

// RecommendationStore persists recommendations to the database.
type RecommendationStore interface {
	Write(ctx context.Context, recs []Recommendation) ([]Recommendation, error)
	Upsert(ctx context.Context, rec Recommendation) error
	ListByInstance(ctx context.Context, instanceID string, opts ListOpts) ([]Recommendation, int, error)
	ListAll(ctx context.Context, opts ListOpts) ([]Recommendation, int, error)
	ListByAlertEvent(ctx context.Context, alertEventID int64) ([]Recommendation, error)
	ListByIncident(ctx context.Context, incidentID int64) ([]Recommendation, error)
	Acknowledge(ctx context.Context, id int64, username string) error
	CleanOld(ctx context.Context, olderThan time.Duration) error
	ResolveStale(ctx context.Context, instanceID string, currentRuleIDs []string) error
}
