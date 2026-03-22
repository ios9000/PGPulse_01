package rca

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PGIncidentStore implements IncidentStore backed by PostgreSQL.
type PGIncidentStore struct {
	pool *pgxpool.Pool
}

// NewPGIncidentStore creates a PostgreSQL-backed incident store.
func NewPGIncidentStore(pool *pgxpool.Pool) *PGIncidentStore {
	return &PGIncidentStore{pool: pool}
}

// incidentJSONB is the JSONB document stored in timeline_json.
type incidentJSONB struct {
	PrimaryChain     *CausalChainResult `json:"primary_chain,omitempty"`
	AlternativeChain *CausalChainResult `json:"alternative_chain,omitempty"`
	Timeline         []TimelineEvent    `json:"timeline"`
	Quality          QualityStatus      `json:"quality"`
	RemediationHooks []string           `json:"remediation_hooks,omitempty"`
}

func (s *PGIncidentStore) Create(ctx context.Context, incident *Incident) (int64, error) {
	doc := incidentJSONB{
		PrimaryChain:     incident.PrimaryChain,
		AlternativeChain: incident.AlternativeChain,
		Timeline:         incident.Timeline,
		Quality:          incident.Quality,
		RemediationHooks: incident.RemediationHooks,
	}
	docJSON, err := json.Marshal(doc)
	if err != nil {
		return 0, fmt.Errorf("rca pgstore marshal: %w", err)
	}

	var primaryChainID, primaryRootCause *string
	if incident.PrimaryChain != nil {
		primaryChainID = &incident.PrimaryChain.ChainID
		primaryRootCause = &incident.PrimaryChain.RootCauseKey
	}

	const query = `INSERT INTO rca_incidents (
		instance_id, trigger_metric, trigger_value, trigger_time, trigger_kind,
		window_from, window_to,
		primary_chain_id, primary_root_cause,
		confidence, confidence_bucket, quality_status,
		timeline_json, summary, auto_triggered,
		remediation_hooks, chain_version, anomaly_source_mode,
		created_at
	) VALUES (
		$1, $2, $3, $4, $5,
		$6, $7,
		$8, $9,
		$10, $11, $12,
		$13, $14, $15,
		$16, $17, $18,
		$19
	) RETURNING id`

	var id int64
	err = s.pool.QueryRow(ctx, query,
		incident.InstanceID,
		incident.TriggerMetric,
		incident.TriggerValue,
		incident.TriggerTime,
		incident.TriggerKind,
		incident.AnalysisWindow.From,
		incident.AnalysisWindow.To,
		primaryChainID,
		primaryRootCause,
		incident.Confidence,
		incident.ConfidenceBucket,
		incident.Quality.AnomalySourceMode,
		docJSON,
		incident.Summary,
		incident.AutoTriggered,
		incident.RemediationHooks,
		incident.ChainVersion,
		incident.AnomalyMode,
		incident.CreatedAt,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("rca pgstore create: %w", err)
	}
	return id, nil
}

func (s *PGIncidentStore) Get(ctx context.Context, id int64) (*Incident, error) {
	const query = `SELECT
		id, instance_id, trigger_metric, trigger_value, trigger_time, trigger_kind,
		window_from, window_to,
		primary_chain_id, primary_root_cause,
		confidence, confidence_bucket, quality_status,
		timeline_json, summary, auto_triggered,
		remediation_hooks, chain_version, anomaly_source_mode,
		review_status, reviewed_by, reviewed_at, review_comment,
		created_at
	FROM rca_incidents WHERE id = $1`

	inc := &Incident{}
	var primaryChainID, primaryRootCause *string
	var qualityStatus string
	var docJSON []byte

	err := s.pool.QueryRow(ctx, query, id).Scan(
		&inc.ID,
		&inc.InstanceID,
		&inc.TriggerMetric,
		&inc.TriggerValue,
		&inc.TriggerTime,
		&inc.TriggerKind,
		&inc.AnalysisWindow.From,
		&inc.AnalysisWindow.To,
		&primaryChainID,
		&primaryRootCause,
		&inc.Confidence,
		&inc.ConfidenceBucket,
		&qualityStatus,
		&docJSON,
		&inc.Summary,
		&inc.AutoTriggered,
		&inc.RemediationHooks,
		&inc.ChainVersion,
		&inc.AnomalyMode,
		&inc.ReviewStatus,
		&inc.ReviewedBy,
		&inc.ReviewedAt,
		&inc.ReviewComment,
		&inc.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("rca pgstore get: %w", err)
	}

	var doc incidentJSONB
	if err := json.Unmarshal(docJSON, &doc); err != nil {
		return nil, fmt.Errorf("rca pgstore unmarshal: %w", err)
	}
	inc.PrimaryChain = doc.PrimaryChain
	inc.AlternativeChain = doc.AlternativeChain
	inc.Timeline = doc.Timeline
	inc.Quality = doc.Quality
	inc.RemediationHooks = doc.RemediationHooks

	return inc, nil
}

