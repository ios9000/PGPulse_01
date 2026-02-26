package collector

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/ios9000/PGPulse_01/internal/version"
)

// progressCreateIndexSQL queries active CREATE INDEX / REINDEX operations.
// PGAM source: analiz2.php Q44.
const progressCreateIndexSQL = `
SELECT
    a.pid::text,
    COALESCE(a.datname, '') AS datname,
    COALESCE(p.relid::regclass::text, 'oid:' || p.relid::text) AS table_name,
    COALESCE(p.index_relid::regclass::text, '') AS index_name,
    p.command, p.phase,
    p.lockers_total, p.lockers_done,
    p.blocks_total, p.blocks_done,
    p.tuples_total, p.tuples_done,
    p.partitions_total, p.partitions_done
FROM pg_stat_progress_create_index p
JOIN pg_stat_activity a ON a.pid = p.pid`

// CreateIndexProgressCollector collects CREATE INDEX / REINDEX progress.
// PGAM source: analiz2.php Q44.
type CreateIndexProgressCollector struct {
	Base
}

// NewCreateIndexProgressCollector creates a new CreateIndexProgressCollector for the given instance.
func NewCreateIndexProgressCollector(instanceID string, v version.PGVersion) *CreateIndexProgressCollector {
	return &CreateIndexProgressCollector{
		Base: newBase(instanceID, v, 10*time.Second),
	}
}

// Name returns the collector's identifier.
func (c *CreateIndexProgressCollector) Name() string { return "progress_create_index" }

// Collect queries create index progress and returns metric points.
// Works on both primary and replica instances.
func (c *CreateIndexProgressCollector) Collect(ctx context.Context, conn *pgx.Conn, _ InstanceContext) ([]MetricPoint, error) {
	qCtx, cancel := queryContext(ctx)
	defer cancel()

	rows, err := conn.Query(qCtx, progressCreateIndexSQL)
	if err != nil {
		return nil, fmt.Errorf("progress_create_index: %w", err)
	}
	defer rows.Close()

	var points []MetricPoint

	for rows.Next() {
		var (
			pid, datname, tableName, indexName, command, phase string
			lockersTotal, lockersDone                         float64
			blocksTotal, blocksDone                           float64
			tuplesTotal, tuplesDone                           float64
			partitionsTotal, partitionsDone                   float64
		)
		if err := rows.Scan(
			&pid, &datname, &tableName, &indexName, &command, &phase,
			&lockersTotal, &lockersDone,
			&blocksTotal, &blocksDone,
			&tuplesTotal, &tuplesDone,
			&partitionsTotal, &partitionsDone,
		); err != nil {
			return nil, fmt.Errorf("progress_create_index scan: %w", err)
		}

		labels := map[string]string{
			"pid":        pid,
			"datname":    datname,
			"table_name": tableName,
			"index_name": indexName,
			"command":    command,
			"phase":      phase,
		}

		pct := completionPct(blocksDone, blocksTotal)

		points = append(points,
			c.point("progress.create_index.blocks_total", blocksTotal, labels),
			c.point("progress.create_index.blocks_done", blocksDone, labels),
			c.point("progress.create_index.tuples_total", tuplesTotal, labels),
			c.point("progress.create_index.tuples_done", tuplesDone, labels),
			c.point("progress.create_index.lockers_total", lockersTotal, labels),
			c.point("progress.create_index.lockers_done", lockersDone, labels),
			c.point("progress.create_index.partitions_total", partitionsTotal, labels),
			c.point("progress.create_index.partitions_done", partitionsDone, labels),
			c.point("progress.create_index.completion_pct", pct, labels),
		)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("progress_create_index rows: %w", err)
	}

	return points, nil
}

// progressBasebackupSQL queries active base backup operations.
// PGAM source: analiz2.php Q46.
const progressBasebackupSQL = `
SELECT
    a.pid::text,
    COALESCE(a.usename, '') AS usename,
    COALESCE(a.application_name, '') AS app_name,
    COALESCE(a.client_addr::text, '') AS client_addr,
    p.phase,
    p.backup_total, p.backup_streamed,
    p.tablespaces_total, p.tablespaces_streamed
FROM pg_stat_progress_basebackup p
JOIN pg_stat_activity a ON a.pid = p.pid`

// BasebackupProgressCollector collects base backup progress.
// PGAM source: analiz2.php Q46.
type BasebackupProgressCollector struct {
	Base
}

// NewBasebackupProgressCollector creates a new BasebackupProgressCollector for the given instance.
func NewBasebackupProgressCollector(instanceID string, v version.PGVersion) *BasebackupProgressCollector {
	return &BasebackupProgressCollector{
		Base: newBase(instanceID, v, 10*time.Second),
	}
}

