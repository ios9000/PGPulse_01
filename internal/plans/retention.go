package plans

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// RetentionWorker periodically deletes expired query plans.
type RetentionWorker struct {
	pool     *pgxpool.Pool
	planDays int
	interval time.Duration
}

// NewRetentionWorker creates a retention cleanup worker.
func NewRetentionWorker(pool *pgxpool.Pool, planDays int) *RetentionWorker {
	return &RetentionWorker{pool: pool, planDays: planDays, interval: time.Hour}
}

// Run blocks and runs retention cleanup hourly until ctx is cancelled.
func (w *RetentionWorker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cutoff := time.Now().AddDate(0, 0, -w.planDays)
			result, err := w.pool.Exec(ctx,
				"DELETE FROM query_plans WHERE captured_at < $1", cutoff)
			if err != nil {
				slog.Warn("plan retention cleanup failed", "err", err)
				continue
			}
			if result.RowsAffected() > 0 {
				slog.Info("plan retention: deleted old plans", "count", result.RowsAffected())
			}
		}
	}
}
