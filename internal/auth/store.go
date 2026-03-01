package auth

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrUserNotFound is returned when a user lookup finds no matching row.
var ErrUserNotFound = errors.New("user not found")

// User represents a PGPulse user account.
type User struct {
	ID           int64
	Username     string
	PasswordHash string
	Role         string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// UserStore provides access to user data.
type UserStore interface {
	GetByUsername(ctx context.Context, username string) (*User, error)
	Create(ctx context.Context, username, passwordHash, role string) (*User, error)
	Count(ctx context.Context) (int64, error)
}

// PGUserStore implements UserStore backed by PostgreSQL.
type PGUserStore struct {
	pool *pgxpool.Pool
}

// NewPGUserStore creates a PGUserStore using the given connection pool.
func NewPGUserStore(pool *pgxpool.Pool) *PGUserStore {
	return &PGUserStore{pool: pool}
}

// GetByUsername looks up a user by username. Returns ErrUserNotFound if absent.
func (s *PGUserStore) GetByUsername(ctx context.Context, username string) (*User, error) {
	var u User
	err := s.pool.QueryRow(ctx,
		`SELECT id, username, password_hash, role, created_at, updated_at
		 FROM users WHERE username = $1`, username,
	).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// Create inserts a new user and returns the created record.
func (s *PGUserStore) Create(ctx context.Context, username, passwordHash, role string) (*User, error) {
	var u User
	err := s.pool.QueryRow(ctx,
		`INSERT INTO users (username, password_hash, role)
		 VALUES ($1, $2, $3)
		 RETURNING id, username, password_hash, role, created_at, updated_at`,
		username, passwordHash, role,
	).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// Count returns the total number of users in the table.
func (s *PGUserStore) Count(ctx context.Context) (int64, error) {
	var count int64
	err := s.pool.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&count)
	return count, err
}
