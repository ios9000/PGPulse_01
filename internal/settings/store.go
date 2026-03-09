package settings

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PGSnapshotStore implements SnapshotStore using PostgreSQL.
type PGSnapshotStore struct {
	pool *pgxpool.Pool
}

// NewPGSnapshotStore creates a settings snapshot store.
func NewPGSnapshotStore(pool *pgxpool.Pool) *PGSnapshotStore {
	return &PGSnapshotStore{pool: pool}
}

func (s *PGSnapshotStore) SaveSnapshot(ctx context.Context, snap Snapshot) error {
	settingsJSON, err := json.Marshal(snap.Settings)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO settings_snapshots (instance_id, captured_at, trigger_type, pg_version, settings)
		VALUES ($1, $2, $3, $4, $5)
	`, snap.InstanceID, snap.CapturedAt, snap.TriggerType, snap.PGVersion, settingsJSON)
	return err
}

func (s *PGSnapshotStore) GetSnapshot(ctx context.Context, id int64) (*Snapshot, error) {
	var snap Snapshot
	var settingsJSON []byte
	err := s.pool.QueryRow(ctx, `
		SELECT id, instance_id, captured_at, trigger_type, pg_version, settings
		FROM settings_snapshots WHERE id = $1
	`, id).Scan(&snap.ID, &snap.InstanceID, &snap.CapturedAt, &snap.TriggerType,
		&snap.PGVersion, &settingsJSON)
	if err != nil {
		return nil, err
	}
	snap.Settings = make(map[string]SettingValue)
	if err := json.Unmarshal(settingsJSON, &snap.Settings); err != nil {
		return nil, err
	}
	return &snap, nil
}

func (s *PGSnapshotStore) ListSnapshots(ctx context.Context, instanceID string, limit int) ([]SnapshotMeta, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, instance_id, captured_at, trigger_type, pg_version
		FROM settings_snapshots
		WHERE instance_id = $1
		ORDER BY captured_at DESC
		LIMIT $2
	`, instanceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var metas []SnapshotMeta
	for rows.Next() {
		var m SnapshotMeta
		if err := rows.Scan(&m.ID, &m.InstanceID, &m.CapturedAt, &m.TriggerType, &m.PGVersion); err != nil {
			continue
		}
		metas = append(metas, m)
	}
	if metas == nil {
		metas = []SnapshotMeta{}
	}
	return metas, rows.Err()
}

func (s *PGSnapshotStore) LatestSnapshot(ctx context.Context, instanceID string) (*Snapshot, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `
		SELECT id FROM settings_snapshots
		WHERE instance_id = $1
		ORDER BY captured_at DESC LIMIT 1
	`, instanceID).Scan(&id)
	if err != nil {
		return nil, err
	}
	return s.GetSnapshot(ctx, id)
}
