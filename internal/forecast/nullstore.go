package forecast

import (
	"context"
	"time"
)

// NullForecastStore is a no-op implementation for when forecast is disabled.
type NullForecastStore struct{}

func (n *NullForecastStore) WriteOperation(_ context.Context, _ *MaintenanceOperation) error {
	return nil
}

func (n *NullForecastStore) ListOperations(_ context.Context, _ OperationFilter) ([]MaintenanceOperation, int, error) {
	return nil, 0, nil
}

func (n *NullForecastStore) CleanOldOperations(_ context.Context, _ time.Time) (int64, error) {
	return 0, nil
}

func (n *NullForecastStore) UpsertForecast(_ context.Context, _ *MaintenanceForecast) error {
	return nil
}

func (n *NullForecastStore) UpsertForecasts(_ context.Context, _ []MaintenanceForecast) error {
	return nil
}

func (n *NullForecastStore) ListForecasts(_ context.Context, _ ForecastFilter) ([]MaintenanceForecast, error) {
	return nil, nil
}

func (n *NullForecastStore) DeleteForecasts(_ context.Context, _ string) error {
	return nil
}

func (n *NullForecastStore) Close() error {
	return nil
}
