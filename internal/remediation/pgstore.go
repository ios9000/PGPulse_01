package remediation

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PGStore implements RecommendationStore using PostgreSQL.
type PGStore struct {
	pool *pgxpool.Pool
}

// NewPGStore creates a PGStore.
func NewPGStore(pool *pgxpool.Pool) *PGStore {
	return &PGStore{pool: pool}
}

func (s *PGStore) Write(ctx context.Context, recs []Recommendation) ([]Recommendation, error) {
	if len(recs) == 0 {
		return nil, nil
	}
	saved := make([]Recommendation, 0, len(recs))
	for _, r := range recs {
		urgency := r.UrgencyScore
		if urgency == 0 {
			urgency = UrgencyFromPriority(r.Priority)
		}
		source := r.Source
		if source == "" {
			source = "alert"
		}
		var rec Recommendation
		err := s.pool.QueryRow(ctx,
			`INSERT INTO remediation_recommendations
				(rule_id, instance_id, alert_event_id, metric_key, metric_value,
				 priority, category, title, description, doc_url, status, evaluated_at,
				 source, urgency_score)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 'active', NOW(), $11, $12)
			 RETURNING id, rule_id, instance_id, alert_event_id, metric_key, metric_value,
			           priority, category, title, description, doc_url, status,
			           source, urgency_score, incident_ids, last_incident_at,
			           created_at, evaluated_at, resolved_at, acknowledged_at, acknowledged_by`,
			r.RuleID, r.InstanceID, r.AlertEventID, r.MetricKey, r.MetricValue,
			string(r.Priority), string(r.Category), r.Title, r.Description, r.DocURL,
			source, urgency,
		).Scan(
			&rec.ID, &rec.RuleID, &rec.InstanceID, &rec.AlertEventID,
			&rec.MetricKey, &rec.MetricValue, &rec.Priority, &rec.Category,
			&rec.Title, &rec.Description, &rec.DocURL, &rec.Status,
			&rec.Source, &rec.UrgencyScore, &rec.IncidentIDs, &rec.LastIncidentAt,
			&rec.CreatedAt, &rec.EvaluatedAt, &rec.ResolvedAt,
			&rec.AcknowledgedAt, &rec.AcknowledgedBy,
		)
		if err != nil {
			return saved, fmt.Errorf("write recommendation: %w", err)
		}
		saved = append(saved, rec)
	}
	return saved, nil
}

// Upsert inserts a new recommendation or updates an existing active one.
// Uses ON CONFLICT with the partial unique index on (rule_id, instance_id) WHERE status='active'.
// On conflict, it bumps urgency_score by the provided delta (soft-capped at 10.0 via LEAST()),
// appends the incident ID, and updates the timestamp.
// If rec.IncidentIDs has at least one element, the first is used as the incident ID.
func (s *PGStore) Upsert(ctx context.Context, rec Recommendation) error {
	urgency := rec.UrgencyScore
	if urgency == 0 {
		urgency = UrgencyFromPriority(rec.Priority)
	}
	source := rec.Source
	if source == "" {
		source = "rca"
	}

	// Extract single incident ID for upsert (or nil).
	var incidentID *int64
	if len(rec.IncidentIDs) > 0 {
		incidentID = &rec.IncidentIDs[0]
	}

	_, err := s.pool.Exec(ctx,
		`INSERT INTO remediation_recommendations
			(rule_id, instance_id, metric_key, metric_value,
			 priority, category, title, description, doc_url,
			 status, evaluated_at, source, urgency_score,
			 incident_ids, last_incident_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9,
		         'active', NOW(), $10, $11,
		         CASE WHEN $12::BIGINT IS NOT NULL THEN ARRAY[$12::BIGINT] ELSE '{}'::BIGINT[] END,
		         CASE WHEN $12::BIGINT IS NOT NULL THEN NOW() ELSE NULL END)
		 ON CONFLICT (rule_id, instance_id) WHERE status = 'active'
		 DO UPDATE SET
		     evaluated_at = NOW(),
		     metric_value = EXCLUDED.metric_value,
		     title = EXCLUDED.title,
		     description = EXCLUDED.description,
		     urgency_score = LEAST($13::FLOAT8, remediation_recommendations.urgency_score + $11::FLOAT8),
		     incident_ids = CASE
		         WHEN $12::BIGINT IS NOT NULL
		         THEN array_append(remediation_recommendations.incident_ids, $12::BIGINT)
		         ELSE remediation_recommendations.incident_ids
		     END,
		     last_incident_at = CASE
		         WHEN $12::BIGINT IS NOT NULL THEN NOW()
		         ELSE remediation_recommendations.last_incident_at
		     END`,
		rec.RuleID, rec.InstanceID, rec.MetricKey, rec.MetricValue,
		string(rec.Priority), string(rec.Category), rec.Title, rec.Description, rec.DocURL,
		source, urgency,
		incidentID,    // $12
		UrgencySoftCap, // $13
	)
	if err != nil {
		return fmt.Errorf("upsert recommendation %s: %w", rec.RuleID, err)
	}
	return nil
}

