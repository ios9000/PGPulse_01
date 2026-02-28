package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ios9000/PGPulse_01/internal/collector"
)

// PGStore implements collector.MetricStore using PostgreSQL (with optional TimescaleDB).
type PGStore struct {
	pool   *pgxpool.Pool
	logger *slog.Logger
}

// NewPGStore creates a PGStore backed by the given connection pool.
func NewPGStore(pool *pgxpool.Pool, logger *slog.Logger) *PGStore {
	return &PGStore{pool: pool, logger: logger}
}

// Write persists a batch of metric points via the PostgreSQL COPY protocol.
func (s *PGStore) Write(ctx context.Context, points []collector.MetricPoint) error {
	if len(points) == 0 {
		return nil
	}

	wCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	columns := []string{"time", "instance_id", "metric", "value", "labels"}
	rows := make([][]any, 0, len(points))

	for _, p := range points {
		if p.Labels == nil {
			p.Labels = map[string]string{}
		}
		labelsJSON, err := json.Marshal(p.Labels)
		if err != nil {
			return fmt.Errorf("marshal labels for metric %q: %w", p.Metric, err)
		}
		rows = append(rows, []any{
			p.Timestamp, p.InstanceID, p.Metric, p.Value, string(labelsJSON),
		})
	}

	_, err := s.pool.CopyFrom(
		wCtx,
		pgx.Identifier{"metrics"},
		columns,
		pgx.CopyFromRows(rows),
	)
	if err != nil {
		return fmt.Errorf("write metrics: %w", err)
	}

	s.logger.Debug("wrote metric points", "count", len(points))
	return nil
}

// Query retrieves metric points matching the query parameters.
// Always returns a non-nil slice (empty when no results).
func (s *PGStore) Query(ctx context.Context, q collector.MetricQuery) ([]collector.MetricPoint, error) {
	qCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	sql, args := buildQuery(q)

	rows, err := s.pool.Query(qCtx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("query metrics: %w", err)
	}
	defer rows.Close()

	points := []collector.MetricPoint{}
	for rows.Next() {
		var p collector.MetricPoint
		var labelsJSON []byte
		if err := rows.Scan(&p.Timestamp, &p.InstanceID, &p.Metric, &p.Value, &labelsJSON); err != nil {
			return nil, fmt.Errorf("scan metric row: %w", err)
		}
		if err := json.Unmarshal(labelsJSON, &p.Labels); err != nil {
			return nil, fmt.Errorf("unmarshal labels: %w", err)
		}
		points = append(points, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("query metrics rows: %w", err)
	}

	return points, nil
}

// Pool returns the underlying connection pool. Used by health checks.
func (s *PGStore) Pool() *pgxpool.Pool {
	return s.pool
}

// Close releases the connection pool.
func (s *PGStore) Close() error {
	s.pool.Close()
	return nil
}

// buildQuery constructs the SELECT SQL and positional args for a MetricQuery.
// Kept unexported; tests are in the same package.
func buildQuery(q collector.MetricQuery) (string, []any) {
	var conditions []string
	var args []any
	n := 1

	if q.InstanceID != "" {
		conditions = append(conditions, fmt.Sprintf("instance_id = $%d", n))
		args = append(args, q.InstanceID)
		n++
	}
	if q.Metric != "" {
		conditions = append(conditions, fmt.Sprintf("metric LIKE $%d", n))
		args = append(args, q.Metric+"%")
		n++
	}
	if !q.Start.IsZero() {
		conditions = append(conditions, fmt.Sprintf("time >= $%d", n))
		args = append(args, q.Start)
		n++
	}
	if !q.End.IsZero() {
		conditions = append(conditions, fmt.Sprintf("time <= $%d", n))
		args = append(args, q.End)
		n++
	}
	if len(q.Labels) > 0 {
		labelsJSON, _ := json.Marshal(q.Labels)
		conditions = append(conditions, fmt.Sprintf("labels @> $%d::jsonb", n))
		args = append(args, string(labelsJSON))
		n++
	}

	sql := "SELECT time, instance_id, metric, value, labels FROM metrics"
	if len(conditions) > 0 {
		sql += " WHERE " + strings.Join(conditions, " AND ")
	}
	sql += " ORDER BY time DESC"

	if q.Limit > 0 {
		sql += fmt.Sprintf(" LIMIT $%d", n)
		args = append(args, q.Limit)
	}

	return sql, args
}
