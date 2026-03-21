package rca

import (
	"context"
	"time"
)

// IncidentStore persists RCA incidents for later retrieval.
type IncidentStore interface {
	// Create persists a new incident and returns its ID.
	Create(ctx context.Context, incident *Incident) (int64, error)
	// Get retrieves an incident by ID with full JSONB deserialization.
	Get(ctx context.Context, id int64) (*Incident, error)
	// ListByInstance returns incidents for a specific instance, paginated.
	// Returns the page of incidents and the total count.
	ListByInstance(ctx context.Context, instanceID string, limit, offset int) ([]Incident, int, error)
	// ListAll returns incidents across all instances, paginated.
	ListAll(ctx context.Context, limit, offset int) ([]Incident, int, error)
	// Cleanup deletes incidents older than the given duration. Returns rows deleted.
	Cleanup(ctx context.Context, olderThan time.Duration) (int64, error)
}