// WriteOrUpdate inserts new recommendations or updates existing active ones.
// Uses the partial unique index idx_remediation_active_unique on (rule_id, instance_id)
// WHERE status = 'active'. If an active recommendation exists, it updates evaluated_at
// and the current metric value. New recommendations are inserted as active.
func (s *PGStore) WriteOrUpdate(ctx context.Context, recs []Recommendation) (int, error) {
	if len(recs) == 0 {
		return 0, nil
	}
	written := 0
	for _, r := range recs {
		tag, err := s.pool.Exec(ctx,
			`INSERT INTO remediation_recommendations
				(rule_id, instance_id, metric_key, metric_value,
				 priority, category, title, description, doc_url, status, evaluated_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'active', NOW())
			 ON CONFLICT (rule_id, instance_id) WHERE status = 'active'
			 DO UPDATE SET evaluated_at = NOW(), metric_value = EXCLUDED.metric_value,
			              title = EXCLUDED.title, description = EXCLUDED.description`,
			r.RuleID, r.InstanceID, r.MetricKey, r.MetricValue,
			string(r.Priority), string(r.Category), r.Title, r.Description, r.DocURL,
		)
		if err != nil {
			return written, fmt.Errorf("write/update recommendation %s: %w", r.RuleID, err)
		}
		written += int(tag.RowsAffected())
	}
	return written, nil
}

func (s *PGStore) ListByInstance(ctx context.Context, instanceID string, opts ListOpts) ([]Recommendation, int, error) {
	opts.InstanceID = instanceID
	return s.listWithOpts(ctx, opts)
}

func (s *PGStore) ListAll(ctx context.Context, opts ListOpts) ([]Recommendation, int, error) {
	return s.listWithOpts(ctx, opts)
}

func (s *PGStore) listWithOpts(ctx context.Context, opts ListOpts) ([]Recommendation, int, error) {
	where := "WHERE 1=1"
	args := make([]any, 0, 8)
	argIdx := 1

	if opts.InstanceID != "" {
		where += fmt.Sprintf(" AND instance_id = $%d", argIdx)
		args = append(args, opts.InstanceID)
		argIdx++
	}
	if opts.Priority != "" {
		where += fmt.Sprintf(" AND priority = $%d", argIdx)
		args = append(args, opts.Priority)
		argIdx++
	}
	if opts.Category != "" {
		where += fmt.Sprintf(" AND category = $%d", argIdx)
		args = append(args, opts.Category)
		argIdx++
	}
	if opts.Status != "" {
		where += fmt.Sprintf(" AND status = $%d", argIdx)
		args = append(args, opts.Status)
		argIdx++
	}
	if opts.Source != "" {
		where += fmt.Sprintf(" AND source = $%d", argIdx)
		args = append(args, opts.Source)
		argIdx++
	}
	if opts.Acknowledged != nil {
		if *opts.Acknowledged {
			where += " AND acknowledged_at IS NOT NULL"
		} else {
			where += " AND acknowledged_at IS NULL"
		}
	}
	if opts.IncidentID != nil {
		where += fmt.Sprintf(" AND $%d = ANY(incident_ids)", argIdx)
		args = append(args, *opts.IncidentID)
		argIdx++
	}

	// Count total.
	var total int
	countSQL := "SELECT COUNT(*) FROM remediation_recommendations " + where
	if err := s.pool.QueryRow(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count recommendations: %w", err)
	}

	limit := opts.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}

	orderBy := "created_at DESC"
	if opts.OrderBy == "urgency_score" {
		orderBy = "urgency_score DESC, created_at DESC"
	}

	querySQL := fmt.Sprintf(
		`SELECT id, rule_id, instance_id, alert_event_id, metric_key, metric_value,
		        priority, category, title, description, doc_url, status,
		        source, urgency_score, incident_ids, last_incident_at,
		        created_at, evaluated_at, resolved_at, acknowledged_at, acknowledged_by
		 FROM remediation_recommendations %s
		 ORDER BY %s
		 LIMIT $%d OFFSET $%d`, where, orderBy, argIdx, argIdx+1)
	args = append(args, limit, opts.Offset)

	rows, err := s.pool.Query(ctx, querySQL, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list recommendations: %w", err)
	}
	defer rows.Close()

	var recs []Recommendation
	for rows.Next() {
		var r Recommendation
		if err := rows.Scan(
			&r.ID, &r.RuleID, &r.InstanceID, &r.AlertEventID,
			&r.MetricKey, &r.MetricValue, &r.Priority, &r.Category,
			&r.Title, &r.Description, &r.DocURL, &r.Status,
			&r.Source, &r.UrgencyScore, &r.IncidentIDs, &r.LastIncidentAt,
			&r.CreatedAt, &r.EvaluatedAt, &r.ResolvedAt,
			&r.AcknowledgedAt, &r.AcknowledgedBy,
		); err != nil {
			return nil, 0, fmt.Errorf("scan recommendation: %w", err)
		}
		recs = append(recs, r)
	}
	return recs, total, rows.Err()
}

