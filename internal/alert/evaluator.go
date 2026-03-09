package alert

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/ios9000/PGPulse_01/internal/collector"
	"github.com/ios9000/PGPulse_01/internal/mlerrors"
)

// Evaluator checks metric points against alert rules and manages state transitions.
type Evaluator struct {
	ruleStore    AlertRuleStore
	historyStore AlertHistoryStore
	logger       *slog.Logger

	mu    sync.Mutex
	rules []Rule
	state map[string]*stateEntry

	forecastProvider       ForecastProvider
	forecastMinConsecutive int
	forecastCooldown       map[string]time.Time // key → cooldown expiry
}

// NewEvaluator creates an Evaluator with the given stores.
func NewEvaluator(ruleStore AlertRuleStore, historyStore AlertHistoryStore, logger *slog.Logger) *Evaluator {
	return &Evaluator{
		ruleStore:    ruleStore,
		historyStore: historyStore,
		logger:       logger,
		state:        make(map[string]*stateEntry),
	}
}

// LoadRules fetches enabled rules from the rule store and replaces the in-memory set.
func (e *Evaluator) LoadRules(ctx context.Context) error {
	rules, err := e.ruleStore.ListEnabled(ctx)
	if err != nil {
		return fmt.Errorf("load enabled rules: %w", err)
	}

	e.mu.Lock()
	e.rules = rules
	e.mu.Unlock()

	e.logger.Info("loaded alert rules", "count", len(rules))
	return nil
}

// RestoreState seeds in-memory state from unresolved history events.
// Call once at startup after LoadRules to resume alerting state.
func (e *Evaluator) RestoreState(ctx context.Context) error {
	events, err := e.historyStore.ListUnresolved(ctx)
	if err != nil {
		return fmt.Errorf("restore alert state: %w", err)
	}

	e.mu.Lock()
	for _, ev := range events {
		key := stateKey(ev.RuleID, ev.InstanceID)
		e.state[key] = &stateEntry{
			State:          StateFiring,
			FiredAt:        ev.FiredAt,
			LastNotifiedAt: ev.FiredAt,
			Severity:       ev.Severity,
		}
	}
	e.mu.Unlock()

	e.logger.Info("restored alert state", "unresolved", len(events))
	return nil
}

// Evaluate processes a batch of metric points against all loaded rules.
// Returns alert events for any state transitions (firing or resolution).
func (e *Evaluator) Evaluate(ctx context.Context, points []collector.MetricPoint) ([]AlertEvent, error) {
	e.mu.Lock()
	rules := make([]Rule, len(e.rules))
	copy(rules, e.rules)
	e.mu.Unlock()

	if len(rules) == 0 || len(points) == 0 {
		return nil, nil
	}

	index := indexPoints(points)
	now := time.Now()
	var events []AlertEvent

	for _, rule := range rules {
		matched := findMatchingPoints(index, rule)

		if len(matched) == 0 {
			continue
		}

		for _, pt := range matched {
			key := stateKeyWithLabels(rule.ID, pt.InstanceID, rule.Labels)
			breached := rule.Operator.Compare(pt.Value, rule.Threshold)

			e.mu.Lock()
			entry, exists := e.state[key]
			if !exists {
				entry = &stateEntry{State: StateOK}
				e.state[key] = entry
			}

			event := e.processTransition(rule, pt, entry, breached, now)
			e.mu.Unlock()

			if event != nil {
				if err := e.recordEvent(ctx, event); err != nil {
					e.logger.Error("failed to record alert event",
						"rule", rule.ID, "instance", pt.InstanceID, "error", err)
					continue
				}
				events = append(events, *event)
			}
		}
	}

	// Run forecast-threshold alert evaluation after standard threshold checks.
	instanceIDs := uniqueInstances(points)
	for _, instID := range instanceIDs {
		if err := e.runForecastAlerts(ctx, instID); err != nil {
			e.logger.Warn("forecast alert evaluation error", "instance", instID, "err", err)
		}
	}

	return events, nil
}

// SetForecastProvider wires in the ML detector for forecast-threshold alert
// evaluation. Must be called before the first Evaluate() call.
// minConsecutive is the global default for rules that specify 0.
func (e *Evaluator) SetForecastProvider(fp ForecastProvider, minConsecutive int) {
	e.forecastProvider = fp
	e.forecastMinConsecutive = minConsecutive
	if e.forecastCooldown == nil {
		e.forecastCooldown = make(map[string]time.Time)
	}
}

