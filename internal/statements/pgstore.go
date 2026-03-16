package statements

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PGSnapshotStore implements SnapshotStore using a PostgreSQL connection pool.
type PGSnapshotStore struct {
	pool *pgxpool.Pool
}

// NewPGSnapshotStore creates a PGSnapshotStore backed by the given pool.
func NewPGSnapshotStore(pool *pgxpool.Pool) *PGSnapshotStore {
	return &PGSnapshotStore{pool: pool}
}

// WriteSnapshot inserts a snapshot and its entries in a single transaction.
// For batches larger than 100 entries, pgx.CopyFrom is used for performance.
func (s *PGSnapshotStore) WriteSnapshot(ctx context.Context, snap Snapshot, entries []SnapshotEntry) (int64, error) {
	// Compute summary fields from entries.
	snap.TotalStatements = len(entries)
	var totalCalls int64
	var totalExecTime float64
	for i := range entries {
		totalCalls += entries[i].Calls
		totalExecTime += entries[i].TotalExecTime
	}
	snap.TotalCalls = totalCalls
	snap.TotalExecTime = totalExecTime

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var snapID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO pgss_snapshots (instance_id, captured_at, pg_version, stats_reset, total_statements, total_calls, total_exec_time)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id`,
		snap.InstanceID, snap.CapturedAt, snap.PGVersion, snap.StatsReset,
		snap.TotalStatements, snap.TotalCalls, snap.TotalExecTime,
	).Scan(&snapID)
	if err != nil {
		return 0, fmt.Errorf("insert snapshot: %w", err)
	}

	if len(entries) == 0 {
		if err := tx.Commit(ctx); err != nil {
			return 0, fmt.Errorf("commit: %w", err)
		}
		return snapID, nil
	}

	if len(entries) > 100 {
		err = s.copyEntries(ctx, tx, snapID, entries)
	} else {
		err = s.batchInsertEntries(ctx, tx, snapID, entries)
	}
	if err != nil {
		return 0, err
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}
	return snapID, nil
}

const entryCols = `snapshot_id, queryid, userid, dbid, database_name, user_name, query,
	calls, total_exec_time, total_plan_time, rows,
	shared_blks_hit, shared_blks_read, shared_blks_dirtied, shared_blks_written,
	local_blks_hit, local_blks_read, temp_blks_read, temp_blks_written,
	blk_read_time, blk_write_time,
	wal_records, wal_fpi, wal_bytes,
	mean_exec_time, min_exec_time, max_exec_time, stddev_exec_time`

func entryValues(snapID int64, e *SnapshotEntry) []any {
	return []any{
		snapID, e.QueryID, e.UserID, e.DbID, e.DatabaseName, e.UserName, e.Query,
		e.Calls, e.TotalExecTime, e.TotalPlanTime, e.Rows,
		e.SharedBlksHit, e.SharedBlksRead, e.SharedBlksDirtied, e.SharedBlksWritten,
		e.LocalBlksHit, e.LocalBlksRead, e.TempBlksRead, e.TempBlksWritten,
		e.BlkReadTime, e.BlkWriteTime,
		e.WALRecords, e.WALFpi, e.WALBytes,
		e.MeanExecTime, e.MinExecTime, e.MaxExecTime, e.StddevExecTime,
	}
}

func (s *PGSnapshotStore) copyEntries(ctx context.Context, tx pgx.Tx, snapID int64, entries []SnapshotEntry) error {
	cols := []string{
		"snapshot_id", "queryid", "userid", "dbid", "database_name", "user_name", "query",
		"calls", "total_exec_time", "total_plan_time", "rows",
		"shared_blks_hit", "shared_blks_read", "shared_blks_dirtied", "shared_blks_written",
		"local_blks_hit", "local_blks_read", "temp_blks_read", "temp_blks_written",
		"blk_read_time", "blk_write_time",
		"wal_records", "wal_fpi", "wal_bytes",
		"mean_exec_time", "min_exec_time", "max_exec_time", "stddev_exec_time",
	}

	src := pgx.CopyFromSlice(len(entries), func(i int) ([]any, error) {
		return entryValues(snapID, &entries[i]), nil
	})

	_, err := tx.CopyFrom(ctx, pgx.Identifier{"pgss_snapshot_entries"}, cols, src)
	if err != nil {
		return fmt.Errorf("copy entries: %w", err)
	}
	return nil
}

func (s *PGSnapshotStore) batchInsertEntries(ctx context.Context, tx pgx.Tx, snapID int64, entries []SnapshotEntry) error {
	sql := fmt.Sprintf(`INSERT INTO pgss_snapshot_entries (%s) VALUES (
		$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28)`, entryCols)

	batch := &pgx.Batch{}
	for i := range entries {
		batch.Queue(sql, entryValues(snapID, &entries[i])...)
	}

	br := tx.SendBatch(ctx, batch)
	defer func() { _ = br.Close() }()

	for range entries {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("batch insert entry: %w", err)
		}
	}
	return nil
}

// GetSnapshot retrieves a single snapshot by ID.
func (s *PGSnapshotStore) GetSnapshot(ctx context.Context, id int64) (*Snapshot, error) {
	var snap Snapshot
	err := s.pool.QueryRow(ctx, `
		SELECT id, instance_id, captured_at, pg_version, stats_reset,
		       total_statements, total_calls, total_exec_time
		FROM pgss_snapshots WHERE id = $1`, id,
	).Scan(&snap.ID, &snap.InstanceID, &snap.CapturedAt, &snap.PGVersion,
		&snap.StatsReset, &snap.TotalStatements, &snap.TotalCalls, &snap.TotalExecTime)
	if err != nil {
		return nil, fmt.Errorf("get snapshot %d: %w", id, err)
	}
	return &snap, nil
}

// GetSnapshotEntries retrieves entries for a snapshot with pagination.
// Returns entries and total count.
func (s *PGSnapshotStore) GetSnapshotEntries(ctx context.Context, snapshotID int64, limit, offset int) ([]SnapshotEntry, int, error) {
	if limit <= 0 {
		limit = 50
	}

	rows, err := s.pool.Query(ctx, fmt.Sprintf(`
		SELECT %s, COUNT(*) OVER() AS total_count
		FROM pgss_snapshot_entries
		WHERE snapshot_id = $1
		ORDER BY total_exec_time DESC
		LIMIT $2 OFFSET $3`, entryCols), snapshotID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("get entries for snapshot %d: %w", snapshotID, err)
	}
	defer rows.Close()

	var entries []SnapshotEntry
	var totalCount int
	for rows.Next() {
		var e SnapshotEntry
		if err := rows.Scan(
			&e.SnapshotID, &e.QueryID, &e.UserID, &e.DbID, &e.DatabaseName, &e.UserName, &e.Query,
			&e.Calls, &e.TotalExecTime, &e.TotalPlanTime, &e.Rows,
			&e.SharedBlksHit, &e.SharedBlksRead, &e.SharedBlksDirtied, &e.SharedBlksWritten,
			&e.LocalBlksHit, &e.LocalBlksRead, &e.TempBlksRead, &e.TempBlksWritten,
			&e.BlkReadTime, &e.BlkWriteTime,
			&e.WALRecords, &e.WALFpi, &e.WALBytes,
			&e.MeanExecTime, &e.MinExecTime, &e.MaxExecTime, &e.StddevExecTime,
			&totalCount,
		); err != nil {
			return nil, 0, fmt.Errorf("scan entry: %w", err)
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("entries rows: %w", err)
	}
	return entries, totalCount, nil
}

// ListSnapshots returns snapshots for an instance with optional time range and pagination.
func (s *PGSnapshotStore) ListSnapshots(ctx context.Context, instanceID string, opts SnapshotListOptions) ([]Snapshot, int, error) {
	if opts.Limit <= 0 {
		opts.Limit = 50
	}

	// Build query with optional time filters.
	query := `SELECT id, instance_id, captured_at, pg_version, stats_reset,
	                 total_statements, total_calls, total_exec_time,
	                 COUNT(*) OVER() AS total_count
	          FROM pgss_snapshots
	          WHERE instance_id = $1`
	args := []any{instanceID}
	argIdx := 2

	if opts.From != nil {
		query += fmt.Sprintf(" AND captured_at >= $%d", argIdx)
		args = append(args, *opts.From)
		argIdx++
	}
	if opts.To != nil {
		query += fmt.Sprintf(" AND captured_at <= $%d", argIdx)
		args = append(args, *opts.To)
		argIdx++
	}

	query += fmt.Sprintf(" ORDER BY captured_at DESC LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
	args = append(args, opts.Limit, opts.Offset)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list snapshots: %w", err)
	}
	defer rows.Close()

	var snapshots []Snapshot
	var totalCount int
	for rows.Next() {
		var snap Snapshot
		if err := rows.Scan(&snap.ID, &snap.InstanceID, &snap.CapturedAt, &snap.PGVersion,
			&snap.StatsReset, &snap.TotalStatements, &snap.TotalCalls, &snap.TotalExecTime,
			&totalCount,
		); err != nil {
			return nil, 0, fmt.Errorf("scan snapshot: %w", err)
		}
		snapshots = append(snapshots, snap)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("snapshots rows: %w", err)
	}
	return snapshots, totalCount, nil
}

// GetLatestSnapshots returns the N most recent snapshots for an instance.
func (s *PGSnapshotStore) GetLatestSnapshots(ctx context.Context, instanceID string, n int) ([]Snapshot, error) {
	if n <= 0 {
		n = 2
	}

	rows, err := s.pool.Query(ctx, `
		SELECT id, instance_id, captured_at, pg_version, stats_reset,
		       total_statements, total_calls, total_exec_time
		FROM pgss_snapshots
		WHERE instance_id = $1
		ORDER BY captured_at DESC
		LIMIT $2`, instanceID, n)
	if err != nil {
		return nil, fmt.Errorf("get latest snapshots: %w", err)
	}
	defer rows.Close()

	var snapshots []Snapshot
	for rows.Next() {
		var snap Snapshot
		if err := rows.Scan(&snap.ID, &snap.InstanceID, &snap.CapturedAt, &snap.PGVersion,
			&snap.StatsReset, &snap.TotalStatements, &snap.TotalCalls, &snap.TotalExecTime,
		); err != nil {
			return nil, fmt.Errorf("scan snapshot: %w", err)
		}
		snapshots = append(snapshots, snap)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("latest snapshots rows: %w", err)
	}
	return snapshots, nil
}

// GetEntriesForQuery returns all snapshot entries for a specific queryid within a time range,
// along with their parent snapshots. Results are ordered by captured_at ASC.
func (s *PGSnapshotStore) GetEntriesForQuery(ctx context.Context, instanceID string, queryID int64, from, to time.Time) ([]SnapshotEntry, []Snapshot, error) {
	rows, err := s.pool.Query(ctx, fmt.Sprintf(`
		SELECT e.%s,
		       snap.id, snap.instance_id, snap.captured_at, snap.pg_version, snap.stats_reset,
		       snap.total_statements, snap.total_calls, snap.total_exec_time
		FROM pgss_snapshot_entries e
		JOIN pgss_snapshots snap ON snap.id = e.snapshot_id
		WHERE e.queryid = $1
		  AND snap.instance_id = $2
		  AND snap.captured_at BETWEEN $3 AND $4
		ORDER BY snap.captured_at ASC`, entryCols),
		queryID, instanceID, from, to)
	if err != nil {
		return nil, nil, fmt.Errorf("get entries for query %d: %w", queryID, err)
	}
	defer rows.Close()

	var entries []SnapshotEntry
	var snapshots []Snapshot
	for rows.Next() {
		var e SnapshotEntry
		var snap Snapshot
		if err := rows.Scan(
			&e.SnapshotID, &e.QueryID, &e.UserID, &e.DbID, &e.DatabaseName, &e.UserName, &e.Query,
			&e.Calls, &e.TotalExecTime, &e.TotalPlanTime, &e.Rows,
			&e.SharedBlksHit, &e.SharedBlksRead, &e.SharedBlksDirtied, &e.SharedBlksWritten,
			&e.LocalBlksHit, &e.LocalBlksRead, &e.TempBlksRead, &e.TempBlksWritten,
			&e.BlkReadTime, &e.BlkWriteTime,
			&e.WALRecords, &e.WALFpi, &e.WALBytes,
			&e.MeanExecTime, &e.MinExecTime, &e.MaxExecTime, &e.StddevExecTime,
			&snap.ID, &snap.InstanceID, &snap.CapturedAt, &snap.PGVersion, &snap.StatsReset,
			&snap.TotalStatements, &snap.TotalCalls, &snap.TotalExecTime,
		); err != nil {
			return nil, nil, fmt.Errorf("scan query entry: %w", err)
		}
		entries = append(entries, e)
		snapshots = append(snapshots, snap)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("query entries rows: %w", err)
	}
	return entries, snapshots, nil
}

// CleanOld deletes snapshots older than the given time.
// CASCADE on pgss_snapshot_entries handles entry cleanup.
func (s *PGSnapshotStore) CleanOld(ctx context.Context, olderThan time.Time) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM pgss_snapshots WHERE captured_at < $1`, olderThan)
	if err != nil {
		return fmt.Errorf("clean old snapshots: %w", err)
	}
	return nil
}
