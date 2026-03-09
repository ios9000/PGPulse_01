package plans

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PGPlanStore implements CaptureStore using PostgreSQL.
type PGPlanStore struct {
	pool *pgxpool.Pool
}

// NewPGPlanStore creates a plan store backed by the given pool.
func NewPGPlanStore(pool *pgxpool.Pool) *PGPlanStore {
	return &PGPlanStore{pool: pool}
}

func (s *PGPlanStore) SavePlan(ctx context.Context, p PlanCapture) error {
	meta, _ := json.Marshal(p.Metadata)
	_, err := s.pool.Exec(ctx, `
		INSERT INTO query_plans
			(instance_id, database_name, query_fingerprint, plan_hash,
			 plan_text, plan_json, trigger_type, duration_ms, query_text,
			 truncated, metadata, captured_at)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, $8, $9, $10, $11::jsonb, $12)
		ON CONFLICT (instance_id, query_fingerprint, plan_hash)
		DO UPDATE SET captured_at = EXCLUDED.captured_at,
		              metadata = EXCLUDED.metadata
	`, p.InstanceID, p.DatabaseName, p.QueryFingerprint, p.PlanHash,
		p.PlanText, p.PlanText, // plan_json = same text cast to jsonb
		string(p.TriggerType), nullInt64(p.DurationMs), p.QueryText,
		p.Truncated, string(meta), p.CapturedAt)
	return err
}

func (s *PGPlanStore) LatestPlanHash(ctx context.Context, instanceID, fingerprint string) (string, error) {
	var hash string
	err := s.pool.QueryRow(ctx, `
		SELECT plan_hash FROM query_plans
		WHERE instance_id = $1 AND query_fingerprint = $2
		ORDER BY captured_at DESC LIMIT 1
	`, instanceID, fingerprint).Scan(&hash)
	if err != nil {
		return "", nil // not found is OK
	}
	return hash, nil
}

// ListPlans returns captured plans for an instance, optionally filtered.
func (s *PGPlanStore) ListPlans(ctx context.Context, instanceID, fingerprint string, since *time.Time, triggerType string) ([]PlanCapture, error) {
	query := `
		SELECT id, instance_id, database_name, query_fingerprint, plan_hash,
		       trigger_type, duration_ms, query_text, truncated, captured_at, metadata
		FROM query_plans
		WHERE instance_id = $1
	`
	args := []any{instanceID}
	n := 2

	if fingerprint != "" {
		query += " AND query_fingerprint = $" + strconv.Itoa(n)
		args = append(args, fingerprint)
		n++
	}
	if since != nil {
		query += " AND captured_at >= $" + strconv.Itoa(n)
		args = append(args, *since)
		n++
	}
	if triggerType != "" {
		query += " AND trigger_type = $" + strconv.Itoa(n)
		args = append(args, triggerType)
	}

	query += " ORDER BY captured_at DESC LIMIT 100"

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanPlans(rows)
}

// GetPlan returns a single plan by ID including the full plan text.
func (s *PGPlanStore) GetPlan(ctx context.Context, planID int64) (*PlanCapture, error) {
	var p PlanCapture
	var triggerStr string
	var durationMs *int64
	var metaBytes []byte
	err := s.pool.QueryRow(ctx, `
		SELECT id, instance_id, database_name, query_fingerprint, plan_hash,
		       plan_text, trigger_type, duration_ms, query_text, truncated,
		       captured_at, metadata
		FROM query_plans WHERE id = $1
	`, planID).Scan(&p.ID, &p.InstanceID, &p.DatabaseName, &p.QueryFingerprint,
		&p.PlanHash, &p.PlanText, &triggerStr, &durationMs, &p.QueryText,
		&p.Truncated, &p.CapturedAt, &metaBytes)
	if err != nil {
		return nil, err
	}
	p.TriggerType = TriggerType(triggerStr)
	if durationMs != nil {
		p.DurationMs = *durationMs
	}
	if len(metaBytes) > 0 {
		_ = json.Unmarshal(metaBytes, &p.Metadata)
	}
	return &p, nil
}

// ListRegressions returns plan hash changes for an instance.
func (s *PGPlanStore) ListRegressions(ctx context.Context, instanceID string) ([]PlanCapture, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, instance_id, database_name, query_fingerprint, plan_hash,
		       trigger_type, duration_ms, query_text, truncated, captured_at, metadata
		FROM query_plans
		WHERE trigger_type = 'hash_diff_signal' AND instance_id = $1
		ORDER BY captured_at DESC LIMIT 50
	`, instanceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPlans(rows)
}

func scanPlans(rows pgx.Rows) ([]PlanCapture, error) {
	var plans []PlanCapture
	for rows.Next() {
		var p PlanCapture
		var triggerStr string
		var durationMs *int64
		var metaBytes []byte
		if err := rows.Scan(&p.ID, &p.InstanceID, &p.DatabaseName, &p.QueryFingerprint,
			&p.PlanHash, &triggerStr, &durationMs, &p.QueryText,
			&p.Truncated, &p.CapturedAt, &metaBytes); err != nil {
			continue
		}
		p.TriggerType = TriggerType(triggerStr)
		if durationMs != nil {
			p.DurationMs = *durationMs
		}
		if len(metaBytes) > 0 {
			_ = json.Unmarshal(metaBytes, &p.Metadata)
		}
		plans = append(plans, p)
	}
	if plans == nil {
		plans = []PlanCapture{}
	}
	return plans, rows.Err()
}

// nullInt64 returns nil for zero values, otherwise a pointer to the value.
func nullInt64(v int64) *int64 {
	if v == 0 {
		return nil
	}
	return &v
}
