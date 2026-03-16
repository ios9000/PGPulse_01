package statements

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ios9000/PGPulse_01/internal/version"
)

// InstanceLister returns the list of enabled instance IDs.
type InstanceLister interface {
	ListInstanceIDs(ctx context.Context) ([]string, error)
}

// ConnProvider returns a connection pool for a given instance.
type ConnProvider interface {
	PoolForInstance(instanceID string) (*pgxpool.Pool, error)
}

// pgssGate holds version-gated SQL for reading pg_stat_statements.
var pgssGate = version.Gate{
	Name: "pgss_snapshot",
	Variants: []version.SQLVariant{
		{
			Range: version.VersionRange{MinMajor: 13, MinMinor: 0},
			SQL: `SELECT s.queryid, s.userid, s.dbid, left(s.query, 8192) AS query,
       s.calls, s.total_exec_time, s.total_plan_time, s.rows,
       s.shared_blks_hit, s.shared_blks_read, s.shared_blks_dirtied, s.shared_blks_written,
       s.local_blks_hit, s.local_blks_read, s.temp_blks_read, s.temp_blks_written,
       s.blk_read_time, s.blk_write_time,
       s.wal_records, s.wal_fpi, s.wal_bytes,
       s.mean_exec_time, s.min_exec_time, s.max_exec_time, s.stddev_exec_time
FROM pg_stat_statements s WHERE s.queryid IS NOT NULL`,
		},
		{
			Range: version.VersionRange{MinMajor: 9, MinMinor: 0, MaxMajor: 12, MaxMinor: 99},
			SQL: `SELECT s.queryid, s.userid, s.dbid, left(s.query, 8192) AS query,
       s.calls, s.total_time AS total_exec_time, s.rows,
       s.shared_blks_hit, s.shared_blks_read, s.shared_blks_dirtied, s.shared_blks_written,
       s.local_blks_hit, s.local_blks_read, s.temp_blks_read, s.temp_blks_written,
       s.blk_read_time, s.blk_write_time,
       NULL::double precision AS total_plan_time,
       NULL::bigint AS wal_records, NULL::bigint AS wal_fpi, NULL::numeric AS wal_bytes,
       NULL::double precision AS mean_exec_time, NULL::double precision AS min_exec_time,
       NULL::double precision AS max_exec_time, NULL::double precision AS stddev_exec_time
FROM pg_stat_statements s WHERE s.queryid IS NOT NULL`,
		},
	},
}

// SnapshotCapturer periodically captures pg_stat_statements snapshots
// for all monitored instances.
type SnapshotCapturer struct {
	store     SnapshotStore
	connProv  ConnProvider
	lister    InstanceLister
	interval  time.Duration
	retention time.Duration
	onStartup bool
	logger    *slog.Logger
	cancel    context.CancelFunc
}

// NewSnapshotCapturer creates a SnapshotCapturer.
func NewSnapshotCapturer(
	store SnapshotStore,
	connProv ConnProvider,
	lister InstanceLister,
	interval time.Duration,
	retentionDays int,
	onStartup bool,
	logger *slog.Logger,
) *SnapshotCapturer {
	return &SnapshotCapturer{
		store:     store,
		connProv:  connProv,
		lister:    lister,
		interval:  interval,
		retention: time.Duration(retentionDays) * 24 * time.Hour,
		onStartup: onStartup,
		logger:    logger,
	}
}

// Start launches the snapshot capture loop in a goroutine.
func (c *SnapshotCapturer) Start(ctx context.Context) {
	ctx, c.cancel = context.WithCancel(ctx)
	go c.run(ctx)
}

// Stop cancels the snapshot capture loop.
func (c *SnapshotCapturer) Stop() {
	if c.cancel != nil {
		c.cancel()
	}
}

func (c *SnapshotCapturer) run(ctx context.Context) {
	if c.onStartup {
		c.runCycle(ctx)
	}
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.runCycle(ctx)
		}
	}
}

