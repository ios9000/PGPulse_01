//go:build integration

package auth_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/ios9000/PGPulse_01/internal/auth"
	"github.com/ios9000/PGPulse_01/internal/storage"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

func setupIntegrationDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()

	pgc, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("pgpulse_test"),
		postgres.WithUsername("pgpulse"),
		postgres.WithPassword("secret"),
	)
	if err != nil {
		t.Fatalf("start postgres container: %v", err)
	}
	t.Cleanup(func() { _ = pgc.Terminate(ctx) })

	dsn, err := pgc.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("get connection string: %v", err)
	}

	pool, err := storage.NewPool(ctx, dsn)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	t.Cleanup(pool.Close)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := storage.Migrate(ctx, pool, logger, storage.MigrateOptions{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	return pool
}

func TestPGUserStore_CreateAndGet(t *testing.T) {
	pool := setupIntegrationDB(t)
	store := auth.NewPGUserStore(pool)
	ctx := context.Background()

	created, err := store.Create(ctx, "testuser", "$2a$12$hash", string(auth.RoleDBA))
	if err != nil {
		t.Fatalf("Create error: %v", err)
	}
	if created.ID == 0 {
		t.Error("expected non-zero ID")
	}

	got, err := store.GetByUsername(ctx, "testuser")
	if err != nil {
		t.Fatalf("GetByUsername error: %v", err)
	}
	if got.Username != "testuser" {
		t.Errorf("Username = %q, want %q", got.Username, "testuser")
	}
}

func TestPGUserStore_GetByUsername_NotFound(t *testing.T) {
	pool := setupIntegrationDB(t)
	store := auth.NewPGUserStore(pool)

	_, err := store.GetByUsername(context.Background(), "nobody")
	if !errors.Is(err, auth.ErrUserNotFound) {
		t.Errorf("error = %v, want ErrUserNotFound", err)
	}
}

func TestPGUserStore_Count(t *testing.T) {
	pool := setupIntegrationDB(t)
	store := auth.NewPGUserStore(pool)
	ctx := context.Background()

	count, err := store.Count(ctx)
	if err != nil {
		t.Fatalf("Count error: %v", err)
	}
	if count != 0 {
		t.Errorf("Count = %d, want 0 on fresh DB", count)
	}

	_, _ = store.Create(ctx, "user1", "hash", string(auth.RoleSuperAdmin))
	count, _ = store.Count(ctx)
	if count != 1 {
		t.Errorf("Count = %d, want 1 after insert", count)
	}
}
