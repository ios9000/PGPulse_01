package ml

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// DBInstanceLister queries the instances table for enabled instance IDs.
type DBInstanceLister struct {
	pool *pgxpool.Pool
}

// NewDBInstanceLister creates a lister backed by the given pool.
func NewDBInstanceLister(pool *pgxpool.Pool) *DBInstanceLister {
	return &DBInstanceLister{pool: pool}
}

// ListInstanceIDs implements InstanceLister by querying the instances DB table.
func (l *DBInstanceLister) ListInstanceIDs(ctx context.Context) ([]string, error) {
	rows, err := l.pool.Query(ctx,
		`SELECT id FROM instances WHERE enabled = true ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if ids == nil {
		ids = []string{}
	}
	return ids, rows.Err()
}