func (c *SnapshotCapturer) runCycle(ctx context.Context) {
	start := time.Now()

	instanceIDs, err := c.lister.ListInstanceIDs(ctx)
	if err != nil {
		c.logger.Error("pgss capture: failed to list instances", "error", err)
		return
	}

	captured := 0
	for _, instanceID := range instanceIDs {
		if ctx.Err() != nil {
			return
		}
		if _, err := c.CaptureInstance(ctx, instanceID); err != nil {
			c.logger.Warn("pgss capture: instance capture failed",
				"instance", instanceID, "error", err)
			continue
		}
		captured++
	}

	// Retention cleanup at end of cycle.
	cutoff := time.Now().Add(-c.retention)
	if err := c.store.CleanOld(ctx, cutoff); err != nil {
		c.logger.Warn("pgss capture: retention cleanup failed", "error", err)
	}

	c.logger.Info("pgss capture cycle complete",
		"instances", len(instanceIDs),
		"captured", captured,
		"duration", time.Since(start).Round(time.Millisecond),
	)
}

// CaptureInstance captures a single pg_stat_statements snapshot for an instance.
func (c *SnapshotCapturer) CaptureInstance(ctx context.Context, instanceID string) (*Snapshot, error) {
	pool, err := c.connProv.PoolForInstance(instanceID)
	if err != nil {
		return nil, fmt.Errorf("get pool for %s: %w", instanceID, err)
	}

	// Acquire a connection from the pool.
	pconn, err := pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("acquire conn for %s: %w", instanceID, err)
	}
	defer pconn.Release()

	rawConn := pconn.Conn()

	// Detect PG version.
	pgVer, err := version.Detect(ctx, rawConn)
	if err != nil {
		return nil, fmt.Errorf("detect version for %s: %w", instanceID, err)
	}

	// Check if pg_stat_statements is available.
	var extOK int
	err = rawConn.QueryRow(ctx, `SELECT 1 FROM pg_extension WHERE extname = 'pg_stat_statements'`).Scan(&extOK)
	if err != nil {
		return nil, fmt.Errorf("pg_stat_statements not available on %s: %w", instanceID, err)
	}

	// Read stats_reset (PG 14+).
	var statsReset *time.Time
	if pgVer.AtLeast(14, 0) {
		var sr time.Time
		if err := rawConn.QueryRow(ctx, `SELECT stats_reset FROM pg_stat_statements_info`).Scan(&sr); err == nil {
			statsReset = &sr
		}
	}

	// Set statement_timeout for the capture query.
	if _, err := rawConn.Exec(ctx, `SET statement_timeout = 30000`); err != nil {
		return nil, fmt.Errorf("set statement_timeout for %s: %w", instanceID, err)
	}

	// Select version-gated SQL.
	sql, ok := pgssGate.Select(pgVer)
	if !ok {
		return nil, fmt.Errorf("no PGSS SQL variant for PG %s", pgVer)
	}

	// Execute the snapshot query.
	entries, err := scanPGSSEntries(ctx, rawConn, sql, pgVer)
	if err != nil {
		return nil, fmt.Errorf("pgss scan for %s: %w", instanceID, err)
	}

	// Resolve database names: oid -> datname.
	dbNames, err := resolveDBNames(ctx, rawConn)
	if err != nil {
		c.logger.Warn("pgss capture: failed to resolve db names", "instance", instanceID, "error", err)
	}

	// Resolve user names: oid -> rolname.
	userNames, err := resolveUserNames(ctx, rawConn)
	if err != nil {
		c.logger.Warn("pgss capture: failed to resolve user names", "instance", instanceID, "error", err)
	}

	// Populate DatabaseName/UserName on each entry.
	for i := range entries {
		if name, ok := dbNames[entries[i].DbID]; ok {
			entries[i].DatabaseName = name
		}
		if name, ok := userNames[entries[i].UserID]; ok {
			entries[i].UserName = name
		}
	}

	// Build snapshot and persist.
	now := time.Now()
	snap := Snapshot{
		InstanceID: instanceID,
		CapturedAt: now,
		PGVersion:  pgVer.Num,
		StatsReset: statsReset,
	}

	snapID, err := c.store.WriteSnapshot(ctx, snap, entries)
	if err != nil {
		return nil, fmt.Errorf("write snapshot for %s: %w", instanceID, err)
	}

	snap.ID = snapID
	snap.TotalStatements = len(entries)
	var totalCalls int64
	var totalExecTime float64
	for i := range entries {
		totalCalls += entries[i].Calls
		totalExecTime += entries[i].TotalExecTime
	}
	snap.TotalCalls = totalCalls
	snap.TotalExecTime = totalExecTime

	return &snap, nil
}