func (s *PGStore) ListByAlertEvent(ctx context.Context, alertEventID int64) ([]Recommendation, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, rule_id, instance_id, alert_event_id, metric_key, metric_value,
		        priority, category, title, description, doc_url, status,
		        source, urgency_score, incident_ids, last_incident_at,
		        created_at, evaluated_at, resolved_at, acknowledged_at, acknowledged_by
		 FROM remediation_recommendations
		 WHERE alert_event_id = $1
		 ORDER BY created_at DESC`, alertEventID)
	if err != nil {
		return nil, fmt.Errorf("list by alert event: %w", err)
	}
	defer rows.Close()

	var recs []Recommendation
	for rows.Next() {
		var r Recommendation
		if err := rows.Scan(
			&r.ID, &r.RuleID, &r.InstanceID, &r.AlertEventID,
			&r.MetricKey, &r.MetricValue, &r.Priority, &r.Category,
			&r.Title, &r.Description, &r.DocURL, &r.Status,
			&r.Source, &r.UrgencyScore, &r.IncidentIDs, &r.LastIncidentAt,
			&r.CreatedAt, &r.EvaluatedAt, &r.ResolvedAt,
			&r.AcknowledgedAt, &r.AcknowledgedBy,
		); err != nil {
			return nil, fmt.Errorf("scan recommendation: %w", err)
		}
		recs = append(recs, r)
	}
	return recs, rows.Err()
}

// ListByIncident returns recommendations linked to a specific RCA incident.
func (s *PGStore) ListByIncident(ctx context.Context, incidentID int64) ([]Recommendation, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, rule_id, instance_id, alert_event_id, metric_key, metric_value,
		        priority, category, title, description, doc_url, status,
		        source, urgency_score, incident_ids, last_incident_at,
		        created_at, evaluated_at, resolved_at, acknowledged_at, acknowledged_by
		 FROM remediation_recommendations
		 WHERE $1 = ANY(incident_ids)
		 ORDER BY urgency_score DESC`, incidentID)
	if err != nil {
		return nil, fmt.Errorf("list by incident: %w", err)
	}
	defer rows.Close()

	var recs []Recommendation
	for rows.Next() {
		var r Recommendation
		if err := rows.Scan(
			&r.ID, &r.RuleID, &r.InstanceID, &r.AlertEventID,
			&r.MetricKey, &r.MetricValue, &r.Priority, &r.Category,
			&r.Title, &r.Description, &r.DocURL, &r.Status,
			&r.Source, &r.UrgencyScore, &r.IncidentIDs, &r.LastIncidentAt,
			&r.CreatedAt, &r.EvaluatedAt, &r.ResolvedAt,
			&r.AcknowledgedAt, &r.AcknowledgedBy,
		); err != nil {
			return nil, fmt.Errorf("scan recommendation: %w", err)
		}
		recs = append(recs, r)
	}
	return recs, rows.Err()
}

func (s *PGStore) Acknowledge(ctx context.Context, id int64, username string) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE remediation_recommendations
		 SET acknowledged_at = NOW(), acknowledged_by = $2
		 WHERE id = $1`, id, username)
	if err != nil {
		return fmt.Errorf("acknowledge recommendation: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("recommendation %d not found", id)
	}
	return nil
}

func (s *PGStore) CleanOld(ctx context.Context, olderThan time.Duration) error {
	_, err := s.pool.Exec(ctx,
		`DELETE FROM remediation_recommendations
		 WHERE created_at < NOW() - $1::interval
		   AND (acknowledged_at IS NOT NULL OR status = 'resolved')`, olderThan.String())
	if err != nil {
		return fmt.Errorf("clean old recommendations: %w", err)
	}
	return nil
}

// ResolveStale marks active recommendations as resolved when they no longer
// appear in the current evaluation cycle for an instance.
func (s *PGStore) ResolveStale(ctx context.Context, instanceID string, currentRuleIDs []string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE remediation_recommendations
		 SET status = 'resolved', resolved_at = NOW()
		 WHERE instance_id = $1
		   AND status = 'active'
		   AND rule_id != ALL($2::text[])`,
		instanceID, currentRuleIDs)
	if err != nil {
		return fmt.Errorf("resolve stale recommendations for %s: %w", instanceID, err)
	}
	return nil
}
