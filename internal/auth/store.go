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
	Active       bool
	LastLogin    *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// UpdateFields holds the mutable fields for user update.
type UpdateFields struct {
	Role   *string
	Active *bool
}

// UserStore provides access to user data.
type UserStore interface {
	GetByUsername(ctx context.Context, username string) (*User, error)
	GetByID(ctx context.Context, id int64) (*User, error)
	Create(ctx context.Context, username, passwordHash, role string) (*User, error)
	Count(ctx context.Context) (int64, error)
	CountActiveByRole(ctx context.Context, role string) (int64, error)
	List(ctx context.Context) ([]*User, error)
	Update(ctx context.Context, id int64, fields UpdateFields) error
	UpdatePassword(ctx context.Context, id int64, passwordHash string) error
	UpdateLastLogin(ctx context.Context, id int64) error
	Delete(ctx context.Context, id int64) error
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
		`SELECT id, username, password_hash, role, active, last_login, created_at, updated_at
		 FROM users WHERE username = $1`, username,
	).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.Active, &u.LastLogin, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// GetByID looks up a user by ID. Returns ErrUserNotFound if absent.
func (s *PGUserStore) GetByID(ctx context.Context, id int64) (*User, error) {
	var u User
	err := s.pool.QueryRow(ctx,
		`SELECT id, username, password_hash, role, active, last_login, created_at, updated_at
		 FROM users WHERE id = $1`, id,
	).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.Active, &u.LastLogin, &u.CreatedAt, &u.UpdatedAt)
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
		 RETURNING id, username, password_hash, role, active, last_login, created_at, updated_at`,
		username, passwordHash, role,
	).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.Active, &u.LastLogin, &u.CreatedAt, &u.UpdatedAt)
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

// List returns all users ordered by ID.
func (s *PGUserStore) List(ctx context.Context) ([]*User, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, username, password_hash, role, active, last_login, created_at, updated_at
		 FROM users ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []*User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.Active, &u.LastLogin, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, err
		}
		users = append(users, &u)
	}
	return users, rows.Err()
}

// Update modifies mutable fields of a user.
func (s *PGUserStore) Update(ctx context.Context, id int64, fields UpdateFields) error {
	if fields.Role != nil {
		if _, err := s.pool.Exec(ctx,
			`UPDATE users SET role = $1, updated_at = now() WHERE id = $2`,
			*fields.Role, id); err != nil {
			return err
		}
	}
	if fields.Active != nil {
		if _, err := s.pool.Exec(ctx,
			`UPDATE users SET active = $1, updated_at = now() WHERE id = $2`,
			*fields.Active, id); err != nil {
			return err
		}
	}
	return nil
}

// UpdatePassword changes a user's password hash.
func (s *PGUserStore) UpdatePassword(ctx context.Context, id int64, passwordHash string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE users SET password_hash = $1, updated_at = now() WHERE id = $2`,
		passwordHash, id)
	return err
}

// UpdateLastLogin records the current time as the user's last login.
func (s *PGUserStore) UpdateLastLogin(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE users SET last_login = now() WHERE id = $1`, id)
	return err
}

// CountActiveByRole returns the number of active users with the given role.
func (s *PGUserStore) CountActiveByRole(ctx context.Context, role string) (int64, error) {
	var count int64
	err := s.pool.QueryRow(ctx,
		`SELECT count(*) FROM users WHERE role = $1 AND active = true`, role).Scan(&count)
	return count, err
}

// Delete removes a user by ID. Returns ErrUserNotFound if the user does not exist.
func (s *PGUserStore) Delete(ctx context.Context, id int64) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrUserNotFound
	}
	return nil
}
