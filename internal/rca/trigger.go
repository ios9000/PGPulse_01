package rca

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/ios9000/PGPulse_01/internal/alert"
)

// AutoTrigger registers an OnAlert hook that fires RCA analysis
// when a qualifying alert event occurs.
type AutoTrigger struct {
	engine    *Engine
	cfg       RCAConfig
	lastFired map[string]time.Time // "instanceID:metric" -> last trigger time
	mu        sync.Mutex
}

// NewAutoTrigger creates an auto-trigger bound to the given engine.
func NewAutoTrigger(engine *Engine, cfg RCAConfig) *AutoTrigger {
	return &AutoTrigger{
		engine:    engine,
		cfg:       cfg,
		lastFired: make(map[string]time.Time),
	}
}

// RegisterHook registers the auto-trigger on an alert dispatcher.
// The dispatcher parameter is typed as an interface to avoid importing
// the concrete Dispatcher type.
func (t *AutoTrigger) RegisterHook(dispatcher interface{ OnAlert(func(alert.AlertEvent)) }) {
	dispatcher.OnAlert(func(event alert.AlertEvent) {
		if !t.shouldTrigger(event) {
			return
		}
		go t.fire(event) //nolint:errcheck // fire logs errors internally
	})
}

// shouldTrigger checks severity and rate limits.
func (t *AutoTrigger) shouldTrigger(event alert.AlertEvent) bool {
	if event.IsResolution {
		return false
	}
	if severityRank(event.Severity) < severityRank(alert.Severity(t.cfg.AutoTriggerSeverity)) {
		return false
	}

	key := event.InstanceID + ":" + event.Metric
	t.mu.Lock()
	defer t.mu.Unlock()

	if last, ok := t.lastFired[key]; ok && time.Since(last) < 15*time.Minute {
		return false
	}
	t.lastFired[key] = time.Now()
	return true
}

// fire runs RCA analysis asynchronously with a timeout.
func (t *AutoTrigger) fire(event alert.AlertEvent) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := t.engine.Analyze(ctx, AnalyzeRequest{
		InstanceID:    event.InstanceID,
		TriggerMetric: event.Metric,
		TriggerValue:  event.Value,
		TriggerTime:   event.FiredAt,
		TriggerKind:   "alert",
	})
	if err != nil {
		slog.Error("auto-triggered RCA failed",
			"instance", event.InstanceID,
			"metric", event.Metric,
			"error", err)
	}
}

// severityRank returns a numeric rank for severity comparison.
func severityRank(s alert.Severity) int {
	switch s {
	case alert.SeverityInfo:
		return 1
	case alert.SeverityWarning:
		return 2
	case alert.SeverityCritical:
		return 3
	default:
		return 0
	}
}
