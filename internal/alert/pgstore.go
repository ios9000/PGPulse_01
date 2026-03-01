package alert

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrRuleNotFound is returned when a rule lookup finds no matching row.
var ErrRuleNotFound = errors.New("alert rule not found")

// PGAlertRuleStore implements AlertRuleStore backed by PostgreSQL.
type PGAlertRuleStore struct {
	pool *pgxpool.Pool
}

// NewPGAlertRuleStore creates a PGAlertRuleStore using the given connection pool.
func NewPGAlertRuleStore(pool *pgxpool.Pool) *PGAlertRuleStore {
	return &PGAlertRuleStore{pool: pool}
}

const ruleColumns = `id, name, description, metric, operator, threshold, severity,
	labels, consecutive_count, cooldown_minutes, channels, source, enabled,
	created_at, updated_at`

// scanRule scans a single rule row into a Rule struct.
func scanRule(row pgx.Row) (*Rule, error) {
	var r Rule
	var labelsJSON, channelsJSON []byte
	var createdAt, updatedAt time.Time

	err := row.Scan(
		&r.ID, &r.Name, &r.Description, &r.Metric, &r.Operator,
		&r.Threshold, &r.Severity, &labelsJSON, &r.ConsecutiveCount,
		&r.CooldownMinutes, &channelsJSON, &r.Source, &r.Enabled,
		&createdAt, &updatedAt,
	)
	if err != nil {
		return nil, err
	}

	if len(labelsJSON) > 0 {
		if err := json.Unmarshal(labelsJSON, &r.Labels); err != nil {
			return nil, fmt.Errorf("unmarshal rule labels: %w", err)
		}
	}
	if len(channelsJSON) > 0 {
		if err := json.Unmarshal(channelsJSON, &r.Channels); err != nil {
			return nil, fmt.Errorf("unmarshal rule channels: %w", err)
		}
	}

	return &r, nil
}

