package alert

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

const defaultBufferSize = 100

// Dispatcher receives alert events and delivers them to registered notifiers.
type Dispatcher struct {
	registry        *NotifierRegistry
	defaultChannels []string
	cooldownMinutes int
	logger          *slog.Logger
	retryDelays     []time.Duration // configurable for testing

	events chan AlertEvent
	done   chan struct{}
	wg     sync.WaitGroup

	mu        sync.Mutex
	cooldowns map[string]time.Time

	remediation RemediationProvider // nil = disabled
}

// NewDispatcher creates a dispatcher that delivers events via registered notifiers.
func NewDispatcher(
	registry *NotifierRegistry,
	defaultChannels []string,
	defaultCooldownMinutes int,
	logger *slog.Logger,
) *Dispatcher {
	return &Dispatcher{
		registry:        registry,
		defaultChannels: defaultChannels,
		cooldownMinutes: defaultCooldownMinutes,
		logger:          logger,
		retryDelays:     []time.Duration{1 * time.Second, 2 * time.Second, 4 * time.Second},
		events:          make(chan AlertEvent, defaultBufferSize),
		done:            make(chan struct{}),
		cooldowns:       make(map[string]time.Time),
	}
}

// SetRemediationProvider wires the remediation engine into the dispatcher.
func (d *Dispatcher) SetRemediationProvider(p RemediationProvider) {
	d.remediation = p
}

// Start begins the background event processing loop.
func (d *Dispatcher) Start() {
	d.wg.Add(1)
	go d.run()
	d.logger.Info("alert dispatcher started", "buffer_size", defaultBufferSize)
}

// Dispatch enqueues an event. Non-blocking. Returns false if buffer is full.
func (d *Dispatcher) Dispatch(event AlertEvent) bool {
	select {
	case d.events <- event:
		return true
	default:
		d.logger.Warn("alert dispatcher buffer full, dropping event",
			"rule", event.RuleID, "instance", event.InstanceID)
		return false
	}
}

// Stop closes the event channel and waits for all pending events to drain.
func (d *Dispatcher) Stop() {
	close(d.events)
	d.wg.Wait()
	d.logger.Info("alert dispatcher stopped")
}

func (d *Dispatcher) run() {
	defer d.wg.Done()
	for event := range d.events {
		d.processEvent(event)
	}
}

func (d *Dispatcher) processEvent(event AlertEvent) {
	if !event.IsResolution && d.isCoolingDown(event) {
		d.logger.Debug("alert notification suppressed by cooldown",
			"rule", event.RuleID, "instance", event.InstanceID)
		return
	}

	// Run remediation BEFORE notifications so recommendations are available in templates.
	if !event.IsResolution {
		event.Recommendations = d.runRemediation(event)
	}

	channels := d.resolveChannels(event)
	if len(channels) == 0 {
		d.logger.Warn("no notification channels resolved",
			"rule", event.RuleID)
		return
	}

	for _, chName := range channels {
		n := d.registry.Get(chName)
		if n == nil {
			d.logger.Warn("notifier not registered", "channel", chName, "rule", event.RuleID)
			continue
		}
		d.sendWithRetry(n, event)
	}

	if !event.IsResolution {
		d.recordCooldown(event)
	}
}

func (d *Dispatcher) isCoolingDown(event AlertEvent) bool {
	key := cooldownKey(event.RuleID, event.InstanceID, string(event.Severity))
	cooldownDuration := time.Duration(d.cooldownMinutes) * time.Minute

	d.mu.Lock()
	lastNotified, exists := d.cooldowns[key]
	d.mu.Unlock()

	return exists && time.Since(lastNotified) < cooldownDuration
}

func (d *Dispatcher) recordCooldown(event AlertEvent) {
	key := cooldownKey(event.RuleID, event.InstanceID, string(event.Severity))
	d.mu.Lock()
	d.cooldowns[key] = time.Now()
	d.mu.Unlock()
}

func (d *Dispatcher) resolveChannels(event AlertEvent) []string {
	if len(event.Channels) > 0 {
		return event.Channels
	}
	return d.defaultChannels
}

func (d *Dispatcher) sendWithRetry(n Notifier, event AlertEvent) {
	maxAttempts := len(d.retryDelays)
	if maxAttempts == 0 {
		maxAttempts = 1
	}

	for attempt := 0; attempt < maxAttempts; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		err := n.Send(ctx, event)
		cancel()

		if err == nil {
			d.logger.Info("alert notification sent",
				"channel", n.Name(), "rule", event.RuleID,
				"instance", event.InstanceID, "attempt", attempt+1)
			return
		}

		d.logger.Warn("alert notification failed",
			"channel", n.Name(), "rule", event.RuleID,
			"attempt", attempt+1, "error", err)

		// Sleep before next retry (not after last attempt).
		if attempt < maxAttempts-1 {
			time.Sleep(d.retryDelays[attempt])
		}
	}

	d.logger.Error("alert notification exhausted retries",
		"channel", n.Name(), "rule", event.RuleID,
		"instance", event.InstanceID, "severity", event.Severity)
}

func (d *Dispatcher) runRemediation(event AlertEvent) []RemediationResult {
	if d.remediation == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	results := d.remediation.EvaluateForAlert(ctx, event.InstanceID, event.Metric, event.Value, event.Labels, string(event.Severity))
	if len(results) > 0 {
		d.logger.Info("remediation recommendations generated",
			"rule", event.RuleID, "instance", event.InstanceID, "count", len(results))
	}
	return results
}

func cooldownKey(ruleID, instanceID, severity string) string {
	return ruleID + ":" + instanceID + ":" + severity
}
