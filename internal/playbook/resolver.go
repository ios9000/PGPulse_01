package playbook

import (
	"context"
	"log/slog"
)

// Resolver selects the best-matching playbook for a given context.
type Resolver struct {
	store  PlaybookStore
	logger *slog.Logger
}

// NewResolver creates a Resolver.
func NewResolver(store PlaybookStore, logger *slog.Logger) *Resolver {
	return &Resolver{store: store, logger: logger}
}

// ResolverContext provides the input for playbook resolution.
type ResolverContext struct {
	HookID       string
	RootCauseKey string
	MetricKey    string
	AdviserRule  string
	InstanceID   string
}

// Resolve returns the best-matching playbook for the given context.
// Returns (playbook, match_reason, error). playbook is nil if no match.
func (r *Resolver) Resolve(ctx context.Context, rc ResolverContext) (*Playbook, string, error) {
	// Priority 1: Explicit hook match.
	if rc.HookID != "" {
		pb, err := r.store.FindByTriggerBinding(ctx, "hooks", rc.HookID)
		if err != nil {
			r.logger.Warn("resolver hook lookup failed", "hook", rc.HookID, "error", err)
		}
		if pb != nil {
			return pb, "rca_hook", nil
		}
	}

	// Priority 2: Root cause key match.
	if rc.RootCauseKey != "" {
		pb, err := r.store.FindByTriggerBinding(ctx, "root_causes", rc.RootCauseKey)
		if err != nil {
			r.logger.Warn("resolver root_cause lookup failed", "key", rc.RootCauseKey, "error", err)
		}
		if pb != nil {
			return pb, "root_cause", nil
		}
	}

	// Priority 3: Metric key match.
	if rc.MetricKey != "" {
		pb, err := r.store.FindByTriggerBinding(ctx, "metrics", rc.MetricKey)
		if err != nil {
			r.logger.Warn("resolver metric lookup failed", "key", rc.MetricKey, "error", err)
		}
		if pb != nil {
			return pb, "metric", nil
		}
	}

	// Priority 4: Adviser rule match.
	if rc.AdviserRule != "" {
		pb, err := r.store.FindByTriggerBinding(ctx, "adviser_rules", rc.AdviserRule)
		if err != nil {
			r.logger.Warn("resolver adviser_rule lookup failed", "key", rc.AdviserRule, "error", err)
		}
		if pb != nil {
			return pb, "adviser_rule", nil
		}
	}

	// Priority 5: No match.
	return nil, "", nil
}
