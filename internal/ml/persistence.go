package ml

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PersistenceStore saves and loads ML baseline snapshots.
type PersistenceStore interface {
	Save(ctx context.Context, snap BaselineSnapshot) error
	Load(ctx context.Context, instanceID, metricKey string) (*BaselineSnapshot, error)
	LoadAll(ctx context.Context) ([]BaselineSnapshot, error)
}

// DBPersistenceStore implements PersistenceStore using PostgreSQL.
type DBPersistenceStore struct {
	pool *pgxpool.Pool
}

// NewDBPersistenceStore creates a persistence store backed by the given pool.
func NewDBPersistenceStore(pool *pgxpool.Pool) *DBPersistenceStore {
	return &DBPersistenceStore{pool: pool}
}

// Save upserts a baseline snapshot into ml_baseline_snapshots.
func (s *DBPersistenceStore) Save(ctx context.Context, snap BaselineSnapshot) error {
	data, err := json.Marshal(snap)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO ml_baseline_snapshots (instance_id, metric_key, period, state, updated_at)
		VALUES ($1, $2, $3, $4, now())
		ON CONFLICT (instance_id, metric_key)
		DO UPDATE SET state = EXCLUDED.state, period = EXCLUDED.period, updated_at = now()
	`, snap.InstanceID, snap.MetricKey, snap.Period, data)
	return err
}

// Load retrieves a single baseline snapshot by instance and metric key.
func (s *DBPersistenceStore) Load(ctx context.Context, instanceID, metricKey string) (*BaselineSnapshot, error) {
	var data []byte
	err := s.pool.QueryRow(ctx, `
		SELECT state FROM ml_baseline_snapshots
		WHERE instance_id = $1 AND metric_key = $2
	`, instanceID, metricKey).Scan(&data)
	if err != nil {
		return nil, err
	}
	var snap BaselineSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, err
	}
	return &snap, nil
}

// LoadAll retrieves all baseline snapshots from the database.
func (s *DBPersistenceStore) LoadAll(ctx context.Context) ([]BaselineSnapshot, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT state FROM ml_baseline_snapshots ORDER BY instance_id, metric_key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var snaps []BaselineSnapshot
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		var snap BaselineSnapshot
		if err := json.Unmarshal(data, &snap); err != nil {
			continue
		}
		snaps = append(snaps, snap)
	}
	if snaps == nil {
		snaps = []BaselineSnapshot{}
	}
	return snaps, rows.Err()
}
