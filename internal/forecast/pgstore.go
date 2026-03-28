package forecast

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PGForecastStore implements ForecastStore using pgx.
type PGForecastStore struct {
	pool   *pgxpool.Pool
	logger *slog.Logger
}

// NewPGForecastStore creates a forecast store backed by the given pool.
func NewPGForecastStore(pool *pgxpool.Pool, logger *slog.Logger) *PGForecastStore {
	return &PGForecastStore{pool: pool, logger: logger}
}

// WriteOperation inserts a completed maintenance operation record.
func (s *PGForecastStore) WriteOperation(ctx context.Context, op *MaintenanceOperation) error {
	qCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	metadata, err := json.Marshal(op.Metadata)
	if err != nil {
		metadata = []byte("{}")
	}

	err = s.pool.QueryRow(qCtx, `
		INSERT INTO maintenance_operations
			(instance_id, operation, outcome, database, table_name, table_size_bytes,
			 started_at, completed_at, duration_sec, final_pct, avg_rate_per_sec, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING id`,
		op.InstanceID, op.Operation, op.Outcome, op.Database, op.Table, op.TableSizeBytes,
		op.StartedAt, op.CompletedAt, op.DurationSec, op.FinalPct, op.AvgRatePerSec, metadata,
	).Scan(&op.ID)
	if err != nil {
		return fmt.Errorf("forecast: write operation: %w", err)
	}
	return nil
}

// ListOperations returns filtered operation history with pagination.
func (s *PGForecastStore) ListOperations(ctx context.Context, filter OperationFilter) ([]MaintenanceOperation, int, error) {
	qCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	since := filter.Since
	if since.IsZero() {
		since = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}

	// Count total matching rows.
	var total int
	err := s.pool.QueryRow(qCtx, `
		SELECT COUNT(*) FROM maintenance_operations
		WHERE instance_id = $1
		  AND ($2 = '' OR operation = $2)
		  AND ($3 = '' OR database = $3)
		  AND ($4 = '' OR table_name = $4)
		  AND completed_at >= $5`,
		filter.InstanceID, filter.Operation, filter.Database, filter.Table, since,
	).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("forecast: count operations: %w", err)
	}

	rows, err := s.pool.Query(qCtx, `
		SELECT id, instance_id, operation, outcome, database, table_name, table_size_bytes,
		       started_at, completed_at, duration_sec, final_pct, avg_rate_per_sec,
		       metadata, created_at
		FROM maintenance_operations
		WHERE instance_id = $1
		  AND ($2 = '' OR operation = $2)
		  AND ($3 = '' OR database = $3)
		  AND ($4 = '' OR table_name = $4)
		  AND completed_at >= $5
		ORDER BY completed_at DESC
		LIMIT $6 OFFSET $7`,
		filter.InstanceID, filter.Operation, filter.Database, filter.Table,
		since, limit, filter.Offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("forecast: list operations: %w", err)
	}
	defer rows.Close()

	var ops []MaintenanceOperation
	for rows.Next() {
		var op MaintenanceOperation
		var metadataBytes []byte
		if err := rows.Scan(
			&op.ID, &op.InstanceID, &op.Operation, &op.Outcome, &op.Database, &op.Table,
			&op.TableSizeBytes, &op.StartedAt, &op.CompletedAt, &op.DurationSec,
			&op.FinalPct, &op.AvgRatePerSec, &metadataBytes, &op.CreatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("forecast: scan operation: %w", err)
		}
		if len(metadataBytes) > 0 {
			_ = json.Unmarshal(metadataBytes, &op.Metadata)
		}
		if op.Metadata == nil {
			op.Metadata = make(map[string]any)
		}
		ops = append(ops, op)
	}
	if ops == nil {
		ops = []MaintenanceOperation{}
	}
	return ops, total, rows.Err()
}

// CleanOldOperations deletes operation records older than the given time.
func (s *PGForecastStore) CleanOldOperations(ctx context.Context, olderThan time.Time) (int64, error) {
	qCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	tag, err := s.pool.Exec(qCtx,
		`DELETE FROM maintenance_operations WHERE completed_at < $1`, olderThan)
	if err != nil {
		return 0, fmt.Errorf("forecast: clean operations: %w", err)
	}
	return tag.RowsAffected(), nil
}

// UpsertForecast inserts or updates a single forecast row.
func (s *PGForecastStore) UpsertForecast(ctx context.Context, f *MaintenanceForecast) error {
	qCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	_, err := s.pool.Exec(qCtx, `
		INSERT INTO maintenance_forecasts
			(instance_id, database, table_name, operation, status, predicted_at,
			 time_until_sec, confidence_lower, confidence_upper, current_value,
			 threshold_value, accumulation_rate, method, evaluated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		ON CONFLICT (instance_id, database, table_name, operation) DO UPDATE SET
			status = EXCLUDED.status,
			predicted_at = EXCLUDED.predicted_at,
			time_until_sec = EXCLUDED.time_until_sec,
			confidence_lower = EXCLUDED.confidence_lower,
			confidence_upper = EXCLUDED.confidence_upper,
			current_value = EXCLUDED.current_value,
			threshold_value = EXCLUDED.threshold_value,
			accumulation_rate = EXCLUDED.accumulation_rate,
			method = EXCLUDED.method,
			evaluated_at = EXCLUDED.evaluated_at`,
		f.InstanceID, f.Database, f.Table, f.Operation, f.Status, f.PredictedAt,
		f.TimeUntilSec, f.ConfidenceLower, f.ConfidenceUpper, f.CurrentValue,
		f.ThresholdValue, f.AccumulationRate, f.Method, f.EvaluatedAt,
	)
	if err != nil {
		return fmt.Errorf("forecast: upsert forecast: %w", err)
	}
	return nil
}

