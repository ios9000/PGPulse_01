package orchestrator

import (
	"context"

	"github.com/ios9000/PGPulse_01/internal/alert"
	"github.com/ios9000/PGPulse_01/internal/collector"
)

// NoOpAlertEvaluator satisfies AlertEvaluator but discards all calls.
// Used when alerting is disabled so callers never need nil checks.
type NoOpAlertEvaluator struct{}

func (n *NoOpAlertEvaluator) Evaluate(_ context.Context, _ []collector.MetricPoint) ([]alert.AlertEvent, error) {
	return nil, nil
}

// NoOpAlertDispatcher satisfies AlertDispatcher but discards all events.
type NoOpAlertDispatcher struct{}

func (n *NoOpAlertDispatcher) Dispatch(_ alert.AlertEvent) bool {
	return true
}
