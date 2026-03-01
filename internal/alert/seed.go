package alert

import (
	"context"
	"fmt"
	"log/slog"
)

// SeedBuiltinRules upserts all builtin rules to the database.
// Safe to call on every startup — preserves user modifications to thresholds.
func SeedBuiltinRules(ctx context.Context, store AlertRuleStore, logger *slog.Logger) error {
	rules := BuiltinRules()
	for _, rule := range rules {
		if err := store.UpsertBuiltin(ctx, &rule); err != nil {
			return fmt.Errorf("seed rule %s: %w", rule.ID, err)
		}
	}
	logger.Info("builtin alert rules seeded", "total", len(rules))
	return nil
}
