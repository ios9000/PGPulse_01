package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// InstanceRecord represents a monitored PostgreSQL instance stored in the database.
type InstanceRecord struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	DSN       string    `json:"dsn"`
	Host      string    `json:"host"`
	Port      int       `json:"port"`
	Enabled   bool      `json:"enabled"`
	Source    string    `json:"source"`
	MaxConns  int       `json:"max_conns"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// InstanceStore defines CRUD operations for monitored instance records.
type InstanceStore interface {
	List(ctx context.Context) ([]InstanceRecord, error)
	Get(ctx context.Context, id string) (*InstanceRecord, error)
	Create(ctx context.Context, rec InstanceRecord) (*InstanceRecord, error)
	Update(ctx context.Context, rec InstanceRecord) (*InstanceRecord, error)
	Delete(ctx context.Context, id string) error
	Seed(ctx context.Context, rec InstanceRecord) error
}

// PGInstanceStore implements InstanceStore using PostgreSQL.
type PGInstanceStore struct {
	pool *pgxpool.Pool
}

// NewPGInstanceStore creates an instance store backed by the given pool.
func NewPGInstanceStore(pool *pgxpool.Pool) *PGInstanceStore {
	return &PGInstanceStore{pool: pool}
}

// List returns all instance records ordered by ID.
func (s *PGInstanceStore) List(ctx context.Context) ([]InstanceRecord, error) {
	qCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	const sql = `SELECT id, name, dsn, host, port, enabled, source, max_conns, created_at, updated_at
		FROM instances ORDER BY id`

	rows, err := s.pool.Query(qCtx, sql)
	if err != nil {
		return nil, fmt.Errorf("list instances: %w", err)
	}
	defer rows.Close()

	var records []InstanceRecord
	for rows.Next() {
		var r InstanceRecord
		if err := rows.Scan(&r.ID, &r.Name, &r.DSN, &r.Host, &r.Port, &r.Enabled,
			&r.Source, &r.MaxConns, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan instance row: %w", err)
		}
		records = append(records, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list instances rows: %w", err)
	}

	return records, nil
}

// Get returns a single instance record by ID.
func (s *PGInstanceStore) Get(ctx context.Context, id string) (*InstanceRecord, error) {
	qCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	const sql = `SELECT id, name, dsn, host, port, enabled, source, max_conns, created_at, updated_at
		FROM instances WHERE id = $1`

	var r InstanceRecord
	err := s.pool.QueryRow(qCtx, sql, id).Scan(
		&r.ID, &r.Name, &r.DSN, &r.Host, &r.Port, &r.Enabled,
		&r.Source, &r.MaxConns, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get instance %q: %w", id, err)
	}

	return &r, nil
}

// Create inserts a new instance record and returns the created row.
func (s *PGInstanceStore) Create(ctx context.Context, rec InstanceRecord) (*InstanceRecord, error) {
	qCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if rec.MaxConns == 0 {
		rec.MaxConns = 5
	}
	if rec.Source == "" {
		rec.Source = "manual"
	}

	const sql = `INSERT INTO instances (id, name, dsn, host, port, enabled, source, max_conns)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, name, dsn, host, port, enabled, source, max_conns, created_at, updated_at`

	var r InstanceRecord
	err := s.pool.QueryRow(qCtx, sql,
		rec.ID, rec.Name, rec.DSN, rec.Host, rec.Port, rec.Enabled, rec.Source, rec.MaxConns,
	).Scan(&r.ID, &r.Name, &r.DSN, &r.Host, &r.Port, &r.Enabled,
		&r.Source, &r.MaxConns, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("create instance: %w", err)
	}

	return &r, nil
}

// Update modifies an existing instance record and returns the updated row.
func (s *PGInstanceStore) Update(ctx context.Context, rec InstanceRecord) (*InstanceRecord, error) {
	qCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	const sql = `UPDATE instances
		SET name = $2, dsn = $3, host = $4, port = $5, enabled = $6, max_conns = $7, updated_at = now()
		WHERE id = $1
		RETURNING id, name, dsn, host, port, enabled, source, max_conns, created_at, updated_at`

	var r InstanceRecord
	err := s.pool.QueryRow(qCtx, sql,
		rec.ID, rec.Name, rec.DSN, rec.Host, rec.Port, rec.Enabled, rec.MaxConns,
	).Scan(&r.ID, &r.Name, &r.DSN, &r.Host, &r.Port, &r.Enabled,
		&r.Source, &r.MaxConns, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("instance %q not found", rec.ID)
		}
		return nil, fmt.Errorf("update instance %q: %w", rec.ID, err)
	}

	return &r, nil
}

// Delete removes an instance record by ID.
func (s *PGInstanceStore) Delete(ctx context.Context, id string) error {
	qCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	tag, err := s.pool.Exec(qCtx, `DELETE FROM instances WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete instance %q: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("instance %q not found", id)
	}

	return nil
}

// Seed inserts an instance record with source='yaml' if it does not already exist.
// Existing records are left untouched (ON CONFLICT DO NOTHING).
func (s *PGInstanceStore) Seed(ctx context.Context, rec InstanceRecord) error {
	qCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if rec.MaxConns == 0 {
		rec.MaxConns = 5
	}

	const sql = `INSERT INTO instances (id, name, dsn, host, port, enabled, source, max_conns)
		VALUES ($1, $2, $3, $4, $5, $6, 'yaml', $7)
		ON CONFLICT (id) DO NOTHING`

	_, err := s.pool.Exec(qCtx, sql,
		rec.ID, rec.Name, rec.DSN, rec.Host, rec.Port, rec.Enabled, rec.MaxConns)
	if err != nil {
		return fmt.Errorf("seed instance %q: %w", rec.ID, err)
	}

	return nil
}
