package storage

import (
	"context"
	"embed"
	"fmt"
	"log/slog"
	"path"
	"sort"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// MigrateOptions controls which migrations are applied.
type MigrateOptions struct {
	UseTimescaleDB bool
}

// Migrate creates the schema_migrations table if needed, then applies every
// pending migration in alphabetical order. Conditional migrations (e.g.
// TimescaleDB hypertable) are skipped based on opts.
func Migrate(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger, opts MigrateOptions) error {
	// Ensure the migrations tracking table exists.
	_, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    TEXT        PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`)
	if err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	// List embedded migration files and sort them.
	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	// Apply each pending migration.
	for _, name := range names {
		// Skip if already applied.
		var exists bool
		err := pool.QueryRow(ctx,
			"SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = $1)", name,
		).Scan(&exists)
		if err != nil {
			return fmt.Errorf("check migration %s: %w", name, err)
		}
		if exists {
			continue
		}

		// Skip conditional migrations that are not enabled.
		if isConditional(name, opts) {
			logger.Info("skipping conditional migration", "version", name)
			continue
		}

		// Read migration SQL from the embedded FS.
		content, err := migrationFS.ReadFile(path.Join("migrations", name))
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}

		// Apply in a transaction so schema change and version record are atomic.
		tx, err := pool.Begin(ctx)
		if err != nil {
			return fmt.Errorf("migration %s: begin: %w", name, err)
		}

		if _, err := tx.Exec(ctx, string(content)); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("migration %s: %w", name, err)
		}

		if _, err := tx.Exec(ctx,
			"INSERT INTO schema_migrations (version) VALUES ($1)", name,
		); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("migration %s: record: %w", name, err)
		}

		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("migration %s: commit: %w", name, err)
		}

		logger.Info("applied migration", "version", name)
	}

	return nil
}

// isConditional returns true when a migration should be skipped given opts.
// Currently only the TimescaleDB migration is conditional.
func isConditional(filename string, opts MigrateOptions) bool {
	return filename == "002_timescaledb.sql" && !opts.UseTimescaleDB
}
