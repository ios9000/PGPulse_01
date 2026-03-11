package api

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// InstanceConnProvider provides connections to monitored PostgreSQL instances.
type InstanceConnProvider interface {
	ConnFor(ctx context.Context, instanceID string) (*pgx.Conn, error)
	// ConnForDB opens a connection to a specific database on the monitored instance.
	ConnForDB(ctx context.Context, instanceID, dbName string) (*pgx.Conn, error)
}