func scanPGSSEntries(ctx context.Context, conn *pgx.Conn, sql string, pgVer version.PGVersion) ([]SnapshotEntry, error) {
	rows, err := conn.Query(ctx, sql)
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}
	defer rows.Close()

	isPG13Plus := pgVer.AtLeast(13, 0)
	var entries []SnapshotEntry
	for rows.Next() {
		var e SnapshotEntry
		if isPG13Plus {
			if err := rows.Scan(
				&e.QueryID, &e.UserID, &e.DbID, &e.Query,
				&e.Calls, &e.TotalExecTime, &e.TotalPlanTime, &e.Rows,
				&e.SharedBlksHit, &e.SharedBlksRead, &e.SharedBlksDirtied, &e.SharedBlksWritten,
				&e.LocalBlksHit, &e.LocalBlksRead, &e.TempBlksRead, &e.TempBlksWritten,
				&e.BlkReadTime, &e.BlkWriteTime,
				&e.WALRecords, &e.WALFpi, &e.WALBytes,
				&e.MeanExecTime, &e.MinExecTime, &e.MaxExecTime, &e.StddevExecTime,
			); err != nil {
				return nil, fmt.Errorf("scan (pg13+): %w", err)
			}
		} else {
			// PG <= 12: column order differs (total_plan_time comes after blk_write_time).
			if err := rows.Scan(
				&e.QueryID, &e.UserID, &e.DbID, &e.Query,
				&e.Calls, &e.TotalExecTime, &e.Rows,
				&e.SharedBlksHit, &e.SharedBlksRead, &e.SharedBlksDirtied, &e.SharedBlksWritten,
				&e.LocalBlksHit, &e.LocalBlksRead, &e.TempBlksRead, &e.TempBlksWritten,
				&e.BlkReadTime, &e.BlkWriteTime,
				&e.TotalPlanTime,
				&e.WALRecords, &e.WALFpi, &e.WALBytes,
				&e.MeanExecTime, &e.MinExecTime, &e.MaxExecTime, &e.StddevExecTime,
			); err != nil {
				return nil, fmt.Errorf("scan (pg<=12): %w", err)
			}
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}
	return entries, nil
}

func resolveDBNames(ctx context.Context, conn *pgx.Conn) (map[uint32]string, error) {
	rows, err := conn.Query(ctx, `SELECT oid, datname FROM pg_database`)
	if err != nil {
		return nil, fmt.Errorf("query pg_database: %w", err)
	}
	defer rows.Close()

	m := make(map[uint32]string)
	for rows.Next() {
		var oid uint32
		var name string
		if err := rows.Scan(&oid, &name); err != nil {
			return nil, fmt.Errorf("scan pg_database: %w", err)
		}
		m[oid] = name
	}
	return m, rows.Err()
}

func resolveUserNames(ctx context.Context, conn *pgx.Conn) (map[uint32]string, error) {
	rows, err := conn.Query(ctx, `SELECT oid, rolname FROM pg_roles`)
	if err != nil {
		return nil, fmt.Errorf("query pg_roles: %w", err)
	}
	defer rows.Close()

	m := make(map[uint32]string)
	for rows.Next() {
		var oid uint32
		var name string
		if err := rows.Scan(&oid, &name); err != nil {
			return nil, fmt.Errorf("scan pg_roles: %w", err)
		}
		m[oid] = name
	}
	return m, rows.Err()
}