// runForecastAlerts evaluates forecast_threshold rules for a single instance.
func (e *Evaluator) runForecastAlerts(ctx context.Context, instanceID string) error {
	if e.forecastProvider == nil {
		return nil
	}

	e.mu.Lock()
	rules := make([]Rule, 0)
	for _, r := range e.rules {
		if r.Type == RuleTypeForecastThreshold {
			rules = append(rules, r)
		}
	}
	e.mu.Unlock()

	now := time.Now()

	for _, rule := range rules {
		required := rule.ConsecutivePointsRequired
		if required <= 0 {
			required = e.forecastMinConsecutive
		}

		points, err := e.forecastProvider.ForecastForAlert(ctx, instanceID, rule.Metric, required+4)
		if err != nil {
			if errors.Is(err, mlerrors.ErrNotBootstrapped) || errors.Is(err, mlerrors.ErrNoBaseline) {
				continue
			}
			e.logger.Warn("forecast provider error", "rule", rule.ID, "err", err)
			continue
		}

		cooldownKey := fmt.Sprintf("forecast:%s:%s:%s", instanceID, rule.Metric, rule.ID)

		e.mu.Lock()
		if expiry, ok := e.forecastCooldown[cooldownKey]; ok && now.Before(expiry) {
			e.mu.Unlock()
			continue
		}
		e.mu.Unlock()

		consecutive := 0
		for _, pt := range points {
			val := pt.Value
			if rule.UseLowerBound {
				val = pt.Lower
			}
			if rule.Operator.Compare(val, rule.Threshold) {
				consecutive++
				if consecutive >= required {
					event := &AlertEvent{
						RuleID:     rule.ID,
						RuleName:   rule.Name,
						InstanceID: instanceID,
						Severity:   rule.Severity,
						Value:      val,
						Threshold:  rule.Threshold,
						Operator:   rule.Operator,
						Metric:     rule.Metric,
						Channels:   rule.Channels,
						FiredAt:    now,
					}
					if err := e.recordEvent(ctx, event); err != nil {
						e.logger.Error("failed to record forecast alert event",
							"rule", rule.ID, "instance", instanceID, "error", err)
					}

					cooldownDur := time.Duration(rule.CooldownMinutes) * time.Minute
					if cooldownDur <= 0 {
						cooldownDur = 15 * time.Minute
					}
					e.mu.Lock()
					e.forecastCooldown[cooldownKey] = now.Add(cooldownDur)
					e.mu.Unlock()
					break
				}
			} else {
				consecutive = 0
			}
		}
	}

	return nil
}

// uniqueInstances returns deduplicated instance IDs from the points slice.
func uniqueInstances(points []collector.MetricPoint) []string {
	seen := make(map[string]struct{}, len(points))
	var ids []string
	for _, p := range points {
		if _, ok := seen[p.InstanceID]; !ok {
			seen[p.InstanceID] = struct{}{}
			ids = append(ids, p.InstanceID)
		}
	}
	return ids
}

// processTransition implements the state machine: OK→PENDING→FIRING→OK.
// Must be called with e.mu held.
func (e *Evaluator) processTransition(rule Rule, pt collector.MetricPoint, entry *stateEntry, breached bool, now time.Time) *AlertEvent {
	switch entry.State {
	case StateOK:
		if breached {
			entry.State = StatePending
			entry.ConsecutiveCount = 1
			entry.Severity = rule.Severity

			if rule.ConsecutiveCount <= 1 {
				return e.fireAlert(rule, pt, entry, now)
			}
		}

	case StatePending:
		if breached {
			entry.ConsecutiveCount++
			if entry.ConsecutiveCount >= rule.ConsecutiveCount {
				return e.fireAlert(rule, pt, entry, now)
			}
		} else {
			entry.State = StateOK
			entry.ConsecutiveCount = 0
		}

	case StateFiring:
		if !breached {
			return e.resolveAlert(rule, pt, entry, now)
		}
		// Still firing — check cooldown for re-notification (not generating new events here).
	}

	return nil
}