// scanRules scans multiple rule rows.
func scanRules(rows pgx.Rows) ([]Rule, error) {
	var rules []Rule
	for rows.Next() {
		var r Rule
		var labelsJSON, channelsJSON []byte
		var createdAt, updatedAt time.Time

		err := rows.Scan(
			&r.ID, &r.Name, &r.Description, &r.Metric, &r.Operator,
			&r.Threshold, &r.Severity, &labelsJSON, &r.ConsecutiveCount,
			&r.CooldownMinutes, &channelsJSON, &r.Source, &r.Enabled,
			&createdAt, &updatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan rule row: %w", err)
		}

		if len(labelsJSON) > 0 {
			if err := json.Unmarshal(labelsJSON, &r.Labels); err != nil {
				return nil, fmt.Errorf("unmarshal rule labels: %w", err)
			}
		}
		if len(channelsJSON) > 0 {
			if err := json.Unmarshal(channelsJSON, &r.Channels); err != nil {
				return nil, fmt.Errorf("unmarshal rule channels: %w", err)
			}
		}

		rules = append(rules, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rule rows: %w", err)
	}
	return rules, nil
}

// List returns all alert rules ordered by id.
func (s *PGAlertRuleStore) List(ctx context.Context) ([]Rule, error) {
	rows, err := s.pool.Query(ctx,
		fmt.Sprintf(`SELECT %s FROM alert_rules ORDER BY id`, ruleColumns))
	if err != nil {
		return nil, fmt.Errorf("list rules: %w", err)
	}
	defer rows.Close()
	return scanRules(rows)
}

// ListEnabled returns all enabled alert rules ordered by id.
func (s *PGAlertRuleStore) ListEnabled(ctx context.Context) ([]Rule, error) {
	rows, err := s.pool.Query(ctx,
		fmt.Sprintf(`SELECT %s FROM alert_rules WHERE enabled = true ORDER BY id`, ruleColumns))
	if err != nil {
		return nil, fmt.Errorf("list enabled rules: %w", err)
	}
	defer rows.Close()
	return scanRules(rows)
}

// Get returns a single rule by id. Returns ErrRuleNotFound if absent.
func (s *PGAlertRuleStore) Get(ctx context.Context, id string) (*Rule, error) {
	row := s.pool.QueryRow(ctx,
		fmt.Sprintf(`SELECT %s FROM alert_rules WHERE id = $1`, ruleColumns), id)
	r, err := scanRule(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrRuleNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get rule %s: %w", id, err)
	}
	return r, nil
}

// Create inserts a new alert rule.
func (s *PGAlertRuleStore) Create(ctx context.Context, rule *Rule) error {
	labelsJSON, err := json.Marshal(rule.Labels)
	if err != nil {
		return fmt.Errorf("marshal labels: %w", err)
	}
	channelsJSON, err := json.Marshal(rule.Channels)
	if err != nil {
		return fmt.Errorf("marshal channels: %w", err)
	}

	_, err = s.pool.Exec(ctx,
		`INSERT INTO alert_rules (id, name, description, metric, operator, threshold,
			severity, labels, consecutive_count, cooldown_minutes, channels, source, enabled)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
		rule.ID, rule.Name, rule.Description, rule.Metric, rule.Operator,
		rule.Threshold, rule.Severity, labelsJSON, rule.ConsecutiveCount,
		rule.CooldownMinutes, channelsJSON, rule.Source, rule.Enabled,
	)
	if err != nil {
		return fmt.Errorf("create rule: %w", err)
	}
	return nil
}

// Update modifies a mutable rule fields.
func (s *PGAlertRuleStore) Update(ctx context.Context, rule *Rule) error {
	labelsJSON, err := json.Marshal(rule.Labels)
	if err != nil {
		return fmt.Errorf("marshal labels: %w", err)
	}
	channelsJSON, err := json.Marshal(rule.Channels)
	if err != nil {
		return fmt.Errorf("marshal channels: %w", err)
	}

	_, err = s.pool.Exec(ctx,
		`UPDATE alert_rules SET
			name = $2, description = $3, metric = $4, operator = $5,
			threshold = $6, severity = $7, labels = $8, consecutive_count = $9,
			cooldown_minutes = $10, channels = $11, enabled = $12, updated_at = now()
		 WHERE id = $1`,
		rule.ID, rule.Name, rule.Description, rule.Metric, rule.Operator,
		rule.Threshold, rule.Severity, labelsJSON, rule.ConsecutiveCount,
		rule.CooldownMinutes, channelsJSON, rule.Enabled,
	)
	if err != nil {
		return fmt.Errorf("update rule: %w", err)
	}
	return nil
}

// Delete removes a rule by id.
func (s *PGAlertRuleStore) Delete(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM alert_rules WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete rule: %w", err)
	}
	return nil
}

// UpsertBuiltin inserts or updates a builtin rule.
// Preserves user-modified fields: threshold, consecutive_count, cooldown_minutes, enabled, channels.
func (s *PGAlertRuleStore) UpsertBuiltin(ctx context.Context, rule *Rule) error {
	labelsJSON, err := json.Marshal(rule.Labels)
	if err != nil {
		return fmt.Errorf("marshal labels: %w", err)
	}
	channelsJSON, err := json.Marshal(rule.Channels)
	if err != nil {
		return fmt.Errorf("marshal channels: %w", err)
	}

	_, err = s.pool.Exec(ctx,
		`INSERT INTO alert_rules (id, name, description, metric, operator, threshold,
			severity, labels, consecutive_count, cooldown_minutes, channels, source, enabled)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		 ON CONFLICT (id) DO UPDATE SET
			name = EXCLUDED.name,
			description = EXCLUDED.description,
			metric = EXCLUDED.metric,
			operator = EXCLUDED.operator,
			severity = EXCLUDED.severity,
			labels = EXCLUDED.labels,
			updated_at = now()
		 WHERE alert_rules.source = 'builtin'`,
		rule.ID, rule.Name, rule.Description, rule.Metric, rule.Operator,
		rule.Threshold, rule.Severity, labelsJSON, rule.ConsecutiveCount,
		rule.CooldownMinutes, channelsJSON, rule.Source, rule.Enabled,
	)
	if err != nil {
		return fmt.Errorf("upsert builtin rule: %w", err)
	}
	return nil
}

// PGAlertHistoryStore implements AlertHistoryStore backed by PostgreSQL.
type PGAlertHistoryStore struct {
	pool *pgxpool.Pool
}

// NewPGAlertHistoryStore creates a PGAlertHistoryStore using the given connection pool.
func NewPGAlertHistoryStore(pool *pgxpool.Pool) *PGAlertHistoryStore {
	return &PGAlertHistoryStore{pool: pool}
}

// scanEvent scans a single alert history row into an AlertEvent struct.
func scanEvent(rows pgx.Rows) (AlertEvent, error) {
	var ev AlertEvent
	var labelsJSON []byte

	err := rows.Scan(
		&ev.RuleID, &ev.InstanceID, &ev.Severity, &ev.Metric,
		&ev.Value, &ev.Threshold, &ev.Operator, &labelsJSON,
		&ev.FiredAt, &ev.ResolvedAt,
	)
	if err != nil {
		return ev, err
	}

	if len(labelsJSON) > 0 {
		if err := json.Unmarshal(labelsJSON, &ev.Labels); err != nil {
			return ev, fmt.Errorf("unmarshal event labels: %w", err)
		}
	}

	ev.IsResolution = ev.ResolvedAt != nil
	return ev, nil
}

const eventColumns = `rule_id, instance_id, severity, metric, value, threshold,
	operator, labels, fired_at, resolved_at`

// Record inserts a new alert event into history.
func (s *PGAlertHistoryStore) Record(ctx context.Context, event *AlertEvent) error {
	labelsJSON, err := json.Marshal(event.Labels)
	if err != nil {
		return fmt.Errorf("marshal event labels: %w", err)
	}

	_, err = s.pool.Exec(ctx,
		`INSERT INTO alert_history (rule_id, instance_id, severity, metric, value,
			threshold, operator, labels, fired_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		event.RuleID, event.InstanceID, event.Severity, event.Metric,
		event.Value, event.Threshold, event.Operator, labelsJSON,
		event.FiredAt,
	)
	if err != nil {
		return fmt.Errorf("record alert event: %w", err)
	}
	return nil
}

// Resolve marks unresolved events for a rule+instance combination as resolved.
func (s *PGAlertHistoryStore) Resolve(ctx context.Context, ruleID, instanceID string, resolvedAt time.Time) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE alert_history SET resolved_at = $3
		 WHERE rule_id = $1 AND instance_id = $2 AND resolved_at IS NULL`,
		ruleID, instanceID, resolvedAt,
	)
	if err != nil {
		return fmt.Errorf("resolve alert: %w", err)
	}
	return nil
}

// ListUnresolved returns all unresolved alert events ordered by fired_at descending.
func (s *PGAlertHistoryStore) ListUnresolved(ctx context.Context) ([]AlertEvent, error) {
	rows, err := s.pool.Query(ctx,
		fmt.Sprintf(`SELECT %s FROM alert_history WHERE resolved_at IS NULL ORDER BY fired_at DESC`, eventColumns))
	if err != nil {
		return nil, fmt.Errorf("list unresolved alerts: %w", err)
	}
	defer rows.Close()

	var events []AlertEvent
	for rows.Next() {
		ev, err := scanEvent(rows)
		if err != nil {
			return nil, fmt.Errorf("scan unresolved event: %w", err)
		}
		events = append(events, ev)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("unresolved events rows: %w", err)
	}
	return events, nil
}

// Query returns alert events matching the given filters.
func (s *PGAlertHistoryStore) Query(ctx context.Context, q AlertHistoryQuery) ([]AlertEvent, error) {
	sql, args := buildHistoryQuery(q)

	rows, err := s.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("query alert history: %w", err)
	}
	defer rows.Close()

	var events []AlertEvent
	for rows.Next() {
		ev, err := scanEvent(rows)
		if err != nil {
			return nil, fmt.Errorf("scan history event: %w", err)
		}
		events = append(events, ev)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("history events rows: %w", err)
	}
	return events, nil
}

// Cleanup deletes resolved alert events older than the given duration.
func (s *PGAlertHistoryStore) Cleanup(ctx context.Context, olderThan time.Duration) (int64, error) {
	seconds := int64(olderThan.Seconds())
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM alert_history
		 WHERE resolved_at IS NOT NULL AND resolved_at < now() - ($1 || ' seconds')::interval`,
		fmt.Sprintf("%d", seconds),
	)
	if err != nil {
		return 0, fmt.Errorf("cleanup alert history: %w", err)
	}
	return tag.RowsAffected(), nil
}