// Name returns the collector's identifier.
func (c *BasebackupProgressCollector) Name() string { return "progress_basebackup" }

// Collect queries basebackup progress and returns metric points.
// Works on both primary and replica instances.
func (c *BasebackupProgressCollector) Collect(ctx context.Context, conn *pgx.Conn, _ InstanceContext) ([]MetricPoint, error) {
	qCtx, cancel := queryContext(ctx)
	defer cancel()

	rows, err := conn.Query(qCtx, progressBasebackupSQL)
	if err != nil {
		return nil, fmt.Errorf("progress_basebackup: %w", err)
	}
	defer rows.Close()

	var points []MetricPoint

	for rows.Next() {
		var (
			pid, usename, appName, clientAddr, phase string
			backupTotal, backupStreamed               float64
			tablespacesTotal, tablespacesStreamed     float64
		)
		if err := rows.Scan(
			&pid, &usename, &appName, &clientAddr, &phase,
			&backupTotal, &backupStreamed,
			&tablespacesTotal, &tablespacesStreamed,
		); err != nil {
			return nil, fmt.Errorf("progress_basebackup scan: %w", err)
		}

		labels := map[string]string{
			"pid":         pid,
			"usename":     usename,
			"app_name":    appName,
			"client_addr": clientAddr,
			"phase":       phase,
		}

		pct := completionPct(backupStreamed, backupTotal)

		points = append(points,
			c.point("progress.basebackup.backup_total", backupTotal, labels),
			c.point("progress.basebackup.backup_streamed", backupStreamed, labels),
			c.point("progress.basebackup.tablespaces_total", tablespacesTotal, labels),
			c.point("progress.basebackup.tablespaces_streamed", tablespacesStreamed, labels),
			c.point("progress.basebackup.completion_pct", pct, labels),
		)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("progress_basebackup rows: %w", err)
	}

	return points, nil
}

// progressCopySQL queries active COPY operations.
// PGAM source: analiz2.php Q47.
// Note: pg_stat_progress_copy does NOT have a phase column.
const progressCopySQL = `
SELECT
    a.pid::text,
    COALESCE(a.datname, '') AS datname,
    COALESCE(p.relid::regclass::text, 'oid:' || p.relid::text) AS table_name,
    p.command, p.type,
    p.bytes_processed, p.bytes_total,
    p.tuples_processed, p.tuples_excluded
FROM pg_stat_progress_copy p
JOIN pg_stat_activity a ON a.pid = p.pid`

// CopyProgressCollector collects COPY operation progress.
// PGAM source: analiz2.php Q47.
type CopyProgressCollector struct {
	Base
}

// NewCopyProgressCollector creates a new CopyProgressCollector for the given instance.
func NewCopyProgressCollector(instanceID string, v version.PGVersion) *CopyProgressCollector {
	return &CopyProgressCollector{
		Base: newBase(instanceID, v, 10*time.Second),
	}
}

// Name returns the collector's identifier.
func (c *CopyProgressCollector) Name() string { return "progress_copy" }

// Collect queries COPY progress and returns metric points.
// Works on both primary and replica instances.
func (c *CopyProgressCollector) Collect(ctx context.Context, conn *pgx.Conn, _ InstanceContext) ([]MetricPoint, error) {
	qCtx, cancel := queryContext(ctx)
	defer cancel()

	rows, err := conn.Query(qCtx, progressCopySQL)
	if err != nil {
		return nil, fmt.Errorf("progress_copy: %w", err)
	}
	defer rows.Close()

	var points []MetricPoint

	for rows.Next() {
		var (
			pid, datname, tableName, command, copyType string
			bytesProcessed, bytesTotal                 float64
			tuplesProcessed, tuplesExcluded             float64
		)
		if err := rows.Scan(
			&pid, &datname, &tableName, &command, &copyType,
			&bytesProcessed, &bytesTotal,
			&tuplesProcessed, &tuplesExcluded,
		); err != nil {
			return nil, fmt.Errorf("progress_copy scan: %w", err)
		}

		labels := map[string]string{
			"pid":        pid,
			"datname":    datname,
			"table_name": tableName,
			"command":    command,
			"type":       copyType,
		}

		pct := completionPct(bytesProcessed, bytesTotal)

		points = append(points,
			c.point("progress.copy.bytes_processed", bytesProcessed, labels),
			c.point("progress.copy.bytes_total", bytesTotal, labels),
			c.point("progress.copy.tuples_processed", tuplesProcessed, labels),
			c.point("progress.copy.tuples_excluded", tuplesExcluded, labels),
			c.point("progress.copy.completion_pct", pct, labels),
		)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("progress_copy rows: %w", err)
	}

	return points, nil
}