// fireAlert transitions state to Firing and creates a fire event.
func (e *Evaluator) fireAlert(rule Rule, pt collector.MetricPoint, entry *stateEntry, now time.Time) *AlertEvent {
	entry.State = StateFiring
	entry.FiredAt = now
	entry.LastNotifiedAt = now

	return &AlertEvent{
		RuleID:     rule.ID,
		RuleName:   rule.Name,
		InstanceID: pt.InstanceID,
		Severity:   rule.Severity,
		Value:      pt.Value,
		Threshold:  rule.Threshold,
		Operator:   rule.Operator,
		Metric:     rule.Metric,
		Labels:     pt.Labels,
		Channels:   rule.Channels,
		FiredAt:    now,
	}
}

// resolveAlert transitions state to OK and creates a resolution event.
func (e *Evaluator) resolveAlert(rule Rule, pt collector.MetricPoint, entry *stateEntry, now time.Time) *AlertEvent {
	firedAt := entry.FiredAt

	entry.State = StateOK
	entry.ConsecutiveCount = 0

	return &AlertEvent{
		RuleID:       rule.ID,
		RuleName:     rule.Name,
		InstanceID:   pt.InstanceID,
		Severity:     rule.Severity,
		Value:        pt.Value,
		Threshold:    rule.Threshold,
		Operator:     rule.Operator,
		Metric:       rule.Metric,
		Labels:       pt.Labels,
		Channels:     rule.Channels,
		FiredAt:      firedAt,
		ResolvedAt:   &now,
		IsResolution: true,
	}
}

// recordEvent persists an alert event: fire → Record, resolution → Resolve.
func (e *Evaluator) recordEvent(ctx context.Context, event *AlertEvent) error {
	if event.IsResolution {
		return e.historyStore.Resolve(ctx, event.RuleID, event.InstanceID, *event.ResolvedAt)
	}
	return e.historyStore.Record(ctx, event)
}

// stateKey returns the composite key for per-rule, per-instance state.
func stateKey(ruleID, instanceID string) string {
	return ruleID + ":" + instanceID
}

// stateKeyWithLabels appends sorted label values for per-label state tracking.
func stateKeyWithLabels(ruleID, instanceID string, labels map[string]string) string {
	key := ruleID + ":" + instanceID
	if len(labels) > 0 {
		keys := make([]string, 0, len(labels))
		for k := range labels {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			key += ":" + k + "=" + labels[k]
		}
	}
	return key
}

// indexPoints builds a map from metric name to matching points.
func indexPoints(points []collector.MetricPoint) map[string][]collector.MetricPoint {
	index := make(map[string][]collector.MetricPoint, len(points))
	for _, p := range points {
		index[p.Metric] = append(index[p.Metric], p)
	}
	return index
}

// findMatchingPoints returns points that match the rule's metric and labels.
func findMatchingPoints(index map[string][]collector.MetricPoint, rule Rule) []collector.MetricPoint {
	candidates, ok := index[rule.Metric]
	if !ok {
		return nil
	}

	if len(rule.Labels) == 0 {
		return candidates
	}

	var matched []collector.MetricPoint
	for _, pt := range candidates {
		if labelsMatch(rule.Labels, pt.Labels) {
			matched = append(matched, pt)
		}
	}
	return matched
}

// labelsMatch checks that all required labels are present in actual with matching values.
func labelsMatch(required, actual map[string]string) bool {
	for k, v := range required {
		if actual[k] != v {
			return false
		}
	}
	return true
}

// StartCleanup launches a periodic goroutine that deletes old resolved alerts.
func (e *Evaluator) StartCleanup(ctx context.Context, retentionDays int) {
	if retentionDays <= 0 {
		retentionDays = 30
	}
	retention := time.Duration(retentionDays) * 24 * time.Hour

	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()

		// Run once immediately on startup.
		e.runCleanup(ctx, retention)

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				e.runCleanup(ctx, retention)
			}
		}
	}()

	e.logger.Info("alert history cleanup started", "retention_days", retentionDays, "interval", "1h")
}

func (e *Evaluator) runCleanup(ctx context.Context, retention time.Duration) {
	cleanupCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	deleted, err := e.historyStore.Cleanup(cleanupCtx, retention)
	if err != nil {
		e.logger.Error("alert history cleanup failed", "error", err)
		return
	}
	if deleted > 0 {
		e.logger.Info("alert history cleanup completed", "deleted", deleted)
	}
}
