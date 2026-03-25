package playbook

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// ConnProvider provides connections to monitored PostgreSQL instances.
// Mirrors api.InstanceConnProvider to avoid import cycles.
type ConnProvider interface {
	ConnFor(ctx context.Context, instanceID string) (*pgx.Conn, error)
}

// Executor safely runs playbook step SQL against monitored instances.
type Executor struct {
	connProv ConnProvider
	rowLimit int
	logger   *slog.Logger
}

// NewExecutor creates an Executor with the given connection provider and config.
func NewExecutor(connProv ConnProvider, cfg PlaybookConfig, logger *slog.Logger) *Executor {
	limit := cfg.ResultRowLimit
	if limit == 0 {
		limit = 100
	}
	return &Executor{
		connProv: connProv,
		rowLimit: limit,
		logger:   logger,
	}
}

// ExecuteStep runs a single step's SQL against the target instance.
// All execution uses transaction-scoped SET LOCAL (C1).
// Multi-statement SQL is rejected (C2).
// Tier 4 (external) steps cannot be executed.
func (e *Executor) ExecuteStep(ctx context.Context, instanceID string, step Step) (*ExecutionResult, error) {
	// Tier 4: manual action only.
	if step.SafetyTier == TierExternal {
		return nil, fmt.Errorf("external steps cannot be executed — manual action required")
	}

	// C2: Multi-statement injection guard.
	trimmed := strings.TrimSpace(step.SQLTemplate)
	trimmed = strings.TrimRight(trimmed, ";")
	if strings.Contains(trimmed, ";") {
		return nil, fmt.Errorf("multi-statement SQL is forbidden in playbook steps")
	}

	// Get connection to the target instance.
	conn, err := e.connProv.ConnFor(ctx, instanceID)
	if err != nil {
		return nil, fmt.Errorf("cannot connect to instance %s: %w", instanceID, err)
	}
	defer func() { _ = conn.Close(ctx) }()

	// C1: Transaction-scoped execution with SET LOCAL.
	tx, err := conn.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Set timeouts.
	timeout := step.TimeoutSeconds
	if timeout == 0 {
		timeout = 5
	}
	if _, err := tx.Exec(ctx, fmt.Sprintf("SET LOCAL statement_timeout = '%ds'", timeout)); err != nil {
		return nil, fmt.Errorf("set statement_timeout: %w", err)
	}
	if _, err := tx.Exec(ctx, "SET LOCAL lock_timeout = '5s'"); err != nil {
		return nil, fmt.Errorf("set lock_timeout: %w", err)
	}

	// Tier 1: enforce READ ONLY.
	if step.SafetyTier == TierDiagnostic {
		if _, err := tx.Exec(ctx, "SET LOCAL default_transaction_read_only = ON"); err != nil {
			return nil, fmt.Errorf("set read_only: %w", err)
		}
	}

	// Execute the step SQL.
	start := time.Now()
	rows, err := tx.Query(ctx, step.SQLTemplate)
	if err != nil {
		return nil, fmt.Errorf("step execution failed: %w", err)
	}
	defer rows.Close()

	// Collect results with row limit (C8).
	columns := make([]string, 0)
	for _, fd := range rows.FieldDescriptions() {
		columns = append(columns, fd.Name)
	}

	var resultRows [][]any
	totalRows := 0
	for rows.Next() {
		totalRows++
		if totalRows <= e.rowLimit {
			vals, scanErr := rows.Values()
			if scanErr != nil {
				e.logger.Warn("failed to scan row", "error", scanErr)
				continue
			}
			resultRows = append(resultRows, vals)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration: %w", err)
	}

	return &ExecutionResult{
		Columns:   columns,
		Rows:      resultRows,
		RowCount:  len(resultRows),
		TotalRows: totalRows,
		Truncated: totalRows > e.rowLimit,
		Duration:  int(time.Since(start).Milliseconds()),
	}, nil
}
