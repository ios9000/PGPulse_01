package api

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// InstanceConnProvider provides connections to monitored PostgreSQL instances.
type InstanceConnProvider interface {
	ConnFor(ctx context.Context, instanceID string) (*pgx.Conn, error)
}