// buildHistoryQuery constructs the SELECT SQL and positional args for an AlertHistoryQuery.
func buildHistoryQuery(q AlertHistoryQuery) (string, []any) {
	var conditions []string
	var args []any
	n := 1

	if q.InstanceID != "" {
		conditions = append(conditions, fmt.Sprintf("instance_id = $%d", n))
		args = append(args, q.InstanceID)
		n++
	}
	if q.RuleID != "" {
		conditions = append(conditions, fmt.Sprintf("rule_id = $%d", n))
		args = append(args, q.RuleID)
		n++
	}
	if q.Severity != "" {
		conditions = append(conditions, fmt.Sprintf("severity = $%d", n))
		args = append(args, string(q.Severity))
		n++
	}
	if !q.Start.IsZero() {
		conditions = append(conditions, fmt.Sprintf("fired_at >= $%d", n))
		args = append(args, q.Start)
		n++
	}
	if !q.End.IsZero() {
		conditions = append(conditions, fmt.Sprintf("fired_at <= $%d", n))
		args = append(args, q.End)
		n++
	}
	if q.UnresolvedOnly {
		conditions = append(conditions, "resolved_at IS NULL")
	}

	sql := fmt.Sprintf("SELECT %s FROM alert_history", eventColumns)
	if len(conditions) > 0 {
		sql += " WHERE " + strings.Join(conditions, " AND ")
	}
	sql += " ORDER BY fired_at DESC"

	if q.Limit > 0 {
		sql += fmt.Sprintf(" LIMIT $%d", n)
		args = append(args, q.Limit)
	}

	return sql, args
}
