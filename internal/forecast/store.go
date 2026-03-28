package forecast

import (
	"context"
	"time"
)

// ForecastStore persists completed operation history and cached forecasts.
type ForecastStore interface {
	// --- Operation history ---
	WriteOperation(ctx context.Context, op *MaintenanceOperation) error
	ListOperations(ctx context.Context, filter OperationFilter) ([]MaintenanceOperation, int, error)
	CleanOldOperations(ctx context.Context, olderThan time.Time) (int64, error)

	// --- Forecasts ---
	UpsertForecast(ctx context.Context, f *MaintenanceForecast) error
	UpsertForecasts(ctx context.Context, forecasts []MaintenanceForecast) error
	ListForecasts(ctx context.Context, filter ForecastFilter) ([]MaintenanceForecast, error)
	DeleteForecasts(ctx context.Context, instanceID string) error

	// --- Cleanup ---
	Close() error
}

// OperationFilter controls history queries.
type OperationFilter struct {
	InstanceID string
	Operation  string // empty = all
	Database   string // empty = all
	Table      string // empty = all
	Since      time.Time
	Limit      int
	Offset     int
}

// ForecastFilter controls forecast queries.
type ForecastFilter struct {
	InstanceID string
	Operation  string   // empty = all
	Database   string
	Table      string
	Statuses   []string // empty = all; e.g. ["imminent", "overdue"]
}
