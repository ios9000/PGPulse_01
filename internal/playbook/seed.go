package playbook

import (
	"context"
	"fmt"
	"log/slog"
)

// SeedBuiltinPlaybooks inserts all built-in playbooks into the database.
// Safe to call on every startup — uses INSERT ON CONFLICT DO NOTHING.
func SeedBuiltinPlaybooks(ctx context.Context, store PlaybookStore, logger *slog.Logger) error {
	playbooks := BuiltinPlaybooks()
	if err := store.SeedBuiltins(ctx, playbooks); err != nil {
		return fmt.Errorf("seed playbooks: %w", err)
	}
	logger.Info("builtin playbooks seeded", "total", len(playbooks))
	return nil
}

// BuiltinPlaybooks returns the Core 10 seed playbooks.
func BuiltinPlaybooks() []Playbook {
	return []Playbook{
		walArchiveFailurePlaybook(),
		replicationLagPlaybook(),
		connectionSaturationPlaybook(),
		lockContentionPlaybook(),
		longTransactionsPlaybook(),
		checkpointStormPlaybook(),
		diskFullPlaybook(),
		autovacuumFailingPlaybook(),
		wraparoundRiskPlaybook(),
		heavyQueryPlaybook(),
	}
}