func (s *PGIncidentStore) ListByInstance(ctx context.Context, instanceID string, limit, offset int) ([]Incident, int, error) {
	// Count total.
	var total int
	err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM rca_incidents WHERE instance_id = $1`, instanceID,
	).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("rca pgstore count: %w", err)
	}

	const query = `SELECT
		id, instance_id, trigger_metric, trigger_value, trigger_time, trigger_kind,
		primary_chain_id, primary_root_cause,
		confidence, confidence_bucket, summary, auto_triggered,
		chain_version, anomaly_source_mode, created_at
	FROM rca_incidents
	WHERE instance_id = $1
	ORDER BY created_at DESC
	LIMIT $2 OFFSET $3`

	rows, err := s.pool.Query(ctx, query, instanceID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("rca pgstore list: %w", err)
	}
	defer rows.Close()

	var incidents []Incident
	for rows.Next() {
		var inc Incident
		var primaryChainID, primaryRootCause *string
		err := rows.Scan(
			&inc.ID,
			&inc.InstanceID,
			&inc.TriggerMetric,
			&inc.TriggerValue,
			&inc.TriggerTime,
			&inc.TriggerKind,
			&primaryChainID,
			&primaryRootCause,
			&inc.Confidence,
			&inc.ConfidenceBucket,
			&inc.Summary,
			&inc.AutoTriggered,
			&inc.ChainVersion,
			&inc.AnomalyMode,
			&inc.CreatedAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("rca pgstore scan: %w", err)
		}
		if primaryChainID != nil {
			inc.PrimaryChain = &CausalChainResult{ChainID: *primaryChainID}
			if primaryRootCause != nil {
				inc.PrimaryChain.RootCauseKey = *primaryRootCause
			}
		}
		incidents = append(incidents, inc)
	}

	return incidents, total, rows.Err()
}

func (s *PGIncidentStore) ListAll(ctx context.Context, limit, offset int) ([]Incident, int, error) {
	var total int
	err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM rca_incidents`).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("rca pgstore count all: %w", err)
	}

	const query = `SELECT
		id, instance_id, trigger_metric, trigger_value, trigger_time, trigger_kind,
		primary_chain_id, primary_root_cause,
		confidence, confidence_bucket, summary, auto_triggered,
		chain_version, anomaly_source_mode, created_at
	FROM rca_incidents
	ORDER BY created_at DESC
	LIMIT $1 OFFSET $2`

	rows, err := s.pool.Query(ctx, query, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("rca pgstore list all: %w", err)
	}
	defer rows.Close()

	var incidents []Incident
	for rows.Next() {
		var inc Incident
		var primaryChainID, primaryRootCause *string
		err := rows.Scan(
			&inc.ID,
			&inc.InstanceID,
			&inc.TriggerMetric,
			&inc.TriggerValue,
			&inc.TriggerTime,
			&inc.TriggerKind,
			&primaryChainID,
			&primaryRootCause,
			&inc.Confidence,
			&inc.ConfidenceBucket,
			&inc.Summary,
			&inc.AutoTriggered,
			&inc.ChainVersion,
			&inc.AnomalyMode,
			&inc.CreatedAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("rca pgstore scan all: %w", err)
		}
		if primaryChainID != nil {
			inc.PrimaryChain = &CausalChainResult{ChainID: *primaryChainID}
			if primaryRootCause != nil {
				inc.PrimaryChain.RootCauseKey = *primaryRootCause
			}
		}
		incidents = append(incidents, inc)
	}

	return incidents, total, rows.Err()
}

func (s *PGIncidentStore) UpdateReview(ctx context.Context, id int64, status, comment string) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE rca_incidents
		 SET review_status = $2, review_comment = $3, reviewed_at = NOW()
		 WHERE id = $1`,
		id, status, comment)
	if err != nil {
		return fmt.Errorf("rca pgstore update review: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("incident %d not found", id)
	}
	return nil
}

func (s *PGIncidentStore) Cleanup(ctx context.Context, olderThan time.Duration) (int64, error) {
	cutoff := time.Now().Add(-olderThan)
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM rca_incidents WHERE created_at < $1`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("rca pgstore cleanup: %w", err)
	}
	return tag.RowsAffected(), nil
}