// UpsertForecasts batch-upserts forecasts within a single transaction.
func (s *PGForecastStore) UpsertForecasts(ctx context.Context, forecasts []MaintenanceForecast) error {
	if len(forecasts) == 0 {
		return nil
	}

	qCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	tx, err := s.pool.Begin(qCtx)
	if err != nil {
		return fmt.Errorf("forecast: begin batch upsert: %w", err)
	}
	defer tx.Rollback(qCtx) //nolint:errcheck

	for i := range forecasts {
		f := &forecasts[i]
		_, err := tx.Exec(qCtx, `
			INSERT INTO maintenance_forecasts
				(instance_id, database, table_name, operation, status, predicted_at,
				 time_until_sec, confidence_lower, confidence_upper, current_value,
				 threshold_value, accumulation_rate, method, evaluated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
			ON CONFLICT (instance_id, database, table_name, operation) DO UPDATE SET
				status = EXCLUDED.status,
				predicted_at = EXCLUDED.predicted_at,
				time_until_sec = EXCLUDED.time_until_sec,
				confidence_lower = EXCLUDED.confidence_lower,
				confidence_upper = EXCLUDED.confidence_upper,
				current_value = EXCLUDED.current_value,
				threshold_value = EXCLUDED.threshold_value,
				accumulation_rate = EXCLUDED.accumulation_rate,
				method = EXCLUDED.method,
				evaluated_at = EXCLUDED.evaluated_at`,
			f.InstanceID, f.Database, f.Table, f.Operation, f.Status, f.PredictedAt,
			f.TimeUntilSec, f.ConfidenceLower, f.ConfidenceUpper, f.CurrentValue,
			f.ThresholdValue, f.AccumulationRate, f.Method, f.EvaluatedAt,
		)
		if err != nil {
			return fmt.Errorf("forecast: upsert forecast[%d]: %w", i, err)
		}
	}

	return tx.Commit(qCtx)
}

// ListForecasts returns filtered forecasts ordered by status priority.
func (s *PGForecastStore) ListForecasts(ctx context.Context, filter ForecastFilter) ([]MaintenanceForecast, error) {
	qCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	var statuses []string
	if len(filter.Statuses) > 0 {
		statuses = filter.Statuses
	}

	rows, err := s.pool.Query(qCtx, `
		SELECT id, instance_id, database, table_name, operation, status,
		       predicted_at, time_until_sec, confidence_lower, confidence_upper,
		       current_value, threshold_value, accumulation_rate, method, evaluated_at
		FROM maintenance_forecasts
		WHERE instance_id = $1
		  AND ($2 = '' OR operation = $2)
		  AND ($3 = '' OR database = $3)
		  AND ($4 = '' OR table_name = $4)
		  AND (array_length($5::text[], 1) IS NULL OR status = ANY($5))
		ORDER BY
			CASE status
				WHEN 'overdue' THEN 1
				WHEN 'imminent' THEN 2
				WHEN 'predicted' THEN 3
				WHEN 'insufficient_data' THEN 4
				WHEN 'not_needed' THEN 5
			END,
			time_until_sec ASC NULLS LAST`,
		filter.InstanceID, filter.Operation, filter.Database, filter.Table, statuses,
	)
	if err != nil {
		return nil, fmt.Errorf("forecast: list forecasts: %w", err)
	}
	defer rows.Close()

	var forecasts []MaintenanceForecast
	for rows.Next() {
		var f MaintenanceForecast
		if err := rows.Scan(
			&f.ID, &f.InstanceID, &f.Database, &f.Table, &f.Operation, &f.Status,
			&f.PredictedAt, &f.TimeUntilSec, &f.ConfidenceLower, &f.ConfidenceUpper,
			&f.CurrentValue, &f.ThresholdValue, &f.AccumulationRate, &f.Method, &f.EvaluatedAt,
		); err != nil {
			return nil, fmt.Errorf("forecast: scan forecast: %w", err)
		}
		forecasts = append(forecasts, f)
	}
	if forecasts == nil {
		forecasts = []MaintenanceForecast{}
	}
	return forecasts, rows.Err()
}

// DeleteForecasts removes all forecasts for the given instance.
func (s *PGForecastStore) DeleteForecasts(ctx context.Context, instanceID string) error {
	qCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	_, err := s.pool.Exec(qCtx,
		`DELETE FROM maintenance_forecasts WHERE instance_id = $1`, instanceID)
	if err != nil {
		return fmt.Errorf("forecast: delete forecasts: %w", err)
	}
	return nil
}

// Close is a no-op — the pool is owned by the caller.
func (s *PGForecastStore) Close() error {
	return nil
}

// Compile-time interface check.
var _ ForecastStore = (*PGForecastStore)(nil)
var _ ForecastStore = (*NullForecastStore)(nil)

